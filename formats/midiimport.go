package formats

import (
	"errors"
	"fmt"
	"sort"

	"github.com/moahbeatlab/404tui/proto"
)

// A part of a MIDI file becomes a pattern in one of two ways:
//
//   - drums: each note number is a drum (the General MIDI drum map: 36 kick, 38 snare, 42 hat...) and goes to a pad of its own, as a
//     plain note;
//   - a melody: every note goes to one pad as a chromatic note. The pad plays its sample at its own pitch for the note Root and a
//     note up to 12 semitones higher or lower for the others, which is as far as a pattern note can be moved; a note outside that is
//     moved by whole octaves into it.

// MIDIImport says how to turn one part of a file into a pattern.
type MIDIImport struct {
	Part int
	// Drums: the pad of each note number; a note with no pad is left out.
	Drums map[uint8]proto.PadIndex
	// A melody (Drums is nil): the pad, and the note that plays at the pad's own pitch.
	Pad  proto.PadIndex
	Root int
	// Bars is the length of the pattern; 0 takes it from the file (rounded up to whole bars).
	Bars int
}

// MIDIResult is the pattern and what was done to make it.
type MIDIResult struct {
	Pattern []byte
	Bars    int
	Notes   int          // notes in the pattern
	Folded  int          // notes moved by octaves to fit the 12 semitones up and down of a pad
	Left    int          // notes left out: no pad for the drum, or after the end of the pattern
	Chords  int          // notes that start together with another one on the same pad (a pad plays one voice)
	Merge   *MergeResult // set by the caller that adds the pattern to one that was there (MergePatterns)
}

// SuggestRoot is the note to play at a pad's own pitch so that the notes of the part sit in the middle of the 12 up and 12 down.
func SuggestRoot(p MIDIPart) int {
	lo, hi := p.Range()
	return (int(lo) + int(hi)) / 2
}

// FoldNote is the semitones of note above root, moved by octaves to be within PitchRange; folded says it had to be moved.
func FoldNote(note, root int) (semitones int, folded bool) {
	d := note - root
	for d > PitchRange {
		d -= 12
		folded = true
	}
	for d < -PitchRange {
		d += 12
		folded = true
	}
	return d, folded
}

// midiTicks is a tick of the file in ticks of a pattern (PPQ of 480).
func midiTicks(t uint32, ppq int) uint32 {
	return uint32((int64(t)*PPQ + int64(ppq)/2) / int64(ppq))
}

// BarTicks is the ticks of one bar of the file's time signature.
func (f MIDIFile) BarTicks() uint32 {
	return uint32(f.Beats * PPQ * 4 / max(f.BeatUnit, 1))
}

// MIDIBars is how many bars the file asks for: its length rounded up to whole bars, and not less than the notes of the part need.
func (f MIDIFile) MIDIBars(part int) int {
	bt := f.BarTicks()
	end := midiTicks(f.Length, f.PPQ)
	for _, n := range f.Parts[part].Notes {
		end = max(end, midiTicks(n.Tick+n.Len, f.PPQ))
	}
	return max(1, int((end+bt-1)/bt))
}

// BuildFromMIDI makes the pattern file of one part.
func BuildFromMIDI(f MIDIFile, o MIDIImport) (MIDIResult, error) {
	var res MIDIResult
	if o.Part < 0 || o.Part >= len(f.Parts) {
		return res, errors.New("there is no such part in the file")
	}
	part := f.Parts[o.Part]
	bars := o.Bars
	if bars == 0 {
		bars = f.MIDIBars(o.Part)
	}
	if bars < 1 || bars > MaxPatternBars {
		return res, fmt.Errorf("the file is %d bars long: a pattern is 1 to %d bars", bars, MaxPatternBars)
	}
	length := uint32(bars) * f.BarTicks()
	var recs []patRecord
	at := map[[2]uint32]int{} // the pad and the tick: how many notes start there
	for _, n := range part.Notes {
		tick := midiTicks(n.Tick, f.PPQ)
		if tick >= length {
			res.Left++
			continue
		}
		gate := uint16(min(max(midiTicks(n.Len, f.PPQ), 1), 65535))
		var pad proto.PadIndex
		var pitch uint8
		if o.Drums != nil {
			p, ok := o.Drums[n.Note]
			if !ok {
				res.Left++
				continue
			}
			pad = p
		} else {
			pad = o.Pad
			semi, folded := FoldNote(int(n.Note), o.Root)
			if folded {
				res.Folded++
			}
			pitch = PitchByte(semi)
			if pitch == 0 {
				pitch = PitchCentre // the note at the root is a chromatic note too, so that every note of the line is the same kind
			}
		}
		k := [2]uint32{uint32(pad.Index()), tick}
		if at[k]++; at[k] > 1 {
			res.Chords++
		}
		r := noteRecord(tick, pad, n.Vel, gate)
		r.bytes[2] = pitch
		recs = append(recs, r)
	}
	if len(recs) == 0 {
		return res, errors.New("no note of this part is left to write")
	}
	res.Notes, res.Bars = len(recs), bars
	res.Pattern = writeRecords(recs, length, midiTrailer(bars, f.Beats))
	return res, nil
}

