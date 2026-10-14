package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

// drums is a second of silence with four decaying bursts, as a WAV: a sample with obvious
// places to cut.
func drums() []byte {
	s := formats.Sample{Channels: 1, Samples: make([]int16, 48000)}
	for _, start := range []int{0, 12000, 24000, 36000} {
		for i := 0; i < 5000; i++ {
			amp := 20000 * math.Exp(-float64(i)/1200)
			s.Samples[start+i] = int16(amp * math.Sin(float64(i)*0.7))
		}
	}
	return s.WAV()
}

// slicerOpen opens the slicer on pad A5, which holds the drums.
func slicerOpen(t *testing.T) *screen {
	t.Helper()
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 140, Height: 50})
	s.sim.AddFile("SD:IMPORT/drums.wav", drums())
	m := s.m()
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/drums.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "j", "Z")
	if s.m().slicer == nil {
		t.Fatalf("the slicer did not open:\n%s", s.model.View())
	}
	return s
}

func (s *screen) slicing() *formats.Slicing { return s.m().slicer.sl }

func TestSlicerOpensWithEightSlices(t *testing.T) {
	s := slicerOpen(t)
	s.shows("Slicer", "A5", "8 slices", "Slice 1/8", "Order", "mode order", "stopped")
	if got := s.slicing(); len(got.Slices) != 8 || got.Frames != 48000 {
		t.Fatalf("%d slices of %d frames", len(got.Slices), got.Frames)
	}
	s.press("esc")
	if s.m().slicer != nil {
		t.Error("Esc did not close the slicer")
	}
}

func TestSlicerEveryLineFitsTheScreen(t *testing.T) {
	for _, size := range [][2]int{{80, 30}, {100, 40}, {140, 50}, {220, 60}} {
		s := slicerOpen(t)
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		s.press("tab", "t", "i", "l")
		if h := len(strings.Split(s.m().slicerBox(), "\n")); h > s.m().geom().bodyH {
			t.Errorf("%dx%d: the box is %d lines, the screen has room for %d", size[0], size[1], h, s.m().geom().bodyH)
		}
		// The box itself, before the screen cuts it to size.
		for i, line := range strings.Split(s.m().slicerBox(), "\n") {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d of the box is %d wide: %q", size[0], size[1], i, w, line)
			}
		}
	}
}

func TestSlicerEachSliceHasItsOwnStartAndEnd(t *testing.T) {
	s := slicerOpen(t)
	s.press("l")
	start, end := s.slicing().Slices[1].Start, s.slicing().Slices[1].End
	s.press("]", "]", "]")
	if got := s.slicing().Slices[1]; got.Start <= start || got.End != end {
		t.Fatalf("start %d (was %d), end %d (was %d)", got.Start, start, got.End, end)
	}
	// Nothing else moved: the slice before still ends where it did.
	if s.slicing().Slices[0].End != start {
		t.Errorf("slice 1 ends at %d, it did at %d", s.slicing().Slices[0].End, start)
	}
	s.press("{", "{", "{")
	if got := s.slicing().Slices[1]; got.End >= end {
		t.Errorf("end %d did not move earlier than %d", got.End, end)
	}
	s.shows("end ")
	// The end can go past the next slice's start: slices overlap.
	s.press("f")
	for i := 0; i < 12; i++ {
		s.press("}")
	}
	if got := s.slicing().Slices[1]; got.End <= s.slicing().Slices[2].Start {
		t.Errorf("end %d did not go over slice 3, which starts at %d", got.End, s.slicing().Slices[2].Start)
	}
	s.shows("eight dots")
	for i := 0; i < 20; i++ {
		s.press("u")
	}
	if got := s.slicing().Slices[1]; got.Start != start || got.End != end {
		t.Errorf("undo left %d-%d, want %d-%d", got.Start, got.End, start, end)
	}
}

func TestSlicerLeavesGapsOutOfTheRender(t *testing.T) {
	s := slicerOpen(t)
	s.press("l")
	for i := 0; i < 6; i++ {
		s.press("{") // shorten slice 2 by a few dots
	}
	short := s.slicing().Length(1)
	if short >= 6000 {
		t.Fatalf("slice 2 is %d frames", short)
	}
	s.press("enter")
	if got := s.player.played[len(s.player.played)-1].Frames(); got != short {
		t.Errorf("the slice played %d frames, it is %d long", got, short)
	}
}

func TestSlicerCutDeleteAndUndo(t *testing.T) {
	s := slicerOpen(t)
	s.press("c")
	if len(s.slicing().Slices) != 9 || s.m().slicer.at != 1 {
		t.Fatalf("after a cut: %d slices, in front %d", len(s.slicing().Slices), s.m().slicer.at)
	}
	s.shows("9 slices", "Slice 2/9")
	s.press("d")
	if len(s.slicing().Slices) != 8 {
		t.Fatalf("after a delete: %d slices", len(s.slicing().Slices))
	}
	s.press("u", "u")
	if len(s.slicing().Slices) != 8 {
		t.Errorf("undoing both left %d slices", len(s.slicing().Slices))
	}
	// The last slice cannot be removed, and the attempt leaves nothing to undo.
	s.press("e")
	for i := 0; i < 40; i++ {
		s.press("d")
	}
	if len(s.slicing().Slices) != 1 {
		t.Errorf("%d slices left", len(s.slicing().Slices))
	}
	s.shows("cannot be removed")
}

