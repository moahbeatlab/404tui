package handheld

import (
	"fmt"
	"github.com/moahbeatlab/404tui/formats"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"
)

// Render draws the frame for the state of the app.
func (a *App) Render() *image.RGBA {
	c := newCanvas()
	if a.splash != nil {
		a.drawSplash(c)
		return c.img
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
	a.drawPanel(c)
	a.footer(c)
	if a.menu != nil {
		a.drawMenu(c)
	}
	if a.help {
		a.drawHelp(c)
	}
	return c.img
}

func (a *App) titleBar(c *canvas) {
	c.rect(0, 0, W, 28, cPanel)
	c.rect(0, 0, 56, 28, cAmber)
	c.text(8, 6, "404h", cBg, true)
	c.big(72, 6, screenNames[a.scr], cAmber, 1)
	right := fmt.Sprintf("%s  bank %c", a.ProjectName(), 'A'+rune(a.bank))
	c.text(W-margin-textW(right), 6, right, cText, false)
	c.rect(0, 28, W, 1, cLine)
}

// Version is the commit the program was built from, shown in the corner so it is always known which one runs.
var Version string

// footer is the message line; the keys are in the panel.
func (a *App) footer(c *canvas) {
	c.rect(0, panelBot, W, 1, cLine)
	c.rect(0, panelBot+1, W, H-panelBot-1, cPanel)
	if a.msg != "" {
		col := cGood
		if a.msgBad {
			col = cBad
		}
		c.text(margin, panelBot+8, fit(a.msg, (W-2*margin)/CellW), col, true)
	}
	c.text(margin, panelBot+28, "Y alone: keys   Select+Start: quit", cDim, false)
	c.text(margin+textW("Y alone: keys   Select+Start: quit")+24, panelBot+28, a.autosaveAgo(), cDim, false)
	if Version != "" {
		c.text(W-margin-textW(Version), panelBot+28, Version, cDim, false)
	}
}

func (a *App) drawHelp(c *canvas) {
	c.rect(60, 70, 520, 300, cPanel2)
	c.box(60, 70, 520, 300, 2, cAmber)
	c.text(80, 82, "KEYS  (only D-pad, A B X Y)", cAmber, true)
	rows := []string{
		"Y held   shift: the second function of a key",
		"Y+< >    previous / next screen",
		"Y+^ v    PADS bank   SLICE zoom   SEQ pattern",
		"A        do / menu / put a note / cut",
		"B        back / stop / join / SEQ menu",
		"X        listen to what is under the cursor",
		"Y alone  this page",
		"Select+Start  quit (asks when not saved)",
		"",
		"PADS  FILES  SLICE  SEQ  SAVE  UNIT",
		"SAVE writes a folder for the SD card; UNIT sends",
		"the project over USB to an SP-404MKII (OTG port).",
	}
	for i, r := range rows {
		c.text(80, 112+i*18, r, cText, false)
	}
}

// ---- PADS

func (a *App) drawPads(c *canvas) {
	const tw, th, gap, x0, y0 = 92, 72, 6, 16, 44
	for i := 0; i < 16; i++ {
		n := a.bank*16 + i
		x, y := x0+(i%4)*(tw+gap), y0+(i/4)*(th+gap)
		p := a.info.Pads[n]
		bg := cPanel
		if p.File {
			bg = cPanel2
		}
		if a.playing == n {
			bg = cAmberD
		}
		c.rect(x, y, tw, th, bg)
		if p.File {
			c.rect(x, y, 4, th, cAmber)
			c.text(x+10, y+18, short(p.Name, 9), cText, true)
			if id, ok := a.padSample(n); ok {
				a.waveform(c, id, 0, p.Frames, x+10, y+38, tw-20, 26, cAmber)
			}
		}
		num := fmt.Sprint(i + 1)
		c.text(x+tw-textW(num)-6, y+4, num, cDim, false)
		for mi, mp := range a.marks { // a pad that is a key of the arp: its place among the keys
			if mp == n {
				c.rect(x+4, y+4, 20, 14, cAmber)
				c.text(x+7, y+3, fmt.Sprint(mi+1), cBg, true)
			}
		}
		if i == a.cur {
			c.box(x-2, y-2, tw+4, th+4, 2, cWhite)
		}
	}
	// the banks
	by := 372
	c.text(margin, by-18, "BANKS", cDim, false)
	for b := 0; b < 10; b++ {
		used := 0
		for i := 0; i < 16; i++ {
			if a.info.Pads[b*16+i].File {
				used++
			}
		}
		x := margin + b*39
		col, fg := cPanel, cDim
		if b == a.bank {
			col, fg = cAmber, cBg
		}
		c.rect(x, by, 35, 24, col)
		c.text(x+13, by+4, string(rune('A'+b)), fg, true)
		c.rect(x, by+27, 35, 3, cLine)
		c.rect(x, by+27, used*35/16, 3, cAmber)
	}
}

// waveform draws the peaks of frames from..to of a sample in a box, mirrored around its middle.
func (a *App) waveform(c *canvas, id, from, to, x, y, w, h int, col color.RGBA) {
	pk, err := a.lab.Peaks(id, from, to, w)
	if err != nil {
		return
	}
	mid := y + h/2
	c.rect(x, mid, w, 1, cLine)
	for i, p := range pk {
		v := p * (h / 2) / 32768
		c.vline(x+i, mid-v, mid+v, col)
	}
}

// ---- FILES

func (a *App) drawFiles(c *canvas) {
	head := short2(a.dir, 48)
	if a.isFavourite(a.dir) {
		head = "* " + short2(a.dir, 46)
	}
	col := cDim
	if a.searchQ != "" {
		head = fmt.Sprintf("search %q in %s", a.searchQ, short2(a.dir, 26))
		if a.searchCut {
			head += " (cut short)"
		}
		col = cAmber
	}
	c.text(margin, 38, fit(head, 56), col, a.searchQ != "")
	const rows = 21
	top := max(0, min(a.fcur-rows/2, len(a.entries)-rows))
	if len(a.entries) == 0 {
		c.text(margin, 70, "(no folders or audio files here)", cDim, false)
		if a.dir == a.Root {
			c.text(margin, 94, "Put .wav files in this folder:", cText, false)
			c.text(margin, 112, fit(a.Root, 48), cAmber, false)
		}
		c.text(margin, 140, "< goes up    Y+X jumps to the usb drive", cDim, false)
	}
	for i := 0; i < rows && top+i < len(a.entries); i++ {
		e := a.entries[top+i]
		y := 60 + i*17
		if top+i == a.fcur {
			c.rect(margin-4, y-1, contentW-margin, 17, cAmberD)
			c.rect(margin-4, y-1, 3, 17, cAmber)
		}
		name := fit(e.name, 50)
		if a.searchQ != "" { // a result is a path below the folder: its end says which file
			name = short2(e.name, 50)
		}
		col := cText
		if e.dir {
			name = fit(e.name, 49) + "/"
			col = cAmber
		}
		if e.project {
			name = short(e.name, 24) + "  project"
			col = cGood
		}
		c.text(margin+4, y, name, col, e.dir)
		if !e.dir {
			sz := fmt.Sprintf("%dK", (e.size+512)/1024)
			c.text(contentW-textW(sz)-12, y, sz, cDim, false)
		}
	}
}

// ---- SLICE

func (a *App) drawSlice(c *canvas) {
	sl := a.sl
	s, _ := a.lab.Sample(a.sid)
	sec := func(f int) float64 { return float64(f) / 48000 }
	name := a.info.Pads[a.spad].Name
	c.text(margin, 38, fmt.Sprintf("%s  %s", a.label(a.spad), fit(name, 40)), cAmber, true)

	const wx, wy, wh = 20, 64, 220
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
		c.text(x+3, wy+2, fmt.Sprint(i+1), cAmber, true)
	}
	cx := px(a.at)
	c.vline(cx, wy-4, wy+wh+3, cWhite)
	c.rect(cx-3, wy-4, 7, 3, cWhite)

	// the overview, with the window in it
	oy := 300
	c.rect(wx-4, oy, waveCols+8, 28, cPanel)
	a.waveform(c, a.sid, 0, sl.Frames, wx, oy+2, waveCols, 24, cDim)
	for _, sg := range sl.Slices {
		c.vline(wx+sg.Start*waveCols/sl.Frames, oy+2, oy+26, cAmber)
	}
	c.box(wx+from*waveCols/sl.Frames, oy, max((to-from)*waveCols/sl.Frames, 2), 28, 1, cWhite)

	// the numbers
	if sel >= 0 {
		sg := sl.Slices[sel]
		c.text(margin, 340, fmt.Sprintf("SLICE %d/%d   %.3f - %.3f s   %.3f s long", sel+1, len(sl.Slices), sec(sg.Start), sec(sg.End), sec(sg.End-sg.Start)), cText, true)
	}
	c.text(margin, 360, fmt.Sprintf("cursor %.3f s", sec(a.at)), cDim, false)
	// the play order: the slices as numbers in the order they play, the slice under the cursor lit
	lbl := "ORDER  " + orderMode(a.sl)
	c.text(contentW-textW(lbl)-4, 360, lbl, cDim, false)
	for i, v := range a.sl.Order {
		if i >= 28 {
			c.text(margin+14*34, 404+20, fmt.Sprintf("+%d", len(a.sl.Order)-28), cDim, false)
			break
		}
		x, y := margin+(i%13)*33, 384+(i/13)*20
		col, fg := cPanel2, cDim
		if v == sel {
			col, fg = cAmber, cBg
		}
		c.rect(x, y, 30, 17, col)
		t := fmt.Sprint(v + 1)
		c.text(x+(30-textW(t))/2, y, t, fg, true)
	}
	_ = s
}

