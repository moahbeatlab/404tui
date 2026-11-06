package record

import (
	"math"
	"math/cmplx"
)

// The spectrum: the sound of the latest window split into bands that are spaced like the ear hears (equal steps in pitch), each as the
// loudest bin in it, in dB.

const (
	// SpectrumBands is how many bands the meter keeps.
	SpectrumBands = 64
	fftSize       = 4096
	spectrumLow   = 30.0
	spectrumHigh  = 18000.0
	// spectrumFloor is the quietest the spectrum shows, in dB below full scale.
	spectrumFloor = -72.0
)

var hann = func() []float64 {
	w := make([]float64, fftSize)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize))
	}
	return w
}()

// fft is the radix-2 transform of x in place; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			f := complex(1, 0)
			for k := 0; k < size/2; k++ {
				a, b := x[start+k], x[start+k+size/2]*f
				x[start+k], x[start+k+size/2] = a+b, a-b
				f *= w
			}
		}
	}
}

// Bands is the spectrum of the last fftSize samples of mono (-1..1) at the rate: SpectrumBands values, 0 (the floor, or silence) to 1
// (full scale). With fewer samples than that it is the spectrum of those, as if the rest were silent.
func Bands(mono []float64, rate int) []float64 {
	buf := make([]complex128, fftSize)
	if len(mono) > fftSize {
		mono = mono[len(mono)-fftSize:]
	}
	for i, v := range mono {
		buf[i] = complex(v*hann[i], 0)
	}
	fft(buf)
	scale := 2 / (float64(fftSize) / 2) // a full-scale sine comes out as 1
	out := make([]float64, SpectrumBands)
	binHz := float64(rate) / fftSize
	edge := func(i int) float64 { return spectrumLow * math.Pow(spectrumHigh/spectrumLow, float64(i)/SpectrumBands) }
	for b := 0; b < SpectrumBands; b++ {
		lo, hi := int(edge(b)/binHz), int(edge(b+1)/binHz)
		hi = max(hi, lo+1)
		peak := 0.0
		for k := max(lo, 1); k < hi && k < fftSize/2; k++ {
			if m := cmplx.Abs(buf[k]) * scale; m > peak {
				peak = m
			}
		}
		db := spectrumFloor
		if peak > 0 {
			db = max(20*math.Log10(peak), spectrumFloor)
		}
		out[b] = (db - spectrumFloor) / -spectrumFloor
	}
	return out
}
