package generate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func rowOf(p Pattern, l formats.Lane) *Row {
	if r := p.Row(l); r != nil {
		return r
	}
	return &Row{Lane: l}
}

// a lane that is locked plays what it played in the roll before, whatever the new seed makes of it; the others are rolled again
func TestOneKeepsTheLockedLanes(t *testing.T) {
	keep := func(l formats.Lane) bool { return GroupOf(l) == 0 || GroupOf(l) == 2 } // the kick and the hats
	for _, mode := range []string{"single", "fill", "roll"} {
		for _, st := range Styles() {
			_, prev := One(mode, st, 2, 11, 12, nil, nil, nil)
			changed := false
			for seed := int64(100); seed < 110; seed++ {
				r, groove := One(mode, st, 2, seed, seed+1, nil, &prev, keep)
				free, _ := One(mode, st, 2, seed, seed+1, nil, nil, nil)
				for _, l := range []formats.Lane{formats.LaneKick, formats.LaneClosedHat, formats.LaneOpenHat} {
					if !reflect.DeepEqual(rowOf(groove, l).Steps, rowOf(prev, l).Steps) {
						t.Fatalf("%s %s seed %d: the locked %s changed in the groove", st.Name, mode, seed, l.LaneName())
					}
				}
				if mode == "single" && !reflect.DeepEqual(rowOf(r.Pattern, formats.LaneKick).Steps, rowOf(prev, formats.LaneKick).Steps) {
					t.Fatalf("%s seed %d: the locked kick changed", st.Name, seed)
				}
				if r.Pattern.Notes() == 0 {
					t.Fatalf("%s %s seed %d: an empty pattern", st.Name, mode, seed)
				}
				if _, err := r.Pattern.Bytes(allPads()); err != nil {
					t.Fatalf("%s %s seed %d: %v", st.Name, mode, seed, err)
				}
				for _, row := range free.Pattern.Rows { // a lane that is not locked is what the seed makes of it
					if !keep(row.Lane) && mode == "single" && !reflect.DeepEqual(rowOf(r.Pattern, row.Lane).Steps, row.Steps) {
						t.Fatalf("%s seed %d: the free %s is not what the seed makes", st.Name, seed, row.Lane.LaneName())
					}
				}
				if r.Pattern.Text() != prev.Text() {
					changed = true
				}
			}
			if !changed {
				t.Errorf("%s %s: ten rolls with the kick and the hats locked all give the pattern before", st.Name, mode)
			}
		}
	}
}

// with every lane kept the groove is the one before: only the fill is rolled again
func TestOneRollsOnlyTheFill(t *testing.T) {
	st, _ := StyleByName("house")
	all := func(formats.Lane) bool { return true }
	_, prev := One("fill", st, 2, 5, 6, nil, nil, nil)
	kinds := map[string]bool{}
	for fs := int64(0); fs < 30; fs++ {
		r, groove := One("fill", st, 2, 999, fs, nil, &prev, all)
		if groove.Text() != prev.Text() {
			t.Fatalf("the groove changed:\n%s\n%s", prev.Text(), groove.Text())
		}
		for _, row := range r.Pattern.Rows { // the first bar has no fill: it is the groove's
			if o := prev.Row(row.Lane); o != nil && !reflect.DeepEqual(row.Steps[:16], o.Steps[:16]) {
				t.Fatalf("bar 1 of %s changed", row.Lane.LaneName())
			}
		}
		kinds[r.Section.Name] = true
	}
	if len(kinds) < 3 {
		t.Fatalf("thirty rolls of the fill give %d kinds", len(kinds))
	}
}

func allPads() map[formats.Lane]proto.PadIndex {
	m := map[formats.Lane]proto.PadIndex{}
	for i, l := range []formats.Lane{formats.LaneKick, formats.LaneSnare, formats.LaneClosedHat, formats.LaneOpenHat, formats.LaneClap, formats.LanePerc,
		formats.LaneRide, formats.LaneShaker, formats.LaneRim, formats.LaneTom} {
		m[l] = pad(i)
	}
	return m
}

// every variation changes the pattern, leaves its length, and says what it did; the source is not touched
func TestVaryChangesACopy(t *testing.T) {
	for _, st := range Styles() {
		src := Make(st, Section{Name: "Pattern", Kind: "groove", Energy: 0.6, Bars: 2}, 3, nil).Pattern
		before := src.Text()
		for _, kind := range Variations {
			seen := map[string]bool{}
			for seed := int64(0); seed < 12; seed++ {
				p, what, err := Vary(src, kind, seed, nil)
				if err != nil {
					t.Fatalf("%s %s seed %d: %v", st.Name, kind, seed, err)
				}
				if p.Text() == before || what == "" || p.Bars != src.Bars {
					t.Fatalf("%s %s seed %d: nothing changed (%q)", st.Name, kind, seed, what)
				}
				if src.Text() != before {
					t.Fatalf("%s %s: the source was changed", st.Name, kind)
				}
				if p.Notes() == 0 {
					t.Fatalf("%s %s seed %d: an empty pattern", st.Name, kind, seed)
				}
				seen[p.Text()] = true
			}
			if kind != "break" && len(seen) < 2 {
				t.Errorf("%s %s: twelve rolls give one result", st.Name, kind)
			}
		}
	}
}

