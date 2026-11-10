package generate

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// A family is the sixteen patterns of a bank made from one theme: the first is the groove, the others are what is done to it. They
// share the kick, the claps, the swing, the hat pattern and the percussion line of the groove, so playing through the bank is hearing
// one pattern develop: layers added one at a time (ghost notes, the percussion, a polymeter, an echo, another hat), a fill, the build, the
// stop, the drop (the full style), a break and an outro. The seed makes the theme; each pattern also has dice of its own, so
// rolling a family again with the same seed gives the same sixteen.

// FamilySize is the number of patterns of a family: a bank.
const FamilySize = 16

type theme struct {
	st       Style
	bars     int
	has      func(formats.Lane) bool
	seed     int64
	groove   Pattern
	full     Pattern // the style at full energy
	hatKind  string
	percLane formats.Lane
	perc     []uint8
	swing    int
}

func newTheme(st Style, bars int, seed int64, has func(formats.Lane) bool) *theme {
	if has == nil { // a kit that has every voice
		has = func(formats.Lane) bool { return true }
	}
	c := NewCtx(seed, has)
	th := &theme{st: st, bars: bars, has: has, seed: seed}
	th.groove = Base(st, bars, 0.5)
	th.full = Base(st, bars, 1.0)
	th.hatKind = "style"
	if c.chance(0.6) {
		th.hatKind = c.pick(st.HatKinds)
		th.groove.HatPattern(c, th.hatKind)
	}
	th.swing = st.Swing[0]
	if st.Swing[1] > st.Swing[0] {
		th.swing += c.Rng.Intn(st.Swing[1] - st.Swing[0] + 1)
	}
	// the percussion line of the theme: laid on a scratch pattern, then lent to the patterns that use it
	th.percLane = formats.LanePerc
	if !c.has(th.percLane) {
		th.percLane = formats.LaneRim
	}
	scratch := Pattern{Bars: bars}
	scratch.Percussion(c, st.PercHits[0], st.PercHits[1])
	if r := scratch.Row(th.percLane); r != nil {
		th.perc = append([]uint8(nil), r.Steps...)
	}
	th.groove.Swing(c, th.swing)
	th.groove.Humanize(c, 0.2)
	return th
}

// withPerc puts the theme's percussion line into a pattern.
func (th *theme) withPerc(p *Pattern) {
	if th.perc == nil || !th.has(th.percLane) {
		return
	}
	r := p.ensure(th.percLane)
	for i, v := range th.perc {
		if i < len(r.Steps) && v > r.Steps[i] {
			r.Steps[i] = v
		}
	}
	if r.Gate == 0 {
		r.Gate = 100
	}
}

// withFull puts the lanes of the style at full energy that the pattern lacks (what the drop brings in).
func (th *theme) withFull(p *Pattern, lanes ...formats.Lane) (changed bool) {
	for _, fr := range th.full.Rows {
		if fr.Lane == formats.LaneKick || fr.Lane == formats.LaneClosedHat || !th.has(fr.Lane) {
			continue
		}
		if len(lanes) > 0 && !hasLane(lanes, fr.Lane) {
			continue
		}
		r := p.ensure(fr.Lane)
		for i, v := range fr.Steps {
			if v > r.Steps[i] {
				r.Steps[i] = v
				changed = true
			}
		}
		if r.Gate == 0 {
			r.Gate = fr.Gate
		}
	}
	return changed
}

func hasLane(ls []formats.Lane, l formats.Lane) bool {
	for _, x := range ls {
		if x == l {
			return true
		}
	}
	return false
}

// a step of a family: the name, the kind (for the log and the energy) and what it does to a copy of the groove
type famStep struct {
	name, kind string
	energy     float64
	do         func(th *theme, p *Pattern, c *Ctx) string
}

func (th *theme) cycleOf(p *Pattern) int {
	fits := p.fits()
	if len(fits) == 0 {
		fits = []int{5, 6, 7, 9, 10, 12}
	}
	return fits[0]
}

