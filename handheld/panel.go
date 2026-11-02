package handheld

import (
	"fmt"
	"image/color"
)

// The right of the screen is a panel that is always there, as on the M8: where you are among the screens, the tempo,
// what is under the cursor, and the keys that work right now with their letters. The screens use the rest, up to
// contentW.
const (
	panelX   = 468
	panelW   = W - panelX
	contentW = panelX - 4
	panelTop = 29
	panelBot = 436
)

type hint struct{ key, what string }

// hints are the keys of the screen: the plain ones and the ones with Y held.
func (a *App) hints() (plain, shift []hint) {
	screen := hint{"Y+<>", "screen"}
	if a.menu != nil {
		return []hint{{"^v", "choose"}, {"<>", "change"}, {"A", "run"}, {"B", "close"}}, nil
	}
	switch a.scr {
	case Pads:
		if a.carry != nil {
			return []hint{{"A", "put it here"}, {"B", "cancel"}, {"^v<>", "move"}}, []hint{screen}
		}
		return []hint{{"A", "menu"}, {"X", "play"}, {"B", "stop"}, {"^v<>", "move"}},
			[]hint{screen, {"Y+^v", "bank"}, {"Y+B", "clear pad"}}
	case Files:
		pick := hint{"A", "to pad " + a.label(a.padN())}
		if e, ok := a.current(); ok && e.project {
			pick = hint{"A", "load project"}
		}
		return []hint{pick, {"X", "listen"}, {"^v", "browse"}, {">", "open"}, {"<", "up"}, {"B", "pads"}},
			[]hint{screen, {"Y+X", "places"}}
	case Slice:
		return []hint{{"A", "cut"}, {"B", "join"}, {"X", "play"}, {"<>", "move"}, {"^v", "marker"}},
			[]hint{screen, {"Y+A", "auto cut"}, {"Y+X", "to pads"}, {"Y+^v", "zoom"}}
	case Seq:
		play, stop := hint{"X", "play"}, hint{"B", "menu"}
		if a.seqOn {
			play, stop = hint{"X", "stop"}, hint{"B", "stop"}
		}
		return []hint{{"A", "note on/off"}, play, stop, {"^v", "step"}, {"<>", "column"}},
			[]hint{screen, {"Y+A", "velocity"}, {"Y+X", "column pad"}, {"Y+^v", "pattern"}}
	case Unit:
		return []hint{{"A", "send"}, {"X", "look again"}, {"^v", "choose"}, {"B", "back"}}, []hint{screen}
	default:
		return []hint{{"A", "write"}, {"<>", "slot"}, {"X", "quit"}, {"B", "back"}}, []hint{screen}
	}
}

func (a *App) tempo() float64 {
	if a.info.Tempo < 30 || a.info.Tempo > 300 {
		return 120
	}
	return a.info.Tempo
}

// drawPanel draws the panel over the right of the frame.
func (a *App) drawPanel(c *canvas) {
	c.rect(panelX, panelTop, panelW, panelBot-panelTop, cPanel)
	c.vline(panelX-1, panelTop, panelBot, cLine)
	x := panelX + 8
	w := panelW - 16

	// the map of the screens, the current one lit
	cw, ch, gap := (w-2*4)/3, 18, 4
	for i, n := range screenNames {
		cx, cy := x+(i%3)*(cw+gap), panelTop+8+(i/3)*(ch+gap)
		bg, fg := cPanel2, cDim
		if Screen(i) == a.scr {
			bg, fg = cAmber, cBg
		}
		c.rect(cx, cy, cw, ch, bg)
		c.text(cx+(cw-textW(n))/2, cy+1, n, fg, Screen(i) == a.scr)
	}

	// tempo and the state of the loop
	y := panelTop + 8 + 2*(ch+gap) + 4
	c.text(x, y, fmt.Sprintf("BPM %.0f", a.tempo()), cText, false)
	if a.seqOn {
		c.text(x+w-textW("PLAY"), y, "PLAY", cGood, true)
	} else {
		c.text(x+w-textW("STOP"), y, "STOP", cDim, false)
	}
	y += 22
	c.rect(x, y-4, w, 1, cLine)

	// the keys, from the bottom up
	plain, shift := a.hints()
	rows := len(plain)
	if len(shift) > 0 {
		rows += len(shift) + 2
	}
	ky := panelBot - 8 - rows*16
	a.keyBlock(c, x, ky, w, plain, !a.yHeld || len(shift) == 0)
	if len(shift) > 0 {
		sy := ky + len(plain)*16 + 4
		tag, bg, fg := "hold Y", cPanel2, cDim
		if a.yHeld {
			tag, bg, fg = "SHIFT", cAmber, cBg
		}
		c.rect(x, sy, w, 15, bg)
		c.text(x+4, sy, tag, fg, a.yHeld)
		a.keyBlock(c, x, sy+18, w, shift, a.yHeld)
	}
	c.rect(x, ky-6, w, 1, cLine)

	// what is under the cursor, in the room between
	a.context(c, x, y+2, w, ky-8)
}

