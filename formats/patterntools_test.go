package formats_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// toolPattern builds a 2-bar pattern (3840 ticks) with notes at the given ticks, all on pad A1
// except where a different pad is wanted. It is the input the pattern tools work on.
func toolPattern(t *testing.T, ticks ...uint32) []byte {
	t.Helper()
	a1 := padOf(t, "A1")
	// Start from a bare pattern: one note at tick 0 that the callers' first tick usually
	// replaces, then insert the rest.
	b, err := formats.StartPattern([]proto.PadIndex{a1}, 2)
	if err != nil {
		t.Fatal(err)
	}
	// StartPattern leaves A1 held for the whole pattern at tick 0; drop it so only the asked-for
	// notes are present.
	notes, _ := formats.NoteRecords(b)
	if err := formats.RemoveNote(b, notes[0].Offset); err != nil {
		t.Fatal(err)
	}
	for _, tk := range ticks {
		b, err = formats.InsertNote(b, tk, a1, 100, 120)
		if err != nil {
			t.Fatalf("insert at %d: %v", tk, err)
		}
	}
	return b
}

// tickSet is the sorted set of ticks the notes of b sit on.
func tickSet(t *testing.T, b []byte) []uint32 {
	t.Helper()
	notes, err := formats.NoteRecords(b)
	if err != nil {
		t.Fatal(err)
	}
	var out []uint32
	for _, n := range notes {
		out = append(out, n.Tick)
	}
	return out
}

// checkWholePattern reparses b and fails if it is malformed, the length changed, the note count
// changed or the trailer changed from before.
func checkWholePattern(t *testing.T, label string, before, after []byte) {
	t.Helper()
	p0, err := formats.ParsePattern(before)
	if err != nil {
		t.Fatalf("%s: the input does not parse: %v", label, err)
	}
	p1, err := formats.ParsePattern(after)
	if err != nil {
		t.Fatalf("%s: the result does not parse: %v", label, err)
	}
	if p1.Length != p0.Length {
		t.Errorf("%s: length changed %d -> %d", label, p0.Length, p1.Length)
	}
	n0, _ := formats.NoteRecords(before)
	n1, _ := formats.NoteRecords(after)
	if len(n1) != len(n0) {
		t.Errorf("%s: note count changed %d -> %d", label, len(n0), len(n1))
	}
	if !bytes.Equal(p1.Trailer, p0.Trailer) {
		t.Errorf("%s: the trailer changed % x -> % x", label, p0.Trailer, p1.Trailer)
	}
}

const sixteenth = formats.PPQ / 4 // 120 ticks

func TestQuantizeSnapsNotesToTheGrid(t *testing.T) {
	// Notes a little off the 16th grid (120 ticks). 118 -> 120, 125 -> 120, 605 -> 600.
	b := toolPattern(t, 118, 125, 605, 1200)
	out, err := formats.Quantize(b, sixteenth, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := tickSet(t, out)
	want := []uint32{120, 120, 600, 1200}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note %d: got %d want %d", i, got[i], want[i])
		}
	}
	checkWholePattern(t, "quantize full", b, out)

	// Half strength moves a note halfway to the grid: 100 is 20 off 120, lands at 110.
	half, err := formats.Quantize(toolPattern(t, 100), sixteenth, 0.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ticks := tickSet(t, half); len(ticks) != 1 || ticks[0] != 110 {
		t.Errorf("half strength landed on %v, want [110]", ticks)
	}

	if _, err := formats.Quantize(b, 0, 1, nil); err == nil {
		t.Error("a zero grid was accepted")
	}
}

func TestSwingPushesTheOffBeats(t *testing.T) {
	// On the 16th grid: steps 0,1,2,3 at 0,120,240,360. At 66% an off-beat is pushed by
	// (66-50)/100 * 2*120 = 38.4 -> 38 ticks. Even steps stay.
	b := toolPattern(t, 0, 120, 240, 360)
	out, err := formats.Swing(b, sixteenth, 66, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := tickSet(t, out)
	want := []uint32{0, 158, 240, 398} // 120+38, 360+38
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d: got %d want %d (all %v)", i, got[i], want[i], got)
		}
	}
	checkWholePattern(t, "swing", b, out)

	// 50% is straight: nothing moves.
	straight, _ := formats.Swing(b, sixteenth, 50, nil)
	if got := tickSet(t, straight); got[1] != 120 || got[3] != 360 {
		t.Errorf("50%% swing moved notes: %v", got)
	}
	// Over 75% is clamped to 75%: off-beat pushed by (75-50)/100*240 = 60.
	hard, _ := formats.Swing(toolPattern(t, 120), sixteenth, 90, nil)
	if got := tickSet(t, hard); got[0] != 180 {
		t.Errorf("clamped swing landed on %v, want [180]", got)
	}
}

