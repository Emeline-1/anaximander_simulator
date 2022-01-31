/* ==================================================================================== *\
    greedy_anaximander.go

    Alternative scheduling for Anaximander:
    ---------------------------------------
    The simulation (for an AS of interest) is performed in parallel, i.e., all ASes at 
    the same time. The exploration of the ASes is momentarily halted at the first useless
    probes, to get back to it at a later time.
    
    Note that the notion of parallelism here has nothing to do with code execution, but has
    to do with the scheduling of the probes.

    This scheduling performs worse to Anaximander's sequential scheduling.

\* ==================================================================================== */
package main

import (
    "strings"
    "strconv"
    "path/filepath"
    "os/exec"
    )

// -------------------------------------------------------------------------------
func generate_anaximander_greedy (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn *SafeSet, output_file string, router_to_addrs *SafeSet) func (string){
    return func (as_interest string) {
        anaximander_greedy (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, as_interest, trim_suffix (output_file, ".txt") + "_" + as_interest + ".txt", router_to_addrs)
    }
}

// -------------------------------------------------------------------------------
/**
 * Perform the simulation on the traces.
 * The simulation is performed in parallel, i.e., all ASes at the same time. This allows to see how the real Anaximander performs in the wild.
 */
func anaximander_greedy (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn *SafeSet, as_interest string, output_file string, routers *SafeSet) {

    adjs, multi_adjs, addresses, routers = filterAS (as_interest, adjs, multi_adjs, addresses, routers, addr_to_asn) // Keep only data relevant to AS of interest.
    output_msg ("raw.txt", as_interest, len (adjs.set), len (multi_adjs.set), len (addresses.set), len (routers.set))
    
    /* --- Probing strategy --- */
    destinations := get_keys (&traces.set)
    sorted_destinations, limits_neighbors := read_strategy (destinations, as_interest) 
    
    /* --- Build the list of ASes to probe --- */
    neighbor_start := 0
    ases_status := make ([]*AS_status, 0, 10)
    for i, AS := range limits_neighbors {
        if AS.limit == neighbor_start {
            continue
        }
        ases_status = append (ases_status, &AS_status {asn: AS.asn, start: neighbor_start, end: AS.limit, curr_probe:neighbor_start, plateau: 0, stopped: false, position: i})
        neighbor_start = AS.limit
    }

    /* --------------------------- *\
               SIMULATION
    \* --------------------------- */
    discovered_adjs, discovered_multi_adjs, discovered_addresses, discovered_routers := create_safeset (), create_safeset (), create_safeset (), create_safeset ()
    in_progress_discovered_routers := create_safeset () // A router is considered as discovered iif we have discovered at least 2 of its addresses. In 'discovered_routers', we only store the routers with 2 or more addresses.
    results := create_safeset ()
    global_counter := 0
    prev_adjs, prev_addresses, prev_routers := 0,0,0
    stopped_ases := 0 // The number of ASes whose probing has stopped (either because we reached a plateau, or because the whole AS has been probed)
    destination := ""
    
    iteration := 0
    for stopped_ases != len (ases_status) {
        for _, as_status := range ases_status { // Loop over the ASes
            discovery := true

            for discovery {
                destination, stopped_ases = launch_as_probing (sorted_destinations, as_status, stopped_ases)
                if destination == "" { // Nothing to probe for current AS, carry on to next AS (stopped AS, or AS completely probed)
                    break
                }
                trace,_ := traces.get (destination) // Missing traces will be treated as traces that did not yield any discovery
            
                process_trace (trace, as_interest, discovered_adjs, discovered_multi_adjs, discovered_addresses, discovered_routers, in_progress_discovered_routers)
                
                new_adjs, new_addresses, new_routers := len (discovered_adjs.set), len (discovered_addresses.set), len (discovered_routers.set)

                if new_adjs != prev_adjs || new_addresses != prev_addresses || new_routers != prev_routers { 
                    /* --- Discovery --- */
                    discovered := []string {
                        strconv.FormatFloat (float64 (len (discovered_adjs.set))/float64 (len (adjs.set)), 'f', 4, 32),
                        strconv.FormatFloat (float64 (len (discovered_multi_adjs.set))/float64 (len (multi_adjs.set)), 'f', 4, 32),
                        strconv.FormatFloat (float64 (len (discovered_addresses.set))/float64 (len (addresses.set)), 'f', 4, 32),
                        strconv.FormatFloat (float64 (len (discovered_routers.set))/float64 (len (routers.set)), 'f', 4, 32),
                    }
                    results.unsafe_add (strconv.Itoa (global_counter), strings.Join (discovered, " "))
                    prev_adjs, prev_addresses, prev_routers = new_adjs, new_addresses, new_routers
                    as_status.plateau = 0
                } else {
                    if as_status.position != 0 { // Don't stop probing /24 internal prefixes.
                        discovery = false
                    }
                    /* --- No discovery --- */
                    as_status.plateau++
                    if float64(as_status.plateau)/float64(as_status.end - as_status.start) > g_args.threshold_parameter {
                        if as_status.stopped == false { // Check if AS has not already been stopped because it was its last probe. In which case don't increment the number of stopped ASes, or it will be false.
                            as_status.stopped = true
                            stopped_ases++
                        }
                        break // To stop probing current batch.
                    }
                }
                global_counter++
            }
        }
        iteration++
    }

    /* --------------------------- *\
           WRITE RESULTS
    \* --------------------------- */
    /* --- Simulation result --- */
    results.write_to_file (output_file)
    dir, filename := filepath.Split (output_file)
    cmd := "sort -t\\  -nk1 " + output_file + " > " + dir + "sorted_" + filename
    err := exec.Command("bash", "-c", cmd).Run()
    if err != nil {
        panic ("[anaximander]: Problem while sorting output file: " + err.Error ())
    }
    exec.Command ("rm", output_file).Run ()
}