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
	s.shows("audio file to add")
	if s.m().slicer == nil || !s.m().slicer.adding {
		t.Fatal("the slicer is not waiting for a sample")
	}
	s.hides("Slice 1/8")
	// A file, not a pad: the voice is in IMPORT.
	s.expand("IMPORT")
	s.cursorOn("voice.wav")
	s.press("Z")
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
	addVoice := func() {
		s.press("A")
		s.expand("IMPORT")
		s.cursorOn("voice.wav")
		s.press("Z")
	}
	addVoice()
	s.press("r")
	s.press("esc")
	if s.m().section != secPads {
		t.Errorf("after adding, the screen is in section %d, not back in the pads", s.m().section)
	}
	s.press("Z")
	addVoice()
	if got := s.slicing(); len(got.Slices) != 16 || !got.Slices[8].Reverse {
		t.Errorf("the slicing of the two was not kept: %+v", got.Slices[8])
	}
}

func TestSlicerChainFitsTheScreen(t *testing.T) {
	for _, size := range [][2]int{{80, 30}, {100, 40}, {140, 50}} {
		s := slicerOpen(t)
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		s.withVoice()
		s.press("Z", "A", "1", "l", "Z", "i")
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
	s.press("l", "7", "7", "9")
	x := s.slicing().Slices[1]
	if x.Attack != 5 || x.Release != 2 {
		t.Errorf("attack %d release %d", x.Attack, x.Release)
	}
	s.shows("attack", "5 ms", "release", "2 ms")
	s.press("6", "8")
	if x := s.slicing().Slices[1]; x.Attack != 2 || x.Release != 0 {
		t.Errorf("attack %d release %d after stepping back", x.Attack, x.Release)
	}
	s.press("7", "9", "X")
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

func TestSlicerAddOpensTheFileBrowser(t *testing.T) {
	s := slicerOpen(t)
	s.press("A")
	if s.m().section != secFiles || !s.m().slicer.adding {
		t.Fatalf("section %d, adding %v", s.m().section, s.m().slicer.adding)
	}
	s.shows("audio file to add")
	// A pad can be added too: 1 goes there. Esc gives up and goes back to where A was pressed.
	s.press("1")
	if s.m().section != secPads || !s.m().slicer.adding {
		t.Fatalf("1 did not go to the pads: section %d", s.m().section)
	}
	s.press("esc")
	if s.m().slicer.adding || s.m().section != secSlicer {
		t.Errorf("Esc: adding %v, section %d", s.m().slicer.adding, s.m().section)
	}
	s.press("A", "1", "Z")
	if len(s.m().slicer.srcs) != 2 || s.m().slicer.adding {
		t.Errorf("a pad was not added: %d samples", len(s.m().slicer.srcs))
	}
}

func TestSlicerEqualSaysWhatComesNext(t *testing.T) {
	s := slicerOpen(t)
	s.press("e")
	s.shows("16 equal slices; e again: 32")
	s.press("e")
	s.shows("32 equal slices; e again: 4")
	s.press("e", "e")
	if n := len(s.slicing().Slices); n != 8 {
		t.Errorf("%d slices after 4 then 8", n)
	}
}

func TestSlicerClearThenPlaceEdgesByHand(t *testing.T) {
	s := slicerOpen(t)
	s.press("D")
	if n := len(s.slicing().Slices); n != 1 || !s.m().slicer.cutting {
		t.Fatalf("%d slices, cutting %v", n, s.m().slicer.cutting)
	}
	s.shows("place an edge") // the help of the cursor
	// Move on and place: the edge lands where the cursor is (without snapping to a zero crossing).
	s.press("z") // snap off
	s.press("L", "L", "L", "L", "L", "L", "L", "L")
	at := s.m().slicer.cutAt
	if at <= 0 {
		t.Fatal("the cursor did not move")
	}
	s.press("p")
	if got := s.slicing(); len(got.Slices) != 2 || got.Slices[1].Start != at {
		t.Fatalf("slices %+v, cursor at %d", got.Slices, at)
	}
	// Further on again: a third slice. h goes back a dot, where an edge is possible too.
	s.press("L", "L", "p")
	if len(s.slicing().Slices) != 3 {
		t.Errorf("%d slices", len(s.slicing().Slices))
	}
	before := s.m().slicer.cutAt
	s.press("h")
	if s.m().slicer.cutAt >= before {
		t.Error("h did not move back")
	}
	// Twice in one place: the second is too close to the first.
	s.press("p", "p")
	s.shows("too close")
	if len(s.slicing().Slices) != 4 {
		t.Errorf("%d slices, want 4", len(s.slicing().Slices))
	}
	// d deletes the slice the cursor is in.
	s.press("d")
	if len(s.slicing().Slices) != 3 {
		t.Errorf("%d slices after d", len(s.slicing().Slices))
	}
	s.press("esc")
	if s.m().slicer.cutting || s.m().slicer == nil {
		t.Error("Esc did not just leave the cut cursor")
	}
	for i := 0; i < 20 && len(s.slicing().Slices) != 8; i++ {
		s.press("u")
	}
	if len(s.slicing().Slices) != 8 {
		t.Errorf("undo brought back %d slices, want the 8 it was cleared from", len(s.slicing().Slices))
	}
}

func TestSlicerCutCursorJumpsAGridStep(t *testing.T) {
	s := slicerOpen(t)
	s.press("T") // a grid
	step := s.slicing().StepFrames()
	if step == 0 {
		t.Fatal("no grid")
	}
	s.press("D", "L")
	if got := s.m().slicer.cutAt; got != step {
		t.Errorf("one jump reaches %d, one step is %d", got, step)
	}
	s.press("L", "L", "L")
	if got := s.m().slicer.cutAt; got != s.slicing().Frames-1 {
		t.Errorf("jumps past the end reach %d, the last frame is %d", got, s.slicing().Frames-1)
	}
	s.press("H", "H", "H", "H", "H")
	if got := s.m().slicer.cutAt; got != 0 {
		t.Errorf("jumps back reach %d, want the start", got)
	}
}

func TestSlicerPlayingThenPlacesAtThePlayhead(t *testing.T) {
	s := slicerOpen(t)
	s.press("D", "z", " ")
	// The playhead is 0.5 s in.
	if cur := s.m().slicer.cur; cur != nil {
		cur.start = time.Now().Add(-500 * time.Millisecond)
	}
	s.press("p")
	if n := len(s.slicing().Slices); n != 2 {
		t.Fatalf("%d slices", n)
	}
	at := s.slicing().Slices[1].Start
	if at < 22000 || at > 26000 {
		t.Errorf("the edge is at frame %d, the playhead was near 24000", at)
	}
}

func TestSlicerCutsOnTheTempoGrid(t *testing.T) {
	s := slicerOpen(t)
	bpm := s.m().slicerBankTempo()
	step := int(60 / bpm * formats.SampleRate * 4 / 16)
	s.press("C") // the grid is off: it goes on at 1/16 and the sample is cut in steps
	got := s.slicing()
	if got.Division != 16 || got.BPM != bpm {
		t.Fatalf("grid 1/%d at %g BPM, want 1/16 at %g", got.Division, got.BPM, bpm)
	}
	if want := (48000 + step - 1) / step; len(got.Slices) < want-1 || len(got.Slices) > want {
		t.Errorf("%d slices of %d frames in 48000", len(got.Slices), step)
	}
	if got.Slices[1].Start != step || got.Slices[0].Start != 0 {
		t.Errorf("the slices start at %d and %d, a step is %d", got.Slices[0].Start, got.Slices[1].Start, step)
	}
	s.shows("bars of 4/4", "u undoes")
	// A coarser grid, and the tempo: 1/4 at 120 BPM is half a second.
	s.press("T", "T") // 1/4 -> 1/8 -> 1/16 -> off? set it by looking
	for s.slicing().Division != 4 {
		s.press("T")
	}
	s.press("C")
	if n := len(s.slicing().Slices); n != int(math.Ceil(48000/(60/bpm*formats.SampleRate))) {
		t.Errorf("%d quarter notes", n)
	}
	// The cut cursor says where a grid line is.
	s.press("D", "L", "C") // the cursor sits one grid step in; a line there changes nothing
	s.shows("with a line at the cursor")
	// Undo brings the old slices back.
	for i := 0; i < 3; i++ {
		s.press("u")
	}
	if n := len(s.slicing().Slices); n == 0 {
		t.Error("undo left no slices")
	}
}

func TestSlicerShowsThePatternUnderTheSlices(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	patternOnA2(t, s, "A2", "A7", "B3")
	s.press("P")
	// The chooser shows the pattern it is on, as the pattern editor does.
	s.shows("Pattern A2", "A5   drums")
	s.press("enter")
	s.shows("with pattern A2", "Pattern A2")
	// While the slices play the playhead goes round it.
	s.press(" ")
	s.send(slicerTickMsg{gen: s.m().slicer.gen})
	s.shows("s to go")
	s.press(" ")
	for _, size := range [][2]int{{80, 30}, {100, 40}, {140, 50}} {
		s.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if h := len(strings.Split(s.m().slicerBox(), "\n")); h > s.m().geom().bodyH {
			t.Errorf("%dx%d: the box is %d lines, room for %d", size[0], size[1], h, s.m().geom().bodyH)
		}
	}
}

func TestSlicerGridTempoComesFromTheName(t *testing.T) {
	for name, want := range map[string]float64{"125_b_downsouth": 125, "loop 90bpm": 90, "kick_039": 0, "take 7 of 3000": 0} {
		m := Model{slicer: &slicer{name: name}}
		got := m.slicerStartTempo()
		if want == 0 {
			if got == want {
				t.Errorf("%q: no tempo in the name, got %g", name, got)
			}
			continue
		}
		if got != want {
			t.Errorf("%q: %g, want %g", name, got, want)
		}
	}
}

func TestSlicerSaysHowManyRoundsCoverThePattern(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	patternOnA2(t, s, "A2", "A7", "B3")
	s.press("T")
	for s.m().slicer.sl.Division != 16 {
		s.press("T")
	}
	s.press("P", "enter")
	c, ok := s.m().slicer.coverRounds()
	if !ok {
		t.Fatalf("no cover with a grid and a pattern: %q", s.m().message)
	}
	// The pattern is a bar of 16 steps; the order has 8 steps: two rounds.
	if c.patSteps != 16 || c.roundSteps != 8 || c.rounds != 2 {
		t.Fatalf("cover %+v, want 2 rounds of 8 steps over 16", c)
	}
	s.shows("a round is 8 steps, the pattern 16 steps", "press N")
	s.press("N")
	if s.m().slicer.rounds != 2 {
		t.Errorf("N set %d rounds, want 2", s.m().slicer.rounds)
	}
	s.shows("2 rounds fit it exactly")
	// What a bounce holds then is a whole number of pattern loops long.
	if got := s.m().slicer.takeSample(0).Frames(); got != s.m().slicer.bed.sample.Frames() {
		t.Errorf("the bounce is %d frames, the pattern %d", got, s.m().slicer.bed.sample.Frames())
	}
}

func TestSlicerLimitsThePatternToSomeBars(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	patternOnA2(t, s, "A2")
	data, err := formats.StartPattern([]proto.PadIndex{pad(t, "A5")}, 4)
	if err != nil {
		t.Fatal(err)
	}
	s.sim.AddFile(formats.PatternPath(1, pad(t, "A2")), data)
	s.press("T", "P", "enter")
	whole := s.m().slicer.bed.sample.Frames()
	s.press("V")
	s.shows("Bar 1 of the pattern only")
	bed := s.m().slicer.bed
	if got := bed.sample.Frames(); got*4 != whole && (got*4 < whole-8 || got*4 > whole+8) {
		t.Errorf("one bar is %d frames, the whole pattern %d", got, whole)
	}
	s.shows("Bar 1 / 4")
	s.press("V", "V", "V", "V")
	s.shows("Bars 1–2 of the pattern only")
	for i := 0; i < 3; i++ {
		s.press("V")
	}
	if s.m().slicer.bedRange != nil || s.m().slicer.bed.sample.Frames() != whole {
		t.Errorf("V did not come round to the whole pattern: %v", s.m().slicer.bedRange)
	}
}

func TestSlicerRedoAndFitTempo(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 170, Height: 54})
	n := len(s.slicing().Slices)
	s.press("e")
	cut := len(s.slicing().Slices)
	s.press("u")
	s.shows("undo: step 0/1")
	if len(s.slicing().Slices) != n {
		t.Fatalf("u left %d slices, want %d", len(s.slicing().Slices), n)
	}
	s.press("U")
	s.shows("redo: step 1/1")
	if len(s.slicing().Slices) != cut {
		t.Errorf("U left %d slices, want %d", len(s.slicing().Slices), cut)
	}
	s.press("U")
	s.shows("nothing to redo")
	// A new edit drops what could be redone.
	s.press("u", "d", "U")
	s.shows("nothing to redo")

	// F fits the tempo to a whole number of bars: 1 s at 120 BPM is half a bar, so one bar at 240.
	s.press("T")
	s.press("F")
	sl := s.slicing()
	bars := float64(sl.Frames) / (240 / sl.BPM * formats.SampleRate)
	if bars != float64(int(bars+0.5)) && (bars-float64(int(bars+0.5)) > 0.01 || float64(int(bars+0.5))-bars > 0.01) {
		t.Errorf("after F the sample is %.3f bars at %g BPM", bars, sl.BPM)
	}
}

