package main

import ("log"
    "strings"
    tree "github.com/Emeline-1/anaximander_simulator/tree")

/**
 * Array holding all heuristic functions
 */
type apply_heuristic_fn func (*SafeSet, *SafeSet, []string)

var apply_heuristic_fc []apply_heuristic_fn = []apply_heuristic_fn {
    apply_shortest_path_heuristic,
    apply_valley_free_heuristic,
}

/* ==================================== *\
        VALLEY FREE HEURISTIC
\* ==================================== */

// Records all nodes already seen to detect cycles.
type Nodes struct {
    // Record all the nodes, as well as all the paths going through that node.
    // key: the node
    // value: key: one of the path going through that node.
    node_to_entries map[string]map[*Rib_entry]interface{}
    pivot_nodes map[string]struct{} // Nodes at which there is a split in the paths
}

func NewNodes () *Nodes {
    return &Nodes{node_to_entries: make (map[string]map[*Rib_entry]interface{}), pivot_nodes:make (map[string]struct{})}
}

/**
 * Returns a function to be called when a cycle is detected,
 * i.e., when a node is absent in the current tree path, but has already been
 * encounetred in another path.
 */
func generate_if_absent (nodes *Nodes) func (string, interface{}) {
    return func (element string, arg interface{}) {
        /* --- Record nodes and detect cycles --- */
        if _, present := nodes.node_to_entries[element]; present {
            nodes.pivot_nodes[element] = struct{}{}
        }
        append_rib (&(nodes.node_to_entries), element, arg)
    }
}

/**
 * Returns a function to be called each time an exisisting node is visited.
 */
func generate_if_present (nodes *Nodes) func (string, interface{}) {
    return func (element string, arg interface{}) {
        append_rib (&(nodes.node_to_entries), element, arg)
    }
}

/**
 * For a set of RIB entries for a given prefix, select the best one according to the valley free heuristic,
 * and record it in the routing_entries_set.
 * 
 * How it is done:
 *  If two or more path diverge at a given AS, select the paths based on the next-hop AS (customer > peer > provider).
 *  If the next-hops have the same relationship with the diverging AS, select the shortest one.
 *  If they have the same length, select the one with the AS of interest, to be more conservative.
 * 
 *  For the first-hop diverging AS path (path that have nothing in common), keep the shortest, then keep the one with the
 *  AS of interest. And if there's no AS of interest, select one at random (this will have no effect on the
    * directed prefixes, the prefix will not be a part of it, period).

 * 1. Particular case where the next-hop of a path is also part of the other path.
 * 2. Case of the X Y Z and W Y Z: so when there are several pivot nodes in a row. 
 *
 * Particular cases (but doesn't affect the algorithm, this is just for documentation purpose):
 * - A BGP collector connected two times to the same AS (but to different BGP peers). The first-hop AS
 *   can be seen as a pivot node and a choice will be made as usual. This is not an issue.
 *   ex: bgpreader -t ribs -c route-views.saopaulo -w 1618876800,1618877100 -A 53013
 * - There are some prefixes where the last AS can be different, meaning that there will be two roots
 *   in the tree of path that is built for the heuristic application. Once again, this is not an issue, the
 *   algorithm can handle two different roots.
 *   ex: bgpreader -t ribs -c rrc22 -w 1618876800,1618877100 -k 176.109.160.0/22
 */
