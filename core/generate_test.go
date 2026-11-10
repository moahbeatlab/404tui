package core

import (
	"testing"

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
	a.generateArc(st, 2)
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
