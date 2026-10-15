package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/muesli/termenv"
	"math"
	"strconv"
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
	want := s.m().slicer.takeSample(0).Frames()
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
	one := s.m().slicer.takeSample(0)
	again := s.m().slicer.takeSample(0)
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
	if two := s.m().slicer.takeSample(0); two.Frames() <= one.Frames() {
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
	if got := s.m().slicer.takeSample(0).Frames(); got != want {
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

// voice is a second of a rising tone with a hit every quarter second, a second sample to put
// with the drums.
func voice() []byte {
	s := formats.Sample{Channels: 1, Samples: make([]int16, 24000)}
	for _, start := range []int{0, 8000, 16000} {
		for i := 0; i < 4000; i++ {
			s.Samples[start+i] = int16(15000 * math.Exp(-float64(i)/1500) * math.Sin(float64(i)*0.3))
		}
	}
	return s.WAV()
}

// withVoice puts a second sample on A6, next to the drums on A5, and leaves the slicer: the
// cursor is on the drums.
func (s *screen) withVoice() {
	s.t.Helper()
	s.sim.AddFile("SD:IMPORT/voice.wav", voice())
	m := s.m()
	if _, err := app.Assign(m.unit, m.unit, pad(s.t, "A6"), "SD:IMPORT/voice.wav"); err != nil {
		s.t.Fatal(err)
	}
	s.press("esc", "r")
}

func TestSlicerRandomCutMakesUnequalSlices(t *testing.T) {
	s := slicerOpen(t)
	s.press("E")
	sl := s.slicing()
	if len(sl.Slices) != 8 {
		t.Fatalf("%d slices, E keeps the number", len(sl.Slices))
	}
	shortest, longest := sl.Frames, 0
	for i := range sl.Slices {
		shortest, longest = min(shortest, sl.Length(i)), max(longest, sl.Length(i))
	}
	if longest-shortest < sl.Frames/50 {
		t.Errorf("slices run from %d to %d frames: they are equal", shortest, longest)
	}
	s.shows("at random places")
	s.press("e")
	if got := s.slicing(); got.Length(0) != got.Length(1) && len(got.Slices) != 16 {
		t.Errorf("e did not make equal slices: %d", len(got.Slices))
	}
	s.press("u", "u")
	if got := s.slicing(); len(got.Slices) != 8 || got.Length(0) != 6000 {
		t.Errorf("undo left %d slices", len(got.Slices))
	}
}

func TestSlicerRollsASlice(t *testing.T) {
	s := slicerOpen(t)
	s.press("x")
	s.shows("rolls", "x2")
	for i := 0; i < 4; i++ {
		s.press("x")
	}
	if s.slicing().Slices[0].Roll != 8 {
		t.Fatalf("roll %d after five presses", s.slicing().Slices[0].Roll)
	}
	s.press("x")
	if s.slicing().Slices[0].Roll != 0 {
		t.Errorf("roll %d: it did not come round to once", s.slicing().Slices[0].Roll)
	}
	s.press("x", "x") // x3
	s.press("enter")
	if got := s.player.played[len(s.player.played)-1]; got.Frames() != 6000 {
		t.Errorf("a slice that rolls is %d frames, want 6000", got.Frames())
	}
}

func TestSlicerSwingAndHumanize(t *testing.T) {
	s := slicerOpen(t)
	s.press("w")
	s.shows("swing needs the grid")
	s.press("T", "w")
	s.shows("swing 54%")
	if s.slicing().Swing != 54 {
		t.Errorf("swing %d", s.slicing().Swing)
	}
	for i := 0; i < 5; i++ {
		s.press("w")
	}
	if s.slicing().Swing != 0 {
		t.Errorf("swing %d: it did not come round to off", s.slicing().Swing)
	}
	s.press("W")
	s.shows("humanize 25%")
	s.press("W", "W", "W")
	if s.slicing().Humanize != 0 {
		t.Errorf("humanize %d", s.slicing().Humanize)
	}
}

func TestSlicerAddsASampleAndPlaysAcrossBoth(t *testing.T) {
	s := slicerOpen(t)
	s.withVoice()
	s.press("Z", "A")
	s.shows("Pads", "move to the pad")
	if s.m().slicer == nil || !s.m().slicer.adding {
		t.Fatal("the slicer is not waiting for a sample")
	}
	s.hides("Slice 1/8")
	// Z on the drums again would add the same sample; the voice is on A6.
	s.press("l", "Z")
	sl := s.m().slicer
	if sl == nil || sl.adding || len(sl.srcs) != 2 {
		t.Fatalf("the sample was not added: %+v", sl)
	}
	if got := s.slicing(); got.Frames != 48000+24000 || len(got.Slices) != 16 {
		t.Fatalf("%d frames, %d slices", got.Frames, len(got.Slices))
	}
	s.shows("Samples", "voice", "16 slices")
	if sl.at != 8 {
		t.Errorf("the cursor is on slice %d, want the first of the new sample", sl.at+1)
	}
	// The random modes pick across both: with enough rounds slices of both samples come up.
	s.press("M")
	seen := map[int]bool{}
	for i := 0; i < 6; i++ {
		for _, p := range sl.render(i, 1).placed {
			seen[sl.sourceAt(sl.sl.Slices[p.Slice].Start)] = true
		}
	}
	if !seen[0] || !seen[1] {
		t.Errorf("only samples %v played", seen)
	}
	// e cuts each sample on its own, by its length: 32 slices make 21 and 11, none across.
	s.press("e", "e")
	for i, x := range sl.sl.Slices {
		if x.Start < 48000 && x.End > 48000 {
			t.Errorf("slice %d runs over the joint: %d-%d", i+1, x.Start, x.End)
		}
	}
	// Undo takes the sample back out.
	s.press("u", "u", "u")
	if len(sl.srcs) != 2 {
		s.press("u")
	}
	for i := 0; i < 5 && len(s.m().slicer.srcs) > 1; i++ {
		s.press("u")
	}
	if got := s.m().slicer; len(got.srcs) != 1 || got.src.Frames() != 48000 || got.sl.Frames != 48000 {
		t.Errorf("%d samples, %d frames after undo", len(got.srcs), got.src.Frames())
	}
}

func TestSlicerAddCanBeCancelledAndKeepsTheChain(t *testing.T) {
	s := slicerOpen(t)
	s.withVoice()
	s.press("Z", "A", "esc")
	if sl := s.m().slicer; sl == nil || sl.adding || len(sl.srcs) != 1 {
		t.Fatalf("Esc did not give up adding: %+v", sl)
	}
	s.shows("Slice 1/8")
	// Put together, edited, closed and put together again: the edit is back.
	s.press("A", "l", "Z")
	s.press("r")
	s.press("esc", "h") // the cursor went to the voice; back to the drums
	s.press("Z", "A", "l", "Z")
	if got := s.slicing(); len(got.Slices) != 16 || !got.Slices[8].Reverse {
		t.Errorf("the slicing of the two was not kept: %+v", got.Slices[8])
	}
}

func TestSlicerChainFitsTheScreen(t *testing.T) {
	for _, size := range [][2]int{{80, 30}, {100, 40}, {140, 50}} {
		s := slicerOpen(t)
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		s.withVoice()
		s.press("Z", "A", "l", "Z", "i")
		if len(s.m().slicer.srcs) != 2 {
			t.Fatalf("%dx%d: not added", size[0], size[1])
		}
		box := s.m().slicerBox()
		if h := len(strings.Split(box, "\n")); h > s.m().geom().bodyH {
			t.Errorf("%dx%d: the box is %d lines, the screen has room for %d", size[0], size[1], h, s.m().geom().bodyH)
		}
		for i, line := range strings.Split(box, "\n") {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Errorf("%dx%d: line %d is %d wide", size[0], size[1], i, w)
			}
		}
	}
}

func TestSlicerTapMarksSlicesWhileTheSamplePlays(t *testing.T) {
	s := slicerOpen(t)
	s.press("p")
	if len(s.player.played) != 1 || s.player.played[0].Frames() != 48000 {
		t.Fatalf("played %d samples", len(s.player.played))
	}
	tap := s.m().slicer.tap
	if tap == nil || len(s.slicing().Slices) != 1 {
		t.Fatalf("tap %v, %d slices: the slicing is cleared to one slice", tap, len(s.slicing().Slices))
	}
	s.shows("tapping", "Space")
	// Taps a little after the hits at 12000 and 36000 (the second, 100 ms late): each lands on
	// its onset.
	tap.start = time.Now().Add(-framesDuration(12000 + 1500))
	s.press("space")
	tap.start = time.Now().Add(-framesDuration(36000 + 4000))
	s.press("space")
	// Keys that are not the tap's are left alone.
	s.press("r", "M", "e")
	if got := s.slicing(); len(got.Slices) != 3 || got.Slices[0].Reverse || got.Mode != formats.ModeOrder {
		t.Fatalf("slices %+v", got.Slices)
	}
	for i, want := range []int{12000, 36000} {
		if got := s.slicing().Slices[i+1].Start; absDiff(got, want) > 300 {
			t.Errorf("mark %d at %d, want about %d", i+1, got, want)
		}
	}
	s.press("u")
	if len(s.slicing().Slices) != 2 {
		t.Errorf("u left %d slices", len(s.slicing().Slices))
	}
	s.press("p")
	if s.m().slicer.tap != nil || s.player.stopped == 0 {
		t.Error("p did not stop the tap")
	}
	s.shows("1 marks", "2 slices")
	// The slicing is kept: u takes the tap back to the eight slices.
	s.press("u", "u")
	if len(s.slicing().Slices) != 8 {
		t.Errorf("%d slices after undoing the tap", len(s.slicing().Slices))
	}
}

func TestSlicerTapEndsWhenTheSampleDoes(t *testing.T) {
	s := slicerOpen(t)
	s.press("p")
	s.player.playing = false
	s.send(slicerTickMsg{gen: s.m().slicer.gen})
	if s.m().slicer.tap != nil {
		t.Error("the tap went on after the sample ended")
	}
	s.shows("no marks")
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

func TestSlicerBounceTakesGoOnFreePads(t *testing.T) {
	s := slicerOpen(t)
	s.press("v")
	s.shows("2 takes")
	s.press("v", "v", "v")
	if s.m().slicer.takes != 1 {
		t.Fatalf("takes %d after four presses", s.m().slicer.takes)
	}
	s.press("v") // 2
	// In order mode with every slice always playing, the takes would all be the same.
	s.press("b")
	s.shows("every take would sound the same")
	if s.m().picker != nil {
		t.Fatal("the picker opened for takes that do not differ")
	}
	s.press("M", "b")
	s.shows("Bounce 2 takes")
	// From A4: A4 is free, A5 holds the drums and is left alone, A6 is free.
	s.press("A", "4")
	s.shows("A4, A6", "left alone")
	s.press("enter")
	s.shows("bounced 2 takes to A4, A6")
	if s.padName("A5") != "drums" {
		t.Errorf("A5 holds %q: the drums were replaced", s.padName("A5"))
	}
	if s.padName("A4") != "drums slc1" || s.padName("A6") != "drums slc2" {
		t.Errorf("A4 %q, A6 %q", s.padName("A4"), s.padName("A6"))
	}
	one, err := app.PadAudio(s.m().unit, s.m().snap.Bank(0)[3])
	if err != nil {
		t.Fatal(err)
	}
	two, err := app.PadAudio(s.m().unit, s.m().snap.Bank(0)[5])
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Samples) == 0 || slicesEqual(one.Samples, two.Samples) {
		t.Error("the two takes are empty or the same")
	}
	want := s.m().slicer.takeSample(1)
	if !slicesEqual(two.Samples, want.Samples) {
		t.Error("take 2 is not what the slicer renders for it")
	}
	if s.m().slicer == nil {
		t.Error("the bounce closed the slicer")
	}
}

func slicesEqual(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSlicerGridTicksShowWithGrid(t *testing.T) {
	s := slicerOpen(t)
	if strings.Contains(s.m().View(), "┃") {
		t.Fatal("ticks shown with the grid off")
	}
	s.press("T")
	v := s.m().View()
	if !strings.Contains(v, "┃") || !strings.Contains(v, "╵") {
		t.Fatalf("no bar and beat ticks with the grid on:\n%s", v)
	}
	if strings.Contains(v, "the same seed picks") {
		t.Fatal("the long seed sentence is back on screen")
	}
}

func TestSlicerPlainSettingsKeepTheSlicesEdges(t *testing.T) {
	s := slicerOpen(t)
	s.press("l", "r", "(", "x")
	before := s.slicing().Slices[1]
	s.press("X")
	got := s.slicing().Slices[1]
	if got.Start != before.Start || got.End != before.End || got.End <= got.Start {
		t.Errorf("X changed the edges: %+v -> %+v", before, got)
	}
	if got.Reverse || got.Chance != 100 || got.Gain != 100 || got.Roll != 0 {
		t.Errorf("X left settings behind: %+v", got)
	}
}

func TestSlicerNumbersDropWhenSlicesAreTiny(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 80, Height: 40})
	// 120 slices of 400 frames: far more than the 72 cells of the waveform can label.
	sl := s.slicing()
	sl.Slices = sl.Slices[:0]
	for i := 0; i < 120; i++ {
		sl.Slices = append(sl.Slices, formats.Slice{Start: i * 400, End: (i + 1) * 400, Chance: 100, Gain: 100})
	}
	sl.ResetOrder()
	s.m().slicer.at = 57
	numbers := ""
	for _, line := range strings.Split(ansiStrip(s.m().slicerBox()), "\n") {
		if numbers == "" && strings.Contains(line, "58") { // the line of slice numbers is the first
			numbers = line
		}
	}
	runs := strings.FieldsFunc(numbers, func(r rune) bool { return r < '0' || r > '9' })
	sawSelected := false
	for _, run := range runs {
		n, _ := strconv.Atoi(run)
		if n < 1 || n > 120 {
			t.Errorf("run %q is not a slice number: %q", run, numbers)
		}
		sawSelected = sawSelected || n == 58
	}
	if !sawSelected {
		t.Errorf("the selected slice 58 has no number: %q", numbers)
	}
	if !strings.Contains(numbers, "▸58") {
		t.Errorf("the selected slice has no start marker: %q", numbers)
	}
}

