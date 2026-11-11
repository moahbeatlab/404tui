package generate

import (
	"bytes"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func rowSteps(p Pattern, l formats.Lane) []uint8 {
	if r := p.Row(l); r != nil {
		return r.Steps
	}
	return nil
}

func TestEveryModeMakesValidPatterns(t *testing.T) {
	for _, st := range Styles() {
		for _, mode := range Modes {
			rs, err := Generate(mode, st, 2, 7, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := FamilySize
			if mode == "arc" {
				want = 8
			}
			if len(rs) != want {
				t.Fatalf("%s %s: %d patterns, want %d", st.Name, mode, len(rs), want)
			}
		}
	}
	if _, err := Generate("nope", Styles()[0], 2, 1, nil); err == nil {
		t.Fatal("an unknown mode is an error")
	}
}

// a family is one theme: the variations keep the kick of the groove and change something else
func TestFamilyIsOneTheme(t *testing.T) {
	for _, st := range Styles() {
		for seed := int64(1); seed <= 20; seed++ {
			rs := Family(st, 2, seed, nil)
			g := rs[0].Pattern
			changed := 0
			for _, r := range rs[1:10] { // the variations
				if !bytes.Equal(rowSteps(r.Pattern, formats.LaneKick), rowSteps(g, formats.LaneKick)) {
					// only the humanising moves the velocity of the kick: the hits must stay where they are
					for i, v := range rowSteps(g, formats.LaneKick) {
						if v > 0 && rowSteps(r.Pattern, formats.LaneKick)[i] == 0 { // a kick stutter may add hits, nothing is taken away
							t.Fatalf("%s seed %d %s: the kick moved", st.Name, seed, r.Section.Name)
						}
					}
				}
				if r.Pattern.Text() != g.Text() {
					changed++
				}
			}
			if changed < 7 {
				t.Fatalf("%s seed %d: only %d of 9 variations differ from the groove", st.Name, seed, changed)
			}
			if k := rowSteps(rs[14].Pattern, formats.LaneKick); k != nil {
				for _, v := range k {
					if v != 0 {
						t.Fatalf("%s seed %d: the break has a kick", st.Name, seed)
					}
				}
			}
		}
	}
}

func TestSeedsMakeDifferentFamilies(t *testing.T) {
	for _, st := range Styles() {
		seen := map[string]bool{}
		for seed := int64(1); seed <= 40; seed++ {
			txt := ""
			for _, r := range Family(st, 2, seed, nil) {
				txt += r.Pattern.Text()
			}
			seen[txt] = true
		}
		if len(seen) < 38 {
			t.Fatalf("%s: %d different families of 40 seeds", st.Name, len(seen))
		}
		if a, b := Family(st, 2, 3, nil), Family(st, 2, 3, nil); a[5].Pattern.Text() != b[5].Pattern.Text() {
			t.Fatal("the same seed must give the same family")
		}
	}
}

func TestRandomModeHasNothingInCommon(t *testing.T) {
	for _, st := range Styles() {
		seen := map[string]bool{}
		for _, r := range Random(st, 2, 11, nil) {
			seen[r.Pattern.Text()] = true
		}
		if len(seen) < 14 {
			t.Fatalf("%s: only %d different patterns of 16", st.Name, len(seen))
		}
	}
}

func TestLayersAddsAndDriftWanders(t *testing.T) {
	st := Styles()[1]
	ls := Layers(st, 2, 5, nil)
	if ls[9].Pattern.Notes() <= ls[0].Pattern.Notes() {
		t.Fatalf("layers: %d notes at the end of the layering, %d in the groove", ls[9].Pattern.Notes(), ls[0].Pattern.Notes())
	}
	ds := Drift(st, 2, 5, nil)
	diff := 0
	for i := 1; i < len(ds); i++ {
		if ds[i].Pattern.Text() != ds[i-1].Pattern.Text() {
			diff++
		}
	}
	if diff < 12 {
		t.Fatalf("drift: only %d of 15 steps change anything", diff)
	}
	if ds[15].Pattern.Text() == ds[0].Pattern.Text() {
		t.Fatal("drift ends where it began")
	}
}

// a kit with only a kick, a snare and a clap still gives sixteen patterns with notes in them (a break has no kick)
func TestFamilyOfASmallKitHasNoEmptyPattern(t *testing.T) {
	has := func(l formats.Lane) bool {
		return l == formats.LaneKick || l == formats.LaneSnare || l == formats.LaneClap
	}
	for _, st := range Styles() {
		for seed := int64(0); seed < 60; seed++ {
			for i, r := range Family(st, 2, seed, has) {
				heard := false
				for _, row := range r.Pattern.Rows {
					for _, v := range row.Steps {
						heard = heard || v > 0 && has(row.Lane)
					}
				}
				if !heard {
					t.Fatalf("%s seed %d: pattern %d (%s) has no note the kit plays", st.Name, seed, i+1, r.Section.Name)
				}
			}
		}
	}
}

// a second pad takes the hits off the beat of the kick, the ghosts of the snare and every other hat: both pads sound, and no hit is lost
func TestSecondPadsShareTheHits(t *testing.T) {
	st, _ := StyleByName("tech house")
	p := Base(st, 2, 1.0)
	pads := map[formats.Lane]proto.PadIndex{formats.LaneKick: pad(0), formats.LaneSnare: pad(1), formats.LaneClosedHat: pad(2)}
	alts := map[formats.Lane]proto.PadIndex{formats.LaneClosedHat: pad(3), formats.LaneKick: pad(4)}
	plain, err := p.Bytes(pads)
	if err != nil {
		t.Fatal(err)
	}
	both, err := p.BytesAlt(pads, alts)
	if err != nil {
		t.Fatal(err)
	}
	count := func(b []byte, n int) int {
		pt, err := formats.ParsePattern(b)
		if err != nil {
			t.Fatal(err)
		}
		c := 0
		for _, e := range pt.Events {
			if !e.Control && int(e.Pad.Index()) == n {
				c++
			}
		}
		return c
	}
	if count(both, 3) == 0 || count(both, 2) == 0 {
		t.Fatalf("both hats sound: %d and %d", count(both, 2), count(both, 3))
	}
	if count(both, 2)+count(both, 3) != count(plain, 2) {
		t.Fatalf("the hats share the hits of one: %d + %d vs %d", count(both, 2), count(both, 3), count(plain, 2))
	}
	if count(both, 0)+count(both, 4) != count(plain, 0) {
		t.Fatal("the kicks share the hits of one")
	}
}

func pad(i int) proto.PadIndex { p, _ := proto.NewPadIndex(uint16(i)); return p }
