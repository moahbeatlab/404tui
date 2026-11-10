package generate

import (
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
)

// Ctx is what a change needs: the dice, and which lanes the kit has (a fill that needs toms is not chosen for a kit with none). Lock
// holds lanes that are left as they are.
type Ctx struct {
	Rng  *rand.Rand
	Has  func(formats.Lane) bool
	Lock map[formats.Lane]bool
}

// NewCtx is a context for a seed.
func NewCtx(seed int64, has func(formats.Lane) bool) *Ctx {
	return &Ctx{Rng: rand.New(rand.NewSource(seed)), Has: has}
}

func (c *Ctx) has(l formats.Lane) bool  { return c.Has == nil || c.Has(l) }
func (c *Ctx) open(l formats.Lane) bool { return !c.Lock[l] }
func (c *Ctx) chance(p float64) bool    { return c.Rng.Float64() < p }

// pick chooses a key by its weight (the keys are taken in order, so a seed always chooses the same).
func (c *Ctx) pick(w map[string]int) string {
	var keys []string
	total := 0
	for k, n := range w {
		keys = append(keys, k)
		total += n
	}
	sort.Strings(keys)
	if total == 0 {
		return ""
	}
	r := c.Rng.Intn(total)
	for _, k := range keys {
		if r -= w[k]; r < 0 {
			return k
		}
	}
	return keys[0]
}

// Base is the style's pattern at an energy (0..1): the lanes that enter at or below it and leave above it, over bars bars.
func Base(st Style, bars int, energy float64) Pattern {
	p := Pattern{Bars: bars}
	for _, sl := range st.Lanes {
		if energy < sl.From || (sl.Until > 0 && energy >= sl.Until) {
			continue
		}
		steps, err := formats.ParseHits(sl.Hits)
		if err != nil {
			continue
		}
		r := p.ensure(sl.Lane)
		for i := range r.Steps {
			if v := steps[i%len(steps)]; v > r.Steps[i] {
				r.Steps[i] = v
			}
		}
		if sl.Swing > 0 {
			r.Swing = sl.Swing
		}
		if sl.Gate > 0 {
			r.Gate = sl.Gate
		}
	}
	if r := p.Row(formats.LaneClap); r != nil {
		r.Nudge = st.ClapNudge
	}
	return p
}

var core = map[formats.Lane]bool{formats.LaneKick: true, formats.LaneClap: true, formats.LaneSnare: true}

// ---- changes

// Ghosts adds quiet hits between the others on the snare and the hats (amount 0..1).
func (p *Pattern) Ghosts(c *Ctx, amount float64) {
	for _, l := range []formats.Lane{formats.LaneClosedHat, formats.LaneRim, formats.LaneSnare} {
		r := p.Row(l)
		if r == nil || !c.open(l) {
			continue
		}
		pr := amount * 0.35
		if l == formats.LaneClosedHat {
			pr = amount * 0.25
		}
		for i, v := range r.Steps {
			if v == 0 && i%2 == 1 && c.chance(pr) {
				r.Steps[i] = 48
			}
		}
	}
}

// HatPattern replaces the closed hat with another pattern: "8th", "16th" or "euclid".
func (p *Pattern) HatPattern(c *Ctx, kind string) {
	if !c.open(formats.LaneClosedHat) {
		return
	}
	r := p.ensure(formats.LaneClosedHat)
	if r.Gate == 0 {
		r.Gate = 60
	}
	var bar [16]uint8
	switch kind {
	case "8th":
		for i := 0; i < 16; i += 2 {
			bar[i] = 80
			if i%4 == 2 {
				bar[i] = 100
			}
		}
	case "euclid":
		hits := 7 + c.Rng.Intn(5)
		rot := c.Rng.Intn(16)
		for i, h := range Euclid(hits, 16, rot) {
			if h {
				bar[i] = 100
				if i%4 != 0 {
					bar[i] = 78
				}
			}
		}
	default: // 16th
		for i := range bar {
			bar[i] = []uint8{82, 50, 104, 50}[i%4]
		}
	}
	for i := range r.Steps {
		r.Steps[i] = bar[i%16]
	}
}

