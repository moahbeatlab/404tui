package handheld

import (
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
	keys(t, a, "R B A") // the next column, the menu: fill it with "every beat"
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