var famSteps = []famStep{
	{"Groove", "groove", 0.5, func(th *theme, p *Pattern, c *Ctx) string { return "the theme: hats " + th.hatKind }},
	{"Ghost notes", "variation", 0.55, func(th *theme, p *Pattern, c *Ctx) string {
		p.Ghosts(c, min(1, th.st.Ghosts*2+0.2))
		return "ghost notes on the hats, rim and snare"
	}},
	{"Percussion", "variation", 0.6, func(th *theme, p *Pattern, c *Ctx) string {
		th.withPerc(p)
		return "the percussion line of the theme"
	}},
	{"Layers", "variation", 0.65, func(th *theme, p *Pattern, c *Ctx) string {
		if th.withFull(p, formats.LaneRim, formats.LaneShaker, formats.LaneOpenHat) {
			return "rim, shaker and open hat from the full style"
		}
		th.withFull(p) // the groove has those already: the rest of the full style (the ride, the toms)
		return "the ride and the toms of the full style"
	}},
	{"Sixteenth hats", "variation", 0.6, func(th *theme, p *Pattern, c *Ctx) string {
		p.HatPattern(c, "16th")
		return "hats in 16ths"
	}},
	{"Other hats", "variation", 0.6, func(th *theme, p *Pattern, c *Ctx) string {
		p.HatPattern(c, "euclid")
		return "a euclidean hat pattern"
	}},
	{"Polymeter", "variation", 0.65, func(th *theme, p *Pattern, c *Ctx) string {
		th.withPerc(p)
		cycle := th.cycleOf(p)
		hits := max(2, cycle*(3+c.Rng.Intn(3))/10)
		p.Polymeter(c, th.percLane, cycle, hits, c.Rng.Intn(cycle))
		return fmt.Sprintf("the percussion as %d hits in a cycle of %d steps", hits, cycle)
	}},
	{"Echo", "variation", 0.65, func(th *theme, p *Pattern, c *Ctx) string {
		th.withPerc(p)
		var lanes []formats.Lane
		for _, l := range []formats.Lane{formats.LaneClap, formats.LaneRim, th.percLane} {
			if p.Row(l) != nil && th.has(l) {
				lanes = append(lanes, l)
			}
		}
		if len(lanes) == 0 {
			return "no lane to echo"
		}
		l := lanes[c.Rng.Intn(len(lanes))]
		rate, reps, decay := echoShape(c)
		p.Echo(c, l, rate, reps, decay)
		return fmt.Sprintf("echo on the %s, %d repeats every %d steps", l.LaneName(), reps, rate)
	}},
	{"Chance hats", "variation", 0.6, func(th *theme, p *Pattern, c *Ctx) string {
		length, prob := 3+c.Rng.Intn(6), 0.7+0.2*c.Rng.Float64()
		p.Chance(c, formats.LaneClosedHat, prob, length)
		return fmt.Sprintf("the hats play with a chance of %.0f%%, the dice come round every %d hits", prob*100, length)
	}},
	{"Fill", "variation", 0.65, func(th *theme, p *Pattern, c *Ctx) string {
		th.withPerc(p)
		return "fill: " + p.Fill(c, th.st, "", p.Bars-1)
	}},
	{"Build", "build", 0.8, func(th *theme, p *Pattern, c *Ctx) string {
		th.withPerc(p)
		p.Build(c)
		return "the backbeat rolls up, the hats fill in"
	}},
	{"Stop", "stop", 0.3, func(th *theme, p *Pattern, c *Ctx) string {
		at := []int{4, 6, 8, 10}[c.Rng.Intn(4)]
		p.StopAfter(p.Bars-1, at)
		return fmt.Sprintf("everything stops at step %d of the last bar", at+1)
	}},
	{"Drop", "drop", 1.0, func(th *theme, p *Pattern, c *Ctx) string {
		th.withFull(p)
		th.withPerc(p)
		p.HatPattern(c, "16th")
		return "the full style with 16th hats and the percussion"
	}},
	{"Drop with a twist", "drop", 1.0, func(th *theme, p *Pattern, c *Ctx) string {
		th.withFull(p)
		th.withPerc(p)
		cycle := th.cycleOf(p)
		p.Polymeter(c, th.percLane, cycle, max(2, cycle*4/10), c.Rng.Intn(cycle))
		if l := formats.LaneClap; p.Row(l) != nil && th.has(l) {
			p.Echo(c, l, 2, 2, 0.55)
		}
		return fmt.Sprintf("the full style, the percussion as a polymeter of %d steps, an echo on the claps, %s", cycle, "fill: "+p.Fill(c, th.st, "", p.Bars-1))
	}},
	{"Break", "break", 0.3, func(th *theme, p *Pattern, c *Ctx) string {
		if r := p.Row(formats.LaneKick); r != nil {
			for i := range r.Steps {
				r.Steps[i] = 0
			}
		}
		p.HalfTime(c)
		th.withPerc(p)
		if r := p.Row(formats.LaneOpenHat); r != nil && c.chance(0.5) { // the hats and the claps stay, the open hat goes now and then
			for i := range r.Steps {
				r.Steps[i] = 0
			}
		}
		return "no kick, the claps in half time, the hats go on"
	}},
	{"Outro", "outro", 0.2, func(th *theme, p *Pattern, c *Ctx) string {
		for _, r := range p.Rows {
			if r.Lane == formats.LaneKick || r.Lane == formats.LaneClosedHat || r.Lane == formats.LaneOpenHat {
				continue
			}
			for i := range r.Steps {
				r.Steps[i] = 0
			}
		}
		return "the kick, the hats and the open hat"
	}},
}

// Family makes the sixteen patterns of a bank from one theme.
func Family(st Style, bars int, seed int64, has func(formats.Lane) bool) []Result {
	th := newTheme(st, bars, seed, has)
	var out []Result
	for i, fs := range famSteps {
		p := th.groove.Clone()
		var notes string
		if i > 0 {
			c := NewCtx(seed+int64(i)*7919, has)
			notes = fs.do(th, &p, c)
			notes += th.visible(&p, th.groove, fs.kind, c)
			p.Swing(c, th.swing)
			p.Humanize(c, 0.2)
		} else {
			notes = fs.do(th, &p, nil)
		}
		secBars := bars
		if fs.kind == "stop" {
			secBars = bars
		}
		out = append(out, Result{Section: Section{Name: fs.name, Kind: fs.kind, Energy: fs.energy, Bars: secBars}, Pattern: p, Notes: notes})
	}
	return out
}

// visible makes sure a variation changes something: when what it did left the pattern as it was (a ghost note where there is no gap, the
// hat pattern that the groove has already), a few hits are added on the lanes around the core.
func (th *theme) visible(p *Pattern, before Pattern, kind string, c *Ctx) string {
	if kind != "variation" || p.Text() != before.Text() {
		return ""
	}
	for try := 0; try < 4; try++ {
		p.Density(c, 0.5)
		if p.Text() != before.Text() {
			return "; (that changed nothing, so a few more hits)"
		}
	}
	return ""
}
