package handheld

import (
	"fmt"
	"github.com/moahbeatlab/404tui/formats"
	"image"
	"image/color"
	"path/filepath"
)

// Render draws the frame for the state of the app.
func (a *App) Render() *image.RGBA { return a.paint().img }

// Frame is a rendered screen: the pixels, and the letters on them by cell (Cols x Rows), so a terminal can show text as
// text.
type Frame struct {
	Img        *image.RGBA // the frame
	Base       *image.RGBA // the frame without its letters
	Text       []Glyph
	Cols, Rows int
}

// RenderFrame draws the screen as Render does and also gives the letters by cell.
func (a *App) RenderFrame() Frame {
	c := a.paint()
	return Frame{Img: c.img, Base: c.base, Text: c.txt, Cols: Cols, Rows: Rows}
}

func (a *App) paint() *canvas {
	c := newCanvas()
	if a.splash != nil {
		a.drawSplash(c)
		return c
	}
	a.titleBar(c)
	switch a.scr {
	case Pads:
		a.drawPads(c)
	case Files:
		a.drawFiles(c)
	case Slice:
		a.drawSlice(c)
	case Seq:
		a.drawSeq(c)
	case Save:
		a.drawSave(c)
	case Unit:
		a.drawUnit(c)
	}
	if a.picker != nil {
		a.drawPicker(c)
	}
	if a.kb != nil {
		a.drawKeyboard(c)
	}
	if a.pp != nil {
		a.drawPadPanel(c)
	}
	if a.np != nil {
		a.drawNotePanel(c)
	}
	a.drawPanel(c)
	a.footer(c)
	if a.menu != nil {
		a.drawMenu(c)
	}
	if a.help {
		a.drawHelp(c)
	}
	return c
}

func (a *App) titleBar(c *canvas) {
	c.rect(0, 0, W, CellH, cPanel)
	c.rect(0, 0, 8*CellW, CellH, cAmber)
	c.cells(1, 0, "404tui", cBg, true)
	c.cells(9, 0, screenNames[a.scr], cAmber, true)
	right := fmt.Sprintf("%s  bank %c", a.ProjectName(), 'A'+rune(a.bank))
	c.text(W-margin-textW(right), 0, fit(right, Cols-20), cText, false)
}

// Terminal is set when the app is shown in a terminal and played from the keyboard: the hint line and the help say so.
var Terminal bool

// Version is the commit the program was built from, shown in the corner so it is always known which one runs.
var Version string

// footer is the message line and the hint line; the keys are in the panel.
func (a *App) footer(c *canvas) {
	c.rect(0, panelBot, W, 1, cLine)
	c.rect(0, panelBot+1, W, H-panelBot-1, cPanel)
	if a.msg != "" {
		col := cGood
		if a.msgBad {
			col = cBad
		}
		c.text(margin, panelBot, fit(a.msg, (W-2*margin)/CellW), col, true)
	} else if a.scr == Seq && a.menu == nil && a.picker == nil {
		// no message: what is known of the note under the cursor
		if info := a.noteInfo(); info != "" {
			c.text(margin, panelBot, fit(info, (W-2*margin)/CellW), cText, false)
		}
	}
	hintLine := "Y keys  Y+L2 undo  Y+R2 redo  Sel+Start quit"
	if Terminal {
		hintLine = "y shift  s select  e hold A  u undo  ? keys  q quit"
	}
	c.text(margin, panelBot+CellH, hintLine, cDim, false)
	c.text(margin+textW(hintLine)+16, panelBot+CellH, a.autosaveAgo(), cDim, false)
	if Version != "" {
		c.text(W-margin-textW(Version), panelBot+CellH, Version, cDim, false)
	}
}

func (a *App) drawHelp(c *canvas) {
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
		"PADS  SEQ  FILES  SLICE  SAVE  UNIT",
		"SAVE writes a folder for the SD card; UNIT sends",
		"the project over USB to an SP-404MKII.",
	}
	if Terminal {
		rows = []string{
			"arrows or h j k l   D-pad (as in vim)",
			"a b x              the buttons A, B, X (Enter, Esc, Space too)",
			"y                  the next key with Y held",
			"s or Tab          the next key with Select held (s b)",
			"e                  hold A (edit a note); e again lets go",
			"[ ] or Ctrl-U/D    L1 R1;   , .   L2 R2",
			"?                  this page;   /  search the files",
			fmt.Sprintf("u undo (%d)   Ctrl-R redo (%d)", len(a.undo), len(a.redo)),
			"q or Ctrl-C        quit (twice when not saved)",
			"",
			"PADS  SEQ  FILES  SLICE  SAVE  UNIT",
			"The side panel shows the keys that work now.",
		}
	}
	cols, h := min(contentCols-4, 60), len(rows)+4
	x, y := (contentCols-cols)/2*CellW, max(1, (Rows-h)/2-1)*CellH
	c.rect(x, y, cols*CellW, h*CellH, cPanel2)
	c.box(x, y, cols*CellW, h*CellH, 2, cAmber)
	c.text(x+2*CellW, y+CellH, "KEYS  (D-pad, A B X Y, L1 R1 L2 R2)", cAmber, true)
	for i, r := range rows {
		c.text(x+2*CellW, y+(i+2)*CellH, fit(r, cols-4), cText, false)
	}
}

