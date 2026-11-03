package handheld

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
)

// The right of the screen is a panel that is always there, as on the M8: where you are among the screens, the tempo,
// what is under the cursor, and the keys that work right now with their letters. It is 30% of the width (at least 24
// cells); the screens use the rest, up to contentW. Row 0 is the title, the last two rows the message and the hints.
var (
	panelCols, panelC0 int // the panel's width and first column, in cells
	contentCols        int // the width the screens have, in cells
	panelX, panelW     int // the same in pixels
	contentW           int
	panelTop, panelBot int // the rows of the panel, in pixels: from under the title to above the footer
	waveCols           int // the slicer's waveform, one column per pixel
)

func init() { layoutScreen() }

// layoutScreen works the geometry out from the size of the screen.
func layoutScreen() {
	panelCols = max(Cols*30/100, 24)
	panelC0 = Cols - panelCols
	contentCols = panelC0
	panelX, panelW = panelC0*CellW, panelCols*CellW
	contentW = panelX
	panelTop = CellH
	panelBot = H - 2*CellH
	waveCols = contentW - 2*margin
}

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
	if a.np != nil {
		return []hint{{"^v", "setting"}, {"<>", "change"}, {"A", "default"}, {"X", "hear note"}, {"R1", "pattern tools"}, {"B", "close"}}, nil
	}
	if a.picker != nil {
		open := "open it"
		if a.picker.pads {
			open = "this pad"
		}
		return []hint{{"^v<>", "choose"}, {"L1 R1", "bank"}, {"A", open}, {"B", "cancel"}}, nil
	}
	if a.menu != nil {
		if a.menu.tabs != nil {
			return []hint{{"^v", "choose"}, {"<>", "change"}, {"A", "run"}, {"L1 R1", "tab"}, {"B", "close"}}, nil
		}
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
		return []hint{{"A", "cut"}, {"B", "join"}, {"X", "play slice"}, {"R2", "play order"}, {"L2", "grid step"}, {"<>", "move"}, {"^v", "marker"}},
			[]hint{{"Y+A", "auto cut"}, {"Y+X", "to pads"}, {"Y+B", "settings"}, {"Sel+B", "tools"}, {"Y+^v", "zoom"}}
	case Seq:
		if a.aHeld { // A is down: what the arrows do now
			return []hint{{"^v", "velocity"}, {"<>", "length"}, {"L1 R1", "move note"}, {"A", "let go: done"}}, nil
		}
		play, stop := hint{"X", "play"}, hint{"B", "note settings"}
		if a.seqOn {
			play = hint{"X", "stop"}
		}
		return []hint{{"A", "note on/off"}, {"A+^<>", "edit (hold)"}, play, stop, {"L2", "hear note"}, {"R2", "mute/solo"}, {"^v<>", "pad, step"}, {"L1 R1", "bar"}},
			[]hint{{"Y+X", "add track"}, {"Y+B", "pick pattern"}, {"Sel+B", "tools"}, {"Y+^v", "pattern +-"}}
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
	x := panelX + CellW
	w := panelW - 2*CellW

	// the map of the screens, the current one lit
	cw := w / 3
	for i, n := range screenNames {
		cx, cy := x+(i%3)*cw, panelTop+(i/3)*CellH
		bg, fg := cPanel2, cDim
		if Screen(i) == a.scr {
			bg, fg = cAmber, cBg
		}
		c.rect(cx, cy+1, cw-3, CellH-2, bg)
		c.text(cx+(cw-3-textW(n))/2, cy, n, fg, Screen(i) == a.scr)
	}

	// tempo and the state of the loop
	y := panelTop + 3*CellH
	c.text(x, y, fmt.Sprintf("BPM %s", bpmText(a.tempo())), cText, true)
	c.text(x+textW("BPM "+bpmText(a.tempo())+" "), y, a.tempoSource(), cDim, false)
	if a.arpOn {
		c.text(x+w-textW("ARP"), y, "ARP", cGood, true)
	} else if a.seqOn {
		c.text(x+w-textW("PLAY"), y, "PLAY", cGood, true)
	} else {
		c.text(x+w-textW("STOP"), y, "STOP", cDim, false)
	}
	c.rect(x, y+CellH+7, w, 1, cLine)

	// the keys, from the bottom up
	plain, shift := a.hints()
	rows := len(plain)
	if len(shift) > 0 {
		rows += len(shift) + 1
	}
	ky := panelBot - (rows+1)*CellH
	a.keyBlock(c, x, ky, w, plain, !a.yHeld || len(shift) == 0)
	if len(shift) > 0 {
		sy := ky + len(plain)*CellH
		tag, bg, fg := "hold Y", cPanel2, cDim
		if a.yHeld {
			tag, bg, fg = "SHIFT", cAmber, cBg
		}
		c.rect(x, sy, w, CellH, bg)
		c.text(x+4, sy, tag, fg, a.yHeld)
		a.keyBlock(c, x, sy+CellH, w, shift, a.yHeld)
	}
	c.rect(x, ky-5, w, 1, cLine)

	// what is under the cursor, in the room between
	a.context(c, x, y+2*CellH, w, ky-8)
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

// context is the panel's middle: what the screen is about, from y0 down to y1, a row (16 px) at a time.
func (a *App) context(c *canvas, x, y0, w, y1 int) {
	row := func(i int) int { return y0 + i*CellH }
	switch a.scr {
	case Pads:
		n := a.padN()
		p := a.info.Pads[n]
		if !p.File {
			c.text(x, row(0), a.label(n), cAmber, true)
			c.text(x, row(1), "empty", cDim, false)
			c.text(x, row(2), "A: pick a sample", cDim, false)
			return
		}
		c.text(x, row(0), a.label(n), cAmber, true)
		c.text(x+textW("A00 "), row(0), fit(p.Name, w/CellW-4), cText, true)
		ch := "mono"
		if p.Ch == 2 {
			ch = "stereo"
		}
		info := fmt.Sprintf("%.2fs %s", float64(p.Frames)/48000, ch)
		c.text(x, row(1), info, cDim, false)
		bx := x + textW(info) + 10
		for _, sw := range []struct {
			id byte
			ch string
		}{{0x6A, "G"}, {0x6B, "L"}} { // G is gate, L is loop: lit when on
			on := false
			if v, err := a.lab.PadParam(n, sw.id); err == nil && v != 0 {
				on = true
			}
			a.badge(c, bx, row(1), sw.ch, on)
			bx += 18
		}
		c.rect(x, row(2), w, 2*CellH, cPanel2)
		if id, ok := a.padSample(n); ok {
			a.waveform(c, id, 0, p.Frames, x+1, row(2), w-2, 2*CellH, cAmber)
		}
		get := func(id byte, def int32) int32 { return a.padValue(n, id, def) }
		// the level: a bar and its number
		lx, lw := x+4*CellW, w-4*CellW-4*CellW
		level := get(0x69, 127)
		c.text(x, row(4), "VOL", cDim, false)
		c.rect(lx, row(4)+4, lw, 8, cPanel2)
		c.rect(lx, row(4)+4, int(level)*lw/127, 8, cAmber)
		c.text(x+w-textW(fmt.Sprint(level)), row(4), fmt.Sprint(level), cText, false)
		// the pan: a bar from the middle, and L, C or R with its amount
		bal := get(0x78, 64)
		c.text(x, row(5), "PAN", cDim, false)
		c.rect(lx, row(5)+4, lw, 8, cPanel2)
		mid := lx + lw*(64-13)/102
		pos := lx + int(bal-13)*lw/102
		c.rect(min(mid, pos), row(5)+4, abs(pos-mid)+2, 8, cAmber)
		c.rect(mid, row(5)+2, 1, 12, cDim)
		c.text(x+w-textW(padPages[0][1].show(bal)), row(5), padPages[0][1].show(bal), cText, false)
		// the envelope: attack up, hold along, release down, drawn to scale
		att, hold, rel := get(0x7D, 0), get(0x7E, 100), get(0x7F, 0)
		c.text(x, row(6), "ENV", cDim, false)
		ex, ew, ey, eh := lx, 10*CellW, row(6), 2*CellH-2
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
		c.text(ex+ew+8, row(6), fit(fmt.Sprintf("%d %d %d", att, hold, rel), (x+w-ex-ew-8)/CellW), cDim, false)
	case Files:
		c.text(x, row(0), "TARGET", cDim, false)
		c.text(x+textW("TARGET ")+8, row(0), a.label(a.padN()), cAmber, true)
		e, ok := a.current()
		if !ok || e.dir {
			if ok && e.project {
				c.text(x, row(2), "a project folder", cGood, false)
				c.text(x, row(3), "A loads it", cDim, false)
			}
			return
		}
		c.text(x, row(2), fit(filepath.Base(e.name), w/CellW), cText, true)
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			c.text(x, row(3), fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			if _, in := a.aud[a.padN()]; in {
				c.text(x+textW("00.00 s  "), row(3), "in pattern", cGood, true)
			}
			c.rect(x, row(4), w, 2*CellH, cPanel2)
			a.waveform(c, id, 0, s.Frames(), x+1, row(4), w-2, 2*CellH, cText)
		} else {
			c.text(x, row(3), fit(err.Error(), w/CellW), cBad, false)
		}
	case Slice:
		if a.sl == nil {
			return
		}
		c.text(x, row(0), fmt.Sprintf("%s %s", a.label(a.spad), fit(a.info.Pads[a.spad].Name, w/CellW-4)), cAmber, true)
		c.text(x, row(1), fmt.Sprintf("%.2fs  %d slices", float64(a.sl.Frames)/48000, len(a.sl.Slices)), cDim, false)
		c.text(x, row(2), fmt.Sprintf("%s  zoom %dx", orderMode(a.sl), 1<<a.zoom), cDim, false)
		if sel := a.sliceAt(a.at); sel >= 0 {
			s := a.sl.Slices[sel]
			c.text(x, row(3), fit(fmt.Sprintf("slice %d/%d  pitch %+d", sel+1, len(a.sl.Slices), s.Pitch), w/CellW), cText, true)
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
			c.text(x, row(4), fit(extra, w/CellW), cText, false)
		}
	case Seq:
		d, ok := a.doc()
		c.text(x, row(0), "PATTERN", cDim, false)
		c.text(x+textW("PATTERN "), row(0), a.label(a.pat-1), cAmber, true)
		if !ok {
			c.text(x, row(1), "empty", cDim, false)
			c.text(x, row(2), "A: first note", cDim, false)
			return
		}
		pad := a.rowPad()
		c.text(x, row(1), fit(fmt.Sprintf("%d.%d %s %s", a.step/16+1, a.step%16+1, a.label(pad), short(a.info.Pads[pad].Name, 8)), w/CellW), cText, false)
		if n, ok := noteAt(d, pad, a.step); ok {
			col := cAmber
			if a.aHeld {
				col = cGood
			}
			c.text(x, row(2), fmt.Sprintf("vel %d  len %.2f", n.Vel, float64(n.Len)/stepTicks), col, true)
			off := a.noteTiming(n, pad)
			c.text(x, row(3), fit(strings.SplitN(off, " (", 2)[0], w/CellW), cText, false)
			if n.Pitch != 0 {
				c.text(x, row(4), fmt.Sprintf("chromatic %d", n.Pitch), cAmber, false)
			}
		} else {
			c.text(x, row(2), "no note", cLine, false)
		}
	case Save:
		used, bytesN := 0, 0
		for _, p := range a.info.Pads {
			if p.File {
				used++
				bytesN += p.Frames * p.Ch * 2
			}
		}
		c.text(x, row(0), fit(a.ProjectName(), w/CellW), cAmber, true)
		c.text(x, row(1), fmt.Sprintf("%d pads", used), cText, false)
		c.text(x, row(2), fmt.Sprintf("%.1f MB", float64(bytesN)/1e6), cText, false)
		c.text(x, row(3), fmt.Sprintf("%d files", a.info.Files), cText, false)
	case Unit:
		a.unitContext(c, x, y0, w)
	}
}

var _ color.RGBA

// badge is a letter in a little box: lit (amber) when the switch is on, dim when it is off.
func (a *App) badge(c *canvas, x, y int, letter string, on bool) {
	if on {
		c.rect(x, y, 14, 16, cAmber)
		c.text(x+3, y, letter, cBg, true)
		return
	}
	c.box(x, y, 14, 16, 1, cLine)
	c.text(x+3, y, letter, cLine, false)
}
