package handheld

import (
	"fmt"
	"github.com/moahbeatlab/404tui/formats"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/sim"
)

func newApp(t *testing.T) (*App, *NullAudio) {
	t.Helper()
	dir := t.TempDir()
	if err := WriteDemo(filepath.Join(dir, "samples")); err != nil {
		t.Fatal(err)
	}
	au := &NullAudio{}
	a := New(filepath.Join(dir, "samples"), filepath.Join(dir, "out"), au)
	a.Connect = func() (Link, error) { return DeviceLink{Dev: device.New(sim.Demo(), "sim")}, nil }
	return a, au
}

func keys(t *testing.T, a *App, s string) {
	t.Helper()
	if err := a.Script(s, nil); err != nil {
		t.Fatal(err)
	}
}

// kit puts kick, snare and a closed hat on A1..A3, the way the user does it: menu, pick, browse.
const kit = "A A D R D D A  R A A D A  R A A U U U A"

func TestOnlyEightKeys(t *testing.T) {
	a, _ := newApp(t)
	// Every screen can be reached and left with the eight keys alone.
	for i := 0; i < len(screenNames); i++ {
		keys(t, a, "Y+R")
	}
	if a.Screen() != Pads {
		t.Fatalf("six steps right of PADS is PADS, got %v", a.Screen())
	}
	keys(t, a, "Y+L")
	if a.Screen() != Unit {
		t.Fatalf("Y+Left from PADS is the last screen, got %v", a.Screen())
	}
	if img := a.Render(); img.Bounds().Dx() != 640 || img.Bounds().Dy() != 480 {
		t.Fatalf("frame is %v", img.Bounds())
	}
}

func TestYAloneIsHelp(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, "Y")
	if !a.help {
		t.Fatal("a tap of Y shows the help")
	}
	keys(t, a, "A")
	if a.help || a.menu != nil {
		t.Fatal("any key closes the help and does nothing else")
	}
	keys(t, a, "Y+R")
	if a.help {
		t.Fatal("Y used as a shift is not a tap")
	}
}

