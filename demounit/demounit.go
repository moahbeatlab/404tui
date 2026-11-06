// Package demounit is the simulated unit of the demo: four banks of different kits and a few patterns in each, made of sines and noise.
package demounit

import (
	"math"
	"math/rand"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/sim"
)

// ---- the sounds of the kit, made of sines and noise ----

func mono(seconds float64, f func(t float64) float64) formats.Sample {
	s := formats.Sample{Channels: 1, Samples: make([]int16, int(seconds*formats.SampleRate))}
	for i := range s.Samples {
		v := f(float64(i) / formats.SampleRate)
		s.Samples[i] = int16(math.Max(-1, math.Min(1, v)) * 30000)
	}
	return s
}

func decay(t, k float64) float64 { return math.Exp(-t * k) }

func kick() formats.Sample {
	return mono(0.45, func(t float64) float64 {
		return math.Sin(2*math.Pi*(45*t+110*(1-math.Exp(-t*30))/30)) * decay(t, 9)
	})
}

func snare(rng *rand.Rand) formats.Sample {
	return mono(0.3, func(t float64) float64 {
		return (math.Sin(2*math.Pi*190*t)*0.4*decay(t, 25) + (rng.Float64()*2-1)*0.7*decay(t, 16))
	})
}

func hat(rng *rand.Rand, k float64, long float64) formats.Sample {
	prev := 0.0
	return mono(long, func(t float64) float64 {
		n := rng.Float64()*2 - 1
		hp := n - prev
		prev = n
		return hp * 0.6 * decay(t, k)
	})
}

func tone(f0, f1 float64, secs, k float64) formats.Sample {
	return mono(secs, func(t float64) float64 {
		f := f0 + (f1-f0)*t/secs
		return math.Sin(2*math.Pi*f*t) * decay(t, k)
	})
}

func saw(f, secs, k float64) formats.Sample {
	return mono(secs, func(t float64) float64 {
		return (2*math.Mod(f*t, 1) - 1) * 0.5 * decay(t, k)
	})
}

// breakbeat is two bars of 120 BPM made of the hits above: something to slice.
func breakbeat(rng *rand.Rand) formats.Sample {
	const bpm = 120.0
	step := 60 / bpm / 4
	n := int(4 * 60 / bpm * 2 * formats.SampleRate)
	out := formats.Sample{Channels: 1, Samples: make([]int16, n)}
	mix := func(at float64, h formats.Sample, gain float64) {
		start := int(at * formats.SampleRate)
		for i, v := range h.Samples {
			if start+i < n {
				x := int(out.Samples[start+i]) + int(float64(v)*gain)
				out.Samples[start+i] = int16(max(-32000, min(32000, x)))
			}
		}
	}
	k, s, h, oh := kick(), snare(rng), hat(rng, 60, 0.08), hat(rng, 14, 0.25)
	kicks := map[int]bool{0: true, 6: true, 8: true, 11: true, 16: true, 22: true, 24: true, 27: true}
	snares := map[int]bool{4: true, 12: true, 20: true, 28: true, 31: true}
	for i := 0; i < 32; i++ {
		at := float64(i) * step
		if kicks[i] {
			mix(at, k, 0.9)
		}
		if snares[i] {
			mix(at, s, 0.8)
		}
		if i%2 == 0 {
			mix(at, h, 0.5)
		}
		if i == 14 || i == 30 {
			mix(at, oh, 0.5)
		}
	}
	return out
}

func square(f, secs, k float64) formats.Sample {
	return mono(secs, func(t float64) float64 {
		if math.Mod(f*t, 1) < 0.5 {
			return 0.35 * decay(t, k)
		}
		return -0.35 * decay(t, k)
	})
}

func chord(f, secs float64) formats.Sample {
	return mono(secs, func(t float64) float64 {
		v := 0.0
		for _, m := range []float64{1, 1.26, 1.5} {
			v += math.Sin(2*math.Pi*f*m*t) * 0.25
		}
		return v * math.Min(1, t*20) * decay(t, 2)
	})
}

func two(f1, f2, secs, k float64) formats.Sample {
	return mono(secs, func(t float64) float64 {
		return (math.Sin(2*math.Pi*f1*t) + math.Sin(2*math.Pi*f2*t)) * 0.3 * decay(t, k)
	})
}

type voice struct {
	name string
	smp  formats.Sample
}

