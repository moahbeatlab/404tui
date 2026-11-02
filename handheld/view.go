package handheld

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"
)

// Render draws the frame for the state of the app.
func (a *App) Render() *image.RGBA {
	c := newCanvas()
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
	x := 76
	for i, n := range screenNames {
		on := Screen(i) == a.scr
		col := cDim
		if on {
			col = cAmber
		}
		c.text(x, 6, n, col, on)
		if on {
			c.rect(x-4, 24, textW(n)+8, 3, cAmber)
		}
		x += textW(n) + 24
	}
	right := fmt.Sprintf("%s  %c", a.ProjectName(), 'A'+rune(a.bank))
	c.text(W-margin-textW(right), 6, right, cText, false)
	c.rect(0, 28, W, 1, cLine)
}

// footer shows the keys of the screen: always on screen, because there is no keyboard to look them up on.
func (a *App) footer(c *canvas) {
	c.rect(0, 436, W, 1, cLine)
	c.rect(0, 437, W, H-437, cPanel)
	if a.msg != "" {
		col := cGood
		if a.msgBad {
			col = cBad
		}
		c.text(margin, 410, a.msg, col, true)
	}
	l1, l2 := a.legend()
	c.text(margin, 442, l1, cText, false)
	c.text(margin, 460, l2, cDim, false)
}

func (a *App) legend() (string, string) {
	shift := "Y+<> screen"
	switch a.scr {
	case Pads:
		if a.carry != nil {
			return "A put it down here  B cancel  arrows move", "pad " + a.label(a.carry.from) + " is in your hand"
		}
		return "A menu  X play  B stop  arrows move", shift + "  Y+^v bank  Y+B clear  Y help"
	case Files:
		if e, ok := a.current(); ok && e.project {
			return "A load this project  > look inside  < up  ^v browse", shift + "  Y+X places  B pads"
		}
		return "A to pad " + a.label(a.padN()) + "  X listen  ^v browse  > open  < up", shift + "  Y+X places  B pads"
	case Slice:
		return "A cut  B join  X play  <> move (hold: faster)  ^v marker", shift + "  Y+A auto  Y+X to pads  Y+^v zoom"
	case Seq:
		if a.seqOn {
			return "A note  X stop  B stop  arrows move", shift + "  Y+A velocity  Y+^v pattern  Y+X bank"
		}
		return "A note  X play  B menu  arrows move", shift + "  Y+A velocity  Y+^v pattern  Y+X bank"
	case Unit:
		return "A send the project  ^v choose a unit project  B back", shift
	default:
		return "A write project  <> slot  X quit  B back", shift + "  (Select+Start quits anywhere)"
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
	a.padDetail(c)
}

func (a *App) padDetail(c *canvas) {
	x, w := 416, 208
	n := a.padN()
	p := a.info.Pads[n]
	c.big(x, 44, a.label(n), cAmber, 3)
	if !p.File {
		c.text(x, 110, "empty", cDim, false)
		c.text(x, 134, "A opens the menu", cDim, false)
		c.text(x, 152, "to pick a sample", cDim, false)
		return
	}
	c.text(x, 110, fit(p.Name, 24), cText, true)
	ch := "mono"
	if p.Ch == 2 {
		ch = "stereo"
	}
	c.text(x, 130, fmt.Sprintf("%.2f s  %s  lvl %d", float64(p.Frames)/48000, ch, p.Level), cDim, false)
	c.rect(x, 158, w, 72, cPanel)
	if id, ok := a.padSample(n); ok {
		a.waveform(c, id, 0, p.Frames, x+2, 158, w-4, 72, cAmber)
	}
	used := 0
	for _, q := range a.info.Pads {
		if q.File {
			used++
		}
	}
	c.text(x, 250, fmt.Sprintf("%d of 160 pads used", used), cDim, false)
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
	c.text(margin, 38, short2(a.dir, 48), cDim, false)
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
			c.rect(margin-4, y-1, 392, 17, cAmberD)
			c.rect(margin-4, y-1, 3, 17, cAmber)
		}
		name := fit(e.name, 34)
		col := cText
		if e.dir {
			name = fit(e.name, 33) + "/"
			col = cAmber
		}
		if e.project {
			name = short(e.name, 24) + "  project"
			col = cGood
		}
		c.text(margin+4, y, name, col, e.dir)
		if !e.dir {
			sz := fmt.Sprintf("%dK", (e.size+512)/1024)
			c.text(margin+4+392-textW(sz)-12, y, sz, cDim, false)
		}
	}
	// preview
	x, w := 416, 208
	c.text(x, 44, "TARGET PAD", cDim, false)
	c.big(x, 62, a.label(a.padN()), cAmber, 3)
	if e, ok := a.current(); ok && !e.dir {
		c.text(x, 130, fit(e.name, 24), cText, true)
		if id, err := a.decode(e); err == nil {
			s, _ := a.lab.Sample(id)
			c.text(x, 150, fmt.Sprintf("%.2f s", float64(s.Frames())/48000), cDim, false)
			c.rect(x, 176, w, 80, cPanel)
			a.waveform(c, id, 0, s.Frames(), x+2, 176, w-4, 80, cText)
		} else {
			c.text(x, 150, fit(err.Error(), 24), cBad, false)
		}
	}
}

// ---- SLICE

func (a *App) drawSlice(c *canvas) {
	sl := a.sl
	s, _ := a.lab.Sample(a.sid)
	sec := func(f int) float64 { return float64(f) / 48000 }
	name := a.info.Pads[a.spad].Name
	c.text(margin, 38, fmt.Sprintf("%s  %s", a.label(a.spad), fit(name, 24)), cAmber, true)
	hdr := fmt.Sprintf("%.2f s   %d slices   zoom %dx", sec(sl.Frames), len(sl.Slices), 1<<a.zoom)
	c.text(W-margin-textW(hdr), 38, hdr, cDim, false)

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
	// chips
	for i := range sl.Slices {
		if i >= 32 {
			break
		}
		x, y := margin+(i%16)*38, 384+(i/16)*20
		col, fg := cPanel2, cDim
		if i == sel {
			col, fg = cAmber, cBg
		}
		c.rect(x, y, 34, 17, col)
		c.text(x+(34-textW(fmt.Sprint(i+1)))/2, y, fmt.Sprint(i+1), fg, true)
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
	c.text(margin, 120, fit(dest, 76), cText, false)
	c.text(margin, 150, fmt.Sprintf("%d pads with samples    %.1f MB of audio    %d files", used, float64(bytesN)/1e6, a.info.Files), cText, false)
	c.rect(margin, 180, W-2*margin, 1, cLine)
	if a.wrote == nil {
		c.rect(margin, 200, 260, 44, cAmber)
		c.text(margin+16, 214, "A  WRITE TO THE CARD", cBg, true)
		c.text(margin, 262, "Left/Right changes the project number.", cDim, false)
		c.text(margin, 280, "An existing folder with that number is replaced.", cDim, false)
		return
	}
	c.text(margin, 196, "WRITTEN", cGood, true)
	show := a.wrote
	if len(show) > 9 {
		show = append(append([]string{}, show[:8]...), fmt.Sprintf("... and %d more", len(a.wrote)-8))
	}
	for i, n := range show {
		c.text(margin, 220+i*17, "  "+n, cText, false)
	}
	c.text(margin, 372, "Put the card in the SP-404MKII. Not yet tried on a real unit.", cDim, false)
}

// Clock is for the playing-pad flash: a screen is a function of the state, not of the time, so tests are repeatable.
var _ = time.Now
var _ = strings.TrimSpace
