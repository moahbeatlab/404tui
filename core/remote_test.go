package core

import (
	"strings"
	"testing"
)

// findItem is the entry of the menu with this label (a prefix), or -1.
func findItem(m *RMenu, label string) int {
	if m == nil {
		return -1
	}
	for _, it := range m.Items {
		if strings.HasPrefix(it.Label, label) {
			return it.Index
		}
	}
	return -1
}

func TestARemoteOpensATool_ChangesIt_AndThrowsItAway(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.toggle(0, 0) // a pattern with one note on the first pad
	r := NewRemote(a)

	p := r.SetPattern(1)
	if p.Label != "A1" || len(p.Tracks) == 0 {
		t.Fatalf("the pattern: %+v", p)
	}
	was := len(r.Pattern().Tracks[0].Notes)

	m := r.OpenPatternTools()
	if m == nil || len(m.Tabs) < 4 {
		t.Fatalf("the tools menu has its pages: %+v", m)
	}
	// the Notes page lists the tools; Fill opens its dialog
	var notes int
	for i, n := range m.Tabs {
		if strings.EqualFold(n, "notes") {
			notes = i
		}
	}
	m = r.Tab(notes)
	fill := findItem(m, "Fill")
	if fill < 0 {
		t.Fatalf("no Fill on the Notes page: %+v", m)
	}
	d := r.Run(fill)
	if d == nil || !strings.HasPrefix(strings.ToUpper(d.Title), "FILL") {
		t.Fatalf("Fill opened its dialog: %+v", d)
	}
	// the trial is already in the pattern
	if got := len(r.Pattern().Tracks[0].Notes); got <= was {
		t.Fatalf("the trial has notes: %d, was %d", got, was)
	}
	// a setting is a choice: changing it applies the tool again
	var choice *RItem
	for i := range d.Items {
		if d.Items[i].Kind == "choice" {
			choice = &d.Items[i]
			break
		}
	}
	if choice == nil || len(choice.Vals) < 2 {
		t.Fatalf("Fill has settings: %+v", d)
	}
	before := len(r.Pattern().Tracks[0].Notes)
	next := (choice.Val + 1) % len(choice.Vals)
	d = r.Set(choice.Index, next)
	for _, it := range d.Items {
		if it.Index == choice.Index && it.Val != next {
			t.Fatalf("the choice is %d, wanted %d", it.Val, next)
		}
	}
	if len(r.Pattern().Tracks[0].Notes) == before {
		t.Log("(the other rhythm has as many notes)")
	}
	// back throws the trial away
	m = r.Back()
	if m == nil || m.Title == d.Title {
		t.Fatalf("back is the tools menu: %+v", m)
	}
	if got := len(r.Pattern().Tracks[0].Notes); got != was {
		t.Fatalf("the trial is gone: %d notes, was %d", got, was)
	}
	// and the menu closes
	if r.Back() != nil {
		t.Fatal("the menu is closed")
	}
}

func TestARemoteKeepsATrial(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.toggle(0, 0) // a pattern with one note on the first pad
	r := NewRemote(a)
	r.SetPattern(1)
	was := len(r.Pattern().Tracks[0].Notes)
	m := r.OpenPatternTools()
	for i, n := range m.Tabs {
		if strings.EqualFold(n, "notes") {
			m = r.Tab(i)
		}
	}
	d := r.Run(findItem(m, "Euclid"))
	if d == nil {
		t.Fatal("Euclid opened")
	}
	keep := findItem(d, "Keep it")
	if keep < 0 {
		t.Fatalf("a trial has Keep it: %+v", d)
	}
	after := len(r.Pattern().Tracks[0].Notes)
	if after <= was {
		t.Fatalf("the trial: %d, was %d", after, was)
	}
	r.Run(keep)
	r.Back()
	r.Back()
	if got := len(r.Pattern().Tracks[0].Notes); got != after {
		t.Fatalf("kept: %d notes, wanted %d", got, after)
	}
	// undo takes it back as one step
	a.undoStep()
	if got := len(r.Pattern().Tracks[0].Notes); got != was {
		t.Fatalf("undone: %d, was %d", got, was)
	}
}

func TestARemoteSaysWhatItDoesNotKnow(t *testing.T) {
	a, _ := newApp(t)
	r := NewRemote(a)
	if r.Menu() != nil || r.Set(3, 1) != nil || r.Run(0) != nil || r.Back() != nil || r.Tab(1) != nil {
		t.Fatal("with no menu open there is nothing")
	}
	if p := r.SetPattern(200); p.Slot == 200 {
		t.Fatal("a slot out of range is not taken")
	}
}
