package sim

import (
	"math"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Tone is a mono sample of a sine wave, for filling a simulated unit with something real.
func Tone(hz float64, seconds float64) formats.Sample {
	s := formats.Sample{Channels: 1, Samples: make([]int16, int(seconds*formats.SampleRate))}
	for i := range s.Samples {
		s.Samples[i] = int16(math.Sin(2*math.Pi*hz*float64(i)/formats.SampleRate) * 12000)
	}
	return s
}

// NotePattern builds the bytes of a pattern file with one note per pad, a quarter note
// apart, in the layout of docs/re/04-patterns.md of SparkyMK2.
func NotePattern(pads ...proto.PadIndex) []byte {
	var b []byte
	for _, p := range pads {
		// delta (480 ticks does not fit a byte: 120 per note), note, group, pitch, velocity,
		// off velocity, length.
		b = append(b, 120, 0x2F+byte(p.Index()%80), byte(p.Index()/80), 0, 0x7F, 0x40, 0x77, 0)
	}
	b = append(b, 0, 0x8C, 0, 0, 0, 0, 0, 0)
	return append(b, 4, 0, 0, 0, 4, 0x80, 0, 0)
}

// Padconf builds a PADCONF.BIN header (version 3) naming the project; the rest is zeros.
func Padconf(name string) []byte {
	b := make([]byte, 52_000)
	copy(b, "RFPD")
	b[4] = 0xA0
	b[8] = 3
	copy(b[0x80:], name)
	return b
}

// Demo is a simulated unit with a few real samples, a pattern, a project file and some
// audio on the SD card: enough for every part of the program to have something to show.
func Demo() *Sim {
	s := New()
	pad := func(i uint16) proto.PadIndex { p, _ := proto.NewPadIndex(i); return p }
	for i, name := range []string{"kick", "snare", "hat"} {
		s.SetSMP(pad(uint16(i)), name, Tone(110*float64(i+1), 0.6).SMP())
	}
	s.SetSMP(pad(20), "vocal loop", Tone(330, 2.5).SMP())
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PADCONF.BIN", Padconf("PROJECT_01"))
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00018.BIN", NotePattern(pad(0), pad(1), pad(2)))
	s.SetPattern(pad(17), pad(0), pad(1), pad(2))
	for _, f := range []string{"SD:IMPORT/pack/break.wav", "SD:IMPORT/pack/bass.wav", "SD:IMPORT/kick 808.wav", "SD:IMPORT/vox/phrase.wav"} {
		s.AddFile(f, Tone(220, 0.8).WAV())
	}
	return s
}
