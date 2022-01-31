/* ==================================================================================== *\
     caida_file_readers.go

     Reads CAIDA data files:
     - as-rel files
     - ppdc files
     - ip2as files

     Also read aliases file, and output some stats on the AS of interest.
\* ==================================================================================== */

package main

import (
        "strings"
        "log"
        "net"
        "sort"
        )

var ( // Read-only variables (set only once)
    as_neighbors map[string]map[string]interface{}; // From CAIDA AS rel file
    as_conesize map[string]int; // From CAIDA AS ppdc file (customers)
    max_conesize int;
    // Breaking down into /24
    as_24prefixes map[string]map[string]interface{}; // From CAIDA ip2as file
    prefix24_as map[string]string; // From CAIDA ip2as file
    // Not broken down into /24
    as_prefixes map[string]map[string]interface{}; // From CAIDA ip2as file
    prefix_as map[string]string; // From CAIDA ip2as file;
)

const (
    Customer = iota
    Peer
    Provider
    Unknown
)

/**
  * Given the AS of interest and an AS, returns the relationship of the AS
  * of interest with that AS in the form: 
  *         AS is a [peer/customer/provider] of AS of interest
  * 0: customer
  * 1: peer
  * 2: provider
  */
 func get_relationship (AS_interest, AS string) int {
    if len (as_neighbors) == 0 {
        log.Fatal ("[Warning]: as_neighbors is empty")
    }
    if neighbors, ok := as_neighbors[AS_interest]; ok {
        if rel, ok2 := neighbors[AS]; ok2 {
            return rel.(int)
        }
    }
    // In case we cannot determine the AS relationships, always prefer the ASes for which we know
    // 3 is superior to all three other values and will be discarded.
    return Unknown 
 }

/* ------------------------------------------------------------------------------- *\
                             Readers
\* ------------------------------------------------------------------------------- */

/**
 * Returns a mapping of an AS and all its neighbors.
 * Format:
 * <provider-as>|<customer-as>|-1
 * <peer-as>|<peer-as>|0
 */
func read_as_rel (filename string) map[string]map[string]interface{} {
    r := NewCompressedReader (filename)
    r.Open ()
    scanner := r.Scanner ()
    defer r.Close ()

    neighbor_ases := make (map[string]map[string]interface{})
    for scanner.Scan() {
        line := scanner.Text ()
        if !strings.Contains(line, "#") {
            s := strings.Split(line, "|")
            if s[2] == "0" {
                append_prefix (&neighbor_ases, s[0], s[1], Peer)
                append_prefix (&neighbor_ases, s[1], s[0], Peer)
            }
            if s[2] == "-1" {
                append_prefix (&neighbor_ases, s[0], s[1], Customer) // s[1] is a customer of s[0]
                append_prefix (&neighbor_ases, s[1], s[0], Provider) // s[0] is a provider of s[1]
            }
        }
    }
    return neighbor_ases
}

/**
 * Returns a set of all Tiers1 
 */
func read_providers (filename string) map[string]interface{} {

    all_ases := make (map[string]interface{})
    customers := make (map[string]interface{})

    r := NewCompressedReader (filename)
    r.Open ()
    scanner := r.Scanner ()
    defer r.Close ()

    for scanner.Scan() {
        line := scanner.Text ()
        if !strings.Contains(line, "#") {
            s := strings.Split(line, "|")
            rel := s[2]
            if rel == "-1" { // Only interest in p2c relationships
                customers[s[1]] = struct{}{}
            }
            all_ases[s[0]] = struct{}{}
            all_ases[s[1]] = struct{}{}
        }

    }

    log.Println ("Nb customers:", len (customers))
    tiers1 := difference (all_ases, customers)
    log.Println ("Nb tiers1: ", len (tiers1))
    return slice_to_map (tiers1)
}

// -------------------------------------------------------------------------------
/**
 * Returns a mapping of an AS and its associated /24 and raw prefixes.
 * Note: In the ip2as file of CAIDA, there can be negative ASes. This corresponds, I think, to IXP prefixes.
 */
func read_ip2as (filename string) (map[string]map[string]interface{}, map[string]string, map[string]map[string]interface{}, map[string]string) {
    defer recovery_function_fatal ()

    /* --- Read file --- */
    r := NewCompressedReader (filename)
    r.Open ()
    scanner := r.Scanner ()
    defer r.Close ()
    
    _as_prefixes := make (map[string]map[string]interface{})
    _prefix_as := make (map[string]string)
    for scanner.Scan() {
        line := scanner.Text ()
        if line == "" || strings.Contains (line, "#") || strings.Contains (line, ":"){ // IPv6 address
            continue
        }
        s := strings.Fields (line)
        prefix := s[0]
        AS := s[1]
        if AS == "-1" {
            continue
        }
        append_prefix (&_as_prefixes, AS, prefix)
        _prefix_as[prefix] = AS
    }


    /* --- Sort the prefixes in increasing order of their mask length --- */
    // Note: we need to order prefixes that way for the most specific prefixes (i.e., the longest mask lengths) 
    //           to be processed last. This is to make sure that the most specifics will be attributed to their real
    //           AS, and not to the provider of the AS.
    prefix_len := make (AS_weights, 0, len (_prefix_as))
    for prefix, _ := range _prefix_as {
        if strings.Contains (prefix, ":") { // IPv6 prefixes
                continue
        }
        prefix_len = append (prefix_len, &AS_weight{name: prefix, weight: extract_mask_length (prefix)})
    }
    sort.Sort (ByWeight{prefix_len}) // /8 is before /24


    /* --- Compute the /24 prefixes ---*/
    _as_24prefixes := make (map[string]map[string]interface{})
    _prefix24_as := make (map[string]string)

    for _, elem := range prefix_len {
        as := _prefix_as[elem.name]
        _, network, err := net.ParseCIDR (elem.name)
        if err != nil {
            panic ("PANIC")
        }
        subnets := get_subnets (network, 24)
        for _,subnet := range subnets {
            append_prefix (&_as_24prefixes, as, subnet.String ())
            _prefix24_as[subnet.String ()] = as // More specifics will override their provider.
        }
    }
    return _as_24prefixes, _prefix24_as, _as_prefixes, _prefix_as
}

