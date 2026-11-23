package formats

import (
	"math"
	"math/rand"
	"testing"
)

// house is a track of a kick on every beat, a hat on every offbeat and some noise, after lead frames of silence.
func house(bpm float64, lead, bars int, seed int64) Sample {
	beat := 60 / bpm * SampleRate
	n := lead + int(float64(bars*4)*beat) + SampleRate
	s := Sample{Channels: 1, Samples: make([]int16, n)}
	r := rand.New(rand.NewSource(seed))
	for i := range s.Samples {
		s.Samples[i] = int16(r.NormFloat64() * 300)
	}
	for b := 0; b < bars*4; b++ {
		at := lead + int(math.Round(float64(b)*beat))
		for k := 0; k < 9000 && at+k < n; k++ {
			f := 50 + 100*math.Exp(-float64(k)/1500)
			s.Samples[at+k] += int16(20000 * math.Sin(2*math.Pi*f*float64(k)/SampleRate) * math.Exp(-float64(k)/5000))
		}
		off := lead + int(math.Round((float64(b)+0.5)*beat))
		for k := 0; k < 2000 && off+k < n; k++ {
			s.Samples[off+k] += int16(r.NormFloat64() * 4000 * math.Exp(-float64(k)/500))
		}
	}
	return s
}

func TestTheTempoOfATrackIsHeard(t *testing.T) {
	for i, c := range []struct {
		bpm  float64
		lead int
	}{{124, 17760}, {122.5, 3000}, {128, 0}, {118, 40000}, {132, 9000}} {
		tp := DetectTempo(house(c.bpm, c.lead, 32, int64(i)))
		if math.Abs(tp.BPM-c.bpm) > 0.05 {
			t.Errorf("%v BPM heard as %v", c.bpm, tp.BPM)
			continue
		}
		beat := 60 / c.bpm * SampleRate
		ph := math.Mod(float64(tp.FirstBeat-c.lead)+100*beat, beat)
		if d := math.Min(ph, beat-ph); d > 0.012*SampleRate {
			t.Errorf("%v BPM: the beats are %.1f ms off the grid", c.bpm, d/SampleRate*1000)
		}
		if tp.Confidence < 0.5 {
			t.Errorf("%v BPM: sure only %.2f", c.bpm, tp.Confidence)
		}
	}
	if tp := DetectTempo(Sample{Channels: 1, Samples: make([]int16, SampleRate)}); tp.BPM != 0 {
		t.Errorf("silence has no tempo: %v", tp.BPM)
	}
}
