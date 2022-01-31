/* ============================================================= *\
   readers.go

   - Readers objects to read warts files and sqlite files.
   - Methods to process warts files and sqlite files.
   - Misc functions to read diverse files.
\* ============================================================= */
package main

import (
  "strings"
  "bufio"
  "os/exec"
  "os"
  "log"
  "database/sql"
  "strconv"
  "fmt"
  "io"
  "errors"
  "compress/bzip2"
  "compress/gzip"
  _ "github.com/mattn/go-sqlite3"
  pool "github.com/Emeline-1/pool")
// the underscore import is used for the side-effect of registering the sqlite3 driver 
// as a database driver in the init() function, without importing any other functions

/* ------------------------------------------------------- *\
 *                     WARTS READER
\* ------------------------------------------------------- */
type WartsReader struct{
  filename string;
  output *strings.Reader
}

func NewWartsReader (filename string) *WartsReader {
  return &WartsReader{
    filename: filename,
  }
}

func (r *WartsReader) Open () {
  var cmd string
  if strings.HasSuffix(r.filename, ".gz") {
    cmd = "gunzip -c " + r.filename + " | sc_tnt -d2"
  } else {
    cmd = "sc_tnt -d2 " + r.filename
  }
  out, err := exec.Command("bash", "-c", cmd).Output()
  if err != nil {
    panic ("[WartsReader.Open]: Problem while reading warts file " + r.filename + ": " + err.Error ())
  }
  r.output = strings.NewReader (string (out))
}

func (r *WartsReader) Scanner () *bufio.Scanner {
  return bufio.NewScanner(r.output)
}

type Trace []Hop

func (t Trace) String () string {
  var r string
  for _,hop := range t {
    r+= fmt.Sprintf ("%v", hop) + "\n"
  }
  return r
}

func (trace Trace) prune_dups () *Trace {
  prev := ""
  new_trace := make (Trace, 0, len (trace))
  for _, hop := range trace {
    if prev != hop.addr {
      new_trace = append (new_trace, hop)
    }
    prev = hop.addr
  }
  return &new_trace
}

type Hop struct {
  addr string; // IP address
  asn string; // The ASN assigned by bdrmapit to that address.
  probe_ttl int; // The TTL of the traceroute probe
  ingress bool;
  egress bool; //If neither ingress nor egress is set, this is a hop inside the AS.
  router string; // The router identifier this address belongs to.
}

func (h Hop) String() string {
  return fmt.Sprintf("[%v (AS: %v) - Ingress:%v - Hop:%v ]", h.addr, h.asn, h.ingress, h.probe_ttl)
}

/**
 * Parse warts files, and annotates hops with their AS and router thanks to bdrmapit output.
 */
func parse_warts () (*SafeSet, *SafeSet, *SafeSet, *SafeSet, *SafeSet, *SafeSet, *SafeSet){
  /* --- Read bdrmapit sqlite file --- */
  log.Println (" ---- Bdrmapit stats ---- ")
  addr_to_asn, router_to_asn, addr_to_router := ReadSqlite (g_args.bdrmapit_file)
  log.Println ("Nb of addresses: ", len (addr_to_asn.set))

  /* --- Read warts --- */
  files := pool.Get_directory_files (g_args.warts_directory)
  if files == nil {
    log.Fatal ("[read]: Problem while parsing warts directory")
  }

  traces, adjs, multi_adjs, addresses, target_to_vp := create_safeset (), create_safeset (), create_safeset (), create_safeset (), create_safeset ()
  warts_parser := generate_warts_parser (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, addr_to_router)
  log.Println ("Reading warts files...")
  pool.Launch_pool (32, *files, warts_parser)

  log.Println (" ---- Warts stats ---- ")
  log.Println ("Number of traces: ", len (traces.set))
  log.Println ("Number of adjs: ", len (adjs.set))
  log.Println ("Number of multi_adjs: ", len (multi_adjs.set))
  log.Println ("Number of addresses (excluding private addresses): ", len (addresses.set))
  log.Println ("Number of routers: ", len (router_to_asn.set))

  return traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, router_to_asn
}

/**
 * Generate a fonction to parse a warts file
 * OUTPUT:
 * - traces: map of the form: "source_dest" -> Trace{}
 * - adjs: set of all adjacencies in the form "ip1_ip2" (usefull for percentage of discovered links/IPs) 
 * - multi_adjs: set of all multiple hops adjencies in the form "ip1_ip2" (same)
 * - addresses: set of all encountered valid routable addresses (usefull for percentage of discovered addresses for simulation)
 *
 * INPUT:
 * - addrToAsn: mapping of the ASN assigned to the address by bdrmapit.
 */
