package core

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/moahbeatlab/404tui/lab"
)

// More of the Remote: the slicer, the tools of a pad, the dialogs a menu may open (a pad or pattern to choose, a text to type) and the project's
// send to the unit. The pages of tools.js draw these from the data below; the program does the work.

// RSlice is a slice of the sample: its frames and what is set on it.
type RSlice struct {
	Start   int  `json:"start"`
	End     int  `json:"end"`
	Pitch   int  `json:"pitch"`
	Gain    int  `json:"gain"`
	Chance  int  `json:"chance"`
	Roll    int  `json:"roll,omitempty"`
	Reverse bool `json:"reverse,omitempty"`
}

// RSlicer is the sample in the slicer, with the peaks of the window from..to.
type RSlicer struct {
	Pad     int      `json:"pad"`
	Label   string   `json:"label"`
	Name    string   `json:"name"`
	Frames  int      `json:"frames"`
	Rate    int      `json:"rate"`
	At      int      `json:"at"`  // the cursor
	Sel     int      `json:"sel"` // the slice under the cursor, -1 for none
	From    int      `json:"from"`
	To      int      `json:"to"`
	Peaks   []int    `json:"peaks"` // 0..32767 for each column of the window
	Mins    []int    `json:"mins"`  // the lowest and the highest sample of each column (-32768..32767)
	Maxs    []int    `json:"maxs"`
	RMS     []int    `json:"rms"` // how loud each column is on the whole (0..32767)
	Slices  []RSlice `json:"slices"`
	Order   []int    `json:"order"`
	Mode    string   `json:"mode"`
	CanUndo bool     `json:"canUndo"`
	CanRedo bool     `json:"canRedo"`
}

// RCell is a cell of a picker: a pad or a pattern.
type RCell struct {
	Used bool   `json:"used"`
	Name string `json:"name,omitempty"`
	Info string `json:"info,omitempty"`
}

// RPicker is the dialog in which a pad or a pattern is chosen.
type RPicker struct {
	Title string  `json:"title"`
	Pads  bool    `json:"pads"`
	Bank  int     `json:"bank"`
	Cur   int     `json:"cur"`
	Cells []RCell `json:"cells"`
}

// RPrompt is a text that is asked for.
type RPrompt struct {
	Title string `json:"title"`
	Hint  string `json:"hint,omitempty"`
	Text  string `json:"text"`
}

// RPad is a pad of the project.
type RPad struct {
	Pad  int     `json:"pad"`
	Name string  `json:"name"`
	Has  bool    `json:"has"` // a sample
	Secs float64 `json:"secs,omitempty"`
}

// Other names a dialog of the program that the page cannot show (the file picker, the pad panel...), or "".
func (a *App) remoteOther() string {
	switch {
	case a.dp != nil:
		return "a folder picker"
	case a.kb != nil:
		return "the search keyboard"
	case a.tap:
		return "the slicer's tap"
	case a.lane:
		return "the order lane"
	}
	return ""
}

func (a *App) remotePicker() *RPicker {
	p := a.picker
	if p == nil {
		return nil
	}
	out := &RPicker{Title: p.title, Pads: p.pads, Bank: p.bank, Cur: p.cur, Cells: make([]RCell, 16)}
	if out.Title == "" {
		out.Title = "Choose a pattern"
	}
	for i, c := range p.info {
		out.Cells[i] = RCell{Used: c.used, Name: c.name}
		if !p.pads && c.used {
			out.Cells[i].Info = fmt.Sprintf("%d bars, %d notes", c.bars, c.notes)
		}
	}
	return out
}

func (a *App) remotePrompt() *RPrompt {
	if a.prompt == nil {
		return nil
	}
	return &RPrompt{Title: a.prompt.title, Hint: a.prompt.hint, Text: a.prompt.buf}
}

