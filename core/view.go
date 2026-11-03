package core

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// Render draws the frame for the state of the app as pixels: the handheld's screen, the web page, the screenshots.
func (a *App) Render() *image.RGBA {
	c := newCanvas()
	a.draw(pixelGfx{c})
	return c.img
}

// TextScreen is the screen as characters, for a terminal: Cols x Rows cells.
type TextScreen struct {
	Cols, Rows int
	Cells      []Cell
	Clash      []string // letters written over other letters (the tests look at it)
}

// RenderCells draws the screen as characters, in the style of the terminal program.
func (a *App) RenderCells() TextScreen {
	t := newTextGfx()
	a.draw(t)
	return TextScreen{Cols: t.cols, Rows: t.rows, Cells: t.cells, Clash: t.clash}
}

// draw draws the whole screen with a backend.
func (a *App) draw(g Gfx) {
	if a.splash != nil {
		a.drawSplash(g)
		return
	}
	a.titleBar(g)
	switch a.scr {
	case Pads:
		a.drawPads(g)
	case Files:
		a.drawFiles(g)
	case Slice:
		a.drawSlice(g)
	case Seq:
		a.drawSeq(g)
	case Save:
		a.drawSave(g)
	case Unit:
		a.drawUnit(g)
	case Config:
		a.drawConfig(g)
	case Perform:
		a.drawPerform(g)
	case Loop:
		a.drawLoop(g)
	}
	if a.picker != nil {
		a.drawPicker(g)
	}
	if a.kb != nil {
		a.drawKeyboard(g)
	}
	if a.prompt != nil {
		a.drawPrompt(g)
	}
	if a.pp != nil {
		a.drawPadPanel(g)
	}
	if a.np != nil {
		a.drawNotePanel(g)
	}
	a.drawPanel(g)
	a.footer(g)
	if a.menu != nil {
		a.drawMenu(g)
	}
	if a.help {
		a.drawHelp(g)
	}
}

func (a *App) titleBar(g Gfx) {
	g.Fill(0, 0, Cols, 1, cPanel)
	g.Fill(0, 0, 8, 1, cAmber)
	g.Text(1, 0, "404tui", cBg, true)
	g.Text(9, 0, screenNames[a.scr], cAmber, true)
	right := fit(fmt.Sprintf("%s  bank %c", a.ProjectName(), 'A'+rune(a.bank)), Cols-30)
	g.Text(Cols-2-len(right), 0, right, cText, false)
	if a.rec != nil {
		g.Fill(Cols/2-6, 0, 12, 1, cBad)
		g.Text(Cols/2-5, 0, a.recText(), cBg, true)
	}
	if a.live != nil {
		tag := fmt.Sprintf("LIVE %d", a.live.project+1)
		if w, _, err := a.liveStatus(); err != nil || w > 0 {
			tag += fmt.Sprintf(" (%d)", w)
		}
		g.Text(Cols-2-len(right)-2-len(tag), 0, tag, cGood, true)
	}
}

// Terminal is set when the app is shown in a terminal and played from the keyboard: the hint line and the help say so.
var Terminal bool

// Version is the commit the program was built from, shown in the corner so it is always known which one runs.
var Version string

// footer is the message line and the hint line; the keys are in the panel.
func (a *App) footer(g Gfx) {
	top := Rows - 2
	g.Fill(0, top, Cols, 2, cPanel)
	if a.msg != "" {
		col := cGood
		if a.msgBad {
			col = cBad
		}
		g.Text(2, top, fit(a.msg, Cols-4), col, true)
	} else if a.scr == Seq && a.menu == nil && a.picker == nil {
		// no message: what is known of the note under the cursor
		if info := a.noteInfo(); info != "" {
			g.Text(2, top, fit(info, Cols-4), cText, false)
		}
	}
	hintLine := "Y keys  Y+L2 undo  Y+R2 redo  Sel+Start quit"
	if Terminal {
		hintLine = "1-9 g h/l screens  y shift  s select  ? keys  q quit"
	}
	g.Text(2, top+1, hintLine, cDim, false)
	room := Cols - 4 - len(hintLine) - 2 // what the autosave note and the version share
	if Version != "" {
		room -= len(Version) + 2
		g.Text(Cols-2-len(Version), top+1, Version, cDim, false)
	}
	if ago := a.autosaveAgo(); len(ago) <= room {
		g.Text(2+len(hintLine)+2, top+1, ago, cDim, false)
	}
}