// a fill and a roll leave everything before the second half of the last bar as it was
func TestVaryFillStaysInTheLastBar(t *testing.T) {
	st, _ := StyleByName("techno")
	src := Make(st, Section{Name: "Pattern", Kind: "groove", Energy: 0.6, Bars: 2}, 3, nil).Pattern
	for _, kind := range []string{"fill", "roll"} {
		for seed := int64(0); seed < 20; seed++ {
			p, what, err := Vary(src, kind, seed, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range p.Rows {
				if o := src.Row(r.Lane); o != nil && !reflect.DeepEqual(r.Steps[:16], o.Steps[:16]) {
					t.Fatalf("%s (%s): bar 1 of %s changed", kind, what, r.Lane.LaneName())
				}
			}
		}
	}
}

func TestVaryThinnerAndBusier(t *testing.T) {
	st, _ := StyleByName("house")
	src := Make(st, Section{Name: "Pattern", Kind: "groove", Energy: 0.7, Bars: 2}, 4, nil).Pattern
	for seed := int64(0); seed < 10; seed++ {
		thin, _, err := Vary(src, "thinner", seed, nil)
		if err != nil {
			t.Fatal(err)
		}
		if thin.Notes() >= src.Notes() {
			t.Fatalf("thinner has %d notes, the pattern %d", thin.Notes(), src.Notes())
		}
		if !reflect.DeepEqual(rowOf(thin, formats.LaneKick).Steps, rowOf(src, formats.LaneKick).Steps) {
			t.Fatal("thinner changed the kick")
		}
		busy, _, err := Vary(src, "busier", seed, nil)
		if err != nil {
			t.Fatal(err)
		}
		if busy.Notes() <= src.Notes() {
			t.Fatalf("busier has %d notes, the pattern %d", busy.Notes(), src.Notes())
		}
		for _, r := range src.Rows { // busier only adds: every hit of the pattern is still there
			for i, v := range r.Steps {
				if v > 0 && rowOf(busy, r.Lane).Steps[i] != v {
					t.Fatalf("busier changed a hit of the %s at step %d", r.Lane.LaneName(), i)
				}
			}
		}
	}
	// a pattern of a kick alone: nothing to take away, and no break of it
	kick := Pattern{Bars: 1}
	kick.ensure(formats.LaneKick).Steps[0] = 120
	if _, _, err := Vary(kick, "thinner", 1, nil); err == nil {
		t.Fatal("thinner of a kick alone is an error")
	}
	if _, _, err := Vary(kick, "break", 1, nil); err == nil {
		t.Fatal("a break of a kick alone is an error")
	}
	if _, _, err := Vary(kick, "nonsense", 1, nil); err == nil {
		t.Fatal("an unknown variation is an error")
	}
}

// a generated pattern, written to notes and read back, is the same steps; the swing, the nudge and the length of the notes are found again
func TestReadFindsTheStepsAndTheFeel(t *testing.T) {
	for _, st := range Styles() {
		for _, mode := range []string{"single", "fill", "roll"} {
			for seed := int64(1); seed < 6; seed++ {
				r, _ := One(mode, st, 2, seed, seed+50, nil, nil, nil)
				src := r.Pattern
				pads := allPads()
				laneOf := map[proto.PadIndex]formats.Lane{}
				for l, p := range pads {
					laneOf[p] = l
				}
				b, err := src.Bytes(pads)
				if err != nil {
					t.Fatal(err)
				}
				parsed, err := formats.ParsePattern(b)
				if err != nil {
					t.Fatal(err)
				}
				var hits []Hit
				for _, e := range parsed.Events {
					if !e.Control {
						hits = append(hits, Hit{Lane: laneOf[e.Pad], Tick: e.Tick, Vel: int(e.Velocity), Len: int(e.Length)})
					}
				}
				got := Read(2, hits)
				for _, row := range src.Rows {
					if row.hits() == 0 {
						continue
					}
					g := got.Row(row.Lane)
					if g == nil {
						t.Fatalf("%s %s seed %d: no %s", st.Name, mode, seed, row.Lane.LaneName())
					}
					for i, v := range row.Steps {
						// a row with a roll: the last hit of a swung triplet is nearer the next step, so only which steps play is compared
						if rolled := row.Ratchet != nil || row.Flam != nil; (!rolled && g.Steps[i] != v) || (rolled && (g.Steps[i] > 0) != (v > 0)) {
							t.Fatalf("%s %s seed %d, %s step %d:\n%v\n%v", st.Name, mode, seed, row.Lane.LaneName(), i, row.Steps, g.Steps)
						}
					}
					odd := false
					for i, v := range row.Steps {
						odd = odd || (v > 0 && i%2 == 1)
					}
					want := row.Swing
					if want <= 50 {
						want = 0
					}
					if odd && row.Ratchet == nil && row.Flam == nil && (g.Swing < want-1 || g.Swing > want+1) {
						t.Errorf("%s %s seed %d, %s: swing %d read as %d", st.Name, mode, seed, row.Lane.LaneName(), row.Swing, g.Swing)
					}
				}
			}
		}
	}
}

// Diff names the steps a change touched, and gives only those to the builder
func TestDiffIsOnlyWhatChanged(t *testing.T) {
	st, _ := StyleByName("house")
	src := Make(st, Section{Name: "Pattern", Kind: "groove", Energy: 0.6, Bars: 2}, 3, nil).Pattern
	if ch, add := Diff(src, src.Clone()); len(ch) != 0 || len(add) != 0 {
		t.Fatalf("a copy differs: %v", ch)
	}
	for _, kind := range Variations {
		p, _, err := Vary(src, kind, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		ch, add := Diff(src, p)
		if len(ch) == 0 {
			t.Fatalf("%s: nothing changed", kind)
		}
		for _, r := range p.Rows {
			for i, v := range r.Steps {
				var was uint8
				if o := src.Row(r.Lane); o != nil {
					was = o.Steps[i]
				}
				if (v != was) && (ch[r.Lane] == nil || !ch[r.Lane][i]) {
					t.Fatalf("%s: step %d of %s changed and is not named", kind, i, r.Lane.LaneName())
				}
			}
		}
		for _, sl := range add {
			for i, v := range sl.Steps {
				if v > 0 && !ch[sl.Lane][i] {
					t.Fatalf("%s: step %d of %s is added and did not change", kind, i, sl.Lane.LaneName())
				}
			}
		}
		if kind == "fill" || kind == "roll" {
			for l, c := range ch {
				for i := 0; i < 16; i++ {
					if c[i] {
						t.Fatalf("%s: bar 1 of %s is named", kind, l.LaneName())
					}
				}
			}
		}
	}
}

// four bars: the last one is the groove with a small change, unless it has a fill, a roll, the build or a stop of its own
func TestFourBarsEndWithATurnaround(t *testing.T) {
	for _, st := range Styles() {
		turned := 0
		for seed := int64(1); seed <= 20; seed++ {
			r, _ := One("single", st, 4, seed, seed+1, nil, nil, nil)
			if !ownLastBar(r) {
				t.Errorf("%s seed %d: the last bar has nothing of its own: %s", st.Name, seed, r.Notes)
			}
			if strings.Contains(r.Notes, "last bar: ") {
				turned++
			}
		}
		if turned == 0 {
			t.Errorf("%s: no pattern without a fill in 20 seeds", st.Name)
		}
		for _, mode := range []string{"arc", "family", "drift", "layers", "random"} {
			rs, err := Generate(mode, st, 4, 3, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rs {
				if r.Pattern.Bars >= 4 && !ownLastBar(r) {
					t.Errorf("%s %s %s: the last bar has nothing of its own", st.Name, mode, r.Section.Name)
				}
			}
		}
		rs, _ := Generate("arc", st, 2, 3, nil)
		for _, r := range rs {
			if strings.Contains(r.Notes, "last bar: ") {
				t.Errorf("%s: two bars get no turnaround", st.Name)
			}
		}
	}
}

// a turnaround changes only the end of the last bar
func TestTurnaroundChangesTheLastBar(t *testing.T) {
	st, _ := StyleByName("house")
	for seed := int64(1); seed <= 30; seed++ {
		p := Base(st, 4, 0.6)
		before := p.Clone()
		if kind := p.Turnaround(NewCtx(seed, nil)); kind == "" {
			t.Fatal("a full kit can always turn round")
		}
		for bar := 0; bar < 3; bar++ {
			if p.shape(bar) != before.shape(bar) {
				t.Fatalf("seed %d: bar %d changed", seed, bar+1)
			}
		}
		if p.shape(3) == before.shape(3) {
			t.Fatalf("seed %d: the last bar is the same", seed)
		}
	}
}
