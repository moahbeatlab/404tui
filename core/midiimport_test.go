package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// A drum track and a bass line in one file: the form opens on the bass, the drums are one choice away, and both land in patterns.
func TestMIDIImportFromFiles(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit) // kick, snare and a closed hat on A1..A3
	a.go_(Seq)
	drums := formats.MIDIOut{Name: "drums", Channel: 9}
	for bar := uint32(0); bar < 2; bar++ {
		for beat := uint32(0); beat < 4; beat++ {
			at := (bar*4 + beat) * formats.PPQ
			drums.Notes = append(drums.Notes, formats.MIDINote{Tick: at, Len: 60, Note: 42, Vel: 80})
			if beat%2 == 0 {
				drums.Notes = append(drums.Notes, formats.MIDINote{Tick: at, Len: 60, Note: 36, Vel: 110})
			} else {
				drums.Notes = append(drums.Notes, formats.MIDINote{Tick: at, Len: 60, Note: 38, Vel: 100})
			}
		}
	}
	bass := formats.MIDIOut{Name: "bass", Channel: 0}
	for i, n := range []uint8{36, 43, 48, 41, 36, 43, 48, 41} {
		bass.Notes = append(bass.Notes, formats.MIDINote{Tick: uint32(i) * formats.PPQ, Len: 240, Note: n, Vel: 100})
	}
	file := formats.WriteMIDI([]formats.MIDIOut{drums, bass}, 124, 4, 8*formats.PPQ)
	if err := os.WriteFile(filepath.Join(a.dir, "Groove.MIDI"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	a.list()
	found := false
	for i, e := range a.entries {
		if e.name == "Groove.MIDI" {
			a.fcur, found = i, true
		}
	}
	if !found {
		t.Fatal("a .mid file is not in the list of FILES")
	}
	a.filesKey(A)
	if a.menu == nil || !strings.HasPrefix(a.menu.title, "MIDI file") {
		t.Fatalf("A on a MIDI file opens the form, got %v (%s)", a.menu, a.msg)
	}
	m := a.menu
	if v := m.items[2].vals[0]; !strings.HasPrefix(v, "A1  kick") || !strings.Contains(m.items[2].vals[19], "no sample") {
		t.Fatalf("Use sample lists the pad with the name of its sample: %q, %q", v, m.items[2].vals[19])
	}
	if m.items[0].val != 1 || m.items[1].val != 0 { // the bass is the second part, and a melody
		t.Fatalf("the form starts on part %d as %d", m.items[0].val, m.items[1].val)
	}
	into := m.items[4].val + 1
	// listening comes first: the pad of the melody (A1) has a sample, so the loop plays, the pattern is not written, and a change of the form goes into the loop
	listen := m.items[len(m.items)-2]
	if !strings.HasPrefix(listen.label, "Play") {
		t.Fatalf("the entry before Import is %q", listen.label)
	}
	loops := au.Loops
	listen.run(0)
	if au.Loops != loops+1 {
		t.Fatalf("the form did not play: %s", a.msg)
	}
	if _, ok := a.lab.Pattern(into); ok == nil {
		t.Fatal("listening wrote the pattern")
	}
	m.items[3].val = (m.items[3].val + 5) % 128
	m.items[3].onChange(m.items[3].val)
	if au.Loops != loops+2 {
		t.Fatal("a change of the form is not heard while it plays")
	}
	listen.run(0)       // again: it stops
	m.items[2].val = 20 // the bass is on pad B5, not on the kick's pad
	m.items[2].onChange(20)
	m.items[len(m.items)-1].run(0)
	d, ok := a.doc()
	if !ok || a.pat != into || len(d.Notes) != 8 || d.Bars != 2 {
		t.Fatalf("the bass: pattern %d (into %d), %d notes in %d bars (%s)", a.pat, into, len(d.Notes), d.Bars, a.msg)
	}
	for _, n := range d.Notes {
		if n.Pitch == 0 {
			t.Fatal("a melody note has no pitch")
		}
	}
	// the drums: the form starts on the pattern of the last import, adding to it
	a.filesKey(A)
	m = a.menu
	if m.items[4].val+1 != into || m.items[5].val != 1 {
		t.Fatalf("the form should remember pattern %d and add to it: pattern %d, mode %d", into, m.items[4].val+1, m.items[5].val)
	}
	if m.items[2].val != 20 {
		t.Fatalf("the form should remember the sample of the last melody (pad 20), it has %d", m.items[2].val)
	}
	m.items[2].run(0) // A on "Use sample" opens the pad dialog
	if a.picker == nil || !a.picker.pads {
		t.Fatal("A on Use sample did not open the pad dialog")
	}
	a.picker.onPick(1)
	a.picker = nil
	if m.items[2].val != 1 {
		t.Fatalf("the pad that was picked is not in the form: %d", m.items[2].val)
	}
	m.items[0].val = 0
	m.items[0].onChange(0)
	if m.items[1].val != 1 {
		t.Fatal("the drum part is made as drums by itself")
	}
	m.items[len(m.items)-1].run(0)
	d, ok = a.doc()
	if !ok || a.pat != into || len(d.Notes) != 8+16 || d.Bars != 2 {
		t.Fatalf("the drums added to the bass: %d notes in %d bars (%s)", len(d.Notes), d.Bars, a.msg)
	}
	pads := map[int]bool{}
	for _, n := range d.Notes {
		pads[n.Pad] = true
	}
	if len(pads) != 4 {
		t.Fatalf("the bass and kick, snare and hat are four pads, got %d (%s)", len(pads), a.msg)
	}
	// the mode that was chosen last is the one the next form starts on
	a.filesKey(A)
	m = a.menu
	m.items[5].val = 2
	m.items[5].onChange(2)
	m.items[len(m.items)-1].run(0)
	a.filesKey(A)
	if a.menu.items[5].val != 2 {
		t.Fatalf("the form should start on the mode that was used last (2), it has %d", a.menu.items[5].val)
	}
	a.menuKey(B)
	// and back out as a file
	a.exportMIDI()
	if !strings.HasPrefix(a.msg, "written: ") {
		t.Fatal(a.msg)
	}
	out, err := os.ReadFile(strings.TrimPrefix(a.msg, "written: "))
	if err != nil {
		t.Fatal(err)
	}
	g, err := formats.ParseMIDI(out)
	if err != nil || len(g.Parts) < 4 {
		t.Fatalf("exported file: %v, %d parts", err, len(g.Parts))
	}
	// a file that is not MIDI says so and opens nothing
	a.menu = nil
	a.openMIDIImport("x.mid", []byte("hello"))
	if a.menu != nil || !a.msgBad {
		t.Fatal("a text file is not a MIDI file")
	}
}

// A song in several files: marked, they are imported one after the other into one pattern, each on a pad of its own, and the pattern
// gets as long as the longest file while the shorter ones go round again.
func TestMIDIImportOfSeveralFilesIntoOnePattern(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	write := func(name string, ch uint8, bars int, notes ...uint8) {
		tr := formats.MIDIOut{Name: name, Channel: ch}
		for i := 0; i < bars*4; i++ {
			tr.Notes = append(tr.Notes, formats.MIDINote{Tick: uint32(i) * formats.PPQ, Len: 120, Note: notes[i%len(notes)], Vel: 100})
		}
		b := formats.WriteMIDI([]formats.MIDIOut{tr}, 120, 4, uint32(bars*4*formats.PPQ))
		if err := os.WriteFile(filepath.Join(a.dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("song_1_drums.mid", 9, 2, 36, 38)
	write("song_2_bass.mid", 0, 4, 36, 43, 48)
	write("song_3_keys.midi", 1, 8, 60, 64, 67)
	a.list()
	a.fmarks = map[string]bool{}
	for i, e := range a.entries {
		if strings.HasPrefix(e.name, "song_") {
			a.fmarks[joinPath(a.dir, e.name)] = true
			a.fcur = i
		}
	}
	a.filesKey(A)
	if a.menu == nil || !strings.Contains(a.menu.title, "1 of 3") {
		t.Fatalf("the first form of three: %v (%s)", a.menu, a.msg)
	}
	into := a.menu.items[4].val + 1
	var pads []int
	for n := 1; n <= 3; n++ {
		m := a.menu
		if m == nil || !strings.Contains(m.title, fmt.Sprintf("%d of 3", n)) {
			t.Fatalf("form %d: %v (%s)", n, m, a.msg)
		}
		if n > 1 && (m.items[4].val+1 != into || m.items[5].val != 1) {
			t.Fatalf("form %d goes into pattern %d added: %d, mode %d", n, into, m.items[4].val+1, m.items[5].val)
		}
		if n > 1 {
			pads = append(pads, m.items[2].val)
		}
		m.cur = len(m.items) - 2 // Import the pattern (the last entry is "leave this file out")
		a.menuKey(A)
	}
	if a.menu != nil || a.midiRun != nil {
		t.Fatal("the run is over after the third file")
	}
	d, ok := a.doc()
	if !ok || a.pat != into || d.Bars != 8 {
		t.Fatalf("pattern %d, %d bars (%s)", a.pat, d.Bars, a.msg)
	}
	// the drums (2 bars, 8 notes) go round four times, the bass (4 bars, 16 notes) twice, the keys (8 bars, 32 notes) once
	if want := 8*4 + 16*2 + 32; len(d.Notes) != want {
		t.Fatalf("%d notes, want %d", len(d.Notes), want)
	}
	if len(pads) != 2 || pads[0] == pads[1] {
		t.Fatalf("each melody is on a pad of its own: %v", pads)
	}
	// B ends a run
	a.filesKey(A)
	a.menuKey(B)
	if a.midiRun != nil {
		t.Fatal("B did not end the run")
	}
}

// The sample that was chosen is the one the next form starts on, even when the form was closed without importing.
func TestMIDIImportRemembersTheSample(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	file := formats.WriteMIDI([]formats.MIDIOut{{Name: "bass", Notes: []formats.MIDINote{{Tick: 0, Len: 100, Note: 40, Vel: 100}}}}, 120, 4, 4*formats.PPQ)
	a.openMIDIImport("x.mid", file)
	m := a.menu
	m.items[2].val = 2
	m.items[2].onChange(2)
	a.menuKey(B)
	a.openMIDIImport("y.mid", file)
	if a.menu.items[2].val != 2 {
		t.Fatalf("Use sample did not keep pad 2: %d", a.menu.items[2].val)
	}
	a.menu.items[2].run(0)
	a.picker.onPick(7)
	a.picker = nil
	a.menuKey(B)
	a.openMIDIImport("z.mid", file)
	if a.menu.items[2].val != 7 {
		t.Fatalf("the pad of the dialog was not kept: %d", a.menu.items[2].val)
	}
}