func (a *App) drawHelp(g Gfx) {
	rows := []string{
		"Y held   shift: the second function of a key",
		"Y+< >    previous / next screen",
		"Y+^ v    PADS bank   SLICE zoom   SEQ pattern",
		"A        do / menu / put a note / cut",
		"B        back / stop / join / SEQ menu",
		"X        listen to what is under the cursor",
		"Y alone  this page",
		fmt.Sprintf("Y+L2  undo (%d)    Y+R2  redo (%d)", len(a.undo), len(a.redo)),
		"Select+Start  quit (asks when not saved)",
		"",
		"In a terminal: arrows or h j k l, a b x, [ ] = L1 R1, , . = L2 R2",
		"  y or s: the next key is pressed with Y / Select held",
		"  e: hold A (arrows edit the note), e again lets go",
		"  Ctrl-C quits (twice when not saved)",
		"",
		"PADS  SEQ  FILES  SLICE  SAVE  UNIT",
		"SAVE writes a folder for the SD card; UNIT sends",
		"the project over USB to an SP-404MKII.",
	}
	if Terminal {
		rows = []string{
			"arrows or h j k l   D-pad (as in vim)",
			"a b x              the buttons A, B, X (Enter, Esc, Space too)",
			"y                  the next key with Y held",
			"1-9, Tab, g h/l    go to a screen (gh before, gl next, gg first)",
			"s                  the next key with Select held (s b)",
			"e                  hold A (edit a note); e again lets go",
			"[ ] or Ctrl-U/D    L1 R1;   , .   L2 R2",
			"?                  this page;   /  search the files",
			fmt.Sprintf("u undo (%d)   Ctrl-R redo (%d)", len(a.undo), len(a.redo)),
			"q or Ctrl-C        quit (twice when not saved)",
			"",
			"PADS  SEQ  FILES  SLICE  SAVE  UNIT",
			"The side panel shows the keys that work now.",
		}
	} else {
		rows = append(rows[:10], rows[15:]...) // the handheld has no keyboard lines
	}
	cols, h := min(contentCols-4, 60), len(rows)+5
	x, y := (contentCols-cols)/2, max(1, (Rows-h)/2-1)
	g.Fill(x, y, cols, h, cPanel2)
	g.Frame(x, y, cols, h, cAmber)
	g.Text(x+2, y+1, "KEYS", cAmber, true)
	for i, r := range rows {
		g.Text(x+2, y+2+i, fit(r, cols-4), cText, false)
	}
}

// ---- PADS

// drawPads is the 4x4 pads of the bank, as big as the room allows, and the banks under them. A pad is a block of cells with a
// cell between it and the next.
func (a *App) drawPads(g Gfx) {
	cols := contentCols - 2
	pw := cols / 4 // a pad's pitch in cells
	x0 := 1 + (cols-4*pw)/2
	ph := max((Rows-6)/4, 3) // the rows the pads have: what is left of the body, less the banks (2) and a gap
	tw, th := pw-1, ph-1
	cursorAt := [4]int{-1}
	for i := 0; i < 16; i++ {
		n := a.bank*16 + i
		x, y := x0+(i%4)*pw, 1+(i/4)*ph
		p := a.info.Pads[n]
		bg := cPanel
		if p.File {
			bg = cPanel2
		}
		if a.playing == n {
			bg = cAmberD
		}
		g.Fill(x, y, tw, th, bg)
		num := fmt.Sprint(i + 1)
		g.Text(x+tw-len(num)-1, y, num, cDim, false)
		if p.File {
			g.Fill(x, y, 1, th, cAmber)
			g.Text(x+2, y, short(p.Name, pw-6), cText, true)
			if Terminal { // a computer's terminal shows the length and the switches (the wave is in the panel)
				if th >= 3 {
					a.drawTileInfo(g, x+2, y+1, tw-3, th >= 4, n, p.Frames)
				}
			} else if id, ok := a.padSample(n); ok && th >= 3 { // the handheld: the wave, an even number of rows, so the middle line is between two
				a.wave(g, id, 0, p.Frames, x+1, y+1, tw-2, (ph-2)/2*2, cAmber)
			}
			// the switches that are on: G for gate, L for loop
			cx := x + tw - len(num) - 1
			for _, sw := range []struct {
				id byte
				ch string
			}{{0x6B, "L"}, {0x6A, "G"}} {
				if v, err := a.lab.PadParam(n, sw.id); err == nil && v != 0 {
					cx -= 2
					g.Badge(cx, y, sw.ch, true)
				}
			}
		}
		for mi, mp := range a.marks { // a pad that is a key of the arp: its place among the keys
			if mp == n {
				g.Fill(x+1, y+th-1, 2, 1, cAmber)
				g.Text(x+1, y+th-1, fmt.Sprint(mi+1), cBg, true)
			}
		}
		if i == a.cur {
			cursorAt = [4]int{x, y, tw, th}
		}
	}
	if cursorAt[0] >= 0 {
		g.Cursor(cursorAt[0], cursorAt[1], cursorAt[2], cursorAt[3])
	}
	a.drawBanks(g, a.bank)
}

