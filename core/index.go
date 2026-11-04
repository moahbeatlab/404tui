package core

import (
	"bufio"
	"crypto/sha1"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// The file index: one text file per indexed folder in <data>/index, a "#root" line and then "relative path<TAB>size"
// for every audio file below it. Building it walks the folder once in the background (nothing is kept in memory but a
// counter); a search streams the lines of the index that covers the folder and keeps only the 400 it shows.

type indexJob struct {
	mu      sync.Mutex
	running bool
	root    string
	count   int64 // files so far (atomic)
	pending string
}

func (a *App) indexDir() string { return filepath.Join(a.dataDir(), "index") }

func (a *App) indexPath(root string) string {
	h := sha1.Sum([]byte(root))
	return filepath.Join(a.indexDir(), fmt.Sprintf("%x.txt", h[:6]))
}

// indexRoot reads the first line of an index file.
func indexRootOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if s.Scan() {
		return strings.TrimPrefix(s.Text(), "#root ")
	}
	return ""
}

// indexFor finds the index that covers the folder: the folder itself or the nearest folder above it that is indexed.
func (a *App) indexFor(dir string) (path, root string) {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		p := a.indexPath(d)
		if _, err := os.Stat(p); err == nil {
			return p, d
		}
		if d == "/" || d == "." || filepath.Dir(d) == d { // the top: "/" here, a drive root such as C:\ on Windows
			return "", ""
		}
	}
}

// startIndex builds the index of the folder in the background.
func (a *App) startIndex(root string) {
	j := &a.idx
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		a.say("already indexing "+short2(j.root, 30), true)
		return
	}
	j.running, j.root = true, root
	atomic.StoreInt64(&j.count, 0)
	j.mu.Unlock()
	a.say("indexing "+short2(root, 30)+" in the background", false)
	go func() {
		n, err := a.buildIndex(root, &j.count)
		j.mu.Lock()
		j.running = false
		if err != nil {
			j.pending = "index failed: " + err.Error()
		} else {
			j.pending = fmt.Sprintf("indexed %d files in %s", n, short2(root, 24))
		}
		j.mu.Unlock()
	}()
}

func (a *App) buildIndex(root string, count *int64) (int64, error) {
	if err := os.MkdirAll(a.indexDir(), 0o755); err != nil {
		return 0, err
	}
	final := a.indexPath(root)
	tmp := final + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	w := bufio.NewWriterSize(f, 32<<10)
	fmt.Fprintf(w, "#root %s\n", root)
	skip := map[string]bool{"/proc": true, "/sys": true, "/dev": true, "/run": true}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if skip[p] {
			return filepath.SkipDir
		}
		if p != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !audioExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		var size int64
		if fi, err := d.Info(); err == nil {
			size = fi.Size()
		}
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(w, "%s\t%d\n", rel, size)
		atomic.AddInt64(count, 1)
		return nil
	})
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	if err := os.Rename(tmp, final); err != nil {
		return 0, err
	}
	return atomic.LoadInt64(count), nil
}

// indexTick hands a finished run's message to the screen.
func (a *App) indexTick() {
	j := &a.idx
	j.mu.Lock()
	msg := j.pending
	j.pending = ""
	j.mu.Unlock()
	if msg != "" {
		a.say(msg, strings.HasPrefix(msg, "index failed"))
	}
}

// searchIndex fills the list from the index; false when none covers the folder.
func (a *App) searchIndex(words []string) bool {
	p, root := a.indexFor(a.dir)
	if p == "" {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	prefix := ""
	if rel, _ := filepath.Rel(root, a.dir); rel != "." {
		prefix = strings.ToLower(rel) + string(filepath.Separator)
	}
	a.searchCut = false
	seenDirs := map[string]bool{}
	s := bufio.NewScanner(f)
	s.Scan() // the #root line
	for s.Scan() {
		line := s.Text()
		tab := strings.LastIndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		low := strings.ToLower(line[:tab])
		if prefix != "" && !strings.HasPrefix(low, prefix) {
			continue
		}
		if a.searchDirs { // folders only: the folders above the indexed files that have all the words in their path
			rel := line[len(prefix):tab]
			for i := 0; i < len(rel); i++ {
				if rel[i] != '/' && rel[i] != filepath.Separator {
					continue
				}
				d := rel[:i]
				lowd := strings.ToLower(d)
				if seenDirs[lowd] {
					continue
				}
				seenDirs[lowd] = true
				all := true
				for _, w := range words {
					if !strings.Contains(lowd, w) {
						all = false
						break
					}
				}
				if all {
					a.entries = append(a.entries, entry{name: d, dir: true})
				}
			}
			if len(a.entries) >= 400 {
				a.searchCut = true
				break
			}
			continue
		}
		ok := true
		for _, w := range words {
			if !strings.Contains(low[len(prefix):], w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		size, _ := strconv.ParseInt(line[tab+1:], 10, 64)
		a.entries = append(a.entries, entry{name: line[len(prefix):tab], size: size})
		if len(a.entries) >= 400 {
			a.searchCut = true
			break
		}
	}
	sort.SliceStable(a.entries, func(i, j int) bool {
		return strings.ToLower(a.entries[i].name) < strings.ToLower(a.entries[j].name)
	})
	a.fcur = 0
	return true
}

// indexItems are the menu entries for the folder being looked at.
func (a *App) indexItems(m *menu) {
	m.head("SEARCH INDEX")
	if p, root := a.indexFor(a.dir); p != "" {
		size := int64(0)
		if st, err := os.Stat(p); err == nil {
			size = st.Size()
		}
		m.items = append(m.items, menuItem{label: fit(fmt.Sprintf("indexed: %s (%d KB)", short2(root, 22), size/1024), 40), header: true})
		m.items = append(m.items, menuItem{label: "Index again", run: func(int) { a.startIndex(root) }})
		m.items = append(m.items, menuItem{label: "Forget this index", run: func(int) {
			os.Remove(p)
			a.say("index forgotten", false)
			a.placesMenu()
		}})
	}
	m.items = append(m.items, menuItem{label: fit("Index "+short2(a.dir, 30), 40), run: func(int) { a.startIndex(filepath.Clean(a.dir)) }})
}

// indexStatus is whether an index is being built, for which folder, and how many files it has found so far.
func (a *App) indexStatus() (running bool, root string, files int64) {
	j := &a.idx
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.running, j.root, atomic.LoadInt64(&j.count)
}

// drawIndexProgress is the panel's part for an index that is being built (three rows from y): the folder, how many files so far, and a
// bar that runs along while it works (the end is not known: the folder is walked once).
func (a *App) drawIndexProgress(g Gfx, x, y, w int) {
	_, root, n := a.indexStatus()
	g.Text(x, y, "INDEXING", cAmber, true)
	cnt := fmt.Sprintf("%d files", n)
	g.Text(x+w-len(cnt), y, cnt, cText, false)
	g.Text(x, y+1, fit(short2(root, w), w), cDim, false)
	g.Fill(x, y+2, w, 1, cPanel2)
	run := min(8, w/3)
	pos := int(a.clock().UnixMilli()/80) % max(2*(w-run), 1)
	if pos > w-run {
		pos = 2*(w-run) - pos // out and back
	}
	g.Fill(x+pos, y+2, run, 1, cAmber)
}
