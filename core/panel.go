package core

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
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
// hints are the keys the panel lists: on a keyboard the key that closes a page or a menu is Esc.
func (a *App) hints() (plain, shift []hint) {
	plain, shift = a.rawHints()
	if Terminal && !a.Modal() && a.prompt == nil && a.kb == nil && (a.scr == Pads || a.scr == Files || a.scr == Slice || a.scr == Seq) {
		sh := make([]hint, 0, len(shift)) // Select+B is the t key
		tools := false
		for _, h := range shift {
			if h.key == "Sel+B" {
				tools = true
				continue
			}
			sh = append(sh, h)
		}
		if a.scr == Pads || tools {
			plain = append(append([]hint{}, plain...), hint{"t", "tools"})
		}
		shift = sh
	}
	if Terminal && a.scr == Slice && !a.tap && !a.lane && !a.Modal() { // the keyboard zooms with + and -
		plain = append(append([]hint{}, plain...), hint{"n", "new order"}, hint{"+ -", "zoom in / out"})
		sh := make([]hint, 0, len(shift))
		for _, h := range shift {
			if h.what != "zoom" {
				sh = append(sh, h)
			}
		}
		shift = sh
	}
	if Terminal && a.scr == Seq && !a.aHeld && !a.Modal() { // [ ] are the bank here, { } the bar
		out := make([]hint, 0, len(plain)+1)
		for _, h := range plain {
			if h.key == "L1 R1" && h.what == "bar" {
				out = append(out, hint{"L1 R1", "bank"}, hint{"{ }", "bar"})
				continue
			}
			out = append(out, h)
		}
		plain = out
	}
	if Terminal && a.scr == Seq && !a.aHeld && !a.Modal() {
		plain = append(append([]hint{}, plain...), hint{"m", "mute track"}, hint{"s", "solo track"}, hint{"f", "freeze view"}, hint{"z", "1 bar / more"})
		sh := make([]hint, 0, len(shift)) // s is solo here: the record key is Ctrl-R
		for _, h := range shift {
			if h.key != "Sel+X" {
				sh = append(sh, h)
			}
		}
		shift = sh
	}
	if Terminal && a.scr == Pads && !a.Modal() && a.prompt == nil && a.carry == nil {
		plain = append(append([]hint{}, plain...), hint{"m", "mark pad"}, hint{"M", "mark all / none"}, hint{"r", "rename pad"}, hint{"^E", "export bank"})
		sh := make([]hint, 0, len(shift)) // y a is m here
		for _, h := range shift {
			if h.what != "mark pad" {
				sh = append(sh, h)
			}
		}
		shift = sh
	}
	if Terminal && (a.scr == Pads || a.scr == Files) && !a.Modal() && a.prompt == nil && a.kb == nil && a.carry == nil { // Space plays, x stops
		out := make([]hint, 0, len(plain)+1)
		for _, h := range plain {
			switch {
			case h.key == "X" && (h.what == "play" || h.what == "listen" || h.what == "in pattern"):
				h = hint{"Space", h.what}
				out = append(out, h)
				out = append(out, hint{"x", "stop"})
				continue
			case h.key == "B" && h.what == "stop":
				continue
			}
			out = append(out, h)
		}
		plain = out
	}
	if Terminal && a.scr == Unit && !a.Modal() && a.prompt == nil {
		plain = append(append([]hint{}, plain...), hint{"o", "open from a folder"}, hint{"t", "project menu"})
	}
	if Terminal && a.scr == Files && !a.Modal() && a.prompt == nil && a.kb == nil {
		auto := "auto play: off"
		if a.set.AutoListen {
			auto = "auto play: ON"
		}
		plain = append(append([]hint{}, plain...), hint{"m", "mark file"}, hint{"M", "mark all"}, hint{"^A", auto})
	}
	if Terminal && a.scr == Files && !a.Modal() && a.prompt == nil && a.kb == nil { // t: load files from where
		out := make([]hint, 0, len(plain)+1)
		for _, h := range plain {
			if h.key == "t" {
				h = hint{"t", "from where"}
			}
			out = append(out, h)
		}
		plain = out
	}
	if Terminal && a.scr == Seq && !a.aHeld && !a.Modal() { // the keyboard: Space plays the pattern, x the note
		out := make([]hint, 0, len(plain))
		for _, h := range plain {
			switch {
			case h.key == "X" && (h.what == "play" || h.what == "stop"):
				h = hint{"Space", "play / stop"}
			case h.key == "L2" && h.what == "hear note":
				h = hint{"x", "hear note"}
			}
			out = append(out, h)
		}
		plain = out
	}
	if Terminal && a.Modal() {
		out := make([]hint, len(plain))
		copy(out, plain)
		for i, h := range out {
			if h.key == "B" && (h.what == "close" || h.what == "cancel" || h.what == "back") {
				out[i].key = "Esc"
			}
		}
		plain = out
	} else if Terminal {
		out := make([]hint, len(plain))
		copy(out, plain)
		for i, h := range out {
			if h.key == "B" && (h.what == "pads" || h.what == "back") { // leaving a screen
				out[i].key = "Esc"
			}
		}
		plain = out
	}
	return plain, shift
}

