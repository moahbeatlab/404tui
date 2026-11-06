package core

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/moahbeatlab/404tui/app"
)

// The ranger view of FILES, for screens wide enough to hold it: the folder above on the left, this folder in the middle
// (where the cursor is), and on the right what the cursor is on: the contents of a folder, the shape and length of a
// sample, what a project holds. The handheld's screen is narrower and keeps the single list.

const rangerMinCols = 64

// rangerCache keeps the last listings read for the side columns, so that drawing a frame never reads the unit.
type rangerCache struct {
	dir  string
	ents []entry
	err  error
}

func (a *App) cachedDir(c *rangerCache, dir string) ([]entry, error) {
	if c.dir != dir || c.ents == nil && c.err == nil {
		c.dir = dir
		c.ents, c.err = a.readDir(dir, false)
	}
	return c.ents, c.err
}

// RefreshRanger forgets the side columns, after something changed on disk.
func (a *App) refreshRanger() { a.rParent, a.rChild = rangerCache{}, rangerCache{} }

func (a *App) drawRanger(g Gfx) {
	w := contentCols - 2
	pw, cw := w*22/100, w*38/100
	pvw := w - pw - cw
	h := Rows - 3
	pane := func(x, w int, title string, focus bool) {
		c := cLine
		if focus {
			c = cAmber
		}
		g.Fill(x, 1, w, h, cBg)
		g.Frame(x, 1, w, h, c)
		tc := cDim
		if focus {
			tc = cAmber
		}
		g.Text(x+2, 1, " "+fit(title, w-6)+" ", tc, focus)
	}
	// the folder above, with this one marked
	ptitle := ""
	parent, hasParent := parentPath(a.dir)
	if hasParent && a.searchQ == "" {
		ptitle = baseName(parent)
	}
	if !hasParent && a.searchQ == "" {
		ptitle = "sources"
	}
	var resDir, resName string // a search: the folder the result in front is in, and its listing
	if a.searchQ != "" {
		if e, ok := a.current(); ok {
			rel := filepath.ToSlash(e.name)
			resDir, resName = joinPath(a.dir, path.Dir(rel)), path.Base(rel)
			if path.Dir(rel) == "." {
				resDir = a.dir
			}
			ptitle = baseName(resDir)
		}
	}
	pane(1, pw, ptitle, false)
	if resDir != "" {
		if ents, err := a.cachedDir(&a.rParent, resDir); err == nil {
			at := -1
			for i, e := range ents {
				if e.name == resName {
					at = i
				}
			}
			a.drawFileList(g, 2, pw-2, ents, at, false)
		}
	}
	if !hasParent && a.searchQ == "" { // the top of a source: the left column says which source this is, and what else there is
		a.drawSources(g, 2, pw-2)
	}
	if hasParent && a.searchQ == "" {
		if ents, err := a.cachedDir(&a.rParent, parent); err == nil {
			at := -1
			for i, e := range ents {
				if e.dir && joinPath(parent, e.name) == a.dir {
					at = i
				}
			}
			a.drawFileList(g, 2, pw-2, ents, at, false)
		}
	}
	ctitle := baseName(a.dir)
	if a.dir == a.Root || ctitle == "" {
		ctitle = short2(a.dir, cw-8)
	}
	if a.isFavourite(a.dir) {
		ctitle = "* " + ctitle
	}
	if a.searchQ != "" {
		ctitle = fmt.Sprintf("search %q", a.searchQ)
		if a.searchCut {
			ctitle += " (cut short)"
		}
	}
	pane(1+pw, cw, ctitle, true)
	if len(a.entries) == 0 {
		g.Text(1+pw+2, 3, "(no folders or audio files here)", cDim, false)
		if a.dir == a.Root {
			g.Text(1+pw+2, 5, "Put .wav files in:", cText, false)
			g.Text(1+pw+2, 6, short2(a.Root, cw-5), cAmber, false)
		}
	}
	a.drawFileList(g, 2+pw, cw-2, a.entries, a.fcur, true)
	e, ok := a.current()
	pt := ""
	if ok && (e.dir || !Terminal) { // a computer's third column is for folders (the shape of a sample is in the panel)
		pt = e.name
	}
	pane(1+pw+cw, pvw, pt, false)
	a.drawPreview(g, 1+pw+cw+2, pvw-4)
}

// drawPreview shows what the cursor is on.
func (a *App) drawPreview(g Gfx, x, w int) {
	e, ok := a.current()
	if !ok {
		return
	}
	path := joinPath(a.dir, e.name)
	switch {
	case e.project:
		g.Text(x, 2, "a project of the SP-404", cGood, true)
		g.Text(x, 3, tk("A loads it"), cDim, false)
		fallthrough
	case e.dir:
		if e.project {
			return
		}
		ents, err := a.cachedDir(&a.rChild, path)
		if err != nil {
			g.Text(x, 2, fit(err.Error(), w), cBad, false)
			return
		}
		if len(ents) == 0 {
			g.Text(x, 2, "(empty)", cDim, false)
			return
		}
		a.drawFileList(g, x-1, w+2, ents, -1, false)
	default:
		if Terminal {
			return
		}
		g.Text(x, 2, fit(filepath.Base(e.name), w), cText, true)
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			g.Text(x, 3, fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			if tg := a.tagsOf(path); tg != "" {
				g.Text(x, 4, fit(tg, w), cAmber, false)
			}
			g.Fill(x, 6, w, 8, cPanel2)
			a.wave(g, id, 0, s.Frames(), x, 6, w, 8, cText)
		} else {
			g.Text(x, 3, fit(err.Error(), w), cBad, false)
		}
	}
}

// fileSource is a place the files can be loaded from.
type fileSource struct{ name, dir string }

// fileSources are the places `t` offers on FILES: this computer, the samples, drives, and the unit's card and storage when one is there.
func (a *App) fileSources() []fileSource {
	var out []fileSource
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, fileSource{"computer", home})
	}
	out = append(out, fileSource{"samples", a.Root})
	for _, pl := range []fileSource{{"usb drive", "/mnt/usbdrive"}, {"games card", "/roms"}, {"the top", "/"}} {
		if st, err := os.Stat(pl.dir); err == nil && st.IsDir() {
			out = append(out, pl)
		}
	}
	if _, ok := a.unitDevice(); ok {
		out = append(out, fileSource{"SD card", app.CardRoot}, fileSource{"Unit storage", app.StorageRoot})
	}
	return out
}

// drawSources lists the sources in the left column, the one this folder is in lit.
func (a *App) drawSources(g Gfx, x0, w int) {
	src := a.fileSources()
	at, best := -1, -1
	for i, s := range src {
		if a.dir == s.dir {
			at, best = i, len(s.dir)+1 // the folder is the source's top
		} else if strings.HasPrefix(a.dir, strings.TrimSuffix(s.dir, "/")+"/") && len(s.dir) > best {
			at, best = i, len(s.dir)
		}
	}
	for i, s := range src {
		y := 2 + i
		if y >= Rows-3 {
			break
		}
		fg := cText
		if i == at {
			g.Fill(x0, y, w, 1, cPanel2)
			fg = cAmber
		}
		g.Text(x0+1, y, fit(s.name, w-2), fg, i == at)
	}
}
