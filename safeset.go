package main

import (
    "log"
    "sync"
    "strings"
    "strconv"
    "bufio"
    "os")

/* --- Note on variable creation: ---
 * The default zero value of a struct has all its fields zeroed. 
 *      So when you declara a variable of type struct, the struct has been allocated, and all its fields are zeroed (0 for int, nil for a map, etc)
 * Two ways to create and initialize a new struct:
 * - The 'new' keyword can be used to create a new struct. It returns a pointer to the newly created struct.
 * - With a struct literal
 */

/**
 * A set that is protetcted by a sync.Mutex
 * Implementation using a map
 */
type SafeSet struct {
    mux sync.Mutex
    //set map[string]struct{} // struct{} takes no memory space
    set map[string]interface{}
    fake interface{} // If set, the Safeset will always return 'fake' for every query.
}

func create_safeset () *SafeSet {
    new_set := new (SafeSet) // Returns a pointer to the newly allocated struct
    new_set.set = make (map[string]interface{})
    return new_set
}

func (set *SafeSet) fake_it (fake interface{}) {
    set.fake = fake
}

func (set *SafeSet) add (key string, arg ...interface{}) {
    set.mux.Lock ()
    switch len (arg) {
        case 0: set.set[key] = struct{}{}
        case 1: set.set[key] = arg[0]
        default: log.Fatal ("Wrong number of arguments to function [add]")
    }
    set.mux.Unlock ()
}

func (set *SafeSet) unsafe_add (key string, arg ...interface{}) {
    switch len (arg) {
        case 0: set.set[key] = struct{}{}
        case 1: set.set[key] = arg[0]
        default: log.Fatal ("Wrong number of arguments to function [unsafe_add]'")
    }
}

func (set *SafeSet) unsafe_append (key, value string) {
    set._append_to_set (key, value)
}

func (set *SafeSet) append (key, value string) {
    set.mux.Lock ()
    set._append_to_set (key, value)
    set.mux.Unlock ()
}

func (set *SafeSet) _append_to_set (key, value string) {
    p, ok := set.unsafe_get (key)
    if ok {
        peers, t := p.(map[string]struct{}) // Type assertion
        if !t {
            log.Fatal ("[append_to_set: type assertion failed")
        }
        peers[value] = struct{}{}
        set.unsafe_add (key, peers)
    } else {
        set.unsafe_add (key, map[string]struct{}{value: struct{}{}})
    }
}

func (set *SafeSet) contains (key string) bool {
    if set.fake != nil {
        return true
    }
    set.mux.Lock ()
    _, present := set.set[key]
    set.mux.Unlock ()
    return present
}

func (set *SafeSet) unsafe_contains (key string) bool {
    if set.fake != nil {
        return true
    }
    _, present := set.set[key]
    return present
}

func (set *SafeSet) get (key string) (v interface{}, ok bool) {
    if set.fake != nil {
        v, ok = set.fake, true
    } else {
        set.mux.Lock ()
        v, ok = set.set[key]
        set.mux.Unlock ()
    }
    return
}

func (set *SafeSet) unsafe_get (key string) (v interface{}, ok bool) {
    if set.fake != nil {
        v, ok = set.fake, true
    } else {
        v, ok = set.set[key]
    }
    return 
}

func (set *SafeSet) unsafe_get_keys () []string {
    return get_keys (&(set.set))
}

func (set *SafeSet) String () string {
    var str strings.Builder
    str.WriteString ("\n")
    set.mux.Lock ()
    for key, s := range set.set {
        switch v := s.(type) {
            case struct{}:
                str.WriteString(key+"\n")
            case int:
                str.WriteString(key + " " + strconv.Itoa (v) + "\n")
            case string:
                str.WriteString(key + " " + v + "\n")
            case map[string]struct{}:
                str.WriteString(key + " " + strings.Join (_get_keys (&v), " ") + "\n")
            case []string:
                str.WriteString(key + " " + strings.Join (v, " ") + "\n")
            default:
                log.Fatal ("No custom print function defined for type: %T\n", v)
                
        }
    }
    set.mux.Unlock ()
    return str.String ()
}

type PrintFn func(w *bufio.Writer, key string, v interface{}) error

func (set *SafeSet) write_to_file (filename string, printfn ...PrintFn) {
    f, err := os.Create(filename) // If the file already exists, it is truncated
    if err != nil {
        log.Print ("[write_to_file]: " + err.Error())
        return
    }
    defer f.Close ()

    w := bufio.NewWriter(f)
    for key, s := range set.set {
        /* custom print function */
        if len (printfn) != 0 {
            err = printfn[0] (w, key, s)
        /* generic print function */
        } else {
            switch v := s.(type) {
                case struct{}:
                    _, err = w.WriteString(key+"\n")
                case int:
                    _, err = w.WriteString(key + " " + strconv.Itoa (v) + "\n")
                case string:
                    _, err = w.WriteString(key + " " + v + "\n")
                case map[string]struct{}:
                    _, err = w.WriteString(key + " " + strings.Join (_get_keys (&v), " ") + "\n")
                case []string:
                    _, err = w.WriteString(key + " " + strings.Join (v, " ") + "\n")
                default:
                    if len (printfn) == 0 {
                        log.Fatal ("No custom print function defined for type: %T\n", v)
                    }
                    err = printfn[0] (w, key, s)
            }
        }
        if err != nil {
            log.Print ("[write_to_file]: " + err.Error())
            return
        }
    }

    w.Flush()
}

func _get_keys (mymap *map[string]struct{}) []string {
    keys := make([]string, len(*mymap))
    i := 0
    for k := range (*mymap) {
        keys[i] = k
        i++
    }
    return keys
}
