//go:build js && wasm

// Command labwasm is the lab package as WebAssembly: the 404jam web app loads it in a Web
// Worker and calls the functions of LabGo. Every function returns its result, or
// {error: "..."} when it failed. Build it with `make lab-wasm`.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"
	"unsafe"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

var l = lab.New()

func bytesIn(v js.Value) []byte {
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func u8(b []byte) js.Value {
	a := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(a, b)
	return a
}

// typed makes a typed array (Int16Array, Int32Array) from raw little-endian bytes.
func typed(kind string, raw []byte) js.Value {
	buf := u8(raw).Get("buffer")
	return js.Global().Get(kind).New(buf)
}

func i16(s []int16) js.Value {
	if len(s) == 0 {
		return js.Global().Get("Int16Array").New(0)
	}
	return typed("Int16Array", unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*2))
}

func i32(s []int) js.Value {
	t := make([]int32, len(s))
	for i, v := range s {
		t[i] = int32(v)
	}
	if len(t) == 0 {
		return js.Global().Get("Int32Array").New(0)
	}
	return typed("Int32Array", unsafe.Slice((*byte)(unsafe.Pointer(&t[0])), len(t)*4))
}

func fail(err error) any { return map[string]any{"error": err.Error()} }

// reg registers a function; a panic inside it becomes an error result.
func reg(api js.Value, name string, f func(a []js.Value) (any, error)) {
	api.Set(name, js.FuncOf(func(this js.Value, a []js.Value) (res any) {
		defer func() {
			if r := recover(); r != nil {
				res = fail(fmt.Errorf("%s: %v", name, r))
			}
		}()
		v, err := f(a)
		if err != nil {
			return fail(err)
		}
		return v
	}))
}

func str(b []byte, err error) (any, error) { return string(b), err }

func num(n int, err error) (any, error) { return n, err }

func none(err error) (any, error) { return true, err }

