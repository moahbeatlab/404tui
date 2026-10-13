package formats

import (
	"math"
	"testing"
)

// crossings counts the upward zero crossings of one channel: the cycles of a tone.
func crossings(s Sample, channel int) int {
	n := 0
	for f := 1; f < s.Frames(); f++ {
		if s.Samples[(f-1)*s.Channels+channel] < 0 && s.Samples[f*s.Channels+channel] >= 0 {
			n++
		}
	}
	return n
}

func TestPitchedChangesSpeedAndPitchTogether(t *testing.T) {
	// One second of 440 Hz on the left and 220 Hz on the right.
	tone := Sample{Channels: 2, Samples: make([]int16, 2*SampleRate)}
	for f := 0; f < SampleRate; f++ {
		x := 2 * math.Pi * float64(f) / SampleRate
		tone.Samples[2*f] = int16(20000 * math.Sin(440*x))
		tone.Samples[2*f+1] = int16(20000 * math.Sin(220*x))
	}
	for _, c := range []struct {
		semitones, frames int
	}{{12, SampleRate / 2}, {-12, SampleRate * 2}, {7, 32036}, {0, SampleRate}} {
		got := tone.Pitched(c.semitones)
		if got.Channels != 2 || got.Frames() != c.frames {
			t.Errorf("%+d semitones: %d channels, %d frames, want %d", c.semitones, got.Channels, got.Frames(), c.frames)
		}
		// The same cycles in a different time: the pitch moved by as much as the speed.
		if l, r := crossings(got, 0), crossings(got, 1); l < 438 || l > 441 || r < 218 || r > 221 {
			t.Errorf("%+d semitones: %d and %d cycles, want 440 and 220", c.semitones, l, r)
		}
	}
	// The curve overshoots at a step from the loudest sample to the quietest; it must stay at
	// the limit rather than wrap around to the other sign.
	step := Sample{Channels: 1, Samples: make([]int16, 16)}
	for i := range step.Samples {
		step.Samples[i] = math.MaxInt16
		if i >= 8 {
			step.Samples[i] = math.MinInt16
		}
	}
	for i, v := range step.Pitched(-12).Samples {
		if (i < 15 && v <= 0) || (i > 16 && v >= 0) {
			t.Errorf("frame %d of the step is %d", i, v)
		}
	}
	if got := (Sample{Channels: 1}).Pitched(5); got.Frames() != 0 {
		t.Errorf("an empty sample became %d frames", got.Frames())
	}
}
