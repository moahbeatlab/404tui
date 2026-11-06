//go:build js && wasm

// Command demowasm is the tools of the web page (pattern, slicer and pad tools) running in the browser against a simulated unit with a
// kit and a few patterns: the demo on GitHub Pages. The page calls sp404Tools(json) as it would POST to /tools/api of sp404 serve, and
// sound goes to SP404Audio.play(bytes, rate, channels) / SP404Audio.stop() in the page.
package main

import (
	"encoding/json"
	"math"
	"math/rand"
	"syscall/js"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/sim"
	"github.com/moahbeatlab/404tui/toolsapi"
)

// browserAudio plays on the page's audio output.
type browserAudio struct{}

func (browserAudio) send(method string, s formats.Sample, extra ...any) error {
	a := js.Global().Get("SP404Audio")
	if a.IsUndefined() {
		return nil
	}
	b := make([]byte, 2*len(s.Samples))
	for i, v := range s.Samples {
		b[2*i], b[2*i+1] = byte(uint16(v)), byte(uint16(v)>>8)
	}
	u := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(u, b)
	a.Call(method, append([]any{u, formats.SampleRate, s.Channels}, extra...)...)
	return nil
}

func (a browserAudio) Play(s formats.Sample) error        { return a.send("play", s, false) }
func (a browserAudio) Loop(s formats.Sample) error        { return a.send("play", s, true) }
func (a browserAudio) ReplaceLoop(s formats.Sample) error { return a.send("replace", s) }
func (a browserAudio) Preview(s formats.Sample) error     { return a.send("preview", s) }

func (browserAudio) StopPreview() {
	if a := js.Global().Get("SP404Audio"); !a.IsUndefined() {
		a.Call("stopPreview")
	}
}

func (browserAudio) Stop() {
	if a := js.Global().Get("SP404Audio"); !a.IsUndefined() {
		a.Call("stop")
	}
}

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

func demoUnit() *device.Device {
	rng := rand.New(rand.NewSource(404))
	s := sim.Demo()
	pad := func(i int) proto.PadIndex { p, _ := proto.NewPadIndex(uint16(i)); return p }
	kit := []struct {
		name string
		smp  formats.Sample
	}{
		{"KICK", kick()}, {"SNARE", snare(rng)}, {"CLAP", hat(rng, 30, 0.2)}, {"HAT", hat(rng, 60, 0.08)},
		{"OPEN HAT", hat(rng, 14, 0.3)}, {"RIM", tone(900, 700, 0.06, 60)}, {"TOM LO", tone(140, 80, 0.4, 8)}, {"TOM HI", tone(240, 150, 0.35, 9)},
		{"BASS", saw(55, 0.5, 5)}, {"STAB", saw(330, 0.25, 12)}, {"CHORD", saw(262, 0.8, 4)}, {"PERC", tone(520, 300, 0.12, 30)},
		{"SHAKER", hat(rng, 25, 0.12)}, {"FX", tone(200, 1600, 0.6, 4)}, {"VOX", tone(300, 260, 0.5, 5)}, {"BREAK 2 BARS", breakbeat(rng)},
	}
	for i, k := range kit {
		s.SetSMP(pad(i), k.name, k.smp.SMP())
	}
	dev := device.New(s, "demo unit")
	lanes := map[formats.Lane]proto.PadIndex{
		formats.LaneKick: pad(0), formats.LaneSnare: pad(1), formats.LaneClap: pad(2), formats.LaneClosedHat: pad(3), formats.LaneOpenHat: pad(4),
		formats.LaneRim: pad(5), formats.LaneTom: pad(6), formats.LanePerc: pad(11), formats.LaneShaker: pad(12),
	}
	for slot, name := range []string{"House", "Techno", "Hip-hop", "Breakbeat", "Electro", "Disco"} {
		for _, g := range formats.GrooveTemplates() {
			if g.Name == name {
				if b, err := g.BuildGroove(lanes, 2); err == nil {
					app.WriteGroove(dev, pad(slot), b)
				}
			}
		}
	}
	return dev
}

func main() {
	core.Terminal = true
	dev := demoUnit()
	a := core.New("/demo", "/demo", browserAudio{})
	a.SetPlatform(core.Platform{Name: "web", Keyboard: true})
	a.Connect = func() (core.Link, error) { return core.DeviceLink{Dev: dev}, nil }
	r := core.NewRemote(a)
	r.Follow(true, true)
	go r.Loop(make(chan struct{}))

	js.Global().Set("sp404Tools", js.FuncOf(func(this js.Value, args []js.Value) any {
		var in toolsapi.Request
		if len(args) > 0 {
			json.Unmarshal([]byte(args[0].String()), &in)
		}
		v, code, msg := toolsapi.Do(r, in)
		out := map[string]any{"code": code}
		if code != 0 {
			out["msg"] = msg
		} else {
			out["body"] = v
		}
		b, _ := json.Marshal(out)
		return string(b)
	}))
	// ready when the project of the unit is loaded
	go func() {
		for {
			if r.State().Live > 0 {
				js.Global().Set("sp404ToolsReady", true)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()
	select {}
}
