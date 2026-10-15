package app_test

import (
	"slices"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func twoBarPattern(t *testing.T) formats.Pattern {
	t.Helper()
	kick, _ := proto.ParsePad("A1")
	snare, _ := proto.ParsePad("A2")
	p := formats.Pattern{Length: 2 * 4 * formats.PPQ}
	for _, tick := range []uint32{0, 480, 960, 1440} {
		p.Events = append(p.Events, formats.Event{Tick: tick, Pad: kick, Velocity: 100, Length: 100})
	}
	p.Events = append(p.Events, formats.Event{Tick: 240, Pad: snare, Velocity: 90, Length: 100})
	return p
}

func ticks(p formats.Pattern, pad string) []uint32 {
	want, _ := proto.ParsePad(pad)
	var out []uint32
	for _, e := range p.Events {
		if e.Pad == want {
			out = append(out, e.Tick)
		}
	}
	return out
}

func TestCondPlaysOnItsLoopOfTheCycle(t *testing.T) {
	c := app.Cond{A: 2, B: 3}
	for loop, want := range []bool{false, true, false, false, true, false} {
		if got := c.Plays(loop, 0); got != want {
			t.Errorf("2:3 on loop %d: %v, want %v", loop, got, want)
		}
	}
	if !(app.Cond{}).Plays(7, 99) || !(app.Cond{}).Plain() {
		t.Error("no condition is not every loop")
	}
	half := app.Cond{Prob: 50}
	if !half.Plays(0, 49) || half.Plays(0, 50) {
		t.Error("a chance of 50 plays under a roll of 50")
	}
	if got := (app.Cond{A: 1, B: 2, Prob: 50}).String(); got != "1:2 · 50%" {
		t.Errorf("%q", got)
	}
}

func TestCondCatalogStepsThroughAndRoundAgain(t *testing.T) {
	cat := app.CondCatalog()
	if len(cat) != 1+2+3+4+8+5 || !cat[0].Plain() {
		t.Fatalf("%d conditions, the first %+v", len(cat), cat[0])
	}
	c := app.Cond{}
	for i := 0; i < len(cat); i++ {
		c = app.StepCond(c, 1)
	}
	if !c.Plain() {
		t.Errorf("a whole round ends on %+v", c)
	}
	if got := app.StepCond(app.Cond{}, -1); got.Prob != 10 {
		t.Errorf("back from always is %+v", got)
	}
	if got := app.StepCond(app.Cond{}, 1); got != (app.Cond{A: 1, B: 2}) {
		t.Errorf("on from always is %+v", got)
	}
}

func TestCycleIsTheLeastCommonMultiple(t *testing.T) {
	kick, _ := proto.ParsePad("A1")
	snare, _ := proto.ParsePad("A2")
	cs := app.Conditions{}
	if cs.CycleLoops() != 1 {
		t.Error("no conditions are one loop")
	}
	cs[app.NoteKey{Pad: kick, Step: 0}] = app.Cond{A: 1, B: 2}
	if cs.CycleLoops() != 2 {
		t.Errorf("1:2 is %d loops", cs.CycleLoops())
	}
	cs[app.NoteKey{Pad: snare, Step: 2}] = app.Cond{A: 2, B: 3}
	if cs.CycleLoops() != 6 {
		t.Errorf("1:2 with 2:3 is %d loops", cs.CycleLoops())
	}
	cs[app.NoteKey{Pad: snare, Step: 4}] = app.Cond{A: 1, B: 4}
	if cs.CycleLoops() != 12 {
		t.Errorf("with 1:4 too: %d loops", cs.CycleLoops())
	}
	p := twoBarPattern(t)
	// Two bars of 4/4 at 120 BPM are 4 s; 12 loops of them are 48 s and 24 bars.
	loops, bars, seconds := app.CycleInfo(p, cs, 120, 4)
	if loops != 12 || bars != 24 || seconds < 47.99 || seconds > 48.01 {
		t.Errorf("cycle: %d loops, %d bars, %.2f s", loops, bars, seconds)
	}
	// Chances roll over at least the loops asked for.
	chance := app.Conditions{app.NoteKey{Pad: kick, Step: 0}: {Prob: 50}}
	if loops, _, _ := app.CycleInfo(p, chance, 120, 4); loops != 4 {
		t.Errorf("a chance rolls over %d loops, want 4", loops)
	}
	if loops, _, _ := app.CycleInfo(p, app.Conditions{}, 120, 4); loops != 1 {
		t.Errorf("plain: %d loops", loops)
	}
}

func TestUnrollPutsEachNoteOnItsLoops(t *testing.T) {
	p := twoBarPattern(t)
	kick, _ := proto.ParsePad("A1")
	// Without conditions one loop is the pattern itself.
	if got := app.Unroll(p, nil, 1, 1); !slices.Equal(ticks(got, "A1"), ticks(p, "A1")) || got.Length != p.Length {
		t.Errorf("one plain loop: %v of %d", ticks(got, "A1"), got.Length)
	}
	// The kick on beat 1 plays on the first of every 2 loops, the snare always: 4 loops.
	cs := app.Conditions{app.NoteKey{Pad: kick, Step: 0}: {A: 1, B: 2}}
	got := app.Unroll(p, cs, 4, 1)
	if got.Length != 4*p.Length {
		t.Errorf("length %d, want %d", got.Length, 4*p.Length)
	}
	want := []uint32{0, 480, 960, 1440, // loop 1: all four
		p.Length + 480, p.Length + 960, p.Length + 1440, // loop 2: not the first
		2 * p.Length, 2*p.Length + 480, 2*p.Length + 960, 2*p.Length + 1440,
		3*p.Length + 480, 3*p.Length + 960, 3*p.Length + 1440}
	if !slices.Equal(ticks(got, "A1"), want) {
		t.Errorf("kick at %v, want %v", ticks(got, "A1"), want)
	}
	if n := len(ticks(got, "A2")); n != 4 {
		t.Errorf("the snare plays %d times in 4 loops", n)
	}
}

func TestUnrollChancesComeFromTheSeed(t *testing.T) {
	p := twoBarPattern(t)
	kick, _ := proto.ParsePad("A1")
	cs := app.Conditions{}
	for _, step := range []int{0, 4, 8, 12} {
		cs[app.NoteKey{Pad: kick, Step: step}] = app.Cond{Prob: 50}
	}
	a, b := app.Unroll(p, cs, 16, 7), app.Unroll(p, cs, 16, 7)
	if !slices.Equal(ticks(a, "A1"), ticks(b, "A1")) {
		t.Error("one seed gave two different rolls")
	}
	if slices.Equal(ticks(a, "A1"), ticks(app.Unroll(p, cs, 16, 8), "A1")) {
		t.Error("two seeds gave the same roll")
	}
	// 4 kicks over 16 loops at 50%: about 32 of 64 play.
	if n := len(ticks(a, "A1")); n < 16 || n > 48 {
		t.Errorf("%d of 64 kicks play at 50%%", n)
	}
	// A chance on one note does not change another's dice: the snare, plain, always plays.
	if n := len(ticks(a, "A2")); n != 16 {
		t.Errorf("the plain snare plays %d times in 16 loops", n)
	}
}

func TestPrunedKeepsOnlyConditionsOfNotesThereAre(t *testing.T) {
	p := twoBarPattern(t)
	kick, _ := proto.ParsePad("A1")
	cs := app.Conditions{
		app.NoteKey{Pad: kick, Step: 0}:  {A: 1, B: 2},
		app.NoteKey{Pad: kick, Step: 99}: {A: 1, B: 2}, // no note there
		app.NoteKey{Pad: kick, Step: 4}:  {},           // plain
	}
	if got := cs.Pruned(p); len(got) != 1 {
		t.Errorf("pruned: %v", got)
	}
}
