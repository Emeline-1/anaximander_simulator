/* ============================================================= *\
   rib.go

   Functions to parse the RIBs from the RouteViews and
   RIRs projects.
\* ============================================================= */

package main

import ("time"
      "log"
      "math/rand"
      "strconv"
      "os/exec"
      "io/ioutil"
      "net/http"
      "encoding/json"
      pool "github.com/Emeline-1/pool")

/** 
 * Read RIB tables and count the numbr of prefixes per collector in order to determine
 * which collectors are sound (> 800k entries)
 */
func count_ribs (output_filename, start, end string) {
   rand.Seed(time.Now().UnixNano())
   
   set := create_safeset ()

   /* --- With collectors --- */
   log.Print ("Retrieving collectors... ")
   collectors := broker_get_collectors ()
   log.Print ("Done")
   if collectors == nil {
      return
   }
   
   bgp_dump_counter := generate_dump_counter (set, start, end)
   pool.Launch_pool (32, collectors, bgp_dump_counter)

   log.Print ("Writing to file")
   log.Print ("Number of elements: " + strconv.Itoa (len (set.set)))
   set.write_to_file (output_filename)
}


/**
 * Launch the multi parsing of the RIBs
 */
func parse_ribs (ases_interest_file, collectors_file, output_dir, start, end string, heuristic int) {
   ases_interest,_ := read_whitespace_delimited_file (ases_interest_file)
   exec.Command("bash", "-c", "mkdir -p "+ output_dir + "/overlays").Run()
   exec.Command("bash", "-c", "mkdir -p "+ output_dir + "/forwarding_tables").Run()
   exec.Command("bash", "-c", "mkdir -p "+ output_dir + "/next-hop_AS").Run()
   exec.Command("bash", "-c", "mkdir -p "+ output_dir + "/collectors").Run()

   /* --- Heuristic specific processing --- */
   if heuristic == 1 {
      as_neighbors = read_as_rel (g_args.as_rel_file)
   }

   origin_set := create_safeset ()
   f := generate_RIB_parser  (origin_set, ases_interest, output_dir, start, end, heuristic)
   
   collectors,_ := read_newline_delimited_file (collectors_file, 0)
   log.Println ("Collectors: ", len (collectors))
   pool.Launch_pool (16, collectors, f)

   /* --- Post Processing (all RIBs have been parsed) --- */
   origin_set.write_to_file (output_dir + "/collectors/origin_ases.txt")
   build_merge_overlays (output_dir)

   // Gather all collectors' peers into one file
   cmd_s := "cat " + output_dir + "/collectors/BGP_peers* > " + output_dir + "/collectors/all_BGP_peers.txt"
   exec.Command("bash", "-c", cmd_s).Run()
   exec.Command("bash", "-c", "rm " + output_dir + "/collectors/BGP_peers*").Run()
}

/* ------------------------------------------------- *\
            Collectors operations
\* ------------------------------------------------- */

/**
 * Queries the CAIDA Broker HTTP API to retrieve meta-data about data available from different data providers.
 * More precisely, we query which collectors are available (ris and routeviews project only), and return them as a slice.
 * In case of error, returns a nil slice.
 */
func broker_get_collectors () (collectors []string) {

   /* --- Recover from panic in case the json has an unexpected format ---*/
   defer func() {
        if r := recover(); r != nil {
            log.Print ("[broker_get_collectors]: Problem while parsing response JSON")
            collectors = nil // With named return value, return value will be nil, which is what we expect.
            return
        }
    }()

   /* --- Query data broker --- */
   resp, err := http.Get("https://broker.bgpstream.caida.org/v2/meta/collectors")
   if err != nil {
      log.Print ("[broker_get_collectors]: " + err.Error ())
      return collectors //nil slice
   } 
   defer resp.Body.Close()
   body, err := ioutil.ReadAll(resp.Body) //resp.Body is an io.ReadCloser
   if err != nil {
      log.Print ("[broker_get_collectors]: " + err.Error ())
      return collectors //nil slice
   }

   /* --- Traverse the json --- */
   var result map[string]interface{}
   json.Unmarshal([]byte(body), &result)

   object := result["data"]
   result = object.(map[string]interface{}) // Type assertion (Acces to underlying data of the interface)
   object = result["collectors"]
   result = object.(map[string]interface{})

   for collector, value := range result {
      collector_data := value.(map[string]interface{})
      project := collector_data["project"]
      project_name := project.(string)
      if project_name == "routeviews" || project_name == "ris"{
         collectors = append (collectors, collector)
      }
   }
   return collectors
}

/* --------------------------------------- *\
 *             MISC.
\* --------------------------------------- */

/**
 * Given a slice of collectors, assign a number to each one of them and return
 * the corresponding mapping. 
 */
func assign_numbers (collectors []string) map[string]int {
   m := make (map[string]int, len (collectors))
   for i,c := range collectors {
      m[c] = i
   }
   return m
}