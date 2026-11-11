package core

import (
	"fmt"
	"github.com/moahbeatlab/404tui/app"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The file browser's helpers: moving between folders (remembering the cursor of each and the folder it was last in),
// search (a filter over the folder and everything below it, typed on an on-screen keyboard), and the places and
// favourites menu.

// cd goes to a folder: the cursor of the folder left is remembered, and the folder is kept for the next start.
func (a *App) cd(dir string) {
	if a.searchQ == "" {
		a.dcur[a.dir] = a.fcur
	}
	a.searchQ, a.tagSearch, a.searchDirs = "", false, false
	a.dir = dir
	a.list()
	if app.IsLocal(dir) { // the unit's folders are not kept for the next start
		a.set.LastDir = dir
		a.saveSettings()
	}
}

var audioExts = map[string]bool{".wav": true, ".aif": true, ".aiff": true, ".flac": true, ".mp3": true, ".smp": true}

// searchList fills the list with the audio files below the folder whose path has all the words of the search. It gives up
// after 1.5 s, 40000 files looked at or 400 found, and says so.
func (a *App) searchList() {
	words := strings.Fields(strings.ToLower(a.searchQ))
	if a.tagSearch && app.IsLocal(a.dir) { // the categories of this computer's samples: kick, hat, loop, 126...
		a.searchLocalTags(words)
		return
	}
	if a.tagSearch && a.tags != nil {
		for _, f := range a.tags.Matches(a.searchQ, a.dir, 400) {
			a.entries = append(a.entries, entry{name: f.Rel})
		}
		a.searchCut = len(a.entries) >= 400
		a.fcur = 0
		return
	}
	if a.searchIndex(words) {
		return
	}
	root := a.dir
	deadline := time.Now().Add(1500 * time.Millisecond)
	looked := 0
	a.searchCut = false
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
		looked++
		if looked > 40000 || time.Now().After(deadline) {
			a.searchCut = true
			return filepath.SkipAll
		}
		if a.searchDirs { // folders only
			if !d.IsDir() || p == root {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			low := strings.ToLower(d.Name()) // the name of the folder itself holds the words, not the folders above it
			for _, w := range words {
				if !strings.Contains(low, w) {
					return nil
				}
			}
			a.entries = append(a.entries, entry{name: rel, dir: true})
			if len(a.entries) >= 400 {
				a.searchCut = true
				return filepath.SkipAll
			}
			return nil
		}
		if d.IsDir() || !audioExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		low := strings.ToLower(rel)
		for _, w := range words {
			if !strings.Contains(low, w) {
				return nil
			}
		}
		var size int64
		if fi, err := d.Info(); err == nil {
			size = fi.Size()
		}
		a.entries = append(a.entries, entry{name: rel, size: size})
		if len(a.entries) >= 400 {
			a.searchCut = true
			return filepath.SkipAll
		}
		return nil
	})
	sort.SliceStable(a.entries, func(i, j int) bool {
		return app.NaturalLess(strings.ToLower(a.entries[i].name), strings.ToLower(a.entries[j].name))
	})
	a.fcur = 0
}

// ---- the on-screen keyboard for the search

var kbRows = []string{"abcdefghij", "klmnopqrst", "uvwxyz0123", "456789_-. "}

type keyboard struct{ r, c int }

func (a *App) openSearch() {
	a.tagSearch, a.searchDirs = false, false
	a.kb = &keyboard{}
	if Terminal {
		a.say("searching file names: Tab looks for folders, then for categories (kick, hat, loop...)", false)
	}
}

// OpenTagSearch is # of a keyboard on FILES: the search by categories (the tags of the samples: kick, hat, loop, 126, the folders' words).
func (a *App) OpenTagSearch() {
	if a.scr != Files || a.kb != nil || a.Modal() || a.prompt != nil {
		return
	}
	a.openSearch()
	a.tagSearch = true
	a.list()
}

func (a *App) kbKey(k Key) {
	kb := a.kb
	switch k {
	case Up:
		kb.r = (kb.r + len(kbRows) - 1) % len(kbRows)
	case Down:
		kb.r = (kb.r + 1) % len(kbRows)
	case Left:
		kb.c = (kb.c + len(kbRows[0]) - 1) % len(kbRows[0])
	case Right:
		kb.c = (kb.c + 1) % len(kbRows[0])
	case A:
		a.searchQ += string(kbRows[kb.r][kb.c])
		a.list()
	case B:
		if a.yHeld { // Y+B: everything
			a.searchQ = ""
			a.list()
			return
		}
		if a.searchQ == "" {
			a.kb = nil
			return
		}
		a.searchQ = a.searchQ[:len(a.searchQ)-1]
		a.list()
	case X: // done: the list stays filtered
		a.kb = nil
		if a.searchQ != "" {
			a.say(fmt.Sprintf("%d files for %q: %s ends the search", len(a.entries), a.searchQ, map[bool]string{true: "Esc or h", false: "<"}[Terminal]), false)
		}
	}
}