func (a *App) remoteSlicer(from, to, cols int) *RSlicer {
	if a.sl == nil || a.scr != Slice {
		return nil
	}
	s := &RSlicer{Pad: a.spad, Label: a.label(a.spad), Name: a.info.Pads[a.spad].Name, Frames: a.sl.Frames, Rate: 48000, At: a.at, Sel: a.sliceAt(a.at),
		Order: a.sl.Order, Mode: modeNames[int(a.sl.Mode)%len(modeNames)], Slices: []RSlice{}, CanUndo: len(a.slUndo) > 0, CanRedo: len(a.slRedo) > 0}
	if to <= from {
		from, to = 0, a.sl.Frames
	}
	from, to = max(from, 0), min(to, a.sl.Frames)
	cols = min(max(cols, 50), 4000)
	s.From, s.To = from, to
	if pk, err := a.peaks(a.sid, from, to, cols); err == nil {
		s.Peaks = pk
	}
	s.Mins, s.Maxs, s.RMS = a.waveform(from, to, cols)
	for _, sl := range a.sl.Slices {
		s.Slices = append(s.Slices, RSlice{Start: sl.Start, End: sl.End, Pitch: sl.Pitch, Gain: sl.Gain, Chance: sl.Chance, Roll: sl.Roll, Reverse: sl.Reverse})
	}
	return s
}

// key presses a key the way the screens get it; on the slicer through Press, which keeps what it changed for undo.
func (a *App) remoteKey(k Key) {
	if a.scr == Slice {
		a.pp, a.np = nil, nil // a panel left open on the pads would take the key
		a.Press(k, 0)
		return
	}
	if a.menu != nil {
		a.menuKey(k)
		return
	}
	a.Press(k, 0)
}

// Pads are the pads of the project (160): which have a sample, and its name.
func (r *Remote) Pads() []RPad {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RPad, 0, 160)
	for i, p := range r.a.info.Pads {
		out = append(out, RPad{Pad: i, Name: p.Name, Has: p.File, Secs: float64(p.Frames) / 48000})
	}
	return out
}

// OpenSlicer takes the sample of a pad into the slicer.
func (r *Remote) OpenSlicer(pad int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.a
	if pad < 0 || pad > 159 {
		return
	}
	a.menu, a.picker, a.prompt, a.pp, a.np = nil, nil, nil, nil, nil
	r.view = [3]int{}
	a.bank, a.cur = pad/16, pad%16
	a.go_(Slice)
	a.openSlicer()
	if a.sl == nil {
		a.go_(Pads)
	}
}

// CloseSlicer goes back to the pads.
func (r *Remote) CloseSlicer() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.a.menu, r.a.picker, r.a.prompt = nil, nil, nil
	r.a.go_(Pads)
}

// SlicerAt moves the cursor to a frame.
func (r *Remote) SlicerAt(frame int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.a; a.sl != nil && a.scr == Slice {
		a.at = min(max(frame, 0), a.sl.Frames-1)
	}
}

// SlicerCut cuts the slice at a frame (A: the cut is moved to a zero crossing, as in the terminal).
func (r *Remote) SlicerCut(frame int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.a; a.sl != nil && a.scr == Slice && a.menu == nil {
		a.at = min(max(frame, 0), a.sl.Frames-1)
		a.remoteKey(A)
	}
}

// SlicerJoin joins the slice at a frame with the one before it (B).
func (r *Remote) SlicerJoin(frame int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.a; a.sl != nil && a.scr == Slice && a.menu == nil && len(a.sl.Slices) > 1 {
		a.at = min(max(frame, 0), a.sl.Frames-1)
		a.remoteKey(B)
	}
}

// SlicerPlay plays the slice at a frame on the computer's speakers.
func (r *Remote) SlicerPlay(frame int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.a; a.sl != nil && a.scr == Slice {
		a.at = min(max(frame, 0), a.sl.Frames-1)
		a.playSlice()
	}
}

// SlicerMenu opens the settings (kind "settings") or the tools (kind "tools") of the slicer.
func (r *Remote) SlicerMenu(kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.a
	if a.sl == nil || a.scr != Slice {
		return
	}
	if kind == "settings" {
		a.sliceMenu()
	} else {
		a.sliceToolsMenu()
	}
}

// SlicerUndo takes back the last change of the slicing (or brings it back).
func (r *Remote) SlicerUndo(redo bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.a; a.sl != nil && a.scr == Slice {
		a.slicerUndo(redo)
	}
}