// the kits: four banks of different sounds, 16 pads each
func kits(rng *rand.Rand) [][]voice {
	return [][]voice{
		{ // A: drums
			{"KICK", kick()}, {"SNARE", snare(rng)}, {"CLAP", hat(rng, 30, 0.2)}, {"HAT", hat(rng, 60, 0.08)},
			{"OPEN HAT", hat(rng, 14, 0.3)}, {"RIM", tone(900, 700, 0.06, 60)}, {"TOM LO", tone(140, 80, 0.4, 8)}, {"TOM HI", tone(240, 150, 0.35, 9)},
			{"BASS", saw(55, 0.5, 5)}, {"STAB", saw(330, 0.25, 12)}, {"CHORD", chord(262, 0.8)}, {"PERC", tone(520, 300, 0.12, 30)},
			{"SHAKER", hat(rng, 25, 0.12)}, {"FX", tone(200, 1600, 0.6, 4)}, {"VOX", tone(300, 260, 0.5, 5)}, {"BREAK 2 BARS", breakbeat(rng)},
		},
		{ // B: latin percussion
			{"CONGA LO", tone(190, 140, 0.25, 14)}, {"CONGA HI", tone(270, 200, 0.22, 16)}, {"BONGO LO", tone(340, 260, 0.18, 20)}, {"BONGO HI", tone(460, 360, 0.15, 22)},
			{"CLAVE", tone(2400, 2300, 0.08, 50)}, {"COWBELL", two(560, 845, 0.3, 12)}, {"GUIRO", hat(rng, 18, 0.2)}, {"AGOGO", two(800, 1190, 0.3, 12)},
			{"MARACAS", hat(rng, 35, 0.1)}, {"TAMBOURINE", hat(rng, 22, 0.15)}, {"CABASA", hat(rng, 28, 0.12)}, {"TRIANGLE", two(1500, 4140, 0.9, 4)},
			{"WOODBLOCK", tone(1700, 1600, 0.07, 45)}, {"SNAP", hat(rng, 45, 0.06)}, {"SHAKER", hat(rng, 25, 0.12)}, {"VIBRASLAP", hat(rng, 8, 0.4)},
		},
		{ // C: bass and synths
			{"SUB", saw(41, 0.7, 3)}, {"ACID", saw(110, 0.25, 10)}, {"PLUCK", saw(330, 0.2, 14)}, {"WARM PAD", chord(220, 1.2)},
			{"STAB", saw(392, 0.2, 12)}, {"MINOR CHORD", chord(196, 1.0)}, {"ARP", square(440, 0.12, 25)}, {"LEAD", square(523, 0.35, 6)},
			{"RISER", tone(200, 2400, 0.9, 2)}, {"IMPACT", tone(80, 35, 0.7, 5)}, {"ZAP", tone(1800, 120, 0.15, 14)}, {"NOISE", hat(rng, 4, 0.5)},
			{"VOX CHOP", tone(260, 230, 0.3, 6)}, {"BELL", two(880, 2430, 0.9, 4)}, {"ORGAN", chord(330, 0.9)}, {"FX HIT", tone(120, 50, 0.5, 5)},
		},
		{ // D: breaks
			{"KICK 2", tone(120, 38, 0.5, 8)}, {"SNARE 2", snare(rng)}, {"GHOST", tone(300, 200, 0.1, 30)}, {"HAT 2", hat(rng, 70, 0.07)},
			{"CRASH", hat(rng, 6, 0.9)}, {"RIDE", two(3200, 8800, 0.6, 6)}, {"TOM", tone(150, 90, 0.35, 8)}, {"BREAK", breakbeat(rng)},
			{"RIM 2", tone(950, 650, 0.05, 55)}, {"CLAP 2", hat(rng, 32, 0.18)}, {"SHAKER 2", hat(rng, 30, 0.1)}, {"SCRATCH", tone(1500, 300, 0.2, 8)},
			{"VOX", tone(340, 300, 0.4, 5)}, {"BASS 2", saw(49, 0.5, 5)}, {"STAB 2", saw(294, 0.25, 12)}, {"FX", tone(100, 2000, 0.6, 4)},
		},
	}
}

// New is the unit.
func New() *device.Device {
	rng := rand.New(rand.NewSource(404))
	s := sim.Demo()
	pad := func(i int) proto.PadIndex { p, _ := proto.NewPadIndex(uint16(i)); return p }
	banks := kits(rng)
	for b, kit := range banks {
		for i, k := range kit {
			s.SetSMP(pad(b*16+i), k.name, k.smp.SMP())
		}
	}
	s.KeepOnReload = true
	s.Keep() // a reload of the project (a kept tool makes the unit read it again) must not empty the pads
	dev := device.New(s, "demo unit")
	// each bank has patterns of its own: genre grooves on that bank's pads
	lanesOf := func(b int, m map[formats.Lane]int) map[formats.Lane]proto.PadIndex {
		out := map[formats.Lane]proto.PadIndex{}
		for l, i := range m {
			out[l] = pad(b*16 + i)
		}
		return out
	}
	laneMaps := []map[formats.Lane]int{
		{formats.LaneKick: 0, formats.LaneSnare: 1, formats.LaneClap: 2, formats.LaneClosedHat: 3, formats.LaneOpenHat: 4, formats.LaneRim: 5, formats.LaneTom: 6, formats.LanePerc: 11, formats.LaneShaker: 12},
		{formats.LaneKick: 0, formats.LaneSnare: 1, formats.LaneClap: 13, formats.LaneClosedHat: 8, formats.LaneOpenHat: 7, formats.LaneRim: 4, formats.LaneTom: 3, formats.LanePerc: 5, formats.LaneShaker: 10, formats.LaneRide: 11},
		{formats.LaneKick: 0, formats.LaneSnare: 4, formats.LaneClap: 2, formats.LaneClosedHat: 6, formats.LaneOpenHat: 13, formats.LanePerc: 10, formats.LaneRide: 3, formats.LaneRim: 1},
		{formats.LaneKick: 0, formats.LaneSnare: 1, formats.LaneClap: 9, formats.LaneClosedHat: 3, formats.LaneOpenHat: 4, formats.LaneRide: 5, formats.LaneTom: 6, formats.LaneShaker: 10, formats.LaneRim: 8},
	}
	names := [][]string{
		{"House", "Techno", "Hip-hop", "Breakbeat", "Electro", "Disco"},
		{"Reggaeton", "Amapiano", "Afro", "Footwork"},
		{"Trance", "Psytrance", "Dubstep"},
		{"Drum and bass", "Jungle", "UK garage", "Trap"},
	}
	for b, list := range names {
		for slot, name := range list {
			for _, g := range formats.GrooveTemplates() {
				if g.Name != name {
					continue
				}
				if built, err := g.BuildGroove(lanesOf(b, laneMaps[b]), 2); err == nil {
					app.WriteGroove(dev, pad(b*16+slot), built)
				}
			}
		}
	}
	return dev
}