func generate_warts_parser (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, addr_to_router *SafeSet) func (string) {
  
  return func (file_name string) {
    defer recovery_function ()

      reader := NewWartsReader (file_name)
      reader.Open ()
      scanner := reader.Scanner ()

      var source, dest string
      var trace *Trace
      for scanner.Scan() {
      line := scanner.Text()
      
      if strings.Contains (line, "#") || strings.Contains (line, "DUMP"){
        continue
      }
      /* --- End of trace --- */
      if line == "" {
        commit_trace (source, dest, trace, traces, adjs, multi_adjs, target_to_vp)
      } else if strings.Contains (line, "from"){ /* --- New trace --- */
        source, dest = get_source_dest (line)
        tmp := make (Trace, 0, 16) // 16 default trace length approximately. 
        trace = &tmp
      } else {
        split := strings.Fields (line)
        probe_ttl,_ := strconv.Atoi (split[0])
        addr := split[1]
        if strings.Contains (line, "rsvd") { // Private address
          continue
        }
        if addr == "*" { // Unresponsive hops
          continue
        }
        if addr == dest { 
          continue
        }
        addresses.add (addr) 
        /* Get AS of address */
        asn_i, ok := addr_to_asn.unsafe_get (addr)
        var asn string
        var t bool
        if !ok {
          asn = "-1"
        } else {
          asn, t = asn_i.(string)
          if !t {
            log.Fatal ("[generate_warts_parser]: unexpected type:", fmt.Sprintf("%T", asn_i))
          }
        }
        /* Get router of address */
        router_i, ok := addr_to_router.unsafe_get (addr)
        var router string
        if !ok {
          router = "-1" // Address not present in bdrmapit output
        } else {
          router,_ = router_i.(string)
        }
        hop := Hop{
          addr: addr,
          asn: asn, 
          probe_ttl: probe_ttl,
          ingress: false,
          egress: false,
          router: router,
        }
        *trace = append (*trace, hop)
      }
    }
  }
}

/**
 * Function called at the end of the parsing of a trace, to sanitize the trace and commit it.
 * - Prune duplicates
 * - Prune loops
 * - Create adjs and multiple adjs.
 * - Assign ingresses and egresses.
 *
 * Those traces will be kept in a map "source_dest" -> Trace{}, for the simulation where we launch probes
 * ourselves that will follow those traces.
 */
func commit_trace (source, dest string, trace *Trace, traces, adjs, multi_adjs, target_to_vp *SafeSet) {
  trace = trace.prune_dups ()
  for i, hop := range *trace {
    if i == len (*trace) - 1 {
      break
    }
    /* --- Adjencies --- */
    next_hop := (*trace)[i+1]
    distance := next_hop.probe_ttl - hop.probe_ttl
    if distance == 1 {
      adjs.add (hop.addr+"_"+next_hop.addr)
    } 
    if distance > 1 {
      multi_adjs.add (hop.addr+"_"+next_hop.addr)
    }
    /* --- AS borders --- */
    if hop.asn != next_hop.asn {
      (*trace)[i].egress = true
      (*trace)[i+1].ingress = true
    } 
  }
  dest_24 := strings.Join (strings.Split (dest, ".")[:3], ".")+".0/24"
  traces.add (dest_24, trace)
  target_to_vp.add (dest_24, source)
}

/* ------------------------------------------------------- *\
 *                    SQLITE READER
\* ------------------------------------------------------- */
type SqliteReader struct{
  filename string;
  rows *sql.Rows
}

func NewSqliteReader (filename string) *SqliteReader {
  return &SqliteReader{
    filename: filename,
  }
}

func (r *SqliteReader) Open () {
  database, _ := sql.Open("sqlite3", r.filename)
  defer database.Close ()

  rows, err := database.Query("SELECT * FROM annotation")
  if err != nil {
    panic ("[SqliteReader.Open]: problem while reading sqlite file")
  }
  r.rows = rows
}

func (r *SqliteReader) Scanner () *sql.Rows{
  return r.rows
}

func ReadSqlite (filename string) (*SafeSet, *SafeSet, *SafeSet){
  defer recovery_function ()
  reader := NewSqliteReader (filename)
  reader.Open ()
  rows := reader.Scanner ()

  columns,_ := rows.Columns ()
  nb_columns := len (columns)
  if nb_columns != 10 && nb_columns != 8 {
    log.Fatal ("[ReadSqlite]: wrong file format.")
  }

  addr_to_asn := create_safeset ()
  router_to_asn := create_safeset ()
  addr_to_router := create_safeset ()

  var addr string
  var router string
  var asn int
  var org string
  var conn_asn int
  var conn_org string 
  var rtype int 
  var itype int
  var prouter string 
  var pasn int 
  // Attributes: addr - router - asn - org - conn_asn - conn_org - rtype - itype
  // New attributes: addr - router - asn - org - conn_asn - conn_org - rtype - itype - prouter - pasn
  // prouter is the preceding router
  // pasn is the ASN attributed to this router
  // pasn should always be equal to conn_asn, or there is something wrong somewhere
  cnt := 0
  for rows.Next() {
    if nb_columns == 10 {
      rows.Scan(&addr, &router, &asn, &org, &conn_asn, &conn_org, &rtype, &itype, &prouter, &pasn)
    } else {
      rows.Scan(&addr, &router, &asn, &org, &conn_asn, &conn_org, &rtype, &itype)
    }
    

    addr_to_asn.unsafe_add (addr, strconv.Itoa (asn))
    m := re_ip.FindStringSubmatch (router)
    if m == nil { // We check field 'router' is not an IP address, in which case it means this address wasn't matched to a router.
      router_to_asn.unsafe_add (router, strconv.Itoa (asn))
      addr_to_router.unsafe_add (addr, router)
    } else {
      addr_to_router.unsafe_add (addr, "")
    }
    
    if asn == -1 {
      cnt++
    }
  }
  log.Println ("There are", cnt, "addresses for which an AS wasn't found.")
  return addr_to_asn, router_to_asn, addr_to_router
}

