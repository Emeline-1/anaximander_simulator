/* ============================================================ *\
   anaximander_strategy.go

   Implements the _Anaximander Strategy Step_ 

   Reads the necessary datasets and apply a strategy. The
   output can then be used to launch the _Anaximander Simulator_
\* ============================================================ */

package main

import (
    "strings"
    "strconv"
    "log"
    "os/exec"
    "net"
    pool "github.com/Emeline-1/pool"
    )

/**
 * A strategy_function takes as input:
 * - A slice of all targets found in the warts files
 * - The AS of interest
 * - A target_to_vp mapping
 * 
 * A strategy_function returns:
 * - a slice of string: ordered list of targets
 * - a slice of AS_limit: this gives, for each AS, the delimitation with the next AS in the oredered list of targets.
 */
type strategy_function func ([]string, string, *SafeSet) ([]string, []*AS_limit)

/**
 * Array holding all probing strategies.
 */ 
var strategy_fc []strategy_function = []strategy_function {
    random,
    increasing_order,
    direct_neighbors, 
    direct_neighbors_and_internal, 
    internal_and_direct_neighbors,
    customer_cone_neighbors_decreasing,
    customer_cone_neighbors_increasing, 
    // Rocketfuel and improvements
    directed_probing,
    directed_probing_internal_neighbors_others,
    directed_probing_internal_neighbors_others_customercone,
    directed_probing_internal_neighbors_others_mixed,
    directed_probing_internal_neighbors_onehopneighbors_others,
    // Rocketfuel directed probing, but the prefixes haven't been broken down to /24 prefixes.
    directed_probing_no24,
    directed_probing_internal_neighbors_onehopneighbors_others_no24,
    directed_probing_internal_neighbors_others_no24,
    //Direct neighbors replay without breaking down into /24
    customer_cone_neighbors_increasing_no24,
    // Rocketfuel best directed probes (prefixes not broken down into /24)
    best_directed_probing_internal_neighbors_onehopneighbors_others_no24,
    overlays_reduction_global,
    // Rocketfuel next hop AS reduction
    next_hop_as_reduction_global,
    //Oracle strategy
    oracle,
    overlays_reduction_global_relationships,
    overlays_reduction_global_relationships_decreasing_cc,
}

/**
 * Structure to hold the ASN, as well as the index of their last probe in the global ordering of the probes.
 */
type AS_limit struct {
    asn string;
    limit int;
}

func launch_anaximander_strategy (break_prefix bool, strategy int, output_dir string) {

    /* --- Read data --- */
    log.Println ("Reading data...")
    as_neighbors = read_as_rel (g_args.as_rel_file)
    as_24prefixes, prefix24_as, as_prefixes, prefix_as = read_ip2as (g_args.ip2as_file)
    if break_prefix {
            as_to_prefixes, prefix_to_as = as_24prefixes, prefix24_as
    } else {
        as_to_prefixes, prefix_to_as = as_prefixes, prefix_as
    }
    as_conesize = read_customer_cone (g_args.ppdc_file) // Must come afterwards.
    ases_interest,_ := read_whitespace_delimited_file (g_args.ases_interest_file)

    vps = []string{"my_VP"}
    target_to_vp := create_safeset ()
    target_to_vp.fake_it ("my_VP")
    destinations := []string{}

    /* --- To be able to record the stratagy for a given warts dataset --- */
    if g_args.warts_directory != "" && g_args.vps_file != ""{
        traces, _, _, _, target_to_vp_local, _, _ := parse_warts ()
        target_to_vp = target_to_vp_local
        destinations = get_keys (&traces.set)
        vps,_ = read_vps_file (g_args.vps_file)
    }

    /* --- Launch Strategy --- */
    log.Println ("Launch Anaximander Strategy...")
    f := generate_anaximander_strategy (strategy, output_dir, target_to_vp, destinations)
    pool.Launch_pool (3, ases_interest, f)
}

func generate_anaximander_strategy (strategy int, output_dir string, target_to_vp *SafeSet, destinations []string) func (string){
    return func (as_interest string) {
        // build directory for the AS
        output_dir_as := output_dir + "/" + as_interest
        cmd_s := "mkdir " + output_dir_as
        exec.Command("bash", "-c", cmd_s).Run()

        write_strategy (strategy, as_interest, target_to_vp, output_dir_as, destinations)
    }
}

/**
 * Writes the Strategy Step output, i.e., a list of ordered targets and of AS delimitation.
 */
func write_strategy (strategy int, as_interest string, target_to_vp *SafeSet, output_dir string, destinations []string) {

    /* --- Launch strategy --- */
    sorted_destinations, limits_neighbors := strategy_fc[strategy](destinations, as_interest, target_to_vp)
    
    /* --- Record results --- */
    w, file := new_bufio_writer (output_dir + "/targets.txt")
    for _, target := range sorted_destinations {
        _, network, _ := net.ParseCIDR (target)
            ip_address := get_random_ip (network).String ()
        w.WriteString (ip_address + "\n")
    }
    w.Flush ()
    file.Close ()

    w, file = new_bufio_writer (output_dir + "/as_limits.txt")
    previous := 0
    for _, limit := range limits_neighbors {
        if limit.limit != previous {
            w.WriteString (strconv.Itoa (limit.limit) + " " + limit.asn + "\n")
        }
        previous = limit.limit
    }
    w.Flush ()
    file.Close ()
}

/**
 * Reads the Strategy Step output, and returns a list of ordered targets and of AS delimitation.
 */
func read_strategy (s []string, as_interest string) ([]string, []*AS_limit) {
    /* --- Read targets --- */
    targets := make ([]string, 0, len (s))
    targets_file := g_args.strategy + "/" + as_interest + "/targets.txt"
    reader := NewCompressedReader (targets_file)
    reader.Open ()
    scanner := reader.Scanner ()
    for scanner.Scan () {
        line := scanner.Text () // Must add /24
        targets = append (targets, strings.Join (strings.Split (line, ".")[:3], ".")+".0/24")
    }
    reader.Close ()

    /* --- Read AS delimitations --- */
    as_limits := make ([]*AS_limit, 0, 10)
    limit_file := g_args.strategy + "/" + as_interest + "/as_limits.txt"
    reader = NewCompressedReader (limit_file)
    reader.Open ()
    scanner = reader.Scanner ()
    for scanner.Scan () {
        line := strings.Fields (scanner.Text ())
        if len(line) < 2 {
            log.Fatal ("[WARNING]: missing ASN in as_limit file:", limit_file, line)
        }
        n,_ := strconv.Atoi (line[0])
        asn := line[1]
        as_limits = append (as_limits, &AS_limit{asn:asn, limit:n})
    }
    reader.Close ()

    return targets, as_limits
}