func TestSlicerOrderLaneMarksTheSelectedSlice(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)
	s := slicerOpen(t)
	s.press("l", "l") // slice 3, lane not focused
	three := s.m().orderLane(80, false)
	s.press("l") // slice 4
	four := s.m().orderLane(80, false)
	if three == four {
		t.Error("the lane looks the same with slice 3 and slice 4 selected")
	}
	if ansiStrip(three) != ansiStrip(four) {
		t.Errorf("selecting changed the lane's text:\n%q\n%q", ansiStrip(three), ansiStrip(four))
	}
}

func TestSlicerEnvelopeKeys(t *testing.T) {
	s := slicerOpen(t)
	s.press("l", "2", "2", "4")
	x := s.slicing().Slices[1]
	if x.Attack != 5 || x.Release != 2 {
		t.Errorf("attack %d release %d", x.Attack, x.Release)
	}
	s.shows("env 5/2 ms")
	s.press("1", "3")
	if x := s.slicing().Slices[1]; x.Attack != 2 || x.Release != 0 {
		t.Errorf("attack %d release %d after stepping back", x.Attack, x.Release)
	}
	s.press("2", "4", "X")
	if x := s.slicing().Slices[1]; x.Attack != 0 || x.Release != 0 {
		t.Errorf("X left an envelope: %+v", x)
	}
}