func (a *App) drawKeyboard(g Gfx) {
	if Terminal { // a computer has a keyboard: the search is one line at the bottom, typed into, as in the classic program
		y := Rows - 3
		g.Fill(1, y, contentCols-2, 1, cPanel2)
		g.Text(2, y, "SEARCH", cAmber, true)
		n := fmt.Sprintf("%d files", len(a.entries))
		if len(a.entries) == 1 {
			n = "1 file"
		}
		if a.searchCut {
			n += "+"
		}
		hint := "Tab folders  Enter keeps  Esc clears"
		label := "SEARCH"
		if a.searchDirs {
			hint, label = "Tab categories  Enter keeps  Esc clears", "FOLDERS"
		} else if a.tagSearch {
			hint, label = "Tab names  Enter keeps  Esc clears", "CATEGORY"
		}
		if contentCols < 64 { // a narrow screen keeps the keys short
			hint = strings.Fields(hint)[0] + " " + strings.Fields(hint)[1]
		}
		g.Text(2, y, label, cAmber, true)
		g.Text(contentCols-2-len(hint), y, hint, cDim, false)
		g.Text(contentCols-2-len(hint)-2-len(n), y, n, cDim, false)
		g.Text(11, y, fit(a.searchQ+"_", max(contentCols-4-len(hint)-len(n)-13, 4)), cText, true)
		return
	}
	wc, hr := contentCols-2, 11
	x0 := 1
	y0 := Rows - 2 - hr
	g.Fill(x0, y0, wc, hr, cPanel2)
	g.Frame(x0, y0, wc, hr, cAmber)
	q := a.searchQ
	g.Text(x0+2, y0+1, "SEARCH", cDim, false)
	g.Text(x0+2+7, y0+1, fit(q+"_", wc-30), cAmber, true)
	n := fmt.Sprintf("%d files", len(a.entries))
	if len(a.entries) == 1 {
		n = "1 file"
	}
	if a.searchCut {
		n += "+"
	}
	g.Text(x0+wc-2-len(n), y0+1, n, cDim, false)
	const kw = 4 // a key: 4 cells wide, 2 high
	gx := x0 + (wc-10*kw)/2
	for r, row := range kbRows {
		for col := 0; col < len(row); col++ {
			x, y := gx+col*kw, y0+2+r*2
			bg, fg := cPanel, cText
			if r == a.kb.r && col == a.kb.c {
				bg, fg = cAmber, cBg
			}
			g.Fill(x, y, kw-1, 1, bg)
			ch := string(row[col])
			if ch == " " {
				ch = "sp"
			}
			g.Text(x+(kw-1-len(ch))/2, y, ch, fg, true)
		}
	}
}

// ---- places and favourites

func (a *App) placesMenu() {
	m := &menu{title: "places and favourites"}
	if Terminal {
		m.title = "load files from where"
	}
	go_ := func(dir string) func(int) { return func(int) { a.cd(dir); a.say("in "+short2(dir, 44), false) } }
	m.head("PLACES")
	places := []struct{ name, dir string }{{"samples", a.Root}, {"usb drive", "/mnt/usbdrive"}, {"games card", "/roms"}, {"the top", "/"}}
	if home, err := os.UserHomeDir(); err == nil && Terminal {
		places = append([]struct{ name, dir string }{{"computer", home}}, places...) // this computer: the home folder
	}
	for _, pl := range places {
		if st, err := os.Stat(pl.dir); err == nil && st.IsDir() {
			m.items = append(m.items, menuItem{label: fmt.Sprintf("%-10s %s", pl.name, short2(pl.dir, 22)), run: go_(pl.dir)})
		}
	}
	a.unitPlaces(m, go_)
	m.head(fmt.Sprintf("FAVOURITES (%d)", len(a.set.Favourites)))
	for _, f := range a.set.Favourites {
		f := f
		m.items = append(m.items, menuItem{label: short2(f, 24), vals: []string{"go there", "remove"}, run: func(v int) {
			if v == 0 {
				go_(f)(0)
				return
			}
			a.toggleFavourite(f)
		}})
	}
	m.head("THIS FOLDER")
	label := "Add " + short2(a.dir, 24) + " to the favourites"
	if a.isFavourite(a.dir) {
		label = "Take it off the favourites"
	}
	m.items = append(m.items, menuItem{label: fit(label, 40), run: func(int) { a.toggleFavourite(a.dir) }})
	a.indexItems(m)
	if Terminal { // the keyboard's t is this menu: the file tools (copy, cut, rename, delete, tags) are one step on
		m.head("FILES")
		m.items = append(m.items, menuItem{label: "Copy, cut, rename, delete, tags...", run: func(int) { a.filesMenu() }})
	}
	a.menu = m
}