func TestSwingScopedToOnePadLeavesTheRestStraight(t *testing.T) {
	a1, a2 := padOf(t, "A1"), padOf(t, "A2")
	// A1 and A2 both on the off-beat 16th at 120; swing only A2's track.
	b, err := formats.StartPattern([]proto.PadIndex{a1}, 2)
	if err != nil {
		t.Fatal(err)
	}
	notes, _ := formats.NoteRecords(b)
	if err := formats.RemoveNote(b, notes[0].Offset); err != nil {
		t.Fatal(err)
	}
	b, _ = formats.InsertNote(b, 120, a1, 100, 120)
	b, _ = formats.InsertNote(b, 120, a2, 100, 120)
	out, err := formats.Swing(b, sixteenth, 66, formats.OnlyPad(a2))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := formats.NoteRecords(out)
	for _, n := range after {
		switch n.Pad {
		case a1:
			if n.Tick != 120 {
				t.Errorf("A1 was swung to %d; only A2 should move", n.Tick)
			}
		case a2:
			if n.Tick != 158 {
				t.Errorf("A2 landed on %d, want 158", n.Tick)
			}
		}
	}
	checkWholePattern(t, "swing one pad", b, out)
}

func TestShiftMovesEveryNoteByWholeSteps(t *testing.T) {
	b := toolPattern(t, 0, 240, 1200)
	out, err := formats.Shift(b, sixteenth, 2, nil) // +240 ticks
	if err != nil {
		t.Fatal(err)
	}
	if got := tickSet(t, out); got[0] != 240 || got[1] != 480 || got[2] != 1440 {
		t.Errorf("shift +2 gave %v", got)
	}
	checkWholePattern(t, "shift", b, out)

	// Shifting earlier past the start clamps at 0.
	back, _ := formats.Shift(b, sixteenth, -3, nil) // -360
	if got := tickSet(t, back); got[0] != 0 {
		t.Errorf("shift -3 past the start gave %v, want it clamped to 0", got)
	}
}

func TestReverseMirrorsThePattern(t *testing.T) {
	// Length is 3840. A note at 0 goes to 3840 (the end), 480 -> 3360, 1200 -> 2640.
	b := toolPattern(t, 0, 480, 1200)
	out, err := formats.Reverse(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := tickSet(t, out)
	want := []uint32{2640, 3360, 3840}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note %d: got %d want %d (all %v)", i, got[i], want[i], got)
		}
	}
	checkWholePattern(t, "reverse", b, out)
	// Reversing twice is the pattern again (in the ticks the notes sit on).
	twice, _ := formats.Reverse(out, nil)
	if a, bk := tickSet(t, b), tickSet(t, twice); len(a) != len(bk) || a[0] != bk[0] || a[1] != bk[1] || a[2] != bk[2] {
		t.Errorf("reverse twice changed the ticks: %v -> %v", a, bk)
	}
}

func TestHumanizeStaysWithinBoundsAndTightensKicks(t *testing.T) {
	a1 := padOf(t, "A1")
	// A pad named like a kick is tightened; A1 here stands for it via the tighten predicate.
	tighten := func(p proto.PadIndex) bool { return p == a1 }
	b := toolPattern(t, 480, 960, 1440, 1920)
	before, _ := formats.NoteRecords(b)
	rng := rand.New(rand.NewSource(1))
	out, err := formats.Humanize(b, 40, 20, tighten, nil, rng)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := formats.NoteRecords(out)
	if len(after) != len(before) {
		t.Fatalf("note count changed %d -> %d", len(before), len(after))
	}
	for i := range after {
		// Kicks are tightened to half the timing spread (20).
		dt := int64(after[i].Tick) - int64(before[i].Tick)
		if dt < -20 || dt > 20 {
			t.Errorf("note %d moved %d ticks, past the tightened +-20", i, dt)
		}
		dv := int(after[i].Velocity) - int(before[i].Velocity)
		if dv < -20 || dv > 20 {
			t.Errorf("note %d velocity moved %d, past +-20", i, dv)
		}
		if after[i].Velocity < 1 || after[i].Velocity > 127 {
			t.Errorf("note %d velocity %d out of range", i, after[i].Velocity)
		}
	}
	checkWholePattern(t, "humanize", b, out)

	// Same seed, same result: repeatable.
	again, _ := formats.Humanize(b, 40, 20, tighten, nil, rand.New(rand.NewSource(1)))
	if !bytes.Equal(again, out) {
		t.Error("humanize with the same seed gave a different pattern")
	}
}

