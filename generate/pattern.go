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
		q.Rows = append(q.Rows, &c)
	}
	return q
}

// Lanes are the rows as the builder takes them.
func (p Pattern) Lanes() []formats.StepLane {
	var out []formats.StepLane
	for _, r := range p.Rows {
		sl := formats.StepLane{Lane: r.Lane, Steps: r.Steps, Swing: r.Swing, Nudge: r.Nudge, Gate: r.Gate}
		if r.Lane == formats.LaneOpenHat {
			sl.Gates = p.choke(r)
		}
		out = append(out, sl)
	}
	return out
}

// Bytes writes the pattern as a PTNnnnnn.BIN for the pads.
func (p Pattern) Bytes(pads map[formats.Lane]proto.PadIndex) ([]byte, error) {
	m := map[formats.Lane]proto.PadIndex{}
	for l, pad := range pads {
		m[l] = pad
	}
	return formats.BuildSteps(p.Lanes(), m, p.Bars)
}

// Notes is how many hits the pattern has.
func (p Pattern) Notes() int {
	n := 0
	for _, r := range p.Rows {
		for _, v := range r.Steps {
			if v > 0 {
				n++
			}
		}
	}
	return n
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
			b.WriteByte(mark(v))
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
