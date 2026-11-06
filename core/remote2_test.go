package core

import (
	"testing"
	"time"
)

func TestARemoteSlicesASample(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
	r := NewRemote(a)

	r.OpenSlicer(1)
	st := r.State()
	if st.Screen != "slicer" || st.Slicer == nil || len(st.Slicer.Slices) != 1 || len(st.Slicer.Peaks) < 50 {
		t.Fatalf("the slicer is open: %+v", st.Slicer)
	}
	frames := st.Slicer.Frames
	// a cut at the middle
	r.SlicerCut(frames / 2)
	st = r.State()
	if len(st.Slicer.Slices) != 2 || !st.Slicer.CanUndo {
		t.Fatalf("a cut: %d slices, undo %v (%q, %d frames)", len(st.Slicer.Slices), st.Slicer.CanUndo, st.Msg, frames)
	}
	// a window
	r.SlicerView(0, frames/4, 100)
	if s := r.State().Slicer; s.To != frames/4 || len(s.Peaks) != 100 {
		t.Fatalf("the window: %d..%d, %d peaks", s.From, s.To, len(s.Peaks))
	}
	// join and undo
	r.SlicerJoin(frames - 10)
	if n := len(r.State().Slicer.Slices); n != 1 {
		t.Fatalf("joined: %d slices", n)
	}
	r.SlicerUndo(false)
	if n := len(r.State().Slicer.Slices); n != 2 {
		t.Fatalf("the join is taken back: %d slices", n)
	}
	r.SlicerUndo(true)
	// the tools: cut in equal parts
	r.SlicerMenu("tools")
	m := r.Menu()
	if m == nil || findItem(m, "Cut in equal parts") < 0 {
		t.Fatalf("the slicer's tools: %+v", m)
	}
	r.Run(findItem(m, "Cut in equal parts"))
	if n := len(r.State().Slicer.Slices); n < 4 {
		t.Fatalf("equal parts: %d slices", n)
	}
	// a menu entry that asks for a pad opens a dialog the page can answer
	r.SlicerMenu("tools")
	keep := 0
	for i, n := range r.Menu().Tabs {
		if n == "KEEP" {
			keep = i
		}
	}
	r.Run(findItem(r.Tab(keep), "Result to a pad"))
	st = r.State()
	if st.Picker == nil || !st.Picker.Pads || len(st.Picker.Cells) != 16 {
		t.Fatalf("a pad is asked for: %+v", st.Picker)
	}
	r.PickerBank(2)
	r.PickerPick(4) // C5
	st = r.State()
	if st.Picker != nil {
		t.Fatal("the dialog is closed")
	}
	if !a.info.Pads[2*16+4].File {
		t.Fatalf("the result is on C5: %s", st.Msg)
	}
	r.CloseSlicer()
	if r.State().Screen != "pattern" {
		t.Fatal("closed")
	}
}

func TestARemoteOpensTheToolsOfAPad_AndAnswersATextAsked(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
	r := NewRemote(a)
	if p := r.Pads(); len(p) != 160 || !p[1].Has {
		t.Fatalf("pads: %d", len(p))
	}
	r.OpenPadTools(1)
	m := r.Menu()
	if m == nil || findItem(m, "Rename") < 0 {
		t.Fatalf("the tools of a pad: %+v", m)
	}
	r.Run(findItem(m, "Rename"))
	st := r.State()
	if st.Prompt == nil {
		t.Fatalf("a name is asked for: %+v", st.Menu)
	}
	r.PromptSet("snare", true, true)
	if a.info.Pads[1].Name != "snare" || r.State().Prompt != nil {
		t.Fatalf("renamed: %q", a.info.Pads[1].Name)
	}
	// Escape leaves a dialog
	r.OpenPadTools(1)
	r.Run(findItem(r.Menu(), "Rename"))
	r.Escape()
	if r.State().Prompt != nil {
		t.Fatal("the prompt is left")
	}
}

func TestARemoteSendsTheProjectToTheUnit(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
	r := NewRemote(a)
	if msg := r.SendToUnit(3); msg != "no unit" {
		t.Fatalf("without a unit: %q", msg)
	}
	a.Tick()
	deadline := time.Now().Add(5 * time.Second)
	for a.link == nil && time.Now().Before(deadline) {
		a.connAt = time.Time{}
		a.Tick()
		time.Sleep(20 * time.Millisecond)
	}
	if a.link == nil {
		t.Fatal("no unit found")
	}
	if msg := r.SendToUnit(3); msg != "" {
		t.Fatalf("send: %q", msg)
	}
	for time.Now().Before(deadline) {
		a.Tick()
		if j := r.State().Job; j != nil && j.Ended {
			if j.Error != "" {
				t.Fatalf("the send failed: %s", j.Error)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the send did not end")
}
