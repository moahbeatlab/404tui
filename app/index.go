package app

import (
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto/fileapi"
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
	// Unreadable are folders the unit would not list, even after trying again: what is in
	// them is missing from the index.
	Unreadable []string
	// Saved is when the saved index this was made from was written; empty for a walk of the
	// card made now.
	Saved   string
	folders []string
}

// ListDirRetry reads a folder, and tries twice more after a short wait when the unit
// answers with an error: it can be busy for a moment, for example just after another
// program let go of it.
func ListDirRetry(list func(string) ([]fileapi.DirEntry, error), dir string) ([]fileapi.DirEntry, error) {
	entries, err := list(dir)
	for try := 0; err != nil && try < 2; try++ {
		time.Sleep(time.Duration(150*(try+1)) * time.Millisecond)
		entries, err = list(dir)
	}
	return entries, err
}

// IndexProgress is how far a walk has come: Queued folders are known and still to read, so
// the share done is Folders/(Folders+Queued).
type IndexProgress struct {
	Folders, Queued, Files, Unreadable int
	Current                            string
}

// BuildIndex walks root breadth first and lists its audio files. A folder that cannot be
// read is skipped.
func BuildIndex(s Storage, root string, lim IndexLimits) (*Index, error) {
	ix, _, err := BuildIndexWith(s.ListDir, root, lim, nil, nil)
	return ix, err
}

// BuildIndexWith is BuildIndex with the folder reading, the progress and a way to stop in
// the caller's hands: list reads one folder (a caller can take a lock in it, or wait, so
// that other work on the unit goes on), progress is told after each folder, and the walk
// ends when cancel returns true, giving what it found so far with completed false.
func BuildIndexWith(list func(string) ([]fileapi.DirEntry, error), root string, lim IndexLimits,
	progress func(IndexProgress), cancel func() bool) (ix *Index, completed bool, err error) {
	ix = &Index{Root: root}
	type item struct {
		path  string
		depth int
	}
	queue := []item{{root, 0}}
	first := true
	prog := IndexProgress{}
	completed = true
	for len(queue) > 0 {
		if cancel != nil && cancel() {
			completed = false
			break
		}
		cur := queue[0]
		queue = queue[1:]
		if ix.Folders >= lim.MaxFolders {
			ix.Truncated = true
			break
		}
		entries, lerr := ListDirRetry(list, cur.path)
		if lerr != nil {
			if first {
				return nil, false, lerr
			}
			ix.Unreadable = append(ix.Unreadable, cur.path)
			prog.Unreadable++
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
		if progress != nil {
			prog.Folders, prog.Queued, prog.Files, prog.Current = ix.Folders, len(queue), len(ix.Files), cur.path
			progress(prog)
		}
	}
	return ix, completed, nil
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
		return NaturalLess(hits[i].file.lower, hits[j].file.lower)
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

// FolderPaths lists every folder that holds an indexed file, and the folders above them
// down to the root, sorted. It is worked out once.
func (ix *Index) FolderPaths() []string {
	if ix.folders != nil {
		return ix.folders
	}
	seen := map[string]bool{}
	for _, f := range ix.Files {
		for dir := Dir(f.Path); dir != ix.Root && dir != "" && dir != "/" && dir != CardRoot && !seen[dir]; dir = Dir(dir) {
			seen[dir] = true
		}
	}
	out := make([]string, 0, len(seen))
	for dir := range seen {
		out = append(out, dir)
	}
	sort.Slice(out, func(i, j int) bool { return NaturalLess(strings.ToLower(out[i]), strings.ToLower(out[j])) })
	ix.folders = out
	return out
}
