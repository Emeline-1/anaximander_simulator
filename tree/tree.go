package tree

import (
	"fmt"
	"io"
)

// Code taken from https://github.com/Tufin/asciitree, with just a some small modifications:
// - For method Add, path is a []string (instead of a string to be split on the '/' character)
// - User-passed functions are called depending on whether a node is absent or present in the tree.

// Tree can be any map with:
// 1. Key that has method 'String() string'
// 2. Value is Tree itself
// You can replace this with your own tree
type Tree map[string]Tree

/**
 * Adds paths to the tree, and call if_absent on current element if
 * it is not present in the current path.
 */
func (tree Tree) Add(path []string, if_absent, if_present func (string, interface{}), arg interface{}) {
	if len(path) == 0 {
		return
	}

	nextTree, ok := tree[path[0]]
	if !ok {
		nextTree = Tree{}
		tree[path[0]] = nextTree
		if_absent (path[0], arg)
	} else {
		if_present (path[0], arg)
	}
	nextTree.Add(path[1:], if_absent, if_present, arg)
}

func (tree Tree) Fprint(w io.Writer, root bool, padding string) {
	if tree == nil {
		return
	}

	index := 0
	for k, v := range tree {
		fmt.Fprintf(w, "%s%s\n", padding+getPadding(root, getBoxType(index, len(tree))), k)
		v.Fprint(w, false, padding+getPadding(root, getBoxTypeExternal(index, len(tree))))
		index++
	}
}

type BoxType int

const (
	Regular BoxType = iota
	Last
	AfterLast
	Between
)

func (boxType BoxType) String() string {
	switch boxType {
	case Regular:
		return "\u251c" // ├
	case Last:
		return "\u2514" // └
	case AfterLast:
		return " "
	case Between:
		return "\u2502" // │
	default:
		panic("invalid box type")
	}
}

func getBoxType(index int, len int) BoxType {
	if index+1 == len {
		return Last
	} else if index+1 > len {
		return AfterLast
	}
	return Regular
}

func getBoxTypeExternal(index int, len int) BoxType {
	if index+1 == len {
		return AfterLast
	}
	return Between
}

func getPadding(root bool, boxType BoxType) string {
	if root {
		return ""
	}

	return boxType.String() + " "
}