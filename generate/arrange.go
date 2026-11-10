package generate

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// Section is one part of an arrangement: a name, the energy it plays at and what kind of part it is.
type Section struct {
	Name   string
	Kind   string // intro, groove, variation, build, stop, drop, break, outro
	Energy float64
	Bars   int
}

// Result is one generated pattern of an arrangement.
type Result struct {
	Section Section
	Pattern Pattern
	Notes   string // what was done (the fill, the hat pattern), for the log
}

// DefaultArc is a tension curve of eight patterns: a bare intro, the groove, a busier variation, a build, a stop, the drop, a break
// and an outro.
func DefaultArc(bars int) []Section {
	return []Section{
		{"Intro", "intro", 0.15, bars},
		{"Groove", "groove", 0.5, bars},
		{"Variation", "variation", 0.65, bars},
		{"Build", "build", 0.8, bars},
		{"Stop", "stop", 0.3, 1},
		{"Drop", "drop", 1.0, bars},
		{"Break", "break", 0.3, bars},
		{"Outro", "outro", 0.2, bars},
	}
}

// Section writes one section's pattern. Each section has a seed of its own, derived from the arrangement's, so one section can be rolled
// again alone.
func Make(st Style, s Section, seed int64, has func(formats.Lane) bool) Result {
	c := NewCtx(seed, has)
	p := Base(st, s.Bars, s.Energy)
	var notes string
	fill := func() {
		if s.Bars >= 1 {
			notes += "fill: " + p.Fill(c, st, "", s.Bars-1) + "; "
		}
	}
	swing := st.Swing[0]
	if st.Swing[1] > st.Swing[0] {
		swing += c.Rng.Intn(st.Swing[1] - st.Swing[0] + 1)
	}
	switch s.Kind {
	case "intro", "outro":
		if c.chance(0.6) {
			p.HatPattern(c, c.pick(st.HatKinds))
		}
		p.Thin(c)
		if c.chance(0.4) {
			p.Percussion(c, max(3, st.PercHits[0]-2), st.PercHits[0])
		}
		p.Swing(c, swing)
		p.Humanize(c, 0.25)
	case "groove":
		if c.chance(0.5) {
			p.HatPattern(c, c.pick(st.HatKinds))
		}
		if c.chance(0.5) {
			p.Percussion(c, max(3, st.PercHits[0]-2), st.PercHits[0])
		}
		p.Ghosts(c, st.Ghosts*(0.3+0.6*c.Rng.Float64()))
		p.Swing(c, swing)
		p.Humanize(c, 0.3)
		if c.chance(0.7) {
			fill()
		}
	case "variation":
		kind := c.pick(st.HatKinds)
		p.HatPattern(c, kind)
		notes += "hats: " + kind + "; "
		p.Percussion(c, st.PercHits[0], st.PercHits[1])
		p.Ghosts(c, st.Ghosts)
		p.Swing(c, swing)
		p.Humanize(c, 0.35)
		fill()
	case "build":
		p.Percussion(c, st.PercHits[0], st.PercHits[1])
		p.Build(c)
		p.Humanize(c, 0.2)
		notes += "build; "
	case "stop":
		at := []int{2, 4, 6, 8}[c.Rng.Intn(4)]
		p.StopAfter(0, at)
		notes += fmt.Sprintf("stop after step %d; ", at)
		if c.chance(0.5) {
			p.Pickup(c, 0)
			notes += "pickup; "
		}
	case "drop":
		kind := c.pick(st.HatKinds)
		p.HatPattern(c, kind)
		notes += "hats: " + kind + "; "
		p.Percussion(c, st.PercHits[0], st.PercHits[1])
		p.Ghosts(c, st.Ghosts*0.8)
		p.Swing(c, swing)
		p.Humanize(c, 0.35)
		fill()
	case "break":
		p.HalfTime(c)
		p.Density(c, -0.3)
		p.Humanize(c, 0.25)
		notes += "half-time; "
	}
	return Result{Section: s, Pattern: p, Notes: notes}
}

// Arrange writes the whole arc for a seed.
func Arrange(st Style, secs []Section, seed int64, has func(formats.Lane) bool) []Result {
	var out []Result
	for i, s := range secs {
		out = append(out, Make(st, s, seed+int64(i)*7919, has))
	}
	return out
}

// Describe is a line about a result.
func (r Result) Describe() string {
	return fmt.Sprintf("%s (energy %.2f, %d bars, %d notes) %s", r.Section.Name, r.Section.Energy, r.Section.Bars, r.Pattern.Notes(), r.Notes)
}
