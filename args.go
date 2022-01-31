/* ==================================================================================== *\
    args.go

    Program arguments handling
\* ==================================================================================== */

package main

import (
  "flag"
  "strings"
  "os"
) 

/* --------------------------------------- *\
 *          RIB PARSING
\* --------------------------------------- */

/** 
 * Handle the args for the Anaximander RIB parsing (count mode).
 */
func handle_args_rib_parsing_count (args []string) (_outputfile, _start, _end string) {
  if len (args) <= 0 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.StringVar(&_outputfile, "o", "", "The output file")
  cmd.StringVar(&_start, "s", "", "The timestamp for the start of the interval at which to retrieve the BGP tables")
  cmd.StringVar(&_end, "e", "", "The timestamp for the end of the interval at which to retrieve the BGP tables")

  cmd.Parse(args[1:])
  return
}

/** 
 * Handle the args for the Anaximander RIB parsing (multi mode).
 */
func handle_args_rib_parsing_multi (args []string) (_ases, _collectors, _outputdir, _start, _end string, _heuristic int) {
  if len (args) <= 0 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.StringVar(&_ases, "a", "", "The file containing the ASes of interest (one line, space separated)")
  cmd.StringVar(&_collectors, "c", "", "The file containing the BGP collectors")
  cmd.StringVar(&_outputdir, "o", "", "The output directory where to store results")
  cmd.StringVar(&_start, "s", "", "The timestamp for the start of the interval at which to retrieve the BGP table")
  cmd.StringVar(&_end, "e", "", "The timestamp for the end of the interval at which to retrieve the BGP table")

  cmd.IntVar(&_heuristic, "h", 1, "The BGP decision process heuristic to apply")
  cmd.StringVar(&g_args.as_rel_file, "asrel", "", "CAIDA file containing the relationships between ASes")

  cmd.Parse(args[1:])
  return
}

/** 
 * Handle the args for building the BDP.
 */
func handle_args_rib_parsing_build (args []string) (_outputdir, _ases, _collectors, _datadir string) {
  if len (args) <= 0 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.StringVar(&_ases, "a", "", "The file containing the ASes of interest (one line, space separated)")
  cmd.StringVar(&_collectors, "c", "", "The file containing the BGP collectors")
  cmd.StringVar(&_outputdir, "o", "", "The output directory where to store results")
  cmd.StringVar(&_datadir, "d", "", "The directory where to find the necessary information for building the BDP (output directory of previous step)")

  cmd.Parse(args[1:])
  return
}

/* --- MISC. ---*/

func handle_args_rib_parsing_ribs (args []string) (_ases, _collectors, _outputfile string, _break_prefix bool, _start, _end string) {
  if len (args) <= 0 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.StringVar(&_ases, "a", "", "The AS of interest")
  cmd.StringVar(&_collectors, "c", "", "The file containing the BGP collectors")
  cmd.StringVar(&_outputfile, "o", "", "The output file")
  cmd.BoolVar (&_break_prefix, "b", false, "Whether to break RIB's prefixes into /24 or not")
  cmd.StringVar(&_start, "s", "", "The timestamp for the start of the interval at which to retrieve the BGP table")
  cmd.StringVar(&_end, "e", "", "The timestamp for the end of the interval at which to retrieve the BGP table")
  cmd.Parse(args[1:])
  return
}

func handle_args_rib_parsing_analyser (args []string) (_outputfile, _collectors_file, _relfile, _start, _end string) {
  if len (args) <= 0 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.StringVar(&_outputfile, "o", "", "The output file")
  cmd.StringVar(&_start, "s", "", "The timestamp for the start of the interval at which to retrieve the BGP table")
  cmd.StringVar(&_end, "e", "", "The timestamp for the end of the interval at which to retrieve the BGP table")
  cmd.StringVar(&_collectors_file, "c", "", "The file containing all collectors")
  cmd.StringVar(&_relfile, "r", "", "The file containing all ASes relationships")

  cmd.Parse(args[1:])
  return
}

func handle_args_fib_parsing_analyser (args []string) (_datadir, _collectors_file, _relfile, _outputfile string) {
  if len (args) <= 0 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.StringVar(&_outputfile, "o", "", "The output file")
  cmd.StringVar(&_collectors_file, "c", "", "The file containing all collectors")
  cmd.StringVar(&_relfile, "r", "", "The file containing all ASes relationships")
  cmd.StringVar(&_datadir, "d", "", "The data dir")
  cmd.Parse(args[1:])
  return
}

