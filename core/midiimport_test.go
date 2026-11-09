package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// A drum track and a bass line in one file: the form opens on the bass, the drums are one choice away, and both land in patterns.
func TestMIDIImportFromFiles(t *testing.T) {
	a, _ := newApp(t)
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
	if err := os.WriteFile(filepath.Join(a.dir, "groove.mid"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	a.list()
	found := false
	for i, e := range a.entries {
		if e.name == "groove.mid" {
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
	if m.items[0].val != 1 || m.items[1].val != 0 { // the bass is the second part, and a melody
		t.Fatalf("the form starts on part %d as %d", m.items[0].val, m.items[1].val)
	}
	into := m.items[4].val + 1
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
	// the drums: pick the first part, as drums
	a.filesKey(A)
	m = a.menu
	m.items[0].val = 0
	m.items[0].onChange(0)
	if m.items[1].val != 1 {
		t.Fatal("the drum part is made as drums by itself")
	}
	m.items[len(m.items)-1].run(0)
	d, ok = a.doc()
	if !ok || len(d.Notes) != 16 {
		t.Fatalf("the drums: %d notes (%s)", len(d.Notes), a.msg)
	}
	pads := map[int]bool{}
	for _, n := range d.Notes {
		pads[n.Pad] = true
	}
	if len(pads) != 3 {
		t.Fatalf("kick, snare and hat go to three pads, got %d (%s)", len(pads), a.msg)
	}
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
	if err != nil || len(g.Parts) != 3 {
		t.Fatalf("exported file: %v, %d parts", err, len(g.Parts))
	}
	// a file that is not MIDI says so and opens nothing
	a.menu = nil
	a.openMIDIImport("x.mid", []byte("hello"))
	if a.menu != nil || !a.msgBad {
		t.Fatal("a text file is not a MIDI file")
	}
}
