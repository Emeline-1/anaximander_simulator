/* ==================================================================================== *\
    parallel_anaximander.go

    Alternative scheduling for Anaximander:
    ---------------------------------------
    The simulation (for an AS of interest) is performed in parallel, i.e., all ASes at 
    the same time. The exploration of the ASes is done in successive batches, whose size
    are determined by the weight attributed to an AS.
    
    Note that the notion of parallelism here has nothing to do with code execution, but has
    to do with the scheduling of the probes.

    This scheduling performs worse or equivalently to Anaximander's sequential scheduling.

\* ==================================================================================== */
package main

import (
    "strings"
    "strconv"
    "path/filepath"
    "os/exec"
    "math"
    "log"
    )

type weight_function func (*AS_status, int) (int)

// Note: y = p^{-1 +\frac{x}{max cc size}} increasing exponential function.

/**
 * Array holding the weighting functions. 
 * 
 * These functions attribute a weight to an AS depending on different criteria. 
 * 
 * The weight represents the percentage of the AS address space that should be
 * probed at once (in a batch). 
 * The return value is the actual number of probes in the batch, i.e., weight * address_space_size
 */
var generate_weight_functions []func ([]float64, int) weight_function = []func ([]float64, int) weight_function {
    generate_constant,
    generate_weight_inverse,
    generate_weight_inverse_iteration_reduction,
    generate_weight_cc_size,
}


func generate_constant (parameters []float64, nb_ases int) weight_function {
    if len (parameters) != 1 {
        log.Fatal ("Wrong weighting parameters. Expecting 1 parameter.")
    }
    return func (as *AS_status, iteration int) int {
        return max (int(parameters[0]),1)
    }
}

/**
 * Computes a parameter 'a' based on the number of ASes and the weight the last AS should have (ex: 0.01)
 * a = (nb_as * desired_weight)/(1 - desired_weight)
 * - Increase the desired weight to increase 'a'
 * - Decrease the desired weight to decrease 'a'
 * 
 * Given the parameter 'a', generate the function:
 * y = a/(x+a)
 * 
 * This function gives:
 * - A decreasing function in the form 1/x, meaning that the first ASes will have a higher
 *   weight (hence, be probed sooner) and the last ASes will have a lower weight (hence be
 *   probed later).
 * - y = 1 for x = 0, meaning the first AS will be probed fully (the first AS in
 *   the list corresponds to the internal prefixes).
 * - The 'a' parameter controls the slope of the function: 
 *   ° Increase 'a' to get a smoother slope (weight decreases slower along the x axis),
 *   ° Decrease 'a' to get a sharper slope (weight decreases faster along the x axis).
 */
func generate_weight_inverse(parameters []float64, nb_ases int) weight_function {
    if len (parameters) != 1 {
        log.Fatal ("Wrong weighting parameters. Expecting 1 parameter.")
    }
    desired_weight := parameters[0]
    parameter := (desired_weight*float64(nb_ases))/(1-desired_weight) // Compute parameter based on the number of ASes and the user-defined desired_weight.
    
    return func (as *AS_status, iteration int) int {
        weight := parameter/ (float64(as.position) + parameter)
        batch_size := weight * float64(as.end - as.start)
        return max (int(math.Ceil (batch_size)), 1)
    }
}

/**
 * Same as generate_weight_inverse, but this time the batch size decreases with the iteration, i.e., the number
 * of times we already visited that AS.
 */
func generate_weight_inverse_iteration_reduction (parameters []float64, nb_ases int) weight_function {
    if len (parameters) != 2 {
        log.Fatal ("Wrong weighting parameters. Expecting 2 parameters.")
    }
    desired_weight := parameters[0]
    second_parameter := parameters[1]
    parameter := (desired_weight*float64(nb_ases))/(1-desired_weight) // Compute parameter based on the number of ASes and the user-defined desired_weight.
    
    return func (as *AS_status, iteration int) int {
        weight := parameter/ (float64(as.position) + parameter)
        batch_size := int (math.Ceil (weight * float64(as.end - as.start)))

        /* weight depending on iteration */
        iteration_weight := second_parameter/(float64 (iteration) + second_parameter)
        batch_size = int (math.Ceil (iteration_weight * float64(batch_size))) // Pbm: how to tune? How many iteration will there be?

        return max (batch_size, 1)
    }
}