func main() {
	api := js.Global().Get("Object").New()
	// project
	reg(api, "reset", func(a []js.Value) (any, error) { l.Reset(); return true, nil })
	reg(api, "newProject", func(a []js.Value) (any, error) { l.NewProject(a[0].String()); return true, nil })
	reg(api, "begin", func(a []js.Value) (any, error) { l.Begin(); return true, nil })
	reg(api, "add", func(a []js.Value) (any, error) { l.Add(a[0].String(), bytesIn(a[1])); return true, nil })
	reg(api, "finish", func(a []js.Value) (any, error) { return num(l.Finish()) })
	reg(api, "loadZip", func(a []js.Value) (any, error) { return num(l.LoadZip(bytesIn(a[0]))) })
	reg(api, "exportZip", func(a []js.Value) (any, error) {
		b, err := l.ExportZip(a[0].String())
		return u8(b), err
	})
	reg(api, "padconf", func(a []js.Value) (any, error) {
		b, err := l.PadconfBytes()
		return u8(b), err
	})
	reg(api, "info", func(a []js.Value) (any, error) { return str(l.Info()) })
	reg(api, "setName", func(a []js.Value) (any, error) { return none(l.SetName(a[0].String())) })
	reg(api, "setTempo", func(a []js.Value) (any, error) { return none(l.SetTempo(a[0].Float())) })
	// samples
	reg(api, "decode", func(a []js.Value) (any, error) { return str(l.Decode(bytesIn(a[0]), a[1].String())) })
	reg(api, "meta", func(a []js.Value) (any, error) { return str(l.Meta(a[0].Int())) })
	reg(api, "padSample", func(a []js.Value) (any, error) { return str(l.PadSample(a[0].Int())) })
	reg(api, "pcm", func(a []js.Value) (any, error) {
		s, err := l.PCM(a[0].Int())
		return i16(s), err
	})
	reg(api, "wav", func(a []js.Value) (any, error) {
		b, err := l.WAV(a[0].Int())
		return u8(b), err
	})
	reg(api, "free", func(a []js.Value) (any, error) { l.Free(a[0].Int()); return true, nil })
	reg(api, "assign", func(a []js.Value) (any, error) { return none(l.Assign(a[0].Int(), a[1].Int(), a[2].String())) })
	reg(api, "clearPad", func(a []js.Value) (any, error) { return none(l.ClearPad(a[0].Int())) })
	reg(api, "renamePad", func(a []js.Value) (any, error) { return none(l.RenamePad(a[0].Int(), a[1].String())) })
	reg(api, "movePad", func(a []js.Value) (any, error) { return none(l.MovePad(a[0].Int(), a[1].Int())) })
	reg(api, "peaks", func(a []js.Value) (any, error) {
		p, err := l.Peaks(a[0].Int(), a[1].Int(), a[2].Int(), a[3].Int())
		return i32(p), err
	})
	reg(api, "transients", func(a []js.Value) (any, error) {
		p, err := l.Transients(a[0].Int(), a[1].Int(), a[2].Float())
		return i32(p), err
	})
	reg(api, "zeroCrossing", func(a []js.Value) (any, error) { return num(l.ZeroCrossing(a[0].Int(), a[1].Int(), a[2].Int())) })
	reg(api, "pitched", func(a []js.Value) (any, error) { return num(l.Pitched(a[0].Int(), a[1].Int())) })
	reg(api, "stretched", func(a []js.Value) (any, error) { return num(l.Stretched(a[0].Int(), a[1].Int())) })
	reg(api, "trim", func(a []js.Value) (any, error) { return num(l.Trim(a[0].Int(), a[1].Int(), a[2].Int())) })
	reg(api, "join", func(a []js.Value) (any, error) {
		var ids []int
		if err := json.Unmarshal([]byte(a[0].String()), &ids); err != nil {
			return nil, err
		}
		return num(l.Join(ids...))
	})
	reg(api, "normalize", func(a []js.Value) (any, error) { return num(l.Normalize(a[0].Int(), a[1].Float())) })
	reg(api, "reversed", func(a []js.Value) (any, error) { return num(l.Reversed(a[0].Int())) })
	reg(api, "stretchToBars", func(a []js.Value) (any, error) { return str(l.StretchToBars(a[0].Int(), a[1].Float())) })
	// slicer
	reg(api, "slicerConsts", func(a []js.Value) (any, error) { return str(lab.SlicerConsts()) })
	reg(api, "slicing", func(a []js.Value) (any, error) {
		return str(l.Slicing(a[0].String(), []byte(a[1].String()), []byte(a[2].String())))
	})
	reg(api, "render", func(a []js.Value) (any, error) {
		return str(l.Render(a[0].Int(), []byte(a[1].String()), a[2].Int(), a[3].Int()))
	})
	reg(api, "sliceAudio", func(a []js.Value) (any, error) {
		return str(l.SliceAudio(a[0].Int(), []byte(a[1].String()), a[2].Int()))
	})
	reg(api, "shuffleOrder", func(a []js.Value) (any, error) {
		return str(lab.ShuffleOrder([]byte(a[0].String()), int64(a[1].Int())))
	})
	// chords and arps
	reg(api, "arpConsts", func(a []js.Value) (any, error) { return str(lab.ArpConsts()) })
	reg(api, "defaultArp", func(a []js.Value) (any, error) { return str(json.Marshal(formats.DefaultArp())) })
	reg(api, "chordNotes", func(a []js.Value) (any, error) {
		return str(json.Marshal(lab.ChordNotes(a[0].Int(), a[1].Int(), a[2].Int(), a[3].Int())))
	})
	reg(api, "chordName", func(a []js.Value) (any, error) {
		var n []int
		if err := json.Unmarshal([]byte(a[0].String()), &n); err != nil {
			return nil, err
		}
		return lab.ChordName(n), nil
	})
	reg(api, "chordKit", func(a []js.Value) (any, error) {
		k, err := lab.ChordKit(a[0].Int(), a[1].Int(), a[2].Bool())
		if err != nil {
			return nil, err
		}
		return str(json.Marshal(k))
	})
	reg(api, "arpVariants", func(a []js.Value) (any, error) {
		return str(lab.ArpVariantsJSON([]byte(a[0].String()), a[1].Int(), int64(a[2].Int())))
	})
	reg(api, "renderArp", func(a []js.Value) (any, error) { return str(l.RenderArp(a[0].Int(), []byte(a[1].String()))) })
	// patterns
	reg(api, "pattern", func(a []js.Value) (any, error) { return str(l.Pattern(a[0].Int())) })
	reg(api, "newPattern", func(a []js.Value) (any, error) { return none(l.NewPattern(a[0].Int(), a[1].Int())) })
	reg(api, "deletePattern", func(a []js.Value) (any, error) { return none(l.DeletePattern(a[0].Int())) })
	reg(api, "copyPattern", func(a []js.Value) (any, error) { return none(l.CopyPattern(a[0].Int(), a[1].Int())) })
	reg(api, "patternOp", func(a []js.Value) (any, error) {
		return none(l.PatternOp(a[0].Int(), a[1].String(), []byte(a[2].String())))
	})
	reg(api, "writeProgression", func(a []js.Value) (any, error) {
		return none(l.WriteProgression(a[0].Int(), []byte(a[1].String())))
	})
	js.Global().Set("LabGo", api)
	select {}
}
