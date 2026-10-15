package formats

import (
	"math"
	"testing"
)

func sine(hz float64, frames int) Sample {
	s := Sample{Channels: 2, Samples: make([]int16, frames*2)}
	for f := 0; f < frames; f++ {
		v := int16(12000 * math.Sin(2*math.Pi*hz*float64(f)/SampleRate))
		s.Samples[f*2], s.Samples[f*2+1] = v, v
	}
	return s
}

// crossings counts the rises through zero: the pitch, as how many a second.
func risesBetween(s Sample, from, to int) int {
	n := 0
	for f := from + 1; f < to; f++ {
		if s.Samples[(f-1)*2] < 0 && s.Samples[f*2] >= 0 {
			n++
		}
	}
	return n
}

func TestStretchedKeepsThePitchAndHitsTheLength(t *testing.T) {
	src := sine(440, SampleRate*2)
	for _, frames := range []int{SampleRate * 3, SampleRate * 3 / 2, SampleRate * 2 * 9 / 10} {
		out := src.Stretched(frames)
		if out.Frames() != frames {
			t.Fatalf("%d frames, want %d", out.Frames(), frames)
		}
		// Away from the ends the pitch is the same: 440 rises a second.
		from, to := SampleRate/4, SampleRate/4+SampleRate/2
		if got := risesBetween(out, from, to); got < 215 || got > 225 {
			t.Errorf("stretched to %d frames: %d rises in half a second, want about 220", frames, got)
		}
	}
	if same := src.Stretched(src.Frames()); same.Frames() != src.Frames() {
		t.Error("the same length changed the sample")
	}
}

func TestStretchedShortAndEmptySamples(t *testing.T) {
	if out := (Sample{Channels: 2}).Stretched(1000); out.Frames() != 0 {
		t.Errorf("empty became %d frames", out.Frames())
	}
	if out := sine(440, 100).Stretched(250); out.Frames() != 250 {
		t.Errorf("short sample: %d frames", out.Frames())
	}
}