/**
 * Same as function 'generate_weight_inverse' but on the customer cone size instead of the relative order.
 * => Bilan: Results are better than with the relative order of ASes. They are also slightly better than purely sequential
 * but it's not much.
 */
func generate_weight_cc_size (parameters []float64, nb_ases int) weight_function {
    if len (parameters) != 1 {
        log.Fatal ("Wrong weighting parameters. Expecting 1 parameters.")
    }
    if len(as_conesize) == 0 {
        log.Fatal ("as_conesize not set")
    }

    desired_weight := parameters[0]
    parameter := (desired_weight*float64(max_conesize))/(1-desired_weight) 
    
    /* --- Weight function --- */
    return func (as *AS_status, iteration int) int {
        if as.position == 0 { // Special case of the internal prefixes.
            return as.end - as.start
        }
        var cc_size int
        var ok bool
        if cc_size, ok = as_conesize[as.asn]; !ok {
            cc_size = 1
        }
        weight := parameter/ (float64(cc_size) + parameter)
        batch_size := weight * float64(as.end - as.start)
        return max (int(math.Ceil (batch_size)), 1)
    }
}

// -------------------------------------------------------------------------------
func generate_anaximander_parallel (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn *SafeSet, output_file string, router_to_addrs *SafeSet) func (string){
    return func (as_interest string) {
        anaximander_parallel (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn, as_interest, trim_suffix (output_file, ".txt") + "_" + as_interest + ".txt", router_to_addrs)
    }
}

// -------------------------------------------------------------------------------
/**
 * Perform the simulation on the traces.
 * The simulation is performed in parallel, i.e., all ASes at the same time. This allows to see how the real Anaximander performs in the wild.
 */
func anaximander_parallel (traces, adjs, multi_adjs, addresses, target_to_vp, addr_to_asn *SafeSet, as_interest string, output_file string, routers *SafeSet) {

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
    weight_function := generate_weight_functions[int (g_args.weight_parameters[0])] (g_args.weight_parameters[1:], len (ases_status))

    iteration := 0
    for stopped_ases != len (ases_status) {
        for _, as_status := range ases_status {

            batch_size := weight_function (as_status, iteration)
            for i := 0; i < batch_size; i++ {
                destination, stopped_ases = launch_as_probing (sorted_destinations, as_status, stopped_ases)
                if destination == "" { // Nothing to probe for current AS, carry on to next AS
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

// -------------------------------------------------------------------------------
/**
 * For a given AS, returns the current target to probe, if the AS hasn't been stopped and if the AS hasn't
 * been completely probed yet.
 * Also increments the global number of stopped_ases when an AS has been completely probed.
 */
func launch_as_probing (sorted_destinations []string, as_status *AS_status, _stopped_ases int) (destination string, stopped_ases int) {
    if as_status.stopped || as_status.curr_probe >= as_status.end {
        destination = ""
    } else {
        destination = sorted_destinations[as_status.curr_probe]
    }
    as_status.curr_probe++
    stopped_ases = _stopped_ases
    if (as_status.curr_probe == as_status.end) && as_status.stopped == false{ 
        // Even if an AS is stoped, we increment its counter. Therefore, we need to check the first condition, otherwise, each time we get back to a stopped AS, we increment the number of stopped ASes
        // We also need to have the second condition, otherwise an AS that was stopped because of a plateau will be stopped two times when we reach the end of its addres space.
        stopped_ases = _stopped_ases+1
        as_status.stopped = true
    }
    return
}

// -------------------------------------------------------------------------------
type AS_status struct {
    asn string;           // The AS number
    start int;            // The index of the beginning of this AS's probes
    end int;              // The index of the end of this AS's probes
    curr_probe int;       // The current probe
    plateau int;          // Whether the probing of this AS has been stopped due to a plateau. curr_probe remains the current probe if we want to get back and continue probing
    stopped bool;         // The current length of the plateau, expressed as a number of probes.
    position int;         // The position of this AS in the as_limit file
} 