// -------------------------------------------------------------------------------
/**
 * Reads a CAIDA customer cone file.
 * Returns a mapping of an AS and the size of its customer cone (nb prefixes in the customer cone of the AS).
 */
func read_customer_cone (filename string) map[string]int {
    if len (as_24prefixes) == 0 {
        log.Fatal ("as_24prefix not set")
    }

    /* --- Read file --- */
    r := NewCompressedReader (filename)
    r.Open ()
    scanner := r.Scanner ()
    defer r.Close ()

    _as_customers := make (map[string]map[string]interface{})
    const maxCapacity = 512*1024  
    buf := make([]byte, maxCapacity)
    scanner.Buffer(buf, maxCapacity)
    for scanner.Scan() {
        line := scanner.Text ()
        if line == "" || strings.Contains (line, "#"){
            continue
        }
        s := strings.Split (line," ")
        for _,customer := range s[1:] {
            append_prefix (&_as_customers, s[0], customer)
        }
    }
    if e := scanner.Err (); e != nil{
        log.Fatal (e.Error ())
    }

    /* --- Customer cone size --- */
    // key: an AS
    // value: all prefixes of its customer ASes
    as_customersPrefixes := make (map[string]map[string]interface{})
    for as, customers := range _as_customers {
        // Note: no need to purge cone on AS of interest.
        /* --- Nb of prefixes --- */
        for customer,_ := range customers {
            for prefix,_ := range as_24prefixes[customer] {
               append_prefix (&as_customersPrefixes, as, prefix)
            }
        }
    }
    min_size := MaxInt
    max_size := 0
    as_cc_size := make (map[string]int)
    for as, customers_prefixes := range as_customersPrefixes {
        as_cc_size[as] = len (customers_prefixes)
        min_size = min (as_cc_size[as], min_size)
        max_size = max (as_cc_size[as], max_size)
    }
    max_conesize = max_size
    return as_cc_size
}

// -------------------------------------------------------------------------------
func append_prefix (set *map[string]map[string]interface{}, args ...interface{}) {
    /* --- Check nb args --- */
    var l interface{}
    switch len (args) {
        case 2: l = struct{}{}
        case 3: l = args[2]
        default: log.Fatal ("Wrong number of arguments to function [append_prefix]")
    }
    as,_ := args[0].(string)
    prefix,_ := args[1].(string)

    if _, ok := (*set)[as]; ok {
        (*set)[as][prefix] = l
    } else {
        (*set)[as] = map[string]interface{}{prefix: l}
    }
}

/* ------------------------------------------------------------------------------- *\
                                 MISC.
\* ------------------------------------------------------------------------------- */

/**
 * For each AS, outputs:
 * - a file containing all addresses of that AS (new line separated)
 * - a file containing all routers of that AS (new line separated)
 */
func ases_main_stats (ases_interest_file, bdrmapit_file, alias_file, output_dir string) {
    /* --- Read files --- */
    addr_to_asn,_,_ := ReadSqlite (bdrmapit_file)
    router_addresses := read_aliases (alias_file)
    ases_interest,_ := read_whitespace_delimited_file (ases_interest_file)

    /* --- Routers --- */
    AS_routers := make (map[string]map[string]interface{})
    unknown := 0
    for router, addresses := range router_addresses {
        if AS, present := addr_to_asn.unsafe_get(addresses[0]); present {
            append_prefix (&AS_routers, AS, router, addresses)
        } else {
            for _, as_interest := range ases_interest {
                if AS == as_interest {
                    unknown++
                    break
                }
            }
        }
    }
    for _, AS := range ases_interest {
        to_print := create_safeset ()
        to_print.set = AS_routers[AS]
        to_print.write_to_file (output_dir + "/routers_"+ AS + ".txt")
    }
    log.Println ("Unknown router:", unknown)


    /* --- Addresses --- */
    AS_addresses := make (map[string]map[string]interface{})
    for addr, asn := range addr_to_asn.set {
        append_prefix (&AS_addresses, asn, addr)
    }
    for _, AS := range ases_interest {
        to_print := create_safeset ()
        to_print.set = AS_addresses[AS]
        to_print.write_to_file (output_dir + "/addresses_"+ AS + ".txt")
    }
}

// -------------------------------------------------------------------------------
/**
 * Input file format:
        node Ni:  ip1 ip2 ... ipn
 */
func read_aliases (alias_file string) map[string][]string {
    /* --- Read file --- */
    r := NewCompressedReader (alias_file)
    r.Open ()
    scanner := r.Scanner ()
    defer r.Close ()

    router_addresses := make (map[string][]string)
    for scanner.Scan() {
        line := scanner.Text ()
        if line == "" || strings.Contains (line, "#"){
            continue
        }
        s := strings.Fields (line)
        router := s[1]
        addresses := s[2:]
        router_addresses[router] = addresses
    }
    if e := scanner.Err (); e != nil{
        log.Fatal (e.Error ())
    }
    return router_addresses
}
