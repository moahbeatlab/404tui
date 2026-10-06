package formats

import "math"

const (
	// Zero crossings of the sinc on each side of the centre.
	sincWings = 32
	// Kaiser window shape: about 100 dB of stopband attenuation.
	kaiserBeta = 9.0
	// Cutoff relative to the narrower Nyquist, leaving room for the filter's transition.
	cutoff = 0.96
)

// resample converts interleaved audio from rate to 48 kHz with a windowed-sinc polyphase
// filter, band-limited to the lower of the two rates.
func resample(in []float32, channels, rate int) []float32 {
	if rate == SampleRate {
		return in
	}
	frames := len(in) / channels
	if frames == 0 {
		return nil
	}
	g := gcd(rate, SampleRate)
	up, down := SampleRate/g, rate/g
	// Filter at the higher of the two rates, in units of the input spacing.
	scale := math.Min(1, float64(SampleRate)/float64(rate)) * cutoff
	wings := int(math.Ceil(sincWings / scale))
	taps := 2*wings + 1

	// One row of weights per output phase.
	table := make([]float32, up*taps)
	for p := 0; p < up; p++ {
		// Output sample n sits at input position n*down/up: whole input frames plus phase/up.
		frac := float64(p) / float64(up)
		var sum float64
		row := table[p*taps : (p+1)*taps]
		for k := range row {
			x := float64(k-wings) - frac
			w := sinc(x*scale) * scale * kaiser(x/float64(wings))
			row[k] = float32(w)
			sum += w
		}
		// Unity gain at DC whatever the phase.
		for k := range row {
			row[k] = float32(float64(row[k]) / sum)
		}
	}

	outFrames := int((int64(frames)*int64(SampleRate) + int64(rate) - 1) / int64(rate))
	out := make([]float32, outFrames*channels)
	for n := 0; n < outFrames; n++ {
		pos := int64(n) * int64(down)
		centre, phase := int(pos/int64(up)), int(pos%int64(up))
		row := table[phase*taps : (phase+1)*taps]
		for c := 0; c < channels; c++ {
			var acc float32
			for k, w := range row {
				i := centre + k - wings
				if i >= 0 && i < frames {
					acc += w * in[i*channels+c]
				}
			}
			out[n*channels+c] = acc
		}
	}
	return out
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// kaiser is the Kaiser window at x in −1..1.
func kaiser(x float64) float64 {
	if x <= -1 || x >= 1 {
		return 0
	}
	return bessel0(kaiserBeta*math.Sqrt(1-x*x)) / bessel0(kaiserBeta)
}

// bessel0 is the zeroth-order modified Bessel function of the first kind.
func bessel0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 50; k++ {
		term *= (x / (2 * float64(k))) * (x / (2 * float64(k)))
		sum += term
		if term < 1e-12*sum {
			break
		}
	}
	return sum
}
