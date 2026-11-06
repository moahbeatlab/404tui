package core

import (
	"fmt"
	"image/color"
	"strings"
	"time"
)

// The PERF screen is for the stage: it is looked at from afar while the unit is played. It shows what the unit's MIDI says: the
// bar and beat of the clock and the tempo, the pads that are hit, the controllers that move (the effect knobs of the two
// buses among them), the level of the sound that is recorded; and the pattern that plays and the one that is next, so that it
// is plain what is to be muted or brought back.

func (a *App) perfKey(k Key) {
	switch {
	case k == B:
		a.go_(a.liveBackOr(Pads))
	case k == A:
		a.pickPattern("the pattern that plays now", max(a.perfNow, 1), func(n int) { a.perfNow = n; a.say("now: pattern "+a.label(n-1), false) })
	case k == X:
		a.pickPattern("the next pattern", max(a.perfNext, a.perfNow%160+1), func(n int) { a.perfNext = n; a.say("next: pattern "+a.label(n-1), false) })
	}
}

// perfShifted is a key with Y held on the PERF screen.
func (a *App) perfShifted(k Key) {
	switch k {
	case A:
		a.perfSwap()
	case X:
		a.perfNext = 0
		a.say("no next pattern", false)
	case B:
		a.askText("what BUS 1 is used for", "the unit does not tell the effect by itself", a.set.BusNotes[0], func(s string) { a.set.BusNotes[0] = s; a.saveSettings() })
	}
}

func (a *App) liveBackOr(def Screen) Screen {
	if a.liveBack == Perform {
		return def
	}
	return a.liveBack
}

// perfSwap makes the next pattern the one that plays.
func (a *App) perfSwap() {
	if a.perfNext == 0 {
		a.say("no next pattern: X chooses it", true)
		return
	}
	a.perfNow, a.perfNext = a.perfNext, 0
	a.mon.Reset()
	a.say("now: pattern "+a.label(a.perfNow-1), false)
}

// perfTracks are the samples a pattern plays, in pad order.
func (a *App) perfTracks(slot int) []string {
	if slot == 0 {
		return nil
	}
	pads := a.patternPads(slot)
	var out []string
	for _, p := range pads {
		out = append(out, a.info.Pads[p].Name)
	}
	return out
}

// patternPads are the pads that have notes in a pattern.
func (a *App) patternPads(slot int) []int {
	b, err := a.lab.PatternBytes(slot)
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, p := range patternPadsOf(b) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sortInts(out)
	return out
}