// OpenPadTools opens the tools of a pad: what is done with its sample, and the pad itself.
func (r *Remote) OpenPadTools(pad int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.a
	if pad < 0 || pad > 159 {
		return
	}
	a.menu, a.picker, a.prompt, a.pp, a.np = nil, nil, nil, nil, nil
	if a.scr != Pads {
		a.go_(Pads)
	}
	a.bank, a.cur = pad/16, pad%16
	a.menu = a.padToolsMenu(pad)
}

// PickerBank turns the dialog to a bank.
func (r *Remote) PickerBank(bank int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.a.picker; p != nil && bank >= 0 && bank < 10 {
		p.bank = bank
		r.a.pickerLoad()
	}
}

// PickerPick chooses a cell (0 to 15) of the dialog, as A does.
func (r *Remote) PickerPick(cell int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.a.picker; p != nil && cell >= 0 && cell < 16 {
		p.cur = cell
		r.a.remoteKey(A)
	}
}

// PromptSet puts a text in the prompt; with done it is finished (or, with ok false, left).
func (r *Remote) PromptSet(text string, done, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.a.prompt; p != nil {
		p.buf = text
		if done {
			r.a.closePrompt(ok)
		}
	}
}

// SlicerView sets the window of the waveform (frames from..to) and how many columns of peaks the page wants.
func (r *Remote) SlicerView(from, to, cols int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.view = [3]int{from, to, cols}
}

// Escape leaves whatever is open: a dialog, a text, a menu (as B does, so a trial is thrown away).
func (r *Remote) Escape() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.a.remoteEscape()
}

func (a *App) remoteEscape() {
	switch {
	case a.prompt != nil:
		a.closePrompt(false)
	case a.picker != nil:
		a.picker = nil
	case a.menu != nil:
		a.remoteKey(B)
	case a.dp != nil || a.kb != nil:
		a.dp, a.kb = nil, nil
	case a.tap || a.lane:
		a.tap, a.lane = false, false
	}
}

// SendToUnit writes the editor's project over project n (0 based) of the unit, in the background (the state has the progress). The page asks first.
func (r *Remote) SendToUnit(n int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.a
	switch {
	case a.link == nil:
		return "no unit"
	case n < 0 || n > 15:
		return "no such project"
	case a.live != nil && a.live.project == n:
		return "this project is edited live: every change is on the unit already"
	case a.job != nil && func() bool { _, _, _, _, fin := a.job.state(); return !fin }():
		return "the unit is busy"
	}
	a.unitCur = n
	a.send()
	return ""
}

// waveform is the shape of the sample in the window: for each column the lowest and highest sample and the RMS, the channels mixed to one. The last
// window is kept, so a page that asks every second for the same one does not make it again.
func (a *App) waveform(from, to, cols int) (mins, maxs, rms []int) {
	k := waveKey{a.sid, from, to, cols}
	if a.waveMemo.key == k && a.waveMemo.mins != nil {
		return a.waveMemo.mins, a.waveMemo.maxs, a.waveMemo.rms
	}
	smp, err := a.lab.Sample(a.sid)
	if err != nil || smp.Frames() == 0 {
		return nil, nil, nil
	}
	ch := max(smp.Channels, 1)
	mins, maxs, rms = make([]int, cols), make([]int, cols), make([]int, cols)
	for c := 0; c < cols; c++ {
		f0 := from + int(int64(c)*int64(to-from)/int64(cols))
		f1 := max(from+int(int64(c+1)*int64(to-from)/int64(cols)), f0+1)
		lo, hi, sum, n := 32767, -32768, 0.0, 0
		step := max((f1-f0)/512, 1) // a column of a long sample is looked at in 512 places at most
		for f := f0; f < f1 && f < smp.Frames(); f += step {
			v := 0
			for j := 0; j < ch; j++ {
				v += int(smp.Samples[f*ch+j])
			}
			v /= ch
			lo, hi = min(lo, v), max(hi, v)
			sum += float64(v) * float64(v)
			n++
		}
		if n == 0 {
			lo, hi = 0, 0
		} else {
			rms[c] = int(math.Sqrt(sum / float64(n)))
		}
		mins[c], maxs[c] = lo, hi
	}
	a.waveMemo = waveCache{key: k, mins: mins, maxs: maxs, rms: rms}
	return
}

