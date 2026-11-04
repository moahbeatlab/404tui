package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if s := a.splash; s != nil && s.restore == "" && !(Terminal && a.link != nil) && a.clock().After(s.until) {
		a.splash = nil
	}
}

// SplashText is a letter of a keyboard on the splash: u works online (the unit's projects), o offline (the project open last,
// or the folder's projects). It says whether the letter was one of them.
func (a *App) SplashText(r string) bool {
	if a.splash == nil {
		return false
	}
	switch r {
	case "u", "U":
		if a.link == nil {
			a.say("no unit is plugged in: working offline", true)
			return true
		}
		a.splash = nil
		a.offlineChosen = false
		a.go_(Unit)
		return true
	case "o", "O":
		a.splash = nil
		a.offlineChosen = true
		if n, ok := a.lastOffline(); ok {
			a.openOffline(n)
		} else {
			a.go_(Unit)
		}
		return true
	}
	return false
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
	w := 65
	if Terminal { // a computer's window is wide: the lines and the box take more of it
		w = min(max(Cols-10, 65), 110)
	}
	lx := max((Cols-w)/2, 2)
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
	g.HRule(lx, 17+dy, w, cLine)

	// what is here
	row := func(r int, what, val string) {
		g.Text(lx, r+dy, what, cDim, false)
		g.Text(lx+12, r+dy, fit(val, w-15), cText, false)
	}
	used := 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
		}
	}
	row(18, "project", fmt.Sprintf("%s   %d pads with samples", a.ProjectName(), used))
	row(19, "samples", fmt.Sprintf("%s   %d in the folder", short2(a.Root, 30), countFiles(a.Root)))
	row(20, "card", short2(filepath.Join(a.OutDir, cardFolder), w-15))
	row(21, "autosave", a.autosaveSetting())
	next := 22
	if Terminal { // the unit: found, or not (it is looked for all the time)
		g.Text(lx, next+dy, "unit", cDim, false)
		if a.link != nil {
			g.Text(lx+12, next+dy, fit("SP-404MKII found: "+a.link.Name(), w-15), cGood, true)
		} else {
			g.Text(lx+12, next+dy, "none found yet: offline", cDim, false)
		}
		next++
	}
	if Version != "" {
		row(next, "version", Version)
		next++
	}
	top := next + 1
	if Terminal {
		lines := []string{}
		if a.link != nil {
			last := "its folder's projects"
			if n, ok := a.lastOffline(); ok {
				last = fmt.Sprintf("PROJECT_%02d, the last one", n+1)
			}
			lines = append(lines, fit("UNIT FOUND: "+a.link.Name(), w-6), "u  work online: the projects of the unit", "o  work offline: "+last)
		} else if n, ok := a.lastOffline(); ok {
			lines = append(lines, "NO UNIT: WORKING OFFLINE", fmt.Sprintf("o  opens PROJECT_%02d again, the one open last", n+1))
		}
		if s.restore != "" {
			lines = append(lines, fit(fmt.Sprintf("AUTOSAVE FOUND: %s, saved at %s", filepath.Base(s.restore), s.when.Format("15:04")), w-6), "Enter restores it     any other key starts empty")
		} else if len(lines) > 0 {
			lines = append(lines, "any other key continues with the project that is open")
		}
		if len(lines) > 0 {
			h := len(lines) + 2
			g.Fill(lx, top+dy, w, h, cAmberD)
			g.Frame(lx, top+dy, w, h, cAmber)
			for i, l := range lines {
				col, bold := cText, false
				if i == 0 || strings.HasPrefix(l, "AUTOSAVE") {
					col, bold = cWhite, true
				}
				g.Text(lx+2, top+1+i+dy, l, col, bold)
			}
		} else if a.clock().Sub(s.started)/(500*time.Millisecond)%2 == 0 {
			g.Text(lx, top+dy, "press any key", cDim, false)
		}
		g.Text(Cols-2-len("moahbeatlab"), Rows-1, "moahbeatlab", cLine, false)
		return
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
