package generate

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// Variations of a pattern that is there already (one that was made by hand, or a generated one that is liked): the pattern stays as it is
// and a copy is changed. No style is needed: what is done depends on the lanes the pattern and the kit have.

// Variations are the kinds Vary knows.
var Variations = []string{"slight", "fill", "roll", "thinner", "busier", "break"}

// FillKinds are the fills that are not rolls.
var FillKinds = []string{"snare roll", "tom cascade", "clap stack", "open hat swell", "ghost shuffle", "rim run", "kick stutter", "hat stop"}

// VaryLabel is what a menu says for a variation.
func VaryLabel(kind string) string {
	switch kind {
	case "fill":
		return "with a fill"
	case "roll":
		return "with a roll"
	case "slight":
		return "slightly different"
	}
	return kind
}

// VaryInfo is a line about a variation.
func VaryInfo(kind string) string {
	switch kind {
	case "fill":
		return "the same pattern with a drum fill in the second half of its last bar."
	case "roll":
		return "the same pattern with a roll at the end of its last bar (a snare roll that speeds up, 32nds, flams, a tom, kick, hat or clap roll)."
	case "slight":
		return "the same groove with two to four small changes: a hat or a percussion hit moved, one less or one more, a ghost snare, an open hat, a kick pickup. Roll again for others."
	case "thinner":
		return "the quiet hits go, and one of the layers around the kick and the backbeat."
	case "busier":
		return "ghost notes, and one layer more: a percussion line, hats in 16ths or an echo."
	case "break":
		return "no kick, the claps in half time, the hats in 8ths, no shaker."
	}
	return ""
}

// Break makes a break of the pattern: the kick goes, the backbeat plays in half time, the hats in 8ths, the shaker goes, and now and then
// the open hat.
func (p *Pattern) Break(c *Ctx) string {
	zero := func(l formats.Lane) {
		if r := p.Row(l); r != nil {
			for i := range r.Steps {
				r.Steps[i] = 0
			}
		}
	}
	zero(formats.LaneKick)
	p.HalfTime(c)
	zero(formats.LaneShaker) // a break has fewer layers: no shaker, the hats in 8ths
	if r := p.Row(formats.LaneClosedHat); r != nil {
		for i := range r.Steps {
			if i%2 == 1 {
				r.Steps[i] = 0
			}
		}
	}
	if c.chance(0.5) { // the hats and the claps stay, the open hat goes now and then
		zero(formats.LaneOpenHat)
	}
	return "no kick, the claps in half time, the hats in 8ths, no shaker"
}

// hits is how many steps of the row play.
func (r *Row) hits() int {
	n := 0
	for _, v := range r.Steps {
		if v > 0 {
			n++
		}
	}
	return n
}

