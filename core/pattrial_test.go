package core

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/generate"
)

func menuItemNamed(t *testing.T, a *App, label string) *menuItem {
	t.Helper()
	for i := range a.menu.items {
		if a.menu.items[i].label == label {
			return &a.menu.items[i]
		}
	}
	t.Fatalf("no item %q in %q", label, a.menu.title)
	return nil
}

// the notes of one pad of a pattern, as ticks
func padTicks(a *App, n, pad int) []uint32 {
	d, _ := a.patternDoc(n)
	var out []uint32
	for _, note := range d.Notes {
		if note.Pad == pad {
			out = append(out, note.Tick)
		}
	}
	return out
}

func sameTicks(x, y []uint32) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// one pattern of the generator is a trial: rolled again in the same slot, the locked voices stay, Keep makes an undo step and the next roll
// goes into the next slot, B throws away what was not kept and the loop that ran plays on
func TestGenerateOneRollsLocksKeepsAndThrowsAway(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.genRand = rand.New(rand.NewSource(1))
	a.openPattern(20) // B4, empty
	st, _ := generate.StyleByName("techno")
	a.generateOne(st, "single", 2)
	if a.menu == nil || a.pat != 20 {
		t.Fatalf("the trial is in B4 with its dialog open: pattern %d, %s", a.pat, a.Message())
	}
	if !a.seqOn || a.playPat != 20 {
		t.Fatal("the trial plays")
	}
	_, pads, _ := a.generatorKit()
	kick := int(pads[formats.LaneKick].Index())
	hat := int(pads[formats.LaneClosedHat].Index())
	menuItemNamed(t, a, "Kick").onChange(1) // stays
	before, hats := padTicks(a, 20, kick), padTicks(a, 20, hat)
	other := false
	for i := 0; i < 6 && !other; i++ {
		menuItemNamed(t, a, "Another roll of the dice").run(0)
		if !sameTicks(padTicks(a, 20, kick), before) {
			t.Fatal("the kick is locked: it stays at the next roll")
		}
		other = !sameTicks(padTicks(a, 20, hat), hats)
	}
	if !other {
		t.Fatal("the hats are not locked: they change")
	}
	if _, ok := a.patternDoc(21); ok {
		t.Fatal("rolling again uses the same slot")
	}
	undo := len(a.undo)
	menuItemNamed(t, a, "Keep it (B throws it away)").run(0)
	if len(a.undo) != undo+1 {
		t.Fatal("keep makes one undo step")
	}
	menuItemNamed(t, a, "Another roll of the dice").run(0)
	if a.pat != 21 {
		t.Fatalf("after keep the next roll goes to B5: %d", a.pat)
	}
	kept, _ := a.lab.PatternBytes(20)
	a.menuKey(B)
	if a.menu != nil && a.menu.title == "generate: "+st.Name {
		t.Fatal("B closes the dialog")
	}
	if _, ok := a.patternDoc(21); ok {
		t.Fatal("the roll that was not kept is thrown away")
	}
	if now, _ := a.lab.PatternBytes(20); !bytes.Equal(now, kept) || a.pat != 20 {
		t.Fatal("the kept pattern stays, and is in front")
	}
	if a.toolBase != nil {
		t.Fatal("the trial is over")
	}
}

// with a fill, only the fill can be rolled again: the groove stays
func TestGenerateOneOnlyAnotherFill(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.genRand = rand.New(rand.NewSource(2))
	a.openPattern(20)
	st, _ := generate.StyleByName("house")
	a.generateOne(st, "fill", 2)
	_, pads, _ := a.generatorKit()
	kick := int(pads[formats.LaneKick].Index())
	firstBar := func() []uint32 {
		var out []uint32
		for _, tk := range padTicks(a, 20, kick) {
			if tk < 1920 {
				out = append(out, tk)
			}
		}
		return out
	}
	before := firstBar()
	for i := 0; i < 3; i++ {
		menuItemNamed(t, a, "Only another fill, the groove stays").run(0)
		if !sameTicks(firstBar(), before) {
			t.Fatal("the groove before the fill stays")
		}
	}
	a.menuKey(B)
	if _, ok := a.patternDoc(20); ok {
		t.Fatal("B throws the trial away")
	}
}

