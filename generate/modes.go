package generate

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// Modes say how a bank is filled.
//
//	family  one theme and what is done to it: the groove, then a variation of it in each slot, a build, a stop, a drop, a break, an outro
//	random  sixteen patterns that have nothing to do with each other (each has the dice of its own)
//	layers  the groove, then one layer added to the one before, up to the full style
//	drift   each pattern is a small change of the one before, so the bank slowly wanders away from the first
//	arc     eight patterns along a tension curve: intro, groove, variation, build, stop, drop, break, outro (each with its own dice)
var Modes = []string{"family", "random", "layers", "drift", "arc"}

// ModeInfo is a line about a mode.
func ModeInfo(mode string) string {
	switch mode {
	case "family":
		return "variations of one pattern (16): the groove, then each slot changes one thing, then build, stop, drop, break, outro"
	case "random":
		return "16 different patterns, each rolled on its own"
	case "layers":
		return "the groove, then one layer more in each slot, up to the full style"
	case "drift":
		return "each pattern is a small change of the one before"
	case "arc":
		return "8 patterns along a tension curve (intro to outro)"
	}
	return ""
}

// Generate fills a bank in a mode: bars is the length of each pattern, seed makes it (the same seed gives the same patterns).
func Generate(mode string, st Style, bars int, seed int64, has func(formats.Lane) bool) ([]Result, error) {
	switch mode {
	case "", "family":
		return Family(st, bars, seed, has), nil
	case "random":
		return Random(st, bars, seed, has), nil
	case "layers":
		return Layers(st, bars, seed, has), nil
	case "drift":
		return Drift(st, bars, seed, has), nil
	case "arc":
		return Arrange(st, DefaultArc(bars), seed, has), nil
	}
	return nil, fmt.Errorf("mode %q: one of family, random, layers, drift, arc", mode)
}

// Random is sixteen patterns each of its own energy and kind, rolled one by one.
func Random(st Style, bars int, seed int64, has func(formats.Lane) bool) []Result {
	c := NewCtx(seed, has)
	kinds := []struct {
		kind string
		lo   float64
		hi   float64
	}{{"groove", 0.4, 0.6}, {"variation", 0.55, 0.8}, {"drop", 0.9, 1}, {"groove", 0.4, 0.6}, {"variation", 0.55, 0.8}, {"build", 0.7, 0.85}, {"break", 0.25, 0.35}, {"intro", 0.1, 0.25}}
	var out []Result
	for i := 0; i < FamilySize; i++ {
		k := kinds[c.Rng.Intn(len(kinds))]
		s := Section{Name: fmt.Sprintf("Random %d", i+1), Kind: k.kind, Energy: k.lo + c.Rng.Float64()*(k.hi-k.lo), Bars: bars}
		out = append(out, Make(st, s, seed+int64(i+1)*104729, has))
		out[i].Section.Name = fmt.Sprintf("Random %s", k.kind)
	}
	return out
}

// Layers adds one thing at a time: slot n has what slot n-1 had and one more (ghost notes, the percussion, rim, shaker and open hat,
// the sixteenth hats, a polymeter, an echo, chance on the hats, a fill), and then the build, the stop and the drop of the full style.
func Layers(st Style, bars int, seed int64, has func(formats.Lane) bool) []Result {
	th := newTheme(st, bars, seed, has)
	cur := th.groove.Clone()
	var out []Result
	for i, fs := range famSteps {
		var p Pattern
		var notes string
		c := NewCtx(seed+int64(i)*7919, has)
		switch {
		case i == 0:
			p, notes = cur.Clone(), "the theme: hats "+th.hatKind
		case fs.kind == "variation":
			before := cur.Clone()
			notes = fs.do(th, &cur, c)
			notes += th.visible(&cur, before, fs.kind, c)
			p = cur.Clone()
		default: // build, stop, drop: from what has been gathered; break and outro go back to the groove
			base := cur.Clone()
			if fs.kind == "break" || fs.kind == "outro" {
				base = th.groove.Clone()
			}
			notes = fs.do(th, &base, c)
			p = base
		}
		p.Swing(c, th.swing)
		p.Humanize(c, 0.2)
		out = append(out, Result{Section: Section{Name: fs.name, Kind: fs.kind, Energy: fs.energy, Bars: bars}, Pattern: p, Notes: notes})
	}
	return out
}

// Drift makes each pattern a small change of the one before: one to three of the changes of a family, each at random.
func Drift(st Style, bars int, seed int64, has func(formats.Lane) bool) []Result {
	th := newTheme(st, bars, seed, has)
	c := NewCtx(seed^0x5bd1e995, has)
	cur := th.groove.Clone()
	var out []Result
	out = append(out, Result{Section: Section{Name: "Groove", Kind: "groove", Energy: 0.5, Bars: bars}, Pattern: cur.Clone(), Notes: "the theme: hats " + th.hatKind})
	for i := 1; i < FamilySize; i++ {
		var notes string
		prev := cur.Clone()
		for k, n := 0, 1+c.Rng.Intn(3); k < n; k++ {
			// the changes that are small: ghost notes, the percussion, another hat, a polymeter, an echo, chance, a fill, fewer layers
			fs := famSteps[1+c.Rng.Intn(9)]
			if c.chance(0.4) {
				th.thinOne(&cur, c)
				notes += "one layer less; "
				continue
			}
			notes += fs.do(th, &cur, c) + "; "
		}
		// a drift must not only grow: too many notes and a layer goes, too few and the percussion comes back
		for n := cur.Notes(); n > out[0].Pattern.Notes()*3/2; n = cur.Notes() {
			th.thinOne(&cur, c)
			notes += "one layer less; "
		}
		if cur.Notes() < out[0].Pattern.Notes()*3/4 {
			th.withPerc(&cur)
			notes += "the percussion back; "
		}
		if cur.Text() == prev.Text() {
			notes += th.visible(&cur, prev, "variation", c)
		}
		p := cur.Clone()
		p.Humanize(c, 0.2)
		out = append(out, Result{Section: Section{Name: fmt.Sprintf("Drift %d", i), Kind: "variation", Energy: 0.55, Bars: bars}, Pattern: p, Notes: notes})
	}
	return out
}

// thinOne takes the hits off one of the optional lanes, so that a drift does not only add.
func (th *theme) thinOne(p *Pattern, c *Ctx) {
	var rows []*Row
	for _, r := range p.Rows {
		if !core[r.Lane] && r.Lane != formats.LaneKick {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return
	}
	r := rows[c.Rng.Intn(len(rows))]
	for i := range r.Steps {
		r.Steps[i] = 0
	}
}