// ---- PADS

// drawPads is the 4x4 pads of the bank, as big as the room allows, and the banks under them. A pad is a block of cells with a
// cell between it and the next.
func (a *App) drawPads(c *canvas) {
	cols := contentCols - 2
	pw := cols / 4 // a pad's pitch in cells
	x0 := CellW * (1 + (cols-4*pw)/2)
	ph := max((Rows-6)/4, 3) // the rows the pads have: what is left of the body, less the banks (2) and a gap
	tw, th := (pw-1)*CellW, (ph-1)*CellH
	for i := 0; i < 16; i++ {
		n := a.bank*16 + i
		x, y := x0+(i%4)*pw*CellW, CellH+(i/4)*ph*CellH
		p := a.info.Pads[n]
		bg := cPanel
		if p.File {
			bg = cPanel2
		}
		if a.playing == n {
			bg = cAmberD
		}
		c.rect(x, y, tw, th, bg)
		num := fmt.Sprint(i + 1)
		c.text(x+tw-textW(num)-CellW/2, y, num, cDim, false)
		if p.File {
			c.rect(x, y, 4, th, cAmber)
			c.text(x+CellW, y, short(p.Name, pw-5), cText, true)
			if id, ok := a.padSample(n); ok && th >= 3*CellH { // an even number of rows, so the middle line is between two
				a.waveform(c, id, 0, p.Frames, x+CellW, y+CellH, tw-2*CellW, (ph-2)/2*2*CellH, cAmber)
			}
			// the switches that are on: G for gate, L for loop
			cx := x + tw - textW(num) - CellW
			for _, sw := range []struct {
				id byte
				ch string
			}{{0x6B, "L"}, {0x6A, "G"}} {
				if v, err := a.lab.PadParam(n, sw.id); err == nil && v != 0 {
					cx -= 2 * CellW
					a.badge(c, cx, y, sw.ch, true)
				}
			}
		}
		for mi, mp := range a.marks { // a pad that is a key of the arp: its place among the keys
			if mp == n {
				c.rect(x+CellW/2, y+th-CellH, 2*CellW, CellH, cAmber)
				c.text(x+CellW/2+4, y+th-CellH, fmt.Sprint(mi+1), cBg, true)
			}
		}
		if i == a.cur {
			c.box(x, y, tw, th, 2, cWhite)
		}
	}
	// the banks
	by := (Rows - 4) * CellH
	bw := (contentCols - 2) / 10 * CellW
	bx0 := CellW * (1 + (contentCols-2-(contentCols-2)/10*10)/2)
	for b := 0; b < 10; b++ {
		used := 0
		for i := 0; i < 16; i++ {
			if a.info.Pads[b*16+i].File {
				used++
			}
		}
		x := bx0 + b*bw
		col, fg := cPanel, cDim
		if b == a.bank {
			col, fg = cAmber, cBg
		}
		c.rect(x, by, bw-CellW, CellH, col)
		c.text(x+(bw-CellW-CellW)/2, by, string(rune('A'+b)), fg, true)
		c.rect(x, by+CellH, bw-CellW, 4, cLine)
		c.rect(x, by+CellH, used*(bw-CellW)/16, 4, cAmber)
	}
}

// waveform draws the peaks of frames from..to of a sample in a box, mirrored around its middle, in bars 2 pixels wide (a
// terminal has 8 pixels to a cell, so a bar is a quarter of one and a cell's bars add up to a bar chart).
func (a *App) waveform(c *canvas, id, from, to, x, y, w, h int, col color.RGBA) {
	pk, err := a.peaks(id, from, to, w/2)
	if err != nil {
		return
	}
	mid := y + h/2
	c.rect(x, mid, w, 1, cLine)
	for i, p := range pk {
		v := p * (h / 2) / 32768
		c.rect(x+2*i, mid-v, 2, 2*v+1, col)
	}
}

