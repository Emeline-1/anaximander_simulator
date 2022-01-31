package main

import ("strings"
        "sort"
        "log"
        "regexp"
        "bufio"
        "os"
        "math"
        "strconv")

var (
    ip_string = `(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`
    net_string = `(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/\d{1,2}`
    re_source_dest = regexp.MustCompile (ip_string + `\s*to\s*` + ip_string)
    re_ip = regexp.MustCompile (ip_string)
    re_net = regexp.MustCompile (net_string)
    MaxInt = int(^uint(0) >> 1)
)

func recovery_function () {
    if r := recover(); r != nil {
        log.Println (r)
        return
    }
}

func recovery_function_fatal () {
    if r := recover(); r != nil {
        log.Fatal (r)
        return
    }
}

func reverse (s []string) {
    for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
        s[i], s[j] = s[j], s[i]
    }
}

func Map(s []string, f func(string) string) []string {
    new_s := make([]string, len(s))
    for i, v := range s {
        new_s[i] = f(v)
    }
    return new_s
}

func IntPow(n, m int) int {
    if m == 0 {
        return 1
    }
    result := n
    for i := 2; i <= m; i++ {
        result *= n
    }
    return result
}

func same (s []string) bool {
    ref := s[0]
    for _, string := range s[1:] {
        if ref != string {
            return false
        }
    }
    return true
}

func extract_mask_length (s string) int {
    v,e := strconv.Atoi (strings.Split (s, "/")[1])
    if e != nil {
        panic ("Problem while extracting mask length")
    }
    return v
}

func longestCommonPrefix (prefixes []string) string {
    if len (prefixes) == 0 {
        return ""
    }

    sort.Sort(ByLen(prefixes))

    lc := ""
    smallest := prefixes[0]

    for index:= 0; index < len (smallest); index++ {
        present := true

        for _,s := range prefixes[1:] {
            if s[index] != smallest[index] {
                present = false
                break
            }
        }

        if !present {
            break
        } else {
            lc += string(smallest[index])
        }
    }
    return lc
}

func longestPrefix(k1, k2 string) int {
    max := len(k1)
    if l := len(k2); l < max {
        max = l
    }
    var i int
    for i = 0; i < max; i++ {
        if k1[i] != k2[i] {
            break
        }
    }
    return i
}

func remove_duplicates (slice []string) []string {
    r := []string{}
    prev := ""
    for _,s := range slice {
        if s != prev {
            r = append (r, s)
        }
        prev = s
    }
    return r
}

func routing_loop (slice []string) bool {
    seen := make (map[string]struct{})
    for _, s := range slice {
        if _, present := seen[s]; present {
            return true
        }
        seen[s] = struct{}{}
    }
    return false
}

func find_index (slice []string, element string) int {
    for i, s := range slice {
        if s == element {
            return i
        }
    }
    return -1
}

type ByLen []string
 
func (a ByLen) Len() int {
   return len(a)
}
 
func (a ByLen) Less(i, j int) bool {
   return len(a[i]) < len(a[j])
}
 
func (a ByLen) Swap(i, j int) {
   a[i], a[j] = a[j], a[i]
}

func split_src_dst (x string) string {
    return strings.Split (x, "_")[1]
}

func get_keys (mymap *map[string]interface{}) []string {
    keys := make([]string, len(*mymap))
    i := 0
    for k := range (*mymap) {
        keys[i] = k
        i++
    }
    return keys
}

func slice_to_map (s []string) map[string]interface{} {
    m := make (map[string]interface{})
    for _, x := range s {
        m[x] = struct{}{}
    }
    return m
}

/**
 * Merges m1 and m2 into a new map
 */
func merge_maps_new (m1, m2 map[string]interface{}) map[string]interface{} {
    new_map := make (map[string]interface{})
    for x,_ := range m1 {
        new_map[x] = struct{}{}
    }
    for x,_ := range m2 {
        new_map[x] = struct{}{}
    }
    return new_map
}

/**
 * Merges m2 into m1
 */
func merge_maps (m1, m2 map[string]interface{}) map[string]interface{} {
    for x,_ := range m2 {
        m1[x] = struct{}{}
    }
    return m1
}

/**
 * Returns a slice with the elements of a that are not in b.
 */
func difference (a, b map[string]interface{}) []string {
    r := make ([]string,0, len (a))
    for x,_ := range a {
        if _,ok := b[x]; !ok {
            r = append (r, x)
        }
    }
    return r
}

/**
 * Returns the key associated to the extremum value, as well
 * as the number of times this extremum value appeard in the map.
 */
func search_map (m map[string]interface{}, operator func (interface{}, interface{})bool) (string, int) {
    var extremum_string string 
    var extremum interface{}
    /* --- gest first value --- */
    for extremum_string, extremum = range m {
        break
    }
    /* --- search for max or min value --- */
    for s,v := range m {
        if operator (v,extremum) {
            extremum_string = s
            extremum = v 
        }
    }
    /* --- Check it is the only max or min --- */
    nb := 0
    for _,v := range m {
        if v == extremum { // This doesnt always make sense depending on the type. For int ok, for slices no.
            nb++
        }
    }
    return extremum_string, nb
}

/**
 * If ASes of interest are found in the slice i, the 'nb_ases_interest' variable is set to the number of 
 * ASes found, if that number is superior to the current value of 'nb_ases_interest'.
 * The returned value here doesn't have much importance, only the 'nb_ases_interest' variable will be used.
 */
func generate_search_function (ases_interest []string, nb_ases_interest *int) func (interface{}, interface{}) bool {
    return func (i_i, j_i interface{}) bool {
        i := i_i.([]string)

        local_nb := 0
        for _, as_interest := range ases_interest {
            index_i := find_index (i, as_interest)
            if index_i != -1 {
                local_nb++
            }
                
        }
        if *nb_ases_interest < local_nb {
            *nb_ases_interest = local_nb
        }
        return false // doesn't matter
    }
}

func LessInt(i_i, j_i interface{}) bool {
    i := i_i.(int) // Panic if wrong cast.
    j := j_i.(int)
    return i < j
}

func MoreInt (i_i, j_i interface{}) bool {
    i := i_i.(int)
    j := j_i.(int)
    return i > j
}

func LessSlice (i_i, j_i interface{}) bool {
    i := i_i.([]string)
    j := j_i.([]string)
    return len(i) < len(j)
}

func MoreSlice (i_i, j_i interface{}) bool {
    i := i_i.([]string)
    j := j_i.([]string)
    return len(i) > len(j)
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

func (data DataFloat64) Sum () float64 {
    s := 0.0
    for _, d := range data {
        s += d
    }
    return s
}

type DataFloat64 []float64

func (data DataFloat64) Mean () float64 {
    return data.Sum () / float64 (len (data))
}

func (data DataFloat64) Variance () float64 {
    m := data.Mean ()

    vars := make (DataFloat64, 0, len (data))
    for _, f := range data {
        vars = append (vars, math.Abs (f - m))
    }
    return vars.Mean ()
}

func stringSlice_to_floatSlice (a []string) (r []float64) {
    r = make ([]float64,0,len (a))
    for _, e := range a {
        n,_ := strconv.ParseFloat (e,64)
        r = append (r, n)
    }
    return
}

func new_bufio_writer (output_file string) (*bufio.Writer, *os.File) {
    file, err := os.Create(output_file)
    if err != nil {
      log.Fatal(err)
    }
    return bufio.NewWriter(file), file
}

func trim_suffix (file, suffix string) string {
    if strings.HasSuffix(file, suffix) {
        file = file[:len(file)-len(suffix)]
    }
    return file
}