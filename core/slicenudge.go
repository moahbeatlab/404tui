package core

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// Nudging a slice by ear (a computer's keys): ( ) move the start of the slice under the cursor 5 ms earlier or later, < > its end.
// A slice of whole bars moves as a whole with ( ): start and end together, so a loop stays a loop. A marker the slice shares with
// its neighbour moves for both. While the slice plays in a loop (x), the loop takes every step at once, so the seam is heard as it
// moves.

const nudgeFrames = formats.SampleRate * 5 / 1000 // 5 ms

// SliceNudgeKey is ( ) (end false) and < > (end true) of a keyboard in the slicer: d -1 earlier, +1 later.
func (a *App) SliceNudgeKey(end bool, d int) {
	if a.scr != Slice || a.sl == nil || a.Modal() || a.prompt != nil || a.lane || a.tap {
		return
	}
	a.CancelShift()
	a.slicerStep(func() { a.nudge(end, d) })
}

func (a *App) nudge(end bool, d int) {
	i := a.sliceAt(a.at)
	if i < 0 {
		a.say("the cursor is in no slice", true)
		return
	}
	sg := a.sl.Slices[i]
	whole := !end && a.wholeBars(sg.End-sg.Start)
	before := a.sl.Clone()
	marks := map[int]bool{}
	for k, v := range a.slMarks {
		marks[k] = v
	}
	step := d * nudgeFrames
	ok := true
	switch {
	case end:
		ok = a.setEdge(i, true, sg.End+step)
	case whole && d < 0: // the start first, so the slice never gets shorter than it may
		ok = a.setEdge(i, false, sg.Start+step) && a.setEdge(i, true, sg.End+step)
	case whole:
		ok = a.setEdge(i, true, sg.End+step) && a.setEdge(i, false, sg.Start+step)
	default:
		ok = a.setEdge(i, false, sg.Start+step)
	}
	if !ok {
		a.sl, a.slMarks = before, marks
		a.say("it cannot move further: the next marker or the edge of the sample is there", true)
		return
	}
	ns := a.sl.Slices[i]
	if a.at == sg.Start || a.sliceAt(a.at) != i { // the cursor stays with the slice
		a.at = ns.Start
	}
	if !a.slPlay.at.IsZero() && a.slPlay.loop && time.Now().Before(a.soundUntil) {
		a.loopSlice() // the loop takes it at once
	}
	what, at := "start", ns.Start
	if end {
		what, at = "end", ns.End
	}
	msg := fmt.Sprintf("slice %d: %s %s", i+1, what, a.offText(at))
	if whole {
		msg += ", the end with it"
	}
	if b := a.barsText(ns.End - ns.Start); b != "" {
		msg += " (" + b + ")"
	}
	a.say(msg, false)
}

// offText is where a frame is: how far from the nearest bar line ("+10 ms from bar 17.1"), or the time without bars.
func (a *App) offText(f int) string {
	if a.barFrames() <= 0 {
		return fmt.Sprintf("at %.3f s", float64(f)/formats.SampleRate)
	}
	nb := a.nearestBar(f)
	ms := float64(f-nb) * 1000 / formats.SampleRate
	if ms > -0.5 && ms < 0.5 {
		return "on " + a.barText(nb)
	}
	return fmt.Sprintf("%+.0f ms from %s", ms, a.barText(nb))
}

// setEdge puts the start (or the end) of slice i at a frame. A neighbour that touches it there moves with it; one that does not is
// as far as it goes. False when it cannot go there (nothing is changed then).
func (a *App) setEdge(i int, end bool, f int) bool {
	sl := a.sl
	sg := &sl.Slices[i]
	if end {
		if f > sl.Frames || f < sg.Start+formats.MinSliceFrames {
			return false
		}
		for j := range sl.Slices {
			n := &sl.Slices[j]
			if j == i || n.Start < sg.End {
				continue
			}
			if n.Start == sg.End { // the marker they share
				if f > n.End-formats.MinSliceFrames {
					return false
				}
				a.moveMark(n.Start, f)
				n.Start = f
			} else if f > n.Start {
				return false
			}
		}
		sg.End = f
		return true
	}
	if f < 0 || f > sg.End-formats.MinSliceFrames {
		return false
	}
	for j := range sl.Slices {
		p := &sl.Slices[j]
		if j == i || p.End > sg.Start {
			continue
		}
		if p.End == sg.Start {
			if f < p.Start+formats.MinSliceFrames {
				return false
			}
			p.End = f
		} else if f < p.End {
			return false
		}
	}
	a.moveMark(sg.Start, f)
	sg.Start = f
	return true
}

// moveMark keeps a slice marked when its start moves.
func (a *App) moveMark(from, to int) {
	if a.slMarks[from] {
		delete(a.slMarks, from)
		a.slMarks[to] = true
	}
}
