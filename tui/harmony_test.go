package tui

import (
	"math"
	"slices"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

func TestHarmonyMakesAnArpOfAPadAndPutsItOnAFreePad(t *testing.T) {
	s := open(t)
	m := s.m()
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "j", "A")
	s.shows("Chords & arps  ·  A5", "SAMPLE KEY", "PATTERN", "Space play", "Enter put on a pad")
	h := s.m().harmony
	if h == nil || !slices.Equal(h.arp.Notes, []int{0, 4, 7}) {
		t.Fatalf("the dialog opens on a C major chord, got %+v", h)
	}
	// Space plays the arp on this computer: one bar of sixteenths at the bank's tempo.
	s.press(" ")
	if len(s.player.played) != 1 {
		t.Fatalf("played %d samples", len(s.player.played))
	}
	want := 16 * 60 / h.arp.BPM / 4 * formats.SampleRate
	if got := s.player.played[0].Frames(); math.Abs(float64(got)-want) > 2 {
		t.Errorf("the arp is %d frames, want %.0f", got, want)
	}
	// ↓ to ROOT, → moves it to Db: the notes follow.
	s.press("down", "right")
	if got := s.m().harmony.arp.Notes; !slices.Equal(got, []int{1, 5, 8}) {
		t.Errorf("a Db major chord is %v", got)
	}
	// The home row switches notes: a is C (off then on), and a note by hand makes the chord custom.
	s.press("a")
	if got := s.m().harmony.arp.Notes; !slices.Equal(got, []int{1, 5, 8, 0}) || !s.m().harmony.custom {
		t.Errorf("notes after a: %v", got)
	}
	s.press("a", "a")
	s.press("enter")
	s.shows("Put the chords on a pad", "UP, 8 notes")
	if s.m().harmony != nil {
		t.Error("the dialog stays open behind the picker")
	}
	s.press("enter")
	s.shows("put ", "chords on A4")
	if got := s.padName("A4"); got != "loop custom UP" {
		t.Errorf("A4 is %q", got)
	}
	// The pad holds the arp: the same length as what was played, close to the bar.
	got, err := app.PadAudio(s.m().unit, s.m().snap.Bank(0)[3])
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(got.Frames())-want) > 2 {
		t.Errorf("the pad holds %d frames", got.Frames())
	}
	// The dialog comes back as it was left.
	s.press("A")
	if h := s.m().harmony; h == nil || h.field != 1 || len(h.arp.Notes) != 4 {
		t.Errorf("settings not remembered: %+v", h)
	}
}

func TestHarmonyOnAnEmptyPadAndWithoutNotes(t *testing.T) {
	s := open(t)
	m := s.m()
	s.press("r", "j", "j", "A")
	s.shows("A9 is empty")
	s.sim.AddFile("SD:IMPORT/loop.wav", wav(4800))
	if _, err := app.Assign(m.unit, m.unit, pad(t, "A5"), "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	s.press("r", "k", "A")
	s.shows("Chords & arps")
	// With every note off there is nothing to play or put.
	s.press("a", "d", "g")
	s.press(" ")
	s.shows("no notes")
	s.press("enter")
	if s.m().picker != nil {
		t.Error("a picker opened for nothing")
	}
	s.press("esc")
	if s.m().harmony != nil {
		t.Error("Esc did not close the dialog")
	}
}