func TestSlicerOnsetsFindTheDrumHits(t *testing.T) {
	s := slicerOpen(t)
	s.press("t")
	got := s.slicing()
	if len(got.Slices) != 4 {
		t.Fatalf("%d slices from the onsets, want 4: %+v", len(got.Slices), got.Slices)
	}
	for i, want := range []int{0, 12000, 24000, 36000} {
		if d := got.Slices[i].Start - want; i > 0 && (d > 0 || d < -600) {
			t.Errorf("slice %d starts at %d, want just before %d", i+1, got.Slices[i].Start, want)
		}
	}
}

func TestSlicerSliceSettings(t *testing.T) {
	s := slicerOpen(t)
	s.press("l", "r", "(", "(", ">", ">", ">", "-")
	x := s.slicing().Slices[1]
	if !x.Reverse || x.Chance != 80 || x.Pitch != 3 || x.Gain != 90 {
		t.Fatalf("slice 2 is %+v", x)
	}
	s.shows("reverse", "+3", "80%", "90%")
	s.press("m")
	if s.slicing().Slices[1].Chance != 0 {
		t.Error("m did not mute")
	}
	s.press("m")
	if s.slicing().Slices[1].Chance != 100 {
		t.Error("m again did not bring it back")
	}
	s.press("X")
	if x := s.slicing().Slices[1]; x.Reverse || x.Pitch != 0 || x.Gain != 100 {
		t.Errorf("X left %+v", x)
	}
}

func TestSlicerOrderLane(t *testing.T) {
	s := slicerOpen(t)
	s.press("tab")
	s.press("L") // carry step 1 one place later
	if got := s.slicing().Order[:3]; got[0] != 1 || got[1] != 0 || got[2] != 2 {
		t.Fatalf("order %v", s.slicing().Order)
	}
	s.press("j") // the step under the cursor plays the next slice instead
	if s.slicing().Order[1] != 1 {
		t.Errorf("order %v", s.slicing().Order)
	}
	s.press("a")
	if len(s.slicing().Order) != 9 {
		t.Errorf("a did not repeat the step: %v", s.slicing().Order)
	}
	s.press("d", "d")
	if len(s.slicing().Order) != 7 {
		t.Errorf("order %v", s.slicing().Order)
	}
	s.press("o")
	if len(s.slicing().Order) != 8 || s.slicing().Order[0] != 0 {
		t.Errorf("o did not reset the order: %v", s.slicing().Order)
	}
	s.press("R")
	seen := map[int]bool{}
	for _, o := range s.slicing().Order {
		seen[o] = true
	}
	if len(seen) != 8 {
		t.Errorf("a shuffle lost slices: %v", s.slicing().Order)
	}
}

func TestSlicerModes(t *testing.T) {
	s := slicerOpen(t)
	for _, want := range []string{"random", "no repeat", "shuffle", "order"} {
		s.press("M")
		s.shows("mode " + want)
	}
	s.press("M")
	seed := s.slicing().Seed
	s.press("S")
	if s.slicing().Seed == seed {
		t.Error("S did not roll a new seed")
	}
}

func TestSlicerPlaysRoundsWithoutAGap(t *testing.T) {
	s := slicerOpen(t)
	s.press("space")
	if len(s.player.looped) != 1 || !s.m().slicer.playing {
		t.Fatalf("looped %d, playing %v", len(s.player.looped), s.m().slicer.playing)
	}
	s.shows("playing")
	round := s.player.looped[0]
	if round.Frames() < 48000*6/5 {
		t.Errorf("a round is %d frames: shorter than it needs to be to queue ahead", round.Frames())
	}
	// A tick well into the round queues the next one behind it.
	s.m().slicer.cur.start = time.Now().Add(-s.m().slicer.cur.dur / 2)
	s.send(slicerTickMsg{gen: s.m().slicer.gen})
	if len(s.player.queued) != 1 {
		t.Fatalf("queued %d rounds", len(s.player.queued))
	}
	// When the first is over the queued one is the round that sounds.
	s.m().slicer.cur.start = time.Now().Add(-s.m().slicer.cur.dur - time.Millisecond)
	s.send(slicerTickMsg{gen: s.m().slicer.gen})
	if s.m().slicer.round != 1 {
		t.Errorf("round %d", s.m().slicer.round)
	}
	// A tick of a play that was stopped does nothing.
	gen := s.m().slicer.gen
	s.press("space")
	if s.m().slicer.playing || s.player.playing {
		t.Error("space did not stop")
	}
	queued := len(s.player.queued)
	s.send(slicerTickMsg{gen: gen})
	if len(s.player.queued) != queued {
		t.Error("a stale tick queued a round")
	}
}