// a variation is a trial in the next free slot; the pattern it is made of stays as it was, every kind changes something
func TestVariationLeavesTheSource(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.genRand = rand.New(rand.NewSource(3))
	st, _ := generate.StyleByName("techno")
	a.openPattern(20)
	a.generateArc(st, "single", 2) // B4
	a.openPattern(20)
	src, _ := a.lab.PatternBytes(20)
	if !a.seqOn {
		a.startLoop() // the user plays B4: the trial takes the loop over and gives it back
	}
	a.variationMenu()
	if a.menu == nil || a.menu.title != "variation of B4" || a.pat != 21 {
		t.Fatalf("the variation is in B5: pattern %d, %s", a.pat, a.Message())
	}
	what := menuItemNamed(t, a, "What")
	made := 0
	for k := range generate.Variations {
		what.val = k
		what.onChange(k)
		if a.pat != 21 { // a groove that is only the kick and the backbeat has nothing to take away: it says so
			t.Logf("%s: %s", generate.Variations[k], a.Message())
			if a.pat != 20 || a.toolBase == nil {
				t.Fatal("a kind that changes nothing leaves the source in front and the dialog open")
			}
			continue
		}
		made++
		v, _ := a.lab.PatternBytes(21)
		if bytes.Equal(v, src) {
			t.Fatalf("%s changes nothing", generate.Variations[k])
		}
		if now, _ := a.lab.PatternBytes(20); !bytes.Equal(now, src) {
			t.Fatal("the source stays as it was")
		}
		// the notes that are not on a changed step are the source's own
		sd, _ := a.patternDoc(20)
		vd, _ := a.patternDoc(21)
		same := 0
		for _, n := range vd.Notes {
			for _, m := range sd.Notes {
				if n == m {
					same++
					break
				}
			}
		}
		if same == 0 {
			t.Fatalf("%s rewrites every note", generate.Variations[k])
		}
	}
	if made < len(generate.Variations)-1 {
		t.Fatalf("only %d kinds made a variation", made)
	}
	what.onChange(0)
	a.menuKey(B)
	if _, ok := a.patternDoc(21); ok || a.pat != 20 {
		t.Fatal("B throws the variation away and goes back to B4")
	}
	if !a.seqOn || a.playPat != 20 {
		t.Fatal("the loop that ran before plays B4 again")
	}
	a.openPattern(30) // empty
	a.variationMenu()
	if a.menu != nil {
		t.Fatal("an empty pattern has no variation")
	}
}

func TestVariationIsInTheMenus(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.seqMenu()
	menuItemNamed(t, a, variationLabel)
	a.menu = nil
	a.templatesMenu()
	a.menu.setTab(1)
	menuItemNamed(t, a, variationLabel)
}

// undo and redo while rolling: the roll before comes back in the same slot, with its groove, and redo goes forward again
func TestTrialUndoAndRedoGoThroughTheRolls(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.genRand = rand.New(rand.NewSource(4))
	a.openPattern(20)
	st, _ := generate.StyleByName("techno")
	a.generateOne(st, "fill", 2)
	first, _ := a.lab.PatternBytes(20)
	menuItemNamed(t, a, "Another roll of the dice").run(0)
	second, _ := a.lab.PatternBytes(20)
	if bytes.Equal(first, second) {
		t.Fatal("two rolls differ")
	}
	undo := len(a.undo)
	keys(t, a, "Y+{")
	if now, _ := a.lab.PatternBytes(20); !bytes.Equal(now, first) || a.menu == nil {
		t.Fatalf("undo brings the first roll back and the dialog stays: %s", a.Message())
	}
	if len(a.undo) != undo {
		t.Fatal("undoing a roll is not a step of the project")
	}
	keys(t, a, "Y+{")
	if !strings.Contains(a.Message(), "nothing to undo") {
		t.Fatalf("before the first roll there is nothing: %s", a.Message())
	}
	keys(t, a, "Y+}")
	if now, _ := a.lab.PatternBytes(20); !bytes.Equal(now, second) {
		t.Fatal("redo brings the second roll back")
	}
	keys(t, a, "Y+{")
	// the groove of the first roll is back: only another fill keeps its first bar
	menuItemNamed(t, a, "Only another fill, the groove stays").run(0)
	_, pads, _ := a.generatorKit()
	kick := int(pads[formats.LaneKick].Index())
	a.lab.SetPatternBytes(21, first)
	firstBar := func(n int) (out []uint32) {
		for _, tk := range padTicks(a, n, kick) {
			if tk < 1920 {
				out = append(out, tk)
			}
		}
		return
	}
	if !sameTicks(firstBar(20), firstBar(21)) {
		t.Fatal("after undo, another fill goes on the groove of the roll that came back")
	}
	a.lab.SetPatternBytes(21, nil)
	a.menuKey(B)
	if a.trialUndo != nil {
		t.Fatal("the dialog is closed: undo is the project's again")
	}
}