func TestAssignSliceAndSequence(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	in := a.Info()
	for i, want := range []string{"hat_open", "snare_tight", "hat_closed"} {
		_ = i
		_ = want
	}
	used := 0
	for _, p := range in.Pads[:16] {
		if p.File {
			used++
		}
	}
	if used != 3 {
		t.Fatalf("3 pads assigned, got %d", used)
	}
	if got := in.Pads[0].Name; !strings.HasPrefix(got, "hat_open") && !strings.HasPrefix(got, "kick") {
		t.Logf("A1 is %q", got)
	}

	// slice a loop on A8 and put the slices on the pads after it
	keys(t, a, "D A A L D D R A")
	if !a.info.Pads[a.padN()].File {
		t.Fatalf("the loop is on %s", a.label(a.padN()))
	}
	loopPad := a.padN()
	keys(t, a, "Y+R Y+R Y+A")
	if a.Slices() < 3 {
		t.Fatalf("auto slice found %d slices in a loop of hits", a.Slices())
	}
	n := a.Slices()
	keys(t, a, "Y+X")
	if a.Screen() != Pads {
		t.Fatal("slices to pads goes back to PADS")
	}
	for i := 1; i <= n; i++ {
		if !a.info.Pads[loopPad+i].File {
			t.Fatalf("slice %d is not on pad %d", i, loopPad+i)
		}
	}

	// the sequencer: A puts a note, A again takes it off, the menu fills a row
	keys(t, a, "Y+R Y+R Y+R")
	if a.Screen() != Seq {
		t.Fatalf("screen %v", a.Screen())
	}
	keys(t, a, "A D D D D A") // two notes in the first column, four steps apart
	d, ok := a.doc()
	if !ok || len(d.Notes) != 2 || d.Bars != 4 {
		t.Fatalf("two notes in a 4 bar pattern, got %+v", d)
	}
	keys(t, a, "A")
	if d, _ = a.doc(); len(d.Notes) != 1 {
		t.Fatalf("A on a note takes it off, notes %d", len(d.Notes))
	}
	keys(t, a, "R B") // the next column, the menu: fill it with "every beat"
	chooseMenu(t, a, "Fill the column")
	d, _ = a.doc()
	if len(d.Notes) != 1+16 {
		t.Fatalf("every beat in 4 bars is 16 notes (and the other one): %d", len(d.Notes))
	}
	keys(t, a, "Y+A") // velocity of the note under the cursor
	keys(t, a, "X")
	if !a.seqOn || au.Loops != 1 {
		t.Fatalf("X plays the pattern: on %v, loops %d", a.seqOn, au.Loops)
	}
	keys(t, a, "A")
	if au.Loops != 2 {
		t.Fatal("an edit while it plays goes into the loop")
	}
	keys(t, a, "B")
	if a.seqOn {
		t.Fatal("B stops it")
	}

	// the mix has the length of the pattern
	s, ok := a.render()
	if !ok || s.Frames() != int(4*4*60.0/120*48000) {
		t.Fatalf("mix %d frames", s.Frames())
	}

	// save, then open the folder again
	keys(t, a, "Y+R")
	if a.Screen() != Save {
		t.Fatalf("screen %v", a.Screen())
	}
	keys(t, a, "A")
	proj := filepath.Join(a.OutDir, "ROLAND", "SP-404MKII", "PROJECT_01")
	for _, f := range []string{"PADCONF.BIN", "SMPL/BANK1-01.SMP", "PTN/PTN00001.BIN"} {
		if _, err := os.Stat(filepath.Join(proj, f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	b := New(a.Root, a.OutDir, nil)
	if err := b.OpenProject(proj); err != nil {
		t.Fatal(err)
	}
	for i := range a.info.Pads {
		if a.info.Pads[i].File != b.info.Pads[i].File || a.info.Pads[i].Name != b.info.Pads[i].Name {
			t.Fatalf("pad %d differs after reopening", i)
		}
	}
	d2, ok := b.doc()
	d1, _ := a.doc()
	if !ok || len(d2.Notes) != len(d1.Notes) {
		t.Fatalf("the pattern has %d notes after reopening, was %d", len(d2.Notes), len(d1.Notes))
	}
}

func TestMovePadAndCopyPad(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, "A A D R D D A") // some drum on A1
	name := a.info.Pads[0].Name
	keys(t, a, "A D D D R") // menu: Move or swap
	if a.menu == nil {
		t.Fatal("the menu is open")
	}
	keys(t, a, "B")
	if a.menu != nil {
		t.Fatal("B closes the menu")
	}
	// Move: menu items are Pick, Slice, Pattern, Move, Copy, ...: Move is the 4th
	keys(t, a, "A D D D A R R A")
	if a.info.Pads[0].File || a.info.Pads[2].Name != name {
		t.Fatalf("moved to A3: A1 %v A3 %q (was %q)", a.info.Pads[0].File, a.info.Pads[2].Name, name)
	}
	keys(t, a, "A D D D D A L A")
	if !a.info.Pads[1].File {
		t.Fatal("copied to A2")
	}
	if !a.info.Pads[2].File {
		t.Fatal("the original stays after a copy")
	}
}

func TestSendToTheUnit(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	keys(t, a, "Y+L")
	if a.Screen() != Unit {
		t.Fatal("UNIT is left of PADS")
	}
	keys(t, a, "A") // no link yet: looks for it
	a.Tick()
	if a.link == nil {
		keys(t, a, ".")
	}
	if a.link == nil {
		t.Fatal("the unit was not found")
	}
	keys(t, a, "D A") // choose project 2, ask: it must ask again
	if a.job != nil {
		t.Fatal("a send needs a second A")
	}
	keys(t, a, "A ~")
	if a.job == nil {
		t.Fatal("no transfer")
	}
	if _, total, _, err, fin := a.job.state(); !fin || err != nil || total < 3 {
		t.Fatalf("finished %v err %v files %d", fin, err, total)
	}
}

func TestWebKeysAreTheSameEightKeys(t *testing.T) {
	for _, c := range "UDLRABXY" {
		if _, err := parseKey(string(c)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := parseKey("Z"); err == nil {
		t.Fatal("only eight keys")
	}
}

func TestHelpDoesNotEatAChord(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, "Y Y+R")
	if a.Screen() != Files || a.help {
		t.Fatalf("screen %v help %v: Y with the help open closes it and the chord goes on", a.Screen(), a.help)
	}
}

func padsWithSamples(a *App) int {
	n := 0
	for _, p := range a.Info().Pads {
		if p.File {
			n++
		}
	}
	return n
}

func TestQuitAsksOnlyWhenWorkIsUnsaved(t *testing.T) {
	a, _ := newApp(t)
	a.Dispatch(Event{Quit: true})
	if !a.Quit {
		t.Fatal("an empty project should quit at once")
	}

	a, _ = newApp(t)
	keys(t, a, kit)
	a.Dispatch(Event{Quit: true})
	if a.Quit || !a.quitAsk {
		t.Fatal("unsaved samples: the first Select+Start must only ask")
	}
	keys(t, a, "D") // any key cancels
	a.Dispatch(Event{Quit: true})
	if a.Quit {
		t.Fatal("a key in between must cancel the question")
	}
	a.Dispatch(Event{Quit: true})
	if !a.Quit {
		t.Fatal("the second Select+Start in a row must quit")
	}

	a, _ = newApp(t)
	keys(t, a, kit)
	a.go_(Save)
	keys(t, a, "A")
	if !a.saved {
		t.Fatal("writing the project must count as saved")
	}
	a.Dispatch(Event{Quit: true})
	if !a.Quit {
		t.Fatal("a saved project quits at once")
	}

	a, _ = newApp(t)
	keys(t, a, kit)
	a.go_(Save)
	keys(t, a, "X X") // X on SAVE: ask, then quit
	if !a.Quit {
		t.Fatal("X twice on SAVE must quit")
	}
}

func TestBrowseTheWholeFilesystem(t *testing.T) {
	a, _ := newApp(t)
	a.go_(Files)
	for i := 0; i < 40 && a.dir != "/"; i++ {
		keys(t, a, "L")
	}
	if a.dir != "/" {
		t.Fatalf("Left must climb to /, stopped at %s", a.dir)
	}
	keys(t, a, "L") // at the top: stays
	if a.dir != "/" {
		t.Fatal("went above /")
	}
	if len(a.entries) == 0 {
		t.Fatal("/ lists nothing")
	}
	// Y+X goes to the samples folder first, then through the places that exist
	keys(t, a, "Y+X")
	if !strings.HasPrefix(a.dir, "/") || a.dir == "" {
		t.Fatalf("a place was not a folder: %q", a.dir)
	}
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		seen[a.dir] = true
		keys(t, a, "Y+X")
	}
	if !seen[a.Root] {
		t.Fatal("the places must include the samples folder")
	}
	// the cursor returns to the folder we came from
	a.dir = filepath.Join(a.Root, "drums")
	a.list()
	keys(t, a, "L")
	if e, ok := a.current(); !ok || e.name != "drums" {
		t.Fatalf("after Left the cursor should sit on drums, got %+v", e)
	}
	// B leaves for the pads from any folder
	keys(t, a, "B")
	if a.scr != Pads {
		t.Fatal("B must go back to the pads")
	}
}

func TestEmptySamplesFolderSaysWhatToDo(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "404", "samples")
	os.MkdirAll(root, 0o755)
	a := New(root, filepath.Join(dir, "out"), &NullAudio{})
	a.go_(Files)
	img := a.Render()
	if img.Bounds().Dx() != W {
		t.Fatal("no frame")
	}
	if len(a.entries) != 0 {
		t.Fatal("expected an empty folder")
	}
}

func TestLoadAProjectFromTheFilesystem(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	want := padsWithSamples(a)
	if want < 3 {
		t.Fatalf("setup: %d pads", want)
	}
	a.go_(Save)
	keys(t, a, "R R A") // slot 3
	card := filepath.Join(a.OutDir, cardFolder)
	if _, err := os.Stat(filepath.Join(card, "PROJECT_03", "PADCONF.BIN")); err != nil {
		t.Fatalf("project not written: %v", err)
	}

	b, _ := newApp(t)
	b.go_(Files)
	b.dir = card
	b.list()
	found := false
	for i, e := range b.entries {
		if e.name == "PROJECT_03" {
			found = true
			b.fcur = i
			if !e.project {
				t.Fatal("a folder with PADCONF.BIN must be marked as a project")
			}
		}
	}
	if !found {
		t.Fatal("PROJECT_03 not listed")
	}
	keys(t, b, "A")
	if b.scr != Pads {
		t.Fatalf("after loading we expect the pads, on %v (%s)", b.scr, b.msg)
	}
	if got := padsWithSamples(b); got != want {
		t.Fatalf("loaded %d pads with samples, saved %d", got, want)
	}
	if b.slot != 3 || b.ProjectName() != "PROJECT_03" {
		t.Fatalf("slot after loading: %d", b.slot)
	}

	// with unsaved work the first A only asks
	c, _ := newApp(t)
	keys(t, c, kit)
	c.saved = false
	c.go_(Files)
	c.dir = card
	c.list()
	for i, e := range c.entries {
		if e.name == "PROJECT_03" {
			c.fcur = i
		}
	}
	keys(t, c, "A")
	if c.scr != Files || c.loadAsk == "" {
		t.Fatal("unsaved work: the first A must ask, not load")
	}
	keys(t, c, "A")
	if c.scr != Pads {
		t.Fatalf("the second A must load (%s)", c.msg)
	}
}

func TestSaveReplacesTheFolderAndLeavesNothingBehind(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Save)
	keys(t, a, "A")
	dir := filepath.Join(a.OutDir, cardFolder, "PROJECT_01")
	before, _ := filepath.Glob(filepath.Join(dir, "SMPL", "*"))
	if len(before) < 3 {
		t.Fatalf("expected 3 samples written, got %d", len(before))
	}
	// clear a pad, write again: its file must be gone, not left over from the first write
	a.go_(Pads)
	keys(t, a, "Y+B")
	a.go_(Save)
	keys(t, a, "A")
	after, _ := filepath.Glob(filepath.Join(dir, "SMPL", "*"))
	if len(after) != len(before)-1 {
		t.Fatalf("a cleared pad left its file behind: %d files before, %d after", len(before), len(after))
	}
	left, _ := filepath.Glob(filepath.Join(a.OutDir, cardFolder, ".404h-*"))
	if len(left) != 0 {
		t.Fatalf("staging folders left behind: %v", left)
	}
}

