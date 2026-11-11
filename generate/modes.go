package generate

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// Modes say how a bank is filled.
//
//	single  one pattern, in the slot in front (or the next free one)
//	fill    one pattern: the groove with a drum fill at the end (one of the style's own)
//	roll    one pattern: the groove with a roll at the end (a snare roll that speeds up, 32nds, flams, a tom or kick roll...)
//	family  one theme and what is done to it: the groove, then a variation of it in each slot, a build, a stop, a drop, a break, an outro
//	random  sixteen patterns that have nothing to do with each other (each has the dice of its own)
//	layers  the groove, then one layer added to the one before, up to the full style
//	drift   each pattern is a small change of the one before, so the bank slowly wanders away from the first
//	fills   the groove, then the same groove with a different drum fill or roll at the end of each pattern (snare rolls that speed up into
//	        32nds, flams, tom and kick rolls, hat triplets, clap stacks), to choose from
//	arc     eight patterns along a tension curve: intro, groove, variation, build, stop, drop, break, outro (each with its own dice)
var Modes = []string{"single", "fill", "roll", "family", "random", "layers", "drift", "arc", "fills"}

// ModeLabel is what a menu says for a mode: how many patterns it makes, and what they are.
func ModeLabel(mode string) string {
	switch mode {
	case "single":
		return "1 pattern"
	case "fill":
		return "1 pattern with a fill"
	case "roll":
		return "1 pattern with a roll"
	case "family":
		return "16 variations of one"
	case "random":
		return "16 different ones"
	case "layers":
		return "16, one layer more each"
	case "drift":
		return "16, each a small change"
	case "arc":
		return "8 from intro to outro"
	case "fills":
		return "16 drum fills on a groove"
	}
	return mode
}

// ModeInfo is a line about a mode.
func ModeInfo(mode string) string {
	switch mode {
	case "fill":
		return "one pattern: a groove of the style with a drum fill in the last bar, one of the fills the style likes. A new one each time."
	case "roll":
		return "one pattern: a groove of the style with a roll in the last bar (a snare roll that speeds up, 32nds, flams, a tom or kick roll, hat or clap rolls)."
	case "single":
		return "one new groove of the style, put in the pattern in front (or the next free one). The tempo of the project stays."
	case "family":
		return "16 patterns that belong together: the groove first, then slots that change one thing each, then a build, a stop, a drop, a break and an outro. Fills the next free slots after the kit's bank."
	case "random":
		return "16 patterns that have nothing to do with each other, each rolled on its own, to pick from."
	case "layers":
		return "the groove, then in each slot one layer more (ghost notes, percussion, hats, an echo...) up to the full style."
	case "drift":
		return "each pattern is a small change of the one before, so the bank slowly wanders away from the first."
	case "arc":
		return "8 patterns along a tension curve: intro, groove, variation, build, stop, drop, break, outro."
	case "fills":
		return "the groove and 15 drum fills and rolls on it, one in each slot (snare rolls, flams, tom and kick rolls...)."
	}
	return ""
}

