/* ==================================================================================== *\
     probing_strategies.go

     Implements the various probing strategies, from most simplest to Anaximander's
     final strategy.
\* ==================================================================================== */

package main 

import (
        "strings"
        "math/rand"
        "sort"
        "time"
        "strconv"
        "log"
        )

var ( // Read-only variables (set only once in anaximander_driver.go)
    as_to_prefixes map[string]map[string]interface{}; // Set to as_24prefixes or as_prefixes depending on if we break down prefixes or not
    prefix_to_as map[string]string; // Set to prefix24_as or prefix_as depending on if we break down prefixes or not
)

var ( // Read-only variables (set only once in anaximander_driver.go)
    vps []string; // The source IP addresses of the VPs.
)

/* ------------------------------------------------------------------------------- *\
                             Probing strategies
\* ------------------------------------------------------------------------------- */

/**
 * 0. Sort the targets in random order
 */
func random (s []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit){
    if len (s) == 0 {
        log.Fatal ("Cannot apply strategy without warts data set")
    }

    rand.Seed(time.Now().UnixNano())
    rand.Shuffle(len(s), func(i, j int) {
        s[i], s[j] = s[j], s[i]
    })
    return s, []*AS_limit{&AS_limit{asn:"0", limit:len (s)}}
}

// -------------------------------------------------------------------------------
/**
 * 1. Sort the targets in increasing order
 */
func increasing_order (s []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    if len (s) == 0 {
        log.Fatal ("Cannot apply strategy without warts data set")
    }

    sort.Strings(s)
    return s, []*AS_limit{&AS_limit{asn:"0", limit:len (s)}}
}

// -------------------------------------------------------------------------------
/**
 * 2. Limit the targets to the /24 prefixes of direct neighbors (no ordering)
 */
