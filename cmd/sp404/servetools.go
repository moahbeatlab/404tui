package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/device"
)

// The tools of the program in the web page (sp404 serve): the program (package core) runs beside the old part that serves Perform, in the same process,
// with the same connection to the unit (two programs cannot hold the serial port), and the page talks to it through core.Remote.

// sharedUnit remembers the device session that the old part opened, for the new part to use as well.
type sharedUnit struct {
	mu  sync.Mutex
	dev *device.Device
}

func (s *sharedUnit) set(d *device.Device) {
	s.mu.Lock()
	s.dev = d
	s.mu.Unlock()
}

func (s *sharedUnit) get() *device.Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dev
}

// remembering wraps the old part's way of finding the unit so that the session it opens is known here.
func (s *sharedUnit) remembering(c func() (app.Unit, io.Closer, error)) func() (app.Unit, io.Closer, error) {
	return func() (app.Unit, io.Closer, error) {
		u, cl, err := c()
		if err == nil {
			if d, ok := u.(*device.Device); ok {
				s.set(d)
			}
		}
		return u, cl, err
	}
}

// newToolsApp makes the program for the page: no screen, no MIDI (the old part listens to the unit's), the unit through the shared session.
func newToolsApp(o options, shared *sharedUnit, player app.Player) *core.Remote {
	core.Terminal = true
	var au core.Audio = &core.NullAudio{}
	if player != nil {
		au = player
	}
	os.MkdirAll(o.samples, 0o755)
	os.MkdirAll(o.out, 0o755)
	a := core.New(o.samples, o.out, au)
	a.SetPlatform(core.Platform{Name: runtime.GOOS, Keyboard: true})
	a.Connect = func() (core.Link, error) {
		if d := shared.get(); d != nil {
			return core.DeviceLink{Dev: d}, nil
		}
		return nil, io.ErrClosedPipe
	}
	r := core.NewRemote(a)
	r.Follow(true, true) // the page shows the project that is on the unit and edits it live: what is kept goes to the unit (COPY on the page keeps it off)
	return r
}

// toolsAPI is the page's way to the tools: one JSON call, an operation and what it needs, and the whole state back.
type toolsAPI struct{ r *core.Remote }

func (t toolsAPI) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var in struct {
		Op   string `json:"op"`
		I    int    `json:"i"`
		Val  int    `json:"val"`
		Slot int    `json:"slot"`
		Tab  int    `json:"tab"`
		Pad  int    `json:"pad"`
		On   bool   `json:"on"`
		Live bool   `json:"live"`
		From int    `json:"from"`
		To   int    `json:"to"`
		Cols int    `json:"cols"`
		Text string `json:"text"`
		Done bool   `json:"done"`
		Kind string `json:"kind"`
	}
	if req.Method == http.MethodPost {
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(w, "POST JSON", http.StatusBadRequest)
			return
		}
	} else {
		in.Op = req.URL.Query().Get("op")
	}
	switch in.Op {
	case "state", "":
	case "open":
		t.r.OpenPatternTools()
	case "tab":
		t.r.Tab(in.Tab)
	case "set":
		t.r.Set(in.I, in.Val)
	case "run":
		t.r.Run(in.I)
	case "back":
		t.r.Back()
	case "pattern":
		t.r.SetPattern(in.Slot)
	case "track":
		t.r.Track(in.Pad)
	case "play":
		t.r.Play(in.On)
	case "undo":
		t.r.Undo()
	case "redo":
		t.r.Redo()
	case "load":
		t.r.LoadProject(in.I, in.Live)
	case "pads":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(t.r.Pads())
		return
	case "slicer":
		t.r.OpenSlicer(in.Pad)
	case "slicer-close":
		t.r.CloseSlicer()
	case "slicer-view":
		t.r.SlicerView(in.From, in.To, in.Cols)
	case "slicer-at":
		t.r.SlicerAt(in.I)
	case "slicer-cut":
		t.r.SlicerCut(in.I)
	case "slicer-join":
		t.r.SlicerJoin(in.I)
	case "slicer-play":
		t.r.SlicerPlay(in.I)
	case "slicer-menu":
		t.r.SlicerMenu(in.Kind)
	case "slicer-undo":
		t.r.SlicerUndo(in.On)
	case "pad-tools":
		t.r.OpenPadTools(in.Pad)
	case "picker-bank":
		t.r.PickerBank(in.I)
	case "picker-pick":
		t.r.PickerPick(in.I)
	case "prompt":
		t.r.PromptSet(in.Text, in.Done, in.On)
	case "escape":
		t.r.Escape()
	case "reload":
		t.r.ReloadUnit()
	case "live":
		t.r.SetLive(in.On)
	case "send":
		if msg := t.r.SendToUnit(in.I); msg != "" {
			http.Error(w, msg, http.StatusConflict)
			return
		}
	default:
		http.Error(w, "unknown operation", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(t.r.State())
}