func TestSlicerBouncesMarkedSlicesToPads(t *testing.T) {
	s := slicerOpen(t)
	s.press("B")
	s.shows("no slice is marked")
	s.press("l", "s", "l", "l", "s") // slices 2 and 4
	s.shows("2 slices marked")
	s.press("B")
	s.shows("Put the 2 marked slices")
	s.press("A", "9", "enter")
	s.shows("bounced")
	bank := s.m().snap.Bank(0)
	a9, a10 := bank[8], bank[9]
	if !a9.HasSample || !a10.HasSample {
		t.Fatalf("A9 %v, A10 %v", a9.HasSample, a10.HasSample)
	}
	for i, row := range []app.PadRow{a9, a10} {
		got, err := app.PadAudio(s.m().unit, row)
		if err != nil {
			t.Fatal(err)
		}
		want := formats.SliceAudio(s.m().slicer.src, s.slicing(), []int{1, 3}[i])
		if got.Frames() != want.Frames() {
			t.Errorf("pad %d holds %d frames, slice %d is %d", 9+i, got.Frames(), []int{2, 4}[i], want.Frames())
		}
	}
	if !strings.Contains(s.padName("A9"), "s2") || !strings.Contains(s.padName("A10"), "s4") {
		t.Errorf("pad names %q %q", s.padName("A9"), s.padName("A10"))
	}
}

