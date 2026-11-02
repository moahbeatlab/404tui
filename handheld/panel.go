package handheld

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
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
	if a.pp != nil {
		if a.pp.page == 2 {
			return []hint{{"^v", "choose"}, {"<>", "change"}, {"A", "run"}, {"L1 R1", "page"}, {"B", "close"}}, nil
		}
		return []hint{{"^v", "setting"}, {"<>", "change"}, {"A", "reset/toggle"}, {"X", "hear pad"}, {"L1 R1", "page"}, {"B", "close"}}, nil
	}
	if a.picker != nil {
		return []hint{{"^v<>", "choose"}, {"L1 R1", "bank"}, {"A", "open it"}, {"B", "cancel"}}, nil
	}
	if a.menu != nil {
		return []hint{{"^v", "choose"}, {"<>", "change"}, {"A", "run"}, {"B", "close"}}, nil
	}
	switch a.scr {
	case Pads:
		if a.carry != nil {
			return []hint{{"A", "put it here"}, {"B", "cancel"}, {"^v<>", "move"}}, []hint{screen}
		}
		return []hint{{"A", "menu"}, {"X", "play"}, {"B", "stop"}, {"^v<>", "move"}, {"L1 R1", "bank"}, {"R2", "arp marked"}},
			[]hint{screen, {"Y+A", "mark pad"}, {"Y+^v", "bank"}, {"Y+B", "clear pad"}}
	case Files:
		if a.kb != nil {
			return []hint{{"^v<>", "move"}, {"A", "type it"}, {"B", "delete"}, {"X", "done"}}, []hint{{"Y+B", "clear all"}}
		}
		pick := hint{"A", "to pad " + a.label(a.padN())}
		if e, ok := a.current(); ok && e.project {
			pick = hint{"A", "load project"}
		}
		listen := hint{"X", "listen"}
		if a.seqOn && a.padInPattern(a.padN()) {
			listen = hint{"X", "in pattern"}
		}
		up := hint{"<", "up"}
		if a.searchQ != "" {
			up = hint{"<", "end search"}
		}
		return []hint{pick, listen, {"^v", "browse"}, {">", "open"}, up, {"L1 R1", "page"}, {"B", "pads"}},
			[]hint{screen, {"Y+X", "places"}, {"Y+A", "search"}, {"Y+B", "favourite"}}
	case Slice:
		return []hint{{"A", "cut"}, {"B", "join"}, {"X", "play slice"}, {"R2", "play order"}, {"<>", "move"}, {"^v", "marker"}, {"L1 R1", "jump 1/4"}},
			[]hint{screen, {"Y+A", "auto cut"}, {"Y+X", "to pads"}, {"Y+B", "slice menu"}, {"Y+^v", "zoom"}}
	case Seq:
		if a.aHeld { // A is down: what the arrows do now
			return []hint{{"^v", "velocity"}, {"<>", "length"}, {"L1 R1", "move note"}, {"A", "let go: done"}}, nil
		}
		play, stop := hint{"X", "play"}, hint{"B", "menu"}
		if a.seqOn {
			play = hint{"X", "stop"}
		}
		return []hint{{"A", "note on/off"}, {"A+^<>", "edit (hold)"}, play, stop, {"L2", "hear note"}, {"R2", "mute/solo"}, {"^v<>", "pad, step"}, {"L1 R1", "bar"}},
			[]hint{screen, {"Y+X", "add track"}, {"Y+B", "pick pattern"}, {"Y+^v", "pattern +-"}}
	case Unit:
		return []hint{{"A", "send"}, {"X", "look again"}, {"^v", "choose"}, {"L1 R1", "page"}, {"B", "back"}}, []hint{screen}
	default:
		return []hint{{"A", "write"}, {"<>", "slot"}, {"^v", "autosave"}, {"L1 R1", "slot 10"}, {"X", "quit"}, {"B", "back"}}, []hint{screen}
	}
}

// tempoOf is the tempo that plays in a bank (0..9): the project's when the project tempo is selected, else the bank's own.
// An unreadable value falls back to 120.
func (a *App) tempoOf(bank int) float64 {
	t := a.info.Tempo
	if !a.info.UsesProjectTempo && bank >= 0 && bank < len(a.info.BankTempos) {
		t = a.info.BankTempos[bank]
	}
	if t < 30 || t > 300 {
		return 120
	}
	return t
}

