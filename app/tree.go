package app

import (
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// Roots of the two volumes a unit has.
const (
	// CardRoot is the SD card, where IMPORT and EXPORT live.
	CardRoot = fileapi.Card
	// StorageRoot is the unit's own storage, where projects live.
	StorageRoot = ""
)

// Node is one visible row of a Tree.
type Node struct {
	Path     string
	Name     string
	Dir      bool
	Depth    int
	Expanded bool
}

// Tree is a folder tree with folders that open and close, kept as the list of rows
// currently visible.
type Tree struct {
	Root  string
	nodes []Node
}

// NewTree starts a tree at root, showing its entries.
func NewTree(root string, entries []fileapi.DirEntry) *Tree {
	return &Tree{Root: root, nodes: children(root, 0, entries)}
}

// Rows are the visible rows, top to bottom.
func (t *Tree) Rows() []Node {
	return t.nodes
}

// Expand opens the folder at row i, showing entries under it.
func (t *Tree) Expand(i int, entries []fileapi.DirEntry) {
	if i < 0 || i >= len(t.nodes) || !t.nodes[i].Dir || t.nodes[i].Expanded {
		return
	}
	t.nodes[i].Expanded = true
	inside := children(t.nodes[i].Path, t.nodes[i].Depth+1, entries)
	rest := append(inside, t.nodes[i+1:]...)
	t.nodes = append(t.nodes[:i+1], rest...)
}

// Collapse closes the folder at row i, hiding everything under it.
func (t *Tree) Collapse(i int) {
	if i < 0 || i >= len(t.nodes) || !t.nodes[i].Expanded {
		return
	}
	end := i + 1
	for end < len(t.nodes) && t.nodes[end].Depth > t.nodes[i].Depth {
		end++
	}
	t.nodes[i].Expanded = false
	t.nodes = append(t.nodes[:i+1], t.nodes[end:]...)
}

// CollapseAll closes every folder, leaving the top level. It returns the row to put a cursor
// on that was on row i: the outermost folder holding it, or the row itself when it was on the
// top level.
func (t *Tree) CollapseAll(i int) int {
	top := i
	for top >= 0 && top < len(t.nodes) && t.nodes[top].Depth > 0 {
		top = t.Parent(top)
	}
	if top < 0 {
		top = 0
	}
	kept := t.nodes[:0]
	moved := 0
	for j, n := range t.nodes {
		if n.Depth > 0 {
			continue
		}
		if j == top {
			moved = len(kept)
		}
		n.Expanded = false
		kept = append(kept, n)
	}
	t.nodes = kept
	return moved
}

// Parent is the row of the folder that holds row i, or -1 at the top level.
func (t *Tree) Parent(i int) int {
	if i < 0 || i >= len(t.nodes) {
		return -1
	}
	for p := i - 1; p >= 0; p-- {
		if t.nodes[p].Depth < t.nodes[i].Depth {
			return p
		}
	}
	return -1
}

// Folder is the folder row i stands for: itself if it is one, otherwise the one holding it.
func (t *Tree) Folder(i int) string {
	if i < 0 || i >= len(t.nodes) {
		return t.Root
	}
	if t.nodes[i].Dir {
		return t.nodes[i].Path
	}
	return Dir(t.nodes[i].Path)
}

// children turns a folder's entries into rows: folders first, then by name.
func children(dir string, depth int, entries []fileapi.DirEntry) []Node {
	nodes := make([]Node, 0, len(entries))
	for _, e := range entries {
		nodes = append(nodes, Node{Path: Join(dir, e.Name), Name: e.Name, Dir: e.IsDir(), Depth: depth})
	}
	sort.SliceStable(nodes, func(a, b int) bool {
		if nodes[a].Dir != nodes[b].Dir {
			return nodes[a].Dir
		}
		return NaturalLess(strings.ToLower(nodes[a].Name), strings.ToLower(nodes[b].Name))
	})
	return nodes
}

// Join adds a name to a folder path. The roots take no separator: "SD:" + "IMPORT" is
// "SD:IMPORT".
func Join(dir, name string) string {
	dir = slash(dir)
	if dir == StorageRoot || dir == CardRoot || dir == "/" {
		return dir + name
	}
	return dir + "/" + name
}

// Dir is the folder holding path, down to the root of its volume.
func Dir(path string) string {
	path = slash(path)
	switch at := strings.LastIndexByte(path, '/'); {
	case at == 0:
		return "/"
	case at > 0:
		return path[:at]
	case strings.HasPrefix(path, CardRoot):
		return CardRoot
	}
	return StorageRoot
}

// Base is the last element of path.
func Base(path string) string {
	path = slash(path)
	if at := strings.LastIndexByte(path, '/'); at >= 0 {
		return path[at+1:]
	}
	return strings.TrimPrefix(path, CardRoot)
}

// MoveInto moves a file or folder into dir, keeping its name, and returns the new path.
func MoveInto(f Storage, path, dir string) (string, error) {
	to := Join(dir, Base(path))
	return to, f.Rename(path, to)
}

// RenamePath gives a file or folder a new name in the same folder, and returns the new path.
func RenamePath(f Storage, path, name string) (string, error) {
	to := Join(Dir(path), name)
	return to, f.Rename(path, to)
}

// Expanded are the paths of the open folders, outermost first.
func (t *Tree) Expanded() []string {
	var paths []string
	for _, n := range t.nodes {
		if n.Expanded {
			paths = append(paths, n.Path)
		}
	}
	return paths
}

// Index is the row showing path, or -1.
func (t *Tree) Index(path string) int {
	for i, n := range t.nodes {
		if n.Path == path {
			return i
		}
	}
	return -1
}

// LoadTree lists root and opens the folders in expanded again (outermost first), leaving
// out any that are gone: a tree as it was before the files under it changed.
func LoadTree(f Storage, root string, expanded []string) (*Tree, error) {
	entries, err := f.ListDir(root)
	if err != nil {
		return nil, err
	}
	tree := NewTree(root, entries)
	for _, path := range expanded {
		row := tree.Index(path)
		if row < 0 {
			continue
		}
		if inside, err := f.ListDir(path); err == nil {
			tree.Expand(row, inside)
		}
	}
	return tree, nil
}

// Siblings are the rows of the folder level row i is on: every row with the same parent and
// the same depth, top to bottom, leaving out what is inside the folders that are open.
func (t *Tree) Siblings(i int) []int {
	if i < 0 || i >= len(t.nodes) {
		return nil
	}
	depth := t.nodes[i].Depth
	start := t.Parent(i) + 1
	var out []int
	for j := start; j < len(t.nodes) && t.nodes[j].Depth >= depth; j++ {
		if t.nodes[j].Depth == depth {
			out = append(out, j)
		}
	}
	return out
}

// Children are the rows directly inside the open folder at row i.
func (t *Tree) Children(i int) []int {
	if i < 0 || i >= len(t.nodes) || !t.nodes[i].Expanded {
		return nil
	}
	var out []int
	for j := i + 1; j < len(t.nodes) && t.nodes[j].Depth > t.nodes[i].Depth; j++ {
		if t.nodes[j].Depth == t.nodes[i].Depth+1 {
			out = append(out, j)
		}
	}
	return out
}
