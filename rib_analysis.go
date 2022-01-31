/* ============================================================= *\
   rib_analysis.go

   Functions to either:
   - build some datasets necessary for Anaximander's 
     simulation (from the parsing results of the RIBs).
     -> Functions starting with 'build'
   - further analyse the parsing results of the RIBs.
     -> Functions starting with 'analyse'
\* ============================================================= */

package main

import ("log"
      "strconv"
      "os/exec"
      "strings"
      "fmt"
      graph "github.com/Emeline-1/basic_graph"
      pool "github.com/Emeline-1/pool")

/* ---------------------------------- *\
        BUILD DIRECTED PROBES
\* ---------------------------------- */

/**
 * Given the next-hop AS saved by 'rib_multi', build a new directed probes file
 * per AS (based on the best path thus).
 *
 * - outdir: where to store the results
 * - ases_file: the file containing the ases of interest (white space separated)
 * - collectors_file: the file containing the collectors (new line separated)
 * - dir: the directory where to find the parsing results of 'rib_multi'
 */
func build_best_path_directed_probes (outdir, ases_file, collectors_file, dir string) {
    collectors,_ := read_newline_delimited_file (collectors_file, 0)
    ases_interest,_ := read_whitespace_delimited_file (ases_file)

    /* --- Data struct initialization --- */
    as_targets := make (map[string]map[string]interface{})
    for _, AS := range ases_interest {
        as_targets[AS] = make (map[string]interface{})
    }

    /* --- Reading of forwarding table --- */
    for _, collector := range collectors {
        file := dir + "/next-hop_AS/" + collector + "/next_hop_AS_" + collector + ".txt" // Forwarding table (format: prefix as_interest next_as)

        reader := NewCompressedReader (file)
        reader.Open ()
        scanner := reader.Scanner ()
        for scanner.Scan () {
            line := strings.Fields (scanner.Text ())
            as_targets[line[1]][line[0]] = struct{}{} //Note: could keep track on which collector it was seen. Later maybe.
        }
        reader.Close ()
    }

    /* --- Write directed probes to file --- */
    for AS, targets := range as_targets {
        s := create_safeset ()
        s.set = targets
        s.write_to_file (outdir + "/directed_prefixes_" + AS + ".txt")
    }
}

/**
 * Gives the average and variance of the number of directed prefixes for each AS, per BGP collector
 * - dir: the directory where to find the parsing results of 'rib_multi'
 */
func analyse_directed_probes_per_collector (ases_file, collectors_file, dir string) {
    collectors,_ := read_newline_delimited_file (collectors_file, 0)
    ases_interest,_ := read_whitespace_delimited_file (ases_file)

    as_collectors := make (map[string]DataFloat64)
    for _, AS := range ases_interest {
        as_collectors[AS] = make (DataFloat64, 0, len (collectors))
        for _, collector := range collectors {
            file := dir + "/next-hop_AS/" + collector + "/next_hop_AS_" + collector + "_" + AS + ".txt" // (format: prefix next_as)
            
            cmd := "wc -l < " + file
            out,err := exec.Command("bash", "-c", cmd).Output()
            if err != nil {
                log.Println ("Problem in command: ", err.Error ())
            }
            nb,err := strconv.Atoi (strings.TrimSuffix(string (out), "\n"))
            if err != nil {
                log.Println ("Problem in strconv: ", err.Error ())
            }
            as_collectors[AS] = append (as_collectors[AS], float64 (nb))
        }
    }

    for as, collectors := range as_collectors {
        log.Printf("AS %s: Mean: %.2f, Var: %.2f", as, collectors.Mean (), collectors.Variance ())
    }
}

/* ---------------------------------- *\
          OVERLAYS ANALYSIS
\* ---------------------------------- */

/**
 * Given the overlays for each collector separately, build the merged 
 * overlay file for all collectors together.
 * - dir: the directory where to find the parsing results of 'rib_multi'
 */
func build_merge_overlays (dir string) {
    /* --- Get all files in dir --- */
    overlay_files := pool.Get_directory_files (dir + "/overlays/")
    if overlay_files == nil {
        return
    }

    /* --- Compute transitive closure of overlays thanks to graphs connected components --- */
    g := graph.New ()
    for _, file := range *overlay_files {
        reader := NewCompressedReader (file)
        reader.Open ()
        scanner := reader.Scanner ()
        for scanner.Scan () {
            overlays := strings.Fields (scanner.Text ())
            for _, overlay := range overlays[1:] {
                g.Add_edge (overlays[0], overlay)
            }
        }
        reader.Close ()
    }

    /* --- Record merged overlays --- */
    overlays_closure := create_safeset ()
    g.Set_iterator ()
    for g.Next_connected_component () {
        connected_component := g.Connected_component ()
        overlays_closure.unsafe_add (connected_component[0], connected_component[1:])
    }
    overlays_closure.write_to_file (dir + "/overlays/all_overlays.txt")
}


/**
 * Given all the forwarding tables, gives the mean reduction of the forwarding tables that the overlays can allow.
 * Note: This is a global reduction on a complete forwarding table. To know what will be the real reduction in 
 * the context of an AS mapping, we need to simulate with the Anaximander Simulator.
 */