// midiTrailer is the trailer of a pattern of bars bars with beats beats to the bar (the code for the beats is the one BeatsPerBar reads).
func midiTrailer(bars, beats int) []byte {
	t := startTrailer(bars)
	switch {
	case beats >= 1 && beats <= 3:
		t[4] = byte(beats)
	case beats >= 5 && beats <= 7:
		t[4] = byte(beats - 1)
	}
	return t
}

// DrumLane is the voice of a General MIDI drum note.
func DrumLane(note uint8) Lane {
	switch note {
	case 35, 36:
		return LaneKick
	case 37:
		return LaneRim
	case 38, 40:
		return LaneSnare
	case 39:
		return LaneClap
	case 42, 44:
		return LaneClosedHat
	case 46:
		return LaneOpenHat
	case 41, 43, 45, 47, 48, 50:
		return LaneTom
	case 49, 51, 52, 53, 55, 57, 59:
		return LaneRide
	case 54, 69, 70, 82:
		return LaneShaker
	case 60:
		return LaneBongoHi
	case 61:
		return LaneBongoLo
	case 62:
		return LaneCongaTip
	case 63:
		return LaneCongaOpen
	case 64:
		return LaneCongaBass
	}
	return LanePerc
}

// DrumNotes are the note numbers a drum part uses, the most used first.
func DrumNotes(p MIDIPart) []uint8 {
	count := map[uint8]int{}
	for _, n := range p.Notes {
		count[n.Note]++
	}
	var out []uint8
	for n := range count {
		out = append(out, n)
	}
	sort.Slice(out, func(a, b int) bool {
		if count[out[a]] != count[out[b]] {
			return count[out[a]] > count[out[b]]
		}
		return out[a] < out[b]
	})
	return out
}

// DrumName is the General MIDI name of a drum note, for a list.
func DrumName(note uint8) string {
	names := map[uint8]string{35: "Kick 2", 36: "Kick", 37: "Side stick", 38: "Snare", 39: "Clap", 40: "Snare 2", 41: "Low tom", 42: "Closed hat", 43: "Low tom 2",
		44: "Pedal hat", 45: "Tom", 46: "Open hat", 47: "Tom 2", 48: "High tom", 49: "Crash", 50: "High tom 2", 51: "Ride", 52: "China", 53: "Ride bell", 54: "Tambourine",
		55: "Splash", 56: "Cowbell", 57: "Crash 2", 59: "Ride 2", 60: "Hi bongo", 61: "Lo bongo", 62: "Mute conga", 63: "Open conga", 64: "Low conga", 69: "Cabasa", 70: "Maracas", 82: "Shaker"}
	if s, ok := names[note]; ok {
		return s
	}
	return "Perc"
}

