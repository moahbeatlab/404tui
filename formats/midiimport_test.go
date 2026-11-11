package formats

import (
	"testing"

	"github.com/moahbeatlab/404tui/proto"
)

func padAt(t *testing.T, i int) proto.PadIndex {
	t.Helper()
	p, ok := proto.NewPadIndex(uint16(i))
	if !ok {
		t.Fatalf("no pad %d", i)
	}
	return p
}

// A file written by hand: running status, a note on with velocity 0 for a note off, a tempo, a 3/4 time signature, a track name.
func TestParseMIDIByHand(t *testing.T) {
	trk := []byte{
		0, 0xFF, 0x03, 4, 'B', 'a', 's', 's',
		0, 0xFF, 0x51, 3, 0x07, 0xA1, 0x20, // 500000 us: 120 BPM
		0, 0xFF, 0x58, 4, 3, 2, 24, 8, // 3/4
		0, 0x91, 40, 100, // note on, channel 2
		0x60, 40, 0, // running status: the same note, velocity 0 = off, 96 ticks later
		0x30, 43, 90, // running status: another note on
		0x60, 0x81, 43, 0, // note off
		0, 0xFF, 0x2F, 0,
	}
	b := []byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x60")
	b = append(b, "MTrk"...)
	b = append(b, 0, 0, 0, byte(len(trk)))
	b = append(b, trk...)
	f, err := ParseMIDI(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.PPQ != 96 || f.Beats != 3 || f.BeatUnit != 4 || int(f.Tempo+0.5) != 120 {
		t.Fatalf("header: %+v", f)
	}
	if len(f.Parts) != 1 || f.Parts[0].Channel != 1 || f.Parts[0].Name != "Bass" || len(f.Parts[0].Notes) != 2 {
		t.Fatalf("parts: %+v", f.Parts)
	}
	n := f.Parts[0].Notes
	if n[0].Tick != 0 || n[0].Len != 0x60 || n[0].Note != 40 || n[0].Vel != 100 {
		t.Errorf("first note: %+v", n[0])
	}
	if n[1].Tick != 0x60+0x30 || n[1].Len != 0x60 {
		t.Errorf("second note: %+v", n[1])
	}
}

func TestParseMIDIRejects(t *testing.T) {
	if _, err := ParseMIDI([]byte("not a midi file at all")); err == nil {
		t.Error("text accepted")
	}
	smpte := []byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\xE7\x28")
	if _, err := ParseMIDI(smpte); err == nil {
		t.Error("time code accepted")
	}
	empty := WriteMIDI([]MIDIOut{{Name: "x"}}, 120, 4, 1920)
	if _, err := ParseMIDI(empty); err == nil {
		t.Error("a file with no notes accepted")
	}
}

func TestMelodyImport(t *testing.T) {
	// a bass line over three octaves, 480 ticks per beat, two bars of 4/4
	notes := []MIDINote{{0, 240, 36, 100}, {480, 240, 43, 90}, {960, 240, 48, 80}, {1440, 240, 60, 70}, {1920, 240, 36, 100}}
	b := WriteMIDI([]MIDIOut{{Name: "bass", Channel: 0, Notes: notes}}, 124, 4, 3840)
	f, err := ParseMIDI(b)
	if err != nil {
		t.Fatal(err)
	}
	if int(f.Tempo+0.5) != 124 || f.MIDIBars(0) != 2 {
		t.Fatalf("tempo %.1f bars %d", f.Tempo, f.MIDIBars(0))
	}
	root := SuggestRoot(f.Parts[0]) // 48
	if root != 48 {
		t.Fatalf("root %d", root)
	}
	res, err := BuildFromMIDI(f, MIDIImport{Part: 0, Pad: padAt(t, 13), Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Bars != 2 || res.Notes != 5 || res.Folded != 0 {
		t.Fatalf("result %+v", res)
	}
	p, err := ParsePattern(res.Pattern)
	if err != nil {
		t.Fatal(err)
	}
	if p.Length != 3840 || len(p.Events) != 5 {
		t.Fatalf("pattern %d ticks, %d events", p.Length, len(p.Events))
	}
	want := []int{-12, -5, 0, 12, -12}
	for i, e := range p.Events {
		if e.Pad.Index() != 13 || PitchSemitones(e.Pitch) != want[i] || e.Pitch == 0 {
			t.Errorf("event %d: pad %d pitch %#x (%d st), want %d", i, e.Pad.Index(), e.Pitch, PitchSemitones(e.Pitch), want[i])
		}
	}
	// a root far from the notes folds them by octaves into the range
	res, err = BuildFromMIDI(f, MIDIImport{Part: 0, Pad: padAt(t, 0), Root: 72})
	if err != nil || res.Folded == 0 {
		t.Fatalf("folded %d err %v", res.Folded, err)
	}
	p, _ = ParsePattern(res.Pattern)
	for _, e := range p.Events {
		if s := PitchSemitones(e.Pitch); s < -PitchRange || s > PitchRange {
			t.Errorf("pitch %d out of range", s)
		}
	}
}

func TestDrumImportAndTicks(t *testing.T) {
	// a PPQ of 96: a beat is 96 ticks, so a note on beat 2 is tick 480 of a pattern
	trk := []byte{
		0, 0x99, 36, 110, 10, 0x99, 36, 0, // kick on, off
		86, 0x99, 38, 100, 10, 0x99, 38, 0, // snare on beat 2 (96 ticks later)
		0, 0x99, 51, 90, 10, 0x99, 51, 0, // a ride at the same tick, with no pad
		0, 0xFF, 0x2F, 0,
	}
	b := []byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x60MTrk")
	b = append(b, 0, 0, 0, byte(len(trk)))
	b = append(b, trk...)
	f, err := ParseMIDI(b)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Parts[0].Drums() || len(f.Parts[0].Notes) != 3 {
		t.Fatalf("part %+v", f.Parts[0])
	}
	res, err := BuildFromMIDI(f, MIDIImport{Part: 0, Drums: map[uint8]proto.PadIndex{36: padAt(t, 0), 38: padAt(t, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Notes != 2 || res.Left != 1 || res.Bars != 1 {
		t.Fatalf("result %+v", res)
	}
	p, _ := ParsePattern(res.Pattern)
	if p.Events[1].Tick != 480 || p.Events[1].Pad.Index() != 1 || p.Events[1].Pitch != 0 || p.Events[1].Length != 50 {
		t.Errorf("snare %+v", p.Events[1])
	}
}

func TestExportThenImport(t *testing.T) {
	notes := []MIDINote{{0, 120, 60, 100}, {480, 120, 67, 90}}
	b := WriteMIDI([]MIDIOut{{Name: "x", Notes: notes}}, 120, 4, 1920)
	f, err := ParseMIDI(b)
	if err != nil {
		t.Fatal(err)
	}
	res, err := BuildFromMIDI(f, MIDIImport{Pad: padAt(t, 20), Root: 60})
	if err != nil {
		t.Fatal(err)
	}
	out, err := PatternToMIDI(res.Pattern, 120)
	if err != nil {
		t.Fatal(err)
	}
	g, err := ParseMIDI(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Parts) != 1 || g.Parts[0].Drums() || g.Parts[0].Name != padAt(t, 20).String()+" keys" {
		t.Fatalf("parts %+v", g.Parts)
	}
	for i, n := range g.Parts[0].Notes {
		if n.Note != notes[i].Note || n.Tick != notes[i].Tick || n.Len != notes[i].Len {
			t.Errorf("note %d: %+v, was %+v", i, n, notes[i])
		}
	}
	if int(g.Tempo+0.5) != 120 || g.MIDIBars(0) != 1 {
		t.Errorf("tempo %.1f bars %d", g.Tempo, g.MIDIBars(0))
	}
}

func TestOddMeterTrailer(t *testing.T) {
	for _, beats := range []int{1, 2, 3, 4, 5, 6, 7} {
		p, err := ParsePattern(writeRecords([]patRecord{noteRecord(0, proto.PadIndex{}, 100, 10)}, uint32(beats*PPQ), midiTrailer(1, beats)))
		if err != nil || p.BeatsPerBar() != beats {
			t.Errorf("%d beats read back as %d (%v)", beats, p.BeatsPerBar(), err)
		}
	}
}

func TestMergePatterns(t *testing.T) {
	two := func(pad int, bars int, step int) []byte {
		var recs []patRecord
		for tick := 0; tick < bars*4*PPQ; tick += step {
			recs = append(recs, noteRecord(uint32(tick), padAt(t, pad), 100, 240))
		}
		return writeRecords(recs, uint32(bars*4*PPQ), startTrailer(bars))
	}
	kick := two(0, 2, 480)   // 8 notes in 2 bars
	melody := two(5, 8, 960) // 16 notes in 8 bars
	res, err := MergePatterns(kick, melody, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Bars != 8 || res.WasBars != 2 || !res.Repeated || res.Notes != 8*4+16 {
		t.Fatalf("repeat: %+v", res)
	}
	p, err := ParsePattern(res.Pattern)
	if err != nil || p.Length != 8*4*PPQ || len(p.Events) != 48 {
		t.Fatalf("pattern: %v, %d ticks, %d events", err, p.Length, len(p.Events))
	}
	if p.Trailer[0] != 8 {
		t.Errorf("the trailer says %d bars", p.Trailer[0])
	}
	res, err = MergePatterns(kick, melody, false)
	if err != nil || res.Notes != 8+16 || res.Bars != 8 {
		t.Fatalf("once: %+v %v", res, err)
	}
	// a part that is longer than the pattern makes it longer; a 3-bar part in 8 bars is cut at the end of the last round
	three := two(7, 3, 480)
	res, err = MergePatterns(melody, three, true)
	if err != nil || res.Bars != 8 {
		t.Fatalf("%+v %v", res, err)
	}
	p, _ = ParsePattern(res.Pattern)
	for _, e := range p.Events {
		if uint32(e.Tick) >= p.Length || uint32(e.Tick)+uint32(e.Length) > p.Length+0 && e.Pad.Index() == 7 {
			t.Errorf("a note reaches past the end: tick %d length %d of %d", e.Tick, e.Length, p.Length)
		}
	}
}
