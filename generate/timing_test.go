package generate

import (
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// an open hat ends before the closed hat that follows it (a closed hat cuts an open one on a 909)
func TestTheOpenHatEndsBeforeTheClosedHat(t *testing.T) {
	p := Pattern{Bars: 1}
	o := p.ensure(formats.LaneOpenHat)
	o.Gate = 400
	o.Steps[2] = 100
	o.Steps[10] = 100
	c := p.ensure(formats.LaneClosedHat)
	c.Steps[3] = 80 // right after the first open hat
	c.Steps[14] = 80
	g := p.choke(o)
	if g[2] == 0 || int(g[2]) >= formats.PPQ/4 {
		t.Errorf("the open hat before a closed hat lasts %d ticks, less than a step (%d) is wanted", g[2], formats.PPQ/4)
	}
	if int(g[10]) <= formats.PPQ/4 || g[10] > 400 {
		t.Errorf("the open hat with room lasts %d ticks", g[10])
	}
}

// the kick and the hits on the beat are not humanised, the off-beat hits are
func TestHumanizeLeavesTheKickAndTheBeat(t *testing.T) {
	p := Pattern{Bars: 1}
	k, h := p.ensure(formats.LaneKick), p.ensure(formats.LaneClosedHat)
	for i := 0; i < 16; i++ {
		h.Steps[i] = 80
	}
	for i := 0; i < 16; i += 4 {
		k.Steps[i] = 120
	}
	p.Humanize(NewCtx(3, hasAll), 0.35)
	for i := 0; i < 16; i += 4 {
		if k.Steps[i] != 120 || h.Steps[i] != 80 {
			t.Fatalf("step %d moved: kick %d, hat %d", i, k.Steps[i], h.Steps[i])
		}
	}
	moved := 0
	for i := 1; i < 16; i += 2 {
		if h.Steps[i] != 80 {
			moved++
		}
	}
	if moved < 4 {
		t.Fatalf("only %d off-beat hats changed", moved)
	}
}

// the claps of a style are nudged (early to lock to the kick, late for a laid-back deep house)
func TestTheClapsAreNudged(t *testing.T) {
	for _, st := range Styles() {
		p := Base(st, 2, 1.0)
		if r := p.Row(formats.LaneClap); r != nil && r.Nudge != st.ClapNudge {
			t.Errorf("%s: the clap nudge is %d, want %d", st.Name, r.Nudge, st.ClapNudge)
		}
	}
}
