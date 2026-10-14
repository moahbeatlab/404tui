package app

import (
	"fmt"
	"math"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Voice is how a pad sounds in a mix on this computer: its sample, already at the pad's level,
// pan, pitch and direction (see PadVoice), and whether the pad is gated or loops, which
// depends on how long each note lasts.
type Voice struct {
	Sample formats.Sample
	// Gate cuts the sample when the note ends; Loop repeats it for as long as the note lasts.
	Gate, Loop bool
}

// MixPattern renders a pattern with each pad's sample as it is stored; see MixPatternVoices.
func MixPattern(p formats.Pattern, bpm float64, audio func(proto.PadIndex) (formats.Sample, error)) (formats.Sample, error) {
	return MixPatternVoices(p, bpm, func(pad proto.PadIndex) (Voice, error) {
		s, err := audio(pad)
		return Voice{Sample: s}, err
	})
}

// MixPatternVoices renders a pattern to audio on this computer, as one round of a loop exactly
// the pattern long: each note starts its pad's voice at its tick, at the given tempo and at
// its velocity, and they are added together. A gated pad stops when its note ends, a looping
// one repeats until then. It is an approximation: the unit's envelope, effects, vinyl and
// time stretch are not applied.
func MixPatternVoices(p formats.Pattern, bpm float64, voice func(proto.PadIndex) (Voice, error)) (formats.Sample, error) {
	if bpm <= 0 {
		return formats.Sample{}, fmt.Errorf("the pattern has no tempo")
	}
	framesPerTick := float64(formats.SampleRate) * 60 / bpm / float64(formats.PPQ)
	total := int(float64(p.Length) * framesPerTick)
	type note struct {
		at, frames int
		sample     formats.Sample
		gain       float64
		loop       bool
	}
	var notes []note
	cache := map[proto.PadIndex]Voice{}
	for _, e := range p.Events {
		if e.Control {
			continue
		}
		v, ok := cache[e.Pad]
		if !ok {
			var err error
			if v, err = voice(e.Pad); err != nil {
				return formats.Sample{}, err
			}
			cache[e.Pad] = v
		}
		s := v.Sample
		gain := float64(e.Velocity) / 127
		if e.Velocity == 0 {
			gain = 1
		}
		// How many frames sound: the whole sample, cut at the note's end when the pad is gated,
		// or as long as the note when it loops.
		frames, held := s.Frames(), int(float64(e.Length)*framesPerTick)
		switch {
		case v.Loop && held > 0:
			frames = held
		case v.Gate && held > 0:
			frames = min(frames, held)
		}
		notes = append(notes, note{at: int(float64(e.Tick) * framesPerTick), frames: frames, sample: s, gain: gain, loop: v.Loop})
	}
	if total == 0 {
		for _, n := range notes {
			total = max(total, n.at+n.frames)
		}
	}
	if len(notes) == 0 {
		return formats.Sample{}, fmt.Errorf("the pattern plays nothing")
	}
	mix := make([]int32, total*2)
	for _, n := range notes {
		ch := max(n.sample.Channels, 1)
		length := n.sample.Frames()
		if length == 0 {
			continue
		}
		// The mix is one round of a loop, exactly the pattern long: what still sounds at the end
		// goes on at the start, as it does when the pattern comes round again. A sample longer
		// than the whole round is cut there.
		for f := 0; f < n.frames && f < total; f++ {
			src := f
			if n.loop {
				src = f % length
			} else if f >= length {
				break
			}
			l := float64(n.sample.Samples[src*ch])
			r := l
			if ch > 1 {
				r = float64(n.sample.Samples[src*ch+1])
			}
			at := (n.at + f) % total
			mix[at*2] += int32(l * n.gain)
			mix[at*2+1] += int32(r * n.gain)
		}
	}
	out := formats.Sample{Channels: 2, Samples: make([]int16, len(mix))}
	for i, v := range mix {
		out.Samples[i] = int16(max(min(v, 32767), -32768))
	}
	return out, nil
}

// PadVoice is a pad's sample made to sound as the pad's settings say, as far as this computer
// can: at its level, panned, pitched by its semitones and cents (by changing its speed, as a
// sampler does), forwards or backwards; gate and loop come along for the mix. Always stereo.
func PadVoice(s formats.Sample, level int32, params map[string]int32) Voice {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	// Pitch: read through the sample faster or slower, between the frames.
	ratio := math.Pow(2, (float64(params["pitch-coarse"])+float64(params["pitch-fine"])/100)/12)
	out := frames
	if ratio != 1 && ratio > 0 {
		out = int(float64(frames) / ratio)
	}
	mode := params["play-mode"]
	reverse := mode == 1 || mode == 3
	// Pan: the balance is 13 (left) to 115 (right), 64 the centre.
	pan := max(min(float64(params["balance"]-64)/51, 1), -1)
	if params["balance"] == 0 {
		pan = 0
	}
	lg, rg := min(1, 1-pan), min(1, 1+pan)
	gain := float64(max(min(level, 127), 0)) / 127
	at := func(frame, c int) float64 {
		return float64(s.Samples[frame*ch+min(c, ch-1)])
	}
	v := Voice{Sample: formats.Sample{Channels: 2, Samples: make([]int16, out*2)}, Gate: params["gate"] != 0, Loop: params["loop"] != 0}
	for i := 0; i < out; i++ {
		pos := float64(i) * ratio
		if ratio == 1 {
			pos = float64(i)
		}
		f := int(pos)
		if f >= frames {
			break
		}
		next := min(f+1, frames-1)
		frac := pos - float64(f)
		dst := i
		if reverse {
			dst = out - 1 - i
		}
		for c := 0; c < 2; c++ {
			x := at(f, c)*(1-frac) + at(next, c)*frac
			g := lg
			if c == 1 {
				g = rg
			}
			v.Sample.Samples[dst*2+c] = int16(max(min(x*gain*g, 32767), -32768))
		}
	}
	return v
}
