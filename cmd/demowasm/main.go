//go:build js && wasm

// Command demowasm is the tools of the web page (pattern, slicer and pad tools) running in the browser against a simulated unit with a
// kit and a few patterns: the demo on GitHub Pages. The page calls sp404Tools(json) as it would POST to /tools/api of sp404 serve, and
// sound goes to SP404Audio.play(bytes, rate, channels) / SP404Audio.stop() in the page.
package main

import (
	"encoding/json"
	"syscall/js"
	"time"

	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/demounit"
	"github.com/moahbeatlab/404tui/formats"
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

func main() {
	core.Terminal = true
	dev := demounit.New()
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
