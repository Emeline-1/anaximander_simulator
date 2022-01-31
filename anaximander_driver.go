/* ==================================================================================== *\
     anaximander_driver.go

     Implements the _Anaximander Simulator_.

     For the simulator, NOTE on how to compute: 
     - the number of useful probes (probes that discovered something): 'wc -l sorted_simulation_as.txt'
     - the total numbers of probes launched: 'cat limits.txt': the last number written (for the AS) is the total 
                nb of probes (cannot look at the final line of sorted_simulation_as.txt, as it displays 
                the last probe that discovered something, but not necessarily the last probe that was launched).
                For directed probing, there is no limit file, just launch 'wc -l directed_prefixes_as.txt'.
     - the final discovery percentage: 'tail sorted_simulation_as.txt': look at the final percentage displayed

\* ==================================================================================== */

package main

import (
        "strings"
        "log"
        "path/filepath"
        "os/exec"
        "time"
        "fmt"
        "math/rand"
        pool "github.com/Emeline-1/pool"
        )

/* ============================================================ *\
                   ANAXIMANDER SIMULATOR
\* ============================================================ */

func output_msg (args ...interface{}) {
    if output_on {
        fmt.Println (args...)
    }
}

type generate_function func (*SafeSet,*SafeSet,*SafeSet,*SafeSet,*SafeSet,*SafeSet,string,*SafeSet) (func(string))
/**
 * Allows to choose the type of simulation that must be performed (sequential vs. parallel vs. greedy)
 */
var generate_functions []generate_function = []generate_function {
    generate_anaximander_sequential,
    generate_anaximander_parallel,
    generate_anaximander_greedy,
}

// -------------------------------------------------------------------------------
/**
 * Launches the simulation in parrallel on the ASes of interest.
 */
func launch_anaximander_simulation (break_prefix bool, output_file string, simulation_mode int) {

    /* ---------------------------------------------------- *\
       READING SIMULATION DATA and setting Global Variables
    \* ---------------------------------------------------- */
    rand.Seed(time.Now().UnixNano())
    start := time.Now()
    traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, router_to_asn := parse_warts ()
    log.Printf("Parsing TNT data took %s", time.Since(start))

    start = time.Now()

    if simulation_mode != 0 { // need to read that for alternative scheduling (greedy or parallel).
        as_neighbors = read_as_rel (g_args.as_rel_file)
        as_24prefixes, prefix24_as, as_prefixes, prefix_as = read_ip2as (g_args.ip2as_file)
        if break_prefix {
            as_to_prefixes, prefix_to_as = as_24prefixes, prefix24_as
        } else {
            as_to_prefixes, prefix_to_as = as_prefixes, prefix_as
        }
        as_conesize = read_customer_cone (g_args.ppdc_file) // Must come afterwards.
        log.Printf("Parsing CAIDA files took %s", time.Since(start))
    }
    
    vps,_ = read_vps_file (g_args.vps_file)
    
    /* ----------------------- *\
             SIMULATION
    \* ----------------------- */
    ases_interest,_ := read_whitespace_delimited_file (g_args.ases_interest_file)
    
    f := generate_functions[simulation_mode] (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, output_file, router_to_asn)
    log.Println ("Launching simulation...")
    pool.Launch_pool (1, ases_interest, f) //pool.Launch_pool (len (ases_interest), ases_interest, f)

    /* --- Gather limits file if any --- */
    output_dir := filepath.Dir (output_file)
    cmd := "cat " + output_dir + "/*limits_reduction.txt > " + output_dir + "/all_reduction.txt"
    exec.Command("bash", "-c", cmd).Run() //Normal if there is an error when there is no limits file.
    exec.Command("bash", "-c", "rm " + output_dir + "/*limits_reduction.txt").Run()
    // Note: An attempt to fetch a map value with a key that is not present in the map will return the zero value 
    // for the type of the entries in the map.
    // This means that some neighbors (who don't have prefixes) will appear in the limit file as two equal consecutive values.
}

// -------------------------------------------------------------------------------
func filterAS (AS string, adjs, multi_adjs, addresses, router_to_asn, addr_to_asn *SafeSet) (*SafeSet, *SafeSet, *SafeSet, *SafeSet) {
    filtered_adjs := create_safeset ()
    filtered_multi_adjs := create_safeset ()
    filtered_addresses := create_safeset ()
    filtered_routers := create_safeset ()

    for addr1_addr2 := range adjs.set {
        s := strings.Split (addr1_addr2, "_")
        as1,_ := addr_to_asn.unsafe_get (s[0])
        as2,_ := addr_to_asn.unsafe_get (s[1])
        if as1 == AS || as2 == AS {
            filtered_adjs.unsafe_add (addr1_addr2)
        }
    }

    for addr1_addr2 := range multi_adjs.set {
        s := strings.Split (addr1_addr2, "_")
        as1,_ := addr_to_asn.unsafe_get (s[0])
        as2,_ := addr_to_asn.unsafe_get (s[1])
        if as1 == AS || as2 == AS {
            filtered_multi_adjs.unsafe_add (addr1_addr2)
        }
    }

    for addr := range addresses.set {
        if as, _ := addr_to_asn.unsafe_get (addr); as == AS {
            filtered_addresses.unsafe_add (addr)
        }
    }

    for router, asn := range router_to_asn.set {
        if asn == AS {
            filtered_routers.unsafe_add (router)
        }
    }

    return filtered_adjs, filtered_multi_adjs, filtered_addresses, filtered_routers
}

// -------------------------------------------------------------------------------
/**
 * Given a trace and an AS of interest, record the new discovered elements in their corresponding
 * set. 
 * Also returns the number of addresses that belonged to the AS of interest. This represents if the trace
 * was successfull or not (and allows to sort them based on the number of addresses).
 */ 
func process_trace (trace_i interface{}, as_interest string, discovered_adjs, discovered_multi_adjs, discovered_addresses, discovered_routers, in_progress_discovered_routers *SafeSet) int {
    if trace, t := trace_i.(*Trace); t {
        discovery := 0
        /* --- Process trace --- */
        for i, hop := range *trace {
            if hop.asn == as_interest {
                discovery++
                // --- Address
                discovered_addresses.unsafe_add (hop.addr)
                // --- Router
                if hop.router != "" { // Address belongs to a router
                    addresses_i, _ := in_progress_discovered_routers.unsafe_get (hop.router)
                    addresses, t := addresses_i.(map[string]struct{}) // Type assertion
                    if !t { // Equivalent to case len (addresses) == 0 
                        in_progress_discovered_routers.unsafe_append (hop.router, hop.addr)
                    }
                    if len (addresses) == 1 {
                        // Check the address is different from the one we already recorded
                        if _, ok := addresses[hop.addr]; !ok {
                            discovered_routers.unsafe_add (hop.router)
                            in_progress_discovered_routers.unsafe_append (hop.router, hop.addr)
                        }
                    }
                    // Note: we only need to store two of the addresses of the routers (reduce memory footprint).
                }
                
            }
            if i == len (*trace) - 1 { // Last hop
                break
            }
            if hop.asn != as_interest  && (*trace)[i+1].asn != as_interest { // Take into account incoming links.
                continue
            }
            /* --- Adjacencies --- */
            next_hop := (*trace)[i+1]
            distance := next_hop.probe_ttl - hop.probe_ttl
            if distance == 1 {
                discovered_adjs.unsafe_add (hop.addr+"_"+next_hop.addr)
            } 
            if distance > 1 {
                discovered_multi_adjs.unsafe_add (hop.addr+"_"+next_hop.addr)
            }
        }
        return discovery
    } else {
        return 0
    }
}