/* --------------------------------------- *\
 *          ANAXIMANDER STRATEGY
\* --------------------------------------- */

func handle_args_strategy (args []string) (break_prefix bool, strategy int, output_dir string) {
  //output_on = false
  if len (args) <= 1 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  cmd.IntVar(&strategy, "s", -1, "The probing strategy")
  cmd.BoolVar (&break_prefix, "break", false, "Whether to break RIB's prefixes into /24 or not")

  cmd.StringVar(&g_args.ases_interest_file, "ases", "", "The file containing the ASes of interest (one line, space separated)")
  cmd.StringVar(&g_args.as_rel_file, "asrel", "", "CAIDA file containing the relationships between ASes")
  cmd.StringVar(&g_args.ppdc_file, "ppdc", "", "CAIDA file containing the customer cones of ASes")
  cmd.StringVar(&g_args.ip2as_file, "ip2as", "", "Output of ip2as.py CAIDA script.")
  cmd.StringVar(&g_args.directed_prefixes_dir, "dp_dir", "", "The directory containing the directed prefixes (output of rib_parsing)")
  cmd.StringVar(&g_args.overlays_global_file, "overlays_file", "", "The file containing all merged overlays (output of rib_parsing)")
  cmd.StringVar(&output_dir, "o", "", "The output directory where to write the list of targets and the delimitations between ASes")
  
  /* Apply the strategy to a given warts data set (not mandatory) */
  cmd.StringVar(&g_args.bdrmapit_file, "bdr", "", "bdrmapit annotation file")
  cmd.StringVar(&g_args.warts_directory, "warts", "", "The directory containing the warts")
  cmd.StringVar (&g_args.vps_file, "vps", "", "The file containing all VPs and their characteristics")

  cmd.Parse(args[1:])
  return
}

/* --------------------------------------- *\
 *          ANAXIMANDER SIMULATION
\* --------------------------------------- */

func handle_args_simulation (args []string) (break_prefix bool, output_file string, simulation_mode int){
  if len (args) <= 1 {
    println ("Missing arguments")
    os.Exit (-1)
  }
  cmd := flag.NewFlagSet(args[0], flag.ExitOnError)

  /* --- Simulation data --- */
  cmd.StringVar(&g_args.ases_interest_file, "ases", "", "The file containing the ASes of interest (one line, space separated)")
  cmd.StringVar(&g_args.bdrmapit_file, "bdr", "", "The output of bdrmapit")
  cmd.StringVar(&g_args.warts_directory, "warts", "", "The directory containing the warts")
    
  /* --- Simulation parameters --- */
  cmd.StringVar (&g_args.strategy, "strategy", "", "The directory where to find the targets and the AS delimitations for each AS of interest")
  cmd.StringVar(&output_file, "o", "", "Output file")
  cmd.Float64Var(&g_args.threshold_parameter, "t", 1, "The threshold (tau) to apply")
  
  /* --- Other simulations mode --- */
  cmd.BoolVar (&break_prefix, "break", false, "Whether to break RIB's prefixes into /24 or not")
  cmd.BoolVar (&succesfull_traces_on, "", false, "True to record succesfull traces, False to not record them. (use form -flag=x for boolean flags)")
  cmd.IntVar (&simulation_mode, "m", 0, "The simulation mode (sequential, parallel, or greedy)")
  cmd.StringVar(&g_args.as_rel_file, "asrel", "", "CAIDA file containing the relationships between ASes")
  cmd.StringVar(&g_args.ppdc_file, "ppdc", "", "CAIDA file containing the customer cones of ASes")
  cmd.StringVar(&g_args.ip2as_file, "ip2as", "", "Output of ip2as.py CAIDA script.")
  var w_string string
  cmd.StringVar (&w_string, "w", "", "The weighting function to use and its parameters. Ex: -w 1-0.1-0.2 is to use function 1 with parameters 0.1 and 0.2")
  
  cmd.Parse(args[1:])
  g_args.weight_parameters = stringSlice_to_floatSlice (strings.Split (w_string, "-"))
  
  return
}