func (a *App) rawHints() (plain, shift []hint) {
	screen := hint{"Y+<>", "screen"}
	if Terminal {
		screen = hint{"g hl", "screen (1-6)"}
	}
	if a.pp != nil {
		if a.pp.page == 2 {
			return []hint{{"^v", "choose"}, {"<>", "change"}, {"A", "run"}, {"L1 R1", "page"}, {"B", "close"}}, nil
		}
		return []hint{{"^v", "setting"}, {"<>", "change"}, {"A", "reset/toggle"}, {"X", "hear pad"}, {"L1 R1", "page"}, {"B", "close"}}, nil
	}
	if a.prompt != nil && Terminal {
		return []hint{{"type", "the text"}, {"Enter", "done"}, {"Esc", "give it up"}, {"Bksp", "delete"}}, nil
	}
	if a.prompt != nil {
		return []hint{{"^v<>", "choose a key"}, {"A", "type it"}, {"B", "delete"}, {"L1 R1", "keys page"}, {"X", "done"}}, []hint{{"Y+B", "clear all"}, {"Y+X", "cancel"}}
	}
	if a.np != nil {
		return []hint{{"^v", "setting"}, {"<>", "change"}, {"A", "default"}, {"X", "hear note"}, {"R1", "pattern tools"}, {"B", "close"}}, nil
	}
	if a.dp != nil {
		return []hint{{"^v", "choose"}, {"l", "open the folder"}, {"h", "up"}, {"Enter", "use the folder in front"}, {"X", "new folder"}, {"/", "type a path"}, {"L1 R1", "places"}, {"B", "cancel"}}, nil
	}
	if a.picker != nil {
		open := "open it"
		if a.picker.pads {
			open = "this pad"
		}
		if a.picker.pads {
			return []hint{{"^v<>", "choose"}, {"L1 R1", "bank"}, {"X", "hear it"}, {"A", open}, {"B", "cancel"}}, nil
		}
		return []hint{{"^v<>", "choose"}, {"L1 R1", "bank"}, {"A", open}, {"B", "cancel"}}, nil
	}
	if a.menu != nil {
		hs := []hint{{"^v", "choose"}, {"<>", "change"}, {"A", "run"}}
		if a.menu.onX != nil {
			x := "X"
			if Terminal {
				x = "Space"
			}
			what := ""
			if a.menu.xHintOf != nil {
				what = a.menu.xHintOf()
			}
			hs = append(hs, hint{x, what})
		}
		if a.menu.tabs != nil {
			hs = append(hs, hint{"L1 R1", "tab"})
		}
		return append(hs, hint{"B", "close"}), nil
	}
	switch a.scr {
	case Pads:
		if a.carry != nil {
			return []hint{{"A", "put it here"}, {"B", "cancel"}, {"^v<>", "move"}}, []hint{screen}
		}
		return []hint{{"A", "menu"}, {"X", "play"}, {"B", "stop"}, {"^v<>", "move"}, {"L1 R1", "bank"}, {"R2", "arp marked"}},
			[]hint{screen, {"Y+A", "mark pad"}, {"Y+^v", "bank"}, {"Y+B", "clear pad"}, {"Sel+X", "record"}}
	case Files:
		if a.kb != nil && Terminal {
			mode := "folders only"
			if a.searchDirs {
				mode = "categories"
			} else if a.tagSearch {
				mode = "file names"
			}
			return []hint{{"type", "the words"}, {"Tab", mode}, {"Enter", "keep the result"}, {"Esc", "end the search"}, {"Bksp", "delete"}}, nil
		}
		if a.kb != nil {
			return []hint{{"^v<>", "move"}, {"A", "type it"}, {"B", "delete"}, {"X", "done"}}, []hint{{"Y+B", "clear all"}}
		}
		pick := hint{"A", "to pad " + a.label(a.padN())}
		if Terminal {
			pick = hint{"A", "to a pad..."}
			if n := len(a.fmarks); n > 0 {
				pick = hint{"A", fmt.Sprintf("%d marked to pads...", n)}
			}
		}
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
			[]hint{screen, {"Y+X", "places"}, {"Y+A", "search"}, {"Y+B", "favourite"}, {"Sel+A", "mark"}, {"Sel+B", "files menu"}, {"Sel+X", "record"}}
	case Slice:
		if a.tap {
			return []hint{{"A", "cut here"}, {"B", "stop tap"}}, nil
		}
		if a.lane {
			return []hint{{"<>", "step"}, {"^v", "other slice"}, {"A", "repeat"}, {"B", "drop"}, {"L1 R1", "carry"}, {"X", "play"}}, []hint{{"Y+B", "leave the lane"}, {"Y+A", "shuffle"}, {"Y+X", "back to 1,2,3"}}
		}
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
		if a.patStrip { // the arrows are on the row of pattern boxes
			return []hint{{"^v<>", "choose the pattern"}, {"L1 R1", "bank"}, play, {"A", "back to the grid"}}, nil
		}
		if Terminal {
			return []hint{{"A", "note on/off"}, {"A+^<>", "edit (hold)"}, play, stop, {"L2", "hear note"}, {"R2", "mute/solo"}, {"^v<>", "pad, step"}, {"L1 R1", "bar"}},
				[]hint{{"Y+X", "add track"}, {"p", "pattern tiles"}, {"Sel+B", "tools"}, {"Y+^v", "pattern +-"}, {"Sel+X", "record"}} // (y b, the dialog with every bank, is in the docs)
		}
		return []hint{{"A", "note on/off"}, {"A+^<>", "edit (hold)"}, play, stop, {"L2", "hear note"}, {"R2", "mute/solo"}, {"^v<>", "pad, step"}, {"L1 R1", "bar"}}, // (z: one bar or more, listed below on a keyboard)
			[]hint{{"Y+X", "add track"}, {"Y+B", "pick pattern"}, {"Sel+B", "tools"}, {"Y+^v", "pattern +-"}, {"Sel+X", "record"}}
	case Loop:
		return []hint{{"A", "make the pattern"}, {"X", "delete (twice)"}, {"^v<>", "move"}, {"L1 R1", "bank"}, {"B", "pads"}}, []hint{screen, {"Y+X", "remix"}, {"Y+^v", "slices"}, {"Sel+X", "record"}}
	case Perform:
		return []hint{{"A", "pattern now"}, {"X", "next pattern"}, {"B", "back"}}, []hint{screen, {"Y+A", "next -> now"}, {"Y+X", "clear next"}, {"Y+B", "note BUS 1"}, {"Sel+B", "note BUS 2"}, {"Sel+X", "record"}}
	case Config:
		return []hint{{"^v", "setting"}, {"<>", "change"}, {"A", "edit / switch"}, {"L1 R1", "jump"}, {"B", "pads"}}, []hint{screen, {"Sel+X", "record"}}
	case Unit:
		if Terminal {
			return []hint{{"A", "project menu"}, {"^v<>", "choose"}, {"B", "back"}}, []hint{screen, {"Sel+X", "record"}}
		}
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
	mapRows := (len(screenNames) + 2) / 3
	if Terminal { // the six screens of a computer; PROJECTS only with a unit
		mapRows = 2
		for i, sc := range termScreens {
			if !a.screenShown(sc) {
				continue
			}
			cx, cy := x+(i%3)*cw, 1+i/3
			bg, fg := cPanel2, cDim
			if sc == a.scr {
				bg, fg = cAmber, cBg
			}
			g.Fill(cx, cy, cw-1, 1, bg)
			n := screenTitle(sc)
			if cw-1 >= 8 {
				n = fmt.Sprintf("%d %s", i+1, n)
			}
			g.Text(cx+max(min(2, (cw-1-len(n))/2), 0), cy, n, fg, sc == a.scr) // the names start at the same column in every button
		}
	} else {
		for i, n := range screenNames {
			cx, cy := x+(i%3)*cw, 1+i/3
			bg, fg := cPanel2, cDim
			if Screen(i) == a.scr {
				bg, fg = cAmber, cBg
			}
			g.Fill(cx, cy, cw-1, 1, bg)
			g.Text(cx+(cw-1-len(n))/2, cy, n, fg, Screen(i) == a.scr)
		}
	}

	// tempo and the state of the loop
	y := 2 + mapRows
	if Terminal {
		if running, _, _ := a.indexStatus(); running {
			a.drawIndexProgress(g, x, y, w)
			y += 4
		}
	}
	if Terminal && a.link != nil { // with a unit plugged in the level of its sound is always in view
		y += a.drawMeter(g, x, y, w)
	}
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

	if Terminal {
		a.drawKeyboardKeys(g, x, y, w)
		return
	}
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

// tkLine is a line of hints written with the handheld's buttons ("<> change  A default  B close") as a keyboard has them: Esc
// closes, the letters are the keys of the buttons.
func tkLine(s string) string {
	if !Terminal {
		return s
	}
	s = regexp.MustCompile(`\bB (close|closes|back|cancel)`).ReplaceAllString(s, "Esc $1")
	s = regexp.MustCompile(`\b[ABX]\b`).ReplaceAllStringFunc(s, strings.ToLower)
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
		wh := 2 // the shape of the sample: taller on a computer when the panel has the room
		if Terminal && y1-row(0) >= 24 {
			wh = 4
		}
		g.Fill(x, row(2), w, wh, cPanel2)
		if id, ok := a.padSampleSoft(n); ok {
			a.wave(g, id, 0, p.Frames, x, row(2), w, wh, cAmber)
		} else if a.live != nil && a.lab.IsRemote(n) {
			g.Text(x+1, row(2), "fetching from the unit...", cDim, false)
		}
		if Terminal { // values, no faders: the numbers say it
			a.drawPadInfo(g, x, row(3+wh), w, y1, n)
			return
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
		if a.job != nil {
			if done, total, name, _, fin := a.job.state(); !fin {
				g.Text(x, row(0), "WORKING", cAmber, true)
				g.Bar(x, row(2), w, float64(done)/float64(max(total, 1)), cAmber, cPanel2)
				g.Text(x, row(3), fit(fmt.Sprintf("%d of %d  %s", done, total, name), w), cDim, false)
				return
			}
		}
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
		if tg := a.tagsOf(joinPath(a.dir, e.name)); tg != "" {
			tagRow := 7
			if Terminal {
				tagRow = 13 // under the waveform
			}
			g.Text(x, row(tagRow), fit(tg, w), cAmber, false)
		}
		if a.searchQ != "" && Terminal { // a search result: the whole folder it is in, under the shape
			full := filepath.Dir(joinPath(a.dir, e.name))
			g.Text(x, row(14), "FOLDER", cDim, true)
			for i, l := range wrapPath(full, w) {
				if row(15+i) >= y1 || i >= 4 {
					break
				}
				g.Text(x, row(15+i), l, cText, false)
			}
		}
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			g.Text(x, row(3), fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			if _, in := a.aud[a.padN()]; in {
				g.Text(x+9, row(3), "in pattern", cGood, true)
			}
			if Terminal { // a computer: the shape of the sample is in the panel, and the ranger's third column keeps to folders
				g.Fill(x, row(5), w, 7, cPanel2)
				a.wave(g, id, 0, s.Frames(), x, row(5), w, 7, cAmber)
			} else if contentCols < rangerMinCols { // the ranger's right pane shows the shape
				g.Fill(x, row(4), w, 2, cPanel2)
				a.wave(g, id, 0, s.Frames(), x, row(4), w, 2, cText)
			}
		} else {
			g.Text(x, row(3), fit(err.Error(), w), cBad, false)
		}
	case Slice:
		if a.sl == nil {
			return
		}
		if Terminal {
			a.sliceHeader(g, x, row(0), w)
			a.drawInfoGroups(g, x, row(2), w, y1, a.sliceInfoGroups(), 2)
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
		if Terminal {
			a.drawInfoGroups(g, x, row(0), w, y1, a.seqInfoGroups(), 2)
			return
		}
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
				g.Text(x, row(4), "pitch "+formats.PitchText(uint8(n.Pitch)), cAmber, false)
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
	case Loop:
		g.Text(x, y0, fmt.Sprintf("BANK %c", 'A'+a.loopBank), cAmber, true)
		n := a.loopPad()
		g.Text(x, y0+1, fit(a.label(n)+" "+a.info.Pads[n].Name, w), cText, false)
		g.Text(x, y0+3, fmt.Sprintf("%d bars", a.loopBars()), cDim, false)
	case Perform:
		g.Text(x, y0, "LIVE", cAmber, true)
		g.Text(x, y0+1, fit(a.midiStatus(), w), cDim, false)
		if s := a.mon.Summary(); s != "" {
			g.Text(x, y0+3, fit(s, w), cText, false)
		}
		if a.rec != nil {
			g.Text(x, y0+5, "RECORDING", cBad, true)
			g.Text(x, y0+6, fit(short2(a.rec.Path, w), w), cDim, false)
		}
	case Config:
		a.configContext(g, x, y0, w, y1)
	}
}

// drawKeyboardKeys is the panel's key area on a computer: the keys that work on every screen first, then the keys of the
// screen in front (and its Y and Select chords after them). When the panel is short the chords go first, then the global
// keys, so that the middle of the panel keeps room.
func (a *App) drawKeyboardKeys(g Gfx, x, y, w int) {
	plain, shift := a.hints()
	type pair struct{ a, b hint }
	global := []pair{
		{hint{"1-6", "screens"}, hint{"g hl", "prev/next"}},
		{hint{"u", "undo"}, hint{"R", "redo"}},
		{hint{"^R F9", "record"}, hint{"?", "all keys"}},
		{hint{"q", "quit"}, hint{"y s", "shift keys"}},
	}
	if a.live != nil { // edited live: the unit can be told to read its files again (patterns written to it show then)
		global = append(global, pair{hint{"^L", "reload unit"}, hint{}})
	}
	room := 6 // rows the middle of the panel keeps; the screens that have a lot to say about what is under the cursor keep more
	_, hasPattern := a.doc()
	if a.scr == Slice || a.scr == Pads && a.info.Pads[a.padN()].File || a.scr == Seq && hasPattern {
		room = 11
	}
	two := w >= 38 // a wide panel lists the screen's keys in two columns
	rowsOf := func(n int) int {
		if two {
			return (n + 1) / 2
		}
		return n
	}
	floor := 0
	if a.scr == Seq && a.patStrip { // p: the arrows are on the tiles and the keys are few; the block keeps the height it has with the grid, so nothing moves
		a.patStrip = false
		p2, s2 := a.hints()
		a.patStrip = true
		floor = 1 + rowsOf(len(p2)) + rowsOf(len(s2))
	}
	viewRows := func() int { return max(1+rowsOf(len(plain))+rowsOf(len(shift)), floor) }
	globalRows := func() int { return 1 + len(global) }
	total := func() int { return viewRows() + globalRows() + 2 } // two rules
	limit := Rows - 3 - (y + 2) - room
	for total() > limit && len(shift) > 0 {
		shift = nil
	}
	for total() > limit && len(global) > 1 {
		global = global[:len(global)-1]
	}
	for total() > limit && len(plain) > 3 {
		plain = plain[:len(plain)-1]
	}
	ky := Rows - 2 - viewRows() // the first row of the screen's keys
	gy := ky - 1 - globalRows() // the first row of the global keys
	// the screen's keys
	switch {
	case a.yHeld: // a key is armed: the screen's chords light up
		g.Fill(x, ky, w, 1, cAmber)
		g.Text(x+1, ky, "SHIFT  (y is down)", cBg, true)
	case a.selHeld:
		g.Fill(x, ky, w, 1, cAmber)
		g.Text(x+1, ky, "SELECT  (s is down)", cBg, true)
	default:
		g.Text(x, ky, screenNames[a.scr], cAmber, true)
	}
	a.keyColumns(g, x, ky+1, w, plain, !a.yHeld || len(shift) == 0, two)
	if len(shift) > 0 {
		a.keyColumns(g, x, ky+1+rowsOf(len(plain)), w, shift, a.yHeld || a.selHeld, two)
	}
	// the global keys, two to a row
	g.HRule(x, ky-1, w, cLine)
	g.Text(x, gy, "ALWAYS", cDim, true)
	col2 := x + w/2
	for i, p := range global {
		for j, h := range []hint{p.a, p.b} {
			if h.key == "" {
				continue
			}
			cx := x
			if j == 1 {
				cx = col2
			}
			g.Text(cx, gy+1+i, h.key, cAmber, true)
			g.Text(cx+len(h.key)+1, gy+1+i, fit(h.what, w/2-len(h.key)-2), cText, false)
		}
	}
	g.HRule(x, gy-1, w, cLine)
	if a.scr == Seq && w >= 40 && gy-2-7-(y+2) >= 12 { // the bank's 16 patterns, 4 by 4, right under the level and the tempo; what is under the cursor below them
		g.Text(x, y+2, "PATTERNS", cDim, true)
		g.Text(x+len("PATTERNS  "), y+2, "p choose with the arrows", cDim, false)
		a.drawPatternStrip(g, x, y+3, w)
		g.HRule(x, y+7, w, cLine)
		a.context(g, x, y+8, w, gy-2)
		return
	}
	a.context(g, x, y+2, w, gy-2)
}

// keyColumns is keyBlock in one column or two (filled down the left one first).
func (a *App) keyColumns(g Gfx, x, y, w int, hs []hint, active, two bool) {
	if !two {
		a.keyBlock(g, x, y, w, hs, active)
		return
	}
	half := (len(hs) + 1) / 2
	a.keyBlock(g, x, y, w/2, hs[:half], active)
	a.keyBlock(g, x+w/2, y, w-w/2, hs[half:], active)
}

// wrapPath breaks a path into lines of at most w characters, after a slash.
func wrapPath(p string, w int) []string {
	var lines []string
	for len(p) > w && w > 4 {
		cut := strings.LastIndex(p[:w], "/")
		if cut <= 0 {
			cut = w
		}
		lines = append(lines, p[:cut])
		p = p[cut:]
	}
	return append(lines, p)
}
