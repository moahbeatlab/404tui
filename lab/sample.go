package lab

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Meta says what a sample is.
type Meta struct {
	ID        int     `json:"id"`
	Frames    int     `json:"frames"`
	Channels  int     `json:"channels"`
	Seconds   float64 `json:"seconds"`
	SrcRate   int     `json:"srcRate,omitempty"`
	Resampled bool    `json:"resampled,omitempty"`
}

func (l *Lab) keep(s formats.Sample) int {
	id := l.next
	l.next++
	l.samples[id] = s
	return id
}

func (l *Lab) meta(id int) Meta {
	s := l.samples[id]
	return Meta{ID: id, Frames: s.Frames(), Channels: s.Channels, Seconds: float64(s.Frames()) / formats.SampleRate}
}

// Sample is the audio behind a handle.
func (l *Lab) Sample(id int) (formats.Sample, error) {
	s, ok := l.samples[id]
	if !ok {
		return formats.Sample{}, fmt.Errorf("no sample %d", id)
	}
	return s, nil
}

// Free lets go of a sample.
func (l *Lab) Free(id int) { delete(l.samples, id) }

// Decode reads an audio file (WAV, AIFF, FLAC, MP3, or an SMP) into a 48 kHz 16 bit sample.
func (l *Lab) Decode(b []byte, ext string) ([]byte, error) {
	if ext == "smp" || ext == ".smp" {
		s, err := formats.ParseSMP(b)
		if err != nil {
			return nil, err
		}
		return json.Marshal(l.meta(l.keep(s)))
	}
	imp, err := formats.DecodeAudio(b, ext)
	if err != nil {
		return nil, err
	}
	if imp.Sample.Frames() == 0 {
		return nil, errors.New("that file has no audio in it")
	}
	m := l.meta(l.keep(imp.Sample))
	m.SrcRate, m.Resampled = imp.SourceRate, imp.Resampled()
	return json.Marshal(m)
}

// Meta is the Meta of a sample.
func (l *Lab) Meta(id int) ([]byte, error) {
	if _, err := l.Sample(id); err != nil {
		return nil, err
	}
	return json.Marshal(l.meta(id))
}

// PadSample loads the sample of a pad (0..159) from the project.
func (l *Lab) PadSample(n int) ([]byte, error) {
	if n < 0 || n > 159 {
		return nil, fmt.Errorf("no pad %d", n)
	}
	f, ok := l.files[padFile(n)]
	if !ok {
		return nil, fmt.Errorf("pad %s has no sample file", padLabel(n))
	}
	s, err := formats.ParseSMP(f)
	if err != nil {
		return nil, fmt.Errorf("pad %s: %w", padLabel(n), err)
	}
	return json.Marshal(l.meta(l.keep(s)))
}

// PCM is the audio as 16 bit interleaved samples.
func (l *Lab) PCM(id int) ([]int16, error) {
	s, err := l.Sample(id)
	return s.Samples, err
}

// WAV is the sample as a WAV file.
func (l *Lab) WAV(id int) ([]byte, error) {
	s, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	return s.WAV(), nil
}

// Assign puts a sample on a pad: the SMP file, and the pad block pointed at it.
func (l *Lab) Assign(n, id int, name string) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if n < 0 || n > 159 {
		return fmt.Errorf("no pad %d", n)
	}
	s, err := l.Sample(id)
	if err != nil {
		return err
	}
	if s.Frames() < 2 {
		return errors.New("the sample is empty")
	}
	smp := s.SMP()
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return err
	}
	blk.AssignSample(uint32(len(smp)), clean(name, 23))
	if lv, ok := blk.Param(0x69); !ok || lv == 0 {
		blk.SetField(0x69-0x66, 127)
	}
	l.files[padFile(n)] = smp
	l.pad.Blocks[n] = blk.Raw()
	return nil
}

// ClearPad takes the sample off a pad.
func (l *Lab) ClearPad(n int) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if n < 0 || n > 159 {
		return fmt.Errorf("no pad %d", n)
	}
	delete(l.files, padFile(n))
	l.pad.Blocks[n] = formats.EmptyPadBlock(uint16(n))
	return nil
}

// RenamePad changes a pad's name.
func (l *Lab) RenamePad(n int, name string) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if n < 0 || n > 159 {
		return fmt.Errorf("no pad %d", n)
	}
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return err
	}
	blk.SetName(clean(name, 23))
	l.pad.Blocks[n] = blk.Raw()
	return nil
}