// tempo is the tempo of the pattern in front: that of its bank.
func (a *App) tempo() float64 { return a.tempoOf((a.pat - 1) / 16) }

// tempoSource says whose tempo it is, for the panel.
func (a *App) tempoSource() string {
	if a.info.UsesProjectTempo {
		return "proj"
	}
	return "bank " + string(rune('A'+(a.pat-1)/16))
}

func bpmText(t float64) string {
	if t == float64(int(t)) {
		return fmt.Sprintf("%d", int(t))
	}
	return fmt.Sprintf("%.1f", t)
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
	c.text(x, y, fmt.Sprintf("BPM %s", bpmText(a.tempo())), cText, true)
	c.text(x+textW("BPM "+bpmText(a.tempo())+" "), y, a.tempoSource(), cDim, false)
	if a.arpOn {
		c.text(x+w-textW("ARP"), y, "ARP", cGood, true)
	} else if a.seqOn {
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
		c.text(x+2+textW("L1 R1")+6, y+i*16, fit(h.what, (w-textW("L1 R1")-10)/CellW), tc, false)
	}
}

// context is the panel's middle: what the screen is about, from y0 down to y1.
func (a *App) context(c *canvas, x, y0, w, y1 int) {
	switch a.scr {
	case Pads:
		n := a.padN()
		p := a.info.Pads[n]
		if !p.File {
			c.big(x, y0, a.label(n), cAmber, 2)
			c.text(x, y0+40, "empty", cDim, false)
			c.text(x, y0+58, "A: pick a sample", cDim, false)
			return
		}
		c.text(x, y0, a.label(n), cAmber, true)
		c.text(x+textW("A00 "), y0, fit(p.Name, w/CellW-4), cText, true)
		ch := "mono"
		if p.Ch == 2 {
			ch = "stereo"
		}
		tags := ""
		for _, sw := range []struct {
			id  byte
			tag string
		}{{0x6A, " GATE"}, {0x6B, " LOOP"}} {
			if v, err := a.lab.PadParam(n, sw.id); err == nil && v != 0 {
				tags += sw.tag
			}
		}
		c.text(x, y0+18, fmt.Sprintf("%.2fs %s", float64(p.Frames)/48000, ch), cDim, false)
		c.text(x+textW(fmt.Sprintf("%.2fs %s", float64(p.Frames)/48000, ch)), y0+18, tags, cAmber, true)
		c.rect(x, y0+36, w, 24, cPanel2)
		if id, ok := a.padSample(n); ok {
			a.waveform(c, id, 0, p.Frames, x+1, y0+36, w-2, 24, cAmber)
		}
		get := func(id byte, def int32) int32 { return a.padValue(n, id, def) }
		// the level: a bar and its number
		level := get(0x69, 127)
		c.text(x, y0+64, "VOL", cDim, false)
		c.rect(x+28, y0+68, w-28-30, 8, cPanel2)
		c.rect(x+28, y0+68, int(level)*(w-28-30)/127, 8, cAmber)
		c.text(x+w-textW(fmt.Sprint(level)), y0+64, fmt.Sprint(level), cText, false)
		// the pan: a bar from the middle, and L, C or R with its amount
		bal := get(0x78, 64)
		c.text(x, y0+80, "PAN", cDim, false)
		bx, bw := x+28, w-28-30
		c.rect(bx, y0+84, bw, 8, cPanel2)
		mid := bx + bw*(64-13)/102
		pos := bx + int(bal-13)*bw/102
		c.rect(min(mid, pos), y0+84, abs(pos-mid)+2, 8, cAmber)
		c.rect(mid, y0+82, 1, 12, cDim)
		c.text(x+w-textW(padPages[0][1].show(bal)), y0+80, padPages[0][1].show(bal), cText, false)
		// the envelope: attack up, hold along, release down, drawn to scale
		att, hold, rel := get(0x7D, 0), get(0x7E, 100), get(0x7F, 0)
		c.text(x, y0+98, "ENV", cDim, false)
		ex, ew, ey, eh := x+28, 64, y0+98, 18
		c.rect(ex, ey, ew, eh, cPanel2)
		ta := 4 + int(att)*20/127 // widths in the little drawing
		th := 6 + int(hold)*28/100
		tr := 4 + int(rel)*20/127
		tot := ta + th + tr
		x1 := ex + ta*ew/tot
		x2 := x1 + th*ew/tot
		for dx := 0; dx < ew; dx++ {
			px := ex + dx
			var f float64
			switch {
			case px < x1:
				f = float64(px-ex) / float64(max(x1-ex, 1))
			case px < x2:
				f = 1
			default:
				f = 1 - float64(px-x2)/float64(max(ex+ew-x2, 1))
			}
			hh := int(f * float64(eh-2))
			c.rect(px, ey+eh-1-hh, 1, hh+1, cAmber)
		}
		c.text(ex+ew+8, y0+98, fmt.Sprintf("%d %d %d", att, hold, rel), cDim, false)
	case Files:
		c.text(x, y0, "TARGET", cDim, false)
		c.big(x+textW("TARGET")+8, y0-4, a.label(a.padN()), cAmber, 2)
		e, ok := a.current()
		if !ok || e.dir {
			if ok && e.project {
				c.text(x, y0+36, "a project folder", cGood, false)
				c.text(x, y0+54, "A loads it", cDim, false)
			}
			return
		}
		c.text(x, y0+34, fit(filepath.Base(e.name), w/CellW), cText, true)
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			c.text(x, y0+52, fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			if _, in := a.aud[a.padN()]; in {
				c.text(x+textW("00.00 s  "), y0+52, "in pattern", cGood, true)
			}
			c.rect(x, y0+70, w, 32, cPanel2)
			a.waveform(c, id, 0, s.Frames(), x+1, y0+70, w-2, 32, cText)
		} else {
			c.text(x, y0+52, fit(err.Error(), w/CellW), cBad, false)
		}
	case Slice:
		if a.sl == nil {
			return
		}
		c.text(x, y0, fmt.Sprintf("%s %s", a.label(a.spad), fit(a.info.Pads[a.spad].Name, w/CellW-4)), cAmber, true)
		c.text(x, y0+18, fmt.Sprintf("%.2fs  %d slices", float64(a.sl.Frames)/48000, len(a.sl.Slices)), cDim, false)
		c.text(x, y0+36, fmt.Sprintf("%s  zoom %dx", orderMode(a.sl), 1<<a.zoom), cDim, false)
		if sel := a.sliceAt(a.at); sel >= 0 {
			s := a.sl.Slices[sel]
			c.text(x, y0+54, fmt.Sprintf("slice %d/%d  pitch %+d", sel+1, len(a.sl.Slices), s.Pitch), cText, true)
			extra := fmt.Sprintf("gain %d%% ", s.Gain)
			if s.Reverse {
				extra += "rev "
			}
			if s.Attack > 0 || s.Release > 0 {
				extra += fmt.Sprintf("fade %d/%d ", s.Attack, s.Release)
			}
			if s.Roll > 0 {
				extra += fmt.Sprintf("roll %d", s.Roll)
			}
			c.text(x, y0+72, fit(extra, w/CellW), cText, false)
		}
	case Seq:
		d, ok := a.doc()
		c.text(x, y0, "PATTERN", cDim, false)
		c.big(x+textW("PATTERN")+8, y0-4, a.label(a.pat-1), cAmber, 2)
		if !ok {
			c.text(x, y0+40, "empty", cDim, false)
			c.text(x, y0+58, "A: first note", cDim, false)
			return
		}
		pad := a.rowPad()
		c.text(x, y0+36, fmt.Sprintf("%d.%d %s %s", a.step/16+1, a.step%16+1, a.label(pad), short(a.info.Pads[pad].Name, 8)), cText, false)
		if n, ok := noteAt(d, pad, a.step); ok {
			col := cAmber
			if a.aHeld {
				col = cGood
			}
			c.text(x, y0+54, fmt.Sprintf("vel %d  len %.2f", n.Vel, float64(n.Len)/stepTicks), col, true)
			off := a.noteTiming(n, pad)
			c.text(x, y0+72, fit(strings.SplitN(off, " (", 2)[0], w/CellW), cText, false)
			if n.Pitch != 0 {
				c.text(x, y0+90, fmt.Sprintf("chromatic %d", n.Pitch), cAmber, false)
			} else if a.info.Pads[pad].File {
				c.text(x, y0+90, "plain note", cDim, false)
			}
		} else {
			c.text(x, y0+54, "no note", cLine, false)
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