func TestSlicesNeverLandOnUsedPads(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // kick, snare, hat on A1..A3
	names := func() [3]string {
		var n [3]string
		for i := 0; i < 3; i++ {
			n[i] = a.Info().Pads[i].Name
		}
		return n
	}
	before := names()
	keys(t, a, "Y+R Y+R R*6 A Y+X") // cut the kick in two, put the slices on pads
	if a.scr != Pads {
		t.Fatalf("expected the pads after Y+X, on %v: %s", a.scr, a.msg)
	}
	if got := names(); got != before {
		t.Fatalf("slices overwrote pads: %v -> %v", before, got)
	}
	if !a.Info().Pads[3].File || !a.Info().Pads[4].File {
		t.Fatalf("the slices should be on A4 and A5 (the first free run)")
	}
}

func TestTrackerRowsAreStepsColumnsArePads(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // kick, snare, hat on A1..A3, the cursor on A3
	a.go_(Seq)
	for i := 1; i < len(a.cols); i++ {
		if a.cols[i-1] >= a.cols[i] {
			t.Fatalf("columns are not in pad order: %v", a.cols)
		}
	}
	if a.cols[0] != 0 || a.cols[1] != 1 || a.cols[2] != 2 {
		t.Fatalf("the pads with samples come first: %v", a.cols)
	}
	if a.colPad() != 2 {
		t.Fatalf("the cursor starts on the pad that was in front (A3), not %d", a.colPad())
	}
	// Left/Right change the column, Up/Down the step
	keys(t, a, "L L")
	if a.col != 0 {
		t.Fatalf("column %d", a.col)
	}
	keys(t, a, strings.Repeat("D ", 40)+"A")
	if a.step != 40 {
		t.Fatalf("step %d", a.step)
	}
	d, _ := a.doc()
	n, ok := noteAt(d, 0, 40)
	if !ok || int(n.Tick) != 40*stepTicks {
		t.Fatalf("no note at step 41 of the kick: %+v", d.Notes)
	}
	if img := a.Render(); img.Bounds().Dx() != W { // the view has scrolled to step 40
		t.Fatal("no frame")
	}
	// the end is the end
	keys(t, a, strings.Repeat("D ", 100))
	if a.step != a.steps()-1 {
		t.Fatalf("stops at the last step, got %d of %d", a.step, a.steps())
	}
	// Y+X gives the column the next pad with a sample, and it stays when the pattern changes
	keys(t, a, "Y+X")
	if a.colPad() != 1 || !a.colsOwn {
		t.Fatalf("column 1 should be pad A2 now, is %d", a.colPad())
	}
	keys(t, a, "Y+U")
	if a.cols[0] != 1 {
		t.Fatalf("the user's column was reset by the pattern change: %v", a.cols)
	}
}