func TestSlicerMarksGoWhenSlicesChange(t *testing.T) {
	s := slicerOpen(t)
	s.press("s", "y")
	if len(s.m().slicer.marks) != 8 {
		t.Fatalf("y marked %d", len(s.m().slicer.marks))
	}
	s.press("y")
	if len(s.m().slicer.marks) != 0 {
		t.Errorf("a second y left %d marked", len(s.m().slicer.marks))
	}
	s.press("s", "c")
	if len(s.m().slicer.marks) != 0 {
		t.Error("cutting a slice kept marks that now point at other slices")
	}
}

func TestSlicerDeletingWhilePlayingDoesNotBreakTheScreen(t *testing.T) {
	s := slicerOpen(t)
	s.press(" ")
	for i := 0; i < 7; i++ {
		s.press("l")
	}
	for i := 0; i < 5; i++ {
		// The playhead is in the last steps of the round, which a delete takes away.
		if cur := s.m().slicer.cur; cur != nil {
			cur.start = time.Now().Add(-cur.dur * 9 / 10)
		}
		s.press("d")
		_ = s.model.View()
		s.press("h")
	}
	if got := len(s.slicing().Slices); got != 3 {
		t.Errorf("%d slices left", got)
	}
}

func TestSlicerPlaysUnderAPattern(t *testing.T) {
	s := slicerOpen(t)
	patternOnA2(t, s)
	s.press("P")
	s.shows("Pattern  bank A")
	s.press("enter")
	bed := s.m().slicer.bed
	if bed == nil {
		t.Fatalf("P did not put a pattern under the slices: %q", s.m().message)
	}
	sl := s.slicing()
	if sl.BPM != bed.bpm || sl.Division != 16 {
		t.Errorf("the grid is %g BPM 1/%d, the pattern is %g BPM", sl.BPM, sl.Division, bed.bpm)
	}
	// A round is exactly the pattern long, or a whole number of times that, so the two stay together.
	for _, passes := range []int{1, 2, 4} {
		round := s.m().slicer.render(0, passes).sample
		if round.Channels != 2 || round.Frames() != bed.sample.Frames()*passes {
			t.Errorf("%d passes: %d frames of %d channels, the pattern is %d", passes, round.Frames(), round.Channels, bed.sample.Frames())
		}
	}
	// The pattern is heard in it: without it the round differs.
	with := s.m().slicer.render(0, 1).sample
	s.m().slicer.bed = nil
	without := s.m().slicer.render(0, 1).sample
	same := with.Frames() == without.Frames()
	for i := 0; same && i < len(with.Samples) && i < len(without.Samples); i++ {
		same = with.Samples[i] == without.Samples[i]
	}
	if same {
		t.Error("the round sounds the same with and without the pattern")
	}
	if loud(bed.sample) == 0 {
		t.Error("the pattern mix is silent")
	}
	s.m().slicer.bed = bed
	// A bounce is the slices alone, without the pattern.
	if got := s.m().slicer.takeSample(0); got.Frames() == bed.sample.Frames() && got.Channels == 2 {
		t.Error("the bounce looks like the pattern mix")
	}
	if got, alone := s.m().slicer.takeSample(0), s.m().slicer.renderUnder(0, 1, nil).sample; len(got.Samples) != len(alone.Samples) {
		t.Errorf("a bounce is %d samples, the slices alone %d", len(got.Samples), len(alone.Samples))
	}
	// The tempo follows the pattern.
	s.press(",")
	if s.slicing().BPM != bed.bpm {
		t.Error(", changed the tempo under a pattern")
	}
	// Playing works, and the next P takes the pattern away again.
	s.press(" ")
	for i := 0; i < 50; i++ {
		s.send(slicerTickMsg{gen: s.m().slicer.gen})
	}
	_ = s.model.View()
	s.press(" ", "P")
	s.shows("Pattern  bank A")
	s.press("0")
	if s.m().slicer.bed != nil {
		t.Error("0 in the chooser did not take the pattern away")
	}
}