// ---- FILES

func (a *App) drawFiles(c *canvas) {
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
	c.cells(2, 1, fit(head, inner), col, a.searchQ != "")
	rows := a.fileRows()
	top := max(0, min(a.fcur-rows/2, len(a.entries)-rows))
	if len(a.entries) == 0 {
		c.cells(2, 3, "(no folders or audio files here)", cDim, false)
		if a.dir == a.Root {
			c.cells(2, 5, "Put .wav files in this folder:", cText, false)
			c.cells(2, 6, fit(a.Root, inner), cAmber, false)
		}
		c.cells(2, 8, "< goes up    Y+X jumps to the usb drive", cDim, false)
	}
	for i := 0; i < rows && top+i < len(a.entries); i++ {
		e := a.entries[top+i]
		y := (2 + i) * CellH
		if top+i == a.fcur {
			c.rect(margin-4, y, contentW-margin, CellH, cAmberD)
			c.rect(margin-4, y, 3, CellH, cAmber)
		}
		avail := inner - 8
		name := fit(e.name, avail)
		if a.searchQ != "" { // a result is a path below the folder: its end says which file
			name = short2(e.name, avail)
		}
		col := cText
		if e.dir {
			name = fit(e.name, avail-1) + "/"
			col = cAmber
		}
		if e.project {
			name = short(e.name, min(24, avail-9)) + "  project"
			col = cGood
		}
		c.text(margin+CellW/2, y, name, col, e.dir)
		if !e.dir {
			sz := fmt.Sprintf("%dK", (e.size+512)/1024)
			c.text(contentW-textW(sz)-12, y, sz, cDim, false)
		}
	}
}

// fileRows is how many files the list shows: the body, less the line of the folder.
func (a *App) fileRows() int { return Rows - 4 }

// ---- SLICE

func (a *App) drawSlice(c *canvas) {
	sl := a.sl
	sec := func(f int) float64 { return float64(f) / 48000 }
	name := a.info.Pads[a.spad].Name
	c.cells(2, 1, fmt.Sprintf("%s  %s", a.label(a.spad), fit(name, 40)), cAmber, true)

	wx, wy := margin, 2*CellH+4
	wh := (a.sliceWaveRows())*CellH - 8
	from, to := a.view()
	c.rect(wx-4, wy-4, waveCols+8, wh+8, cPanel)
	sel := a.sliceAt(a.at)
	px := func(f int) int { return wx + (f-from)*waveCols/max(to-from, 1) }
	if sel >= 0 {
		x0, x1 := max(px(sl.Slices[sel].Start), wx), min(px(sl.Slices[sel].End), wx+waveCols)
		if x1 > x0 {
			c.rect(x0, wy, x1-x0, wh, cAmberD)
		}
	}
	a.waveform(c, a.sid, from, to, wx, wy, waveCols, wh, cText)
	for i, sg := range sl.Slices {
		if sg.Start < from || sg.Start >= to {
			continue
		}
		x := px(sg.Start)
		c.vline(x, wy, wy+wh, cAmber)
		c.text(x+3, wy, fmt.Sprint(i+1), cAmber, true)
	}
	cx := px(a.at)
	c.vline(cx, wy-4, wy+wh+3, cWhite)
	c.rect(cx-3, wy-4, 7, 3, cWhite)

	// the overview, with the window in it
	oy := (2 + a.sliceWaveRows()) * CellH
	c.rect(wx-4, oy, waveCols+8, 2*CellH, cPanel)
	a.waveform(c, a.sid, 0, sl.Frames, wx, oy+2, waveCols, 2*CellH-4, cDim)
	for _, sg := range sl.Slices {
		c.vline(wx+sg.Start*waveCols/sl.Frames, oy+2, oy+2*CellH-4, cAmber)
	}
	c.box(wx+from*waveCols/sl.Frames, oy, max((to-from)*waveCols/sl.Frames, 2), 2*CellH, 1, cWhite)

	// the numbers
	ny := oy + 2*CellH
	if sel >= 0 {
		sg := sl.Slices[sel]
		c.text(margin, ny, fit(fmt.Sprintf("SLICE %d/%d   %.3f - %.3f s   %.3f s long", sel+1, len(sl.Slices), sec(sg.Start), sec(sg.End), sec(sg.End-sg.Start)), contentCols-4), cText, true)
	}
	c.text(margin, ny+CellH, fmt.Sprintf("cursor %.3f s", sec(a.at)), cDim, false)
	// the order as it plays: one cell per step, the slice's number in it, whatever the mode and the grid make of the order. A
	// step that stays silent (chance) is dim, the slices under the cursor are lit, the step that sounds is amber.
	plan := a.slicePlan()
	lbl := fmt.Sprintf("ORDER  %s  %d steps", orderMode(a.sl), len(plan))
	if a.sl.Division > 0 {
		lbl += fmt.Sprintf("  1/%d at %s BPM", a.sl.Division, bpmText(a.tempoOf(a.spad/16)))
	}
	c.text(contentW-textW(lbl)-8, ny+CellH, fit(lbl, contentCols-24), cDim, false)
	per := max((contentCols-4)/4, 1) // a step is 3 cells and a gap
	now := a.playingStep()
	cellRows := Rows - 3 - (ny/CellH + 2) + 1 // what is left under the numbers
	for i, st := range plan {
		if i >= cellRows*per {
			c.text(margin+(per-1)*4*CellW, ny+2*CellH+(cellRows-1)*CellH, fmt.Sprintf("+%d", len(plan)-cellRows*per+1), cDim, false)
			break
		}
		x, y := margin+(i%per)*4*CellW, ny+2*CellH+(i/per)*CellH
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
		c.rect(x, y, 3*CellW, CellH, col)
		t := fmt.Sprint(st.Slice + 1)
		if st.Rest {
			t = "-" + t
		}
		c.text(x+(3*CellW-textW(t))/2, y, t, fg, true)
	}
}

