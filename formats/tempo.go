package formats

import "math"

// Tempo detection for a track with a steady beat (house, techno: a kick on the beats): an onset curve (how much the low end and the
// whole sound rise, about 190 times a second), its autocorrelation for the tempos 60-200 BPM (the octave folded towards 100-140),
// then the tempo refined to a hundredth of a BPM by laying a grid of beats over the whole track, which also gives where the beats
// are.

// Tempo is what DetectTempo finds.
type Tempo struct {
	BPM        float64 // 0: none found
	FirstBeat  int     // the frame of the first beat of the grid in the sample
	Confidence float64 // 0..1: how much the grid stands out of the rest
}

const tempoHop = 256 // frames of the onset curve

// onsetCurve is how much the sound rises, frame by frame of tempoHop: the low end (the kick) weighs most.
func onsetCurve(s Sample) []float64 {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	n := frames / tempoHop
	if n < 8 {
		return nil
	}
	low, full := make([]float64, n), make([]float64, n)
	var lp float64
	a := 1 - math.Exp(-2*math.Pi*150/SampleRate) // one pole low pass at 150 Hz
	for i := 0; i < n; i++ {
		var el, ef float64
		for f := i * tempoHop; f < (i+1)*tempoHop; f++ {
			var v float64
			for c := 0; c < ch; c++ {
				v += float64(s.Samples[f*ch+c])
			}
			v /= float64(ch) * 32768
			lp += a * (v - lp)
			el += lp * lp
			ef += v * v
		}
		low[i], full[i] = math.Log1p(1000*el), math.Log1p(1000*ef)
	}
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		out[i] = 2*math.Max(0, low[i]-low[i-1]) + math.Max(0, full[i]-full[i-1])
	}
	// take the slow swell out: what is left are the hits
	w := 16
	var sum float64
	mean := make([]float64, n)
	for i := 0; i < n; i++ {
		sum += out[i]
		if i >= w {
			sum -= out[i-w]
		}
		mean[i] = sum / float64(min(i+1, w))
	}
	for i := range out {
		out[i] = math.Max(0, out[i]-mean[i])
	}
	return out
}

// tempoWeight prefers the tempos of dance music when the octave is in doubt.
func tempoWeight(bpm float64) float64 {
	d := math.Log2(bpm / 122)
	return math.Exp(-d * d / (2 * 0.35 * 0.35))
}

// gridScore is how much onset lies on a grid of beats of period p (frames of the curve), at its best phase: the score, the phase.
func gridScore(on []float64, p float64) (float64, float64) {
	steps := int(p * 4) // phases a quarter frame apart
	best, bestPh := -1.0, 0.0
	for k := 0; k < steps; k++ {
		ph := float64(k) / 4
		var sc float64
		var cnt int
		for t := ph; t < float64(len(on)-1); t += p {
			i := int(t)
			fr := t - float64(i)
			sc += on[i]*(1-fr) + on[i+1]*fr
			cnt++
		}
		if cnt > 0 {
			sc /= float64(cnt)
		}
		if sc > best {
			best, bestPh = sc, ph
		}
	}
	return best, bestPh
}

// DetectTempo finds the tempo and the beats of a track with a steady beat.
func DetectTempo(s Sample) Tempo {
	on := onsetCurve(s)
	if len(on) < 200 {
		return Tempo{}
	}
	fps := float64(SampleRate) / tempoHop
	// autocorrelation over up to 90 s in the middle of the track
	lo, hi := 0, len(on)
	if span := int(90 * fps); hi-lo > span {
		lo = (len(on) - span) / 2
		hi = lo + span
	}
	seg := on[lo:hi]
	var mean float64
	for _, v := range seg {
		mean += v
	}
	mean /= float64(len(seg))
	type cand struct{ bpm, score float64 }
	var cands []cand
	for bpm := 60.0; bpm <= 200; bpm += 0.5 {
		lag := 60 * fps / bpm
		l0 := int(lag)
		fr := lag - float64(l0)
		var r float64
		for i := 0; i+l0+1 < len(seg); i++ {
			v := seg[i] - mean
			r += v * ((seg[i+l0]-mean)*(1-fr) + (seg[i+l0+1]-mean)*fr)
		}
		cands = append(cands, cand{bpm, r})
	}
	// the best, its octave folded towards dance tempos
	best := cand{}
	for _, c := range cands {
		if c.score <= 0 {
			continue
		}
		w := c.score * tempoWeight(c.bpm)
		if best.bpm == 0 || w > best.score {
			best = cand{c.bpm, w}
		}
	}
	if best.bpm == 0 {
		return Tempo{}
	}
	// refined on a grid of beats laid over the whole track
	bestBPM, bestScore, bestPh := best.bpm, -1.0, 0.0
	for _, step := range []float64{0.1, 0.01} {
		from, to := bestBPM-1, bestBPM+1
		if step < 0.1 {
			from, to = bestBPM-0.1, bestBPM+0.1
		}
		for bpm := from; bpm <= to+1e-9; bpm += step {
			sc, ph := gridScore(on, 60*fps/bpm)
			if sc > bestScore {
				bestBPM, bestScore, bestPh = bpm, sc, ph
			}
		}
	}
	if r := math.Round(bestBPM); math.Abs(bestBPM-r) < 0.06 { // most tracks are at a whole tempo
		bestBPM = r
		bestScore, bestPh = gridScore(on, 60*fps/bestBPM)
	} else {
		bestBPM = math.Round(bestBPM*10) / 10
	}
	var avg float64
	for _, v := range on {
		avg += v
	}
	avg /= float64(len(on))
	conf := 0.0
	if bestScore > 0 {
		conf = math.Max(0, math.Min(1, 1-avg/bestScore))
	}
	return Tempo{BPM: bestBPM, FirstBeat: int(bestPh * tempoHop), Confidence: conf}
}

// BeatAt is where the first beat of a known tempo lies (the frame): the grid of that tempo laid where the most onset falls on it.
// -1 when the sample has too little to go by.
func BeatAt(s Sample, bpm float64) int {
	on := onsetCurve(s)
	if len(on) < 200 || bpm <= 0 {
		return -1
	}
	_, ph := gridScore(on, 60*float64(SampleRate)/tempoHop/bpm)
	return int(ph * tempoHop)
}
