/* ================================================================= *\
     rocketfuel.go

     Partial RocketFuel simulator: Evaluation of the different Path 
     Reduction Techniques on TNT data:
     - Ingress Reduction
     - Next-Hop AS Reduction 
     - Directed Probing
     - Egress Reduction 
\* ================================================================= */

package main

import (
    "log"
    "time"
    "strings"
    "bufio"
    "math/rand"
    "sort"
    "os"
    "strconv"
    "fmt"
    "os/exec"
    "math/bits"
    pool "github.com/Emeline-1/pool")

/* --------------------------------------- *\
 *          Ingress Reduction
\* --------------------------------------- */
// Note: will not be able to simulate ingress reduction, as I am limited by TNT data, and cannot launch
// a trace from the VP that I want. 
// But it doesn't matter, as we already have this analysis. 
func ingress_reduction (ases_file, output_dir string) {
    traces,_,_,_,target_to_vp,_,_ := parse_warts ()
    ases,_ := read_whitespace_delimited_file (ases_file)

    /* --- Process traces --- */
    vp_as_ingresses := make (map[string]map[string]map[string]struct{})
    as_vpNextAs_egresses := make (map[string]map[string]map[string]struct{})

    for dst, trace_i := range traces.set {
        if trace, t := trace_i.(*Trace); t {
            /* -- Loop over hops -- */
            var ingress string
            for i,hop := range (*trace) {
                // Ingress reduction
                if hop.ingress == true {
                    for _,as := range ases {
                        if as == hop.asn { // We have an ingress for one of the ASes of interest
                            src_i,_ := target_to_vp.unsafe_get (dst)
                            src,_ := src_i.(string)
                            append_ingress (&vp_as_ingresses, src , as, hop.addr)
                            ingress = hop.addr
                        }
                    }
                }
                // Next hop AS reduction
                if hop.egress == true {
                    for _,as := range ases {
                        if as == hop.asn { // We have an egress for one of the ASes of interest
                            append_ingress (&as_vpNextAs_egresses, as, ingress + (*trace)[i+1].asn, hop.addr)
                        }
                    }
                }
            }
        } else {
            log.Fatal ("[parse_warts]: unexpected type:", fmt.Sprintf("%T", trace_i))
        }
        
    }

    print_table_to_file (vp_as_ingresses, ases, output_dir + "/ingresses_per_vp.txt")
    // --- Global stat on Next-hop ASes --- //
    as_nbegresses := make (map[string]interface{}) //Nb egresses, not by next-hop AS, but by (ingress, next-hop AS) pair
    for as, next_as := range as_vpNextAs_egresses {
        as_nbegresses[as] = make ([]string, 0, 10)
        for _, eggresses := range next_as {
            t := as_nbegresses[as].([]string)
            as_nbegresses[as] = append (t, strconv.Itoa (len (eggresses)))
        }
    }
    to_print := create_safeset ()
    to_print.set = as_nbegresses
    to_print.write_to_file (output_dir + "/nbegresses_per_as.txt")
}

func append_ingress (set *map[string]map[string]map[string]struct{}, src string, as string, addr string) {
    if as_ingresses, ok := (*set)[src]; ok {
        if _, ok2 := as_ingresses[as]; ok2 {
            (*set)[src][as][addr] = struct{}{}
        } else {
            (*set)[src][as] = map[string]struct{} {addr: struct{}{}}
        }
    } else {
        (*set)[src] = map[string]map[string]struct{} {as: map[string]struct{} {addr: struct{}{}}}
    }
}

/* --------------------------------------- *\
 *          AS Next-Hop Reduction
\* --------------------------------------- */

/**
 * Preliminary analysis of the next-hop AS Rocketfuel Reduction technique.
 * Outputs:
 * - A file per AS of interest, giving the number of prefixes that saw more than 1 next-hop AS,
 *   as well as the number of different next-hop ASes.
 * 
 * - outdir: where to store the results
 * - ases_file: the file containing the ases of interest (white space separated)
 * - collectors_file: the file containing the collectors (new line separated)
 * - dir: the directory where to find the 'next-hop_AS' parsing results of 'rib_multi'
 */
func analyse_next_hops (outdir, ases_file, collectors_file, dir string) {

    exec.Command("bash", "-c", "mkdir " + outdir).Run()
    ases,_ := read_whitespace_delimited_file (ases_file)
    collectors,_ := read_newline_delimited_file (collectors_file, 0)

    for _, AS := range ases {
        // key: the prefix
        // value: all next ASes that were seen for that prefix
        prefix_nextASes := make (map[string]map[string]interface{})

        for _, collector := range collectors {
            file := dir + "/" + collector + "/next_hop_AS_" + collector + "_" + AS + ".txt" // (format: prefix next_as)
            log.Println (file)

            reader := NewCompressedReader (file)
            err := reader.Open ()
            if err != nil {
                continue
            }
            scanner := reader.Scanner ()
            for scanner.Scan () {
                line := strings.Fields (scanner.Text ())
                prefix := line[0]
                nextAS := line[1]

                if _, ok := prefix_nextASes[prefix]; ok {
                    prefix_nextASes[prefix][nextAS] = struct{}{}
                } else {
                    prefix_nextASes[prefix] = map[string]interface{}{nextAS: struct{}{}}
                }
            }
            reader.Close ()

        }

        f, err := os.Create(outdir + "/" + AS + ".txt") // If the file already exists, it is truncated
        if err != nil {
            log.Print ("[write_to_file]: " + err.Error())
            return
        }

        w := bufio.NewWriter(f)
        for key, s := range prefix_nextASes {
            if len (s) != 1 { // Only print prefixes for which more than one next-hop AS was seen.
                w.WriteString(key + " " + strconv.Itoa (len (s)) + "\n")
            }
        }
        f.Close ()
    }
}