// Vary is a changed copy of a pattern and what was done to it. The dice choose which fill, which roll, which layer; has says which
// voices the kit has. A variation that would leave the pattern as it is (a hat stop where there are no hats) is not chosen; when none of a
// kind changes anything, that is an error.
func Vary(src Pattern, kind string, seed int64, has func(formats.Lane) bool) (Pattern, string, error) {
	c := NewCtx(seed, has)
	before := src.Text()
	differs := func(p Pattern) bool { return p.Text() != before }
	switch kind {
	case "fill", "roll":
		pool := FillKinds
		if kind == "roll" {
			pool = RollKinds
		}
		var kinds []string
		for _, k := range pool {
			if fillNeeds(k, c.has) {
				kinds = append(kinds, k)
			}
		}
		c.Rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
		for _, k := range kinds {
			p := src.Clone()
			p.Fill(c, Style{}, k, p.Bars-1)
			if differs(p) {
				return p, k, nil
			}
		}
		return src, "", fmt.Errorf("the kit has no voice for a %s (a snare, a clap, toms, hats)", kind)
	case "slight":
		p := src.Clone()
		what := p.Slight(c)
		if !differs(p) {
			return src, "", fmt.Errorf("the pattern has nothing to change a little")
		}
		return p, join(what), nil
	case "thinner":
		p := src.Clone()
		var what []string
		ghosts := 0
		for _, r := range p.Rows {
			if r.Lane == formats.LaneKick {
				continue
			}
			for i, v := range r.Steps {
				if v > 0 && v < 64 {
					r.Steps[i] = 0
					ghosts++
				}
			}
		}
		if ghosts > 0 {
			what = append(what, "no ghost notes")
		}
		var layers []*Row
		for _, r := range p.Rows {
			if !core[r.Lane] && r.hits() > 0 {
				layers = append(layers, r)
			}
		}
		switch {
		case len(layers) > 1: // one of the layers goes, never the last of them
			r := layers[c.Rng.Intn(len(layers))]
			for i := range r.Steps {
				r.Steps[i] = 0
			}
			what = append(what, "no "+lower(r.Lane.LaneName()))
		case len(layers) == 1 && layers[0].hits() > p.Bars*8: // the one layer plays half as often
			for i := range layers[0].Steps {
				if i%2 == 1 {
					layers[0].Steps[i] = 0
				}
			}
			what = append(what, "the "+lower(layers[0].Lane.LaneName())+" in 8ths")
		}
		if !differs(p) {
			return src, "", fmt.Errorf("there is nothing to take away: the pattern is the kick and the backbeat")
		}
		return p, join(what), nil
	case "busier":
		p := src.Clone()
		var what []string
		p.Ghosts(c, 0.8)
		if differs(p) {
			what = append(what, "ghost notes")
		}
		perc := formats.LanePerc
		if !c.has(perc) {
			perc = formats.LaneRim
		}
		var more []string
		if r := p.Row(perc); c.has(perc) && (r == nil || r.hits() == 0) {
			more = append(more, "perc")
		}
		if r := p.Row(formats.LaneClosedHat); c.has(formats.LaneClosedHat) && (r == nil || r.hits() < p.Bars*12) {
			more = append(more, "hats")
		}
		for _, l := range []formats.Lane{formats.LaneClap, formats.LaneRim, formats.LanePerc} {
			if r := p.Row(l); r != nil && r.hits() > 0 && c.has(l) {
				more = append(more, "echo")
				break
			}
		}
		if len(more) > 0 {
			switch more[c.Rng.Intn(len(more))] {
			case "perc":
				p.Percussion(c, 4, 7)
				what = append(what, "a percussion line")
			case "hats": // the gaps of the hats are filled: the hits that are there stay as they are
				r := p.ensure(formats.LaneClosedHat)
				if r.Gate == 0 {
					r.Gate = 60
				}
				for i, v := range r.Steps {
					if v == 0 {
						r.Steps[i] = []uint8{82, 50, 104, 50}[i%4]
					}
				}
				what = append(what, "hats in 16ths")
			case "echo":
				var lanes []formats.Lane
				for _, l := range []formats.Lane{formats.LaneClap, formats.LaneRim, formats.LanePerc} {
					if r := p.Row(l); r != nil && r.hits() > 0 && c.has(l) {
						lanes = append(lanes, l)
					}
				}
				l := lanes[c.Rng.Intn(len(lanes))]
				p.Echo(c, l, 3, 3, 0.58)
				what = append(what, "an echo on the "+lower(l.LaneName()))
			}
		}
		if !differs(p) {
			p.Density(c, 0.5)
			what = append(what, "a few more hits")
		}
		if !differs(p) {
			return src, "", fmt.Errorf("there is no room for more: every step plays")
		}
		return p, join(what), nil
	case "break":
		p := src.Clone()
		what := p.Break(c)
		if !differs(p) {
			return src, "", fmt.Errorf("the pattern is a break already: no kick, no 16ths")
		}
		if p.Notes() == 0 {
			return src, "", fmt.Errorf("a break of this pattern would be empty: it has only a kick")
		}
		return p, what, nil
	}
	return src, "", fmt.Errorf("variation %q: one of fill, roll, thinner, busier, break", kind)
}

func lower(s string) string {
	b := []byte(s)
	if len(b) > 0 && b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}

func join(what []string) string {
	out := ""
	for i, w := range what {
		if i > 0 {
			out += ", "
		}
		out += w
	}
	return out
}

