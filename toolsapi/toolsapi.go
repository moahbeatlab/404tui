// Package toolsapi is the page's way to the tools: one JSON call, an operation and what it needs, and the whole state back. The
// program serves it over HTTP (sp404 serve) and the demo on GitHub Pages runs it in the browser (cmd/demowasm).
package toolsapi

import "github.com/moahbeatlab/404tui/core"

// Request is one call: Op says what to do, the rest is what it needs.
type Request struct {
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

// Do does what the request says and returns what is to be sent back (the state, or the pads for "pads"); a failure has a status code
// (an HTTP one) and a text.
func Do(r *core.Remote, in Request) (v any, code int, msg string) {
	switch in.Op {
	case "state", "":
	case "open":
		r.OpenPatternTools()
	case "tab":
		r.Tab(in.Tab)
	case "set":
		r.Set(in.I, in.Val)
	case "run":
		r.Run(in.I)
	case "back":
		r.Back()
	case "pattern":
		r.SetPattern(in.Slot)
	case "track":
		r.Track(in.Pad)
	case "play":
		r.Play(in.On)
	case "undo":
		r.Undo()
	case "redo":
		r.Redo()
	case "load":
		r.LoadProject(in.I, in.Live)
	case "pads":
		return r.Pads(), 0, ""
	case "slicer":
		r.OpenSlicer(in.Pad)
	case "pattern-view":
		r.ShowPattern()
	case "slicer-resume":
		r.ResumeSlicer()
	case "slicer-close":
		r.CloseSlicer()
	case "slicer-view":
		r.SlicerView(in.From, in.To, in.Cols)
	case "slicer-at":
		r.SlicerAt(in.I)
	case "slicer-cut":
		r.SlicerCut(in.I)
	case "slicer-join":
		r.SlicerJoin(in.I)
	case "slicer-play":
		r.SlicerPlay(in.I)
	case "slicer-order":
		r.SlicerOrder(in.On)
	case "slicer-shuffle":
		r.SlicerShuffle()
	case "slicer-menu":
		r.SlicerMenu(in.Kind)
	case "slicer-undo":
		r.SlicerUndo(in.On)
	case "pad-tools":
		r.OpenPadTools(in.Pad)
	case "picker-bank":
		r.PickerBank(in.I)
	case "picker-pick":
		r.PickerPick(in.I)
	case "prompt":
		r.PromptSet(in.Text, in.Done, in.On)
	case "close-menus":
		r.CloseMenus()
	case "escape":
		r.Escape()
	case "reload":
		r.ReloadUnit()
	case "live":
		r.SetLive(in.On)
	case "send":
		if msg := r.SendToUnit(in.I); msg != "" {
			return nil, 409, msg
		}
	default:
		return nil, 400, "unknown operation"
	}
	return r.State(), 0, ""
}