// ---- SAVE

func (a *App) drawSave(c *canvas) {
	used, bytesN := 0, 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
			bytesN += p.Frames * p.Ch * 2
		}
	}
	c.text(margin, 44, "WRITE THE PROJECT", cDim, false)
	c.big(margin, 66, a.ProjectName(), cAmber, 3)
	dest := filepath.Join(a.OutDir, cardFolder, a.ProjectName())
	c.text(margin, 120, short2(dest, 55), cText, false)
	c.text(margin, 150, fmt.Sprintf("%d pads with samples   %.1f MB   %d files", used, float64(bytesN)/1e6, a.info.Files), cText, false)
	c.rect(margin, 180, contentW-2*margin, 1, cLine)
	if a.wrote == nil {
		c.rect(margin, 200, 260, 44, cAmber)
		c.text(margin+16, 214, "A  WRITE TO THE CARD", cBg, true)
		c.text(margin, 262, "Left/Right changes the project number.", cDim, false)
		c.text(margin, 280, "An existing folder with that number is replaced.", cDim, false)
		a.drawAutosave(c)
		return
	}
	c.text(margin, 196, "WRITTEN", cGood, true)
	show := a.wrote
	if len(show) > 5 {
		show = append(append([]string{}, show[:4]...), fmt.Sprintf("... and %d more", len(a.wrote)-4))
	}
	for i, n := range show {
		c.text(margin, 220+i*17, "  "+n, cText, false)
	}
	c.text(margin, 312, "Put the card in the SP-404MKII.", cDim, false)
	a.drawAutosave(c)
}

// drawAutosave is the interval and the log of the autosave, at the foot of the SAVE screen.
func (a *App) drawAutosave(c *canvas) {
	c.rect(margin, 332, contentW-2*margin, 1, cLine)
	c.text(margin, 340, "AUTOSAVE", cDim, false)
	c.text(margin+textW("AUTOSAVE")+12, 340, a.autosaveSetting(), cAmber, true)
	c.text(contentW-margin-textW("^v changes"), 340, "^v changes", cDim, false)
	a.as.mu.Lock()
	h := append([]string{}, a.as.history...)
	a.as.mu.Unlock()
	if len(h) == 0 {
		c.text(margin, 362, "nothing yet: it saves when the project has changed", cDim, false)
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
		c.text(margin, 362+i*17, fit(l, (contentW-2*margin)/CellW), col, false)
	}
}

// Clock is for the playing-pad flash: a screen is a function of the state, not of the time, so tests are repeatable.
var _ = time.Now
var _ = strings.TrimSpace

// orderMode is how the order is played, for the label above it.
func orderMode(sl *formats.Slicing) string {
	if int(sl.Mode) < len(modeNames) {
		return modeNames[sl.Mode]
	}
	return ""
}