// drawBanks is the row of the ten banks at the bottom, the one in front lit.
func (a *App) drawBanks(g Gfx, cur int) {
	by := Rows - 4
	bw := (contentCols - 2) / 10
	bx0 := 1 + (contentCols-2-bw*10)/2
	for b := 0; b < 10; b++ {
		used := 0
		for i := 0; i < 16; i++ {
			if a.info.Pads[b*16+i].File {
				used++
			}
		}
		x := bx0 + b*bw
		col, fg := cPanel, cDim
		if b == cur {
			col, fg = cAmber, cBg
		}
		g.Fill(x, by, bw-1, 1, col)
		g.Text(x+(bw-1)/2, by, string(rune('A'+b)), fg, true)
		if !Terminal { // the handheld shows how full each bank is; the terminal has the highlighter only
			g.Bar(x, by+1, bw-1, float64(used)/16, cAmber, cLine)
		}
	}
}

// drawTileInfo is what a pad that is not selected shows: how long its sample is and, when there is a row for it, the
// switches of the pad (G gate, L loop, O one shot, R not forward, S BPM sync, V vinyl), lit when on.
func (a *App) drawTileInfo(g Gfx, x, y, w int, flags bool, n, frames int) {
	secs := float64(frames) / 48000
	txt := fmt.Sprintf("%.2f s", secs)
	if bpm := a.padValue(n, 0x6F, 12000); bpm > 0 && secs > 0.5 {
		if bars := secs * float64(bpm) / 100 / 60 / 4; bars >= 0.9 {
			txt = fmt.Sprintf("%.1f bars", bars)
		}
	}
	g.Text(x, y, fit(txt, w), cDim, false)
	if !flags {
		return
	}
	v := func(id byte) int32 { r, _ := a.lab.PadParam(n, id); return r }
	for i, f := range []struct {
		ch string
		on bool
	}{{"G", v(0x6A) != 0}, {"L", v(0x6B) != 0}, {"O", v(0x70)>>5&1 != 0}, {"R", v(0x75) != 0}, {"S", v(0x6E) != 0}, {"V", v(0x77) != 0}} {
		if 2*i+1 > w {
			break
		}
		col := cLine
		if f.on {
			col = cGood
		}
		g.Text(x+2*i, y+1, f.ch, col, f.on)
	}
}

// wave draws the peaks of frames from..to of a sample in a box of cells, mirrored around its middle.
func (a *App) wave(g Gfx, id, from, to, col, row, cols, rows int, c color.RGBA) {
	a.waveScaled(g, id, from, to, col, row, cols, rows, c, 0)
}

// samplePeak is the loudest value of a sample, which the slicer's waveform is scaled to (the same size at every zoom).
func (a *App) samplePeak(id int) int {
	s, err := a.lab.Sample(id)
	if err != nil || len(s.Samples) == 0 {
		return 1
	}
	key := peakKey{&s.Samples[0], len(s.Samples)}
	if v, ok := a.peakMax[key]; ok {
		return v
	}
	if a.peakMax == nil || len(a.peakMax) > 48 {
		a.peakMax = map[peakKey]int{}
	}
	m := 1
	for _, v := range s.Samples {
		if v < 0 {
			v = -v
		}
		m = max(m, int(v))
	}
	a.peakMax[key] = m
	return m
}

