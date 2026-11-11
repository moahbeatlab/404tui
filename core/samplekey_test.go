package core

import (
	"fmt"
	"math"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

func TestNoteFromName(t *testing.T) {
	for name, want := range map[string]int{"bass_G1": 7, "Pad 3 Am": 9, "stab-F#": 6, "lead Bb": 10, "chord_Ebmin": 3, "C3 piano": 0} {
		if got, ok := noteFromName(name); !ok || got != want {
			t.Errorf("%s: %d %v, want %d", name, got, ok, want)
		}
	}
	for _, name := range []string{"kick", "a loop", "Snare 2", "hat b"} {
		if got, ok := noteFromName(name); ok {
			t.Errorf("%s names no note, got %d", name, got)
		}
	}
}

func tone(f float64, secs float64) formats.Sample {
	n := int(secs * formats.SampleRate)
	s := formats.Sample{Channels: 1, Samples: make([]int16, n)}
	for i := range s.Samples {
		x := math.Sin(2*math.Pi*f*float64(i)/formats.SampleRate) + 0.4*math.Sin(4*math.Pi*f*float64(i)/formats.SampleRate)
		s.Samples[i] = int16(9000 * x)
	}
	return s
}

func TestNoteFromSound(t *testing.T) {
	for _, c := range []struct {
		f    float64
		want int
	}{{196, 7}, {55, 9}, {261.63, 0}, {440, 9}, {92.5, 6}} {
		if got, ok := noteFromSound(tone(c.f, 0.5)); !ok || got != c.want {
			t.Errorf("%.1f Hz: %d %v, want %d", c.f, got, ok, c.want)
		}
	}
	noise := formats.Sample{Channels: 1, Samples: make([]int16, formats.SampleRate/2)}
	seed := uint32(1)
	for i := range noise.Samples {
		seed = seed*1664525 + 1013904223
		noise.Samples[i] = int16(seed >> 17)
	}
	if got, ok := noteFromSound(noise); ok {
		t.Errorf("noise has no note, got %d", got)
	}
}

func TestOnePadFollowsTheNoteOfTheSample(t *testing.T) {
	// a sample in G, the triad of C major one octave: from G up to C is 5 semitones
	o := onePad{scale: 1, stack: 3, oct: 3, from: 7}
	if got := fmt.Sprint(onePadSemis(o)); got != "[5 9 12]" {
		t.Fatalf("C triad from a G sample: %s", got)
	}
	a, _ := newApp(t)
	keys(t, a, kit)
	a.info.Pads[0].Name = "bass_G1"
	a.openOnePadMenu(0, true)
	if a.chordp.from != 7 || menuItemNamed(t, a, "Sample is in").val != 7 {
		t.Fatalf("the name says G: %d (%s)", a.chordp.from, a.Message())
	}
	it := menuItemNamed(t, a, "Sample is in")
	it.val = 2
	it.onChange(2)
	a.menu = nil
	a.openOnePadMenu(0, false)
	if a.onepad.from != 2 {
		t.Fatalf("the note chosen for the pad is kept, for the arp too: %d", a.onepad.from)
	}
	// every chord type of the chord tool
	a.menu = nil
	a.openOnePadMenu(0, true)
	if got := len(menuItemNamed(t, a, "Chord").vals); got != len(chordTypes) || got < 25 {
		t.Fatalf("the chord tool lists %d chords", got)
	}
	for _, it := range a.menu.items {
		if it.label == "Scale" {
			t.Fatal("the chord tool has no scale")
		}
	}
}
