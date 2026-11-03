package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// The slicer's extras: tap (cut while the sample plays), the order lane (the order the slices play in, edited step by step), several
// samples in one slicer (joined end to end), and a pattern under the slices (the slices on pads, and a pattern that plays them).

// ---- tap

// startTap plays the whole sample; every A puts a cut where it is playing (moved back to the onset just before it, up to 100 ms), B stops.
func (a *App) startTap() {
	s, err := a.lab.Sample(a.sid)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.lane = false
	a.tap, a.tapT0, a.at = true, time.Now(), 0
	a.play(s)
	a.say("tap: A cuts where it plays, B stops", false)
}

func (a *App) stopTap() {
	if a.tap {
		a.tap = false
		a.stop()
		a.say(fmt.Sprintf("%d slices", len(a.sl.Slices)), false)
	}
}

// tapTick moves the cursor with the sample that plays, and ends the tap at its end.
func (a *App) tapTick() {
	if !a.tap || a.sl == nil {
		return
	}
	pos := int(time.Since(a.tapT0).Seconds() * formats.SampleRate)
	if pos >= a.sl.Frames {
		a.stopTap()
		return
	}
	a.at = pos
}

func (a *App) tapKey(k Key) {
	switch k {
	case A:
		pos := a.at
		at := pos
		if on, err := a.lab.Transients(a.sid, 128, a.autoSensitivity()); err == nil {
			best := -1
			for _, o := range on {
				if o <= pos && pos-o <= formats.SampleRate/10 && o > best {
					best = o
				}
			}
			if best >= 0 {
				at = best
			}
		}
		i := a.sliceAt(at)
		if i < 0 || a.sl.Split(i, at) < 0 {
			a.say("cannot cut there", true)
			return
		}
		a.say(fmt.Sprintf("%d slices (cut at %.3f s)", len(a.sl.Slices), float64(at)/formats.SampleRate), false)
	case B:
		a.stopTap()
	}
}

// ---- the order lane

func (a *App) openLane() {
	a.stopTap()
	a.lane, a.laneAt = true, 0
	if len(a.sl.Order) == 0 {
		a.sl.ResetOrder()
	}
	a.say("order lane: ^v another slice, A repeat, B drop, L1 R1 carry, Y+B leaves", false)
}

func (a *App) laneKey(k Key) {
	o := a.sl.Order
	n := len(a.sl.Slices)
	if a.laneAt >= len(o) {
		a.laneAt = max(len(o)-1, 0)
	}
	switch k {
	case Left:
		a.laneAt = max(a.laneAt-1, 0)
	case Right:
		a.laneAt = min(a.laneAt+1, len(o)-1)
	case Up:
		o[a.laneAt] = (o[a.laneAt] + 1) % n
	case Down:
		o[a.laneAt] = (o[a.laneAt] + n - 1) % n
	case A: // repeat: a copy of the step, after it
		a.sl.Order = append(o[:a.laneAt+1], append([]int{o[a.laneAt]}, o[a.laneAt+1:]...)...)
		a.laneAt++
	case B: // drop
		if len(o) > 1 {
			a.sl.Order = append(o[:a.laneAt], o[a.laneAt+1:]...)
			a.laneAt = min(a.laneAt, len(a.sl.Order)-1)
		}
	case L1:
		if a.laneAt > 0 {
			o[a.laneAt], o[a.laneAt-1] = o[a.laneAt-1], o[a.laneAt]
			a.laneAt--
		}
	case R1:
		if a.laneAt < len(o)-1 {
			o[a.laneAt], o[a.laneAt+1] = o[a.laneAt+1], o[a.laneAt]
			a.laneAt++
		}
	case X:
		prev := a.at
		a.at = a.sl.Slices[o[a.laneAt]].Start
		a.playSlice()
		a.at = prev
	}
}

func (a *App) laneShifted(k Key) {
	switch k {
	case B:
		a.lane = false
		a.say("order: "+orderText(a.sl.Order), false)
	case A:
		a.shuffleOrder()
	case X:
		a.sl.ResetOrder()
		a.laneAt = 0
	}
}

// drawLane draws the order as cells (the slice numbers) in the place of the plan, with the step in front lit.
func (a *App) drawLane(g Gfx, top, rows int) {
	per := max((contentCols-4)/4, 1)
	for i, s := range a.sl.Order {
		if i >= rows*per {
			g.Text(2+(per-1)*4, top+rows-1, fmt.Sprintf("+%d", len(a.sl.Order)-rows*per+1), cDim, false)
			break
		}
		x, y := 2+(i%per)*4, top+i/per
		col, fg := cPanel2, cText
		if i == a.laneAt {
			col, fg = cAmber, cBg
		}
		g.Fill(x, y, 3, 1, col)
		t := fmt.Sprint(s + 1)
		g.Text(x+(3-len(t))/2, y, t, fg, true)
	}
}