func TestSlicerEditWhilePlayingIsQueued(t *testing.T) {
	s := slicerOpen(t)
	s.press("space")
	s.m().slicer.cur.start = time.Now().Add(-s.m().slicer.cur.dur / 2)
	s.send(slicerTickMsg{gen: s.m().slicer.gen})
	queued := len(s.player.queued)
	s.press("m") // mute slice 1
	s.send(slicerTickMsg{gen: s.m().slicer.gen})
	if len(s.player.queued) != queued+1 {
		t.Fatalf("the edit queued %d more rounds, want 1", len(s.player.queued)-queued)
	}
	// Slice 1 is a rest in the round that was made after the edit.
	next := s.m().slicer.nxt
	for _, p := range next.placed {
		if p.Slice == 0 && !p.Rest {
			t.Fatal("the muted slice still plays in the queued round")
		}
	}
}

func TestSlicerAuditionPlaysTheSliceOnce(t *testing.T) {
	s := slicerOpen(t)
	s.press("l", "r", "enter")
	if len(s.player.played) != 1 || s.player.played[0].Frames() != 6000 {
		t.Fatalf("played %d samples", len(s.player.played))
	}
}

func TestSlicerRememberedBetweenOpens(t *testing.T) {
	s := slicerOpen(t)
	s.press("l", "r", "(", "c")
	want := len(s.slicing().Slices)
	s.press("esc", "Z")
	if s.m().slicer == nil {
		t.Fatal("the slicer did not open again")
	}
	if got := s.slicing(); len(got.Slices) != want || !got.Slices[1].Reverse || got.Slices[1].Chance != 90 {
		t.Errorf("slices not kept: %+v", got.Slices)
	}
}

func TestSlicerBounceToAPad(t *testing.T) {
	s := slicerOpen(t)
	s.press("e") // 16 equal slices
	s.press("l", "r")
	s.press("b")
	s.shows("Bounce the slices to a pad")
	want := s.m().slicer.bounceSample().Frames()
	s.press("A", "9", "enter")
	s.shows("bounced")
	row := s.m().snap.Bank(0)[8]
	if !row.HasSample {
		t.Fatal("A9 has no sample")
	}
	got, err := app.PadAudio(s.m().unit, row)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frames() != want {
		t.Errorf("A9 holds %d frames, bounced %d", got.Frames(), want)
	}
	if s.m().slicer == nil {
		t.Error("the bounce closed the slicer")
	}
	// A pad that holds a sample is replaced only on a second Enter.
	s.press("b", "A", "5")
	s.press("enter")
	s.shows("Press Enter again")
	s.press("esc")
}

func TestSlicerBounceRoundsAndSeedsAreRepeatable(t *testing.T) {
	s := slicerOpen(t)
	s.press("M") // random
	one := s.m().slicer.bounceSample()
	again := s.m().slicer.bounceSample()
	if one.Frames() != again.Frames() || len(one.Samples) == 0 {
		t.Fatal("two bounces of the same state differ in length")
	}
	for i := range one.Samples {
		if one.Samples[i] != again.Samples[i] {
			t.Fatal("two bounces of the same state differ")
		}
	}
	s.press("n")
	if s.m().slicer.rounds != 2 {
		t.Errorf("rounds %d", s.m().slicer.rounds)
	}
	if two := s.m().slicer.bounceSample(); two.Frames() <= one.Frames() {
		t.Errorf("two rounds are %d frames, one %d", two.Frames(), one.Frames())
	}
}

func TestSlicerOnAFile(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 140, Height: 50})
	s.sim.AddFile("SD:IMPORT/drums.wav", drums())
	s.press("2")
	s.expand("IMPORT")
	s.cursorOn("drums.wav")
	s.press("Z")
	if s.m().slicer == nil {
		t.Fatalf("the slicer did not open on the file:\n%s", s.model.View())
	}
	s.shows("drums.wav")
	s.press("esc")
	s.cursorOn("IMPORT")
	s.press("Z")
	s.shows("audio file")
}

func TestSlicerGrid(t *testing.T) {
	s := slicerOpen(t)
	s.press("T")
	sl := s.slicing()
	if sl.Division != 4 || sl.BPM != 120 {
		t.Fatalf("division %d, bpm %g", sl.Division, sl.BPM)
	}
	s.shows("grid 1/4 at 120 BPM")
	s.press("T")
	s.press("T") // 1/16
	s.press(".", ".")
	if sl := s.slicing(); sl.Division != 16 || sl.BPM != 122 {
		t.Fatalf("division %d, bpm %g", sl.Division, sl.BPM)
	}
	want := s.slicing().StepFrames() * len(s.slicing().Order)
	if got := s.m().slicer.bounceSample().Frames(); got != want {
		t.Errorf("a bounce is %d frames, %d steps of the grid are %d", got, len(s.slicing().Order), want)
	}
	s.press("T")
	s.shows("grid off")
	s.press(".")
	s.shows("no grid")
}

func TestSlicerEmptyPad(t *testing.T) {
	s := open(t)
	s.press("r", "j", "j", "Z")
	if s.m().slicer != nil {
		t.Error("the slicer opened on an empty pad")
	}
	s.shows("is empty")
}
