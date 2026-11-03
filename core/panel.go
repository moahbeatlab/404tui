package core

import (
	"fmt"
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
	if a.prompt != nil {
		return []hint{{"^v<>", "choose a key"}, {"A", "type it"}, {"B", "delete"}, {"L1 R1", "keys page"}, {"X", "done"}}, []hint{{"Y+B", "clear all"}, {"Y+X", "cancel"}}
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
			[]hint{screen, {"Y+A", "mark pad"}, {"Y+^v", "bank"}, {"Y+B", "clear pad"}, {"Sel+X", "record"}}
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
			[]hint{screen, {"Y+X", "places"}, {"Y+A", "search"}, {"Y+B", "favourite"}, {"Sel+X", "record"}}
	case Slice:
		return []hint{{"A", "cut"}, {"B", "join"}, {"X", "play slice"}, {"R2", "play order"}, {"L2", "grid step"}, {"<>", "move"}, {"^v", "marker"}},
			[]hint{{"Y+A", "auto cut"}, {"Y+X", "to pads"}, {"Y+B", "settings"}, {"Sel+B", "tools"}, {"Y+^v", "zoom"}, {"Sel+X", "record"}}
	case Seq:
		if a.aHeld { // A is down: what the arrows do now
			return []hint{{"^v", "velocity"}, {"<>", "length"}, {"L1 R1", "move note"}, {map[bool]string{true: "e", false: "A"}[Terminal], "let go: done"}}, nil
		}
		play, stop := hint{"X", "play"}, hint{"B", "note settings"}
		if a.seqOn {
			play = hint{"X", "stop"}
		}
		return []hint{{"A", "note on/off"}, {"A+^<>", "edit (hold)"}, play, stop, {"L2", "hear note"}, {"R2", "mute/solo"}, {"^v<>", "pad, step"}, {"L1 R1", "bar"}},
			[]hint{{"Y+X", "add track"}, {"Y+B", "pick pattern"}, {"Sel+B", "tools"}, {"Y+^v", "pattern +-"}, {"Sel+X", "record"}}
	case Config:
		return []hint{{"^v", "setting"}, {"<>", "change"}, {"A", "edit / switch"}, {"L1 R1", "jump"}, {"B", "pads"}}, []hint{screen, {"Sel+X", "record"}}
	case Unit:
		return []hint{{"A", "project menu"}, {"X", "look again"}, {"^v", "choose"}, {"L1 R1", "page"}, {"B", "back"}}, []hint{screen, {"Sel+X", "record"}}
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
func (a *App) drawPanel(g Gfx) {
	g.Fill(panelC0, 1, panelCols, Rows-3, cPanel)
	g.VRule(panelC0, 1, Rows-3, cLine)
	x := panelC0 + 1
	w := panelCols - 2

	// the map of the screens, the current one lit: buttons of whole cells with a cell between them
	cw := w / 3
	for i, n := range screenNames {
		cx, cy := x+(i%3)*cw, 1+i/3
		bg, fg := cPanel2, cDim
		if Screen(i) == a.scr {
			bg, fg = cAmber, cBg
		}
		g.Fill(cx, cy, cw-1, 1, bg)
		g.Text(cx+(cw-1-len(n))/2, cy, n, fg, Screen(i) == a.scr)
	}

	// tempo and the state of the loop
	y := 2 + (len(screenNames)+2)/3
	g.Text(x, y, fmt.Sprintf("BPM %s", bpmText(a.tempo())), cText, true)
	g.Text(x+len("BPM "+bpmText(a.tempo())+" "), y, a.tempoSource(), cDim, false)
	if a.arpOn {
		g.Text(x+w-len("ARP"), y, "ARP", cGood, true)
	} else if a.seqOn {
		g.Text(x+w-len("PLAY"), y, "PLAY", cGood, true)
	} else {
		g.Text(x+w-len("STOP"), y, "STOP", cDim, false)
	}
	g.HRule(x, y+1, w, cLine)

	// the keys, from the bottom up
	plain, shift := a.hints()
	rows := len(plain)
	if len(shift) > 0 {
		rows += len(shift) + 1
	}
	ky := Rows - 2 - rows - 1
	a.keyBlock(g, x, ky, w, plain, !a.yHeld || len(shift) == 0)
	if len(shift) > 0 {
		sy := ky + len(plain)
		tag, bg, fg := tk("hold Y"), cPanel2, cDim
		if a.yHeld {
			tag, bg, fg = "SHIFT", cAmber, cBg
		}
		g.Fill(x, sy, w, 1, bg)
		g.Text(x+1, sy, tag, fg, a.yHeld)
		a.keyBlock(g, x, sy+1, w, shift, a.yHeld)
	}
	g.HRule(x, ky-1, w, cLine)

	// what is under the cursor, in the room between
	a.context(g, x, y+2, w, ky-1)
}

// tk writes a button's name as the key of the keyboard when the app is shown in a terminal (D-pad: h j k l, L1 R1: [ ],
// L2 R2: , . ; Y and Select are keys that hold for the next one): the panel and the hints then name what to press.
func tk(s string) string {
	if !Terminal {
		return s
	}
	switch s {
	case "A", "B", "X":
		return strings.ToLower(s)
	}
	return keyWords.Replace(s)
}

var keyWords = strings.NewReplacer(
	"A+^<>", "e hjkl", "Sel+B", "s b", "Sel+A", "s a", "Sel+X", "s x", "Y+A", "y a", "Y+B", "y b", "Y+X", "y x", "Y+", "y ", "L1 R1", "[ ]", "L1", "[", "R1", "]", "L2", ",", "R2", ".",
	"^v<>", "hjkl", "<>", "hl", "^v", "jk", "hold Y", "y, then", "<", "h", ">", "l")

// keyBlock draws key letters (amber) and what they do; dim when the other set is the one in force.
func (a *App) keyBlock(g Gfx, x, y, w int, hs []hint, active bool) {
	kc, tc := cAmber, cText
	if !active {
		kc, tc = cAmberD, cDim
	}
	const kw = 7 // the width of the key column: "e hjkl" is the longest of the keyboard's
	for i, h := range hs {
		g.Text(x, y+i, tk(h.key), kc, true)
		g.Text(x+kw, y+i, fit(h.what, w-kw), tc, false)
	}
}

// context is the panel's middle: what the screen is about, from row y0 down to y1 (cells).
func (a *App) context(g Gfx, x, y0, w, y1 int) {
	row := func(i int) int { return y0 + i }
	switch a.scr {
	case Pads:
		n := a.padN()
		p := a.info.Pads[n]
		if !p.File {
			g.Text(x, row(0), a.label(n), cAmber, true)
			g.Text(x, row(1), "empty", cDim, false)
			g.Text(x, row(2), "A: pick a sample", cDim, false)
			return
		}
		g.Text(x, row(0), a.label(n), cAmber, true)
		g.Text(x+4, row(0), fit(p.Name, w-4), cText, true)
		ch := "mono"
		if p.Ch == 2 {
			ch = "stereo"
		}
		info := fmt.Sprintf("%.2fs %s", float64(p.Frames)/48000, ch)
		g.Text(x, row(1), info, cDim, false)
		bx := x + len(info) + 1
		for _, sw := range []struct {
			id byte
			ch string
		}{{0x6A, "G"}, {0x6B, "L"}} { // G is gate, L is loop: lit when on
			on := false
			if v, err := a.lab.PadParam(n, sw.id); err == nil && v != 0 {
				on = true
			}
			g.Badge(bx, row(1), sw.ch, on)
			bx += 2
		}
		g.Fill(x, row(2), w, 2, cPanel2)
		if id, ok := a.padSample(n); ok {
			a.wave(g, id, 0, p.Frames, x, row(2), w, 2, cAmber)
		}
		get := func(id byte, def int32) int32 { return a.padValue(n, id, def) }
		// the level: a bar and its number
		lx, lw := x+4, w-4-4
		level := get(0x69, 127)
		g.Text(x, row(4), "VOL", cDim, false)
		g.Bar(lx, row(4), lw, float64(level)/127, cAmber, cPanel2)
		g.Text(x+w-len(fmt.Sprint(level)), row(4), fmt.Sprint(level), cText, false)
		// the pan: a bar from the middle, and L, C or R with its amount
		bal := get(0x78, 64)
		g.Text(x, row(5), "PAN", cDim, false)
		g.Fader(lx, row(5), lw, float64(bal-13)/102, float64(64-13)/102, true)
		pv := padPages[0][1].show(bal)
		g.Text(x+w-len(pv), row(5), pv, cText, false)
		// the envelope: attack up, hold along, release down, drawn to scale
		att, hold, rel := get(0x7D, 0), get(0x7E, 100), get(0x7F, 0)
		g.Text(x, row(6), "ENV", cDim, false)
		ew := 10
		ta := 4 + float64(att)*20/127 // widths in the little drawing
		th := 6 + float64(hold)*28/100
		tr := 4 + float64(rel)*20/127
		tot := ta + th + tr
		n2 := ew * g.BarsPerCell()
		env := make([]float64, n2)
		for i := range env {
			p := float64(i) / float64(max(n2-1, 1)) * tot
			switch {
			case p < ta:
				env[i] = p / ta
			case p < ta+th:
				env[i] = 1
			default:
				env[i] = 1 - (p-ta-th)/tr
			}
		}
		g.Fill(lx, row(6), ew, 2, cPanel2)
		g.Bars(lx, row(6), ew, 2, env, false, cAmber)
		g.Text(lx+ew+1, row(6), fit(fmt.Sprintf("%d %d %d", att, hold, rel), x+w-lx-ew-1), cDim, false)
	case Files:
		g.Text(x, row(0), "TARGET", cDim, false)
		g.Text(x+7, row(0), a.label(a.padN()), cAmber, true)
		e, ok := a.current()
		if !ok || e.dir {
			if ok && e.project {
				g.Text(x, row(2), "a project folder", cGood, false)
				g.Text(x, row(3), "A loads it", cDim, false)
			}
			return
		}
		g.Text(x, row(2), fit(filepath.Base(e.name), w), cText, true)
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			g.Text(x, row(3), fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			if _, in := a.aud[a.padN()]; in {
				g.Text(x+9, row(3), "in pattern", cGood, true)
			}
			g.Fill(x, row(4), w, 2, cPanel2)
			a.wave(g, id, 0, s.Frames(), x, row(4), w, 2, cText)
		} else {
			g.Text(x, row(3), fit(err.Error(), w), cBad, false)
		}
	case Slice:
		if a.sl == nil {
			return
		}
		g.Text(x, row(0), fmt.Sprintf("%s %s", a.label(a.spad), fit(a.info.Pads[a.spad].Name, w-4)), cAmber, true)
		g.Text(x, row(1), fmt.Sprintf("%.2fs  %d slices", float64(a.sl.Frames)/48000, len(a.sl.Slices)), cDim, false)
		g.Text(x, row(2), fmt.Sprintf("%s  zoom %dx", orderMode(a.sl), 1<<a.zoom), cDim, false)
		if sel := a.sliceAt(a.at); sel >= 0 {
			s := a.sl.Slices[sel]
			g.Text(x, row(3), fit(fmt.Sprintf("slice %d/%d  pitch %+d", sel+1, len(a.sl.Slices), s.Pitch), w), cText, true)
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
			g.Text(x, row(4), fit(extra, w), cText, false)
		}
	case Seq:
		d, ok := a.doc()
		g.Text(x, row(0), "PATTERN", cDim, false)
		g.Text(x+8, row(0), a.label(a.pat-1), cAmber, true)
		if !ok {
			g.Text(x, row(1), "empty", cDim, false)
			g.Text(x, row(2), "A: first note", cDim, false)
			return
		}
		pad := a.rowPad()
		g.Text(x, row(1), fit(fmt.Sprintf("%d.%d %s %s", a.step/16+1, a.step%16+1, a.label(pad), short(a.info.Pads[pad].Name, 8)), w), cText, false)
		if n, ok := noteAt(d, pad, a.step); ok {
			col := cAmber
			if a.aHeld {
				col = cGood
			}
			g.Text(x, row(2), fmt.Sprintf("vel %d  len %.2f", n.Vel, float64(n.Len)/stepTicks), col, true)
			off := a.noteTiming(n, pad)
			g.Text(x, row(3), fit(strings.SplitN(off, " (", 2)[0], w), cText, false)
			if n.Pitch != 0 {
				g.Text(x, row(4), fmt.Sprintf("chromatic %d", n.Pitch), cAmber, false)
			}
		} else {
			g.Text(x, row(2), "no note", cLine, false)
		}
	case Save:
		used, bytesN := 0, 0
		for _, p := range a.info.Pads {
			if p.File {
				used++
				bytesN += p.Frames * p.Ch * 2
			}
		}
		g.Text(x, row(0), fit(a.ProjectName(), w), cAmber, true)
		g.Text(x, row(1), fmt.Sprintf("%d pads", used), cText, false)
		g.Text(x, row(2), fmt.Sprintf("%.1f MB", float64(bytesN)/1e6), cText, false)
		g.Text(x, row(3), fmt.Sprintf("%d files", a.info.Files), cText, false)
	case Unit:
		a.unitContext(g, x, y0, w)
	case Config:
		a.configContext(g, x, y0, w, y1)
	}
}
