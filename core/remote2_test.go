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

func TestARemoteFollowsTheUnitsCurrentProject(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	r := NewRemote(a)
	r.Follow(true, false)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.Do(func(a *App) { a.connAt = time.Time{}; a.Tick() })
		r.followTick()
		r.Do(func(a *App) { a.Tick() })
		if st := r.State(); st.Project > 0 && st.Job == nil || (st.Job != nil && st.Job.Ended && st.Project > 0) {
			if st.Mode != "copy" || len(st.Patterns) != 16 || !st.Follow {
				t.Fatalf("the project is in: %+v", st)
			}
			// the project is not loaded again and again
			before := a.job
			r.followTick()
			if a.job != before {
				t.Fatal("loaded again")
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("the unit's current project was not loaded")
}

func TestKeepItKeepsTheTrialAndGoesBack(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.toggle(0, 0)
	r := NewRemote(a)
	r.SetPattern(1)
	m := r.OpenPatternTools()
	notes := 0
	for i, n := range m.Tabs {
		if n == "NOTES" || n == "Notes" {
			notes = i
		}
	}
	m = r.Tab(notes)
	d := r.Run(findItem(m, "Fill"))
	if d == nil || findItem(d, "Keep it") < 0 {
		t.Fatalf("Fill's dialog: %+v", d)
	}
	title := d.Title
	n := len(r.Pattern().Tracks[0].Notes)
	back := r.Run(findItem(d, "Keep it"))
	if back == nil || back.Title == title {
		t.Fatalf("after Keep it the menu is the one before: %+v", back)
	}
	if got := len(r.Pattern().Tracks[0].Notes); got != n {
		t.Fatalf("the kept notes stay: %d, was %d", got, n)
	}
	r.Undo()
	if got := len(r.Pattern().Tracks[0].Notes); got >= n {
		t.Fatalf("one undo takes the tool back: %d, was %d", got, n)
	}
}
