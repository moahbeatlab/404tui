//go:build !windows

package handheld

import (
	"fmt"
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
	a.searchQ = ""
	a.dir = dir
	a.list()
	a.set.LastDir = dir
	a.saveSettings()
}

var audioExts = map[string]bool{".wav": true, ".aif": true, ".aiff": true, ".flac": true, ".mp3": true, ".smp": true}

// searchList fills the list with the audio files below the folder whose path has all the words of the search. It gives up
// after 1.5 s, 40000 files looked at or 400 found, and says so.
func (a *App) searchList() {
	words := strings.Fields(strings.ToLower(a.searchQ))
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
		return strings.ToLower(a.entries[i].name) < strings.ToLower(a.entries[j].name)
	})
	a.fcur = 0
}

// ---- the on-screen keyboard for the search

var kbRows = []string{"abcdefghij", "klmnopqrst", "uvwxyz0123", "456789_-. "}

type keyboard struct{ r, c int }

func (a *App) openSearch() {
	a.kb = &keyboard{}
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
			a.say(fmt.Sprintf("%d files for %q: < ends the search", len(a.entries), a.searchQ), false)
		}
	}
}

func (a *App) drawKeyboard(c *canvas) {
	x0, y0, w, h := 16, 250, contentW-32, 170
	c.rect(x0, y0, w, h, cPanel2)
	c.box(x0, y0, w, h, 2, cAmber)
	q := a.searchQ
	c.text(x0+12, y0+10, "SEARCH", cDim, false)
	c.text(x0+12+textW("SEARCH "), y0+10, q+"_", cAmber, true)
	n := fmt.Sprintf("%d files", len(a.entries))
	if len(a.entries) == 1 {
		n = "1 file"
	}
	if a.searchCut {
		n += "+"
	}
	c.text(x0+w-12-textW(n), y0+10, n, cDim, false)
	const kw, kh = 34, 26
	gx := x0 + (w-10*kw)/2
	for r, row := range kbRows {
		for col := 0; col < len(row); col++ {
			x, y := gx+col*kw, y0+36+r*(kh+4)
			bg, fg := cPanel, cText
			if r == a.kb.r && col == a.kb.c {
				bg, fg = cAmber, cBg
			}
			c.rect(x, y, kw-3, kh, bg)
			ch := string(row[col])
			if ch == " " {
				ch = "sp"
			}
			c.text(x+(kw-3-textW(ch))/2, y+5, ch, fg, true)
		}
	}
}

// ---- places and favourites

func (a *App) placesMenu() {
	m := &menu{title: "places and favourites"}
	go_ := func(dir string) func(int) { return func(int) { a.cd(dir); a.say("in "+short2(dir, 44), false) } }
	m.head("PLACES")
	for _, pl := range []struct{ name, dir string }{{"samples", a.Root}, {"usb drive", "/mnt/usbdrive"}, {"games card", "/roms"}, {"the top", "/"}} {
		if st, err := os.Stat(pl.dir); err == nil && st.IsDir() {
			m.items = append(m.items, menuItem{label: fmt.Sprintf("%-10s %s", pl.name, short2(pl.dir, 22)), run: go_(pl.dir)})
		}
	}
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
	a.menu = m
}
