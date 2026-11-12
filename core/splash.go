package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/moahbeatlab/404tui/lab"
)

// The splash: the logo and a short dashboard, shown for a moment before the app opens. A key skips it. When an autosave
// of an earlier session is there and nothing is open, it waits for a key (A restores it, any other starts empty).
type splash struct {
	until    time.Time
	restore  string // the autosaved project, when there is one
	when     time.Time
	started  time.Time
	patterns int // the patterns of the open project with notes (counted once: the launcher says it)
}

// ShowSplash puts the splash in front for d (a script or a test never calls it).
func (a *App) ShowSplash(d time.Duration) {
	s := &splash{started: a.clock(), until: a.clock().Add(d), patterns: a.patternsWithNotes()}
	if dir, when := a.findAutosave(); dir != "" && !a.hasWork() {
		s.restore, s.when = dir, when
	}
	a.splash = s
}

func (a *App) splashKey(k Key) {
	s := a.splash
	a.splash = nil
	if k != A {
		return
	}
	switch {
	case s.restore != "":
		a.restoreAutosave(s.restore)
	case Terminal && !a.hasWork(): // the launcher: Enter continues with the project opened last
		a.openRecent(0)
	}
}

func (a *App) splashTick() {
	if s := a.splash; s != nil && s.restore == "" && !Terminal && a.clock().After(s.until) { // the launcher of a computer waits for a choice
		a.splash = nil
	}
}

// SplashText is a letter of a keyboard on the launcher: u works online (the unit's projects), o offline (the project open last,
// or the folder's projects), f opens a project from a folder, 1 to 4 a recent project. It says whether the letter was one of them.
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
		a.go_(Unit)
		a.goOnline() // the unit's open project comes into the editor, and what is changed goes to the unit
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
	case "f", "F":
		a.splash = nil
		a.openProjectFolder()
		return true
	case "1", "2", "3", "4":
		if a.openRecent(int(r[0] - '1')) {
			a.splash = nil
		}
		return true
	}
	return false
}

// patternsWithNotes is how many of the 160 patterns have notes.
func (a *App) patternsWithNotes() int {
	n := 0
	for slot := 1; slot <= 160; slot++ {
		b, err := a.lab.Pattern(slot)
		var d lab.PatternDoc
		if err == nil && json.Unmarshal(b, &d) == nil && len(d.Notes) > 0 {
			n++
		}
	}
	return n
}

func (a *App) drawSplash(g Gfx) {
	if Terminal {
		a.drawLauncher(g)
		return
	}
	s := a.splash
	dy := max((Rows-MinRows)/2, 0) // a taller screen: everything a little lower
	w := 65
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
	row(19, "folder", short2(filepath.Join(a.OutDir, cardFolder), w-15))
	row(20, "autosave", a.autosaveSetting())
	if Version != "" {
		row(21, "version", Version)
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
