package handheld

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The splash: the logo and a short dashboard, shown for a moment before the app opens. A key skips it. When an autosave
// of an earlier session is there and nothing is open, it waits for a key (A restores it, any other starts empty).
type splash struct {
	until   time.Time
	restore string // the autosaved project, when there is one
	when    time.Time
	started time.Time
}

// ShowSplash puts the splash in front for d (a script or a test never calls it).
func (a *App) ShowSplash(d time.Duration) {
	s := &splash{started: a.clock(), until: a.clock().Add(d)}
	if dir, when := a.findAutosave(); dir != "" && !a.hasWork() {
		s.restore, s.when = dir, when
	}
	a.splash = s
}

func (a *App) splashKey(k Key) {
	s := a.splash
	a.splash = nil
	if k == A && s.restore != "" {
		a.restoreAutosave(s.restore)
	}
}

func (a *App) splashTick() {
	if s := a.splash; s != nil && s.restore == "" && a.clock().After(s.until) {
		a.splash = nil
	}
}

// countFiles counts the files in a folder (not below it), for the dashboard.
func countFiles(dir string) int {
	des, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, de := range des {
		if !de.IsDir() {
			n++
		}
	}
	return n
}

func (a *App) drawSplash(c *canvas) {
	s := a.splash
	dy := max((Rows-MinRows)/2, 0) * CellH // a taller screen: everything a little lower
	lx := max((W-520)/2/CellW, 2) * CellW
	// the logo: the name in big letters beside a 4x4 pad grid with a kick pattern lit
	c.logo(lx/CellW, 4+dy/CellH, "404tui", cAmber)
	gx, gy := lx+42*CellW+16, 4*CellH+dy
	for i := 0; i < 16; i++ {
		x, y := gx+(i%4)*36, gy+(i/4)*36
		col := cPanel2
		switch i {
		case 0, 4, 8, 12:
			col = cAmber
		case 2, 10:
			col = cAmberD
		}
		c.rect(x, y, 30, 30, col)
	}
	c.text(lx, 14*CellH+dy, "SAMPLE LAB FOR THE SP-404MKII", cText, true)
	c.text(lx, 15*CellH+dy, "samples, slices, patterns, projects", cDim, false)
	c.rect(lx, 17*CellH+dy-4, 520, 1, cLine)

	// what is here
	row := func(r int, what, val string) {
		c.text(lx, r*CellH+dy, what, cDim, false)
		c.text(lx+12*CellW, r*CellH+dy, fit(val, 50), cText, false)
	}
	used := 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
		}
	}
	row(18, "project", fmt.Sprintf("%s   %d pads with samples", a.ProjectName(), used))
	row(19, "samples", fmt.Sprintf("%s   %d in the folder", short2(a.Root, 30), countFiles(a.Root)))
	row(20, "card", short2(filepath.Join(a.OutDir, cardFolder), 50))
	row(21, "autosave", a.autosaveSetting())
	if Version != "" {
		row(22, "version", Version)
	}
	if s.restore != "" {
		c.rect(lx, 24*CellH+dy, 520, 3*CellH, cAmberD)
		c.box(lx, 24*CellH+dy, 520, 3*CellH, 2, cAmber)
		c.text(lx+CellW, 24*CellH+dy+CellH/2, fit(fmt.Sprintf("AUTOSAVE FOUND: %s, saved at %s", filepath.Base(s.restore), s.when.Format("15:04")), 63), cWhite, true)
		c.text(lx+CellW, 25*CellH+dy+CellH/2, "A restores it     any other key starts empty", cText, false)
	} else {
		lbl := "press any key"
		if a.clock().Sub(s.started)/(500*time.Millisecond)%2 == 0 {
			c.text(lx, 25*CellH+dy, lbl, cDim, false)
		}
	}
	c.text(W-margin-textW("moahbeatlab"), H-CellH, "moahbeatlab", cLine, false)
}