/**
 * Preparation for a more in-depth analysis of the next-hop AS Rocketfuel reduction technique.
 * The output generated can be used by the simulator to simulate next-hop AS Reduction.
 * Outputs:
 * - Given the next-hops ASes for each collector and each AS, builds the directed prefixes
 * for each AS (all collectors merged together). This is actually equivalent to building the directed probes
 * from the next-hop ASes, but with an indication of the next-hop AS for the prefix.
 * 
 * - outdir: where to store the results
 * - ases_file: the file containing the ases of interest (white space separated)
 * - collectors_file: the file containing the collectors (new line separated)
 * - dir: the directory where to find the 'next-hop_AS' parsing results of 'rib_multi'
 */
func merge_next_hops (outdir, ases_file, collectors_file, dir string) {

    exec.Command("bash", "-c", "mkdir " + outdir).Run()
    ases,_ := read_whitespace_delimited_file (ases_file)
    collectors,_ := read_newline_delimited_file (collectors_file, 0)

    for _, AS := range ases {
        // key: the prefix
        // value: the next-hop AS
        prefix_nextAS := make (map[string]interface{})

        for _, collector := range collectors {
            file := dir + "/" + collector + "/next_hop_AS_" + collector + "_" + AS + ".txt" // (format: prefix next_as)
            log.Println (file)

            reader := NewCompressedReader (file)
            err := reader.Open ()
            if err != nil {
                continue
            }
            scanner := reader.Scanner ()
            for scanner.Scan () {
                line := strings.Fields (scanner.Text ())
                prefix := line[0]
                nextAS := line[1]

                prefix_nextAS[prefix] = nextAS // Last one of the collectors to be read will be the one that is kept.
            }
            reader.Close ()
        }

        s := create_safeset ()
        s.set = prefix_nextAS
        s.write_to_file (outdir + "/merged_next_AS_" + AS + ".txt")
    }
}

/* --------------------------------------- *\
 *  Directed Probing and Egress Reduction
\* --------------------------------------- */

/** 
 * Read RIB tables and retrieve prefixes where the AS of interest was seen in the AS path.
 * The prefix is accompanied with a mention of whether it is a dependent or up/down prefix.
 */
func parse_ribs_dependent (as, collectors_file, output_filename string, break_prefix bool, start, end string) {
    rand.Seed(time.Now().UnixNano())
    
    set := create_safeset ()
    /* --- ASes of interest --- */
    ases := []string {as}

    /* --- With collectors --- */
    log.Print ("Retrieving collectors... ")
    //collectors := broker_get_collectors () // Cannot distinguish bad collectors. 
    collectors,_ := read_newline_delimited_file (collectors_file, 0)
    log.Print ("Done")
    sort.Strings(collectors)
    if collectors == nil {
        return
    }
    if len (collectors) > 64 {
        log.Fatal ("Fatal error: cannot handle more than 64 collectors")
    }
    
    collectors_to_index := assign_numbers (collectors)
    bgp_dump_parser := generate_RIB_parser_dependent (set, ases, collectors_to_index, break_prefix, start, end)
    pool.Launch_pool (32, collectors, bgp_dump_parser)

    log.Print ("Writing to file")
    set.write_to_file (output_filename, generate_print_collectors (len (collectors_to_index)))
}

func generate_print_collectors (count int) PrintFn {
    return func (w *bufio.Writer, key string, v interface{}) error {
        var err error
        if value, ok := v.(uint64); ok {
            if count == bits.OnesCount64 (value) { // Dependent prefix
                    _, err = w.WriteString(key + " d " + strconv.FormatUint (value,2) + "\n")
                } else { // Up/down prefix
                    _, err = w.WriteString(key + " u/d " + strconv.FormatUint (value,2) + "\n")
            }
        } else {
            log.Fatal ("Unexpected type: %T\n", v)
        }
        return err
    }
}

/* --------------------------------------- *\
 *              MISC.
\* --------------------------------------- */
func print_table_to_file (vp_as_ingresses map[string]map[string]map[string]struct{}, ases []string, output_file string) {

    /* --- Write result to file --- */
    f, err := os.Create(output_file) // If the file already exists, it is truncated
    if err != nil {
        log.Print ("[print_to_file]: " + err.Error())
        return
    }
    defer f.Close ()
    w := bufio.NewWriter(f)

    // Header
    _, err = w.WriteString("VP " + strings.Join (ases, " ")+"\n")
    // Content
    for vp, as_ingresses := range vp_as_ingresses {
        line_s := make ([]string,0,len (ases)+1)
        line_s = append (line_s, vp) // vp name
        for _, as := range ases { // nb ingresses per AS for that VP.
            line_s = append (line_s, strconv.Itoa (len (as_ingresses[as])))
        }
        _, err = w.WriteString(strings.Join (line_s, " ")+"\n")
        if err != nil {
            log.Print ("[print_to_file]: " + err.Error())
            return
        }
    }
    w.Flush()
}
