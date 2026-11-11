package formats

import (
	"errors"
	"fmt"
	"sort"
)

// Standard MIDI files (.mid): read into parts (the notes of one channel of one track) and written from notes. Only what a pattern
// needs is kept: notes with their channel, velocity and length, the first tempo, the first time signature and the track names.

// MIDINote is one note of a file, in the file's own ticks.
type MIDINote struct {
	Tick uint32
	Len  uint32
	Note uint8
	Vel  uint8
}

// MIDIPart is the notes of one channel of one track: a file of format 1 has a track for each instrument, a file of format 0 has one
// track with every channel, and each of them is one part here.
type MIDIPart struct {
	Track   int    // the track of the file, from 0
	Channel uint8  // 0..15
	Name    string // the name of the track, "" when it has none
	Notes   []MIDINote
}

// Drums is whether the part is on the drum channel (channel 10).
func (p MIDIPart) Drums() bool { return p.Channel == 9 }

// Range is the lowest and the highest note of the part.
func (p MIDIPart) Range() (lo, hi uint8) {
	if len(p.Notes) == 0 {
		return 0, 0
	}
	lo, hi = 127, 0
	for _, n := range p.Notes {
		lo, hi = min(lo, n.Note), max(hi, n.Note)
	}
	return lo, hi
}

// MIDIFile is what was read from a .mid file.
type MIDIFile struct {
	Format   int
	PPQ      int     // ticks per quarter note
	Tempo    float64 // BPM of the first tempo event, 0 when the file has none
	Beats    int     // the first time signature: Beats of a note of 1/BeatUnit; 4 and 4 when the file has none
	BeatUnit int
	Length   uint32 // the last tick of the file (where its tracks end, or its last note ends)
	Parts    []MIDIPart
}

// ParseMIDI reads a standard MIDI file (format 0 or 1; a file of format 2 is read as if its tracks played together).
func ParseMIDI(b []byte) (MIDIFile, error) {
	var f MIDIFile
	if len(b) < 14 || string(b[:4]) != "MThd" {
		return f, errors.New("this is not a MIDI file (it does not start with MThd)")
	}
	hl := int(be32(b[4:]))
	if hl < 6 || 8+hl > len(b) {
		return f, errors.New("the header of the MIDI file is damaged")
	}
	f.Format = int(be16(b[8:]))
	ntracks := int(be16(b[10:]))
	div := be16(b[12:])
	if div&0x8000 != 0 {
		return f, errors.New("this MIDI file counts in time code (SMPTE), not in beats: it cannot be a pattern")
	}
	f.PPQ = int(div)
	if f.PPQ == 0 {
		return f, errors.New("the MIDI file has no ticks per beat")
	}
	f.Beats, f.BeatUnit = 4, 4
	at, timeSig, tempoSeen := 8+hl, false, false
	for t := 0; at+8 <= len(b) && t < ntracks; {
		id, n := string(b[at:at+4]), int(be32(b[at+4:]))
		at += 8
		end := min(at+n, len(b)) // a file that ends early still gives what it has
		if id != "MTrk" {
			at = end
			continue
		}
		parts, err := f.readTrack(b[at:end], t, &timeSig, &tempoSeen)
		if err != nil {
			return f, fmt.Errorf("track %d: %w", t+1, err)
		}
		f.Parts = append(f.Parts, parts...)
		at, t = end, t+1
	}
	if len(f.Parts) == 0 {
		return f, errors.New("this MIDI file has no notes")
	}
	return f, nil
}