func analyse_overlays (forwarding_tables []string) {
    reductions := make (DataFloat64, 0, len (forwarding_tables))
    for _, file := range forwarding_tables {
        //Extract collector name
        s := strings.Split (file, "/")
        collector := s[len (s)-1]

        // Get corresponding overlay file
        overlay_file_s := s[:len (s)-2]
        overlay_file := strings.Join (overlay_file_s, "/")
        overlay_file += "/overlays/overlays_" + collector

        nb_groups, total := _analyse_overlay (overlay_file, 1)

        // Get nb of entries in forwarding tables
        out, err := exec.Command("bash", "-c", "wc -l < " + file).Output()
        if err != nil {
            panic ("[analyse_overlays]: Problem while counting forwarding entries " + file + ": " + err.Error ())
        }
        nb, errn := strconv.Atoi (strings.TrimSuffix(string (out), "\n"))
        if errn != nil {
            log.Fatal ("could not get nb entries" + errn.Error ())
        }

        new_targets := nb - total + nb_groups
        reductions = append (reductions, float64 (new_targets)/float64 (nb))
    }

    log.Println ("Mean reduction:", reductions.Mean ())
    log.Println ("Var reduction:", reductions.Variance ())
}

/**
 * Same as above, but based on the merged overlays.
 */
func analyse_merged_overlays (all_overlay_file string, forwarding_tables []string) {
    reductions := make (DataFloat64, 0, len (forwarding_tables))
    nb_groups, total := _analyse_overlay (all_overlay_file, 1)
    for _, file := range forwarding_tables {
        // Get nb of entries in forwarding tables
        out, err := exec.Command("bash", "-c", "wc -l < " + file).Output()
        if err != nil {
            panic ("[analyse_overlays]: Problem while counting forwarding entries " + file + ": " + err.Error ())
        }
        nb, errn := strconv.Atoi (strings.TrimSuffix(string (out), "\n"))
        if errn != nil {
            log.Fatal ("could not get nb entries" + errn.Error ())
        }

        new_targets := nb - total + nb_groups
        reductions = append (reductions, float64 (new_targets)/float64 (nb))
    }

    log.Println ("Mean reduction:", reductions.Mean ())
    log.Println ("Var reduction:", reductions.Variance ())
}

func _analyse_overlay (overlay_file string, nb_vp int) (int, int) {
    // Reading the overlays
    reader := NewCompressedReader (overlay_file)
    reader.Open ()
    scanner := reader.Scanner ()

    reduction := 0 // the nb of prefixes you keep
    total := 0
    for scanner.Scan () {
        nb_overlays := len (strings.Fields (scanner.Text ()))
        total += nb_overlays
        reduction += min (nb_vp, nb_overlays)
    }
    reader.Close ()
    return reduction, total
}

/**
 * Gives the theoretical reduction of overlays such as:
 * For the overlays, spread each overlay group among the VPs, and if there are more overlays in the group than 
 *  there are VPs, we have a reduction.
 */
func analyse_overlays_repartition_vp (overlay_file, forwarding_table string) {

    reductions := make ([]int, 0, 25)
    for i := 1; i < 25 ; i++ {
        to_keep, total := _analyse_overlay (overlay_file, i)

        // Get nb of entries in forwarding tables
        out, err := exec.Command("bash", "-c", "wc -l < " + forwarding_table).Output()
        if err != nil {
            panic ("[analyse_overlays]: Problem while counting forwarding entries " + forwarding_table + ": " + err.Error ())
        }
        nb, errn := strconv.Atoi (strings.TrimSuffix(string (out), "\n"))
        if errn != nil {
            log.Fatal ("could not get nb entries" + errn.Error ())
        }
        new_targets := nb - total + to_keep
        reductions = append (reductions, new_targets)
    }
    log.Println ("The reductions: ", reductions)
}

/**
 * Given:
 * - the file with all overlays (merged)
 * - the directed prefixes for each AS of interest.
 * 
 * builds a file per AS of interest with all its overlays (same format as the all overlays file)
 */
func build_overlays_per_AS (ases_file, all_overlays_file, directed_prefixes_dir, outdir string) {
    ases,_ := read_whitespace_delimited_file (ases_file)

    /* --- Read all overlays --- */
    overlays := read_overlay_file (all_overlays_file)

    /* --- Loop over ASes of interest --- */
    for _, AS := range ases {
        directed_prefixes,_ := read_newline_delimited_file (directed_prefixes_dir + "/directed_prefixes_"+AS+".txt", 0)

        overlays_per_AS := create_safeset ()
        /* --- Parse directed prefixes --- */
        for _, prefix := range directed_prefixes {
            if o, ok := overlays[prefix]; ok {
                overlays_per_AS.unsafe_add (prefix, get_keys (&o))
            }   else {
                overlays_per_AS.unsafe_add (prefix, struct{}{})
            }
        }

        /* --- Write overlays to file --- */
        overlays_per_AS.write_to_file (outdir + "/overlays_" + AS + ".txt")
    }
}

