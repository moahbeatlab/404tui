package app

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// MixPattern renders a pattern to audio on this computer: each note starts its pad's sample
// at its tick, at the given tempo, and the samples are added together. It plays what the
// pads hold as stored (no pitch, envelope, effects or gate) at the note's velocity; the
// unit's own playback is what the pattern really sounds like. audio gives a pad's sample.
func MixPattern(p formats.Pattern, bpm float64, audio func(proto.PadIndex) (formats.Sample, error)) (formats.Sample, error) {
	if bpm <= 0 {
		return formats.Sample{}, fmt.Errorf("the pattern has no tempo")
	}
	framesPerTick := float64(formats.SampleRate) * 60 / bpm / float64(formats.PPQ)
	total := int(float64(p.Length) * framesPerTick)
	type note struct {
		at     int
		sample formats.Sample
		gain   float64
	}
	var notes []note
	cache := map[proto.PadIndex]formats.Sample{}
	for _, e := range p.Events {
		if e.Control {
			continue
		}
		s, ok := cache[e.Pad]
		if !ok {
			var err error
			if s, err = audio(e.Pad); err != nil {
				return formats.Sample{}, err
			}
			cache[e.Pad] = s
		}
		gain := float64(e.Velocity) / 127
		if e.Velocity == 0 {
			gain = 1
		}
		n := note{at: int(float64(e.Tick) * framesPerTick), sample: s, gain: gain}
		notes = append(notes, n)
		total = max(total, n.at+s.Frames())
	}
	if len(notes) == 0 {
		return formats.Sample{}, fmt.Errorf("the pattern plays nothing")
	}
	mix := make([]int32, total*2)
	for _, n := range notes {
		ch := max(n.sample.Channels, 1)
		for f := 0; f < n.sample.Frames() && n.at+f < total; f++ {
			l := float64(n.sample.Samples[f*ch])
			r := l
			if ch > 1 {
				r = float64(n.sample.Samples[f*ch+1])
			}
			mix[(n.at+f)*2] += int32(l * n.gain)
			mix[(n.at+f)*2+1] += int32(r * n.gain)
		}
	}
	out := formats.Sample{Channels: 2, Samples: make([]int16, len(mix))}
	for i, v := range mix {
		out.Samples[i] = int16(max(min(v, 32767), -32768))
	}
	return out, nil
}