// NoteName is the name of a MIDI note: 60 is C4.
func NoteName(n int) string {
	names := [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	return fmt.Sprintf("%s%d", names[(n%12+12)%12], n/12-1)
}

// The notes a pattern is written to a file with: a plain note of a pad is the note of the unit's own chart for that pad (36 for the
// first pad of a bank) on the drum channel, and a chromatic note is 60 plus its semitones on channel 1. Read back, the first is a
// drum part and the second a melody with the root 60.
const exportRoot = 60

// PatternToMIDI writes a pattern file as a MIDI file, a track for each pad (and kind of note) it plays.
func PatternToMIDI(pattern []byte, bpm float64) ([]byte, error) {
	p, err := ParsePattern(pattern)
	if err != nil {
		return nil, err
	}
	type key struct {
		pad   uint16
		chrom bool
	}
	tracks := map[key]*MIDIOut{}
	var order []key
	for _, e := range p.Events {
		if e.Control {
			continue
		}
		k := key{e.Pad.Index(), e.Pitch != 0}
		t := tracks[k]
		if t == nil {
			t = &MIDIOut{Name: e.Pad.String()}
			if k.chrom {
				t.Name += " keys"
				t.Channel = 0
			} else {
				t.Channel = 9
			}
			tracks[k] = t
			order = append(order, k)
		}
		note := 36 + int(e.Pad.Index()%16)
		if k.chrom {
			note = exportRoot + PitchSemitones(e.Pitch)
		}
		t.Notes = append(t.Notes, MIDINote{Tick: e.Tick, Len: uint32(max(e.Length, 1)), Note: uint8(note), Vel: max(e.Velocity, 1)})
	}
	if len(order) == 0 {
		return nil, errors.New("the pattern has no notes")
	}
	sort.Slice(order, func(a, b int) bool {
		if order[a].pad != order[b].pad {
			return order[a].pad < order[b].pad
		}
		return !order[a].chrom && order[b].chrom
	})
	out := make([]MIDIOut, 0, len(order))
	for _, k := range order {
		out = append(out, *tracks[k])
	}
	return WriteMIDI(out, bpm, p.BeatsPerBar(), p.Length), nil
}

// MergeResult says what MergePatterns made.
type MergeResult struct {
	Pattern  []byte
	Bars     int
	WasBars  int // the bars of the pattern that was there
	Notes    int
	Repeated bool // a shorter part was repeated to fill the length
}

// MergePatterns puts the notes of add into the pattern base, so that several files make one pattern. The pattern becomes as long as the
// longer of the two; with repeat, the shorter one goes round again to fill it (a 2-bar drum loop under an 8-bar melody plays four
// times, and a last round that does not fit is cut off at the end), without it the shorter one plays once. The trailer (the bars and
// the beats in a bar) is that of base, with the bars of the new length.
func MergePatterns(base, add []byte, repeat bool) (MergeResult, error) {
	var res MergeResult
	br, bl, bt, err := readRecords(base)
	if err != nil {
		return res, err
	}
	ar, al, at, err := readRecords(add)
	if err != nil {
		return res, err
	}
	if bl == 0 || al == 0 {
		return res, errors.New("a pattern with no length cannot be merged")
	}
	length := max(bl, al)
	round := func(recs []patRecord, own uint32) []patRecord {
		if !repeat || own >= length {
			return recs
		}
		var out []patRecord
		for off := uint32(0); off < length; off += own {
			for _, r := range recs {
				r.tick += off
				if r.tick >= length {
					continue
				}
				if r.note && off > 0 { // a note of a later round does not reach past the end
					gate := uint32(r.bytes[6])<<8 | uint32(r.bytes[5])
					if r.tick+gate > length {
						gate = length - r.tick
						r.bytes[5], r.bytes[6] = byte(gate), byte(gate>>8)
					}
				}
				out = append(out, r)
			}
			res.Repeated = true
		}
		return out
	}
	recs := append(round(br, bl), round(ar, al)...)
	bars := int((length + uint32(PPQ*4) - 1) / uint32(PPQ*4))
	trailer := bt
	if len(trailer) < 8 {
		trailer = at
	}
	if len(trailer) < 8 {
		trailer = startTrailer(bars)
	}
	trailer = append([]byte(nil), trailer...)
	beats := ParsePatternBeats(trailer)
	bars = int((length + uint32(beats*PPQ) - 1) / uint32(beats*PPQ))
	trailer[0] = byte(bars)
	res.Bars, res.WasBars = bars, int((bl+uint32(beats*PPQ)-1)/uint32(beats*PPQ))
	for _, r := range recs {
		if r.note {
			res.Notes++
		}
	}
	res.Pattern = writeRecords(recs, length, trailer)
	return res, nil
}

// ParsePatternBeats is the beats in a bar that a trailer says (see Pattern.BeatsPerBar).
func ParsePatternBeats(trailer []byte) int { return Pattern{Trailer: trailer}.BeatsPerBar() }
