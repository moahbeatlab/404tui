package generate

import (
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

func rowText(p Pattern, l formats.Lane) string {
	r := p.Row(l)
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, v := range r.Steps {
		b.WriteByte(mark(v))
	}
	return b.String()
}

func TestPolymeterCyclesAcrossThePattern(t *testing.T) {
	p := Pattern{Bars: 3} // 48 steps: a cycle of 6 divides it
	p.Polymeter(NewCtx(1, hasAll), formats.LanePerc, 6, 2, 0)
	r := p.Row(formats.LanePerc)
	hits := 0
	for i, v := range r.Steps {
		if v > 0 {
			hits++
			if i%6 != 0 && i%6 != 3 {
				t.Errorf("a hit on step %d: 2 hits in 6 are on 0 and 3", i)
			}
		}
	}
	if hits != 16 {
		t.Errorf("%d hits, want 16 (2 in each of 8 cycles)", hits)
	}
	if f := p.fits(); len(f) == 0 || f[0] != 3 {
		t.Errorf("cycles that fit 3 bars: %v", f)
	}
	two := Pattern{Bars: 2}
	if f := two.fits(); len(f) != 0 {
		t.Errorf("no odd cycle divides two bars, got %v", f)
	}
}

func TestEchoRepeatsAndFades(t *testing.T) {
	p := Pattern{Bars: 1}
	r := p.ensure(formats.LaneClap)
	r.Steps[4] = 120
	p.Echo(NewCtx(1, hasAll), formats.LaneClap, 2, 3, 0.5)
	if r.Steps[6] != 60 || r.Steps[8] != 30 || r.Steps[10] != 0 { // 120, 60, 30; the third is under the floor of 20? 15
		t.Errorf("echo: %v", r.Steps)
	}
	// round the end of the pattern
	q := Pattern{Bars: 1}
	q.ensure(formats.LaneRim).Steps[15] = 100
	q.Echo(NewCtx(1, hasAll), formats.LaneRim, 1, 1, 0.6)
	if q.Row(formats.LaneRim).Steps[0] != 60 {
		t.Errorf("the repeat goes to the start of the loop: %v", q.Row(formats.LaneRim).Steps)
	}
	// it never replaces a hit that is there
	z := Pattern{Bars: 1}
	zr := z.ensure(formats.LaneRim)
	zr.Steps[0], zr.Steps[2] = 100, 90
	z.Echo(NewCtx(1, hasAll), formats.LaneRim, 2, 1, 0.5)
	if zr.Steps[2] != 90 {
		t.Errorf("a hit was replaced: %v", zr.Steps)
	}
}

func TestChanceRepeatsItsChoicesAndKeepsAccents(t *testing.T) {
	p := Pattern{Bars: 4}
	r := p.ensure(formats.LaneClosedHat)
	for i := range r.Steps {
		r.Steps[i] = 80
	}
	r.Steps[0] = 120 // an accent
	p.Chance(NewCtx(3, hasAll), formats.LaneClosedHat, 0.5, 4)
	if r.Steps[0] != 120 {
		t.Error("the accent was dropped")
	}
	// the hits that are not accents are numbered 0,1,2,...; hit n uses the choice n%4, so the choices come round every 4 hits
	var kept []bool
	for i := 1; i < len(r.Steps); i++ {
		kept = append(kept, r.Steps[i] > 0)
	}
	for n := 4; n < len(kept); n++ {
		if kept[n] != kept[n-4] {
			t.Fatalf("hit %d differs from hit %d: the dice repeat every 4 hits", n, n-4)
		}
	}
	all, none := 0, 0
	for _, k := range kept {
		if k {
			all++
		} else {
			none++
		}
	}
	if all == 0 && none == 0 {
		t.Fatal("no hits")
	}
	// probability 1 keeps everything, 0 drops everything below accent
	a := Pattern{Bars: 1}
	ar := a.ensure(formats.LaneClosedHat)
	for i := range ar.Steps {
		ar.Steps[i] = 80
	}
	a.Chance(NewCtx(1, hasAll), formats.LaneClosedHat, 1, 8)
	if a.Notes() != 16 {
		t.Errorf("chance 1 dropped hits: %d", a.Notes())
	}
	a.Chance(NewCtx(1, hasAll), formats.LaneClosedHat, 0, 8)
	if a.Notes() != 0 {
		t.Errorf("chance 0 kept hits: %d", a.Notes())
	}
}

// The effects come up in the arrangements, and never touch the kick.
func TestTheEffectsShowUpInTheArc(t *testing.T) {
	for _, st := range Styles() {
		seen := map[string]int{}
		for seed := int64(1); seed <= 200; seed++ {
			for _, r := range Arrange(st, DefaultArc(2), seed, hasAll) {
				for _, k := range []string{"polymeter", "echo", "chance"} {
					if strings.Contains(r.Notes, k) {
						seen[k]++
					}
				}
			}
		}
		for _, k := range []string{"polymeter", "echo", "chance"} {
			if seen[k] == 0 {
				t.Errorf("%s: no %s in 200 seeds", st.Name, k)
			}
		}
		t.Logf("%s: %v", st.Name, seen)
	}
}