// Percussion lays a euclidean line on the perc (or the rim when the kit has no perc): hits of 16 steps in each bar, turned by the dice.
func (p *Pattern) Percussion(c *Ctx, from, to int) {
	l := formats.LanePerc
	if !c.has(l) {
		l = formats.LaneRim
	}
	if !c.open(l) || !c.has(l) {
		return
	}
	r := p.ensure(l)
	hits := from
	if to > from {
		hits += c.Rng.Intn(to - from + 1)
	}
	rot := c.Rng.Intn(16)
	for i := range r.Steps {
		r.Steps[i] = 0
	}
	for b := 0; b < p.Bars; b++ {
		h := hits
		if b%2 == 1 && c.chance(0.5) {
			h++
		}
		for i, on := range Euclid(h, 16, rot) {
			if on {
				r.Steps[b*16+i] = []uint8{100, 70, 85, 60}[i%4]
			}
		}
	}
}

// Density adds (delta > 0) or takes away (delta < 0) hits on the lanes that are not the core: delta is the share of the steps.
func (p *Pattern) Density(c *Ctx, delta float64) {
	for _, r := range p.Rows {
		if core[r.Lane] || r.Lane == formats.LaneKick || !c.open(r.Lane) {
			continue
		}
		for i, v := range r.Steps {
			switch {
			case delta > 0 && v == 0 && c.chance(delta*0.4):
				r.Steps[i] = 60
			case delta < 0 && v > 0 && c.chance(-delta):
				r.Steps[i] = 0
			}
		}
	}
}

// Swing sets the swing of every lane but the kick (50 is straight).
func (p *Pattern) Swing(c *Ctx, amount int) {
	for _, r := range p.Rows {
		if r.Lane != formats.LaneKick && c.open(r.Lane) {
			r.Swing = amount
		}
	}
}

// Humanize moves the velocities a little (amount 0..1).
func (p *Pattern) Humanize(c *Ctx, amount float64) {
	spread := int(amount * 32) // 0.2 is +-6, 0.35 is +-11: the 5 to 12 steps of the off-beat hits
	if spread < 1 {
		return
	}
	for _, r := range p.Rows {
		if !c.open(r.Lane) || r.Lane == formats.LaneKick { // the kick and the hits on the beat stay as they are: they hold the groove
			continue
		}
		for i, v := range r.Steps {
			if v > 0 && i%4 != 0 {
				r.Steps[i] = uint8(max(20, min(127, int(v)+c.Rng.Intn(2*spread+1)-spread)))
			}
		}
	}
}

// HalfTime puts the claps and snares on the third beat of the bar only (the groove at half speed).
func (p *Pattern) HalfTime(c *Ctx) {
	for _, l := range []formats.Lane{formats.LaneClap, formats.LaneSnare} {
		r := p.Row(l)
		if r == nil || !c.open(l) {
			continue
		}
		for b := 0; b < p.Bars; b++ {
			v := max(r.Steps[b*16+4], r.Steps[b*16+12])
			r.Steps[b*16+4], r.Steps[b*16+12] = 0, 0
			if v > 0 {
				r.Steps[b*16+8] = v
			}
		}
	}
}

// StopAfter silences everything in a bar from a step on (a stop before the drop).
func (p *Pattern) StopAfter(bar, step int) {
	for _, r := range p.Rows {
		for i := bar*16 + step; i < (bar+1)*16 && i < len(r.Steps); i++ {
			r.Steps[i] = 0
		}
	}
}

// ---- fills