func TestSlicerStretchesTheSampleToBars(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	s.press("T")
	for s.slicing().Division != 16 {
		s.press("T")
	}
	s.press(".", ".", ".", ".", ".", ".", ".", ".", ".", ".", "K")
	if s.m().picker == nil || s.m().picker.bounceWhat != "stretch" {
		t.Fatalf("K opened no pad chooser: %q", s.m().message)
	}
	s.shows("Put the stretched sample on a pad", "bars at")
	got := s.m().picker.bounces[0]
	bpm := s.slicing().BPM
	bars := float64(got.Frames()) / (240 / bpm * formats.SampleRate)
	if bars != float64(int(bars+0.5)) && (bars-float64(int(bars+0.5)) > 0.001 || float64(int(bars+0.5))-bars > 0.001) {
		t.Errorf("the stretched sample is %.3f bars at %g BPM", bars, bpm)
	}
	s.press("A", "9", "enter")
	s.shows("stretched to")
}

func TestSlicerAuditionHasTheRollOfTheSlice(t *testing.T) {
	s := slicerOpen(t)
	sl := s.slicing()
	sl.Slices[s.m().slicer.at].Roll = 4
	rolled := s.m().slicer.auditionAudio(s.m().slicer.at)
	sl.Slices[s.m().slicer.at].Roll = 0
	plain := s.m().slicer.auditionAudio(s.m().slicer.at)
	if rolled.Frames() == 0 || rolled.Frames() != plain.Frames() {
		t.Fatalf("lengths %d and %d", rolled.Frames(), plain.Frames())
	}
	same := true
	for i := range rolled.Samples {
		if rolled.Samples[i] != plain.Samples[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("a rolling slice sounds the same as a plain one when it is heard alone")
	}
}

func TestSlicerRollSliders(t *testing.T) {
	s := slicerOpen(t)
	s.send(tea.WindowSizeMsg{Width: 140, Height: 50})
	s.press("j")
	s.shows("x makes the slice roll first")
	s.press("x", "j", "j", "j")
	if got := s.m().slicer.cur1().RollLoud; got != -75 {
		t.Fatalf("loudness %d, want -75", got)
	}
	s.shows("quieter, from 100% down to 25%", "-75%")
	s.press("'", "'", "'")
	if got := s.m().slicer.cur1().RollPitch; got != 3 {
		t.Errorf("pitch %d, want 3", got)
	}
	s.shows("pitch", "+3")
	s.press("X")
	if x := s.m().slicer.cur1(); x.RollLoud != 0 || x.RollPitch != 0 || x.Roll != 0 {
		t.Errorf("X left %+v", x)
	}
}

func TestSlicerFillsThePageAtAnySize(t *testing.T) {
	s := slicerOpen(t)
	patternOnA2(t, s, "A2")
	s.press("T", "P", "enter")
	for w := 60; w <= 320; w += 20 {
		for h := 16; h <= 110; h += 6 {
			s.send(tea.WindowSizeMsg{Width: w, Height: h})
			box := strings.Split(s.m().slicerBox(), "\n")
			wide := 0
			for _, l := range box {
				wide = max(wide, ansi.StringWidth(l))
			}
			if body := s.m().geom().bodyH; len(box) != body || wide > w {
				t.Errorf("%dx%d: the box is %d lines of %d wide, the page %d lines of %d", w, h, len(box), wide, body, w)
			}
		}
	}
}

func TestSlicerZoomedHAndLZoomOutFirstAndEdgesMoveInAStillView(t *testing.T) {
	s := slicerOpen(t)
	s.press("l", "i")
	if !s.m().slicer.zoomed || s.m().slicer.at != 1 {
		t.Fatalf("zoomed %v at %d", s.m().slicer.zoomed, s.m().slicer.at)
	}
	from, to := s.m().slicer.view()
	// Moving the start of the slice does not move the waveform: only the edge moves.
	s.press("[", "[", "[", "]", "]")
	if f, tt := s.m().slicer.view(); f != from || tt != to {
		t.Errorf("the view moved from %d-%d to %d-%d", from, to, f, tt)
	}
	// h first zooms out, without leaving the slice; the next h moves to the other slice.
	s.press("h")
	if s.m().slicer.zoomed || s.m().slicer.at != 1 {
		t.Errorf("after h: zoomed %v, slice %d", s.m().slicer.zoomed, s.m().slicer.at)
	}
	s.press("h")
	if s.m().slicer.at != 0 {
		t.Errorf("the second h left us on slice %d", s.m().slicer.at)
	}
	// Another slice zoomed gets a view of its own.
	s.press("i")
	f0, _ := s.m().slicer.view()
	s.press("0", "l", "i")
	if f1, _ := s.m().slicer.view(); f1 == f0 {
		t.Error("the next slice shows the same part")
	}
}

func TestAFolderInTheAssignDialogSaysWhereItsSamplesWereUsed(t *testing.T) {
	s := open(t)
	m := s.m()
	m.origins = &app.Origins{Pads: map[string]app.Origin{
		"1/A5": {Path: "SD:IMPORT/Kit/kick.wav", Name: "kick"},
		"1/B2": {Path: "SD:IMPORT/Kit/snare.wav", Name: "snare"},
		"2/C1": {Path: "SD:IMPORT/Kit/hat.wav", Name: "hat"},
		"2/C2": {Path: "SD:IMPORT/Other/hat.wav", Name: "hat"},
	}}
	got := ansi.Strip(strings.Join(m.folderPadLines("SD:IMPORT/Kit", 80), "\n"))
	for _, want := range []string{"Project 1: 2 pads  A5 B2", "Project 2: 1 pad  C1"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "Other") || len(m.folderPadLines("SD:IMPORT/None", 80)) != 0 {
		t.Error("pads from other folders are counted")
	}
}

func TestSlicerSoundKeysChangeEveryMarkedSlice(t *testing.T) {
	s := slicerOpen(t)
	// Slices 2 and 4 marked: the sound row changes both, and only them.
	s.press("l", "s", "l", "l", "s")
	s.shows("2 slices marked", "change all of them")
	s.press("-", "-", "<", "7", "7", "m")
	for i, x := range s.slicing().Slices {
		marked := i == 1 || i == 3
		if marked && (x.Gain != 80 || x.Pitch != -1 || x.Attack == 0 || x.Chance != 0) {
			t.Errorf("slice %d: gain %d pitch %d attack %d chance %d", i+1, x.Gain, x.Pitch, x.Attack, x.Chance)
		}
		if !marked && (x.Gain != 100 || x.Pitch != 0 || x.Attack != 0 || x.Chance != 100) {
			t.Errorf("slice %d was changed: %+v", i+1, x)
		}
	}
	s.press("X")
	if x := s.slicing().Slices[3]; x.Gain != 100 || x.Chance != 100 || x.Attack != 0 {
		t.Errorf("X left %+v on a marked slice", x)
	}
	s.press("u")
	if s.slicing().Slices[3].Gain != 80 {
		t.Error("u did not undo the plain reset of all the marked slices in one step")
	}
}

func TestSlicerSlashCutsTheSliceInFrontInPartsAndUndoes(t *testing.T) {
	s := slicerOpen(t)
	s.press("l")
	before, start, end := len(s.slicing().Slices), s.slicing().Slices[1].Start, s.slicing().Slices[1].End
	s.press("/")
	if got := len(s.slicing().Slices); got != before+1 {
		t.Fatalf("/ made %d slices from %d, want one more", got, before)
	}
	s.shows("slice cut in 2 parts", "/ again: 3")
	s.press("/")
	if got := len(s.slicing().Slices); got != before+3 {
		t.Errorf("the second / cut the left part in 3: %d slices, want %d", got, before+3)
	}
	first, last := s.slicing().Slices[1], s.slicing().Slices[1]
	for _, x := range s.slicing().Slices {
		if x.Start >= start && x.End <= end {
			first.Start, last.End = min(first.Start, x.Start), max(last.End, x.End)
		}
	}
	if first.Start != start || last.End != end {
		t.Errorf("the parts cover %d-%d, the slice was %d-%d", first.Start, last.End, start, end)
	}
	s.press("u", "u")
	if len(s.slicing().Slices) != before {
		t.Errorf("two undos left %d slices, want %d", len(s.slicing().Slices), before)
	}
}

func TestMarkedSlicesHaveTheGroundOfTheSliceInFront(t *testing.T) {
	s := slicerOpen(t)
	m := s.m()
	plain := m.slicerStyle(cellStyle{kind: cellSlice, slice: 3})
	marked := m.slicerStyle(cellStyle{kind: cellSlice, slice: 3, marked: true})
	front := m.slicerStyle(cellStyle{kind: cellSlice, slice: 3, selected: true})
	if marked.GetBackground() != front.GetBackground() || marked.GetBackground() == plain.GetBackground() {
		t.Errorf("grounds: plain %v, marked %v, in front %v", plain.GetBackground(), marked.GetBackground(), front.GetBackground())
	}
}