// waveScaled is wave with the peak the heights are measured against (0: the loudest of what is drawn).
func (a *App) waveScaled(g Gfx, id, from, to, col, row, cols, rows int, c color.RGBA, peakOf int) {
	n := cols * g.BarsPerCell()
	pk, err := a.peaks(id, from, to, n)
	if err != nil {
		return
	}
	h := make([]float64, len(pk))
	peak := 1
	if Terminal && peakOf > 0 {
		peak = peakOf
	} else if Terminal { // as in the classic program: the peak fills the box and the quiet parts show
		for _, p := range pk {
			peak = max(peak, p)
		}
	} else {
		peak = 32768
	}
	for i, p := range pk {
		h[i] = float64(p) / float64(peak)
		if Terminal {
			h[i] = math.Sqrt(h[i])
		}
	}
	g.Bars(col, row, cols, rows, h, true, c)
}

// ---- FILES

func (a *App) drawFiles(g Gfx) {
	if contentCols >= rangerMinCols {
		a.drawRanger(g)
		return
	}
	inner := contentCols - 4
	head := short2(a.dir, inner)
	if a.isFavourite(a.dir) {
		head = "* " + short2(a.dir, inner-2)
	}
	col := cDim
	if a.searchQ != "" {
		head = fmt.Sprintf("search %q in %s", a.searchQ, short2(a.dir, max(inner-30, 12)))
		if a.searchCut {
			head += " (cut short)"
		}
		col = cAmber
	}
	g.Text(2, 1, fit(head, inner), col, a.searchQ != "")
	if len(a.entries) == 0 {
		g.Text(2, 3, "(no folders or audio files here)", cDim, false)
		if a.dir == a.Root {
			g.Text(2, 5, "Put .wav files in this folder:", cText, false)
			g.Text(2, 6, fit(a.Root, inner), cAmber, false)
		}
		g.Text(2, 8, tk("< goes up    Y+X jumps to the usb drive"), cDim, false)
	}
	a.drawFileList(g, 1, contentCols-2, a.entries, a.fcur, true)
}

// drawFileList draws a column of entries from x0, w cells wide; the cursor is shown when focus is set.
func (a *App) drawFileList(g Gfx, x0, w int, ents []entry, cur int, focus bool) {
	rows := a.fileRows()
	top := max(0, min(cur-rows/2, len(ents)-rows))
	for i := 0; i < rows && top+i < len(ents); i++ {
		e := ents[top+i]
		y := 2 + i
		fgSel := cText
		if top+i == cur {
			if focus {
				g.Highlight(x0, y, w, 1)
				fgSel = cWhite
			} else {
				g.Fill(x0, y, w, 1, cPanel2)
			}
		}
		avail := w - 9
		name := fit(e.name, avail)
		if a.searchQ != "" && focus { // a result is a path below the folder: its end says which file
			name = short2(e.name, avail)
		}
		col := fgSel
		if e.dir {
			name = fit(e.name, avail-1) + "/"
			col = cAmber
		}
		if e.project {
			name = short(e.name, min(24, avail-9)) + "  project"
			col = cGood
		}
		if focus && a.fmarks[joinPath(a.dir, e.name)] {
			g.Text(x0, y, "*", cAmber, true)
		}
		g.Text(x0+1, y, name, col, e.dir)
		if !e.dir && w >= 20 {
			sz := fmt.Sprintf("%dK", (e.size+512)/1024)
			g.Text(x0+w-1-len(sz), y, sz, cDim, false)
		}
	}
}

// fileRows is how many files the list shows: the body, less the line of the folder.
func (a *App) fileRows() int {
	if contentCols >= rangerMinCols {
		return Rows - 6 // the panes have a frame
	}
	return Rows - 4
}

// ---- SLICE

