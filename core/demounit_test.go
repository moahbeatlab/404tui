package core

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/demounit"
	"github.com/moahbeatlab/404tui/proto"
)

func TestAKeptToolShowsInThePatternOfTheDemoUnit(t *testing.T) {
	termOn(t)
	dev := demounit.New()
	a := New(t.TempDir(), t.TempDir(), &NullAudio{})
	a.Connect = func() (Link, error) { return DeviceLink{Dev: dev}, nil }
	r := NewRemote(a)
	r.Follow(true, true)
	stop := make(chan struct{})
	defer close(stop)
	go r.Loop(stop)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && r.State().Live == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if r.State().Live == 0 {
		t.Fatalf("not live: %+v", r.State().Msg)
	}
	count := func() (n int) {
		for _, tr := range r.Pattern().Tracks {
			n += len(tr.Notes)
		}
		return
	}
	r.SetPattern(1)
	was := count()
	m := r.OpenPatternTools()
	for i, n := range m.Tabs {
		if strings.EqualFold(n, "notes") {
			m = r.Tab(i)
		}
	}
	d := r.Run(findItem(m, "Echo"))
	dump := func() string {
		var b strings.Builder
		for _, tr := range r.Pattern().Tracks {
			b.WriteString(tr.Label + "=" + strconv.Itoa(len(tr.Notes)) + " ")
		}
		return b.String()
	}
	t.Logf("tracks: %s", dump())
	time.Sleep(2 * time.Second)
	t.Logf("trial: %s; %s", dump(), r.State().Msg)
	r.Run(findItem(d, "Keep it"))
	r.CloseMenus()
	time.Sleep(3 * time.Second)
	t.Logf("kept: %d notes on A1; %s", count(), r.State().Msg)
	if count() <= was {
		t.Errorf("the notes are not in the pattern: %d, was %d", count(), was)
	}
}

func TestThePatternTheEditorLoadedIsTheOneOnTheUnit(t *testing.T) {
	termOn(t)
	dev := demounit.New()
	a := New(t.TempDir(), t.TempDir(), &NullAudio{})
	a.Connect = func() (Link, error) { return DeviceLink{Dev: dev}, nil }
	r := NewRemote(a)
	r.Follow(true, true)
	stop := make(chan struct{})
	defer close(stop)
	go r.Loop(stop)
	for i := 0; i < 600 && r.State().Live == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	var have []byte
	r.Do(func(a *App) { have, _ = a.lab.PatternBytes(1) })
	onUnit, err := dev.ReadFile("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
	t.Logf("editor %d bytes, unit %d bytes (%v)", len(have), len(onUnit), err)
	if len(have) != len(onUnit) {
		t.Errorf("the editor holds %d bytes of the pattern, the unit %d", len(have), len(onUnit))
	}
}

func TestASliceOrderBouncedToAPadShowsOnThePadsAndTheUnit(t *testing.T) {
	termOn(t)
	dev := demounit.New()
	a := New(t.TempDir(), t.TempDir(), &NullAudio{})
	a.Connect = func() (Link, error) { return DeviceLink{Dev: dev}, nil }
	r := NewRemote(a)
	r.Follow(true, true)
	stop := make(chan struct{})
	defer close(stop)
	go r.Loop(stop)
	for i := 0; i < 600 && r.State().Live == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	r.OpenSlicer(15)
	time.Sleep(time.Second)
	r.SlicerMenu("tools")
	m := r.Menu()
	for ti, n := range m.Tabs {
		if strings.EqualFold(n, "keep") {
			m = r.Tab(ti)
		}
	}
	i := findItem(m, "Result to a pad")
	if i < 0 {
		t.Fatalf("no such tool: %+v", m)
	}
	r.Run(i)
	st := r.State()
	if st.Picker == nil {
		t.Fatalf("no pad is asked for: %s / %+v", st.Msg, st.Menu)
	}
	t.Logf("picker: %+v", st.Picker)
	r.PickerBank(4)
	r.PickerPick(0)
	t.Logf("after the pick: %s", r.State().Msg)
	deadline := time.Now().Add(20 * time.Second)
	var has bool
	for time.Now().Before(deadline) {
		if p := r.Pads(); len(p) == 160 && p[64].Has {
			has = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !has {
		t.Fatalf("the pad E1 does not show a sample: %s", r.State().Msg)
	}
	time.Sleep(4 * time.Second) // the unit is written, and reads the project again
	b, err := dev.PadBlock(func() proto.PadIndex { p, _ := proto.NewPadIndex(64); return p }())
	if err != nil || !b.HasSample() {
		t.Errorf("the unit's E1 has no sample (%v)", err)
	}
	if p := r.Pads(); !p[64].Has {
		t.Errorf("after the reload the page's E1 is empty again")
	}
}
