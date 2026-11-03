package handheld

import (
	"encoding/json"
	"fmt"

	"github.com/moahbeatlab/404tui/lab"
)

// The note panel (B in SEQ): the settings of the note under the cursor as horizontal faders (velocity, length, timing off the
// grid), as the pad panel has for a pad. The pattern tools are a menu of their own (Select+B). A changes a fader back to its
// default, X hears the note, B closes. On a step with no note A puts one there.

type notePanel struct {
	pad  int
	tick uint32 // where the note is now (it moves with the timing fader)
	row  int
}

var notePanelRows = []struct {
	label string
	def   int
}{{"Velocity", 100}, {"Length", stepTicks}, {"Timing", 0}}

// noteHere is the note on the cursor's step.
func (a *App) noteHere() (lab.Note, bool) {
	d, ok := a.doc()
	if !ok {
		return lab.Note{}, false
	}
	return noteAt(d, a.rowPad(), a.step)
}

func (a *App) openNotePanel() {
	np := &notePanel{pad: a.rowPad()}
	if n, ok := a.noteHere(); ok {
		np.tick = n.Tick
	} else {
		np.tick = ^uint32(0)
	}
	a.np = np
}

// npNote finds the note the panel is about.
func (a *App) npNote() (lab.Note, bool) {
	np := a.np
	if np.tick == ^uint32(0) {
		return lab.Note{}, false
	}
	d, ok := a.doc()
	if !ok {
		return lab.Note{}, false
	}
	for _, n := range d.Notes {
		if n.Pad == np.pad && n.Tick == np.tick {
			return n, true
		}
	}
	return lab.Note{}, false
}

// gridOff is how far a note is from the nearest step, in ticks (late is plus).
func gridOff(tick uint32) int {
	off := int(tick) % stepTicks
	if off > stepTicks/2 {
		off -= stepTicks
	}
	return off
}

// editNote changes a note: velocity, length (ticks) and a move (ticks); it says where the note is after.
func (a *App) editNote(pad int, tick uint32, vel, ln, move int) (uint32, bool) {
	d, ok := a.doc()
	if !ok {
		return tick, false
	}
	vel = min(max(vel, 1), 127)
	ln = min(max(ln, 15), max(int(d.Length)-int(tick), 15))
	move = min(max(move, -int(tick)), int(d.Length)-1-int(tick))
	args, _ := json.Marshal(lab.PatternArgs{Notes: []lab.PadAt{{Pad: pad, Tick: tick}}, Vel: vel, Len: ln, Move: move})
	if err := a.lab.PatternOp(a.pat, "edit", args); err != nil {
		a.say(err.Error(), true)
		return tick, false
	}
	a.refresh()
	a.replay()
	return uint32(int(tick) + move), true
}

func (a *App) npKey(k Key, repeat int) {
	np := a.np
	if (a.selHeld && k == B) || k == R1 || k == L1 { // the pattern tools, a menu of their own
		a.np = nil
		a.seqMenu()
		return
	}
	switch k {
	case B:
		a.np = nil
		return
	case Up:
		np.row = (np.row + len(notePanelRows) - 1) % len(notePanelRows)
		return
	case Down:
		np.row = (np.row + 1) % len(notePanelRows)
		return
	case X:
		if n, ok := a.npNote(); ok {
			if s, ok := a.noteSample(n.Pad, n.Vel, n.Len); ok {
				a.play(s)
			}
		}
		return
	}
	n, ok := a.npNote()
	if !ok {
		if k == A { // no note on the step: A puts one there
			a.toggle(np.pad, a.step)
			np.tick = uint32(a.step * stepTicks)
		}
		return
	}
	acc := accel(repeat)
	vel, ln, move := n.Vel, n.Len, 0
	d := 1
	switch k {
	case Left:
		d = -1
	case Right:
	case A: // back to the default
		switch np.row {
		case 0:
			vel = notePanelRows[0].def
		case 1:
			ln = notePanelRows[1].def
		case 2:
			move = -gridOff(n.Tick)
		}
		d = 0
	default:
		return
	}
	if d != 0 {
		switch np.row {
		case 0:
			vel += d * acc
		case 1:
			ln += d * 15 * acc
		case 2:
			move = d * 5 * acc
		}
	}
	if tick, ok := a.editNote(n.Pad, n.Tick, vel, ln, move); ok {
		np.tick = tick
		a.step = int(tick) / stepTicks
		if n2, ok := a.npNote(); ok {
			if s, ok := a.noteSample(n2.Pad, n2.Vel, n2.Len); ok && np.row != 2 {
				a.play(s)
			}
		}
	}
}