// ---- several samples

// joinSample puts another sample behind the one in the slicer: its part becomes one more slice. Up to 8 samples.
func (a *App) joinSample(other int) {
	if len(a.joinStack) >= 7 {
		a.say("8 samples are the most", true)
		return
	}
	nid, err := a.lab.Join(a.sid, other)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	s, err := a.lab.Sample(nid)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.joinStack = append(a.joinStack, joinState{sid: a.sid, sl: a.sl.Clone()})
	a.sid = nid
	a.sl.Extend(s.Frames(), 1)
	a.say(fmt.Sprintf("%d samples, %d slices", len(a.joinStack)+1, len(a.sl.Slices)), false)
}

// takeLastSampleOut goes back to before the last sample was added.
func (a *App) takeLastSampleOut() {
	if len(a.joinStack) == 0 {
		a.say("only one sample", true)
		return
	}
	last := a.joinStack[len(a.joinStack)-1]
	a.joinStack = a.joinStack[:len(a.joinStack)-1]
	a.sid, a.sl = last.sid, last.sl
	a.say(fmt.Sprintf("%d samples, %d slices", len(a.joinStack)+1, len(a.sl.Slices)), false)
}

type joinState struct {
	sid int
	sl  *formats.Slicing
}

// addSampleFromPad asks for a pad with a sample and joins it.
func (a *App) addSampleFromPad() {
	a.pickPad("add the sample of which pad?", a.spad, true, func(pad int) {
		id, ok := a.padSample(pad)
		if !ok {
			a.say("that pad has no sample", true)
			return
		}
		a.joinSample(id)
	})
}

// addSampleFromFile goes to the file browser; A on a file there joins it and comes back.
func (a *App) addSampleFromFile() {
	a.joinPick = true
	a.go_(Files)
	a.say("choose the file to add: A joins it, B goes back", false)
}

// ---- a pattern under the slices

// patternUnder puts the slices on a run of pads and writes a pattern that plays them in the order of the slicer.
func (a *App) patternUnder(bars int) {
	n := len(a.sl.Slices)
	if n < 2 {
		a.say("cut it first (A) or Y+A for auto", true)
		return
	}
	start := a.freeRun(n, a.spad+1)
	if start < 0 {
		start = a.spad + 1
	}
	a.pickPad(fmt.Sprintf("First pad of the %d slices", n), start, false, func(first int) {
		for i := 0; i < n; i++ {
			if first+i > 159 || a.info.Pads[first+i].File {
				a.say(fmt.Sprintf("%d empty pads in a row are needed from %s", n, a.label(first)), true)
				return
			}
		}
		a.pickPattern("the pattern under the slices", a.pat, func(slot int) { a.writeUnder(first, slot, bars) })
	})
}

func (a *App) writeUnder(first, slot, bars int) {
	if _, id, err := a.renderResult(false); err != nil {
		a.say(err.Error(), true)
		return
	} else {
		a.lab.Free(id)
	}
	placed := append([]lab.Placed(nil), a.placed...)
	bpm := a.tempoOf(a.spad / 16)
	ticksPer := float64(formats.PPQ) * bpm / 60 / formats.SampleRate // ticks per frame
	pre := a.lab.Snapshot()
	padOf := a.slicePadMap()
	a.slicesToPadsFrom(first, false)
	if err := a.lab.NewPattern(slot, bars); err != nil {
		a.say(err.Error(), true)
		return
	}
	length := bars * 4 * formats.PPQ
	notes := 0
	for _, p := range placed {
		tick := int(float64(p.At)*ticksPer + 0.5)
		if p.Rest || tick >= length {
			continue
		}
		args, _ := json.Marshal(lab.PatternArgs{Pad: first + padOf[p.Slice], Tick: uint32(tick), Vel: 127, Len: max(int(float64(p.Frames)*ticksPer), 1)})
		if err := a.lab.PatternOp(slot, "insert", args); err == nil {
			notes++
		}
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	a.say(fmt.Sprintf("%d slices on %s and %d notes in pattern %s", n0(padOf), a.label(first), notes, a.label(slot-1)), false)
}

func n0(m map[int]int) int { return len(m) }

// slicePadMap is the position of each slice among the pads it is put on (the order it plays in).
func (a *App) slicePadMap() map[int]int {
	out := map[int]int{}
	for pos, i := range a.padOrder() {
		out[i] = pos
	}
	return out
}
