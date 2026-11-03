package core

import (
	"fmt"
	"path/filepath"
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
	pane(1, pw, ptitle, false)
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
	if ok {
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