type waveKey struct{ sid, from, to, cols int }
type waveCache struct {
	key             waveKey
	mins, maxs, rms []int
}

// follow is the editor following the unit's current project: what it loaded, in which mode, and when it tried last.
type follow struct {
	on       bool
	live     bool // what is wanted: edit live (else a copy)
	loaded   int  // the project in the editor (0 based), -1 for none
	liveMode bool // the mode it was loaded in
	tried    time.Time
	polled   time.Time
	holding  bool // the unit plays: the reload waits for its Stop
	reload   bool // something was kept: when it has all been sent the unit is told to read the project again
}

// Follow makes the editor follow the unit's current project: the page shows the project that is loaded on the unit, with its pads and patterns,
// and opens the other one when the unit's project is changed. With live, every change that is kept goes to the unit; else the project is a copy.
func (r *Remote) Follow(on, live bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fl = follow{on: on, live: live, loaded: -1}
}

// SetLive changes how the current project is held (live or a copy); it is loaded again in that mode.
func (r *Remote) SetLive(live bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fl.live != live {
		r.fl.live, r.fl.loaded = live, -1
	}
}

// followTick looks at the unit's current project and loads it when it is not the one in the editor.
func (r *Remote) followTick() {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, f := r.a, &r.fl
	if !f.on {
		return
	}
	dev, ok := a.unitDevice()
	if !ok {
		f.loaded = -1
		return
	}
	if a.job != nil {
		if _, _, _, _, fin := a.job.state(); !fin {
			return
		}
	}
	cur, err := dev.CurrentProject()
	if err != nil || int(cur) > 15 {
		return
	}
	n := int(cur)
	stale := f.loaded != n || (f.live && !f.liveMode) || (!f.live && f.liveMode)
	if !stale && f.live && a.live == nil && time.Since(f.tried) > 10*time.Second {
		stale = true // live editing stopped by itself
	}
	if !stale || time.Since(f.tried) < 3*time.Second {
		return
	}
	f.tried = time.Now()
	f.loaded, f.liveMode = n, f.live
	a.menu, a.picker, a.prompt = nil, nil, nil
	a.unitLoadNow(dev, n, f.live) // the unit's project replaces what is in the editor: it follows the unit
}

// remotePatterns are the 16 patterns of the bank in front: which have notes.
func (a *App) remotePatterns() []RCell {
	bank := (a.pat - 1) / 16
	out := make([]RCell, 16)
	for i := range out {
		slot := bank*16 + i + 1
		b, err := a.lab.Pattern(slot)
		var d lab.PatternDoc
		if err != nil || json.Unmarshal(b, &d) != nil {
			continue
		}
		out[i] = RCell{Used: true, Info: fmt.Sprintf("%d bars, %d notes", d.Bars, len(d.Notes))}
	}
	return out
}

// reloadTick makes the unit read the project again once what was kept has been sent: a pattern written to its card shows on the unit only then
// (this stops the sound there). A page that edits live does it by itself; the terminal has it on a key (Reload on the unit).
func (r *Remote) reloadTick() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.fl.reload {
		return
	}
	a := r.a
	if a.live == nil {
		r.fl.reload = false
		return
	}
	if waiting, _, err := a.liveStatus(); waiting > 0 {
		return
	} else if err != nil {
		r.fl.reload = false // what was kept did not get there: the message says so
		return
	}
	// The unit does not read the project again while it plays: that stops it, and holds the serial line for a while. It waits for the unit's Stop.
	if dev, ok := a.unitDevice(); ok {
		if time.Since(r.fl.polled) < time.Second {
			return
		}
		r.fl.polled = time.Now()
		if raw, err := dev.RawStatus(); err == nil && len(raw) > 12 && raw[12] != 0 {
			if !r.fl.holding {
				a.say("the unit is playing: the pattern is written; it shows when the unit has stopped and reads the project again (RELOAD UNIT)", false)
			}
			r.fl.holding = true
			return
		}
	}
	r.fl.reload, r.fl.holding = false, false
	a.reloadLive()
}

// ReloadUnit makes the unit read the project that is edited live again now (patterns written to it show; the sound stops).
func (r *Remote) ReloadUnit() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fl.reload, r.fl.holding = false, false
	r.a.reloadLive()
}