func direct_neighbors (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    neighbors := as_neighbors[as_interest]
    s := make ([]string, 0, 10)
    limits := make ([]*AS_limit, 0, len (neighbors))
    s, limits = add_AS_probes (s, get_keys (&neighbors), limits, as_24prefixes, _get_24_prefix)

    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 3. Limit the targets to the /24 prefixes of the direct neighbors and
 * the internal prefixes of the AS (no ordering inside respective groups)
 */
func direct_neighbors_and_internal (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    neighbors := _direct_neighbors (as_interest)
    internals := _internals (as_interest)
    s := make ([]string, len (neighbors)+ len (internals))
    copy(s, neighbors)
    copy(s[len(neighbors):], internals)

    return s, []*AS_limit{&AS_limit{asn:"0", limit:len (neighbors)}, &AS_limit{asn:"1", limit:len (s)}}
}

// -------------------------------------------------------------------------------
/**
 * 4. Limit the targets to the /24 prefixes of the direct neighbors and
 * the internal prefixes of the AS. Order: first internals, then neighbors.
 * (no ordering inside respective groups)
 */
func internal_and_direct_neighbors (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    neighbors := _direct_neighbors (as_interest)
    internals := _internals (as_interest)
    s := make ([]string, len (neighbors)+ len (internals))
    copy(s, internals)
    copy(s[len(internals):], neighbors)

    return s, []*AS_limit{&AS_limit{asn:"0", limit:len (internals)}, &AS_limit{asn:"1", limit:len (s)}}
}

// -------------------------------------------------------------------------------
/**
 * 5. Limit the targets to the prefixes of direct neighbors.
 * The prefixes are raw or broken down into /24 prefixes according to the break_prefix arg.
 * Sort the neighbors according to their customer cone (decreasing order)
 */
func customer_cone_neighbors_decreasing (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    return _customer_cone_neighbors (nil, as_interest, true)
}

// -------------------------------------------------------------------------------
/**
 * 6. Limit the targets to the prefixes of direct neighbors.
 * The prefixes are raw or broken down into /24 prefixes according to the break_prefix arg.
 * Sort the neighbors according to their customer cone (increasing order)
 */
func customer_cone_neighbors_increasing (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    return _customer_cone_neighbors (nil, as_interest, false)
}

// -------------------------------------------------------------------------------
func _customer_cone_neighbors (_ []string, as_interest string, reverse bool) ([]string, []*AS_limit) {

    ordered_neighbors := _get_neighbors_ordered_customer_cone (as_interest, reverse)

    s := make ([]string, 0, len (ordered_neighbors))
    limits := make ([]*AS_limit, 0, len (ordered_neighbors))
    s, limits = add_AS_probes (s, ordered_neighbors, limits, as_to_prefixes, _get_24_prefix)

    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 7. Rocketfuel directed probing
 */
func directed_probing (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    
    prefixes := get_directed_probes (as_interest)
    return prefixes, []*AS_limit{&AS_limit{asn:"0", limit:len (prefixes)}}
}

// -------------------------------------------------------------------------------
/**
 * 8. Directed probing in three groups:
 *     - Internal prefixes
 *     - Direct neighbors (no order)
 *     - Others (grouped by AS, but no order between ASes).
 */
func directed_probing_internal_neighbors_others (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    return _directed_probing_internal_neighbors_others (nil, as_interest, false)
}

// -------------------------------------------------------------------------------
/**
 * 9. Directed probing in three groups:
 *     - Internal prefixes
 *     - Direct neighbors (ordered by increasing customer cone)
 *     - Others (ordered by increasing customer cone).
 */
func directed_probing_internal_neighbors_others_customercone (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    return _directed_probing_internal_neighbors_others (nil, as_interest, true)
}

// -------------------------------------------------------------------------------
func _directed_probing_internal_neighbors_others (_ []string, as_interest string, ordered bool) ([]string, []*AS_limit) {
    AS_probes, neighbors_map, one_hop_neighbors_map, other_AS_map, nb_probes := get_directed_probes_and_groups (as_interest)

    s := make ([]string, 0, nb_probes)
    limits := make ([]*AS_limit, 0, len (neighbors_map) + len (one_hop_neighbors_map) + len (other_AS_map) + 1)

    /* --- Group 1: internal prefixes --- */
    for probe,_ := range AS_probes[as_interest] {
            s = append (s, probe)
    }
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: the neighbors --- */
    var neighbors []string
    if ordered {
        neighbors = order_by_customer_cone (neighbors_map, as_interest, false)
    }
    s, limits = add_AS_probes (s, neighbors, limits, AS_probes, _get_24_prefix)
    group_2 := len (s)

    /* --- Group 3: the one hop neighbors and the others --- */
    //mixed := append (one_hop_neighbors_map, other_AS_map) // Mix both groups
    tmp := merge_maps (one_hop_neighbors_map, other_AS_map)
    mixed := get_keys (&tmp)
    if ordered {
        mixed = order_by_customer_cone (tmp, as_interest, false) 
    }
    s, limits = add_AS_probes (s, mixed, limits, AS_probes, _get_24_prefix)
    group_3 := len (s)
    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2, group_3)
    //Note: those delimitation are only valid if there is NO reduction!!!

    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 10. Directed probing in two groups:
 *     - Internal prefixes
 *     - Direct neighbors, one hope neighbors and others 
 *              (ordered by increasing customer cone - no distinction between three groups)
 */
func directed_probing_internal_neighbors_others_mixed (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    AS_probes, neighbors_map, one_hop_neighbors_map, other_AS_map, nb_probes := get_directed_probes_and_groups (as_interest)
    
    s := make ([]string, 0, nb_probes)
    limits := make ([]*AS_limit, 0, len (neighbors_map) + len (one_hop_neighbors_map) + len (other_AS_map) + 1)

    /* --- Group 1: internal prefixes --- */
    for probe,_ := range AS_probes[as_interest] {
            s = append (s, probe)
    }
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: the neighbors, the one hope neighbors, and the others mixed together --- */
    mixed := merge_maps (neighbors_map, one_hop_neighbors_map)
    mixed = merge_maps (mixed, other_AS_map) // Mix three groups together
    mixed_slice := order_by_customer_cone (mixed, as_interest, false)
    s, limits = add_AS_probes (s, mixed_slice, limits, AS_probes, _get_24_prefix)
    group_2 := len (s)

    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2)
    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 11. Directed probing into 4 groups:
 *     - Internal prefixes
 *     - Direct neighbors
 *       - One hope neighbors
 *       - Others 
 *              (all groups ordered by increasing customer cone)
 */
func directed_probing_internal_neighbors_onehopneighbors_others (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    AS_probes, neighbors_map, one_hop_neighbors_map, other_AS_map, nb_probes := get_directed_probes_and_groups (as_interest)

    s := make ([]string, 0, nb_probes)
    limits := make ([]*AS_limit, 0, len (neighbors_map) + len (one_hop_neighbors_map) + len (other_AS_map) + 1)

    /* --- Group 1: internal prefixes --- */
    for probe,_ := range AS_probes[as_interest] {
        s = append (s, probe)
    }
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: the neighbors --- */
    neighbors := order_by_customer_cone (neighbors_map, as_interest, false)
    s, limits = add_AS_probes (s, neighbors, limits, AS_probes, _get_24_prefix)
    group_2 := len (s)

    /* --- Group 3: the one hop neighbors --- */
    one_hop_neighbors := order_by_customer_cone (one_hop_neighbors_map, as_interest, false)
    s, limits = add_AS_probes (s, one_hop_neighbors, limits, AS_probes, _get_24_prefix)
    group_3 := len (s)

    /* --- Group 4: the others --- */
    other_AS := order_by_customer_cone (other_AS_map, as_interest, false)
    s, limits = add_AS_probes (s, other_AS, limits, AS_probes, _get_24_prefix)
    group_4 := len (s)


    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2, group_3, group_4)
    return s, limits
}

/* ============================================================================== *\
                            No /24 probes
\* ============================================================================== */

// -------------------------------------------------------------------------------
/**
 * 12. Rocketfuel's directed probe without breaking them down in /24 prefixes.
 */
func directed_probing_no24 (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    return directed_probing (nil, as_interest, nil)
}

// -------------------------------------------------------------------------------
/**
 * 13. 
 *  - Internal prefixes (/24)
 *  - Directed probe (no /24 prefixes) as:
 *    - Direct neighbors
 *    - One hope neighbors
 *    - Others 
 *        (all groups ordered by increasing customer cone)
 */
func directed_probing_internal_neighbors_onehopneighbors_others_no24 (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    AS_probes, neighbors_map, one_hop_neighbors_map, other_AS_map, nb_probes := get_directed_probes_and_groups (as_interest)

    s := make ([]string, 0, nb_probes)
    limits := make ([]*AS_limit, 0, len (neighbors_map) + len (one_hop_neighbors_map) + len (other_AS_map) + 1)

    /* --- Group 1: internal prefixes (/24) --- */
    s = append (s, _internals (as_interest)...) // Would be better to have Rocketfuel /24, but not straightforward at this point
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: the neighbors --- */
    neighbors := order_by_customer_cone (neighbors_map, as_interest, false)
    s, limits = add_AS_probes (s, neighbors, limits, AS_probes, _get_24_prefix)
    group_2 := len (s)

    /* --- Group 3: the one hop neighbors --- */
    one_hop_neighbors := order_by_customer_cone (one_hop_neighbors_map, as_interest, false)
    s, limits = add_AS_probes (s, one_hop_neighbors, limits, AS_probes, _get_24_prefix)
    group_3 := len (s)

    /* --- Group 4: the others --- */
    other_AS := order_by_customer_cone (other_AS_map, as_interest, false)
    s, limits = add_AS_probes (s, other_AS, limits, AS_probes, _get_24_prefix)
    group_4 := len (s)

    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2, group_3, group_4)
    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 14. 
 *  - Internal prefixes (/24)
 *  - Directed probe (no /24 prefixes) as:
 *    - Direct neighbors
 *    - Others 
 *        (all groups ordered by increasing customer cone)
 */
func directed_probing_internal_neighbors_others_no24 (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    AS_probes, neighbors_map, one_hop_neighbors_map, other_AS_map, nb_probes := get_directed_probes_and_groups (as_interest)

    s := make ([]string, 0, nb_probes)
    limits := make ([]*AS_limit, 0, len (neighbors_map) + len (one_hop_neighbors_map) + len (other_AS_map) + 1)

    /* --- Group 1: internal prefixes (/24) --- */
    s = append (s, _internals (as_interest)...) // Would be better to have Rocketfuel /24, but not straightforward at this point
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: the neighbors --- */
    neighbors := order_by_customer_cone (neighbors_map, as_interest, false)
    s, limits = add_AS_probes (s, neighbors, limits, AS_probes, _get_24_prefix)
    group_2 := len (s)

    /* --- Group 3: the one hop neighbors and the others --- */
    mixed := merge_maps (one_hop_neighbors_map, other_AS_map) // Mix both groups
    mixed_slice := order_by_customer_cone (mixed, as_interest, false) 
    s, limits = add_AS_probes (s, mixed_slice, limits, AS_probes, _get_24_prefix)
    group_3 := len (s)

    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2, group_3)
    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 15. Internals (/24) + direct neighbors (no /24)
 * Sort the neighbors according to their customer cone (increasing order)
 * 
 * Same results as mode 13, where se stop right after the neighbors. We have exactly the same
     level of discovery (as expected)
 */
func customer_cone_neighbors_increasing_no24 (s []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    s = make ([]string, 0, len (s))
    limits := make ([]*AS_limit, 0, len (s))

    /* --- Group 1: internal prefixes (/24) --- */
    s = append (s, _internals (as_interest)...) // Would be better to have Rocketfuel /24, but not straightforward at this point
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: direct neighbors --- */
    neighbors := _get_neighbors_ordered_customer_cone (as_interest, false)

    // Build the mapping between an AS and its prefixes
    AS_probes := make (map[string]map[string]interface{})
    for _,as := range neighbors {
        for prefix,_ := range as_to_prefixes[as] {
            append_prefix (&AS_probes, as, prefix)
        }
    }

    s, limits = add_AS_probes (s, neighbors, limits, AS_probes, _get_24_prefix) 
    group_2 := len (s)

    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2)
    
    return s, limits
}

