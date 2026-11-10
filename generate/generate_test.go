package generate

import (
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func TestEuclid(t *testing.T) {
	str := func(b []bool) string {
		var s strings.Builder
		for _, x := range b {
			if x {
				s.WriteByte('x')
			} else {
				s.WriteByte('.')
			}
		}
		return s.String()
	}
	for _, c := range []struct {
		k, n, rot int
		want      string
	}{
		{4, 16, 0, "x...x...x...x..."},
		{3, 8, 0, "x..x..x."},
		{0, 8, 0, "........"},
		{8, 8, 0, "xxxxxxxx"},
	} {
		if got := str(Euclid(c.k, c.n, c.rot)); got != c.want {
			t.Errorf("Euclid(%d,%d,%d) = %s, want %s", c.k, c.n, c.rot, got, c.want)
		}
	}
	for k := 0; k <= 16; k++ { // always k hits
		n := 0
		for _, x := range Euclid(k, 16, 5) {
			if x {
				n++
			}
		}
		if n != k {
			t.Errorf("%d hits asked, %d made", k, n)
		}
	}
}

func hasAll(formats.Lane) bool { return true }

// The core of a style never moves: the kick is four on the floor at every energy, in every pattern of the arc.
func TestTheKickStaysOnTheBeat(t *testing.T) {
	for _, st := range Styles() {
		for seed := int64(1); seed <= 20; seed++ {
			for _, r := range Arrange(st, DefaultArc(2), seed, hasAll) {
				if r.Section.Kind == "stop" || r.Section.Kind == "break" {
					continue
				}
				k := r.Pattern.Row(formats.LaneKick)
				if k == nil {
					t.Fatalf("%s %s has no kick", st.Name, r.Section.Name)
				}
				for i, v := range k.Steps {
					beat := i%4 == 0
					if beat && v == 0 && !strings.Contains(r.Notes, "stop") {
						t.Errorf("%s %s seed %d: no kick on step %d\n%s", st.Name, r.Section.Name, seed, i, r.Pattern.Text())
					}
					if !beat && v > 0 && !strings.Contains(r.Notes, "kick stutter") {
						t.Errorf("%s %s seed %d: a kick off the beat on step %d\n%s", st.Name, r.Section.Name, seed, i, r.Pattern.Text())
					}
				}
			}
		}
	}
}

func TestEnergyAddsLayers(t *testing.T) {
	for _, st := range Styles() {
		low, high := Base(st, 2, 0.1), Base(st, 2, 1.0)
		if low.Notes() >= high.Notes() {
			t.Errorf("%s: energy 0.1 has %d notes, 1.0 has %d", st.Name, low.Notes(), high.Notes())
		}
		if low.Notes() == 0 {
			t.Errorf("%s: the intro is empty", st.Name)
		}
	}
}

func TestTheSameSeedMakesTheSamePatterns(t *testing.T) {
	st, _ := StyleByName("tech house")
	a := Arrange(st, DefaultArc(2), 42, hasAll)
	b := Arrange(st, DefaultArc(2), 42, hasAll)
	c := Arrange(st, DefaultArc(2), 43, hasAll)
	same := func(x, y []Result) bool {
		for i := range x {
			if x[i].Pattern.Text() != y[i].Pattern.Text() {
				return false
			}
		}
		return true
	}
	if !same(a, b) {
		t.Error("one seed, two results")
	}
	if same(a, c) {
		t.Error("another seed, the same result")
	}
}

func TestEveryPatternOfTheArcIsAValidFile(t *testing.T) {
	pads := map[formats.Lane]proto.PadIndex{}
	for i, l := range []formats.Lane{formats.LaneKick, formats.LaneSnare, formats.LaneClap, formats.LaneClosedHat, formats.LaneOpenHat,
		formats.LanePerc, formats.LaneRim, formats.LaneRide, formats.LaneShaker, formats.LaneTom} {
		pad, _ := proto.NewPadIndex(uint16(i))
		pads[l] = pad
	}
	for _, st := range Styles() {
		for _, r := range Arrange(st, DefaultArc(2), 7, hasAll) {
			b, err := r.Pattern.Bytes(pads)
			if err != nil {
				t.Fatalf("%s %s: %v", st.Name, r.Section.Name, err)
			}
			p, err := formats.ParsePattern(b)
			if err != nil {
				t.Fatalf("%s %s: %v", st.Name, r.Section.Name, err)
			}
			if got := len(p.Events); got != r.Pattern.Notes() && !onlyMissingLanes(r.Pattern, pads) {
				t.Errorf("%s %s: %d notes in the file, %d made", st.Name, r.Section.Name, got, r.Pattern.Notes())
			}
		}
	}
}

func onlyMissingLanes(p Pattern, pads map[formats.Lane]proto.PadIndex) bool {
	for _, r := range p.Rows {
		if _, ok := pads[r.Lane]; !ok {
			return true
		}
	}
	return false
}

func TestAKitWithoutTomsGetsNoTomFill(t *testing.T) {
	st, _ := StyleByName("techno")
	noToms := func(l formats.Lane) bool { return l != formats.LaneTom }
	for seed := int64(0); seed < 50; seed++ {
		p := Base(st, 2, 0.8)
		if k := p.Fill(NewCtx(seed, noToms), st, "", 1); k == "tom cascade" {
			t.Fatalf("seed %d chose a tom fill for a kit with no toms", seed)
		}
	}
}

func TestBuildRisesToSixteenths(t *testing.T) {
	st, _ := StyleByName("house")
	p := Base(st, 2, 0.8)
	p.Build(NewCtx(1, hasAll))
	r := p.Row(formats.LaneClap)
	if r == nil {
		t.Fatal("no clap row")
	}
	last := 0
	for i := 24; i < 32; i++ {
		if r.Steps[i] > 0 {
			last++
		}
	}
	first := 0
	for i := 0; i < 8; i++ {
		if r.Steps[i] > 0 {
			first++
		}
	}
	if last != 8 || first != 2 {
		t.Errorf("the roll: %d hits in the last half bar, %d in the first\n%s", last, first, p.Text())
	}
	if r.Steps[31] <= r.Steps[0] {
		t.Error("the build rises in velocity")
	}
}
