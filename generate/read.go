package generate

import (
	"math"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
)

// Reading a pattern that is there (one played or written by hand, or a generated one) as rows of steps, so that the changes of this package
// work on it, and finding what a change did, so that only the steps it touched are written back: the other notes keep their own timing.

// Hit is a note of a pattern as a drum voice plays it.
type Hit struct {
	Lane     formats.Lane
	Tick     uint32
	Vel, Len int
}

const (
	stepTicks = formats.PPQ / 4
	// early is how far before its step a note may start and still belong to it: the grace note of a flam, a hit played ahead of the beat.
	// A note up to 84 ticks late belongs to the step too: a swung 16th, the hits of a roll in 32nds or triplets.
	early = 36
)

// StepOf is the 16th step a note at tick belongs to, in a pattern of steps steps.
func StepOf(tick uint32, steps int) int {
	return max(min((int(tick)+early)/stepTicks, steps-1), 0)
}

func median(v []int) int {
	if len(v) == 0 {
		return 0
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[len(s)/2]
}

// swingOf is the swing in percent that pushes the off 16ths by push ticks (0: straight).
func swingOf(push int) int {
	if push < 2 {
		return 0
	}
	return min(50+int(math.Round(float64(push)*100/float64(2*stepTicks))), 75)
}

// Read makes a pattern of bars bars from notes: the velocity of a step is that of its loudest note. The swing, the nudge and the note length of
// each lane are taken from where its notes are and how long they are, so that a hit added to a lane sits and sounds like the ones it has.
func Read(bars int, hits []Hit) Pattern {
	p := Pattern{Bars: bars}
	type place struct{ even, odd, lens []int }
	at := map[formats.Lane]*place{}
	var even, odd []int // of every lane but the kick: the swing of a lane that has no off 16ths of its own
	for _, h := range hits {
		r := p.ensure(h.Lane)
		s := StepOf(h.Tick, len(r.Steps))
		if v := uint8(max(min(h.Vel, 127), 1)); v > r.Steps[s] {
			r.Steps[s] = v
		}
		pl := at[h.Lane]
		if pl == nil {
			pl = &place{}
			at[h.Lane] = pl
		}
		off := int(h.Tick) - s*stepTicks
		if s%2 == 0 {
			pl.even = append(pl.even, off)
		} else {
			pl.odd = append(pl.odd, off)
		}
		if h.Lane != formats.LaneKick {
			if s%2 == 0 {
				even = append(even, off)
			} else {
				odd = append(odd, off)
			}
		}
		if h.Len > 0 {
			pl.lens = append(pl.lens, h.Len)
		}
	}
	all := 0
	if len(odd) > 0 {
		all = swingOf(median(odd) - median(even))
	}
	for _, r := range p.Rows {
		pl := at[r.Lane]
		if len(pl.even) > 0 {
			r.Nudge = max(min(median(pl.even), 30), -30)
		}
		switch {
		case len(pl.odd) > 0:
			r.Swing = swingOf(median(pl.odd) - r.Nudge)
		case r.Lane != formats.LaneKick:
			r.Swing = all
		}
		if n := median(pl.lens); n > 0 {
			r.Gate = uint16(min(n, 65535))
		}
	}
	return p
}

// same says whether a step plays the same in both rows: the velocity, the hits of a roll and a flam.
func same(a, b *Row, i int) bool {
	step := func(r *Row) (v uint8, ratchet int, flam uint8) {
		if r == nil || i >= len(r.Steps) || r.Steps[i] == 0 {
			return 0, 1, 0
		}
		if i < len(r.Flam) {
			flam = r.Flam[i]
		}
		return r.Steps[i], max(r.ratchet(i), 1), flam
	}
	av, ar, af := step(a)
	bv, br, bf := step(b)
	return av == bv && ar == br && af == bf
}

// Diff is what a change did to a pattern: for each lane the steps that play differently after it (nil: none), and the lanes of after with
// those steps only, for the builder: the notes that are new. The notes of the steps that are the same are left where they are.
func Diff(before, after Pattern) (map[formats.Lane][]bool, []formats.StepLane) {
	n := after.Bars * 16
	changed := map[formats.Lane][]bool{}
	mark := func(l formats.Lane, i int) {
		if changed[l] == nil {
			changed[l] = make([]bool, n)
		}
		changed[l][i] = true
	}
	for _, r := range after.Rows {
		b := before.Row(r.Lane)
		for i := 0; i < n && i < len(r.Steps); i++ {
			if !same(b, r, i) {
				mark(r.Lane, i)
			}
		}
	}
	for _, b := range before.Rows { // a lane the change took out
		if after.Row(b.Lane) != nil {
			continue
		}
		for i := 0; i < n && i < len(b.Steps); i++ {
			if b.Steps[i] > 0 {
				mark(b.Lane, i)
			}
		}
	}
	var add []formats.StepLane
	for _, sl := range after.Lanes() { // the lanes of the whole pattern (an open hat ends where the next closed hat is), then only the new steps
		ch := changed[sl.Lane]
		if ch == nil {
			continue
		}
		steps := make([]uint8, n)
		hit := false
		for i := range steps {
			if ch[i] && i < len(sl.Steps) && sl.Steps[i] > 0 {
				steps[i] = sl.Steps[i]
				hit = true
			}
		}
		if hit {
			sl.Steps = steps
			add = append(add, sl)
		}
	}
	return changed, add
}