/* ------------------------------------------------------- *\
 *               Compressed File Reader
\* ------------------------------------------------------- */

type CompressedReader struct{
  filename string;
  fp io.ReadCloser;
  decompressed io.Reader;
  to_close io.ReadCloser; // All because bzip2.Reader has no Close method --'
}

func NewCompressedReader (filename string) *CompressedReader {
  return &CompressedReader{
    filename: filename,
  }
}

func (r *CompressedReader) Open () error {
  var err error
  r.fp, err = os.Open(r.filename) // Read only
  if err != nil {
    return errors.New ("[CompressedReader]: " + err.Error() + " " + r.filename)
  }

  if strings.HasSuffix(r.filename, ".gz") {
    r.to_close,_ = gzip.NewReader (r.fp)
    r.decompressed = r.to_close
  } else if strings.HasSuffix (r.filename, ".bz2"){
    r.decompressed = bzip2.NewReader (r.fp)
  } else {
    r.decompressed = r.fp
  }
  return nil
}

func (r *CompressedReader) Scanner () *bufio.Scanner {
  return bufio.NewScanner(r.decompressed)
}

func (r *CompressedReader) Close () {
  r.fp.Close ()
  if r.to_close != nil {
    r.to_close.Close ()
  }
}

/* ------------------------------------------------------- *\
 *                          Misc.
\* ------------------------------------------------------- */

func read_whitespace_delimited_file (filename string) ([]string, error) {
  r := NewCompressedReader (filename)
  err := r.Open ()
  if err != nil {
    return []string{}, err
  }
  scanner := r.Scanner ()
  defer r.Close ()

  scanner.Scan ()
  line := scanner.Text ()

  return strings.Fields (line), nil
}

/**
 * Returns the lines of nexline-delimited file, selecting the corresponding field.
 */
func read_newline_delimited_file (filename string, field int) ([]string, error) {
  r := NewCompressedReader (filename)
  err := r.Open ()
  if err != nil {
    return []string{}, err
  }
  scanner := r.Scanner ()
  defer r.Close ()

  s := make ([]string, 0, 43)
  for scanner.Scan () {
    s = append (s, strings.Fields (scanner.Text ())[field])
  }
  return s, nil
}

/**
 * Returns a slice of the source IP addresses of the different VPs.
 * File format: VP_name source_IP AS
 */
func read_vps_file (filename string) ([]string, error) {
  return read_newline_delimited_file (filename, 1)
}

/**
 * Given a collector file containg all its overlays (overlays are new-line separated,
 * prefixes in an overlay are white-space separated):
 *
 * ex:
 *    ip1 ip2 ip3 ip4
 *    ip5 ip6
 *
 * Build the following map:
 *
 * ip1 -> (ip1, ip2, ip3, ip4)
 * ip2 -> (ip1, ip2, ip3, ip4)
 * ip3 -> (ip1, ip2, ip3, ip4)
 * ip4 -> (ip1, ip2, ip3, ip4)
 * ip5 -> (ip5, ip6)
 * ip6 -> (ip5, ip6)
 *
 * In the map returned, there are as many keys as there are prefixes in the file.
 * There are as many values as there are overlays in the file (not copies)
 */
func read_overlay_file (filename string) map[string]map[string]interface{} {
  r := NewCompressedReader (filename)
  r.Open ()
  scanner := r.Scanner ()
  defer r.Close ()

  m := make (map[string]map[string]interface{})
  for scanner.Scan () {
    overlays := strings.Fields (scanner.Text ())
    overlays_map := slice_to_map (overlays)

    for _, overlay := range overlays {
      m[overlay] = overlays_map
    }
  }
  return m
}

/**
 * Reads a nextAS file in the format:
 *   prefix next_AS
 * and returns a prefix to next-AS mapping and a next-AS to prefixes mapping.
 */
func read_nextAS_file (filename string) (map[string]string, map[string]map[string]interface{}) {
  r := NewCompressedReader (filename)
  r.Open ()
  scanner := r.Scanner ()
  defer r.Close ()

  prefix_to_nextAS := make (map[string]string)
  nextAS_to_prefixes := make (map[string]map[string]interface{})

  for scanner.Scan () {
    line := strings.Fields (scanner.Text ())
    prefix_to_nextAS[line[0]] = line[1]
    append_prefix (&nextAS_to_prefixes, line[1], line[0])
  }
  return prefix_to_nextAS, nextAS_to_prefixes
}

func get_source_dest (line string) (source, dest string) {
  m := re_source_dest.FindStringSubmatch (line)
  if m != nil {
    source = m[1]
    dest = m[2]
    return
  }
  return
}