func (a *App) drawSlice(g Gfx) {
	sl := a.sl
	sec := func(f int) float64 { return float64(f) / 48000 }
	name := a.info.Pads[a.spad].Name
	g.Text(2, 1, fmt.Sprintf("%s  %s", a.label(a.spad), fit(name, 40)), cAmber, true)

	wc, wr := contentCols-4, a.sliceWaveRows()
	g.Fill(1, 2, contentCols-2, wr, cPanel)
	from, to := a.view()
	sel := a.sliceAt(a.at)
	frac := func(f int) float64 { return float64(f-from) / float64(max(to-from, 1)) }
	if Terminal { // as in the classic program: the slice is a quiet ground, its dots in its own colour, scaled to the whole sample
		if sel >= 0 {
			x0, x1 := max(frac(sl.Slices[sel].Start), 0), min(frac(sl.Slices[sel].End), 1)
			if x1 > x0 {
				c0, c1 := 2+int(x0*float64(wc)), 2+min(int(x1*float64(wc)+0.999), wc)
				g.Fill(c0, 2, max(c1-c0, 1), wr, cPanel2)
			}
		}
		pk := a.samplePeak(a.sid)
		a.waveScaled(g, a.sid, from, to, 2, 2, wc, wr, cLine, pk) // what no slice holds
		for i, sg := range sl.Slices {
			lo, hi := max(sg.Start, from), min(sg.End, to)
			if hi <= lo {
				continue
			}
			c0, c1 := int(frac(lo)*float64(wc)), min(int(frac(hi)*float64(wc)+0.999), wc)
			col := cAmber
			if i%2 == 1 {
				col = cBlue
			}
			if sg.Chance == 0 {
				col = cDim
			}
			a.waveScaled(g, a.sid, lo, hi, 2+c0, 2, max(c1-c0, 1), wr, col, pk)
		}
	} else {
		if sel >= 0 {
			g.Region(2, 2, wc, wr, max(frac(sl.Slices[sel].Start), 0), min(frac(sl.Slices[sel].End), 1))
		}
		a.wave(g, a.sid, from, to, 2, 2, wc, wr, cText)
	}
	for i, sg := range sl.Slices {
		if sg.Start < from || sg.Start >= to {
			continue
		}
		g.Mark(2, 2, wc, wr, frac(sg.Start), cAmber)
		g.Text(2+int(frac(sg.Start)*float64(wc))+1, 2, fmt.Sprint(i+1), cAmber, true)
	}
	g.Mark(2, 2, wc, wr, min(max(frac(a.at), 0), 1), cWhite)

	// the overview, with the window in it
	oy := 2 + wr
	g.Fill(1, oy, contentCols-2, 2, cPanel)
	a.wave(g, a.sid, 0, sl.Frames, 2, oy, wc, 2, cDim)
	for _, sg := range sl.Slices {
		g.Mark(2, oy, wc, 2, float64(sg.Start)/float64(sl.Frames), cAmber)
	}
	g.Mark(2, oy, wc, 2, float64(from)/float64(sl.Frames), cWhite)
	g.Mark(2, oy, wc, 2, float64(to)/float64(sl.Frames), cWhite)

	// the numbers
	ny := oy + 2
	if sel >= 0 {
		sg := sl.Slices[sel]
		g.Text(2, ny, fit(fmt.Sprintf("SLICE %d/%d   %.3f - %.3f s   %.3f s long", sel+1, len(sl.Slices), sec(sg.Start), sec(sg.End), sec(sg.End-sg.Start)), contentCols-4), cText, true)
	}
	g.Text(2, ny+1, fmt.Sprintf("cursor %.3f s", sec(a.at)), cDim, false)
	// the order as it plays: one cell per step, the slice's number in it, whatever the mode and the grid make of the order. A
	// step that stays silent (chance) is dim, the slices under the cursor are lit, the step that sounds is amber.
	plan := a.slicePlan()
	lbl := fmt.Sprintf("ORDER  %s  %d steps", orderMode(a.sl), len(plan))
	if a.sl.Division > 0 {
		lbl += fmt.Sprintf("  1/%d at %s BPM", a.sl.Division, bpmText(a.tempoOf(a.spad/16)))
	}
	lbl = fit(lbl, contentCols-24)
	g.Text(contentCols-1-len(lbl), ny+1, lbl, cDim, false)
	per := max((contentCols-4)/4, 1) // a step is 3 cells and a gap
	now := a.playingStep()
	cellRows := Rows - 3 - (ny + 2) + 1 // what is left under the numbers
	if a.lane {
		g.Fill(1, ny+1, contentCols-2, 1, cPanel)
		g.Text(2, ny+1, fmt.Sprintf("ORDER LANE  %d steps of %d slices", len(a.sl.Order), len(a.sl.Slices)), cAmber, true)
		a.drawLane(g, ny+2, cellRows)
		return
	}
	for i, st := range plan {
		if i >= cellRows*per {
			g.Text(2+(per-1)*4, ny+2+cellRows-1, fmt.Sprintf("+%d", len(plan)-cellRows*per+1), cDim, false)
			break
		}
		x, y := 2+(i%per)*4, ny+2+i/per
		col, fg := cPanel2, cText
		switch {
		case i == now:
			col, fg = cAmber, cBg
		case st.Slice == sel:
			col, fg = cAmberD, cWhite
		}
		if st.Rest && i != now {
			fg = cLine
		}
		g.Fill(x, y, 3, 1, col)
		t := fmt.Sprint(st.Slice + 1)
		if st.Rest {
			t = "-" + t
		}
		g.Text(x+(3-len(t))/2, y, t, fg, true)
	}
}