// Generate fills a bank in a mode: bars is the length of each pattern, seed makes it (the same seed gives the same patterns).
func Generate(mode string, st Style, bars int, seed int64, has func(formats.Lane) bool) ([]Result, error) {
	switch mode {
	case "single", "fill", "roll":
		r, _ := One(mode, st, bars, seed, seed+7919, has, nil, nil)
		return []Result{r}, nil
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
	case "fills":
		return Fills(st, bars, seed, has), nil
	}
	return nil, fmt.Errorf("mode %q: one of single, fill, roll, family, random, layers, drift, arc, fills", mode)
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

// fillList is what the fills mode puts on the groove, a slot each.
var fillList = []string{"snare roll", "accelerating roll", "32nd roll", "snare triplets", "flam run", "tom cascade", "tom roll", "kick stutter",
	"kick roll", "hat roll", "clap roll", "clap stack"}

// OnePattern says whether a mode makes one pattern, not a bank.
func OnePattern(mode string) bool { return mode == "single" || mode == "fill" || mode == "roll" }

// OneFill is one pattern: the groove with a fill, or with a roll, at the end of its last bar.
func OneFill(st Style, bars int, seed int64, has func(formats.Lane) bool, roll bool) Result {
	mode := "fill"
	if roll {
		mode = "roll"
	}
	r, _ := One(mode, st, bars, seed, seed+7919, has, nil, nil)
	return r
}

// Groups are the voices a roll of the dice can leave as they were: the kick, the snare and the clap, the hats, and everything else.
var Groups = []string{"Kick", "Snare and clap", "Hats", "Percussion and the rest"}

// GroupOf is the group of Groups a lane belongs to.
func GroupOf(l formats.Lane) int {
	switch l {
	case formats.LaneKick:
		return 0
	case formats.LaneSnare, formats.LaneClap:
		return 1
	case formats.LaneClosedHat, formats.LaneOpenHat:
		return 2
	}
	return 3
}

// take puts the rows of prev that keep chooses in place of the pattern's own: those lanes play what they played in prev (nothing, when prev
// had no such row), whatever the dice made of them.
func (p *Pattern) take(prev *Pattern, keep func(formats.Lane) bool) {
	if prev == nil || keep == nil || prev.Bars != p.Bars {
		return
	}
	var rows []*Row
	for _, r := range p.Rows {
		if !keep(r.Lane) {
			rows = append(rows, r)
		}
	}
	for _, r := range prev.Clone().Rows {
		if keep(r.Lane) {
			rows = append(rows, r)
		}
	}
	p.Rows = rows
}

// One is the one pattern of a mode that makes one (single, fill, roll). seed makes the groove and fillSeed the fill or the roll on it, so that
// either can be rolled again alone. The lanes keep chooses are taken from prev, the groove of the roll before, and not from the dice: a
// kick or a hat line that is liked stays while the rest is rolled again. The second result is the groove without its fill, to give as prev
// to the next roll.
func One(mode string, st Style, bars int, seed, fillSeed int64, has func(formats.Lane) bool, prev *Pattern, keep func(formats.Lane) bool) (Result, Pattern) {
	if mode != "fill" && mode != "roll" {
		c := NewCtx(seed, has)
		r := Make(st, Section{Name: "Pattern", Kind: "groove", Energy: 0.45 + c.Rng.Float64()*0.4, Bars: bars}, seed, has)
		r.Pattern.take(prev, keep)
		return r, r.Pattern.Clone()
	}
	roll := mode == "roll"
	th := newTheme(st, bars, seed, has)
	groove := th.groove.Clone()
	groove.take(prev, keep)
	c := NewCtx(fillSeed, th.has)
	pool := st.Fills
	if roll {
		pool = RollKinds
	}
	var ok []string
	for _, k := range pool {
		if fillNeeds(k, th.has) {
			ok = append(ok, k)
		}
	}
	if len(ok) == 0 {
		ok = []string{"snare roll"}
	}
	kind := ok[c.Rng.Intn(len(ok))]
	p := groove.Clone()
	p.Fill(c, st, kind, bars-1)
	if keep != nil { // a lane that stays is not moved again: it was humanised when it was made
		c.Lock = map[formats.Lane]bool{}
		for _, r := range p.Rows {
			c.Lock[r.Lane] = keep(r.Lane)
		}
	}
	p.Humanize(c, 0.2)
	name := "Fill: " + kind
	if roll {
		name = "Roll: " + kind
	}
	return Result{Section: Section{Name: name, Kind: "fill", Energy: 0.6, Bars: bars}, Pattern: p, Notes: "the groove with " + kind}, groove
}

// Fills is the groove, and then the groove with a fill or a roll at the end of its last bar in each slot, then a roll build, fills in both
// bars and a roll into a stop.
func Fills(st Style, bars int, seed int64, has func(formats.Lane) bool) []Result {
	th := newTheme(st, bars, seed, has)
	var out []Result
	add := func(name string, p Pattern, notes string) {
		out = append(out, Result{Section: Section{Name: name, Kind: "fill", Energy: 0.6, Bars: bars}, Pattern: p, Notes: notes})
	}
	add("Groove", th.groove.Clone(), "the theme: hats "+th.hatKind)
	for i, kind := range fillList {
		c := NewCtx(seed+int64(i+1)*7919, th.has)
		p := th.groove.Clone()
		if !fillNeeds(kind, th.has) {
			kind = "snare roll"
		}
		p.Fill(c, st, kind, bars-1)
		p.Humanize(c, 0.2)
		add("Fill: "+kind, p, "fill: "+kind)
	}
	c := NewCtx(seed+14*7919, th.has)
	p := th.groove.Clone()
	th.withPerc(&p)
	p.Build(c)
	p.roll(c, "32nd roll", bars-1)
	add("Roll build", p, "the backbeat rolls up and ends in 32nds")
	c = NewCtx(seed+15*7919, th.has)
	p = th.groove.Clone()
	if bars > 1 {
		p.Fill(c, st, "", 0)
	}
	p.Fill(c, st, "accelerating roll", bars-1)
	add("Fills in both bars", p, "a fill halfway and an accelerating roll at the end")
	c = NewCtx(seed+16*7919, th.has)
	p = th.groove.Clone()
	p.Fill(c, st, "snare triplets", bars-1)
	p.StopAfter(bars-1, 15)
	add("Roll, then a stop", p, "triplets, and the last step is silent")
	return out
}
