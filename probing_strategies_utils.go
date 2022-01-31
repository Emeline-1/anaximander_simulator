/* ==================================================================================== *\
     probing_strategies_utils.go

     Utility functions to group the directed probes by AS and get the different
     groups (internals, direct neighbors, one hop neighbors, others).
     Utility functions to sort the groups and the ASes according to various criteria.
\* ==================================================================================== */

package main

import (
        "strings"
        "sort"
        pool "github.com/Emeline-1/pool"
        )

/* ------------------------------------------------------------------------------- *\
                           Directed Probes and Grouping
\* ------------------------------------------------------------------------------- */

/**
 * Reads the targets in the file corresponding to the AS of interest.
 * 
 * The targets returned are always "valid" targets for Anaximander's simulator,
 * in the sense they are /24 prefix (a different mask length would never produce a hit, 
    * due to simulator's implementation).
 * 
 * ex: if there is a prefix x.x.0.0/16, a random /24 prefix will be picked from the initial prefix.
 */
func get_directed_probes (as_interest string) []string {
    
    /* --- Get AS directed prefix file --- */
    files := pool.Get_directory_files (g_args.directed_prefixes_dir)
    var as_file string
    for _,file := range *files {
        if strings.Contains (file, as_interest) {
            as_file = file
        }
    }

    /* --- Read file --- */
    prefixes,_:= read_newline_delimited_file (as_file, 0)

    /* --- Pick a /24 prefix randomly within the larger prefix --- */
    directed_prefixes := make ([]string, 0, len (prefixes))
    for _, prefix := range prefixes {
        directed_prefixes = append (directed_prefixes, _get_24_prefix (prefix))
    }
    return directed_prefixes
}

/**
 * Returns:
 * - The directed probes, grouped by their AS.
 * - The directed neighbors of the AS of interest (slice of ASes)
 * - The one hop neighbors of the AS of interest (slice of ASes)
 * - The other ASes of the directed probes, that are not part of the neighbors nor the one hop neighbors (slice of ASes)
 * - The number of directed probes.
 * 
 * The slices of ASes returned never contain the AS of interest
 */
func get_directed_probes_and_groups (as_interest string) (map[string]map[string]interface{}, map[string]interface{}, map[string]interface{}, map[string]interface{}, int) {
    /* --- Get Directed Probes --- */
    directed_probes := get_directed_probes (as_interest)

    /* --- Group directed probes by the AS they belong to --- */
    AS_probes := make (map[string]map[string]interface{})
    missing_prefixes := 0
    for _, probe := range directed_probes {
        AS, present := prefix24_as[probe]
        if !present {
            missing_prefixes++
            AS = "-1" // Default AS for prefixes for which we can't attribute an AS. 
        }
        append_prefix (&AS_probes, AS, probe)
    }
    output_msg ("missing_prefixes.txt", as_interest, missing_prefixes)

    // Build a set of the ASes present in the directed probes (and remove the AS of interest at the same time)
    AS_probes_map := make (map[string]interface{})
    for AS,_ := range AS_probes { 
        if AS == as_interest {
            continue
        }
        AS_probes_map[AS] = struct{}{}
    }

    /* --- Get the neighbors --- */
    neighbors_map := as_neighbors[as_interest]
    neighbors_map = filter_on_directedProbes (neighbors_map, AS_probes_map) // Remove ASes not present in the directed probes
    
    /* --- Get the one hop neighbors --- */
    one_hop_neighbors_slice := get_one_hop_neighbors (as_interest)
    one_hop_neighbors_map := filter_on_directedProbes (slice_to_map (one_hop_neighbors_slice), AS_probes_map) // Remove ASes not present in the directed probes
    
    /* --- Get the ASes that are not part of the neighbors nor the one hop neighbors --- */
    other_AS := difference (AS_probes_map, merge_maps_new (neighbors_map, one_hop_neighbors_map))  //Remove the neighbors and the one hop neighbors

    return AS_probes, neighbors_map, one_hop_neighbors_map, slice_to_map (other_AS), len (directed_probes)
}

// -------------------------------------------------------------------------------
/**
 * Given a set of AS and a reference set of AS, keep only the ASes that are present in the reference set.
 */
func filter_on_directedProbes (to_filter, reference map[string]interface{}) map[string]interface{} {
    s := make (map[string]interface{})

    for AS,_ := range to_filter {
        if _, ok := reference[AS]; ok {
            s[AS] = struct{}{}
        }
    }
    return s
}

// -------------------------------------------------------------------------------
/**
 * Given an AS of interest, returns its one hop neighbors (excluding direct neighbors, and the AS of interest itself)
 */
func get_one_hop_neighbors (as_interest string) []string {

    /* --- Get the direct neighbors of the AS of interest --- */
    neighbors := as_neighbors[as_interest]

    /* --- Get the neighbors of the neighbors --- */
    one_hop_neighbors := make (map[string]interface{})
    for neighbor,_ := range neighbors {
        neighbor_neighbors := as_neighbors[neighbor]
        for n,_ := range neighbor_neighbors {
            if n == as_interest {
                continue
            }
            one_hop_neighbors[n] = struct{}{}
        }
    }

    /* --- Remove direct neighbors from list --- */
    return difference (one_hop_neighbors, neighbors)
}