func sortInts(v []int) {
	for i := range v {
		for j := i + 1; j < len(v); j++ {
			if v[j] < v[i] {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}

// mix is a colour a fraction of the way from one to the other.
func mix(a, b color.RGBA, f float64) color.RGBA {
	return lerp(a, b, min(max(f, 0), 1))
}

func (a *App) drawPerform(g Gfx) {
	now := time.Now()
	w := contentCols - 4
	g.Text(2, 1, "PERFORM", cAmber, true)
	st := fit(a.midiStatus(), contentCols-14)
	g.Text(contentCols-2-len(st), 1, st, cDim, false)

	// the pads of the bank the unit is on: lit by how hard they were hit
	gridCols := 4 * 4
	gx := contentCols - 2 - gridCols
	bank := a.perfBank()
	g.Text(gx, 3, fmt.Sprintf("BANK %c", 'A'+bank), cDim, true)
	for i := 0; i < 16; i++ {
		x, y := gx+(i%4)*4, 4+i/4
		glow := a.mon.Glow(bank, i, now)
		bg := cPanel2
		if a.info.Pads[bank*16+i].File {
			bg = mix(cPanel2, cLine, 0.6)
		}
		g.Fill(x, y, 3, 1, mix(bg, cAmber, glow))
		g.Text(x+1-len(fmt.Sprint(i+1))/2+0, y, fmt.Sprint(i+1), mix(cDim, cBg, glow), glow > 0.5)
	}

	// the pattern that plays and the next
	rowAt := 3
	for _, p := range []struct {
		label string
		slot  int
		col   color.RGBA
	}{{"NOW ", a.perfNow, cGood}, {"NEXT", a.perfNext, cAmber}} {
		head := p.label + "  -"
		if p.slot > 0 {
			head = fmt.Sprintf("%s  %s", p.label, a.label(p.slot-1))
			if b, err := a.lab.PatternBytes(p.slot); err == nil && len(b) > 0 {
				head += fmt.Sprintf("  %d bars", patternBars(b))
			}
		}
		g.Text(2, rowAt, fit(head, gx-4), p.col, true)
		names := strings.Join(a.perfTracks(p.slot), "  ")
		if names == "" && p.slot > 0 {
			names = "no notes"
		}
		g.Text(2, rowAt+1, fit(names, gx-4), cDim, false)
		rowAt += 3
	}

	// the clock
	y := 10
	g.Text(2, y, "CLOCK", cDim, true)
	if a.clk.At.IsZero() || now.Sub(a.clk.At) > 2*time.Second {
		g.Text(10, y, "no clock", cDim, false)
	} else {
		bar, beat := a.clk.Beat()
		g.Text(10, y, fmt.Sprintf("bar %d  beat %d", bar+1, beat+1), cText, true)
		for i := 0; i < 4; i++ {
			col := cPanel2
			if i == beat && a.clk.Running {
				col = cAmber
			}
			g.Fill(26+i*3, y, 2, 1, col)
		}
		if a.mon.BPM > 0 {
			g.Text(40, y, fmt.Sprintf("%.1f BPM", a.mon.BPM), cText, false)
		}
		if !a.clk.Running {
			g.Text(w-5, y, "(stopped)", cDim, false)
		}
	}

	// the level
	y = 12
	g.Text(2, y, "LEVEL", cDim, true)
	if a.lvl != nil && a.lvl.ok {
		for c, name := range []string{"L", "R"} {
			g.Text(10, y+c, name, cText, true)
			g.Bar(12, y+c, w-30, levelFrac(a.lvl.hold[c]), levelColour(a.lvl.hold[c]), cPanel2)
			g.Text(12+w-30+1, y+c, dbText(a.lvl.hold[c]), cText, false)
		}
		if a.lvl.clipping(now) {
			g.Fill(w-6, y, 6, 1, cBad)
			g.Text(w-5, y, "CLIP", cBg, true)
		} else {
			g.Text(w-4, y, "ok", cDim, false)
		}
	} else {
		g.Text(10, y, "no level: needs a recorder that listens (ffmpeg or sox) and the unit's audio", cDim, false)
	}

	// the two buses: the effect the user noted, and the knobs C1 to C3 as the unit says them
	y = 15
	for bus := 1; bus <= 2; bus++ {
		col := a.busColour(bus)
		on := a.mon.CCValue(bus, 19)
		state := "  "
		if on >= 64 {
			state = "ON"
		}
		g.Text(2, y, fmt.Sprintf("BUS %d", bus), col, true)
		g.Text(9, y, state, cGood, true)
		note := a.set.BusNotes[bus-1]
		if note == "" {
			note = "-"
		}
		g.Text(13, y, fit(note, w-14), cText, false)
		for k := 0; k < 3; k++ {
			v := a.mon.CCValue(bus, 16+k)
			g.Text(2+k*16, y+1, fmt.Sprintf("C%d", k+1), cDim, false)
			g.Bar(5+k*16, y+1, 9, float64(max(v, 0))/127, col, cPanel2)
		}
		y += 3
	}

	// every other controller seen, as a fader with its value
	g.Text(2, y, "CONTROLS", cDim, true)
	n := 0
	for _, c := range a.mon.Controls() {
		if (c.Chan == 1 || c.Chan == 2) && (c.Num == 83 || c.Num == 19 || (c.Num >= 16 && c.Num <= 18)) {
			continue
		}
		if y+1+n >= Rows-3 {
			break
		}
		g.Text(2, y+1+n, fmt.Sprintf("%2d:%-3d", c.Chan, c.Num), cDim, false)
		g.Bar(10, y+1+n, w-22, float64(c.Val)/127, cAmber, cPanel2)
		g.Text(10+w-21, y+1+n, fmt.Sprint(c.Val), cText, false)
		n++
	}
	if n == 0 {
		g.Text(12, y, "nothing has been turned yet", cDim, false)
	}
}

// perfBank is the bank the unit was last heard in (the pads that were hit), else the bank of the editor.
func (a *App) perfBank() int {
	best, bestAt := a.bank, time.Time{}
	for b := 0; b < 10; b++ {
		for p := 0; p < 16; p++ {
			if t, ok := a.mon.HitAt(b, p); ok && t.After(bestAt) {
				best, bestAt = b, t
			}
		}
	}
	return best
}