func TestMoveNoteNudgesOneNoteByTicks(t *testing.T) {
	b := toolPattern(t, 0, 240, 480)
	a1 := padOf(t, "A1")
	// 24 ticks later: only the note at 240 moves; its velocity and length stay.
	after, err := formats.MoveNote(bytes.Clone(b), a1, 240, 24)
	if err != nil {
		t.Fatal(err)
	}
	checkWholePattern(t, "move later", b, after)
	if got := tickSet(t, after); len(got) != 3 || got[0] != 0 || got[1] != 264 || got[2] != 480 {
		t.Errorf("ticks after moving 240 by +24 = %v, want [0 264 480]", got)
	}
	notes, _ := formats.NoteRecords(after)
	if n := notes[1]; n.Velocity != 100 || n.Length != 120 {
		t.Errorf("the moved note changed: velocity %d, length %d", n.Velocity, n.Length)
	}
	// Earlier past the start stops at 0; later past the end stops on the last tick.
	if early, _ := formats.MoveNote(bytes.Clone(b), a1, 0, -50); tickSet(t, early)[0] != 0 {
		t.Errorf("a note moved before the start: %v", tickSet(t, early))
	}
	late, _ := formats.MoveNote(bytes.Clone(b), a1, 480, 10000)
	if got := tickSet(t, late); got[len(got)-1] != 3839 {
		t.Errorf("a note moved past the end: %v", got)
	}
	// No note there, nothing moved.
	if _, err := formats.MoveNote(bytes.Clone(b), a1, 100, 5); err == nil {
		t.Error("moving a note that is not there succeeded")
	}
}

func TestDoublePatternPlaysItAgainAfterItself(t *testing.T) {
	b := toolPattern(t, 0, 600, 1920) // 2 bars
	after, err := formats.DoublePattern(bytes.Clone(b))
	if err != nil {
		t.Fatal(err)
	}
	p, err := formats.ParsePattern(after)
	if err != nil {
		t.Fatalf("the doubled pattern does not parse: %v", err)
	}
	if p.Length != 7680 || p.Trailer[0] != 4 {
		t.Errorf("doubled: length %d, %d bars in the trailer; want 7680 and 4", p.Length, p.Trailer[0])
	}
	want := []uint32{0, 600, 1920, 3840, 3840 + 600, 3840 + 1920}
	if got := tickSet(t, after); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("doubled ticks %v, want %v", got, want)
	}
	notes, _ := formats.NoteRecords(after)
	if n := notes[4]; n.Velocity != 100 || n.Length != 120 {
		t.Errorf("a copied note changed: velocity %d, length %d", n.Velocity, n.Length)
	}
}