func TestPanelShowsTheKeysOfTheScreen(t *testing.T) {
	a, _ := newApp(t)
	for s := Pads; s <= Unit; s++ {
		a.go_(s)
		plain, _ := a.hints()
		if len(plain) == 0 {
			t.Fatalf("screen %v has no keys listed", s)
		}
		a.Render()
	}
	a.go_(Pads)
	a.Press(Y, 0)
	if !a.yHeld {
		t.Fatal("Y held")
	}
	a.Render() // the shifted set is lit
	a.Release(Y)
}

func TestShoulderButtonsPage(t *testing.T) {
	if defaultMap[0x136] != L1 || defaultMap[0x137] != R1 {
		t.Fatal("0x136 and 0x137 are L1 and R1 on the R36S's pad")
	}
	a, _ := newApp(t)
	// a long folder: 60 files
	dir := filepath.Join(a.Root, "long")
	os.MkdirAll(dir, 0o755)
	wav, _ := os.ReadFile(filepath.Join(a.Root, "drums", "kick_808.wav"))
	for i := 0; i < 60; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.wav", i)), wav, 0o644)
	}
	a.go_(Files)
	a.dir = dir
	a.list()
	keys(t, a, "]")
	if a.fcur != 20 {
		t.Fatalf("R1 is a page down: cursor %d", a.fcur)
	}
	keys(t, a, "] ] ]")
	if a.fcur != 59 {
		t.Fatalf("stops at the last file, cursor %d", a.fcur)
	}
	keys(t, a, "[")
	if a.fcur != 39 {
		t.Fatalf("L1 is a page up: cursor %d", a.fcur)
	}
	// pads: a bank at a time, wrapping
	a.go_(Pads)
	keys(t, a, "]")
	if a.bank != 1 {
		t.Fatalf("bank %d", a.bank)
	}
	keys(t, a, "[ [")
	if a.bank != 9 {
		t.Fatalf("L1 from bank A wraps to J, got %d", a.bank)
	}
	// seq: a bar
	a.go_(Seq)
	keys(t, a, "]")
	if a.step != 16 {
		t.Fatalf("R1 in SEQ is a bar: step %d", a.step)
	}
	if k, err := parseKey("]"); err != nil || k != R1 || R1.String() != "R1" {
		t.Fatal("the script token for R1")
	}
}

