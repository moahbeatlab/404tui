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
	// the logo: the name in big letters beside a 4x4 pad grid with a kick pattern lit
	c.big(56, 70, "404h", cAmber, 7)
	gx, gy := 400, 70
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
	c.text(58, 192, "SAMPLE LAB FOR THE SP-404MKII", cText, true)
	c.text(58, 212, "samples, slices, patterns, projects", cDim, false)
	c.rect(56, 244, 528, 1, cLine)

	// what is here
	row := func(y int, what, val string) {
		c.text(58, y, what, cDim, false)
		c.text(150, y, fit(val, 54), cText, false)
	}
	used := 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
		}
	}
	row(260, "project", fmt.Sprintf("%s   %d pads with samples", a.ProjectName(), used))
	row(280, "samples", fmt.Sprintf("%s   %d in the folder", short2(a.Root, 30), countFiles(a.Root)))
	row(300, "card", short2(filepath.Join(a.OutDir, cardFolder), 54))
	row(320, "autosave", a.autosaveSetting())
	if Version != "" {
		row(340, "version", Version)
	}
	if s.restore != "" {
		c.rect(56, 372, 528, 44, cAmberD)
		c.box(56, 372, 528, 44, 2, cAmber)
		c.text(70, 378, fmt.Sprintf("AUTOSAVE FOUND: %s, saved at %s", filepath.Base(s.restore), s.when.Format("15:04")), cWhite, true)
		c.text(70, 396, "A restores it     any other key starts empty", cText, false)
	} else {
		lbl := "press any key"
		if a.clock().Sub(s.started)/(500*time.Millisecond)%2 == 0 {
			c.text(58, 392, lbl, cDim, false)
		}
	}
	c.text(W-margin-textW("moahbeatlab"), H-24, "moahbeatlab", cLine, false)
}
