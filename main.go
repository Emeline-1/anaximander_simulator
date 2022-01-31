package main

import (
    "log"
    "os"
    "path"
    "os/exec"
) 

// Global structure holding all necessary data files.
type Args struct{
    /* simulation-data */
    as_rel_file string; 
    ppdc_file string; 
    ip2as_file string; 
    bdrmapit_file string;
    warts_directory string;
    /* ribs-data */
    directed_prefixes_dir string; 
    oracle_prefixes_dir string; 
    overlays_global_file string; 
    nexthop_as_dir_global string;
    /* AS specifics */
    vps_file string; 
    collectors_file string; 
    ases_interest_file string;
    /* simulation-parameters */
    threshold_parameter float64; 
    weight_parameters []float64; 
    /* Strategy */
    strategy string; 
}

var ( // Global Parameters
    g_args Args
)

var ( // Output mode
    output_on bool = true;
    succesfull_traces_on bool = false;
)

func output_mode () {
    o, _ := os.Stdout.Stat()
    if (o.Mode() & os.ModeCharDevice) == os.ModeCharDevice { //Terminal
        log.Fatal ("\n /!\\ Please redirect output to a file to get some statistics on Anaximander's run /!\\ \n")
    } else { //It is not the terminal
        // Display info to a pipe
    }
}

func usage () {
    println ("\nUsage of Anaximander:\n")
    println ("Anaximander has several modes:")
    println ("  - rib_parsing: to parse RIBs and collect all necessary information for either the strategy or the simulation.")
    println ("  - strategy: to output the ordered list of targets built by Anaximander.")
    println ("  - simulation: to simulate Anaximander on a warts dataset.\n")
    println ("Type")
    println ("  ./anaximander [mode] -h")
    println ("for further information on each mode.\n")
}

func main () {
    log.SetFlags(0)
    if len (os.Args) == 1 {
        usage ()
        return
    }
    switch command := os.Args[1]; command {

        /* --------------------------- *\
                  RIB PARSING
        \* --------------------------- */
        case "rib_parsing":
            launch_rib_parsing (os.Args[2:])

        /* --------------------------- *\
            Anaximander Strategy Step
        \* --------------------------- */
        case "strategy":
            break_prefix, strategy, output_dir := handle_args_strategy (os.Args[1:])
            output_mode () // Check redirection
            launch_anaximander_strategy (break_prefix, strategy, output_dir)
            // To split the information into different files based on the first column value.
            // awk '{outfile=$1; $1=""; print>outfile}' output.txt
            exec.Command("bash", "-c", "cd " + output_dir + " && awk '{outfile=$1; $1=\"\"; print>outfile}' output.txt").Run()
        /* --------------------------- *\
              Anaximander Simulator
        \* --------------------------- */
        case "simulation":
            break_prefix, output_file, simulation_mode := handle_args_simulation (os.Args[1:])
            output_mode () // Check redirection
            launch_anaximander_simulation (break_prefix, output_file, simulation_mode)
            dir := path.Dir(output_file)
            exec.Command("bash", "-c", "cd " + dir + " && awk '{outfile=$1; $1=\"\"; print>outfile}' output.txt").Run()
            
        /* --------------------------- *\
              Rocketfuel Simulator
        \* --------------------------- */
        /* --- Partial simulation of Rocketfuel Path Reduction techniques. --- */
        case "rocketfuel_simulation":
            rocketfuel_simulation (os.Args[2:])

        /* --------------------------- *\
                      Misc.
        \* --------------------------- */
        /* --- Various analysis and processing of the data. --- */
        case "analysis":
            analysis (os.Args[2:])
        case "-h":
            usage ()
        case "--help":
            usage ()
        default:
            log.Println("Unknown command:", command)
            log.Println("Type './anaximander -h' for help:")
    }
}