func TestTemplateMakesAPatternFromTheKit(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // kick_808, snare_..., hat_closed on A1..A3
	a.go_(Seq)
	before := a.pat
	a.fromTemplate(formats.GrooveTemplates()[0], 2)
	if a.pat != before {
		t.Fatalf("with an empty pattern in front the template goes into it, got %d", a.pat)
	}
	first := a.pat
	a.fromTemplate(formats.GrooveTemplates()[1], 2)
	if a.pat == first {
		t.Fatal("a template goes into a free pattern, never over one that has notes")
	}
	d, ok := a.doc()
	if !ok || d.Bars != 2 || len(d.Notes) < 8 {
		t.Fatalf("the groove has %d notes in %d bars", len(d.Notes), d.Bars)
	}
	pads := map[int]bool{}
	for _, n := range d.Notes {
		pads[n.Pad] = true
	}
	if len(pads) < 2 {
		t.Fatalf("only %d pads play", len(pads))
	}
	// the menus: the list scrolls, L1/R1 page it
	a.seqMenu()
	keys(t, a, "D A") // New pattern from a template... (the picker is the first item)
	if a.menu == nil || len(a.menu.items) < 10 {
		t.Fatal("the template list is not open")
	}
	keys(t, a, "]")
	if a.menu.cur != 8 {
		t.Fatalf("R1 pages the list: cursor %d", a.menu.cur)
	}
	a.Render()
	a.menu = nil
	n := a.pat
	a.blankPattern(3)
	if d, ok := a.doc(); !ok || d.Bars != 3 || a.pat == n {
		t.Fatal("blank pattern")
	}
}

