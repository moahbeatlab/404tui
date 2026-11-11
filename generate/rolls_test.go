package generate

import (
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// a roll plays more notes than steps: a step with a ratchet plays its hits inside the step, louder each time
func TestARatchetPlaysSeveralHitsInAStep(t *testing.T) {
	kick, _ := proto.NewPadIndex(0)
	lane := formats.StepLane{Lane: formats.LaneKick, Steps: make([]uint8, 16), Ratchet: make([]uint8, 16), Flam: make([]uint8, 16)}
	lane.Steps[0], lane.Steps[8] = 100, 100
	lane.Ratchet[8] = 3
	lane.Flam[0] = 20
	b, err := formats.BuildSteps([]formats.StepLane{lane}, map[formats.Lane]proto.PadIndex{formats.LaneKick: kick}, 1)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := formats.ParsePattern(b)
	if err != nil {
		t.Fatal(err)
	}
	var ticks []uint32
	var vels []uint8
	for _, e := range pt.Events {
		if !e.Control {
			ticks = append(ticks, e.Tick)
			vels = append(vels, e.Velocity)
		}
	}
	// the flam before step 1 (clamped to the start of the pattern), step 1, and a triplet on step 9
	if len(ticks) != 5 {
		t.Fatalf("%d notes at %v, want 5", len(ticks), ticks)
	}
	if ticks[2] != 8*120 || ticks[3] != 8*120+40 || ticks[4] != 8*120+80 {
		t.Errorf("the triplet is at %v", ticks[2:])
	}
	if !(vels[2] < vels[3] && vels[3] < vels[4]) {
		t.Errorf("the roll does not grow: %v", vels[2:])
	}
}

func TestEveryRollWritesSomethingAndShowsInTheText(t *testing.T) {
	for _, k := range RollKinds {
		p := Base(Styles()[2], 2, 1.0)
		before := p.Notes()
		c := NewCtx(1, nil)
		p.Fill(c, Styles()[2], k, 1)
		txt := p.Text()
		if p.Notes() == before && txt == Base(Styles()[2], 2, 1.0).Text() {
			t.Errorf("%s changed nothing", k)
		}
		if k != "kick roll" && k != "flam run" && !strings.ContainsAny(txt, "234") {
			t.Errorf("%s: no ratchet in the text:\n%s", k, txt)
		}
	}
}

func TestTheFillsModeHasTheGrooveAndFifteenFills(t *testing.T) {
	for _, st := range Styles() {
		rs := Fills(st, 2, 3, nil)
		if len(rs) != FamilySize {
			t.Fatalf("%s: %d patterns", st.Name, len(rs))
		}
		g := rs[0].Pattern.Text()
		same := 0
		for _, r := range rs[1:] {
			if r.Pattern.Text() == g {
				same++
			}
		}
		if same > 1 {
			t.Errorf("%s: %d fills left the groove as it was", st.Name, same)
		}
		// the first bar of a fill pattern is the groove's: a fill is at the end
		if !strings.HasPrefix(rs[2].Pattern.Text(), strings.SplitN(g, "|", 2)[0][:11]) {
			t.Errorf("%s: the lanes of a fill pattern differ from the groove's", st.Name)
		}
	}
}