// --------------------------------------------------------------------------------
func launch_rib_parsing (args []string) {
    usage_rib_parsing_f := func () {
        println ("Usage of rib_parsing:")
        println ("")
        println ("  ./anaximader rib_parsing count: Step1 - for each collector, count the number of entries, in order to determine which collectors are sound (nb entries > 800k)")
        println ("  ./anaximader rib_parsing ribs_multi: Step2 - parse RIBs from all (sound) collectors and outputs several information from them.")
        println ("  ./anaximader rib_parsing build_best_directed_probes: Step3 - build the BDP from the parsing of the RIBs")
        println ("\nType")
        println ("  ./anaximander rib_parsing [sub_mode] -h")
        println ("for further information on each sub mode.\n")
    }

    if len (args) == 0 {
        usage_rib_parsing_f ()
        return
    }
    switch command := args[0]; command {
        /**
         * Step1: For each collector, count the number of entries, in order to determine which collectors are sound (nb entries > 800k)
         */
        case "count":
            count_ribs (handle_args_rib_parsing_count (args))
        /**
         * Step2: Parse RIBs from all (valid) collectors and outputs several information from them.
         *
         * To get a single RIB at a given time, specify the time interval for which you want to retrieve the table.
         * Route Views collectors output a RIB every 2 hours whereas RIPE RIS collectors output a RIB every 8 hours
         * (both aligned to midnight).
         * As RIB dumps are not made atomically, you should specify a window of a few minutes ((e.g., 00:00 -> 00:05)
         *  - Cycle 141
         *   start=1601856000
         end=  1601856300 
         *  - Cycle 176
         *   start=1618876800
         *   end=  1618877100 
             */
        case "ribs_multi":
            parse_ribs (handle_args_rib_parsing_multi (args))
        /**
         * Step3: Build the BDP.
         */
        case "build_best_directed_probes": 
            build_best_path_directed_probes (handle_args_rib_parsing_build (args))

        /* --------------------------- *\
                      Misc.
        \* --------------------------- */
        case "analyse_rib":
            analyse_ribs (handle_args_rib_parsing_analyser (args))
        case "analyse_fib":
            analyse_fibs (handle_args_fib_parsing_analyser (args))
        case "-h":
            usage_rib_parsing_f ()
        default:
            log.Println ("Unknown sub-command:", command)
    }
}

// --------------------------------------------------------------------------------
func rocketfuel_simulation (args []string) {
    if len (args) == 0 {
        println ("Missing arguments")
        return
    }
    switch command := args[0]; command {
        /**
         * Ingress Reduction
         */
        case "ingress_reduction": // ./anaximander read <ases_file> <sqlite_file> <warts_directory> <output_dir>
        g_args.bdrmapit_file, g_args.warts_directory = args[2], args[3]
            ingress_reduction (args[1], args[4])
        /**
         * Next-AS Reduction
         */
        case "nextAS": // ./anaximander analyse_next_hops (outdir, ases_file, collectors_file, dir string) //the directory where next-AS are found
            analyse_next_hops (args[1], args[2], args[3], args[4])
        case "merge_nextAS": // ./anaximander merge_nextAS (outdir, ases_file, collectors_file, dir string) //the directory where next-AS are found
            merge_next_hops (args[1], args[2], args[3], args[4])
        /**
         * Directed probing and Egress reduction
         * Parse RIBs from all (valid) collectors looking for a particular AS in the AS path.
         * Output all prefixes for which the AS was seen in the AS path, with an annotation of dependent or up/down prefixes 
         * (see RocketFuel paper)
         */
        case "directed_prefixes": // (as, collectors_file, output_filename string, break_prefix bool, start, end string)
            parse_ribs_dependent (handle_args_rib_parsing_ribs (args))
        default:
            log.Println ("Unknown sub-command:", command)
    }
}

// --------------------------------------------------------------------------------
func analysis (args []string) {
    if len (args) == 0 {
        println ("Missing arguments")
        return
    }
    switch command := args[0]; command {
        /* ---------------------- *\
            Overlays processing
        \* ---------------------- */
        case "overlays":
            analyse_overlays (args[1:])
        case "analyse_merged_overlays": // ./anaximander analyse_merged_overlays merged_overlays all_forwarding_tables
            analyse_merged_overlays (args[1], args[2:])
        case "overlays_repartition_vp": // ./anaximander overlays_repartition_vp overlay_file forwarding_table
            analyse_overlays_repartition_vp (args[1], args[2])
        case "merge_overlays": // ./anaximander dir
            build_merge_overlays (args[1])
        case "build_overlays_per_AS": // ./anaximander ases_file, all_overlays_file, directed_prefixes_dir, outdir string
            build_overlays_per_AS (args[1], args[2], args[3], args[4])
        default:
            log.Println ("Unknown sub-command:", command)
    }
}