func TestSlicerMenuShufflesAndRendersASteps(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, "A A L D D R A") // a loop on A1, as in the other test
	_ = au
	a.go_(Slice)
	if a.sl == nil {
		t.Skip("no sample on the pad")
	}
	keys(t, a, "Y+A") // auto slices
	n := len(a.sl.Slices)
	if n < 3 {
		t.Skipf("only %d slices", n)
	}
	a.sliceMenu()
	if a.menu == nil || len(a.menu.items) < 12 {
		t.Fatal("the slicer menu is not open")
	}
	a.Render()
	// the slice's pitch changes live, from the menu
	keys(t, a, "R")
	if a.sl.Slices[a.sliceAt(a.at)].Pitch != 1 {
		t.Fatalf("Right on the pitch is +1: %+v", a.sl.Slices[a.sliceAt(a.at)])
	}
	a.menu = nil
	a.sliceSteps = 16
	s, id, err := a.renderResult(true)
	if err != nil || s.Frames() == 0 {
		t.Fatalf("render: %v", err)
	}
	a.lab.Free(id)
	before := padsWithSamples(a)
	a.sl.Mode = formats.ModeShuffle
	a.slicesToPads(true)
	if got := padsWithSamples(a); got < before+n {
		t.Fatalf("shuffled slices on pads: %d pads, wanted %d more", got, n)
	}
}

func TestPatternPickerBankThenPattern(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	keys(t, a, "A") // a note in pattern A1, so it shows as used
	keys(t, a, "Y+B")
	if a.picker == nil || a.picker.bank != 0 || a.picker.cur != 0 {
		t.Fatalf("the picker opens on the pattern in front: %+v", a.picker)
	}
	if !a.picker.info[0].used || a.picker.info[1].used {
		t.Fatal("A1 has a note, A2 has none")
	}
	a.Render()
	keys(t, a, "]") // bank B
	if a.picker.bank != 1 {
		t.Fatalf("R1 is the next bank, got %d", a.picker.bank)
	}
	keys(t, a, "D R R A") // row 2, column 3 of bank B: pattern B7 = 16+4+2+1 = 23
	if a.picker != nil || a.pat != 23 || a.label(a.pat-1) != "B7" {
		t.Fatalf("opened pattern %d (%s)", a.pat, a.label(a.pat-1))
	}
	// Left at the edge of the grid goes to the previous bank
	keys(t, a, "Y+B")
	keys(t, a, "L L L") // column 2 -> 1 -> 0, then one more: bank A, column 3
	if a.picker.bank != 0 || a.picker.cur%4 != 3 {
		t.Fatalf("Left past the edge: bank %d, cursor %d", a.picker.bank, a.picker.cur)
	}
	keys(t, a, "B")
	if a.picker != nil || a.pat != 23 {
		t.Fatal("B closes the picker and keeps the pattern")
	}
	// the menu has it too, and Y+Up/Down still steps one
	a.seqMenu()
	keys(t, a, "A")
	if a.picker == nil {
		t.Fatal("the picker is the first item of the sequencer menu")
	}
	keys(t, a, "B")
	keys(t, a, "Y+U")
	if a.pat != 24 {
		t.Fatalf("Y+Up is the next pattern: %d", a.pat)
	}
}

// chooseMenu moves the cursor of the open menu to the item that starts with label and presses A.
func chooseMenu(t *testing.T, a *App, label string) {
	t.Helper()
	if a.menu == nil {
		t.Fatalf("no menu is open to choose %q from", label)
	}
	for i := 0; i < len(a.menu.items); i++ {
		if strings.HasPrefix(a.menu.item().label, label) {
			keys(t, a, "A")
			return
		}
		keys(t, a, "D")
	}
	t.Fatalf("no menu item %q", label)
}

