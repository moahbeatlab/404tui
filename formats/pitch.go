package formats

import "math"

// Pitched is the sample a number of semitones higher, or lower when negative, the way a tape
// or a sampler's chromatic mode does it: by changing the speed, so a higher note is also a
// shorter one. The frames in between are read off a cubic curve through the four around them;
// nothing is filtered, which is good enough to audition a note.
func (s Sample) Pitched(semitones int) Sample {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	if semitones == 0 || frames < 2 {
		return s
	}
	ratio := math.Pow(2, float64(semitones)/12)
	n := int(float64(frames) / ratio)
	out := make([]int16, n*ch)
	for c := 0; c < ch; c++ {
		at := func(f int) float64 {
			return float64(s.Samples[min(max(f, 0), frames-1)*ch+c])
		}
		for i := 0; i < n; i++ {
			pos := float64(i) * ratio
			f := int(pos)
			t := pos - float64(f)
			y0, y1, y2, y3 := at(f-1), at(f), at(f+1), at(f+2)
			a := -0.5*y0 + 1.5*y1 - 1.5*y2 + 0.5*y3
			b := y0 - 2.5*y1 + 2*y2 - 0.5*y3
			d := -0.5*y0 + 0.5*y2
			v := ((a*t+b)*t+d)*t + y1
			out[i*ch+c] = int16(math.Round(min(max(v, math.MinInt16), math.MaxInt16)))
		}
	}
	return Sample{Channels: s.Channels, Samples: out}
}
