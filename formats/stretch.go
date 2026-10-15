package formats

import "math"

// Stretched is the sample made longer or shorter without changing its pitch, to exactly
// frames frames: overlapping windows of the sound are laid down at a different spacing, each
// one moved a little to where it joins the one before without a jump (WSOLA). Good for loops
// and for steady sounds; a hit is softened a little when the change is large.
func (s Sample) Stretched(frames int) Sample {
	ch := max(s.Channels, 1)
	in := s.Frames()
	if frames <= 0 || in < 2 {
		return Sample{Channels: s.Channels}
	}
	if frames == in {
		return s
	}
	ratio := float64(frames) / float64(in)
	win := 2048
	if in < win*2 {
		win = max(in/2&^1, 16)
	}
	hop := win / 2
	tol := win / 4
	inHop := float64(hop) / ratio

	// What the search compares: the channels added together, as floats.
	mono := make([]float64, in)
	for f := 0; f < in; f++ {
		var sum float64
		for c := 0; c < ch; c++ {
			sum += float64(s.Samples[f*ch+c])
		}
		mono[f] = sum / float64(ch)
	}
	at := func(i int) float64 { return mono[min(max(i, 0), in-1)] }

	hann := make([]float64, win)
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(win))
	}
	acc := make([]float64, (frames+win*2)*ch)
	weight := make([]float64, frames+win*2)

	prev := 0 // where the window before was taken from
	for k := 0; ; k++ {
		outAt := k * hop
		if outAt >= frames+hop {
			break
		}
		nominal := int(math.Round(float64(k) * inHop))
		pos := nominal
		if k > 0 {
			// The window that would follow the last one if the sound went on as it was.
			want := prev + hop
			best, bestScore := nominal, math.Inf(-1)
			for d := -tol; d <= tol; d += 4 {
				cand := nominal + d
				if cand < 0 || cand >= in {
					continue
				}
				var score float64
				for i := 0; i < win; i += 8 {
					score += at(cand+i) * at(want+i)
				}
				if score > bestScore {
					best, bestScore = cand, score
				}
			}
			pos = best
		}
		prev = pos
		for i := 0; i < win; i++ {
			o := outAt + i
			if o >= frames+win {
				break
			}
			src := pos + i
			if src >= in {
				break
			}
			w := hann[i]
			weight[o] += w
			for c := 0; c < ch; c++ {
				acc[o*ch+c] += w * float64(s.Samples[src*ch+c])
			}
		}
	}
	out := make([]int16, frames*ch)
	for f := 0; f < frames; f++ {
		w := weight[f]
		if w < 1e-3 {
			continue
		}
		for c := 0; c < ch; c++ {
			out[f*ch+c] = int16(math.Round(math.Min(math.Max(acc[f*ch+c]/w, math.MinInt16), math.MaxInt16)))
		}
	}
	return Sample{Channels: s.Channels, Samples: out}
}