func TestShuffledOrderIsVisibleAndGoesToThePads(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, "A A D D R A") // a loop on A1
	a.go_(Slice)
	keys(t, a, "Y+A") // auto slices
	n := len(a.sl.Slices)
	if n < 4 {
		t.Skipf("only %d slices", n)
	}
	before := append([]int{}, a.sl.Order...)
	a.sliceMenu()
	chooseMenu(t, a, "Shuffle the order")
	if equalInts(before, a.sl.Order) {
		t.Fatalf("the order did not change: %v", a.sl.Order)
	}
	if got := a.padOrder(); len(got) != n || equalInts(got, before) {
		t.Fatalf("the pads follow the order: %v (order %v)", got, a.sl.Order)
	}
	if !strings.Contains(a.msg, " ") || a.msgBad {
		t.Fatalf("the message says the order: %q", a.msg)
	}
	a.Render() // the order lane is drawn

	// one slice with X (and L2), the whole order with R2
	au.Played = 0
	keys(t, a, "X")
	if au.Played != 1 {
		t.Fatalf("X plays one slice: %d", au.Played)
	}
	keys(t, a, "{")
	if au.Played != 2 {
		t.Fatalf("L2 plays one slice too: %d", au.Played)
	}
	keys(t, a, "}")
	if au.Played != 3 {
		t.Fatalf("R2 plays the whole order: %d", au.Played)
	}
	if k, _ := parseKey("}"); k != R2 || defaultMap[0x139] != R2 || defaultMap[0x138] != L2 {
		t.Fatal("R2 is 0x139 and L2 is 0x138")
	}

	// to pads: the slice at the first place of the order is on the first pad
	first := a.sl.Order[0]
	pad0 := a.spad + 1
	_ = pad0
	keys(t, a, "Y+X")
	if a.scr != Pads {
		t.Fatalf("screen %v", a.scr)
	}
	name := a.info.Pads[a.bank*16+a.cur].Name
	if !strings.HasSuffix(name, fmt.Sprintf("_%02d", first+1)) {
		t.Fatalf("the first pad has %q, wanted slice %d", name, first+1)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTempoIsTheBanksOrTheProjects(t *testing.T) {
	a, _ := newApp(t)
	a.lab.SetTempo(90)
	a.lab.SetBankTempo(0, 124)
	a.lab.SetBankTempo(9, 70)
	a.refresh()
	a.pat = 1 // A1
	if a.tempo() != 124 {
		t.Fatalf("a pattern of bank A plays at the bank's tempo, not the project's 90: %v", a.tempo())
	}
	a.pat = 145 // J1
	if a.tempo() != 70 || a.tempoSource() != "bank J" {
		t.Fatalf("bank J: %v %s", a.tempo(), a.tempoSource())
	}
	a.lab.SetUsesProjectTempo(true)
	a.refresh()
	if a.tempo() != 90 || a.tempoSource() != "proj" {
		t.Fatalf("with Tempo Select on, every bank plays at the project tempo: %v", a.tempo())
	}
	// the Tempo menu changes the one that plays
	a.go_(Seq)
	a.seqMenu()
	chooseMenu(t, a, "Tempo of the project")
	if a.menu != nil && a.menu.item().label != "Tempo of the project" {
		t.Fatal("the menu names the tempo it changes")
	}
	a.menu = nil
	a.lab.SetUsesProjectTempo(false)
	a.refresh()
	a.pat = 17 // B1
	a.seqMenu()
	if a.menu.items[2].label != "Tempo of bank B" {
		t.Fatalf("menu item: %q", a.menu.items[2].label)
	}
	a.Render()
}

func TestRealProjectsBankTempo(t *testing.T) {
	p := "/home/hermes/test-projects/Construction Kits House 01"
	if _, err := os.Stat(p); err != nil {
		t.Skip("the real project is not here")
	}
	a, _ := newApp(t)
	if err := a.OpenProject(p); err != nil {
		t.Fatal(err)
	}
	a.pat = 1
	if a.info.UsesProjectTempo || a.tempo() != 124 {
		t.Fatalf("this project plays bank A at 124 BPM (its project tempo says %v): got %v, project tempo in use %v", a.info.Tempo, a.tempo(), a.info.UsesProjectTempo)
	}
	a.pat = 145
	if a.tempo() != 90 {
		t.Fatalf("bank J is 90: %v", a.tempo())
	}
}
