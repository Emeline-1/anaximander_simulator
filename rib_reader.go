/* ============================================================= *\
     rib_reader.go

     Specifics of parsing the RIBs from the RouteViews and 
     RIRs projects.
     - Can count the number of prefixes per collector to determine
       which collectors are valid or not.
     - Can output a file of prefixes for which an AS of interest
       was seen in the AS path, and indicate if this prefix is a
       dependent prefix or an up/down prefix (RocketFuel Directed 
       Probes and Egress Reduction evaluation).
       Note: select valid collectors before doing that operation.
     - Other functions are available as well, check the file for
       further details.
\* ============================================================= */

package main

import (
    "log"
    "strings"
    "bufio"
    "os/exec"
    "net"
    "strconv"
    pool "github.com/Emeline-1/pool")

var reserved_prefixes [15]net.IPNet = [15]net.IPNet{
    *string_to_net ("0.0.0.0/8"),
    *string_to_net ("10.0.0.0/8"),
    *string_to_net ("100.64.0.0/10"),
    *string_to_net ("127.0.0.0/8"),
    *string_to_net ("169.254.0.0/16"),
    *string_to_net ("172.16.0.0/12"),
    *string_to_net ("192.0.0.0/24"),
    *string_to_net ("192.0.2.0/24"),
    *string_to_net ("192.88.99.0/24"),
    *string_to_net ("192.168.0.0/16"),
    *string_to_net ("198.18.0.0/15"),
    *string_to_net ("198.51.100.0/24"),
    *string_to_net ("203.0.113.0/24"),
    *string_to_net ("224.0.0.0/4"),
    *string_to_net ("240.0.0.0/4"),
}

func check_prefix_validity (prefix string) (*net.IPNet, bool) {
    ip, network, err := net.ParseCIDR (prefix)
    if err != nil {
        log.Print ("[check_prefix_validity]: " + err.Error() + ": " + prefix)
        return nil, false
    }
    /* --- Not an IPv4 address --- */
    if network.IP.To4 () == nil {
        return nil, false
    }
    /* --- Sound BGP entries --- */
    l,_ := network.Mask.Size ()
    if l < 8 || l > 24 {
        return nil,false
    }
    /* --- Reserved address --- */
    for _, reserved := range reserved_prefixes {
        if reserved.Contains (ip) {
            return nil,false
        }
    }
    return network, true
}

/**
 * Starts a command and wait until it is completed.
 * The done channel is to receive a signal when the processing of the command is completed. (This is different from the cmd that
    * is completed. For example, if the processing takes more time than the execution of the command itself).
 * Returns true if no errors, false otherwise
 */
func start_and_wait (cmd *exec.Cmd, done chan struct{}) bool {
    err := cmd.Start() // Non blocking
    if err != nil {
        log.Print ("[start_and_wait]: Start: " +  err.Error())
        return false
    }
    
    <-done // Wait for the whole file to be processed

    err = cmd.Wait() // Wait for the command to finish
    if err != nil {
        log.Print ("[start_and_wait]: Wait: " + err.Error())
        return false
    }
    return true
}

type Rib_entry struct{
    as_path       []string
    as_to_next_hop_AS       map[string]string
}

/**
 * Print each and every routing entry as
 * [prefix as_path]
 */
func print_rib_entry (w *bufio.Writer, key string, v interface{}) error {
    var err error
    if value, ok := v.(*Rib_entry); ok {
        _, err = w.WriteString(key + " " + strings.Join (value.as_path, " ") + "\n")
    } else {
        log.Fatal ("Unexpected type: %T", v)
    }
    return err
}

/**
 * Print a routing entry only if an AS of interest is in the path, as:
 * [prefix AS_interest next-hop_AS]
 */
func print_next_as (w *bufio.Writer, key string, v interface{}) error {
    var err error
    if value, ok := v.(*Rib_entry); ok {
        if len (value.as_to_next_hop_AS) != 0 {
            for as, next_hop_AS := range value.as_to_next_hop_AS {
                _, err = w.WriteString(key + " " + as + " " + next_hop_AS + "\n")
            }
        }
    } else {
        log.Fatal ("Unexpected type: %T", v)
    }
    return err
}           

/**
 * Returns a routing entry composed of:
 * - the AS path
 * - If one or more of the ASes of interest are present in the AS path, a mapping between
 *   the AS of interest and its next-hop AS or its previous-hop AS.
 * as_path format: AS1 AS2 ... ASn
 */