// sliceWaveRows is the rows of the big waveform: the body less the title, the overview (2 rows) and the numbers and
// the order under it (4 rows).
func (a *App) sliceWaveRows() int { return max(Rows-3-1-2-4, 6) &^ 1 }

// ---- SAVE

func (a *App) drawSave(g Gfx) {
	used, bytesN := 0, 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
			bytesN += p.Frames * p.Ch * 2
		}
	}
	inner := contentCols - 4
	g.Text(2, 2, "WRITE THE PROJECT", cDim, false)
	g.Text(2, 4, fit(strings.ToUpper(a.ProjectName()), inner), cAmber, true)
	dest := filepath.Join(a.OutDir, cardFolder, a.ProjectName())
	g.Text(2, 7, short2(dest, inner), cText, false)
	g.Text(2, 9, fmt.Sprintf("%d pads with samples   %.1f MB   %d files", used, float64(bytesN)/1e6, a.info.Files), cText, false)
	g.HRule(2, 11, contentCols-4, cLine)
	if a.wrote == nil {
		g.Fill(2, 12, 24, 3, cAmber)
		g.Text(4, 13, "A  WRITE TO THE CARD", cBg, true)
		g.Text(2, 16, "Left/Right changes the project number.", cDim, false)
		g.Text(2, 17, "An existing folder with that number is replaced.", cDim, false)
		a.drawAutosave(g)
		return
	}
	g.Text(2, 12, "WRITTEN", cGood, true)
	show := a.wrote
	if len(show) > 5 {
		show = append(append([]string{}, show[:4]...), fmt.Sprintf("... and %d more", len(a.wrote)-4))
	}
	for i, n := range show {
		g.Text(2, 13+i, fit("  "+n, inner), cText, false)
	}
	g.Text(2, 19, "Put the card in the SP-404MKII.", cDim, false)
	a.drawAutosave(g)
}

// drawAutosave is the interval and the log of the autosave, at the foot of the SAVE screen.
func (a *App) drawAutosave(g Gfx) {
	y0 := Rows - 9
	g.HRule(2, y0-1, contentCols-4, cLine)
	g.Text(2, y0, "AUTOSAVE", cDim, false)
	g.Text(2+len("AUTOSAVE")+1, y0, a.autosaveSetting(), cAmber, true)
	g.Text(contentCols-2-len(tk("^v changes")), y0, tk("^v changes"), cDim, false)
	a.as.mu.Lock()
	h := append([]string{}, a.as.history...)
	a.as.mu.Unlock()
	if len(h) == 0 {
		g.Text(2, y0+2, "nothing yet: it saves when the project has changed", cDim, false)
		return
	}
	if len(h) > 4 {
		h = h[len(h)-4:]
	}
	for i, l := range h {
		col := cText
		if i == len(h)-1 {
			col = cGood
		}
		g.Text(2, y0+2+i, fit(l, contentCols-4), col, false)
	}
}

// orderMode is how the order is played, for the label above it.
func orderMode(sl *formats.Slicing) string {
	if int(sl.Mode) < len(modeNames) {
		return modeNames[sl.Mode]
	}
	return ""
}
