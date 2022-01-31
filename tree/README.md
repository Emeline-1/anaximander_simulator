# tree

This project implements a simple tree in the [Go Language](https://golang.org). 
The code is taken from [Tufin's repository](https://github.com/Tufin/asciitree), with some small modifications:

- For method Add, the argument `path` is a []string (instead of a string to be split on the '/' character)
- User-passed functions are called depending on whether a node is absent or present in the tree.

#### Example
```
import (radix "github.com/Emeline-1/tree")

func generate_if_present (w *bufio.Writer) func (string, interface{}) {
    return func (element string, arg interface{}) {
    	w.WriteString (element + " is present")
    }
}

func generate_if_absent (w *bufio.Writer) func (string, interface{}) {
    return func (element string, arg interface{}) {
        w.WriteString (element + " is absent")
    }
}

func main () {
    f, _ := os.Create(filename) 
    defer f.Close ()
    w := bufio.NewWriter(f)
    f_absent := generate_if_absent (w)
    f_present := generate_if_present (w)

    path_tree := &tree.Tree{}
    path_tree.Add (["1", "2", "3"], f_absent, f_present, nil)
    path_tree.Add (["1", "2", "4"], f_absent, f_present, nil)
}

```

In this example, `f_absent` will be called on elements "1", "2", and "3". Then `f_present` will be called on elements "1" and "2". And finally, `f_absent` will be called on element "4".