func TestSlicerPatternChooser(t *testing.T) {
	s := slicerOpen(t)
	patternOnA2(t, s, "A2", "A7", "B3")
	s.press("P")
	if c := s.m().slicer.chooser; c == nil || c.cursor != 1 {
		t.Fatalf("the chooser starts on %+v, want slot 2", c)
	}
	s.shows("arrows move")
	// An empty slot is not taken.
	s.press("l", "enter")
	if s.m().slicer.bed != nil || s.m().slicer.chooser == nil {
		t.Error("an empty slot was taken or the chooser closed")
	}
	// Other bank, then leave without choosing.
	s.press("]")
	if c := s.m().slicer.chooser; c.bank != 1 || c.cursor != 2 {
		t.Fatalf("bank %d cursor %d, want bank B with the cursor on slot 3", c.bank, c.cursor)
	}
	s.press("esc")
	if s.m().slicer.chooser != nil || s.m().slicer.bed != nil {
		t.Error("Esc did not leave the chooser as it was")
	}
	// The pattern of another bank is taken, with its own tempo.
	s.press("P", "]", "l", "enter")
	bed := s.m().slicer.bed
	if bed == nil || bed.slot.String() != "B3" {
		t.Fatalf("bed %+v: %q", bed, s.m().message)
	}
	s.shows("with pattern B3")
	// Reopened, the chooser starts on the pattern that is chosen.
	s.press("P")
	if c := s.m().slicer.chooser; c.bank != 1 || c.cursor != 2 {
		t.Errorf("reopened on bank %d slot %d", c.bank, c.cursor+1)
	}
	s.press("esc")
	// Every line still fits.
	for _, size := range [][2]int{{80, 30}, {140, 50}} {
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		s.press("P")
		if h := len(strings.Split(s.m().slicerBox(), "\n")); h > s.m().geom().bodyH {
			t.Errorf("%dx%d: the box with the chooser is %d lines, room for %d", size[0], size[1], h, s.m().geom().bodyH)
		}
		s.press("esc")
	}
}