func get_Rib_entry (as_path string, ases_interest []string, direction int) *Rib_entry {
    ases := strings.Split (as_path, " ")

    r := &Rib_entry{as_path: ases, as_to_next_hop_AS: make (map[string]string)}

    for _,as_interest := range ases_interest {
        target := get_prev_or_next_as (as_interest, ases, direction)
        if target != "" {
            r.as_to_next_hop_AS[as_interest] = target
        }
    }
    return r
}

/**
 * Given an AS of interest, an AS path, and a direction,
 * returns the previous (if direction = -1) or the next (if direction = +1) AS
 * of the AS of interest.
 * Return "" if the AS of interest is not in the path.
 * 
 * Note: direction can only be +1 or -1.
 */
func get_prev_or_next_as (AS_interest string, as_path []string, direction int) string {
    for i,as := range as_path { // Loop over the AS path
        if as == AS_interest {
            // as of interest is first or last in the path.
            // Depending on whether we want next or previous AS, do not access illegal cells.
            if (i+direction < 0) || (i+direction) >= len (as_path) { 
                return AS_interest // To keep track of all prefixes where an AS of interest was seen in the path
            } else {
                return as_path[i+direction]
            }
        }
    }
    return ""
}

/* ------------------------------------------------- *\
            BGP multi parser
\* ------------------------------------------------- */

/**
 * ASSUMPTION: the RIB entries are always grouped by prefix. In other words, for a single prefix,
 * all entries are grouped together and not scattered all accross the file.
 * Assumption verified for the 44 valid collectors on April 20th, 2021
 * 
 * OUTPUTS:
 * - A file per collector and per AS of interest, giving for each prefix of the table, the next-hop AS in the format:
 *   [prefix nexthop_AS]
 *   
 * - A file per collector giving all the BGP peers of the collector in the format:
 *   [collector peer_1 peer_2 ... peer_n]
 *
 * - A file common to all collectors giving all the prefixes advertized by a given AS in the format:
 *   [origin_AS prefix_1 prefix_2 ... prefix_n]
 *
 * - A file per collector giving the overlays (new-line separated)
 */
func generate_RIB_parser (origin_set *SafeSet, ases_interest []string, output_dir, start, end string, heuristic int) func (string) {
    return func (collector_name string) {
        
        cmd := exec.Command("bgpreader", "-t", "ribs", "-c", collector_name, "-w", start+","+end) // No filtering on AS path
        r, _ := cmd.StdoutPipe() // Get a pipe to read from standard output
        //r,_ := os.Open ("ribs/valley_free_test/"+collector_name)
        scanner := bufio.NewScanner(r) // Create a scanner which scans the output line-by-line

        // Channel for communication when the goroutine is done parsing the whole file
        done := make(chan struct{}) // An empty struct takes up no memory space

        /* ----------------------- *\
                RIB Processing
        \* ----------------------- */
        routing_entries_set := create_safeset () // Keep for each prefix the RIB entry that corresponds to the 'best' AS path, according to heuristic
        current_routing_entries_set := create_safeset () // For the CURRENT prefix, keep track of ALL BGP entries.
        collector_peers_set := create_safeset () // Record BGP peers of current collector
        var prev_prefix string
        counter := 0
        memory_set := create_safeset () // For checking assumption.
        go func() {
            // Read line by line and process it
            for scanner.Scan() {
                line := scanner.Text()
                prev_prefix = parse_bgp_record_multi (memory_set, line, routing_entries_set, current_routing_entries_set, origin_set, collector_peers_set, ases_interest, prev_prefix, collector_name, &counter, heuristic)
            }
            // Trigger processing for last prefix in table
            apply_heuristic_fc[heuristic] (routing_entries_set, current_routing_entries_set, ases_interest)
            done <- struct{}{} // We're all done, unblock the channel

        }()
        
        // Actually start the bgpreader command
        if ! start_and_wait (cmd, done) {
            return
        }

        /* ----------------------- *\
               Post Processing
        \* ----------------------- */

        /* --- Save BGP peers to file --- */
        collector_peers_set.write_to_file (output_dir + "/collectors/BGP_peers_" + collector_name + ".txt")

        /* --- Overlay processing --- */
        overlays := process_overlays (routing_entries_set)
        overlays.write_to_file (output_dir + "/overlays/overlays_" + collector_name + ".txt")

        /* --- Save "forwarding table" --- */
        routing_entries_set.write_to_file (output_dir + "/forwarding_tables/" + collector_name + ".txt", print_rib_entry)

        /* --- Save next hop ASes --- */
        collector_dir := output_dir + "/next-hop_AS/" + collector_name
        cmd_s := "mkdir -p " + collector_dir
        exec.Command("bash", "-c", cmd_s).Run()
        output_file := collector_dir + "/next_hop_AS_" + collector_name + ".txt"
        routing_entries_set.write_to_file (output_file, print_next_as)

        /* --- Split file based on the AS of interest (Format of the file: prefix as_interest next_hop_as) --- */
        new_output_file := trim_suffix (output_file, ".txt") + "_"
        cmd_s = "awk -v var=" + new_output_file + " '{out=var$2\".txt\"; $2=\"\";print>out}' " + output_file
        err := exec.Command("bash", "-c", cmd_s).Run()
        if err != nil {
            panic ("[generate_RIB_parser]: Problem while splitting output file: " + err.Error ())
        }
    }
}