// MovePad moves a pad's sample and settings to another pad (swapping when both have one).
func (l *Lab) MovePad(from, to int) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if from < 0 || from > 159 || to < 0 || to > 159 {
		return errors.New("no such pad")
	}
	if from == to {
		return nil
	}
	fb, tb := append([]byte(nil), l.pad.Blocks[from]...), append([]byte(nil), l.pad.Blocks[to]...)
	setPad := func(b []byte, n int) []byte { b[1], b[2] = byte(n), byte(n>>8); return b }
	l.pad.Blocks[to], l.pad.Blocks[from] = setPad(fb, to), setPad(tb, from)
	ff, fok := l.files[padFile(from)]
	tf, tok := l.files[padFile(to)]
	delete(l.files, padFile(from))
	delete(l.files, padFile(to))
	if fok {
		l.files[padFile(to)] = ff
	}
	if tok {
		l.files[padFile(from)] = tf
	}
	return nil
}

// Peaks is the loudest value of cols parts of frames from..to, 0..32768, for drawing.
func (l *Lab) Peaks(id, from, to, cols int) ([]int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	return s.Peaks(from, to, cols), nil
}

// Transients finds where sounds start.
func (l *Lab) Transients(id, limit int, sensitivity float64) ([]int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	return s.Transients(limit, sensitivity), nil
}

// ZeroCrossing moves a frame to the nearest zero crossing within radius frames.
func (l *Lab) ZeroCrossing(id, frame, radius int) (int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return 0, err
	}
	return s.ZeroCrossing(frame, radius), nil
}

// Settle moves a tapped frame to the onset just before it.
func Settle(onsets []int, frame, before, after int) int {
	return formats.Settle(onsets, frame, before, after)
}

// Pitched is the sample changed by semitones the way a sampler's chromatic mode does it.
func (l *Lab) Pitched(id, semitones int) (int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return 0, err
	}
	return l.keep(s.Pitched(semitones)), nil
}

// Stretched is the sample made frames long without a change of pitch.
func (l *Lab) Stretched(id, frames int) (int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return 0, err
	}
	if frames < 64 || frames > 48000*600 {
		return 0, errors.New("that length is out of range")
	}
	return l.keep(s.Stretched(frames)), nil
}

// Trim is the part of the sample from..to (frames).
func (l *Lab) Trim(id, from, to int) (int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return 0, err
	}
	n := s.Frames()
	from, to = max(from, 0), min(to, n)
	if to-from < 2 {
		return 0, errors.New("nothing is left of that")
	}
	ch := max(s.Channels, 1)
	return l.keep(formats.Sample{Channels: s.Channels, Samples: append([]int16(nil), s.Samples[from*ch:to*ch]...)}), nil
}

// Join puts samples one after the other (up to 8).
func (l *Lab) Join(ids ...int) (int, error) {
	if len(ids) == 0 || len(ids) > 8 {
		return 0, errors.New("join 1 to 8 samples")
	}
	var parts []formats.Sample
	for _, id := range ids {
		s, err := l.Sample(id)
		if err != nil {
			return 0, err
		}
		parts = append(parts, s)
	}
	return l.keep(formats.JoinSamples(parts...)), nil
}

// Normalize brings the loudest peak to peak dB below full scale (a value like -1).
func (l *Lab) Normalize(id int, peakDB float64) (int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return 0, err
	}
	top := 0
	for _, v := range s.Samples {
		top = max(top, absInt(int(v)))
	}
	if top == 0 {
		return 0, errors.New("the sample is silent")
	}
	target := 32767 * pow10(peakDB/20)
	g := target / float64(top)
	out := formats.Sample{Channels: s.Channels, Samples: make([]int16, len(s.Samples))}
	for i, v := range s.Samples {
		out.Samples[i] = int16(min(max(float64(v)*g, -32768), 32767))
	}
	return l.keep(out), nil
}

// Reversed plays the sample backwards.
func (l *Lab) Reversed(id int) (int, error) {
	s, err := l.Sample(id)
	if err != nil {
		return 0, err
	}
	ch := max(s.Channels, 1)
	n := s.Frames()
	out := formats.Sample{Channels: s.Channels, Samples: make([]int16, len(s.Samples))}
	for f := 0; f < n; f++ {
		copy(out.Samples[f*ch:(f+1)*ch], s.Samples[(n-1-f)*ch:(n-f)*ch])
	}
	return l.keep(out), nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
