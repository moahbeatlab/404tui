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

func (a *App) drawSplash(g Gfx) {
	s := a.splash
	dy := max((Rows-MinRows)/2, 0) // a taller screen: everything a little lower
	lx := max((Cols-65)/2, 2)
	// the logo: the name in big letters beside a 4x4 pad grid with a kick pattern lit
	g.Logo(lx, 4+dy, "404tui", cAmber)
	gx, gy := lx+43, 4+dy
	for i := 0; i < 16; i++ {
		x, y := gx+(i%4)*3, gy+(i/4)*2
		col := cPanel2
		switch i {
		case 0, 4, 8, 12:
			col = cAmber
		case 2, 10:
			col = cAmberD
		}
		g.Fill(x, y, 2, 1, col)
	}
	g.Text(lx, 14+dy, "SAMPLE LAB FOR THE SP-404MKII", cText, true)
	g.Text(lx, 15+dy, "samples, slices, patterns, projects", cDim, false)
	g.HRule(lx, 17+dy, 65, cLine)

	// what is here
	row := func(r int, what, val string) {
		g.Text(lx, r+dy, what, cDim, false)
		g.Text(lx+12, r+dy, fit(val, 50), cText, false)
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
		g.Fill(lx, 23+dy, 65, 4, cAmberD)
		g.Frame(lx, 23+dy, 65, 4, cAmber)
		g.Text(lx+2, 24+dy, fit(fmt.Sprintf("AUTOSAVE FOUND: %s, saved at %s", filepath.Base(s.restore), s.when.Format("15:04")), 61), cWhite, true)
		g.Text(lx+2, 25+dy, "A restores it     any other key starts empty", cText, false)
	} else {
		lbl := "press any key"
		if a.clock().Sub(s.started)/(500*time.Millisecond)%2 == 0 {
			g.Text(lx, 25+dy, lbl, cDim, false)
		}
	}
	g.Text(Cols-2-len("moahbeatlab"), Rows-1, "moahbeatlab", cLine, false)
}
