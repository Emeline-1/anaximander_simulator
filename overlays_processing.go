package main

import (
    "strings"
    radix "github.com/Emeline-1/radix"
    graph "github.com/Emeline-1/basic_graph")

/* =============================================== *\
                Overlay Computation
\* =============================================== */

/**
 * Input: a forwarding table (one entry per prefix)
 * Output: a set containing the overlays and their aggregate
 *
 * The overlays don't have to span the aggregate exactly, they can be isolated.
 */
func process_overlays (routing_entries_set *SafeSet) *SafeSet {
    // Note: If I have 4 more specifics that span an aggregate, but that the aggregate is not
    // in the table, then the overlays won't be found.
    // In the probing, 4 probes are sent that could be reduced to 1.
    
    /* --- Build Radix tree from forwarding table, recording AS path of each entry --- */
    tree := radix.New()
    for prefix, rib_entry_i := range routing_entries_set.set {
        rib_entry, _ := rib_entry_i.(Rib_entry)
        radix_prefix := get_binary_string (prefix)
        tree.Insert (radix_prefix, strings.Join (rib_entry.as_path, " "))
    }

    /* --- Walk radix tree, recording overlays (parent and direct children) --- */
    overlays := create_safeset ()
    walk_radix_tree := generate_walk_radix_tree (overlays)
    tree.Walk_post (walk_radix_tree)

    /* --- Compute transitive closure of overlays thanks to graphs connected components --- */
    g := graph.New ()
    for aggregate, overlays_i := range overlays.set {
        overlays_v, _ := overlays_i.(map[string]struct{})
        for overlay, _ := range overlays_v {
            g.Add_edge (aggregate, overlay)
        }
    }

    overlays_closure := create_safeset ()
    g.Set_iterator ()
    for g.Next_connected_component () {
        connected_component := g.Connected_component ()
        overlays_closure.unsafe_add (connected_component[0], connected_component[1:])
    }
    return overlays_closure
}

/**
 * Function performing an action during the post-order walk of a radix tree.
 * - overlays: key: the aggregate prefix
 *             value: all its overlays.
 */
func generate_walk_radix_tree (overlays *SafeSet) radix.WalkFnPost {
    return func (parent *radix.LeafNode, children []*radix.LeafNode) {
        aggregate_prefix := get_prefix_from_binary (parent.Key)
        aggregate_aspath,_ := parent.Val.(string)

        marked_prefixes := make ([]string, 0, len (children))
        marked_ases := make ([]string, 0, len (children))
        for _, more_specific := range children {
            more_specific_aspath,_ := more_specific.Val.(string)
            if more_specific_aspath == aggregate_aspath {
                overlays.unsafe_append (aggregate_prefix, get_prefix_from_binary (more_specific.Key))
            } else {
                marked_prefixes = append (marked_prefixes, more_specific.Key) 
                marked_ases = append (marked_ases, more_specific.Val.(string))
            }
        }

        /* --- Detect implicit aggregate of overlays --- */
        // NB: not perfect, only detect overlays if the children are exactly the overlays (don't do several groups)
        nb_prefixes := len (marked_prefixes)
        if nb_prefixes >= 2 {

            common_prefix := longestCommonPrefix (marked_prefixes)
            if common_prefix == "" {
                return
            }
            
            suffix_length := len (marked_prefixes[0]) - len (common_prefix)
            if IntPow(2, suffix_length) == nb_prefixes { // Implicit aggregate detected
                if same (marked_ases) {
                    for _, prefix := range marked_prefixes {
                        overlays.unsafe_append (get_prefix_from_binary (common_prefix), get_prefix_from_binary (prefix))
                    }
                }
            }
        }
    }
}

/* =============================================== *\
                Overlay Reduction
\* =============================================== */

/**
 * Given: 
 * - AS_probes: a mapping between an AS and all its probes (must be raw probes)
 * - ases: the ases for which the overlay reduction must be applied
 * - target_to_vp: a mapping between a target (/24) and the VP that launched it
 * - overlays: the overlays per collector/VP (raw, come directly from the forwarding tables)
 * only record one prefix (/24) per overlay group.
 *
 * Post: overlay reduction has been applied to AS_probes
 *
 * The issue is that we are working on a raw (no /24) level, but that to get the VP, we need to pick a /24 randomly from the
 * raw prefix.
 * Because of this, some targets will be reduced, some not, depending on the VP that we get. But we cannot control everything,
 * because of TNT data.
 */
func remove_overlays (AS_probes map[string]map[string]interface{}, ases []string, target_to_vp *SafeSet, overlays map[string]map[string]map[string]interface{}) {
    
    /* --- Range over the ASes --- */
    for _, AS := range ases {
        seen := make (map[string]map[string]interface{})
        s := make (map[string]interface{})

        /* --- Range over the probes of the ASes --- */
        for probe,_ := range AS_probes[AS] {
            probe_24 := _get_24_prefix (probe)
            VP_i, present := target_to_vp.get (probe_24)
            if ! present { // some directed probes are not in the traces. Simply add it in the probes (in order not to count that
                // as an overlay reduction. And it will be ignored by the simulation engine anyway).
                s[probe_24] = struct{}{}
                continue
            }
            VP,_ := VP_i.(string)

            if _, present := seen[VP][probe]; present {
                continue 
            } else {
                s[probe_24] = struct{}{} // Record probe
                // Record all other probes in its overlay group
                overlays_group := overlays[VP][probe]
                append_overlays (seen, VP, overlays_group)
            }
        }

        /* --- Update the probes of the AS with the overlay reduction --- */
        AS_probes[AS] = s
    }
}

func append_overlays (seen map[string]map[string]interface{}, vp string, overlays map[string]interface{}) {
    if already_seen, present := seen[vp]; present {
        seen[vp] = merge_maps (already_seen, overlays)
    } else {
        seen[vp] = merge_maps_new (overlays, nil) // Make a copy
    }
}