package record

import (
	"math"
	"testing"
)

func sine(freq, amp float64, n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = amp * math.Sin(2*math.Pi*freq*float64(i)/48000)
	}
	return x
}

func TestSpectrumFindsTheFrequency(t *testing.T) {
	for _, freq := range []float64{100, 1000, 8000} {
		b := Bands(sine(freq, 0.5, 4800), 48000)
		at, top := 0, 0.0
		for i, v := range b {
			if v > top {
				at, top = i, v
			}
		}
		lo := spectrumLow * math.Pow(spectrumHigh/spectrumLow, float64(at)/SpectrumBands)
		hi := spectrumLow * math.Pow(spectrumHigh/spectrumLow, float64(at+1)/SpectrumBands)
		if freq < lo*0.9 || freq > hi*1.1 {
			t.Errorf("%v Hz peaks in the band %.0f-%.0f Hz", freq, lo, hi)
		}
		// 0.5 is -6 dB: (72-6)/72
		if math.Abs(top-(72-6.0)/72) > 0.06 {
			t.Errorf("%v Hz at half scale reads %.2f", freq, top)
		}
	}
}

func TestSpectrumOfSilenceIsFlat(t *testing.T) {
	for _, v := range Bands(make([]float64, 4800), 48000) {
		if v != 0 {
			t.Fatalf("silence is %v", v)
		}
	}
}