func TestSlicerPatternIsMixedAgainWhenAPadItPlaysChanges(t *testing.T) {
	s := slicerOpen(t)
	patternOnA2(t, s)
	s.press("P", "enter")
	before := s.m().slicer.bed
	if before == nil {
		t.Fatal("no pattern under the slices")
	}
	// Put the first slice on A5, the pad the pattern plays: the pattern must sound different.
	s.press("s", "B", "A", "5", "enter", "enter")
	s.shows("bounced")
	after := s.m().slicer.bed
	if after == nil || after.slot != before.slot {
		t.Fatalf("the pattern is gone: %+v", after)
	}
	if loud(before.sample) == 0 {
		t.Fatal("the pattern was silent to begin with")
	}
	// The slice is 0.1 s of the 1 s the pad held, so far less of the mix is sound now.
	if loud(after.sample) >= loud(before.sample)/2 {
		t.Errorf("the pattern still has %d loud samples of %d after a short slice took the pad", loud(after.sample), loud(before.sample))
	}
}

// patternOnA2 puts a pattern on A2 that plays A5 (the drums), with A5 at a level you can hear;
// the simulated pads start at level 0.
func patternOnA2(t *testing.T, s *screen, slots ...string) {
	t.Helper()
	level, _ := proto.ParamByName("level")
	if err := s.m().unit.SetParam(level.ID, proto.TargetPad(pad(t, "A5")), 127); err != nil {
		t.Fatal(err)
	}
	data, err := formats.StartPattern([]proto.PadIndex{pad(t, "A5")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 {
		slots = []string{"A2"}
	}
	for _, slot := range slots {
		s.sim.SetPattern(pad(t, slot), pad(t, "A5"))
		s.sim.AddFile(formats.PatternPath(1, pad(t, slot)), data)
	}
	// The screen reads the pads again, so the level is known.
	s.press("esc", "r", "Z")
	if s.m().slicer == nil {
		t.Fatal("the slicer did not open again")
	}
}

// loud counts the samples of a mix that are not silence.
func loud(x formats.Sample) (n int) {
	for _, v := range x.Samples {
		if v > 200 || v < -200 {
			n++
		}
	}
	return n
}
