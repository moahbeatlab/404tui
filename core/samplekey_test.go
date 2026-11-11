package core

import (
	"fmt"
	"math"
	"strings"
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
	// a bounced chord is named after it
	if chordSymbol(0) != "" || chordSymbol(1) != "m" || chordSymbol(11) != "m7" || chordSymbol(14) != "m7b5" {
		t.Fatalf("symbols: %q %q %q %q", chordSymbol(0), chordSymbol(1), chordSymbol(11), chordSymbol(14))
	}
	a.menu = nil
	a.openOnePadMenu(0, true)
	root := menuItemNamed(t, a, "Root")
	root.val = 9
	root.onChange(9)
	ch := menuItemNamed(t, a, "Chord")
	ch.val = 11
	ch.onChange(11)
	chooseMenu(t, a, "Bounce the chord to a new pad")
	named := false
	for _, p := range a.info.Pads {
		named = named || strings.HasPrefix(p.Name, "Am7_")
	}
	if !named {
		t.Fatalf("the bounced chord is named Am7_...: %s", a.Message())
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

func TestChordProgressions(t *testing.T) {
	// the anthem in A minor from a sample in C: Am F C G, each root within a fifth of A
	o := onePad{key: 9, oct: 3, prog: 0}
	if got := fmt.Sprint(progSemis(o)); got != "[[9 12 16] [5 9 12] [12 16 19] [7 11 14]]" {
		t.Fatalf("Am F C G: %s", got)
	}
	if got := progNames(0, 9, 0); fmt.Sprint(got) != "[Am F C G]" {
		t.Fatalf("names: %v", got)
	}
	if got := progNames(12, 0, 1); fmt.Sprint(got) != "[Dm7 G7 Cmaj7 Am7]" {
		t.Fatalf("sevenths: %v", got)
	}
	if got := progNames(8, 9, 0); fmt.Sprint(got) != "[A E F#m D]" {
		t.Fatalf("A major is written with sharps: %v", got)
	}
	if got := progNames(0, 4, 0); fmt.Sprint(got) != "[Em C G D]" {
		t.Fatalf("E minor: %v", got)
	}
	if got := progNames(0, 6, 0); fmt.Sprint(got) != "[F#m D A E]" {
		t.Fatalf("F# minor: %v", got)
	}
	if got := progNames(0, 0, 0); fmt.Sprint(got) != "[Cm Ab Eb Bb]" {
		t.Fatalf("C minor keeps flats: %v", got)
	}
	if got := progNames(0, 9, 2); fmt.Sprint(got) != "[Am9 Fmaj9 Cmaj9 Gmaj9]" {
		t.Fatalf("ninths: %v", got)
	}
	if got := progNames(3, 9, 3); fmt.Sprint(got) != "[Am11 Dm11 Fmaj9#11 E11]" {
		t.Fatalf("elevenths: %v", got)
	}
	for i := range progressions {
		for _, s := range progressions[i].steps {
			for color := range progColors {
				if c, _ := progChord(s.q, color); c == 0 && (color > 0 || s.q != 'M' && s.q != 'D') {
					t.Fatalf("%s: no chord type for %c", progressions[i].name, s.q)
				}
			}
		}
	}
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Pads)
	a.openOnePadMenu(0, true)
	it := menuItemNamed(t, a, "Root")
	it.val = 9
	it.onChange(9)
	if !strings.HasSuffix(menuItemNamed(t, a, "Progression").vals[0], "Am F C G") {
		t.Fatalf("the progressions follow the root: %q", menuItemNamed(t, a, "Progression").vals[0])
	}
	au.Played = 0
	chooseMenu(t, a, "Hear the progression")
	if au.Played != 1 || !strings.Contains(a.Message(), "Am F C G") {
		t.Fatalf("heard once: %d (%s)", au.Played, a.Message())
	}
	before := padsWithSamples(a)
	chooseMenu(t, a, "Bounce the progression to free pads") // and A in the pad dialog
	if padsWithSamples(a) != before+4 {
		t.Fatalf("4 chords on 4 pads: %d -> %d (%s)", before, padsWithSamples(a), a.Message())
	}
	if a.menu == nil || a.menu.title != "chord from A1" {
		t.Fatalf("the chord tool is back: %+v", a.menu)
	}
	found := 0
	for _, p := range a.info.Pads {
		if strings.HasPrefix(p.Name, "Am_") || strings.HasPrefix(p.Name, "F_") || strings.HasPrefix(p.Name, "C_") || strings.HasPrefix(p.Name, "G_") {
			found++
		}
	}
	if found != 4 {
		t.Fatalf("the pads are named after their chords: %d", found)
	}
}

func TestSpacePlaysTheProgressionOnItsRows(t *testing.T) {
	termOn(t)
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Pads)
	a.openOnePadMenu(0, true)
	hold := menuItemNamed(t, a, "Hold")
	hold.val = 1
	hold.onChange(1)
	keys(t, a, "X") // Space on the chord rows: the chord loops
	chordLoop := au.Loops
	if !a.arpOn || a.chordp.progOn {
		t.Fatalf("the chord loops: %v %v", a.arpOn, a.chordp.progOn)
	}
	if !seekMenu(t, a, func(l string) bool { return l == "Progression" }) {
		t.Fatal("no Progression row")
	}
	if !strings.Contains(screenStr(a), "loop progression") {
		t.Fatal("on the progression rows the hints say Space plays the progression")
	}
	keys(t, a, "X") // Space there: the progression takes the chord's place
	if !a.arpOn || !a.chordp.progOn || au.Loops != chordLoop+1 {
		t.Fatalf("the progression loops instead: on %v prog %v loops %d", a.arpOn, a.chordp.progOn, au.Loops)
	}
	keys(t, a, "X")
	if a.arpOn {
		t.Fatal("Space again stops it")
	}
	// from the chord loop, "Hear the progression" switches too
	keys(t, a, "U U U U U U") // back up to the chord rows
	keys(t, a, "X")
	chooseMenu(t, a, "Hear the progression")
	if !a.arpOn || !a.chordp.progOn {
		t.Fatalf("Hear the progression switches the loop: on %v prog %v", a.arpOn, a.chordp.progOn)
	}
}