func apply_valley_free_heuristic (routing_entries_set, current_routing_entries_set *SafeSet, ases_interest []string) {
    
    /* --- Build the tree of path --- */
    _, nodes := build_tree (current_routing_entries_set)

    /* --- Record the number of paths going through the next-hops (relative to pivots) --- */
    next_hops_count := make (map[string]int)
    for pivot_node,_ := range nodes.pivot_nodes {
        for routing_entry, _ := range nodes.node_to_entries[pivot_node] { // Reversed path.
            path := routing_entry.as_path

            index := find_index (path, pivot_node)
            if index == 0 { 
                // This means that we are in a situation such that:
                // 1. X Y Z
                // 2. W Y
                // Y will be considered a pivot node (by construction of the tree, but it's not really a pivot node -> Just delete it)
                delete (nodes.pivot_nodes, pivot_node)
                continue
            }
            next_hop := path[index-1]
            next_hops_count[next_hop] = len(nodes.node_to_entries[next_hop])
        }
    }
    // Convert to map of interface for compiling reasons.
    next_hops_count_i := make (map[string]interface{}, len (next_hops_count))
    for s,i := range next_hops_count {
        next_hops_count_i[s] = i
    }
    max_next_hop, nb := search_map (next_hops_count_i, MoreInt) // Get more popular next-hop

    /* --- Select entries among those going through pivot nodes --- */
    selected_entries := make (map[*Rib_entry]interface{})
    for pivot_node,_ := range nodes.pivot_nodes { // Loop over all pivot nodes
        selected_entry := select_entry (pivot_node, nodes.node_to_entries[pivot_node], max_next_hop, nb)
        selected_entries[selected_entry] = struct{}{}
    }

    /* --- Take into account the paths that don't go through pivot nodes --- */
    var prefix string
    for prefix_counter, routing_entry_i := range current_routing_entries_set.set {
        /* --- Get prefix --- */
        if prefix == "" {
            prefix = strings.Split (prefix_counter, "_")[0]
        }

        routing_entry := routing_entry_i.(*Rib_entry)
        found := false
        for pivot_node,_ := range nodes.pivot_nodes { 
            for entry,_ := range nodes.node_to_entries[pivot_node] {
                if entry == routing_entry {
                    found = true
                    break
                }
            }
            if found {
                break
            }
        }
        if !found {
            selected_entries[routing_entry] = struct{}{}
        }
    }

    /* --- Add best routing entry to the rest --- */
    s := select_entry ("", selected_entries, "", 0)
    if s != nil { // If all entries have been deleted because of loops.
        routing_entries_set.unsafe_add (prefix, s) // Choice on shortest path then most AS of interest.
    }

    /* --- Delete all current entries --- */
    for k := range current_routing_entries_set.set {
        delete (current_routing_entries_set.set,k)
    }
}

/**
 * returns true if the heuristic could be applied (subsequent heuristics mustn't be launched)
 * or false if it could not be applied.
 */
type heuristic_fn func (string, *Rib_entry, *string, **Rib_entry) bool

func generate_valley_free_heuristic (pivot_node string) heuristic_fn {
    return func (next_hop string, routing_entry *Rib_entry, selected_next_hop *string, selected_entry **Rib_entry) bool {
        if get_relationship (pivot_node, next_hop) == get_relationship (pivot_node, *selected_next_hop) {
            return false
        }
        if get_relationship (pivot_node, next_hop) < get_relationship (pivot_node, *selected_next_hop) {
            *selected_entry = routing_entry
            *selected_next_hop = next_hop
        }
        return true
    }
}

func generate_heuristic_check (pivot_node string) heuristic_fn {
    return func (next_hop string, routing_entry *Rib_entry, selected_next_hop *string, selected_entry **Rib_entry) bool {
        if get_relationship (pivot_node, next_hop) == get_relationship (pivot_node, *selected_next_hop) {
            return false // Subsequent heuristics can be applied
        }
        return true // Subsequent heuristics won't be applied
    }
}

func generate_next_hop_popularity_heuristic (pivot_node, max_next_hop string, nb int) heuristic_fn {
    return func (next_hop string, routing_entry *Rib_entry, selected_next_hop *string, selected_entry **Rib_entry) bool {
        if nb == 1 && (next_hop != *selected_next_hop) {
            if next_hop == max_next_hop { 
                *selected_next_hop = next_hop
                *selected_entry = routing_entry
            }
            return true
        }
        return false
    }
}

func generate_shortest_path_heuristic () heuristic_fn {
    return func (next_hop string, routing_entry *Rib_entry, selected_next_hop *string, selected_entry **Rib_entry) bool {
        if len (routing_entry.as_path) == len ((*selected_entry).as_path) {
            return false
        }
        if len (routing_entry.as_path) < len ((*selected_entry).as_path) {
            *selected_next_hop = next_hop
            *selected_entry = routing_entry
        }
        return true
    }
}

func generate_most_ases_interest_heuristic () heuristic_fn {
    return func (next_hop string, routing_entry *Rib_entry, selected_next_hop *string, selected_entry **Rib_entry) bool {
        if len (routing_entry.as_path) == len ((*selected_entry).as_path) {
            if len (routing_entry.as_to_next_hop_AS) > len ((*selected_entry).as_to_next_hop_AS) {
                *selected_next_hop = next_hop
                *selected_entry = routing_entry
            }
            return true
        }
        return false
    }
}

