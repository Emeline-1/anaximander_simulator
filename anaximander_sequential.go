/* ==================================================================================== *\
   sequential_anaximander.go

   Implementation of Anaximander's scheduling:
   ------------------------------------------
   The simulation (for an AS of interest) is performed sequentially, i.e., one AS after the other. 
   This allows to see for plateaux between ASes.

   See parallel_anaximander.go or greedy_anaximander.go for another type of scheduling.
   
\* ==================================================================================== */
package main

import (
    "strings"
    "strconv"
    "path/filepath"
    "os/exec")

// -------------------------------------------------------------------------------
func generate_anaximander_sequential (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn *SafeSet, output_file string, router_to_addrs *SafeSet) func (string){
  return func (as_interest string) {
    anaximander_sequential (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, as_interest, trim_suffix (output_file, ".txt") + "_" + as_interest + ".txt", router_to_addrs)
  }
}

// -------------------------------------------------------------------------------
/**
 * Perform the simulation on the traces.
 * The simulation is performed sequentially, i.e., one AS after the other. This allows to see for plateaux between ASes.
 */
func anaximander_sequential (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn *SafeSet, as_interest string, output_file string, routers *SafeSet) {

  adjs, multi_adjs, addresses, routers = filterAS (as_interest, adjs, multi_adjs, addresses, routers, addr_to_asn) // Keep only data relevant to AS of interest.
  output_msg ("raw.txt", as_interest, len (adjs.set), len (multi_adjs.set), len (addresses.set), len (routers.set))
  
  /* --- Probing strategy --- */
  destinations := get_keys (&traces.set)
  sorted_destinations, limits_neighbors := read_strategy (destinations, as_interest)
 
  /* --- Record limits between neighbors --- */
  w, file := new_bufio_writer (trim_suffix (output_file, ".txt") + "_limits_reduction.txt")
  defer file.Close ()
  w.WriteString (as_interest + " ")
  
  /* --------------------------- *\
             SIMULATION
  \* --------------------------- */
  discovered_adjs, discovered_multi_adjs, discovered_addresses, discovered_routers := create_safeset (), create_safeset (), create_safeset (), create_safeset ()
  in_progress_discovered_routers := create_safeset () // A router is considered as discovered iif we have discovered at least 2 of its addresses. In 'discovered_routers', we only store the routers with 2 or more addresses.
  results := create_safeset ()
  successful_traces := create_safeset ()

  global_counter := 0
  prev_adjs, prev_addresses, prev_routers := 0,0,0

  /* --- Loop over neighbors --- */
  neighbor_start := 0
  total_length := 0
  missing_traces := 0
  false_positives := 0
  for _, AS := range limits_neighbors {
    neighbor_stop := AS.limit
    if neighbor_stop == neighbor_start {
      continue
    }
    current_plateau_length := 0
    stop := false
    /* --- Loop over prefixes of neighbors --- */
    k := neighbor_start
    for ; k < neighbor_stop; k++ {
      destination := sorted_destinations[k]
      trace, present := traces.get (destination)
      if !present {
        missing_traces++ // Missing traces are treated as traces that did not yield any discovery.
      }
      discovery := process_trace (trace, as_interest, discovered_adjs, discovered_multi_adjs, discovered_addresses, discovered_routers, in_progress_discovered_routers)
      if discovery != 0 {
        successful_traces.unsafe_add (destination, discovery)
      } else {
        false_positives++
      }

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
        current_plateau_length = 0
      } else {
        /* --- No discovery --- */
        current_plateau_length++
        if float64(current_plateau_length)/float64(neighbor_stop-neighbor_start) > g_args.threshold_parameter {
          stop = true
        }
      }
      global_counter++
      
      /* --- Stop probing and go to next neighbor --- */
      if stop {
        k++ // Necessary when we break the loop, because in case of breaking, there is no k+1 that is done.
        break
      }
    } // End of loop on the neighbor's prefixes - end of current neighbor

    // Record neighbor's new limit
    neighbor_total_length := k - neighbor_start // No k+1, because at end of loop, we already exceeded the limit by 1.
    total_length += neighbor_total_length
    w.WriteString (strconv.Itoa (total_length) + " ")
    
    neighbor_start = neighbor_stop
  } // End of loop on neighbors
  w.WriteString ("\n")
  w.Flush ()
  
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

  /* --- Successful traces --- */
  if succesfull_traces_on {
    successful_traces.write_to_file (dir + "successful_traces_" + as_interest + ".txt")
  }

  output_msg ("missing_traces.txt", as_interest, missing_traces)
  output_msg ("false_positives.txt", as_interest, false_positives)
}