/**
 * Records a RIB entry in the current_routing_entries_set. Once all entries for a given prefix
 * have been read, trigger the BGP selection process according to provided heuristic.
 * Other information are also recorded for each valid prefix.
 */
func parse_bgp_record_multi(memory_set *SafeSet, record string, routing_entries_set, current_routing_entries_set, origin_set, collector_peers_set *SafeSet, ases_interest []string, prev_prefix, collector_name string, counter *int, heuristic int) string{
    defer recovery_function ()

    s := strings.Split(record, "|")
    if s[1] == "R" { // Only care about RIB content
        prefix := s[9]
        network, valid := check_prefix_validity (prefix)
        curr_prefix := ""
        if valid {
            curr_prefix = network.String ()
        }
        
        /* --- Trigger BGP decision process according to heuristic --- */
        if (curr_prefix == "") || ((prev_prefix != "") && (prev_prefix != curr_prefix)) {
            apply_heuristic_fc[heuristic] (routing_entries_set, current_routing_entries_set, ases_interest)
            *counter = 0
        } 

        /* --- Record current RIB entry if valid --- */
        if valid {
            if *counter == 0 { // First time encoutering prefix, record it
                if memory_set.unsafe_contains (curr_prefix) {
                    log.Println ("RIB ASSUMPTION VIOLATED!!!")
                }
                memory_set.unsafe_add (curr_prefix)
            }

            as_path := s[11]
            routing_entry := get_Rib_entry (as_path, ases_interest, 1)
            current_routing_entries_set.unsafe_add (curr_prefix + "_" + strconv.Itoa(*counter), routing_entry)
            (*counter)++

            // We record everything, irrespective of best path.
            /* --- Origin AS of prefix --- */
            origin_as := s[12]
            origin_set.append (origin_as, network.String ()) //Origin AS -> All prefixes announced by that AS

            /* --- BGP peer of collector --- */
            bgp_peer := s[7]
            collector_peers_set.unsafe_append (collector_name, bgp_peer) //Collector -> All its bgp peers
        }

        return curr_prefix
    }
    return "" // Will not happen as I use "-t ribs" in my bgpreader command.
}

/* ------------------------------------------------- *\
            BGP entries counter
\* ------------------------------------------------- */

/**
 * Generate a function to count BGP dump entries.
 * - set: where to store the results for all collectors
 */
func generate_dump_counter (set *SafeSet, start, end string) func (string) {

    return func (collector_name string) {
        /* --- Count prefixes --- */
        cmd := exec.Command("bgpreader", "-t", "ribs", "-c", collector_name, "-w", start+","+end)
        r, _ := cmd.StdoutPipe() // Get a pipe to read from standard output
        scanner := bufio.NewScanner(r) // Create a scanner which scans the output line-by-line

        // Channel for communication when the goroutine is done parsing the whole file
        done := make(chan struct{}) // An empty struct takes up no memory space

        /* ----------------------- *\
                RIB Processing
        \* ----------------------- */
        // Store all prefixes of a table (no duplicate)
        memory_set := create_safeset ()
        go func() {
            // Read line by line and process it
            for scanner.Scan() {
                line := scanner.Text()
                count_bgp_record (line, memory_set)
            }
            done <- struct{}{} // We're all done, unblock the channel

        }()
        
        // Actually start the bgpreader command
        if ! start_and_wait (cmd, done) {
            return
        }

        /* ----------------------- *\
               Post Processing
        \* ----------------------- */
        set.add (collector_name, len (memory_set.set))
    }
}