// -------------------------------------------------------------------------------
/**
 * 16. Same as mode 13, except that we simulate on the BEST directed probes.
 */ 
func best_directed_probing_internal_neighbors_onehopneighbors_others_no24 (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {
    return directed_probing_internal_neighbors_onehopneighbors_others_no24 (nil, as_interest, target_to_vp)
}

// -------------------------------------------------------------------------------
/**
 * 17. Rocketfuel's BEST directed probes (not broken down into /24)
 *     Reduction on overlays.
 *       Same as 16, but reduction on overlays.
 */
func overlays_reduction_global (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    /* --- Read the global overlay file --- */
    // key: the VP
    // value: key: a target
    //        value: all its overlays.
    overlays := make (map[string]map[string]map[string]interface{})
    global_overlays := read_overlay_file (g_args.overlays_global_file)
    for _, vp := range vps {
        overlays[vp] = global_overlays // All VPs points towards the same overlays (as we have a global overlay file)
    }

    return _overlays_reduction (nil, as_interest, target_to_vp, overlays, false, false)
}

// -------------------------------------------------------------------------------
/**
 * 20. Rocketfuel's BEST directed probes (not broken down into /24)
 *     Reduction on overlays.
 *       Same as 17, but direct neighbors are grouped by their relationships and then ordered by customer cone.
 */
func overlays_reduction_global_relationships (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    /* --- Read the global overlay file --- */
    // key: the VP
    // value: key: a target
    //        value: all its overlays.
    overlays := make (map[string]map[string]map[string]interface{})
    global_overlays := read_overlay_file (g_args.overlays_global_file)
    for _, vp := range vps {
        overlays[vp] = global_overlays // All VPs points towards the same overlays (as we have a global overlay file)
    }

    return _overlays_reduction (nil, as_interest, target_to_vp, overlays, true, false)
}

// -------------------------------------------------------------------------------
/**
 * 21. Rocketfuel's BEST directed probes (not broken down into /24)
 *     Reduction on overlays.
 *       Same as 17, but reverse order of customer cone
 */
func overlays_reduction_global_relationships_decreasing_cc (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    /* --- Read the global overlay file --- */
    // key: the VP
    // value: key: a target
    //        value: all its overlays.
    overlays := make (map[string]map[string]map[string]interface{})
    global_overlays := read_overlay_file (g_args.overlays_global_file)
    for _, vp := range vps {
        overlays[vp] = global_overlays // All VPs points towards the same overlays (as we have a global overlay file)
    }

    return _overlays_reduction (nil, as_interest, target_to_vp, overlays, true, true)
}

func _overlays_reduction (_ []string, as_interest string, target_to_vp *SafeSet, overlays map[string]map[string]map[string]interface{}, relationships bool, reverse bool) ([]string, []*AS_limit) {

    /* --- Get Rocketfuel directod probes --- */
    AS_probes, neighbors_map, one_hop_neighbors_map, other_AS_map, nb_probes := get_directed_probes_and_groups (as_interest)

    s := make ([]string, 0, nb_probes)
    limits := make ([]*AS_limit, 0, len (neighbors_map) + len (one_hop_neighbors_map) + len (other_AS_map) + 1)

    /* --- Group 1: internal prefixes (/24) --- */
    s = append (s, _internals (as_interest)...) // Would be better to have Rocketfuel /24, but not straightforward at this point
    limits = append (limits, &AS_limit{asn: as_interest, limit: len (s)})
    group_1 := len (s)

    /* --- Group 2: the neighbors --- */
    var neighbors []string
    if relationships {
        neighbors = group_by_relationships (AS_probes, as_interest, reverse)
    } else {
        neighbors = order_by_customer_cone (neighbors_map, as_interest, reverse)
    }
    remove_overlays (AS_probes, neighbors, target_to_vp, overlays)
    s, limits = add_AS_probes (s, neighbors, limits, AS_probes, _get_24_prefix)
    group_2 := len (s)

    /* --- Group 3: the one hop neighbors --- */
    one_hop_neighbors := order_by_customer_cone (one_hop_neighbors_map, as_interest, reverse)
    remove_overlays (AS_probes, one_hop_neighbors, target_to_vp, overlays)
    s, limits = add_AS_probes (s, one_hop_neighbors, limits, AS_probes, _get_24_prefix)
    group_3 := len (s)

    /* --- Group 4: the others --- */
    other_AS := order_by_customer_cone (other_AS_map, as_interest, reverse)
    remove_overlays (AS_probes, other_AS, target_to_vp, overlays)
    s, limits = add_AS_probes (s, other_AS, limits, AS_probes, _get_24_prefix)
    group_4 := len (s)

    output_msg ("main_groups_limits.txt", as_interest, group_1, group_2, group_3, group_4)

    /* --- Group 3: the one hop neighbors and the others --- */
    //mixed := append (one_hop_neighbors, other_AS...) // Mix both groups
    //mixed = order_by_customer_cone (slice_to_map (mixed), as_interest, false)
    //remove_overlays (AS_probes, mixed, target_to_vp, overlays)
    //s, limits = add_AS_probes (s, mixed, limits, AS_probes, _get_24_prefix)
    //group_3 := len (s)

    //output_msg ("main_groups_limits.txt", as_interest, group_1, group_2, group_3)
    
    return s, limits
}

/* ============================================================================== *\
                            Next-hop AS Reduction
\* ============================================================================== */

/**
 * 18. Rocketfuel's Next Hop AS reduction (on global file)
 */
func next_hop_as_reduction_global (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    /* --- Read global nextAS file --- */
    prefix_to_nextAS, nextAS_to_prefixes := read_nextAS_file (g_args.nexthop_as_dir_global + "/merged_next_AS_"+as_interest+".txt")
    prefix_to_prefixes := make (map[string]map[string]interface{})
    for prefix, nextAS := range prefix_to_nextAS {
        if nextAS == as_interest { // The AS of interest is actually the next-hop -> Don't apply nextAS reduction on the AS of interest itself.
            continue
        }
        prefix_to_prefixes[prefix] = nextAS_to_prefixes[nextAS]
    }

    vp_prefix_to_prefixes := make (map[string]map[string]map[string]interface{})
    for _, vp := range vps {
        vp_prefix_to_prefixes[vp] = prefix_to_prefixes // All VPs points towards the same nextASes (as we have a global file)
    }

    /* --- Get Rocketfuel directed prefixes --- */
    directed_probes := get_directed_probes(as_interest)
    AS_probes := make (map[string]map[string]interface{})
    AS_probes["."] = slice_to_map (directed_probes)

    remove_overlays (AS_probes, []string{"."}, target_to_vp, vp_prefix_to_prefixes)

    s := make ([]string, 0, len (AS_probes["."]))
    for probe, _ := range AS_probes["."] {
        s = append (s, probe)
    }

    output_msg ("nextAS_reduction.txt", as_interest, len (s), len (directed_probes))
    
    return s, []*AS_limit{&AS_limit{asn: "0", limit: len (s)}} 
}

// -------------------------------------------------------------------------------
/**
 * 19. Look at the traces that yielded discovery (from run on mode 0).
 */
func oracle (_ []string, as_interest string, target_to_vp *SafeSet) ([]string, []*AS_limit) {

    oracle_prefixes_file := g_args.oracle_prefixes_dir + "/successful_traces_" + as_interest + ".txt"

    /* --- Read file --- */
    reader := NewCompressedReader (oracle_prefixes_file)
    reader.Open ()
    defer reader.Close ()
    scanner := reader.Scanner ()

    /* --- Sort prefixes according to the level of discovery --- */
    prefixes := make (AS_weights, 0, 100)
    for scanner.Scan () {
        line := strings.Fields (scanner.Text ())
        w,_ := strconv.Atoi (line[1])
        prefixes = append (prefixes, &AS_weight{name: line[0], weight: w})
    }
    sort.Sort (sort.Reverse (ByWeight{prefixes}))

    // Build the slice of prefixes
    s := make ([]string, 0, len (prefixes))
    for _,as_weight := range prefixes {
        s = append (s, as_weight.name)
    }

    return s, []*AS_limit{&AS_limit{asn: "0", limit: len (s)}}
}