// drawNotePanel docks the panel at the bottom of the grid, or at the top when the cursor's row is down there.
func (a *App) drawNotePanel(c *canvas) {
	np := a.np
	wc, hr := contentCols-2, 10
	x0, w, h := CellW, wc*CellW, hr*CellH
	y0 := panelBot - h
	if a.row >= 0 {
		if rowY := 3*CellH + (a.row-a.rowTop)*a.seqRowPx(); rowY+a.seqRowPx() > y0 {
			y0 = CellH // the cursor's row is low: the panel goes to the top
		}
	}
	c.rect(x0, y0, w, h, cPanel2)
	c.box(x0, y0, w, h, 2, cAmber)
	n, ok := a.npNote()
	title := fmt.Sprintf("NOTE  %s  %d.%d", a.label(np.pad), a.step/16+1, a.step%16+1)
	c.text(x0+2*CellW, y0+CellH, title, cAmber, true)
	if !ok {
		c.text(x0+2*CellW, y0+3*CellH, "no note on this step", cDim, false)
		c.text(x0+2*CellW, y0+4*CellH, "A puts one here", cText, false)
		c.text(x0+2*CellW, y0+h-2*CellH, "A note   B close   R1 pattern tools", cDim, false)
		return
	}
	vals := []int{n.Vel, n.Len, gridOff(n.Tick)}
	for i, row := range notePanelRows {
		y := y0 + 2*CellH + i*2*CellH
		if i == np.row {
			c.rect(x0+6, y, w-12, 2*CellH, cAmberD)
			c.rect(x0+6, y, 3, 2*CellH, cAmber)
		}
		ly := y + CellH/2
		c.text(x0+2*CellW, ly, row.label, cText, i == np.row)
		bx, bw := x0+14*CellW, max(min(wc-14-14, 40), 12)*CellW
		var frac, dfrac float64
		var vt string
		switch i {
		case 0:
			frac, dfrac = float64(vals[0]-1)/126, float64(row.def-1)/126
			vt = fmt.Sprint(vals[0])
		case 1:
			maxLen := 8 * stepTicks
			frac, dfrac = float64(min(vals[1], maxLen))/float64(maxLen), float64(row.def)/float64(maxLen)
			vt = fmt.Sprintf("%.2f steps", float64(vals[1])/stepTicks)
		case 2:
			frac, dfrac = float64(vals[2]+stepTicks/2)/stepTicks, 0.5
			vt = "on the grid"
			if vals[2] != 0 {
				vt = fmt.Sprintf("%+d ticks", vals[2])
			}
		}
		c.rect(bx, ly+3, bw, 10, cPanel)
		if i == 2 { // from the middle out
			mid, end := bx+int(dfrac*float64(bw)), bx+int(frac*float64(bw))
			c.rect(min(mid, end), ly+3, abs(end-mid)+1, 10, cAmber)
		} else {
			c.rect(bx, ly+3, int(frac*float64(bw)), 10, cAmber)
		}
		c.rect(bx+int(dfrac*float64(bw)), ly, 2, CellH, cDim)
		c.rect(min(bx+int(frac*float64(bw))-1, bx+bw-4), ly, 4, CellH, cWhite)
		c.text(bx+bw+CellW, ly, vt, cAmber, true)
	}
	c.text(x0+2*CellW, y0+h-2*CellH, fit("<> change  A default  X hear  B close  R1 tools", wc-4), cDim, false)
}