/**
 * Given a pivot node, browse all paths going through that pivot node and select the best one according to:
 * 1. Valley free heuristic
 * 2. Tie-break: most popular next-hop
 * 3. Tie-break: shortest AS path
 * 4. Tie-break: AS path with the most ASes of interest
 */
func select_entry (pivot_node string, entries map[*Rib_entry]interface{}, max_next_hop string, nb int) *Rib_entry {
    
    /* --- Select heuristics to apply --- */
    heuristics := make ([]heuristic_fn, 0, 4)
    if pivot_node != "" {
        heuristics = append (heuristics, generate_valley_free_heuristic (pivot_node))
        heuristics = append (heuristics, generate_heuristic_check (pivot_node)) // Check for subsequent heuristics.
        heuristics = append (heuristics, generate_next_hop_popularity_heuristic (pivot_node, max_next_hop, nb))
    }
    heuristics = append (heuristics, generate_shortest_path_heuristic ())
    heuristics = append (heuristics, generate_most_ases_interest_heuristic ())

    /* --- Apply heuristics --- */
    var selected_entry *Rib_entry
    var selected_next_hop string
    for routing_entry, _ := range entries { // Loop over all the paths going through pivot_node
        path := routing_entry.as_path
        index := find_index (path, pivot_node)
        var next_hop string
        if index == -1 {
            next_hop = "" // Special case where we don"t care about the next hop
        } else {
            next_hop = path[index-1]
        }
        
        if selected_entry == nil { // First iteration
            selected_entry = routing_entry
            selected_next_hop = next_hop
            continue
        }

        for _, heuristic := range heuristics {
            if heuristic (next_hop, routing_entry, &selected_next_hop, &selected_entry) {
                break // If heuristic could be applied, stop applying subsequent heuristics
            }
        }
    }
    return selected_entry
}

/**
 * From a set of AS path for a given prefix, will build the tree of path rooted at the destination AS.
 * Removes AS pre-pending and routing loops in the paths.
 *  More precisely:
 *  X A B C X -> Will be deleted
    X A A A X -> Will be deleted
    X X X A B -> becomes X A B
 */
func build_tree (current_routing_entries_set *SafeSet) (*tree.Tree, *Nodes) {

    /* --- Build the tree of path --- */
    path_tree := &tree.Tree{}
    nodes := NewNodes ()
    f_absent := generate_if_absent (nodes)
    f_present := generate_if_present (nodes)
    for i, routing_entry_i := range current_routing_entries_set.set {
        routing_entry := routing_entry_i.(*Rib_entry)
        routing_entry.as_path = remove_duplicates (routing_entry.as_path)
        if routing_loop (routing_entry.as_path) {
            delete (current_routing_entries_set.set, i) // Safe to delete key while iterating
            continue
        } 

        reverse (routing_entry.as_path) // In place modification.
        path_tree.Add (routing_entry.as_path, f_absent, f_present, routing_entry)
    }
    return path_tree, nodes
}

func append_rib (set *map[string]map[*Rib_entry]interface{}, args ...interface{}) {
    /* --- Check nb args --- */
    var l interface{}
    switch len (args) {
        case 2: l = struct{}{}
        case 3: l = args[2]
        default: log.Fatal ("Wrong number of arguments to function [append_prefix]")
    }
    as := args[0].(string)
    prefix := args[1].(*Rib_entry)

    if _, ok := (*set)[as]; ok {
        (*set)[as][prefix] = l
    } else {
        (*set)[as] = map[*Rib_entry]interface{}{prefix: l}
    }
}

/* ==================================== *\
        SHORTEST PATH HEURISTIC
\* ==================================== */

func apply_shortest_path_heuristic (routing_entries_set, current_routing_entries_set *SafeSet, ases_interest []string) {

    // Get prefix
    var prefix string
    selected_entries := make (map[*Rib_entry]interface{})
    for prefix_counter,entry := range current_routing_entries_set.set {
        prefix = strings.Split (prefix_counter, "_")[0]
        selected_entries[entry.(*Rib_entry)] = struct{}{}
    }

    /* --- Add best routing entry to the rest --- */
    s := select_entry ("", selected_entries, "", 0)
    if s != nil { // If all entries have been deleted because of loops.
        routing_entries_set.unsafe_add (prefix, s) // Choice on shortest path then most AS of interest.
    }

    /* --- Delete all current entries --- */
    for k := range current_routing_entries_set.set {
        delete (current_routing_entries_set.set,k)
    }
}