func TestHalvePatternKeepsTheFirstHalf(t *testing.T) {
	b := toolPattern(t, 0, 600, 1920, 3000) // 2 bars
	after, err := formats.HalvePattern(bytes.Clone(b))
	if err != nil {
		t.Fatal(err)
	}
	p, err := formats.ParsePattern(after)
	if err != nil {
		t.Fatalf("the halved pattern does not parse: %v", err)
	}
	if p.Length != 1920 || p.Trailer[0] != 1 {
		t.Errorf("halved: length %d, %d bars in the trailer; want 1920 and 1", p.Length, p.Trailer[0])
	}
	if got := tickSet(t, after); fmt.Sprint(got) != "[0 600]" {
		t.Errorf("halved ticks %v, want [0 600]", got)
	}
	// One bar is as short as it gets; doubling back gives the bar twice.
	if _, err := formats.HalvePattern(after); err == nil {
		t.Error("a 1-bar pattern was halved")
	}
	again, _ := formats.DoublePattern(after)
	if got := tickSet(t, again); fmt.Sprint(got) != "[0 600 1920 2520]" {
		t.Errorf("halved then doubled ticks %v", got)
	}
	// Three bars cannot be halved into whole bars.
	three, err := formats.BlankPattern(3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formats.HalvePattern(three); err == nil {
		t.Error("a 3-bar pattern was halved")
	}
}

func TestEditNotesChangesOnlyTheChosenNotes(t *testing.T) {
	pad := proto.PadIndex{}
	other, _ := proto.PadFromBankPad(0, 1)
	b, err := formats.BlankPattern(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []uint32{0, 480, 960, 1440} {
		if b, err = formats.InsertNote(b, at, pad, 100, 120); err != nil {
			t.Fatal(err)
		}
	}
	if b, err = formats.InsertNote(b, 480, other, 100, 120); err != nil {
		t.Fatal(err)
	}
	chosen := map[uint32]bool{480: true, 1440: true}
	edited, n, err := formats.EditNotes(b, pad, chosen, formats.NoteEdit{Velocity: -30, Length: 60, Move: 10})
	if err != nil || n != 2 {
		t.Fatalf("%d changed, %v", n, err)
	}
	notes, _ := formats.NoteRecords(edited)
	got := map[string][]formats.NoteRecord{}
	for _, x := range notes {
		got[x.Pad.String()] = append(got[x.Pad.String()], x)
	}
	for _, x := range got[pad.String()] {
		moved := x.Tick == 490 || x.Tick == 1450
		if moved && (x.Velocity != 70 || x.Length != 180) {
			t.Errorf("chosen note at %d: velocity %d length %d", x.Tick, x.Velocity, x.Length)
		}
		if !moved && (x.Velocity != 100 || x.Length != 120) {
			t.Errorf("note at %d changed: velocity %d length %d", x.Tick, x.Velocity, x.Length)
		}
	}
	if len(got[pad.String()]) != 4 || len(got[other.String()]) != 1 || got[other.String()][0].Tick != 480 {
		t.Errorf("notes %+v", got)
	}
	removed, n, err := formats.EditNotes(b, pad, chosen, formats.NoteEdit{Delete: true})
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if notes, _ := formats.NoteRecords(removed); len(notes) != 3 {
		t.Errorf("%d notes left, want 3", len(notes))
	}
	snapped, _, err := formats.EditNotes(b, pad, map[uint32]bool{480: true}, formats.NoteEdit{Move: 40})
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := formats.EditNotes(snapped, pad, map[uint32]bool{520: true}, formats.NoteEdit{Snap: 120})
	if err != nil {
		t.Fatal(err)
	}
	if notes, _ := formats.NoteRecords(again); !slicesContainsFunc(notes, func(x formats.NoteRecord) bool { return x.Pad == pad && x.Tick == 480 }) {
		t.Errorf("snap did not bring the note back to 480: %+v", notes)
	}
}

func slicesContainsFunc[S ~[]E, E any](s S, f func(E) bool) bool {
	for _, e := range s {
		if f(e) {
			return true
		}
	}
	return false
}

func TestExtendPatternDoublesTheLengthAndLeavesTheNewHalfEmpty(t *testing.T) {
	b := toolPattern(t, 0, 600, 1920) // 2 bars
	after, err := formats.ExtendPattern(bytes.Clone(b))
	if err != nil {
		t.Fatal(err)
	}
	p, err := formats.ParsePattern(after)
	if err != nil {
		t.Fatalf("the extended pattern does not parse: %v", err)
	}
	if p.Length != 7680 || p.Trailer[0] != 4 {
		t.Errorf("extended: length %d, %d bars in the trailer; want 7680 and 4", p.Length, p.Trailer[0])
	}
	want := []uint32{0, 600, 1920}
	if got := tickSet(t, after); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("extended ticks %v, want %v: nothing may play in the new half", got, want)
	}
	// Twice more is 8 bars; past the most a pattern can be it is refused.
	long := toolPattern(t, 0)
	for i := 0; i < 8; i++ {
		next, err := formats.ExtendPattern(long)
		if err != nil {
			if i < 3 {
				t.Fatalf("extending step %d: %v", i, err)
			}
			return
		}
		long = next
	}
	t.Error("a pattern was extended past the longest there is")
}