// sliceWaveRows is the rows of the big waveform: the body less the title, the overview (2 rows) and the numbers and
// the order under it (4 rows).
func (a *App) sliceWaveRows() int { return max(Rows-3-1-2-4, 6) &^ 1 }

// ---- SAVE

func (a *App) drawSave(c *canvas) {
	used, bytesN := 0, 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
			bytesN += p.Frames * p.Ch * 2
		}
	}
	inner := contentCols - 4
	c.cells(2, 2, "WRITE THE PROJECT", cDim, false)
	c.cells(2, 4, fit(a.ProjectName(), inner), cAmber, true)
	dest := filepath.Join(a.OutDir, cardFolder, a.ProjectName())
	c.cells(2, 7, short2(dest, inner), cText, false)
	c.cells(2, 9, fmt.Sprintf("%d pads with samples   %.1f MB   %d files", used, float64(bytesN)/1e6, a.info.Files), cText, false)
	c.rect(margin, 11*CellH+8, contentW-2*margin, 1, cLine)
	if a.wrote == nil {
		c.rect(margin, 12*CellH+8, 24*CellW, 3*CellH, cAmber)
		c.cells(4, 13, "A  WRITE TO THE CARD", cBg, true)
		c.cells(2, 16, "Left/Right changes the project number.", cDim, false)
		c.cells(2, 17, "An existing folder with that number is replaced.", cDim, false)
		a.drawAutosave(c)
		return
	}
	c.cells(2, 12, "WRITTEN", cGood, true)
	show := a.wrote
	if len(show) > 5 {
		show = append(append([]string{}, show[:4]...), fmt.Sprintf("... and %d more", len(a.wrote)-4))
	}
	for i, n := range show {
		c.cells(2, 13+i, fit("  "+n, inner), cText, false)
	}
	c.cells(2, 19, "Put the card in the SP-404MKII.", cDim, false)
	a.drawAutosave(c)
}

// drawAutosave is the interval and the log of the autosave, at the foot of the SAVE screen.
func (a *App) drawAutosave(c *canvas) {
	y0 := Rows - 9
	c.rect(margin, y0*CellH-4, contentW-2*margin, 1, cLine)
	c.cells(2, y0, "AUTOSAVE", cDim, false)
	c.text(margin+textW("AUTOSAVE")+12, y0*CellH, a.autosaveSetting(), cAmber, true)
	c.text(contentW-margin-textW("^v changes"), y0*CellH, "^v changes", cDim, false)
	a.as.mu.Lock()
	h := append([]string{}, a.as.history...)
	a.as.mu.Unlock()
	if len(h) == 0 {
		c.cells(2, y0+2, "nothing yet: it saves when the project has changed", cDim, false)
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
		c.text(margin, (y0+2+i)*CellH, fit(l, (contentW-2*margin)/CellW), col, false)
	}
}

// orderMode is how the order is played, for the label above it.
func orderMode(sl *formats.Slicing) string {
	if int(sl.Mode) < len(modeNames) {
		return modeNames[sl.Mode]
	}
	return ""
}