func count_bgp_record (record string, memory_set *SafeSet) {
    s := strings.Split(record, "|")
    prefix := s[9]
    network, valid := check_prefix_validity (prefix)
    if s[1] == "R" && valid { // Only care about RIB content
        memory_set.unsafe_add (network.String ())
    }
}

/* ------------------------------------------------- *\
             BGP dependent prefix parser
\* ------------------------------------------------- */

/**
 * Generate a function to parse BGP dump files
 * Closure to have all that we need, self-contained in the function (and not have to passe them as args) 
 * - set: where to store the results for all collectors
 * - ases: the ASes of interest
 * - collectors_to_index: a mapping between a collector and its assigned number.
 *
 * Outputs a file of prefixes for which an AS of interest was seen in the AS path, and indicate if this prefix is a
 * dependent prefix or an up/down prefix.
 * 
 * Note: The cmd.Start as well as a goroutine are used to process the output of the command concurrently, as RIB tables
 * can be quite long.
 */
func generate_RIB_parser_dependent (set *SafeSet, ases []string, collectors_to_index map[string]int, break_prefix bool, start, end string) func (string) {

    return func (collector_name string) {

        /* --- 'bgpreader' command, filtering on specific ASes in the AS path --- */
        cmd := exec.Command("bgpreader", "-t", "ribs", "-c", collector_name, "-w", start+","+end, "-A", generate_aspath_regex (ases))
        r, _ := cmd.StdoutPipe() // Get a pipe to read from standard output
        scanner := bufio.NewScanner(r) // Create a scanner which scans the output line-by-line

        // Channel for communication when the goroutine is done parsing the whole file
        done := make(chan struct{}) // An empty struct takes up no memory space

        /* ----------------------- *\
               RIB Processing
        \* ----------------------- */
        memory_set := create_safeset ()
        go func() {
            // Read line by line and process it
            for scanner.Scan() {
                line := scanner.Text()
                parse_bgp_record (line, set, memory_set, collectors_to_index[collector_name], break_prefix)
            }
            done <- struct{}{} // We're all done, unblock the channel

        }()
        
        // Actually start the bgpreader command
        start_and_wait (cmd, done)
    }
}

/**
 * Output format of 'bgpreader': <dump-type>|<elem-type>|<record-ts>|<project>|<collector>|<router-name>|<router-ip>|<peer-ASn>|<peer-IP>|<prefix>|<next-hop-IP>|<AS-path>|<origin-AS>|<communities>|<old-state>|<new-state>
 * - set: global set where all results are stored
 * - memory_set: set for a single collector to not redo previous operations
 * - collector_index: the number assigned to current collector
 */
func parse_bgp_record (record string, set *SafeSet, memory_set *SafeSet, collector_index int, break_prefix bool) {
    defer recovery_function ()

    s := strings.Split(record, "|")
    prefix := s[9]
    network, valid := check_prefix_validity (prefix)
    if s[1] == "R" && valid { // Only care about RIB content
        /* --- That prefix was already seen for current collector --- */
        if memory_set.unsafe_contains (network.String ()) {
            return
        }
        memory_set.unsafe_add (network.String ())

        /* --- Transform subnet into /24 subnets (or not, depending on prefix_length) ---*/
        var prefix_length int
        if break_prefix {
            prefix_length = 24
        } else {
            l,_ := network.Mask.Size ()
            prefix_length = l
        }
        subnets := get_subnets (network, prefix_length)
        for _, subnet := range subnets {
            add_to_set (set, subnet.String (), collector_index) 
            memory_set.unsafe_add (subnet.String ())
        }
    }
}

/**
 * Adds the subnet to the set, and/or updates the BGP collector for
 * which it was seen.
 */
func add_to_set (set *SafeSet, subnet string, index int) {
    c, ok := set.get (subnet)
    if ok {
        current_state, t := c.(uint64) // Type assertion
        if !t {
            log.Fatal ("[add_to_set]: type assertion failed")
        }
        set.add (subnet, current_state | uint64 (1<<index))
    } else {
        set.add (subnet, uint64 (1<<index))
    }
}

/* ------------------------------------------------- *\
            AS PATH and Tier1 analyser
\* ------------------------------------------------- */

/**
 * Generate a function to analyze the AS path of BGP dump entries.
 * - set: where to store the results for all collectors
 * - tier1: set containing all ASes that are Tiers 1 (0 providers).
 * 
 * First result: There are 20% of roads that contain at least two successive Tiers 1.
 */