// keyBlock draws key letters (amber) and what they do; dim when the other set is the one in force.
func (a *App) keyBlock(c *canvas, x, y, w int, hs []hint, active bool) {
	kc, tc := cAmber, cText
	if !active {
		kc, tc = cAmberD, cDim
	}
	for i, h := range hs {
		c.text(x+2, y+i*16, h.key, kc, true)
		c.text(x+2+textW("Y+^v")+8, y+i*16, fit(h.what, (w-textW("Y+^v")-12)/CellW), tc, false)
	}
}

// context is the panel's middle: what the screen is about, from y0 down to y1.
func (a *App) context(c *canvas, x, y0, w, y1 int) {
	switch a.scr {
	case Pads:
		n := a.padN()
		p := a.info.Pads[n]
		c.big(x, y0, a.label(n), cAmber, 2)
		if !p.File {
			c.text(x, y0+40, "empty", cDim, false)
			c.text(x, y0+58, "A: pick a sample", cDim, false)
			return
		}
		c.text(x, y0+38, fit(p.Name, w/CellW), cText, true)
		ch := "mono"
		if p.Ch == 2 {
			ch = "stereo"
		}
		c.text(x, y0+56, fmt.Sprintf("%.2fs %s lvl %d", float64(p.Frames)/48000, ch, p.Level), cDim, false)
		c.rect(x, y0+76, w, 44, cPanel2)
		if id, ok := a.padSample(n); ok {
			a.waveform(c, id, 0, p.Frames, x+1, y0+76, w-2, 44, cAmber)
		}
		used := 0
		for _, q := range a.info.Pads {
			if q.File {
				used++
			}
		}
		c.text(x, y0+126, fmt.Sprintf("%d of 160 pads", used), cDim, false)
	case Files:
		c.text(x, y0, "TARGET", cDim, false)
		c.big(x+textW("TARGET")+8, y0-4, a.label(a.padN()), cAmber, 2)
		e, ok := a.current()
		if !ok || e.dir {
			if ok && e.project {
				c.text(x, y0+40, "a project folder", cGood, false)
				c.text(x, y0+58, "A loads it", cDim, false)
			}
			return
		}
		c.text(x, y0+40, fit(e.name, w/CellW), cText, true)
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			c.text(x, y0+58, fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			c.rect(x, y0+78, w, 44, cPanel2)
			a.waveform(c, id, 0, s.Frames(), x+1, y0+78, w-2, 44, cText)
		} else {
			c.text(x, y0+58, fit(err.Error(), w/CellW), cBad, false)
		}
	case Slice:
		if a.sl == nil {
			return
		}
		c.big(x, y0, a.label(a.spad), cAmber, 2)
		c.text(x, y0+38, fit(a.info.Pads[a.spad].Name, w/CellW), cText, true)
		c.text(x, y0+58, fmt.Sprintf("%.2f s", float64(a.sl.Frames)/48000), cDim, false)
		c.text(x, y0+76, fmt.Sprintf("%d slices", len(a.sl.Slices)), cText, false)
		c.text(x, y0+94, fmt.Sprintf("zoom %dx", 1<<a.zoom), cDim, false)
	case Seq:
		d, ok := a.doc()
		c.text(x, y0, "PATTERN", cDim, false)
		c.big(x+textW("PATTERN")+8, y0-4, a.label(a.pat-1), cAmber, 2)
		if !ok {
			c.text(x, y0+40, "empty", cDim, false)
			c.text(x, y0+58, "A: first note", cDim, false)
			return
		}
		c.text(x, y0+40, fmt.Sprintf("%d bars  %d steps", d.Bars, a.steps()), cText, false)
		c.text(x, y0+58, fmt.Sprintf("step %d.%d", a.step/16+1, a.step%16+1), cText, false)
		pad := a.colPad()
		c.text(x, y0+76, fmt.Sprintf("%s %s", a.label(pad), short(a.info.Pads[pad].Name, 11)), cDim, false)
		if n, ok := noteAt(d, pad, a.step); ok {
			c.text(x, y0+94, fmt.Sprintf("vel %02X", n.Vel), cAmber, true)
		}
	case Save:
		used, bytesN := 0, 0
		for _, p := range a.info.Pads {
			if p.File {
				used++
				bytesN += p.Frames * p.Ch * 2
			}
		}
		c.text(x, y0, a.ProjectName(), cAmber, true)
		c.text(x, y0+22, fmt.Sprintf("%d pads", used), cText, false)
		c.text(x, y0+40, fmt.Sprintf("%.1f MB", float64(bytesN)/1e6), cText, false)
		c.text(x, y0+58, fmt.Sprintf("%d files", a.info.Files), cText, false)
	case Unit:
		a.unitContext(c, x, y0, w)
	}
}

var _ color.RGBA
