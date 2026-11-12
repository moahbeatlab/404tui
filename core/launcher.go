package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// homeShort is a path with the home folder written as ~.
func homeShort(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// The launcher: the start of the program on a computer. The name in pixel letters with a shadow, then four doors with a key each
// (carry on, the unit, the offline folder, a folder) and the projects opened last; it waits for a choice.

// logoPx is the name in a square pixel font, 4 by 5 (the i is one wide); each pixel is two columns.
var logoPx = map[rune][]string{
	'4': {"1..1", "1..1", "1111", "...1", "...1"},
	'0': {"1111", "1..1", "1..1", "1..1", "1111"},
	't': {".1..", "1111", ".1..", ".1..", ".111"},
	'u': {"....", "1..1", "1..1", "1..1", "1111"},
	'i': {"1", ".", "1", "1", "1"},
}

// shadowLogo draws "404tui" in amber pixels with a dim shadow one pixel down and to the right.
func shadowLogo(g Gfx, x, y int) {
	var lit [][2]int
	px := 0
	for _, r := range "404tui" {
		rows := logoPx[r]
		for ry, row := range rows {
			for rx, c := range row {
				if c == '1' {
					lit = append(lit, [2]int{x + (px+rx)*2, y + ry})
				}
			}
		}
		px += len(rows[0]) + 1
	}
	for _, p := range lit { // the shadow first, the letters over it
		g.Text(p[0]+2, p[1]+1, "░░", cAmberD, false)
	}
	for _, p := range lit {
		g.Fill(p[0], p[1], 2, 1, cAmber)
	}
}

// keyCap is a key as a cap: lit (amber, the choice Enter makes) or plain.
func keyCap(g Gfx, x, y int, k string, lit bool) int {
	if lit {
		g.Fill(x, y, len([]rune(k))+2, 1, cAmber)
		g.Text(x+1, y, k, cBg, true)
	} else {
		g.Fill(x, y, len([]rune(k))+2, 1, cPanel2)
		g.Text(x+1, y, k, cAmber, true)
	}
	return len([]rune(k)) + 2
}

// door is one of the four choices: a frame with the key, what it does and two lines about it.
func door(g Gfx, x, y, w int, k, title, line1, line2 string, lit bool) {
	frame := cLine
	if lit {
		frame = cAmber
	}
	g.Frame(x, y, w, 5, frame)
	kw := keyCap(g, x+2, y+1, k, lit)
	g.Text(x+2+kw+2, y+1, fit(title, w-kw-6), cWhite, true)
	g.Text(x+2+kw+2, y+2, fit(line1, w-kw-6), cText, false)
	g.Text(x+2+kw+2, y+3, fit(line2, w-kw-6), cDim, false)
}

func (a *App) drawLauncher(g Gfx) {
	s := a.splash
	w := min(max(Cols-10, 70), 110)
	lx := max((Cols-w)/2, 2)
	y := 1 + max((Rows-MinRows)/2, 0)
	now := a.clock()

	shadowLogo(g, lx, y)
	g.Text(lx, y+7, "SAMPLE LAB FOR THE SP-404MKII", cText, true)
	g.Text(lx, y+8, fit("samples, slices, patterns, projects", w-38), cDim, false)
	info := "autosave " + a.autosaveSetting()
	if Version != "" {
		info = "version " + Version + "   " + info
	}
	info = fit(info, 36)
	g.Text(lx+w-len([]rune(info)), y+8, info, cDim, false)

	// the doors: Enter carries on (or restores the autosave), u the unit, o the offline folder, f a folder
	dw := (w - 2) / 2
	used := 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
		}
	}
	recent := a.recent()
	cy := y + 10
	switch {
	case s.restore != "":
		door(g, lx, cy, dw, "Enter", "Restore the autosave", fmt.Sprintf("%s, saved at %s", filepath.Base(s.restore), s.when.Format("15:04")),
			"any other key starts empty", true)
	case a.hasWork():
		door(g, lx, cy, dw, "Enter", "Continue", a.ProjectName()+"  "+bpmText(a.tempoOf(0))+" BPM",
			fmt.Sprintf("%d pads with samples · %d patterns", used, s.patterns), true)
	case len(recent) > 0:
		r := recent[0]
		door(g, lx, cy, dw, "Enter", "Continue", recentLabel(r.Dir), fmt.Sprintf("%d pads with samples · opened %s", r.Pads, ago(r.When, now)), true)
	default:
		door(g, lx, cy, dw, "Enter", "Start empty", "a new project, nothing on the pads", "samples come from FILES", true)
	}
	if a.link != nil {
		door(g, lx+dw+2, cy, dw, "u", "Unit, online", "SP-404MKII found: "+a.link.Name(), "its open project, edited live", false)
	} else {
		door(g, lx+dw+2, cy, dw, "u", "Unit, online", "no unit found yet", "it is looked for all the time", false)
	}
	off := fmt.Sprintf("%d of 16 projects there", countProjects(a.offlineDir()))
	if n, ok := a.lastOffline(); ok {
		off = fmt.Sprintf("opens PROJECT_%02d again, the last one", n+1)
	}
	door(g, lx, cy+5, dw, "o", "Offline card folder", short2(homeShort(a.offlineDir()), dw-12), off, false)
	last := "the folder chooser opens"
	if a.set.LastProjectDir != "" {
		last = "last: " + short2(homeShort(a.set.LastProjectDir), dw-18)
	}
	door(g, lx+dw+2, cy+5, dw, "f", "Open a folder", "a project, or a card backup", last, false)

	// the projects opened last, as many as there is room for
	ry := cy + 11
	if len(recent) > 0 && ry+1 < Rows-3 {
		g.Text(lx, ry, "RECENT", cAmber, true)
		g.HRule(lx+8, ry, w-8, cLine)
		for i, r := range recent {
			if i >= 4 || ry+1+i >= Rows-3 {
				break
			}
			yy := ry + 1 + i
			g.Text(lx, yy, fmt.Sprint(i+1), cAmber, true)
			nameW := max(w-42, 20)
			g.Text(lx+3, yy, fit(recentLabel(r.Dir), nameW), cText, false)
			g.Text(lx+3+nameW+2, yy, fmt.Sprintf("%3d pads", r.Pads), cDim, false)
			g.Text(lx+3+nameW+12, yy, bpmText(r.BPM)+" BPM", cDim, false)
			g.Text(lx+3+nameW+22, yy, fit(ago(r.When, now), w-nameW-25), cDim, false)
		}
	}

	// the keys
	keys := "Enter u o f  choose"
	if len(recent) > 0 {
		if n := min(len(recent), 4); n == 1 {
			keys += "    1  recent"
		} else {
			keys += fmt.Sprintf("    1-%d  recent", n)
		}
	}
	if s.restore != "" {
		keys += "    Space  start empty"
	} else {
		keys += "    any other key  carry on"
	}
	g.Text(lx, Rows-3, fit(keys, w), cDim, false)
	g.Text(Cols-2-len("moahbeatlab"), Rows-1, "moahbeatlab", cLine, false)
}