// Fill writes a fill into the second half of a bar (the kind is one of Fills, "" lets the dice choose from the style's).
func (p *Pattern) Fill(c *Ctx, st Style, kind string, bar int) string {
	if kind == "" {
		w := map[string]int{}
		for _, k := range st.Fills {
			if fillNeeds(k, c.has) {
				w[k] = 1
			}
		}
		kind = c.pick(w)
	}
	set := func(l formats.Lane, step int, v uint8) {
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
	snare := p.backbeat(c)
	switch kind {
	case "snare roll":
		clr(8, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		for i, s := range []int{8, 10, 12, 13, 14, 15} {
			set(snare, s, uint8(70+i*10))
		}
	case "tom cascade":
		clr(8, 16, formats.LaneClosedHat, formats.LaneOpenHat)
		for i, v := range []uint8{120, 95, 120, 95, 120, 95, 120, 120} {
			set(formats.LaneTom, 8+i, v)
		}
	case "clap stack":
		for i, s := range []int{11, 12, 13, 14, 15} {
			set(formats.LaneClap, s, uint8(80+i*10))
		}
	case "open hat swell":
		clr(8, 16, formats.LaneClosedHat)
		for i, s := range []int{8, 10, 12, 14} {
			set(formats.LaneOpenHat, s, uint8(60+i*20))
		}
	case "ghost shuffle":
		for _, s := range []int{9, 11, 13, 15} {
			set(snare, s, 50)
		}
		set(snare, 14, 100)
	case "rim run":
		for i := 8; i < 16; i++ {
			set(formats.LaneRim, i, uint8(50+(i-8)*10))
		}
	case "kick stutter":
		for _, s := range []int{11, 13, 14, 15} {
			set(formats.LaneKick, s, 90)
		}
	case "hat stop":
		clr(12, 16, formats.LaneClosedHat, formats.LaneOpenHat, formats.LaneShaker)
	}
	return kind
}

func fillNeeds(kind string, has func(formats.Lane) bool) bool {
	h := func(l formats.Lane) bool { return has == nil || has(l) }
	switch kind {
	case "snare roll":
		return h(formats.LaneSnare) || h(formats.LaneClap)
	case "tom cascade":
		return h(formats.LaneTom)
	case "clap stack":
		return h(formats.LaneClap)
	case "open hat swell":
		return h(formats.LaneOpenHat)
	case "ghost shuffle":
		return h(formats.LaneSnare) || h(formats.LaneClap)
	case "rim run":
		return h(formats.LaneRim)
	}
	return true
}

// Build makes a build-up of the whole pattern: the snare (or clap) goes from the beats to 8ths and to 16ths, rising in velocity, and the
// hats get busier. Where it speeds up and whether the hats cut out for the last beat are up to the dice.
func (p *Pattern) Build(c *Ctx) {
	l := p.backbeat(c)
	if !c.has(l) || !c.open(l) {
		return
	}
	r := p.ensure(l)
	for i := range r.Steps {
		r.Steps[i] = 0
	}
	n := p.Bars * 16
	t8 := []float64{0.35, 0.5, 0.6}[c.Rng.Intn(3)]
	t16 := []float64{0.7, 0.75, 0.85}[c.Rng.Intn(3)]
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n)
		gap := 4
		switch {
		case t >= t16:
			gap = 1
		case t >= t8:
			gap = 2
		}
		if i%gap == 0 {
			r.Steps[i] = uint8(60 + 65*i/n)
		}
	}
	if h := p.Row(formats.LaneClosedHat); h != nil && c.open(formats.LaneClosedHat) {
		from := n/2 + c.Rng.Intn(n/4+1)
		for i := from; i < n; i++ {
			if h.Steps[i] == 0 {
				h.Steps[i] = 60
			}
		}
		if c.chance(0.5) { // the hats drop out for the last beat
			for i := n - 4; i < n; i++ {
				h.Steps[i] = 0
			}
		}
	}
}

// Thin leaves only some of the optional lanes of a bare pattern (an intro or an outro): the open hat, the shaker, the rim, the perc.
func (p *Pattern) Thin(c *Ctx) {
	for _, r := range p.Rows {
		if core[r.Lane] || r.Lane == formats.LaneKick || !c.open(r.Lane) {
			continue
		}
		if c.chance(0.35) {
			for i := range r.Steps {
				r.Steps[i] = 0
			}
		}
	}
}

// Pickup puts a short roll on the last steps of the bar, into what comes next.
func (p *Pattern) Pickup(c *Ctx, bar int) {
	l := p.backbeat(c)
	if !c.has(l) || !c.open(l) {
		return
	}
	r := p.ensure(l)
	steps := [][]int{{14, 15}, {15}, {13, 14, 15}, {12, 14}}[c.Rng.Intn(4)]
	for i, s := range steps {
		r.Steps[bar*16+s] = uint8(80 + i*15)
	}
}