// Slight makes two to four small changes to a pattern, so that it is the same groove a little different: a hit of the hats or the
// percussion moved a step, one of them less or one more on an offstep, a ghost on the snare or the clap, an open hat on an offbeat in
// place of the closed one, a kick before the end of a bar. The kick on the beats and the backbeat stay. It says what it changed.
func (p *Pattern) Slight(c *Ctx) []string {
	steps := p.Bars * 16
	var layers []*Row // the rows around the kick and the backbeat that play
	for _, r := range p.Rows {
		if !core[r.Lane] && r.Lane != formats.LaneOpenHat && r.hits() > 0 && len(r.Steps) == steps {
			layers = append(layers, r)
		}
	}
	plain := func(r *Row, i int) bool { // a hit with no roll and no flam on it
		return (len(r.Ratchet) <= i || r.Ratchet[i] <= 1) && (len(r.Flam) <= i || r.Flam[i] == 0)
	}
	hitsOff := func(r *Row) []int { // its hits off the beat
		var out []int
		for i, v := range r.Steps {
			if v > 0 && i%4 != 0 && plain(r, i) {
				out = append(out, i)
			}
		}
		return out
	}
	name := func(r *Row) string { return lower(r.Lane.LaneName()) }
	type spot struct {
		l formats.Lane
		i int
	}
	touched := map[spot]bool{} // a step is changed once: two changes never undo each other
	free := func(l formats.Lane, is ...int) bool {
		for _, i := range is {
			if touched[spot{l, i}] {
				return false
			}
		}
		for _, i := range is {
			touched[spot{l, i}] = true
		}
		return true
	}
	changes := []func() string{
		func() string { // a hit moved a step
			if len(layers) == 0 {
				return ""
			}
			r := layers[c.Rng.Intn(len(layers))]
			hs := hitsOff(r)
			if len(hs) == 0 {
				return ""
			}
			i := hs[c.Rng.Intn(len(hs))]
			j := i + []int{-1, 1}[c.Rng.Intn(2)]
			if j < 0 || j >= steps || r.Steps[j] > 0 || j%4 == 0 || !free(r.Lane, i, j) {
				return ""
			}
			r.Steps[j], r.Steps[i] = r.Steps[i], 0
			return fmt.Sprintf("a %s moved (bar %d)", name(r), i/16+1)
		},
		func() string { // one hit less
			if len(layers) == 0 {
				return ""
			}
			r := layers[c.Rng.Intn(len(layers))]
			hs := hitsOff(r)
			if len(hs) < 3 {
				return ""
			}
			i := hs[c.Rng.Intn(len(hs))]
			if !free(r.Lane, i) {
				return ""
			}
			r.Steps[i] = 0
			return fmt.Sprintf("a %s less (bar %d)", name(r), i/16+1)
		},
		func() string { // one hit more, on an offstep, as loud as the row is
			if len(layers) == 0 {
				return ""
			}
			r := layers[c.Rng.Intn(len(layers))]
			sum, n := 0, 0
			for _, v := range r.Steps {
				if v > 0 {
					sum, n = sum+int(v), n+1
				}
			}
			i := c.Rng.Intn(steps/2)*2 + 1 // an odd 16th
			if r.Steps[i] > 0 || !free(r.Lane, i) {
				return ""
			}
			r.Steps[i] = uint8(max(sum/max(n, 1)-12, 40))
			return fmt.Sprintf("a %s more (bar %d)", name(r), i/16+1)
		},
		func() string { // a ghost on the snare or the clap
			for _, l := range []formats.Lane{formats.LaneSnare, formats.LaneClap} {
				r := p.Row(l)
				if r == nil || r.hits() == 0 || len(r.Steps) != steps {
					continue
				}
				i := c.Rng.Intn(steps)
				if i%4 == 0 || r.Steps[i] > 0 || (i > 0 && r.Steps[i-1] > 0) || (i+1 < steps && r.Steps[i+1] > 0) || !free(l, i) {
					return ""
				}
				r.Steps[i] = uint8(38 + c.Rng.Intn(16))
				return fmt.Sprintf("a ghost %s (bar %d)", lower(l.LaneName()), i/16+1)
			}
			return ""
		},
		func() string { // an open hat on an offbeat 8th, the closed one there goes
			if !c.has(formats.LaneOpenHat) {
				return ""
			}
			i := c.Rng.Intn(p.Bars*4)*4 + 2
			o := p.ensure(formats.LaneOpenHat)
			if len(o.Steps) != steps || o.Steps[i] > 0 || !free(formats.LaneOpenHat, i) || !free(formats.LaneClosedHat, i) {
				return ""
			}
			v := uint8(90)
			if ch := p.Row(formats.LaneClosedHat); ch != nil && len(ch.Steps) == steps && ch.Steps[i] > 0 {
				v = ch.Steps[i]
				ch.Steps[i] = 0
			}
			o.Steps[i] = v
			return fmt.Sprintf("an open hat (bar %d)", i/16+1)
		},
		func() string { // a kick before the end of a bar, a pickup into the next
			k := p.Row(formats.LaneKick)
			if k == nil || len(k.Steps) != steps || c.Rng.Intn(3) > 0 {
				return ""
			}
			b := c.Rng.Intn(p.Bars)
			i := b*16 + []int{14, 15}[c.Rng.Intn(2)]
			if k.Steps[i] > 0 || k.Steps[i-1] > 0 || !free(formats.LaneKick, i) {
				return ""
			}
			k.Steps[i] = 88
			return fmt.Sprintf("a kick pickup (bar %d)", b+1)
		},
	}
	want := 2 + c.Rng.Intn(3)
	var what []string
	for tries := 0; len(what) < want && tries < 60; tries++ {
		if w := changes[c.Rng.Intn(len(changes))](); w != "" {
			what = append(what, w)
		}
	}
	if len(what) == 0 { // a bare pattern (a kick and a backbeat): a few hits louder or softer, the kick on the beats as it is
		var at [][2]int
		for ri, r := range p.Rows {
			for i, v := range r.Steps {
				if v > 0 && !(r.Lane == formats.LaneKick && i%4 == 0) {
					at = append(at, [2]int{ri, i})
				}
			}
		}
		for k := 0; k < min(3, len(at)); k++ {
			h := at[c.Rng.Intn(len(at))]
			r := p.Rows[h[0]]
			d := []int{-24, -16, 16, 24}[c.Rng.Intn(4)]
			r.Steps[h[1]] = uint8(min(max(int(r.Steps[h[1]])+d, 30), 127))
		}
		if len(at) > 0 {
			what = append(what, "some hits louder or softer")
		}
	}
	return what
}
