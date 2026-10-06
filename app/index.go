package app

import (
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// IndexLimits bound how much of a source is walked, so a huge folder cannot hang the search.
type IndexLimits struct {
	MaxFolders int
	MaxFiles   int
	MaxDepth   int
}

// DefaultLimits suit an SD card and a home folder.
var DefaultLimits = IndexLimits{MaxFolders: 4000, MaxFiles: 60000, MaxDepth: 10}

// skipFolders are never walked: they hold nothing to put on a pad and a lot of files.
var skipFolders = map[string]bool{
	"Library": true, "node_modules": true, "$RECYCLE.BIN": true, "System Volume Information": true,
}

// IndexedFile is an audio file in an Index.
type IndexedFile struct {
	// Path is the file's path in the storage; Rel the same relative to the index's root.
	Path, Rel string
	lower     string
}

// Name is the file's own name.
func (f IndexedFile) Name() string { return Base(f.Path) }

// Index lists every audio file under a root, built once and searched as you type.
type Index struct {
	Root  string
	Files []IndexedFile
	// Truncated is set when a limit stopped the walk early.
	Truncated bool
	Folders   int
}

// BuildIndex walks root breadth first and lists its audio files. A folder that cannot be
// read is skipped.
func BuildIndex(s Storage, root string, lim IndexLimits) (*Index, error) {
	ix := &Index{Root: root}
	type item struct {
		path  string
		depth int
	}
	queue := []item{{root, 0}}
	first := true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if ix.Folders >= lim.MaxFolders {
			ix.Truncated = true
			break
		}
		entries, err := s.ListDir(cur.path)
		if err != nil {
			if first {
				return nil, err
			}
			continue
		}
		first = false
		ix.Folders++
		for _, e := range entries {
			path := Join(cur.path, e.Name)
			switch {
			case e.IsDir():
				if cur.depth+1 > lim.MaxDepth {
					ix.Truncated = true
				} else if !skipFolders[e.Name] {
					queue = append(queue, item{path, cur.depth + 1})
				}
			case formats.IsAudioFile(e.Name):
				if len(ix.Files) >= lim.MaxFiles {
					ix.Truncated = true
					continue
				}
				rel := strings.TrimPrefix(strings.TrimPrefix(path, root), "/")
				ix.Files = append(ix.Files, IndexedFile{Path: path, Rel: rel, lower: strings.ToLower(rel)})
			}
		}
	}
	return ix, nil
}

// Search lists the files whose path (folders included) contains every word of query, case
// ignored. Files whose own name matches come first, then by path. An empty query lists
// nothing.
func (ix *Index) Search(query string, limit int) []IndexedFile {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	type hit struct {
		file  IndexedFile
		named bool
	}
	var hits []hit
	for _, f := range ix.Files {
		all, named := true, true
		name := strings.ToLower(f.Name())
		for _, w := range words {
			if !strings.Contains(f.lower, w) {
				all = false
				break
			}
			if !strings.Contains(name, w) {
				named = false
			}
		}
		if all {
			hits = append(hits, hit{f, named})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].named != hits[j].named {
			return hits[i].named
		}
		return hits[i].file.lower < hits[j].file.lower
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]IndexedFile, len(hits))
	for i, h := range hits {
		out[i] = h.file
	}
	return out
}

// Ancestors lists the folders above path, down from root, to open in a tree to show it.
func Ancestors(root, path string) []string {
	var out []string
	for dir := Dir(path); dir != root && dir != "" && dir != "/" && dir != CardRoot; dir = Dir(dir) {
		out = append([]string{dir}, out...)
	}
	return out
}