func be16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }
func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// readTrack reads one MTrk chunk: its notes (a note on pairs with the next note off of the same note and channel), the first tempo
// and time signature of the file, and the name of the track.
func (f *MIDIFile) readTrack(b []byte, track int, timeSig, tempoSeen *bool) ([]MIDIPart, error) {
	var (
		tick   uint32
		status byte
		name   string
		byChan = map[uint8]*MIDIPart{}
		open   = map[[2]uint8][]int{} // channel and note: the notes still sounding (indexes of the part's notes)
		i      int
	)
	varlen := func() (uint32, bool) {
		var v uint32
		for k := 0; k < 4; k++ {
			if i >= len(b) {
				return 0, false
			}
			c := b[i]
			i++
			v = v<<7 | uint32(c&0x7F)
			if c&0x80 == 0 {
				return v, true
			}
		}
		return v, true
	}
	part := func(ch uint8) *MIDIPart {
		if p := byChan[ch]; p != nil {
			return p
		}
		p := &MIDIPart{Track: track, Channel: ch}
		byChan[ch] = p
		return p
	}
	for i < len(b) {
		d, ok := varlen()
		if !ok || i >= len(b) {
			break
		}
		tick += d
		if b[i]&0x80 != 0 {
			status = b[i]
			i++
		} else if status == 0 {
			return nil, errors.New("a message has no status (the file is damaged)")
		}
		switch {
		case status == 0xFF: // meta event
			if i >= len(b) {
				break
			}
			typ := b[i]
			i++
			n, ok := varlen()
			if !ok || i+int(n) > len(b) {
				i = len(b)
				break
			}
			data := b[i : i+int(n)]
			i += int(n)
			switch {
			case typ == 0x03 && name == "":
				name = cleanName(data)
			case typ == 0x51 && n == 3 && !*tempoSeen:
				if us := int(data[0])<<16 | int(data[1])<<8 | int(data[2]); us > 0 {
					f.Tempo, *tempoSeen = 60e6/float64(us), true
				}
			case typ == 0x58 && n >= 2 && !*timeSig:
				if data[0] > 0 && data[1] < 8 {
					f.Beats, f.BeatUnit, *timeSig = int(data[0]), 1<<data[1], true
				}
			}
			status = 0 // a meta event ends running status
		case status == 0xF0 || status == 0xF7: // system exclusive
			n, ok := varlen()
			if !ok || i+int(n) > len(b) {
				i = len(b)
				break
			}
			i += int(n)
			status = 0
		default:
			ch := status & 0x0F
			args := 2
			if s := status & 0xF0; s == 0xC0 || s == 0xD0 {
				args = 1
			}
			if i+args > len(b) {
				i = len(b)
				break
			}
			d1 := b[i] & 0x7F
			var d2 byte
			if args == 2 {
				d2 = b[i+1] & 0x7F
			}
			i += args
			switch status & 0xF0 {
			case 0x90:
				if d2 > 0 {
					p := part(ch)
					p.Notes = append(p.Notes, MIDINote{Tick: tick, Note: d1, Vel: d2})
					k := [2]uint8{ch, d1}
					open[k] = append(open[k], len(p.Notes)-1)
					break
				}
				fallthrough // a note on with velocity 0 is a note off
			case 0x80:
				k := [2]uint8{ch, d1}
				if idx := open[k]; len(idx) > 0 {
					n := &byChan[ch].Notes[idx[0]]
					n.Len = tick - n.Tick
					open[k] = idx[1:]
				}
			}
		}
	}
	// a note that was never ended lasts to the end of the track
	for k, idx := range open {
		p := byChan[k[0]]
		for _, j := range idx {
			p.Notes[j].Len = tick - p.Notes[j].Tick
		}
	}
	if tick > f.Length {
		f.Length = tick
	}
	var out []MIDIPart
	for ch := uint8(0); ch < 16; ch++ {
		if p := byChan[ch]; p != nil && len(p.Notes) > 0 {
			p.Name = name
			sort.SliceStable(p.Notes, func(a, b int) bool { return p.Notes[a].Tick < p.Notes[b].Tick })
			out = append(out, *p)
		}
	}
	return out, nil
}

// cleanName keeps the printable characters of a track name.
func cleanName(b []byte) string {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		if c >= 32 && c < 127 {
			out = append(out, rune(c))
		}
	}
	s := string(out)
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// MIDIOut is one track of a file to write.
type MIDIOut struct {
	Name    string
	Channel uint8
	Notes   []MIDINote // in ticks of PPQ (480)
}

// WriteMIDI writes a file of format 1: a first track with the tempo and the time signature, then a track for each of tracks. The notes
// are in ticks of PPQ (480) and the file is length ticks long.
func WriteMIDI(tracks []MIDIOut, bpm float64, beats int, length uint32) []byte {
	if bpm <= 0 {
		bpm = 120
	}
	if beats < 1 {
		beats = 4
	}
	out := []byte("MThd\x00\x00\x00\x06\x00\x01")
	out = append(out, byte((len(tracks)+1)>>8), byte(len(tracks)+1), byte(PPQ>>8), byte(PPQ&0xFF))
	us := int(60e6/bpm + 0.5)
	conductor := []byte{0, 0xFF, 0x51, 3, byte(us >> 16), byte(us >> 8), byte(us), 0, 0xFF, 0x58, 4, byte(beats), 2, 24, 8}
	conductor = appendVar(conductor, length)
	conductor = append(conductor, 0xFF, 0x2F, 0)
	out = appendChunk(out, conductor)
	for _, t := range tracks {
		type ev struct {
			tick uint32
			on   bool
			note uint8
			vel  uint8
		}
		var evs []ev
		for _, n := range t.Notes {
			evs = append(evs, ev{n.Tick, true, n.Note, max(n.Vel, 1)}, ev{n.Tick + max(n.Len, 1), false, n.Note, 0})
		}
		// at the same tick the notes end before the next ones start
		sort.SliceStable(evs, func(a, b int) bool {
			if evs[a].tick != evs[b].tick {
				return evs[a].tick < evs[b].tick
			}
			return !evs[a].on && evs[b].on
		})
		var tr []byte
		if t.Name != "" {
			tr = append(tr, 0, 0xFF, 0x03, byte(len(t.Name)))
			tr = append(tr, t.Name...)
		}
		var at uint32
		for _, e := range evs {
			tr = appendVar(tr, e.tick-at)
			at = e.tick
			if e.on {
				tr = append(tr, 0x90|t.Channel&0x0F, e.note, e.vel)
			} else {
				tr = append(tr, 0x80|t.Channel&0x0F, e.note, 0)
			}
		}
		tr = appendVar(tr, max(length, at)-at)
		tr = append(tr, 0xFF, 0x2F, 0)
		out = appendChunk(out, tr)
	}
	return out
}

func appendChunk(out, body []byte) []byte {
	out = append(out, "MTrk"...)
	out = append(out, byte(len(body)>>24), byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	return append(out, body...)
}

func appendVar(b []byte, v uint32) []byte {
	var tmp [5]byte
	n := 0
	tmp[0] = byte(v & 0x7F)
	for v >>= 7; v > 0; v >>= 7 {
		n++
		tmp[n] = byte(v&0x7F) | 0x80
	}
	for ; n >= 0; n-- {
		b = append(b, tmp[n])
	}
	return b
}
