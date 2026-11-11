// Package generate writes drum patterns on its own: a style (a groove written as text lanes, one character a 16th step), an energy
// that decides which layers play, variations (fills, ghost notes, hat patterns, syncopation, half-time, swing, humanising, euclidean
// rhythms) and arrangements (a set of patterns that build tension, drop and break). Everything is driven by a seed: the same seed
// gives the same result, a new one gives another.
package generate

import (
	"fmt"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Row is one lane of a pattern: the velocity of each 16th step, 16 to a bar.
type Row struct {
	Lane         formats.Lane
	Steps        []uint8
	Swing, Nudge int
	Gate         uint16
	// Ratchet and Flam are per step like Steps (nil: none): the hits a step plays (a roll) and the ticks of a grace note before it
	Ratchet, Flam []uint8
}

// chokeGap is how many ticks before the closed hat an open hat ends.
const chokeGap = 8

// Pattern is a drum pattern of Bars bars.
type Pattern struct {
	Bars int
	Rows []*Row
}

// Row is the row of the lane, nil when the pattern has none.
func (p *Pattern) Row(l formats.Lane) *Row {
	for _, r := range p.Rows {
		if r.Lane == l {
			return r
		}
	}
	return nil
}

// ensure is the row of the lane, made empty when there is none.
func (p *Pattern) ensure(l formats.Lane) *Row {
	if r := p.Row(l); r != nil {
		return r
	}
	r := &Row{Lane: l, Steps: make([]uint8, p.Bars*16)}
	p.Rows = append(p.Rows, r)
	return r
}

// Clone is a copy that can be changed on its own.
func (p Pattern) Clone() Pattern {
	q := Pattern{Bars: p.Bars}
	for _, r := range p.Rows {
		c := *r
		c.Steps = append([]uint8(nil), r.Steps...)
		c.Ratchet = append([]uint8(nil), r.Ratchet...)
		c.Flam = append([]uint8(nil), r.Flam...)
		q.Rows = append(q.Rows, &c)
	}
	return q
}

// Lanes are the rows as the builder takes them.
func (p Pattern) Lanes() []formats.StepLane {
	var out []formats.StepLane
	for _, r := range p.Rows {
		sl := formats.StepLane{Lane: r.Lane, Steps: r.Steps, Swing: r.Swing, Nudge: r.Nudge, Gate: r.Gate, Ratchet: r.Ratchet, Flam: r.Flam}
		if r.Lane == formats.LaneOpenHat {
			sl.Gates = p.choke(r)
		}
		out = append(out, sl)
	}
	return out
}

// Bytes writes the pattern as a PTNnnnnn.BIN for the pads.
func (p Pattern) Bytes(pads map[formats.Lane]proto.PadIndex) ([]byte, error) {
	return p.BytesAlt(pads, nil)
}

// altBase is where the lanes of the second pads start: lane + altBase plays on alts[lane].
const altBase formats.Lane = 100

// BytesAlt is Bytes with a second pad for some lanes. The kick plays it on the hits off the beat, the snare on its ghost notes, the others
// on every second hit, so a kit of 16 pads is heard in full and the hits are not all the same sample.
func (p Pattern) BytesAlt(pads, alts map[formats.Lane]proto.PadIndex) ([]byte, error) {
	m := map[formats.Lane]proto.PadIndex{}
	for l, pad := range pads {
		m[l] = pad
	}
	var lanes []formats.StepLane
	for _, sl := range p.Lanes() {
		alt, ok := alts[sl.Lane]
		if !ok || len(sl.Steps) == 0 {
			lanes = append(lanes, sl)
			continue
		}
		n := p.Bars * 16
		main, second := sl, sl
		main.Steps, second.Steps = make([]uint8, n), make([]uint8, n)
		second.Lane = sl.Lane + altBase
		m[second.Lane] = alt
		hit := 0
		for i := 0; i < n; i++ {
			v := sl.Steps[i%len(sl.Steps)]
			if v == 0 {
				continue
			}
			hit++
			var useAlt bool
			switch sl.Lane {
			case formats.LaneKick:
				useAlt = i%4 != 0
			case formats.LaneSnare:
				useAlt = v < 80
			default:
				useAlt = hit%2 == 0
			}
			if useAlt {
				second.Steps[i] = v
			} else {
				main.Steps[i] = v
			}
		}
		// per-step lists follow the steps: they are as long as the pattern now
		for _, sp := range []*formats.StepLane{&main, &second} {
			sp.Gates = tile(sl.Gates, n)
			sp.Ratchet = tile(sl.Ratchet, n)
			sp.Flam = tile(sl.Flam, n)
		}
		lanes = append(lanes, main, second)
	}
	return formats.BuildSteps(lanes, m, p.Bars)
}

// tile repeats a per-step list to n steps (nil stays nil).
func tile[T any](s []T, n int) []T {
	if len(s) == 0 {
		return nil
	}
	out := make([]T, n)
	for i := range out {
		out[i] = s[i%len(s)]
	}
	return out
}

// Notes is how many hits the pattern has.
func (p Pattern) Notes() int {
	n := 0
	for _, r := range p.Rows {
		for i, v := range r.Steps {
			if v > 0 {
				n += max(1, r.ratchet(i))
				if i < len(r.Flam) && r.Flam[i] > 0 {
					n++
				}
			}
		}
	}
	return n
}

// ratchet is the hits the step plays.
func (r *Row) ratchet(i int) int {
	if i < len(r.Ratchet) {
		return int(r.Ratchet[i])
	}
	return 1
}

// setRatchet and setFlam make the per-step lists when they are first needed.
func (r *Row) setRatchet(i, n int) {
	if r.Ratchet == nil {
		r.Ratchet = make([]uint8, len(r.Steps))
	}
	r.Ratchet[i] = uint8(n)
}

func (r *Row) setFlam(i, ticks int) {
	if r.Flam == nil {
		r.Flam = make([]uint8, len(r.Steps))
	}
	r.Flam[i] = uint8(ticks)
}

// Text draws the pattern as rows of steps (a beat every four steps, a bar between | marks), for reading what was made.
func (p Pattern) Text() string {
	var b strings.Builder
	for _, r := range p.Rows {
		hit := false
		for _, v := range r.Steps {
			hit = hit || v > 0
		}
		if !hit {
			continue
		}
		fmt.Fprintf(&b, "%-11s", r.Lane.LaneName())
		for i, v := range r.Steps {
			if i > 0 && i%16 == 0 {
				b.WriteString("|")
			} else if i > 0 && i%4 == 0 {
				b.WriteString(" ")
			}
			switch {
			case v > 0 && r.ratchet(i) > 1:
				b.WriteByte(byte('0' + r.ratchet(i))) // a roll: how many hits the step plays
			case v > 0 && i < len(r.Flam) && r.Flam[i] > 0:
				b.WriteByte('f') // a flam
			default:
				b.WriteByte(mark(v))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func mark(v uint8) byte {
	switch {
	case v == 0:
		return '.'
	case v >= 115:
		return 'X'
	case v >= 90:
		return 'x'
	case v >= 65:
		return 'o'
	}
	return 'g'
}

// choke is the length of each open hat note: it ends just before the next closed hat starts (a closed hat cuts an open one on a 909), so
// the two do not smear into each other. A step with no open hat has 0 (the row's gate).
func (p *Pattern) choke(open *Row) []uint16 {
	closed := p.Row(formats.LaneClosedHat)
	if closed == nil {
		return nil
	}
	const step = formats.PPQ / 4
	n := len(open.Steps)
	gates := make([]uint16, n)
	gate := int(open.Gate)
	if gate == 0 {
		gate = step
	}
	for i, v := range open.Steps {
		if v == 0 {
			continue
		}
		g := gate
		for k := 1; k <= 8 && i+k < n; k++ {
			if closed.Steps[i+k] > 0 {
				// where the closed hat starts, counting its swing and nudge against the open hat's own
				at := k*step + closed.Nudge - open.Nudge
				if (i+k)%2 == 1 {
					at += formats.SwingTicks(closed.Swing)
				}
				if i%2 == 1 {
					at -= formats.SwingTicks(open.Swing)
				}
				g = min(g, max(at-chokeGap, 20))
				break
			}
		}
		gates[i] = uint16(g)
	}
	return gates
}
