package record

import (
	"bytes"
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

func TestMeterKeepsTheHighestPeakBetweenTwoReadings(t *testing.T) {
	win := int(48000 * meterWindow.Seconds())
	var pcm []byte
	for _, amp := range []float64{0.2, 0.9, 0.1} { // three short windows: the middle one is the loud one
		for i := 0; i < win; i++ {
			v := int16(amp * 32767)
			pcm = append(pcm, byte(v), byte(v>>8), byte(v), byte(v>>8))
		}
	}
	m := &Meter{}
	m.read(bytes.NewReader(pcm))
	p, ok := m.Level()
	if !ok || p[0] < 0.89 || p[0] > 0.91 {
		t.Fatalf("the reading has the loudest of the windows since the last one: %v %v", p, ok)
	}
	if p2, _ := m.Level(); p2[0] != 0 {
		t.Fatalf("and starts again after it: %v", p2)
	}
	if spec, ok := m.Spectrum(); !ok || len(spec) != SpectrumBands {
		t.Fatalf("a spectrum from the latest windows: %d", len(spec))
	}
}