// backbeat is the lane that has the backbeat: the snare when the pattern has one, else the clap (house and techno are clap music),
// else whichever of the two the kit has.
func (p *Pattern) backbeat(c *Ctx) formats.Lane {
	switch {
	case p.Row(formats.LaneSnare) != nil && c.has(formats.LaneSnare):
		return formats.LaneSnare
	case p.Row(formats.LaneClap) != nil && c.has(formats.LaneClap):
		return formats.LaneClap
	case c.has(formats.LaneSnare):
		return formats.LaneSnare
	}
	return formats.LaneClap
}

// ---- the effects (after MIDI Conductor's chain: each one takes the pattern as the one before left it)

// Polymeter lays a euclidean line of hits in cycle steps on a lane and repeats it across the pattern. A cycle that does not divide the
// bar drifts against the kick and comes round again when the pattern does; one that does not divide the pattern is cut where it ends.
func (p *Pattern) Polymeter(c *Ctx, lane formats.Lane, cycle, hits, rot int) {
	if !c.has(lane) || !c.open(lane) || cycle < 1 {
		return
	}
	r := p.ensure(lane)
	line := Euclid(hits, cycle, rot)
	for i := range r.Steps {
		r.Steps[i] = 0
		if line[i%cycle] {
			r.Steps[i] = []uint8{100, 70, 85, 60}[(i%cycle)%4]
		}
	}
}

// fits are the cycles from 3 to 24 steps that divide the pattern evenly and are not a plain beat, so a polymeter loops with the pattern.
func (p Pattern) fits() []int {
	var out []int
	for n := 3; n <= 24; n++ {
		if (p.Bars*16)%n == 0 && 16%n != 0 {
			out = append(out, n)
		}
	}
	return out
}

// Echo repeats each hit of a lane: repeats times, rate steps apart, each quieter by decay (0..1 of the one before). The repeats go
// round the end of the pattern to its start, as it loops, and never replace a louder hit.
func (p *Pattern) Echo(c *Ctx, lane formats.Lane, rate, repeats int, decay float64) {
	r := p.Row(lane)
	if r == nil || !c.open(lane) || rate < 1 || repeats < 1 {
		return
	}
	n := len(r.Steps)
	src := append([]uint8(nil), r.Steps...)
	for i, v := range src {
		if v == 0 {
			continue
		}
		vel := float64(v)
		for k := 1; k <= repeats; k++ {
			if vel *= decay; vel < 20 {
				break
			}
			j := (i + k*rate) % n
			if r.Steps[j] < uint8(vel) && src[j] == 0 {
				r.Steps[j] = uint8(vel)
			}
		}
	}
}

// Chance drops hits at random: each hit of the lane below accent strength plays with the probability p. The dice are rolled length
// times and then repeat, so the choices come round again every length hits and the groove settles (MIDI Conductor's LOOP).
func (p *Pattern) Chance(c *Ctx, lane formats.Lane, prob float64, length int) {
	r := p.Row(lane)
	if r == nil || !c.open(lane) || length < 1 {
		return
	}
	keep := make([]bool, length)
	for i := range keep {
		keep[i] = c.chance(prob)
	}
	n := 0
	for i, v := range r.Steps {
		if v == 0 || v >= 110 || i%2 == 0 { // the accents stay, and the hits on the beat and the 8ths: only the off-beat 16ths are left to chance
			continue
		}
		if !keep[n%length] {
			r.Steps[i] = 0
		}
		n++
	}
}

// echoShape is the rate (in steps), the number of repeats and the fade of an echo: mostly the dotted 8th of a dub delay (3 steps, three
// repeats at about 60%, 35% and 20% of the velocity), else the dice.
func echoShape(c *Ctx) (rate, repeats int, decay float64) {
	if c.chance(0.6) {
		return 3, 3, 0.55 + 0.07*c.Rng.Float64()
	}
	return 1 + c.Rng.Intn(4), 1 + c.Rng.Intn(3), 0.45 + 0.25*c.Rng.Float64()
}