/* ---------------------------------- *\
      AS PATH and Tier1 ANALYSIS
\* ---------------------------------- */

/**
 * Analyze the AS path of BGP dump entries.
 */
func analyse_ribs (output_filename, collectors_file, relfile, start, end string) {
    set := create_safeset ()
    collectors,_ := read_newline_delimited_file (collectors_file, 0)

    /* Read AS rel file */
    log.Println ("Reading AS relationships...")
    tiers1 := read_providers (relfile)

    /* Read RIBs */
    log.Println ("Reading RIBs...")
    bgp_dump_analyser := generate_RIB_as_path_analyser (set, tiers1, start, end)
    pool.Launch_pool (32, collectors[0:1], bgp_dump_analyser)
    set.write_to_file (output_filename)
}

/**
 * Same analysis as above, but on the constructed FIBs instead of on the RIBs.
 */
func analyse_fibs (data_dir, collectors_file, relfile, output_file string) {
    collectors,_ := read_newline_delimited_file (collectors_file, 0)

    /* Read AS rel file */
    log.Println ("Reading AS relationships...")
    tiers1 := read_providers (relfile)

    /* Read forwarding tables */
    set := create_safeset ()
    values := make (DataFloat64, 0, 100)
    for _, collector := range collectors {
        file := data_dir + "/" + collector + ".txt"
        log.Println (file)

        reader := NewCompressedReader (file)
        err := reader.Open ()
        if err != nil {
            continue
        }
        scanner := reader.Scanner ()
        nb_path := 0 // How many paths where the last hop was a Tier1
        nb_entries := 0 // How many path where the last two hops were Tiers1.
        for scanner.Scan () {
            line := scanner.Text()
            as_path := strings.Split (line, " ")[1:] // Remove first token, which isn't part of the AS PATH.
            r1,r2 := analyse_aspath (as_path, tiers1)
            if r1 != -1 {
                nb_entries += r2
                nb_path += r1
            }
        }
        reader.Close ()

        set.unsafe_append (collector, strconv.Itoa (nb_path))
        set.unsafe_append (collector, strconv.Itoa (nb_entries))

        values = append (values, float64 (nb_entries)/float64 (nb_path))
    }
    set.write_to_file (output_file)
    log.Println ("Mean:", values.Mean ())
    log.Println ("Var:", values.Variance ())
}

/* ---------------------------------- *\
                MISC.
\* ---------------------------------- */

/**
 * See how many traces are impacted by -1 ASes with some more particular stats:
 * - The first hop 
 * - The last hop
 * - Hop in between same ASes
 * - Hop in between different ASes
 */
func ases_stats (traces *SafeSet) {
    // --- Stats on -1 ases --- //
    // 549 addresses on 847 914 were attributed -1 by bdrmapit, which is negligeable.
    // But why are they 100 000 addresses missing from bdrmapit output?? This is 1/9th
    // of all addresses -> Non nongligeable!
    first_position := 0
    last_position := 0
    in_between_same := 0
    same_adjs := 0
    same_multi_adjs := 0
    in_between_diff := 0
    diff_adjs := 0
    diff_multi_adjs := 0
    other := 0
    var trace *Trace

    defer func () {
        if r := recover(); r != nil {
            log.Println (trace)
            return
        }
    } ()

    for _, trace_i := range traces.set {
        trace_v, t := trace_i.(*Trace)
        if !t {
            log.Fatal ("[ases_stats]: unexpected type:", fmt.Sprintf("%T", trace_i))
        }
        trace = trace_v
        for i, hop := range *trace {
            if hop.asn != "-1" {
                continue
            }
            if i == 0 && hop.probe_ttl != 1 { // Special case
                continue
            }
            if hop.probe_ttl == 1 {
                first_position++
            } else if i == len (*trace)-1 {
                last_position++
            } else if (*trace)[i-1].asn != (*trace)[i+1].asn {
                in_between_diff++
                if (*trace)[i+1].probe_ttl - (*trace)[i-1].probe_ttl == 2 { // A -1 B
                    diff_adjs++
                } else {
                    diff_multi_adjs++
                    // A -1 * B
                    // A * -1 B
                    // A * -1 * B
                }
            } else if (*trace)[i-1].asn == (*trace)[i+1].asn {
                in_between_same++
                if (*trace)[i+1].probe_ttl - (*trace)[i-1].probe_ttl == 2 { // A -1 A
                    same_adjs++
                } else {
                    same_multi_adjs++
                    // A -1 * A
                    // A * -1 A
                    // A * -1 * A
                }
            } else {
                other++
            }
        }
    }
    log.Println ("First position:", first_position)
    log.Println ("Last position:", last_position)
    log.Println ("In between same:", in_between_same)
    log.Println ("In between same - consecutive:", same_adjs)
    log.Println ("In between same - non consecutive:", same_multi_adjs)
    log.Println ("In between diff:", in_between_diff)
    log.Println ("In between diff - consecutive:", diff_adjs)
    log.Println ("In between diff - non consecutive:", diff_multi_adjs)
    log.Println ("Other:", other)
}