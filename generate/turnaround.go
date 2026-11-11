package generate

import (
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// A pattern of four bars or more ends with a turnaround: the last bar is the groove with a small change at its
// end (a pickup on the backbeat, an open hat lift, the hats dropping out, ghost notes or a percussion answer), so the loop is heard to come
// round. A last bar that has a fill, a roll, the build or a stop already is left as it is.

// TurnaroundKinds are the small changes of a last bar.
var TurnaroundKinds = []string{"pickup", "open hat lift", "hat drop", "ghost answer", "percussion answer"}

// shape is which steps of a bar play, for comparing bars (velocities and timing are left out: humanising changes those).
func (p *Pattern) shape(bar int) string {
	b := make([]byte, 0, len(p.Rows)*17)
	for _, r := range p.Rows {
		quiet := true
		for i := bar * 16; i < bar*16+16 && i < len(r.Steps); i++ {
			quiet = quiet && r.Steps[i] == 0
		}
		if quiet { // a lane that is silent in the bar is not part of it (a lane added for another bar changes nothing here)
			continue
		}
		b = append(b, byte('A'+r.Lane))
		for i := bar * 16; i < bar*16+16 && i < len(r.Steps); i++ {
			switch {
			case r.Steps[i] == 0:
				b = append(b, '.')
			case r.ratchet(i) > 1:
				b = append(b, byte('0'+r.ratchet(i)))
			default:
				b = append(b, 'x')
			}
		}
	}
	return string(b)
}

// ownLastBar says whether a result's last bar has something of its own already: a fill, a roll, the build or a stop.
func ownLastBar(r Result) bool {
	switch r.Section.Kind {
	case "fill", "build", "stop":
		return true
	}
	return strings.Contains(r.Notes, "fill: ") || strings.Contains(r.Notes, "last bar: ")
}

// Turnaround changes the end of the last bar a little; it says what it did ("" when nothing of the kit could).
func (p *Pattern) Turnaround(c *Ctx) string {
	bar := p.Bars - 1
	before := p.shape(bar)
	at := func(l formats.Lane, step int, v uint8) {
		if c.has(l) && c.open(l) {
			p.ensure(l).Steps[bar*16+step] = v
		}
	}
	clr := func(from, to int, lanes ...formats.Lane) {
		for _, l := range lanes {
			if r := p.Row(l); r != nil && c.open(l) {
				for i := from; i < to; i++ {
					r.Steps[bar*16+i] = 0
				}
			}
		}
	}
	order := c.Rng.Perm(len(TurnaroundKinds))
	for _, i := range order {
		kind := TurnaroundKinds[i]
		switch kind {
		case "pickup":
			p.Pickup(c, bar)
		case "open hat lift":
			clr(14, 16, formats.LaneClosedHat)
			at(formats.LaneOpenHat, 14, 100)
		case "hat drop":
			clr(12, 16, formats.LaneClosedHat, formats.LaneShaker)
		case "ghost answer":
			l := p.backbeat(c)
			at(l, 13, 48)
			at(l, 15, 56)
		case "percussion answer":
			l := formats.LanePerc
			if !c.has(l) {
				l = formats.LaneRim
			}
			clr(8, 16, l)
			for _, s := range [][]int{{11, 14}, {10, 13, 15}, {12, 14, 15}}[c.Rng.Intn(3)] {
				at(l, s, uint8(70+c.Rng.Intn(30)))
			}
		}
		if p.shape(bar) != before {
			return kind
		}
	}
	return ""
}

// turnarounds gives each result of four bars or more whose last bar has nothing of its own a turnaround.
func turnarounds(rs []Result, seed int64, has func(formats.Lane) bool) {
	for i := range rs {
		p := &rs[i].Pattern
		if p.Bars < 4 || ownLastBar(rs[i]) {
			continue
		}
		c := NewCtx(seed+int64(i)*31337, has)
		if kind := p.Turnaround(c); kind != "" {
			rs[i].Notes = strings.TrimRight(rs[i].Notes, "; ") + "; last bar: " + kind
		}
	}
}