// -------------------------------------------------------------------------------
/**
 * Given a slice of ASes, add the probes of thoses ASes to the target list.
 * 
 * Input:
 *  - s: the slice where to add the probes
 *  - ases: the ASes whose probes must be added to the slice
 *  - limits: the slice where to record the separation between the ASes.
 *  - AS_probes: the probes corresponding each AS
 *  - get_probe: a function that take a prefix as input and returns a probe.
 * Output:
 *  - s: the slice where the probes have been added
 *  - limits: the separations between the ASes
 */
func add_AS_probes (s, ases []string, limits []*AS_limit, AS_probes map[string]map[string]interface{}, get_probe func (string) string) ([]string, []*AS_limit) {
    for _,AS := range ases {
        if probes, ok := AS_probes[AS]; ok {
            for probe,_ := range probes {
                s = append (s, get_probe (probe))
            }
            limits = append (limits, &AS_limit{asn: AS, limit: len (s)})
        }
    }
    return s, limits
}

/* ------------------------------------------------------------------------------- *\
                             Sorting & Scheduling
\* ------------------------------------------------------------------------------- */

/**
 * Group the direct neighbors of the AS of interest in customers > peers > providers and within each group,
 * order them by their customer cone (increasing or decreasing).
 * Returns a slice of ASes.
 */
func group_by_relationships (AS_probes map[string]map[string]interface{}, as_interest string, reverse bool) []string {

    /* --- Get ASes based on their relationships and order them --- */
    c_p_p := map[int]map[string]interface{}{Customer: make (map[string]interface{}), Peer: make (map[string]interface{}), Provider: make (map[string]interface{})}
    for as, neighbors := range as_neighbors {
        if as == as_interest {
            for neighbor, rel := range neighbors { // 'neighbor' is a [customer/peer/provider] of 'as'
                c_p_p[rel.(int)][neighbor] = struct{}{}
            }
        }
    }
    customers := c_p_p[Customer]
    providers := c_p_p[Provider]
    peers := c_p_p[Peer]

    // Build a set of the ASes present in the directed probes (and remove the AS of interest at the same time)
    AS_probes_map := make (map[string]interface{})
    for AS,_ := range AS_probes { 
        if AS == as_interest {
            continue
        }
        AS_probes_map[AS] = struct{}{}
    }

    customers = filter_on_directedProbes (customers, AS_probes_map)
    providers =filter_on_directedProbes (providers, AS_probes_map)
    peers =filter_on_directedProbes (peers, AS_probes_map)

    ordered_customers := order_by_customer_cone (customers, as_interest, reverse)
    ordered_providers := order_by_customer_cone (providers, as_interest, reverse)
    ordered_peers := order_by_customer_cone (peers, as_interest, reverse)

    // Build slice
    r := make ([]string, 0, len (ordered_peers) + len (ordered_customers)+ len (ordered_providers))

    // several orders are possible: change in the code here to test different orders
    for _, AS := range ordered_customers {
        r = append (r, AS)
    }
    for _, AS := range ordered_peers {
        r = append (r, AS)
    }
    for _, AS := range ordered_providers {
        r = append (r, AS)
    }
    return r
}

/**
 * Given a set of ASes, order them by their customer cone (increasing or decreasing)
 */
func order_by_customer_cone (ases map[string]interface{}, as_interest string, reverse bool) []string {
    
    // Build a slice of (AS,weight)
    as_customersWeight := make (AS_weights, 0, len (ases))
    for as,_ := range ases {
        as_customersWeight = append (as_customersWeight, &AS_weight{name: as, weight: as_conesize[as]})
    }

    /* --- Sort neighbors according to their weight --- */
    if reverse {
        sort.Sort (sort.Reverse (ByWeight{as_customersWeight}))
    } else {
        sort.Sort (ByWeight{as_customersWeight})
    }
    // Build a slice of (AS)
    r := make ([]string, 0, len (as_customersWeight))
    for _,as_weight := range as_customersWeight {
        r = append (r, as_weight.name)
    }
    return r
}

/**
 * Sorting of neighbors by weight
 */
type AS_weight struct {
    name     string
    weight   int
}
type AS_weights []*AS_weight

func (s AS_weights) Len() int      { return len(s) }
func (s AS_weights) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByWeight struct{AS_weights}
func (s ByWeight) Less(i, j int) bool { return s.AS_weights[i].weight < s.AS_weights[j].weight }

/* ------------------------------------------------------------------------------- *\
                          Simple Group Access
\* ------------------------------------------------------------------------------- */

/**
 * Returns a slice of all the prefixes (/24) of the direct neighbors of the AS of interest.
 */
func _direct_neighbors (as_interest string) []string {
    neighbors := as_neighbors[as_interest]

    s := make ([]string, 0, 10)
    for neighbor,_ := range neighbors {
        for prefix,_ := range as_24prefixes[neighbor] {
            s = append (s, prefix)
        }
    }
    return s
}

// -------------------------------------------------------------------------------
/**
 * Returns a slice of all the prefixes (/24) of the AS of interest.
 */
func _internals (as_interest string) []string {
    s := make ([]string, 0, 10)
    for prefix, _ := range as_24prefixes[as_interest] {
        s = append (s, prefix) 
    }
    return s
}

/**
 * Returns the neighbors of the AS of interest ordered by their customer cone.
 */
func _get_neighbors_ordered_customer_cone (as_interest string, reverse bool) []string {
    neighbors := as_neighbors[as_interest]
    return order_by_customer_cone (neighbors, as_interest, reverse)
}