func generate_RIB_as_path_analyser (set *SafeSet, tiers1 map[string]interface{}, start, end string) func (string) {

    return func (collector_name string) {

        cmd := exec.Command("bgpreader", "-t", "ribs", "-c", collector_name, "-w", start+","+end)
        r, _ := cmd.StdoutPipe() // Get a pipe to read from standard output
        scanner := bufio.NewScanner(r) // Create a scanner which scans the output line-by-line

        // Channel for communication when the goroutine is done parsing the whole file
        done := make(chan struct{}) // An empty struct takes up no memory space

        /* ----------------------- *\
                RIB Processing
        \* ----------------------- */
        nb_path := 0 // How many paths where the last hop was a Tier1
        nb_entries := 0 // How many path where the last two hops were Tiers1.
        go func() {
            // Read line by line and process it
            for scanner.Scan() {
                line := scanner.Text()
                r1,r2 := analyse_bgp_record (line, tiers1)
                if r1 != -1 {
                    nb_entries += r2
                    nb_path += r1
                }
            }
            done <- struct{}{} // We're all done, unblock the channel

        }()
        
        // Actually start the bgpreader command
        if ! start_and_wait (cmd, done) {
            return
        }

        /* ----------------------- *\
               Post Processing
        \* ----------------------- */

        set.unsafe_append (collector_name, strconv.Itoa (nb_path))
        set.unsafe_append (collector_name, strconv.Itoa (nb_entries))
    }
}

/**
 * Returns if the last hop was a Tier1 and if the last two hops were Tier1
 */
func analyse_bgp_record (record string, tiers1 map[string]interface{}) (last, before_last int) {
    last, before_last = -1,-1
    s := strings.Split(record, "|")
    prefix := s[9]
    _, valid := check_prefix_validity (prefix)
    if s[1] == "R" && valid { // Only care about RIB content
        /* --- Analyze AS path --- */
        as_path := s[11]
        ases := strings.Split (as_path, " ")
        last, before_last = analyse_aspath (ases, tiers1)
    }
    return
}

func analyse_aspath (as_path []string, tiers1 map[string]interface{}) (last, before_last int){
    last, before_last = -1,-1

    if len (as_path) >= 2 {
        last_as := as_path[len (as_path) - 1]
        before_last_as := as_path[len(as_path) - 2]
        last, before_last = 0,0
        if _, ok := tiers1[last_as]; ok { // The last hop is a Tier1.
            last = 1
            if _, ok2 := tiers1[before_last_as]; ok2 { // The before last hop is also a Tier1.
                before_last = 1;
            }
        }
    }
    return
}

/* --------------------------------------- *\
 *                  MISC.
\* --------------------------------------- */

/** 
 * From a file of prefixes, output a file of addresses
 */
func from_prefixes_to_addresses (filename, output_filename string) {
    new_set := create_safeset ()
    prefix_parser := generate_prefix_parser (new_set)
    pool.Launch_pool (32, filename, prefix_parser)
    log.Print ("Done parsing file")
    new_set.write_to_file (output_filename)
}

/**
 * Generate a function to parse prefixes and return a random IP address out of it.
 */
func generate_prefix_parser (set *SafeSet) func (string){
    return func (prefix string) {
        _, network, err := net.ParseCIDR (prefix)
        if err != nil {
            log.Print ("[parse_prefix]: " + err.Error())
            return
        }
        ip_address := get_random_ip (network).String () 
        set.add (ip_address)
    }
}

/**
 * Generates the argument to bgpreader
 * -k: only records containing that prefix
 * -A: only records whose AS path matches the regex
 */
func generate_args (regex string, prefixes *SafeSet) []string {
    args := make ([]string, 2 + 2*len (prefixes.set)) // *2 because of the letter -k. +2 because of the single arg for AS path
    i := 0
    for prefix, _ := range prefixes.set {
        args[i*2] = "-k"
        args[i*2+1] = prefix
        i++
    }
    args[i*2] = "-A"
    args[i*2+1] = regex
    return args
}

/**
 * Generate a regex that will match any AS path that contains one of the ASes contained in ases
 * The regex: (^|[^1-9]+)(701|3549)([^1-9]+|$)
 */
func generate_aspath_regex (ases []string) string {
    return "(^|[^0-9]+)(" + strings.Join(ases, "|") + ")([^0-9]+|$)"
}
