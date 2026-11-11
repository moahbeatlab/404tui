package core

import (
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/generate"
)

// the generator works on the editor's project, so it needs no unit: offline it fills the project, live the patterns go on to the unit
func TestGenerateRunsOfflineAndUndoes(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // kick, snare, hat on A1..A3
	a.go_(Seq)
	st, err := generate.StyleByName("techno")
	if err != nil {
		t.Fatal(err)
	}
	a.generateArc(st, "arc", 2)
	n := 0
	for slot := 17; slot <= 24; slot++ { // bank B, after the kit's bank A
		if d, ok := a.patternDoc(slot); ok && len(d.Notes) > 0 {
			n++
		}
	}
	if n != 8 {
		t.Fatalf("%d of 8 patterns have notes in B1..B8", n)
	}
	a.undoStep()
	if d, ok := a.patternDoc(18); ok && len(d.Notes) > 0 {
		t.Fatal("undo leaves the generated patterns")
	}
}

func TestGenerateFamilyFillsABank(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	st, _ := generate.StyleByName("house")
	a.generateArc(st, "family", 2)
	for slot := 17; slot <= 32; slot++ { // B1..B16
		if d, ok := a.patternDoc(slot); !ok || len(d.Notes) == 0 {
			t.Fatalf("slot %d is empty (%s)", slot, a.Message())
		}
	}
}

func TestGeneratorKitFollowsWhatWasChosen(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	_, pads, _ := a.generatorKit()
	if _, ok := pads[formats.LaneKick]; !ok {
		t.Fatal("the names give a kick")
	}
	a.genKitMenu()
	if a.menu == nil || !strings.Contains(screenStr(a), "WHICH PAD PLAYS WHAT") {
		t.Fatalf("the kit menu is open:\n%s", screenStr(a))
	}
	a.set.GenKit.Pad[formats.LaneKick] = 5
	a.set.GenKit.Pad[formats.LaneSnare] = -1
	_, pads, _ = a.generatorKit()
	if pads[formats.LaneKick].Index() != 5 {
		t.Fatalf("the kick is the pad chosen: %d", pads[formats.LaneKick].Index())
	}
	if _, ok := pads[formats.LaneSnare]; ok {
		t.Fatal("none takes the voice out")
	}
}

// Enter on a voice opens the pad picker, and the pad that is picked is kept in the settings for the next start
func TestGeneratorKitPicksAPadAndRemembersIt(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.genKitMenu()
	a.menu.items[a.menu.cur].run(0) // Kick
	if a.picker == nil {
		t.Fatal("Enter opens the pad picker")
	}
	a.picker.onPick(7)
	if a.menu == nil || a.set.GenKit.Pad[formats.LaneKick] != 7 {
		t.Fatalf("the kick is pad 7 and the menu is back: %v", a.set.GenKit.Pad)
	}
	b := New(a.Root, a.OutDir, &NullAudio{})
	if b.set.GenKit.Pad[formats.LaneKick] != 7 {
		t.Fatalf("the choice is there at the next start: %v", b.set.GenKit)
	}
}

// X (Space on a keyboard) in the pad picker plays the sample of the pad in front
func TestPadPickerHearsThePad(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.genKitMenu()
	a.menu.items[a.menu.cur].run(0)
	if a.picker == nil {
		t.Fatal("the picker is open")
	}
	before := au.Played
	a.pickerKey(X)
	if au.Played == before || a.picker == nil {
		t.Fatalf("X plays the pad and keeps the picker open: %d plays", au.Played)
	}
}
