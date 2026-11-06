package core

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/moahbeatlab/404tui/formats"
)

// WriteDemo makes a folder of synthetic samples (drums, a bass note, two loops) to try the app with.
func WriteDemo(root string) error {
	rng := rand.New(rand.NewSource(404))
	mono := func(n int) []float64 { return make([]float64, n) }
	sr := float64(formats.SampleRate)
	kick := func() []float64 {
		b := mono(int(0.45 * sr))
		ph := 0.0
		for i := range b {
			t := float64(i) / sr
			f := 45 + 120*math.Exp(-t*28)
			ph += 2 * math.Pi * f / sr
			b[i] = math.Sin(ph) * math.Exp(-t*7)
		}
		return b
	}
	snare := func() []float64 {
		b := mono(int(0.3 * sr))
		for i := range b {
			t := float64(i) / sr
			b[i] = (rng.Float64()*2-1)*math.Exp(-t*18)*0.8 + math.Sin(2*math.Pi*190*t)*math.Exp(-t*25)*0.5
		}
		return b
	}
	hat := func(dec float64) []float64 {
		b := mono(int(0.18 * sr))
		prev := 0.0
		for i := range b {
			t := float64(i) / sr
			n := rng.Float64()*2 - 1
			b[i] = (n - prev*0.6) * math.Exp(-t*dec) * 0.5
			prev = n
		}
		return b
	}
	bass := func(hz float64) []float64 {
		b := mono(int(0.9 * sr))
		for i := range b {
			t := float64(i) / sr
			b[i] = (math.Sin(2*math.Pi*hz*t) + 0.3*math.Sin(4*math.Pi*hz*t)) * math.Exp(-t*2.2) * 0.7
		}
		return b
	}
	mix := func(dst []float64, at int, src []float64, g float64) {
		for i, v := range src {
			if at+i < len(dst) {
				dst[at+i] += v * g
			}
		}
	}
	loop := func(bpm float64, bars int, swing bool) []float64 {
		beat := 60 / bpm * sr
		out := mono(int(beat * 4 * float64(bars)))
		k, s, h, oh := kick(), snare(), hat(60), hat(14)
		for b := 0; b < bars; b++ {
			base := float64(b) * 4 * beat
			for _, st := range []float64{0, 2.5} {
				mix(out, int(base+st*beat), k, 0.9)
			}
			if b%2 == 1 {
				mix(out, int(base+3.5*beat), k, 0.8)
			}
			for _, st := range []float64{1, 3} {
				mix(out, int(base+st*beat), s, 0.8)
			}
			for st := 0.0; st < 4; st += 0.5 {
				off := 0.0
				if swing && int(st*2)%2 == 1 {
					off = 0.08
				}
				g := hat
				_ = g
				if int(st*2)%4 == 3 {
					mix(out, int(base+(st+off)*beat), oh, 0.5)
				} else {
					mix(out, int(base+(st+off)*beat), h, 0.5)
				}
			}
		}
		return out
	}
	put := func(rel string, data []float64) error {
		pcm := make([]int16, len(data))
		for i, v := range data {
			pcm[i] = int16(math.Max(-1, math.Min(1, v)) * 30000)
		}
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, formats.Sample{Channels: 1, Samples: pcm}.WAV(), 0o644)
	}
	for rel, d := range map[string][]float64{
		"drums/kick_808.wav":      kick(),
		"drums/snare_tight.wav":   snare(),
		"drums/hat_closed.wav":    hat(60),
		"drums/hat_open.wav":      hat(14),
		"bass/bass_C1.wav":        bass(32.7),
		"bass/bass_G1.wav":        bass(49),
		"loops/break_96bpm.wav":   loop(96, 2, false),
		"loops/shuffle_88bpm.wav": loop(88, 2, true),
	} {
		if err := put(rel, d); err != nil {
			return err
		}
	}
	return nil
}
