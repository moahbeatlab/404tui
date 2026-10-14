package formats_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// tmpl finds the groove template by name, or fails.
func tmpl(t *testing.T, name string) formats.GrooveTemplate {
	t.Helper()
	for _, g := range formats.GrooveTemplates() {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no groove template named %q", name)
	return formats.GrooveTemplate{}
}

func TestGrooveTemplatesAreTheGenres(t *testing.T) {
	got := formats.GrooveTemplates()
	want := []string{"House", "Deep house", "Tech house", "Techno", "Hypnotic techno", "Hip-hop"}
	if len(got) != len(want) {
		t.Fatalf("got %d templates, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("template %d: got %q want %q", i, got[i].Name, w)
		}
	}
}

// allLanes maps every lane a groove uses to its own pad, A1 upward.
func allLanes(t *testing.T, g formats.GrooveTemplate) map[formats.Lane]proto.PadIndex {
	t.Helper()
	pads := map[formats.Lane]proto.PadIndex{}
	for i, l := range g.Lanes {
		pads[l] = padOf(t, fmt.Sprintf("A%d", i+1))
	}
	return pads
}

// hitsOf are the ticks and velocities of a pad's notes in a built groove.
func hitsOf(t *testing.T, b []byte, pad proto.PadIndex) (ticks []uint32, vels []uint8) {
	t.Helper()
	p, err := formats.ParsePattern(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range p.Events {
		if !e.Control && e.Pad == pad {
			ticks, vels = append(ticks, e.Tick), append(vels, e.Velocity)
		}
	}
	return ticks, vels
}

func TestEveryGrooveBuildsAtItsOwnLengthWithEveryLane(t *testing.T) {
	for _, g := range formats.GrooveTemplates() {
		pads := allLanes(t, g)
		b, err := g.BuildGroove(pads, 0)
		if err != nil {
			t.Fatalf("%s: %v", g.Name, err)
		}
		p, err := formats.ParsePattern(b)
		if err != nil {
			t.Fatalf("%s does not parse: %v", g.Name, err)
		}
		if want := uint32(g.Bars() * 1920); p.Length != want {
			t.Errorf("%s: length %d, want %d (%d bars)", g.Name, p.Length, want, g.Bars())
		}
		for _, l := range g.Lanes {
			if ticks, _ := hitsOf(t, b, pads[l]); len(ticks) == 0 {
				t.Errorf("%s: the %s lane plays nothing", g.Name, l.LaneName())
			}
		}
		if g.Tempo == "" {
			t.Errorf("%s has no tempo", g.Name)
		}
	}
}

func TestTechHouseIsAFourBarPhraseWithItsFills(t *testing.T) {
	g := tmpl(t, "Tech house")
	if g.Bars() != 4 {
		t.Fatalf("tech house is %d bars, want 4", g.Bars())
	}
	pads := allLanes(t, g)
	b, err := g.BuildGroove(pads, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The snare doubles at the end of every two bars: steps 15 and 16 of bars 2 and 4.
	snare, _ := hitsOf(t, b, pads[formats.LaneSnare])
	want := []uint32{1920 + 14*120, 1920 + 15*120, 3*1920 + 14*120, 3*1920 + 15*120}
	if fmt.Sprint(snare) != fmt.Sprint(want) {
		t.Errorf("snare on %v, want %v", snare, want)
	}
	// The clap is pushed early: 6 ticks before beats 2 and 4.
	clap, _ := hitsOf(t, b, pads[formats.LaneClap])
	if clap[0] != 480-6 || clap[1] != 1440-6 {
		t.Errorf("clap on %v, want 6 ticks ahead of 480 and 1440", clap[:2])
	}
}

func TestSwingNudgeAndGhostsLandWhereTheStyleSays(t *testing.T) {
	// Deep house: the clap sits 8 ticks back; the closed hat's off-beat 16ths swing at 60%,
	// which pushes them 24 ticks (10% of two 16ths).
	deep := tmpl(t, "Deep house")
	pads := allLanes(t, deep)
	b, _ := deep.BuildGroove(pads, 0)
	if clap, _ := hitsOf(t, b, pads[formats.LaneClap]); clap[0] != 488 {
		t.Errorf("deep house clap on %d, want 488", clap[0])
	}
	if hat, _ := hitsOf(t, b, pads[formats.LaneClosedHat]); hat[0] != 120+24 {
		t.Errorf("deep house first closed hat on %d, want %d", hat[0], 120+24)
	}
	// Techno: ghost kicks just before beats 2 and 4, soft against the four on the floor.
	techno := tmpl(t, "Techno")
	pads = allLanes(t, techno)
	b, _ = techno.BuildGroove(pads, 0)
	kick, vels := hitsOf(t, b, pads[formats.LaneKick])
	if kick[1] != 360 || vels[1] >= vels[0] {
		t.Errorf("techno kick on %v (velocities %v): want a ghost at 360 softer than the beat", kick[:3], vels[:3])
	}
}

func TestHouseGrooveLandsKicksOnEveryBeat(t *testing.T) {
	kick, clap, oh := padOf(t, "A1"), padOf(t, "A2"), padOf(t, "A3")
	house := tmpl(t, "House")
	b, err := house.BuildGroove(map[formats.Lane]proto.PadIndex{
		formats.LaneKick:    kick,
		formats.LaneClap:    clap,
		formats.LaneOpenHat: oh,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	p, err := formats.ParsePattern(b)
	if err != nil {
		t.Fatalf("built groove does not parse: %v", err)
	}
	if p.Length != 3840 {
		t.Errorf("length %d, want 3840 (2 bars)", p.Length)
	}
	// Kick on every beat of both bars, and the syncopated one near the end of bar 2.
	var kickTicks []uint32
	for _, e := range p.Events {
		if !e.Control && e.Pad == kick {
			kickTicks = append(kickTicks, e.Tick)
		}
	}
	wantKick := []uint32{0, 480, 960, 1440, 1920, 2400, 2880, 3360, 3600}
	if len(kickTicks) != len(wantKick) {
		t.Fatalf("kick hits %v, want %v", kickTicks, wantKick)
	}
	for i, w := range wantKick {
		if kickTicks[i] != w {
			t.Errorf("kick %d on %d, want %d", i, kickTicks[i], w)
		}
	}
	// Clap on beats 2 and 4 of each bar, with the two pickups closing bar 2.
	var clapTicks []uint32
	for _, e := range p.Events {
		if !e.Control && e.Pad == clap {
			clapTicks = append(clapTicks, e.Tick)
		}
	}
	wantClap := []uint32{480, 1440, 2400, 3360, 3600, 3720}
	if len(clapTicks) != len(wantClap) {
		t.Fatalf("clap hits %v, want %v", clapTicks, wantClap)
	}
	for i, w := range wantClap {
		if clapTicks[i] != w {
			t.Errorf("clap %d on %d, want %d", i, clapTicks[i], w)
		}
	}
}

func TestGrooveSkipsUnmappedLanes(t *testing.T) {
	// Hip-hop names kick, snare, closed hat; map only the kick. The result has only kick notes.
	kick := padOf(t, "A1")
	hh := tmpl(t, "Hip-hop")
	b, err := hh.BuildGroove(map[formats.Lane]proto.PadIndex{formats.LaneKick: kick}, 2)
	if err != nil {
		t.Fatal(err)
	}
	p, err := formats.ParsePattern(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range p.Events {
		if e.Control {
			continue
		}
		if e.Pad != kick {
			t.Errorf("note on %s; only the kick was mapped", e.Pad)
		}
	}
	if len(p.Events) == 0 {
		t.Error("the kick lane wrote no notes")
	}
}

func TestGrooveWithNoMappedPadsIsAnError(t *testing.T) {
	house := tmpl(t, "House")
	if _, err := house.BuildGroove(map[formats.Lane]proto.PadIndex{}, 2); err == nil {
		t.Error("a groove with no mapped pad was built anyway")
	}
}

func TestBlankPatternHasNoNotesButParses(t *testing.T) {
	b, err := formats.BlankPattern(2)
	if err != nil {
		t.Fatal(err)
	}
	p, err := formats.ParsePattern(b)
	if err != nil {
		t.Fatalf("blank pattern does not parse: %v", err)
	}
	if p.Length != 3840 {
		t.Errorf("length %d, want 3840", p.Length)
	}
	for _, e := range p.Events {
		if !e.Control {
			t.Errorf("blank pattern has a note on %s at %d", e.Pad, e.Tick)
		}
	}
	// The trailer matches StartPattern's for 2 bars.
	want := []byte{2, 0, 0, 0, 0, 0x80, 0x04, 0x01}
	if !bytes.Equal(p.Trailer, want) {
		t.Errorf("trailer % x, want % x", p.Trailer, want)
	}
}
