package core

import (
	"fmt"
	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/record"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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
	keys(t, a, "Y+L")
	if a.Screen() != Loop {
		t.Fatalf("Y+Left from PADS is the last screen, got %v", a.Screen())
	}
	keys(t, a, "Y+R")
	if a.Screen() != Pads {
		t.Fatalf("Y+Right from the last screen is PADS, got %v", a.Screen())
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
	keys(t, a, "Y+R Y+R Y+R Y+A") // PADS, SEQ, FILES, SLICE
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
	keys(t, a, "Y+R")
	if a.Screen() != Seq {
		t.Fatalf("screen %v", a.Screen())
	}
	keys(t, a, "A R R R R A") // two notes in the first row, four steps apart
	d, ok := a.doc()
	if !ok || len(d.Notes) != 2 || d.Bars != 4 {
		t.Fatalf("two notes in a 4 bar pattern, got %+v", d)
	}
	keys(t, a, "A")
	if d, _ = a.doc(); len(d.Notes) != 1 {
		t.Fatalf("A on a note takes it off, notes %d", len(d.Notes))
	}
	keys(t, a, "Y+X A S+B") // a second track (the next pad with a sample, chosen in the pad dialog), the tools: fill it with "every beat"
	chooseMenu(t, a, "Fill...")
	d, _ = a.doc() // the trial is already in the pattern: every beat
	if len(d.Notes) != 1+16 {
		t.Fatalf("every beat in 4 bars is 16 notes (and the other one): %d", len(d.Notes))
	}
	chooseMenuContaining(t, a, "Keep it")
	keys(t, a, "B") // the tool's menu stays open until it is closed
	if a.seqOn {
		t.Fatal("closing the tool stops the loop the trial started")
	}
	base := au.Loops
	keys(t, a, "X")
	if !a.seqOn || au.Loops != base+1 {
		t.Fatalf("X plays the pattern: on %v, loops %d (was %d)", a.seqOn, au.Loops, base)
	}
	keys(t, a, "A")
	if au.Loops != base+2 {
		t.Fatal("an edit while it plays goes into the loop")
	}
	keys(t, a, "B")
	if !a.seqOn || a.np == nil {
		t.Fatal("B opens the note settings and the pattern goes on playing")
	}
	keys(t, a, "B S+B")
	if !a.seqOn || a.menu == nil {
		t.Fatal("Select+B opens the pattern tools and the pattern goes on playing")
	}
	keys(t, a, "B")
	keys(t, a, "X")
	if a.seqOn {
		t.Fatal("X stops it")
	}

	// the mix has the length of the pattern
	s, ok := a.render()
	if !ok || s.Frames() != int(4*4*60.0/120*48000) {
		t.Fatalf("mix %d frames", s.Frames())
	}

	// save, then open the folder again
	keys(t, a, "Y+R Y+R Y+R") // SEQ, FILES, SLICE, SAVE
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
	keys(t, a, "A")
	if a.pp == nil || a.pp.page != 0 {
		t.Fatal("A on a pad with a sample opens its settings first")
	}
	keys(t, a, "B")
	if a.pp != nil {
		t.Fatal("B closes the pad panel")
	}
	keys(t, a, "A [") // the settings, and L1 round to the tools
	chooseMenu(t, a, "Move or swap")
	keys(t, a, "R R A")
	if a.info.Pads[0].File || a.info.Pads[2].Name != name {
		t.Fatalf("moved to A3: A1 %v A3 %q (was %q)", a.info.Pads[0].File, a.info.Pads[2].Name, name)
	}
	keys(t, a, "A [")
	chooseMenu(t, a, "Copy to")
	keys(t, a, "L A")
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
	onScreen(t, a, Unit)
	keys(t, a, "A") // no link yet: looks for it
	a.Tick()
	if a.link == nil {
		keys(t, a, ".")
	}
	if a.link == nil {
		t.Fatal("the unit was not found")
	}
	keys(t, a, "D A") // choose project 2: A opens the menu of what can be done with it
	if a.menu == nil || a.job != nil {
		t.Fatal("A opens the menu of the unit's project")
	}
	chooseMenuNoPick(t, a, "Send: replace this project")
	if a.job != nil || !a.confirm {
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
		if _, err := ParseKey(string(c)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ParseKey("Z"); err == nil {
		t.Fatal("only eight keys")
	}
}

func TestHelpDoesNotEatAChord(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, "Y Y+R")
	if a.Screen() != Seq || a.help {
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
	atTop := func() bool { return a.dir == filepath.Dir(a.dir) } // "/" here, a drive root such as C:\ on Windows
	for i := 0; i < 40 && !atTop(); i++ {
		keys(t, a, "L")
	}
	if !atTop() {
		t.Fatalf("Left must climb to the top, stopped at %s", a.dir)
	}
	top := a.dir
	keys(t, a, "L") // at the top: stays
	if a.dir != top {
		t.Fatal("went above the top")
	}
	if len(a.entries) == 0 {
		t.Fatal("/ lists nothing")
	}
	// Y+X is the places menu: the samples folder is one of them
	keys(t, a, "Y+X")
	if a.menu == nil || !strings.Contains(a.menu.title, "places") {
		t.Fatal("Y+X opens the places and favourites")
	}
	chooseMenu(t, a, "samples")
	if a.dir != a.Root {
		t.Fatalf("the samples place goes to the samples folder: %q", a.dir)
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
	left, _ := filepath.Glob(filepath.Join(a.OutDir, cardFolder, ".404tui-*"))
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
	keys(t, a, "Y+R Y+R Y+R R*6 A Y+X") // to SLICE: cut the kick in two, put the slices on pads
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

func TestStepGridPadsDownStepsAcross(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // kick, snare, hat on A1..A3, the cursor on A3
	a.go_(Seq)
	// nothing is in the pattern yet: one track, the pad that was in front
	if len(a.rows) != 1 || a.rowPad() != 2 {
		t.Fatalf("an empty pattern shows the pad in front, A3: rows %v", a.rows)
	}
	// Left/Right move the step; the first note makes the track a used one
	keys(t, a, strings.Repeat("R ", 40)+"A")
	if a.step != 40 {
		t.Fatalf("step %d", a.step)
	}
	d, _ := a.doc()
	n, ok := noteAt(d, 2, 40)
	if !ok || int(n.Tick) != 40*stepTicks {
		t.Fatalf("no note at step 41 of the hat: %+v", d.Notes)
	}
	if img := a.Render(); img.Bounds().Dx() != W { // the view has scrolled to step 40
		t.Fatal("no frame")
	}
	// the end is the end
	keys(t, a, strings.Repeat("R ", 100))
	if a.step != a.steps()-1 {
		t.Fatalf("stops at the last step, got %d of %d", a.step, a.steps())
	}
	// Y+X adds a track for the next pad with a sample that is not a row; the cursor goes to it
	keys(t, a, "Y+X A")
	if len(a.rows) != 2 || a.rowPad() != 3 && a.rowPad() != 0 && a.rowPad() != 1 {
		t.Fatalf("a track was added: %v on %d", a.rows, a.rowPad())
	}
	added := a.rowPad()
	if !a.info.Pads[added].File {
		t.Fatalf("the track is a pad with a sample: %d", added)
	}
	for i := 1; i < len(a.rows); i++ {
		if a.rows[i-1] >= a.rows[i] {
			t.Fatalf("rows are in pad order: %v", a.rows)
		}
	}
	// the added track has a note on 1.1 and so stays when the cursor leaves it
	if d, _ := a.doc(); !hasNote(d, added, 0) {
		t.Fatalf("the added track has a note on 1.1: %v", d.Notes)
	}
	keys(t, a, "U")
	if a.rowPad() == added {
		keys(t, a, "D")
	}
	if len(a.rows) != 2 {
		t.Fatalf("the added track stays: %v on %d", a.rows, a.rowPad())
	}
	a.toggle(added, 0) // its note off: now it drops out when the cursor is elsewhere
	for i, p := range a.rows {
		if p != added {
			a.row = i
		}
	}
	a.autoRows()
	if len(a.rows) != 1 {
		t.Fatalf("a track with no notes drops out: %v", a.rows)
	}
	// up and down change the pad (the row)
	a.addTrack(0)
	a.addTrack(1)
	a.step = 3
	keys(t, a, "A") // a note on A2, so that its track stays
	a.addTrack(0)
	keys(t, a, "A")
	a.step = 4
	a.addTrack(2)
	if len(a.rows) != 3 {
		t.Fatalf("A1, A2 and A3 are tracks with notes: %v", a.rows)
	}
	a.row = 0
	keys(t, a, "D")
	if a.row != 1 {
		t.Fatalf("Down is the next pad: row %d", a.row)
	}
	keys(t, a, "U U")
	if a.row != 0 {
		t.Fatalf("Up stops at the first: %d", a.row)
	}
	// a new pattern forgets the tracks added to the old one
	keys(t, a, "Y+U")
	if len(a.rowExtra) != 0 {
		t.Fatalf("the tracks added belong to their pattern: %v", a.rowExtra)
	}
}

func TestOnlyTheTracksInUseShow(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.row = 0
	// put a note on A3 (the pad in front), then add A1 and A2 as tracks and put notes there
	keys(t, a, "A")
	a.addTrack(0)
	keys(t, a, "A")
	a.addTrack(1)
	keys(t, a, "A")
	if fmt.Sprint(a.rows) != "[0 1 2]" {
		t.Fatalf("three tracks in use: %v", a.rows)
	}
	// take the only note off a track and move away: it drops out
	a.row = 1
	a.step = 0
	keys(t, a, "A") // the note on A2 goes (on release)
	if _, ok := noteOn(a, 1); ok {
		t.Fatal("the note should be off")
	}
	if fmt.Sprint(a.rows) != "[0 1 2]" {
		t.Fatalf("the track the cursor is on stays: %v", a.rows)
	}
	keys(t, a, "U")
	if fmt.Sprint(a.rows) != "[0 2]" {
		t.Fatalf("an empty track drops out when the cursor leaves it: %v", a.rows)
	}
	if a.rowPad() != 0 {
		t.Fatalf("the cursor is on A1: %d", a.rowPad())
	}
	// marked pads are tracks too (the keys of the arp)
	a.marks = []int{5}
	a.autoRows()
	if fmt.Sprint(a.rows) != "[0 2 5]" {
		t.Fatalf("a marked pad is a row: %v", a.rows)
	}
	// more than 16 tracks: the view scrolls with the cursor
	a.marks = nil
	a.rowExtra = nil
	for p := 0; p < 24; p++ {
		a.setTrack(p, []lab.Note{{Tick: 0, Pad: p, Vel: 100, Len: stepTicks}})
	}
	a.autoRows()
	if len(a.rows) != 24 || a.rowTop != 0 {
		t.Fatalf("24 rows: %d, top %d", len(a.rows), a.rowTop)
	}
	for i := 0; i < 20; i++ {
		keys(t, a, "D")
	}
	if a.row < a.rowTop || a.row >= a.rowTop+maxRows || a.rowTop == 0 {
		t.Fatalf("the cursor is in view: row %d top %d", a.row, a.rowTop)
	}
	a.Render()
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
	if k, err := ParseKey("]"); err != nil || k != R1 || R1.String() != "R1" {
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
	if a.menu.tab != 1 || a.menu.tabs[1] != "GENERATOR" {
		t.Fatalf("R1 turns to the next tab, the generator: tab %d", a.menu.tab)
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
	m := menuOf(a)
	if m == nil {
		t.Fatalf("no menu is open to choose %q from", label)
	}
	if seekMenu(t, a, func(l string) bool { return strings.HasPrefix(l, label) }) {
		keys(t, a, "A")
		if a.picker != nil && a.picker.pads { // a pad is asked for: the one the dialog starts on
			keys(t, a, "A")
		}
		return
	}
	t.Fatalf("no menu item %q", label)
}

// menuOf is the menu in front: an open one, or the tools page of the pad panel.
func menuOf(a *App) *menu {
	if a.menu != nil {
		return a.menu
	}
	if a.pp != nil && a.pp.page == 2 {
		return a.pp.tools
	}
	return nil
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
	a.sliceToolsMenu()
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
	if k, _ := ParseKey("}"); k != R2 {
		t.Fatal("} is R2")
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
	found := false
	for _, l := range menuLabels(a.menu) {
		found = found || l == "Tempo of bank B"
	}
	if !found {
		t.Fatal("the pattern menu names the tempo it changes: bank B")
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

// waitAutosave waits for a background autosave to finish.
func waitAutosave(t *testing.T, a *App) {
	t.Helper()
	for i := 0; i < 300; i++ {
		a.as.mu.Lock()
		busy := a.as.busy
		a.as.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the autosave did not finish")
}

func lastLog(a *App) string {
	a.as.mu.Lock()
	defer a.as.mu.Unlock()
	if len(a.as.history) == 0 {
		return ""
	}
	return a.as.history[len(a.as.history)-1]
}

func TestAutosaveWritesOnlyWhatChanged(t *testing.T) {
	a, _ := newApp(t)
	now := time.Now()
	a.Now = func() time.Time { return now }
	a.as.lastRun = now
	keys(t, a, kit)
	a.Tick()
	if lastLog(a) != "" {
		t.Fatal("nothing is due before the interval")
	}
	now = now.Add(4 * time.Minute)
	a.Tick()
	waitAutosave(t, a)
	dir := filepath.Join(a.autosaveRoot(), "PROJECT_01")
	if _, err := os.Stat(filepath.Join(dir, "PADCONF.BIN")); err != nil {
		t.Fatalf("no autosave: %v (%s)", err, lastLog(a))
	}
	if smp, _ := filepath.Glob(filepath.Join(dir, "SMPL", "*.SMP")); len(smp) != 3 {
		t.Fatalf("3 samples expected, %d", len(smp))
	}
	if !strings.Contains(lastLog(a), "saved ") || strings.Contains(lastLog(a), "failed") {
		t.Fatalf("log: %q", lastLog(a))
	}
	t.Log(lastLog(a))
	// the saved folder (not the autosave) is untouched
	if _, err := os.Stat(filepath.Join(a.OutDir, cardFolder, "PROJECT_01")); err == nil {
		t.Fatal("an autosave must not write where a save does")
	}
	// one pad cleared: PADCONF changes and one sample goes
	a.go_(Pads)
	keys(t, a, "Y+B")
	now = now.Add(4 * time.Minute)
	a.Tick()
	waitAutosave(t, a)
	if !strings.Contains(lastLog(a), "saved 2 of") {
		t.Fatalf("only the changed files are written: %q", lastLog(a))
	}
	if smp, _ := filepath.Glob(filepath.Join(dir, "SMPL", "*.SMP")); len(smp) != 2 {
		t.Fatalf("the cleared pad's sample should be gone: %d", len(smp))
	}
	// no change, no run
	n := len(a.as.history)
	a.as.mu.Lock()
	a.changed = false
	a.as.mu.Unlock()
	now = now.Add(10 * time.Minute)
	a.Tick()
	waitAutosave(t, a)
	if len(a.as.history) != n {
		t.Fatal("an unchanged project is not saved again")
	}
	// the interval is chosen on SAVE and remembered
	a.go_(Save)
	keys(t, a, "U")
	if a.set.AutosaveMin == nil || *a.set.AutosaveMin != 5 || a.as.every != 5*time.Minute {
		t.Fatalf("every %v, settings %v", a.as.every, a.set.AutosaveMin)
	}
	if raw, err := os.ReadFile(a.settingsPath()); err != nil || !strings.Contains(string(raw), `"autosave_min": 5`) {
		t.Fatalf("the interval is in settings.json beside the card folder: %q (%v)", raw, err)
	}
	if strings.HasPrefix(a.settingsPath(), a.OutDir+string(filepath.Separator)) || strings.HasPrefix(a.autosaveRoot(), a.OutDir+string(filepath.Separator)) {
		t.Fatal("settings and autosaves must stay out of the card folder")
	}
	keys(t, a, "D D D D") // 5 -> 3 -> 2 -> 1 -> off
	if a.as.every != 0 || a.autosaveSetting() != "off" {
		t.Fatalf("off: %v", a.as.every)
	}
	a.Render()
	if a.autosaveAgo() != "autosave off" {
		t.Fatal(a.autosaveAgo())
	}
	// a restart finds it and the splash offers it
	c := New(a.Root, a.OutDir, nil)
	c.ShowSplash(time.Second)
	if c.splash == nil || c.splash.restore == "" {
		t.Fatal("the splash should offer the autosave to an empty project")
	}
	c.Render()
	c.Press(A, 0)
	if c.splash != nil || padsWithSamples(c) != 2 {
		t.Fatalf("A restores it: %d pads", padsWithSamples(c))
	}
	if c.saved {
		t.Fatal("a restored autosave is not a save")
	}
}

func TestSplashGoesAwayByItselfOrByAKey(t *testing.T) {
	a, _ := newApp(t)
	now := time.Now()
	a.Now = func() time.Time { return now }
	a.ShowSplash(2 * time.Second)
	if a.splash == nil || a.splash.restore != "" {
		t.Fatal("no autosave, so the splash only shows")
	}
	a.Render()
	now = now.Add(1 * time.Second)
	a.Tick()
	if a.splash == nil {
		t.Fatal("still showing")
	}
	now = now.Add(2 * time.Second)
	a.Tick()
	if a.splash != nil {
		t.Fatal("it ends by itself")
	}
	a.ShowSplash(time.Minute)
	keys(t, a, "D") // a key skips it and is not used otherwise
	if a.splash != nil || a.cur != 0 {
		t.Fatalf("a key only closes the splash (cursor %d)", a.cur)
	}
}

func TestArpTypes(t *testing.T) {
	flat := func(typ, n, steps int) []int {
		var out []int
		for _, s := range arpSequence(n, typ, steps, rand.New(rand.NewSource(1))) {
			if len(s) != 1 {
				t.Fatalf("type %d plays %d keys on a step", typ, len(s))
			}
			out = append(out, s[0])
		}
		return out
	}
	cases := []struct {
		typ, n, steps int
		want          []int
	}{
		{0, 5, 7, []int{0, 1, 2, 3, 4, 0, 1}},    // UP wraps
		{1, 4, 5, []int{3, 2, 1, 0, 3}},          // DOWN
		{2, 4, 8, []int{0, 1, 2, 3, 2, 1, 0, 1}}, // U/D: the ends once
		{3, 3, 8, []int{0, 1, 2, 2, 1, 0, 0, 1}}, // U/D+: the ends twice
		{7, 5, 5, []int{0, 4, 1, 3, 2}},          // CONV: outside in
		{8, 5, 5, []int{2, 3, 1, 4, 0}},          // DIV: middle out
		{9, 4, 6, []int{0, 3, 1, 3, 2, 3}},       // PNKY: the top key between
		{10, 4, 6, []int{0, 1, 0, 2, 0, 3}},      // THMB: the bottom key between
		{5, 3, 4, []int{0, 1, 2, 0}},             // ORD
	}
	for _, c := range cases {
		got := flat(c.typ, c.n, c.steps)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s with %d keys: %v, want %v", arpTypes[c.typ], c.n, got, c.want)
		}
	}
	if s := arpSequence(4, 6, 2, nil); len(s[0]) != 4 { // CHRD: every key on every step
		t.Errorf("CHRD plays %v", s)
	}
	for _, s := range arpSequence(5, 4, 200, rand.New(rand.NewSource(3))) { // RND stays among the keys
		if len(s) != 1 || s[0] < 0 || s[0] > 4 {
			t.Fatalf("RND: %v", s)
		}
	}
	if len(arpTypes) != 11 || len(arpTypeNotes) != 11 {
		t.Fatal("each type has a name and a note")
	}
}

func TestPatternArpPrintsOverTheMarkedPads(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // A1..A3 have samples
	a.go_(Seq)
	a.marks = []int{0, 1, 2}
	a.arpPrintDefaults()
	keys3 := a.markedKeys()
	if len(keys3) != 3 {
		t.Fatalf("keys %v", keys3)
	}
	// 1/16 over 2 bars = 32 steps: UP over 3 keys puts a note on every step, on the keys in turn
	a.arpp.rate, a.arpp.typ = 3, 0
	a.printArpKeys(keys3)
	d, ok := a.doc()
	if !ok || d.Bars != 2 || len(d.Notes) != 32 {
		t.Fatalf("%d notes in %d bars", len(d.Notes), d.Bars)
	}
	sort.Slice(d.Notes, func(i, j int) bool { return d.Notes[i].Tick < d.Notes[j].Tick })
	for i, n := range d.Notes {
		if int(n.Tick) != i*stepTicks || n.Pad != keys3[i%len(keys3)] {
			t.Fatalf("note %d: tick %d pad %d, want tick %d pad %d", i, n.Tick, n.Pad, i*stepTicks, keys3[i%len(keys3)])
		}
		if n.Len != stepTicks/2 || n.Vel != 100 {
			t.Fatalf("gate 50%% and velocity 100: len %d vel %d", n.Len, n.Vel)
		}
	}
	// printing again replaces the notes of the key pads, it does not add
	a.arpp.typ = 6 // CHRD: every key on every step
	a.arpp.rate = 1
	a.printArpKeys(keys3)
	d, _ = a.doc()
	if len(d.Notes) != 2*4*2*len(keys3) {
		t.Fatalf("CHRD at 1/8 over 2 bars: %d notes", len(d.Notes))
	}
	// with a single key there is nothing to arpeggiate
	a2, _ := newApp(t)
	keys(t, a2, "A A D R D D A")
	a2.go_(Seq)
	a2.printArpKeys(a2.markedKeys())
	if !a2.msgBad {
		t.Fatal("one key is not an arp")
	}
}

func TestSampleToolsArpAndKeyboard(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	a.cur = 0 // A1, the kick
	a.go_(Pads)
	keys(t, a, "A [")
	chooseMenu(t, a, "Chords & arps")
	if a.menu == nil || !strings.Contains(a.menu.title, "A1") {
		t.Fatalf("the arp menu: %+v", a.menu)
	}
	a.Render()
	au.Played = 0
	chooseMenu(t, a, "Play it")
	if au.Played != 1 {
		t.Fatalf("Play it plays the arp: %d (%s)", au.Played, a.msg)
	}
	before := padsWithSamples(a)
	a.menu = nil
	a.arpToPad(0)
	keys(t, a, "A") // the pad dialog: the free pad it starts on
	if padsWithSamples(a) != before+1 {
		t.Fatalf("the arp goes on a free pad: %s", a.msg)
	}
	// the keyboard: 5 pitched copies on free pads, a major scale
	a.menu = nil
	a.keyboard(0, 5, 0)
	if padsWithSamples(a) != before+1+5 {
		t.Fatalf("5 keys: %s", a.msg)
	}
	if keySemis(2, 0) != 4 || keySemis(7, 0) != 12 || keySemis(3, -1) != 3 {
		t.Fatalf("scale steps: %d %d %d", keySemis(2, 0), keySemis(7, 0), keySemis(3, -1))
	}
	// the arp lays the notes over the keyboard
	a.marks = []int{a.padN(), a.padN() + 1, a.padN() + 2, a.padN() + 3, a.padN() + 4}
	a.go_(Pads)
	a.go_(Seq)
	if len(a.markedKeys()) < 5 || len(a.rows) < 5 {
		t.Fatalf("the marked keyboard pads are the rows: %v", a.rows)
	}
}

func TestMenuHeadingsAreSkipped(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.menu = a.padToolsMenu(0) // a menu with headings and no tabs
	if a.menu.items[0].header != true {
		t.Fatal("the menu starts with a heading")
	}
	keys(t, a, "D") // the first Down must not rest on a heading
	for i := 0; i < len(a.menu.items)*2; i++ {
		if a.menu.item().header {
			t.Fatalf("the cursor is on the heading %q", a.menu.item().label)
		}
		keys(t, a, "D")
	}
	for i := 0; i < len(a.menu.items)*2; i++ {
		keys(t, a, "U")
		if a.menu.item().header {
			t.Fatal("the cursor went up onto a heading")
		}
	}
	keys(t, a, "]") // L1/R1 page the list and still miss the headings
	if a.menu.item().header {
		t.Fatal("a page lands on a heading")
	}
	a.Render()
}

func TestMarkPadsAndLiveArp(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit) // samples on A1..A3, the cursor on A3
	a.go_(Pads)
	a.cur = 0
	keys(t, a, "Y+A")
	if len(a.marks) != 1 || a.marks[0] != 0 {
		t.Fatalf("A1 is marked: %v", a.marks)
	}
	keys(t, a, "R")
	keys(t, a, "Y+A")
	a.cur = 5
	keys(t, a, "Y+A") // an empty pad cannot be a key
	if len(a.marks) != 2 || !a.msgBad {
		t.Fatalf("an empty pad is not marked: %v (%s)", a.marks, a.msg)
	}
	a.Render() // the badges are drawn
	// one mark is not an arp; two are
	keys(t, a, "Y+A") // unmark A2 again... (the cursor is on the empty pad: nothing happens)
	a.cur = 1
	keys(t, a, "Y+A")
	if len(a.marks) != 1 {
		t.Fatalf("Y+A on a marked pad unmarks it: %v", a.marks)
	}
	keys(t, a, "}")
	if a.arpOn || !a.msgBad {
		t.Fatal("R2 with one marked pad says to mark more")
	}
	a.cur = 1
	keys(t, a, "Y+A")
	a.cur = 2
	keys(t, a, "Y+A")
	au.Loops = 0
	keys(t, a, "}")
	if !a.arpOn || au.Loops != 1 {
		t.Fatalf("R2 loops the arp over the 3 marked pads: on %v, loops %d (%s)", a.arpOn, au.Loops, a.msg)
	}
	a.Render() // the panel says ARP
	// a change of setting while it plays goes into the loop
	a.openMarkedArpMenu()
	keys(t, a, "R")
	if au.Loops != 2 {
		t.Fatalf("Right on Type goes into the loop without a gap: loops %d", au.Loops)
	}
	a.menu = nil
	keys(t, a, "B")
	if a.arpOn {
		t.Fatal("B stops the arp")
	}
	keys(t, a, "}")
	a.go_(Files)
	if a.arpOn {
		t.Fatal("leaving PADS stops it")
	}
	// the same keys, written into the pattern, and the marked pads lead the columns
	a.go_(Pads)
	a.openMarkedArpMenu()
	chooseMenu(t, a, "Write it into the pattern")
	if a.scr != Seq {
		t.Fatalf("writing shows the pattern: %v", a.scr)
	}
	d, ok := a.doc()
	if !ok || len(d.Notes) == 0 {
		t.Fatalf("no notes written: %s", a.msg)
	}
	if a.rows[0] != 0 || a.rows[1] != 1 || a.rows[2] != 2 {
		t.Fatalf("the marked pads are the columns: %v", a.rows)
	}
	// and bounced to a pad
	before := padsWithSamples(a)
	a.go_(Pads)
	a.bounceMarkedArp()
	keys(t, a, "A")
	if padsWithSamples(a) != before+1 {
		t.Fatalf("the arp goes on a free pad: %s", a.msg)
	}
	keys(t, a, "{") // L2 clears the marks
	if len(a.marks) != 0 {
		t.Fatal("L2 clears the marks")
	}
}

func TestFileBrowserRemembersWhereYouWere(t *testing.T) {
	a, _ := newApp(t)
	a.go_(Pads)
	keys(t, a, "A A D R D") // pick a sample: into FILES, bass, drums, ... open drums
	if filepath.Base(a.dir) != "drums" {
		t.Fatalf("in %s", a.dir)
	}
	keys(t, a, "D D")
	cursor, dir := a.fcur, a.dir
	keys(t, a, "A") // assigned: back on the pads
	if a.scr != Pads {
		t.Fatal("assigned")
	}
	a.cur = 3
	keys(t, a, "A A")
	if a.dir != dir || a.fcur != cursor {
		t.Fatalf("the browser comes back to %s item %d, not %s item %d", dir, cursor, a.dir, a.fcur)
	}
	// leaving a folder and coming back puts the cursor where it was
	keys(t, a, "L")
	keys(t, a, "R") // the folder under the cursor is drums again
	if a.fcur != cursor {
		t.Fatalf("drums again: cursor %d, was %d", a.fcur, cursor)
	}
	// the folder is kept for the next start
	b := New(a.Root, a.OutDir, nil)
	if b.dir != dir {
		t.Fatalf("the next start is in %s, not %s", b.dir, dir)
	}
}

func TestPreviewDoesNotCutThePatternAndAuditionsInIt(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit) // kick, snare, hat on A1..A3
	a.go_(Seq)
	a.setRow(0, rowPresets[0].mask) // the kick on every beat
	keys(t, a, "X")
	if !a.seqOn || au.Loops != 1 {
		t.Fatalf("the pattern plays: on %v loops %d", a.seqOn, au.Loops)
	}
	// a file for pad A1, listened to while the pattern plays
	a.cur, a.bank = 0, 0
	a.go_(Files)
	a.dir = filepath.Join(a.Root, "drums")
	a.list()
	for i, e := range a.entries {
		if strings.HasPrefix(e.name, "snare") {
			a.fcur = i
		}
	}
	au.Loops = 0
	keys(t, a, "X")
	if !a.seqOn {
		t.Fatal("listening must not stop the pattern")
	}
	if _, in := a.aud[0]; !in || au.Loops == 0 {
		t.Fatalf("A1 has notes, so the file is heard in the pattern in its place: aud %v, loops %d", a.aud, au.Loops)
	}
	// a pad without notes in the pattern: the file plays over the loop
	a.aud = nil
	a.cur = 9
	au.Previewed = 0
	keys(t, a, "X")
	if au.Previewed != 1 || !a.seqOn {
		t.Fatalf("a preview over the loop: %d previews, pattern on %v", au.Previewed, a.seqOn)
	}
	// assigning on A1 keeps the pattern going, with the new sample
	a.cur = 0
	keys(t, a, "X")
	before := a.info.Pads[0].Name
	au.Loops = 0
	keys(t, a, "A")
	if !a.seqOn || au.Loops == 0 {
		t.Fatalf("the pattern goes on with the new sample: on %v loops %d", a.seqOn, au.Loops)
	}
	if a.info.Pads[0].Name == before || a.aud != nil {
		t.Fatalf("the sample changed (%q -> %q) and the audition is over: %v", before, a.info.Pads[0].Name, a.aud)
	}
	// B on the pads stops everything
	keys(t, a, "B")
	if a.seqOn {
		t.Fatal("B stops the pattern")
	}
}

func TestSearchFavouritesAndMemory(t *testing.T) {
	a, _ := newApp(t)
	a.go_(Files)
	a.cd(a.Root)
	// search: kick_808.wav is in drums/
	keys(t, a, "Y+A")
	if a.kb == nil {
		t.Fatal("Y+A opens the on-screen keyboard")
	}
	for _, ch := range "kick" {
		typeOn(t, a, byte(ch))
	}
	if a.searchQ != "kick" || len(a.entries) != 1 || !strings.Contains(a.entries[0].name, "kick") {
		t.Fatalf("search %q: %v", a.searchQ, a.entries)
	}
	a.Render()
	keys(t, a, "B B") // delete two letters: "ki" matches more files? at least the kick
	if a.searchQ != "ki" {
		t.Fatalf("B deletes a letter: %q", a.searchQ)
	}
	keys(t, a, "X") // done
	if a.kb != nil || len(a.entries) == 0 {
		t.Fatal("X closes the keyboard and keeps the results")
	}
	keys(t, a, "A") // a result goes to the pad
	if !a.info.Pads[a.padN()].File || !strings.HasPrefix(a.info.Pads[a.padN()].Name, "kick") {
		t.Fatalf("assigned %q (a result is a path: the pad gets the file's name)", a.info.Pads[a.padN()].Name)
	}
	a.go_(Files)
	keys(t, a, "L") // ends the search
	if a.searchQ != "" || a.dir != a.Root {
		t.Fatalf("Left ends the search and stays in the folder: %q %q", a.searchQ, a.dir)
	}
	// favourites
	drums := filepath.Join(a.Root, "drums")
	a.cd(drums)
	keys(t, a, "Y+B")
	if !a.isFavourite(drums) {
		t.Fatal("Y+B makes the folder a favourite")
	}
	a.cd(a.Root)
	keys(t, a, "Y+X")
	chooseMenuContaining(t, a, "drums") // the label is the end of the path
	if a.dir != drums {
		t.Fatalf("the favourite is in the places menu: in %s", a.dir)
	}
	b := New(a.Root, a.OutDir, nil)
	if !b.isFavourite(drums) {
		t.Fatal("favourites are kept")
	}
	// browsing many files does not keep them all decoded
	wav, _ := os.ReadFile(filepath.Join(drums, "kick_808.wav"))
	big := filepath.Join(a.Root, "many")
	os.MkdirAll(big, 0o755)
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(big, fmt.Sprintf("s%02d.wav", i)), wav, 0o644)
	}
	a.cd(big)
	for i := range a.entries {
		a.fcur = i
		a.Render()
	}
	if len(a.prev) > 8 {
		t.Fatalf("%d files kept decoded, the limit is 8", len(a.prev))
	}
}

// typeOn moves the cursor of the on-screen keyboard to a character and types it.
func typeOn(t *testing.T, a *App, ch byte) {
	t.Helper()
	for r, row := range kbRows {
		for c := 0; c < len(row); c++ {
			if row[c] != ch {
				continue
			}
			for a.kb.r != r {
				keys(t, a, "D")
			}
			for a.kb.c != c {
				keys(t, a, "R")
			}
			keys(t, a, "A")
			return
		}
	}
	t.Fatalf("no key %q", ch)
}

// chooseMenuContaining is chooseMenu for an item with the text somewhere in its label.
func chooseMenuContaining(t *testing.T, a *App, text string) {
	t.Helper()
	if seekMenu(t, a, func(l string) bool { return strings.Contains(l, text) }) {
		keys(t, a, "A")
		return
	}
	t.Fatalf("no menu item with %q", text)
}

func TestUndoRedoOfPadAndPatternEdits(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, "Y+{")
	if !strings.Contains(a.msg, "nothing to undo") {
		t.Fatalf("an empty stack says so: %q", a.msg)
	}
	if a.help {
		t.Fatal("Y+L2 is a chord: Y must not show the help afterwards")
	}
	keys(t, a, kit) // three pads, one change each
	if padsWithSamples(a) != 3 || len(a.undo) != 3 {
		t.Fatalf("3 pads and 3 steps: %d pads, %d steps", padsWithSamples(a), len(a.undo))
	}
	keys(t, a, "Y+{")
	if padsWithSamples(a) != 2 || !strings.HasPrefix(a.msg, "undone:") {
		t.Fatalf("undo takes the last pad away: %d (%s)", padsWithSamples(a), a.msg)
	}
	keys(t, a, "Y+{ Y+{")
	if padsWithSamples(a) != 0 || len(a.undo) != 0 || len(a.redo) != 3 {
		t.Fatalf("all undone: %d pads, undo %d redo %d", padsWithSamples(a), len(a.undo), len(a.redo))
	}
	keys(t, a, "Y+}")
	if padsWithSamples(a) != 1 || !a.info.Pads[0].File {
		t.Fatalf("redo puts the first pad back: %d", padsWithSamples(a))
	}
	keys(t, a, "Y+} Y+}")
	if padsWithSamples(a) != 3 {
		t.Fatalf("redo all: %d", padsWithSamples(a))
	}
	// the undo itself is not an edit, and a new edit ends the redo
	keys(t, a, "Y+{")
	if len(a.undo) != 2 || len(a.redo) != 1 {
		t.Fatalf("undo %d redo %d", len(a.undo), len(a.redo))
	}
	a.go_(Seq)
	keys(t, a, "A") // a note: a new edit
	if len(a.redo) != 0 || len(a.undo) != 3 {
		t.Fatalf("a new edit empties redo: undo %d redo %d", len(a.undo), len(a.redo))
	}
	d, _ := a.doc()
	if len(d.Notes) != 1 {
		t.Fatalf("a note: %d", len(d.Notes))
	}
	keys(t, a, "Y+{")
	if d, ok := a.doc(); ok && len(d.Notes) != 0 {
		t.Fatalf("undo takes the note away: %d", len(d.Notes))
	}
	// the tempo (pad settings) comes back too
	a.seqMenu()
	chooseMenuContaining(t, a, "Tempo of bank")
	keys(t, a, "R")
	if a.tempo() != 121 {
		t.Fatalf("tempo %v", a.tempo())
	}
	a.menu = nil
	keys(t, a, "Y+{")
	if a.tempo() != 120 {
		t.Fatalf("undo gives back the tempo: %v", a.tempo())
	}
	// a pattern that plays goes on with what undo gave back
	keys(t, a, "A R R R R A") // two notes
	keys(t, a, "X")
	au.Loops = 0
	keys(t, a, "Y+{") // the second note goes
	if au.Loops == 0 || !a.seqOn {
		t.Fatalf("undo while the pattern plays goes into the loop: loops %d, playing %v", au.Loops, a.seqOn)
	}
	keys(t, a, "Y+{") // and the first: nothing is left to play
	if a.seqOn {
		t.Fatal("an emptied pattern stops")
	}
}

func TestUndoCoalescesAHeldKeyAndIsBounded(t *testing.T) {
	a, _ := newApp(t)
	now := time.Now()
	a.Now = func() time.Time { return now }
	keys(t, a, kit)
	base := len(a.undo)
	a.go_(Seq)
	a.seqMenu()
	chooseMenuContaining(t, a, "Tempo of bank")
	for i := 0; i < 10; i++ { // Right held: the first press, then repeats
		now = now.Add(80 * time.Millisecond)
		a.Press(Right, i)
	}
	if a.tempo() != 130 {
		t.Fatalf("10 steps of tempo: %v", a.tempo())
	}
	if len(a.undo) != base+1 {
		t.Fatalf("a held key is one undo step: %d steps, wanted %d", len(a.undo), base+1)
	}
	a.menu = nil
	keys(t, a, "Y+{")
	if a.tempo() != 120 {
		t.Fatalf("one undo gives all ten steps back: %v", a.tempo())
	}
	// 40 separate edits: the stack stops at 30
	a.go_(Seq)
	for i := 0; i < 40; i++ {
		now = now.Add(2 * time.Second)
		a.Press(A, 0)
		a.Release(A)
	}
	if len(a.undo) > maxUndo {
		t.Fatalf("the stack holds %d steps, the limit is %d", len(a.undo), maxUndo)
	}
}

func TestUndoCountsTheSamplesItHolds(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	if a.undoHeld() != 0 {
		t.Fatalf("while every file is still in the project the stack holds nothing extra: %d", a.undoHeld())
	}
	// a file put on A1 replaces the kick: the old one is only kept alive by its undo step
	a.go_(Files)
	a.cd(filepath.Join(a.Root, "drums"))
	a.cur, a.bank = 0, 0
	for i, e := range a.entries {
		if strings.HasPrefix(e.name, "snare") {
			a.fcur = i
		}
	}
	keys(t, a, "A")
	if h := a.undoHeld(); h < 1000 {
		t.Fatalf("the replaced sample is counted: %d bytes", h)
	}
	keys(t, a, "Y+{")
	if a.info.Pads[0].Name != "kick_808" {
		t.Fatalf("undo gave the kick back: %q", a.info.Pads[0].Name)
	}
}

func TestUndoAProjectLoad(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Save)
	keys(t, a, "A") // written
	card := filepath.Join(a.OutDir, cardFolder)
	b, _ := newApp(t)
	keys(t, b, "A A D D R A") // one loop sample: a different project
	if padsWithSamples(b) != 1 {
		t.Fatalf("setup: %d", padsWithSamples(b))
	}
	b.go_(Files)
	b.cd(card)
	for i, e := range b.entries {
		if e.name == "PROJECT_01" {
			b.fcur = i
		}
	}
	b.saved = true // nothing to lose: the load does not ask
	keys(t, b, "A")
	if padsWithSamples(b) != 3 {
		t.Fatalf("the project is loaded: %d pads (%s)", padsWithSamples(b), b.msg)
	}
	keys(t, b, "Y+{")
	if padsWithSamples(b) != 1 {
		t.Fatalf("undo brings back the project that was open: %d pads", padsWithSamples(b))
	}
	keys(t, b, "Y+}")
	if padsWithSamples(b) != 3 {
		t.Fatalf("redo loads it again: %d", padsWithSamples(b))
	}
}

func noteOn(a *App, pad int) (lab.Note, bool) {
	d, _ := a.doc()
	for _, n := range d.Notes {
		if n.Pad == pad {
			return n, true
		}
	}
	return lab.Note{}, false
}

func TestHoldAEditsTheNote(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0) // A1
	a.step = 4    // step 5
	base := len(a.undo)
	// A down puts a note; with A held the arrows shape it: Up velocity, Right length, R1 nudge
	a.Press(A, 0)
	if !a.aHeld {
		t.Fatal("A is held")
	}
	n, ok := noteOn(a, 0)
	if !ok || n.Vel != 100 || n.Len != stepTicks {
		t.Fatalf("a new note: %+v", n)
	}
	for i := 0; i < 5; i++ {
		a.Press(Up, i%3) // short holds: one at a time
	}
	for i := 0; i < 4; i++ {
		a.Press(Right, 0)
	}
	a.Press(R1, 0)
	au.Played = 0
	a.Press(Down, 0)
	a.Release(Up)
	a.Release(A)
	n, _ = noteOn(a, 0)
	if n.Vel != 104 || n.Len != stepTicks+60 || int(n.Tick) != 4*stepTicks+5 {
		t.Fatalf("velocity 100+5-1, length +4 quarter steps, nudged 5 ticks: %+v", n)
	}
	if au.Played == 0 {
		t.Fatal("a change is heard: the note plays")
	}
	if len(a.undo) != base+1 {
		t.Fatalf("placing and shaping a note is one undo step: %d -> %d", base, len(a.undo))
	}
	if a.aHeld {
		t.Fatal("A is let go")
	}
	// keys no longer edit
	a.Press(Up, 0)
	a.Release(Up)
	if n2, _ := noteOn(a, 0); n2.Vel != 104 || a.row != 0 {
		t.Fatalf("with A up the arrows move the cursor: %+v row %d", n2, a.row)
	}
	// a tap of A on a note takes it off, on release
	a.step = int(n.Tick) / stepTicks
	a.row = 0
	a.Press(A, 0)
	if _, still := noteOn(a, 0); !still {
		t.Fatal("the note waits for the release")
	}
	a.Release(A)
	if _, still := noteOn(a, 0); still {
		t.Fatal("a tap on a note removes it")
	}
	keys(t, a, "Y+{") // and undo brings it back, as it was
	if n3, ok := noteOn(a, 0); !ok || n3.Vel != 104 {
		t.Fatalf("undo: %+v %v", n3, ok)
	}
	// holding A on a note and using the arrows edits it and does not remove it
	a.step = int(n.Tick) / stepTicks
	a.Press(A, 0)
	a.Press(Up, 0)
	a.Release(A)
	if n4, ok := noteOn(a, 0); !ok || n4.Vel != 105 {
		t.Fatalf("held and edited: %+v %v", n4, ok)
	}
	// limits: velocity 1..127, length 15 ticks up to the end of the pattern
	a.step = int(n.Tick) / stepTicks
	a.Press(A, 0)
	for i := 0; i < 200; i++ {
		a.Press(Down, 0)
		a.Press(Left, 0)
	}
	a.Release(A)
	if n5, _ := noteOn(a, 0); n5.Vel != 1 || n5.Len != 15 {
		t.Fatalf("lower limits: %+v", n5)
	}
	// the script chord does the same
	a2, _ := newApp(t)
	keys(t, a2, kit)
	a2.go_(Seq)
	a2.addTrack(0)
	keys(t, a2, "A+U A+U A+R")
	if n6, ok := noteOn(a2, 0); !ok || n6.Vel != 102 || n6.Len != stepTicks+15 {
		t.Fatalf("chord: %+v %v", n6, ok)
	}
}

func TestHearTheNoteUnderTheCursor(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	au.Played = 0
	keys(t, a, "{")
	if au.Played != 1 {
		t.Fatalf("L2 plays the pad even without a note: %d", au.Played)
	}
	keys(t, a, "A")
	n, _ := noteOn(a, 0)
	short, _ := a.noteSample(0, 64, 60, 0)
	long, _ := a.noteSample(0, 127, 100000, 0)
	if short.Frames() >= long.Frames() {
		t.Fatalf("a short note is cut: %d vs %d frames", short.Frames(), long.Frames())
	}
	var maxS, maxL int
	for _, v := range short.Samples {
		maxS = max(maxS, int(abs(int(v))))
	}
	for _, v := range long.Samples {
		maxL = max(maxL, int(abs(int(v))))
	}
	if maxS >= maxL {
		t.Fatalf("a soft note is quieter: %d vs %d", maxS, maxL)
	}
	_ = n
}

func TestTimingAppliesToEveryPadOrOneTrack(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.setRow(0, rowPresets[4].mask) // sixteenths on A1: the off-beat ones swing
	a.setRow(1, rowPresets[4].mask) // and on A2
	ticks := func(pad int) []uint32 {
		var out []uint32
		for _, n := range a.trackNotes(pad) {
			out = append(out, n.Tick)
		}
		return out
	}
	before1 := fmt.Sprint(ticks(1))
	a.autoRows()
	a.addTrack(0)
	a.seqMenu()
	chooseMenu(t, a, "Applies to")
	keys(t, a, "R")
	if !a.scopeRow {
		t.Fatal("Applies to: only this pad")
	}
	chooseMenu(t, a, "Swing")
	if fmt.Sprint(ticks(1)) != before1 {
		t.Fatal("A2 was not the track chosen: its notes must not move")
	}
	if fmt.Sprint(ticks(0)) == fmt.Sprint(ticks(0)[:0]) || len(ticks(0)) == 0 {
		t.Fatal("A1 has notes")
	}
	moved := false
	for _, tk := range ticks(0) {
		moved = moved || tk%uint32(stepTicks) != 0
	}
	if !moved {
		t.Fatalf("swing on A1: %v", ticks(0))
	}
	if a.menu == nil {
		t.Fatal("the menu stays open: more can be changed")
	}
	// every pad again
	a.scopeRow = false
	chooseMenu(t, a, "Humanize")
	if fmt.Sprint(ticks(1)) == before1 {
		t.Fatal("with every pad, A2 changes too")
	}
}

func TestTrackTools(t *testing.T) {
	// euclid: 4 hits in 16 steps are every fourth step; rotate moves them; the cycle repeats over the length
	ns := euclidNotes(0, 1920, 120, 16, 4, 0, 50, 100)
	if len(ns) != 4 || ns[1].Tick != 480 || ns[0].Len != 60 || ns[0].Vel != 100 {
		t.Fatalf("euclid 4/16: %+v", ns)
	}
	ns = euclidNotes(0, 3840, 120, 16, 4, 2, 100, 100) // two bars, rotated by 2 steps
	if len(ns) != 8 || ns[0].Tick != 240 {
		t.Fatalf("rotated, two bars: %d notes, first at %d", len(ns), ns[0].Tick)
	}
	if got := euclidNotes(0, 1920, 120, 8, 3, 0, 50, 100); len(got) != 6 { // 3 hits in 8 steps, 16 steps long: twice
		t.Fatalf("euclid 3/8 over a bar: %d", len(got))
	}
	if got := euclidNotes(0, 1920, 120, 4, 0, 0, 50, 100); len(got) != 0 {
		t.Fatal("no hits, no notes")
	}
	base := euclidNotes(0, 1920, 120, 16, 16, 0, 50, 90) // every step
	if len(chanceNotes(base, 100, 0, 1)) != 16 || len(chanceNotes(base, 0, 0, 1)) != 0 {
		t.Fatal("chance 100 keeps all, 0 none")
	}
	if k := len(chanceNotes(base, 50, 0, 7)); k < 3 || k > 13 {
		t.Fatalf("about half: %d", k)
	}
	// with a loop of 4 the choices repeat
	kept := chanceNotes(base, 50, 4, 9)
	got := map[int]bool{}
	for _, n := range kept {
		got[int(n.Tick)/120] = true
	}
	for i := 4; i < 16; i++ {
		if got[i] != got[i%4] {
			t.Fatalf("the choices repeat every 4 notes: step %d", i)
		}
	}
	r := randomizeNotes(base, 20, false, 0, 3)
	for i, n := range r {
		if n.Vel < 70 || n.Vel > 110 || n.Tick != base[i].Tick {
			t.Fatalf("randomize keeps the timing and moves the velocity by at most 20: %+v", n)
		}
	}
	if randomizeNotes(base, 5, true, 30, 3)[0].Vel < 90+30-5 {
		t.Fatal("the bias lifts")
	}
	e := echoNotes([]lab.Note{{Tick: 0, Pad: 1, Vel: 100, Len: 120}}, 1920, 240, 3, 50, 50)
	if len(e) != 4 || e[1].Tick != 240 || e[1].Vel != 50 || e[3].Vel != 12 {
		t.Fatalf("echo x50%% three times: %+v", e)
	}
	if got := echoNotes([]lab.Note{{Tick: 1800, Pad: 1, Vel: 100, Len: 120}}, 1920, 240, 8, 70, 50); len(got) != 1 {
		t.Fatalf("an echo past the end is cut: %d", len(got))
	}
	c := cycleNotes([]lab.Note{{Tick: 0, Vel: 100, Len: 120}, {Tick: 240, Vel: 100, Len: 120}, {Tick: 1200, Vel: 100, Len: 120}}, 1920, 480)
	if len(c) != 8 || c[2].Tick != 480 {
		t.Fatalf("cycle of 4 steps over a bar: %d %+v", len(c), c)
	}
	inv := invertNotes([]lab.Note{{Tick: 0}, {Tick: 240}}, 3, 480, 100)
	if len(inv) != 2 || inv[0].Tick != 120 || inv[1].Tick != 360 || inv[0].Pad != 3 {
		t.Fatalf("invert: %+v", inv)
	}
}

func TestTrackToolsInThePattern(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.seqMenu()
	chooseMenu(t, a, "Euclid")
	if a.menu == nil || !strings.Contains(a.menu.title, "euclid") {
		t.Fatalf("euclid menu: %+v", a.menu)
	}
	if got := len(a.trackNotes(0)); got != 5*4 {
		t.Fatalf("5 hits in 16 steps, 4 bars, heard at once: %d notes", got)
	}
	chooseMenuContaining(t, a, "Keep it")
	keys(t, a, "B")
	a.seqMenu()
	chooseMenu(t, a, "Echo")
	if len(a.trackNotes(0)) <= 20 {
		t.Fatal("echo adds notes, already in the trial")
	}
	chooseMenuContaining(t, a, "Keep it")
	keys(t, a, "B")
	// dispatch over marked pads moves notes onto them
	a.marks = []int{1, 2}
	a.seqMenu()
	chooseMenuContaining(t, a, "Dispatch")
	if len(a.trackNotes(0)) != 0 || len(a.trackNotes(1)) == 0 || len(a.trackNotes(2)) == 0 {
		t.Fatalf("A1 %d, A2 %d, A3 %d notes", len(a.trackNotes(0)), len(a.trackNotes(1)), len(a.trackNotes(2)))
	}
	chooseMenuContaining(t, a, "Keep it")
	keys(t, a, "B")
	keys(t, a, "Y+{") // one undo: back before the dispatch
	if len(a.trackNotes(0)) == 0 {
		t.Fatal("undo gives the track back")
	}
	// Select+Start-free check that the arp over the columns is no longer in the menu
	a.menu = nil
	a.seqMenu()
	for _, it := range a.menu.items {
		if strings.Contains(it.label, "Arp over") || strings.Contains(it.label, "column") {
			t.Fatalf("a leftover in the pattern menu: %q", it.label)
		}
	}
}

func TestNoteInfoSaysHowFarOffTheGrid(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.step = 4
	if a.noteInfo() != "" {
		t.Fatal("no note, no info")
	}
	a.Press(A, 0)
	a.Release(A)
	info := a.noteInfo()
	for _, want := range []string{"A1 1.5", "vel 100", "len 1.00", "on the grid", "plain"} {
		if !strings.Contains(info, want) {
			t.Fatalf("%q is missing from %q", want, info)
		}
	}
	// nudged 5 ticks later: 5 ticks at 120 BPM is 5.2 ms
	a.Press(A, 0)
	a.Press(R1, 0)
	a.Release(A)
	info = a.noteInfo()
	if !strings.Contains(info, "5t late (5.2ms)") || !strings.Contains(info, "beat+5t") {
		t.Fatalf("late: %q", info)
	}
	// nudged to nearly the next step it is early for that one
	n, _ := noteOn(a, 0)
	if got := a.noteTiming(lab.Note{Tick: n.Tick - 5 + 110}, 0); !strings.Contains(got, "10 ticks early") {
		t.Fatalf("early: %q", got)
	}
	// the footer shows it when there is no message, and a message wins
	a.msg = ""
	a.Render()
	if img := a.Render(); img.Bounds().Dx() != W {
		t.Fatal("frame")
	}
	// a chromatic note is told apart: a note with a pitch byte
	if got := fmt.Sprint(lab.Note{Pitch: 3}.Pitch); got != "3" {
		t.Fatal(got)
	}
}

func TestPadPanelFadersAndSwitches(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Pads)
	a.cur = 0
	keys(t, a, "A")
	if a.pp == nil || a.pp.page != 0 || padPages[0][0].label != "Level" {
		t.Fatal("A opens SOUND, the level first")
	}
	get := func(id byte) int32 { v, _ := a.lab.PadParam(0, id); return v }
	if get(0x69) != 127 {
		t.Fatalf("level %d", get(0x69))
	}
	for i := 0; i < 20; i++ {
		keys(t, a, "L")
	}
	if get(0x69) != 107 {
		t.Fatalf("Left lowers the level: %d", get(0x69))
	}
	// a key held runs faster: repeats 6 and up are 4 at a time
	a.Press(Left, 0)
	a.Press(Left, 4) // held half a second: steps of 4
	a.Press(Left, 9) // held more than a second: steps of 10
	if get(0x69) != 107-1-4-10 {
		t.Fatalf("held: %d", get(0x69))
	}
	keys(t, a, "A") // back to the default
	if get(0x69) != 127 {
		t.Fatalf("A puts a fader to its default: %d", get(0x69))
	}
	keys(t, a, "D D") // pitch
	keys(t, a, "R R R")
	if get(0x73) != 3 {
		t.Fatalf("pitch %d", get(0x73))
	}
	for i := 0; i < 40; i++ {
		keys(t, a, "R")
	}
	if get(0x73) != 12 {
		t.Fatalf("held to the end of its range: %d", get(0x73))
	}
	keys(t, a, "D D D D D D") // wraps round to the first row again
	if a.pp.row[0] != (2+6)%len(padPages[0]) {
		t.Fatalf("row %d", a.pp.row[0])
	}
	// the second page: switches, flags and choices
	keys(t, a, "]")
	if a.pp.page != 1 {
		t.Fatal("R1 turns to SHAPE")
	}
	keys(t, a, "D D D") // play mode
	keys(t, a, "R")
	if get(0x75) != 1 {
		t.Fatalf("play mode reverse: %d", get(0x75))
	}
	keys(t, a, "D D") // loop (gate then loop)
	keys(t, a, "A")
	if get(0x6B) != 1 {
		t.Fatal("A switches the loop on")
	}
	keys(t, a, "D D") // fixed velocity is the bit 0 of the mode flags, one shot the bit 5
	keys(t, a, "A")
	keys(t, a, "U")
	keys(t, a, "A")
	flags := get(0x70)
	if flags&(1<<5) == 0 || flags&1 == 0 {
		t.Fatalf("one shot and fixed velocity are bits of 0x70: %b", flags)
	}
	if get(0x75) != 1 {
		t.Fatal("other settings are untouched")
	}
	// X plays the pad as set: quieter at a low level, turned away on one side with the pan
	au.Played = 0
	keys(t, a, "X")
	if au.Played != 1 {
		t.Fatal("X plays the pad")
	}
	a.lab.SetPadParam(0, 0x69, 40)
	a.lab.SetPadParam(0, 0x78, 13) // all the way left: the right side is silent
	a.lab.SetPadParam(0, 0x75, 0)
	v, _ := a.padVoice(0)
	var l, r int
	for i := 0; i+1 < len(v.Samples); i += 2 {
		l = max(l, abs(int(v.Samples[i])))
		r = max(r, abs(int(v.Samples[i+1])))
	}
	if l == 0 || r != 0 {
		t.Fatalf("the pan sends it left: %d %d", l, r)
	}
	full, _ := a.noteSample(0, 127, 0, 0)
	var fl int
	for _, x := range full.Samples {
		fl = max(fl, abs(int(x)))
	}
	if l >= fl {
		t.Fatalf("level 40 is quieter than full: %d vs %d", l, fl)
	}
	// the pitch changes its length, the direction turns it round
	a.lab.SetPadParam(0, 0x78, 64)
	a.lab.SetPadParam(0, 0x69, 127)
	a.lab.SetPadParam(0, 0x73, 0) // (the panel test above left the pitch at +12)
	norm, _ := a.padVoice(0)
	a.lab.SetPadParam(0, 0x73, 12)
	up, _ := a.padVoice(0)
	if up.Frames() >= norm.Frames() {
		t.Fatalf("an octave up is half as long: %d vs %d", up.Frames(), norm.Frames())
	}
	// every change is one undo step, and a held key is one
	keys(t, a, "B")
	if a.pp != nil {
		t.Fatal("B closes")
	}
	a.Render()
}

func TestPadPanelTheToolsPageAndEmptyPads(t *testing.T) {
	a, _ := newApp(t)
	a.go_(Pads)
	a.cur = 5 // an empty pad
	keys(t, a, "A")
	if a.pp == nil || a.pp.page != 2 {
		t.Fatalf("an empty pad has no settings: it opens on the tools")
	}
	a.Render()
	chooseMenu(t, a, "Pick a sample")
	if a.scr != Files || a.pp != nil {
		t.Fatalf("Pick a sample goes to the files: %v", a.scr)
	}
	a.go_(Pads)
	a.cur = 0
	keys(t, a, kit)
	a.go_(Pads)
	a.cur = 0
	keys(t, a, "A [")
	if a.pp == nil || a.pp.page != 2 {
		t.Fatalf("L1 round from SOUND is the tools page: %d", a.pp.page)
	}
	keys(t, a, "] ] ]")
	if a.pp.page != 2 {
		t.Fatalf("three pages: %d", a.pp.page)
	}
	a.Render()
	keys(t, a, "B")
	if a.pp != nil {
		t.Fatal("B closes the tools page")
	}
	// the setting of a pad is in the undo stack
	a.cur = 1
	keys(t, a, "A")
	before := len(a.undo)
	keys(t, a, "L L L")
	keys(t, a, "B")
	if v, _ := a.lab.PadParam(1, 0x69); v != 124 {
		t.Fatalf("level %d", v)
	}
	keys(t, a, "Y+{")
	if v, _ := a.lab.PadParam(1, 0x69); v == 124 {
		t.Fatal("undo takes the fader change back")
	}
	_ = before
}

func TestAHeldKeyStepsBiggerTheLongerItIsHeld(t *testing.T) {
	if accel(0) != 1 || accel(2) != 1 || accel(3) != 4 || accel(7) != 4 || accel(8) != 10 || accel(30) != 10 {
		t.Fatalf("accel: %d %d %d %d %d", accel(0), accel(3), accel(7), accel(8), accel(30))
	}
	// only the direction keys and the shoulders go on when held; a repeat of A does nothing
	if Holds(A) || Holds(B) || !Holds(Up) || !Holds(R1) {
		t.Fatal("holds")
	}
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.step = 2
	a.Press(A, 0)
	a.Press(A, 3) // the pad repeating the A press: nothing
	a.Press(A, 5)
	a.Release(A)
	if _, ok := noteOn(a, 0); !ok {
		t.Fatal("a held A is one note, not a note taken off again")
	}
	// the note edit takes the big steps
	a.Press(A, 0)
	a.Press(Up, 0)
	a.Press(Up, 4)
	a.Press(Up, 9)
	a.Release(A)
	if n, _ := noteOn(a, 0); n.Vel != 100+1+4+10 {
		t.Fatalf("velocity %d", n.Vel)
	}
}

func TestBulkEditOfMarkedPads(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit) // A1..A3
	a.go_(Pads)
	a.lab.SetPadParam(0, 0x69, 100)
	a.lab.SetPadParam(1, 0x69, 80)
	a.lab.SetPadParam(2, 0x69, 127)
	a.refresh()
	level := func(p int) int32 { v, _ := a.lab.PadParam(p, 0x69); return v }
	// mark the bank's pads with samples
	a.cur = 0
	a.markBank()
	if len(a.marks) != 3 {
		t.Fatalf("3 pads marked: %v", a.marks)
	}
	keys(t, a, "A")
	if a.pp == nil || len(a.padTargets()) != 3 {
		t.Fatal("the panel edits the 3 marked pads")
	}
	a.Render() // "3 MARKED PADS", the level shows a * (they differ)
	keys(t, a, "L L L L L")
	if level(0) != 95 || level(1) != 75 || level(2) != 122 {
		t.Fatalf("each pad moves by the same amount, keeping the differences: %d %d %d", level(0), level(1), level(2))
	}
	keys(t, a, "A") // the default, on all
	if level(0) != 127 || level(1) != 127 || level(2) != 127 {
		t.Fatalf("A puts every pad to the default: %d %d %d", level(0), level(1), level(2))
	}
	// a switch is set on all of them (SHAPE: gate, loop are the 5th and 6th rows)
	keys(t, a, "]")
	keys(t, a, "D D D D D A")
	for p := 0; p < 3; p++ {
		if v, _ := a.lab.PadParam(p, 0x6B); v != 1 {
			t.Fatalf("pad %d: loop is on", p)
		}
	}
	// the tools act on all the marked pads
	keys(t, a, "[ [") // round to the tools page
	if a.pp.page != 2 {
		t.Fatalf("page %d", a.pp.page)
	}
	m := menuOf(a)
	found := false
	for _, it := range m.items {
		found = found || strings.Contains(it.label, "Reverse (3 marked pads)")
	}
	if !found {
		t.Fatal("the tools say they act on 3 pads")
	}
	before := make([]int16, 3)
	for p := 0; p < 3; p++ {
		id, _ := a.padSample(p)
		s, _ := a.lab.Sample(id)
		before[p] = s.Samples[len(s.Samples)-1]
	}
	chooseMenuContaining(t, a, "Reverse (3 marked pads)")
	if !strings.Contains(a.msg, "reverse on 3 pads") {
		t.Fatalf("message %q", a.msg)
	}
	for p := 0; p < 3; p++ {
		id, _ := a.padSample(p)
		s, _ := a.lab.Sample(id)
		if s.Samples[0] != before[p] {
			t.Fatalf("pad %d is reversed: first sample %d, last was %d", p, s.Samples[0], before[p])
		}
	}
	// one undo takes the whole bulk change back
	keys(t, a, "Y+{")
	id, _ := a.padSample(0)
	s0, _ := a.lab.Sample(id)
	if s0.Samples[len(s0.Samples)-1] != before[0] {
		t.Fatal("undo gave the first pad's direction back")
	}
	// clear them all
	a.go_(Pads)
	a.cur = 0
	keys(t, a, "A [")
	chooseMenuContaining(t, a, "Clear (3 marked pads)")
	if padsWithSamples(a) != 0 {
		t.Fatalf("all 3 are cleared: %d left", padsWithSamples(a))
	}
}

func TestFillGateDecayAndShape(t *testing.T) {
	// every beat, gate 50%, decay 20%: the velocity falls on the hits of a bar and starts again in the next bar
	ns := fillNotes(0, 2*1920, []int{0, 4, 8, 12}, 100, 50, 20)
	if len(ns) != 8 {
		t.Fatalf("4 hits in 2 bars: %d", len(ns))
	}
	want := []int{100, 80, 64, 51, 100, 80, 64, 51}
	for i, n := range ns {
		if n.Vel != want[i] || n.Len != 60 {
			t.Fatalf("note %d: vel %d len %d, want vel %d len 60", i, n.Vel, n.Len, want[i])
		}
	}
	if ln := fillNotes(0, 1920, []int{0}, 100, 200, 0)[0].Len; ln != 240 {
		t.Fatalf("gate 200%% ties over a second step: %d", ln)
	}
	if len(fillPresets) < 15 {
		t.Fatalf("only %d rhythms", len(fillPresets))
	}
	for _, f := range fillPresets {
		for _, s := range f.mask {
			if s < 0 || s > 15 {
				t.Fatalf("%s has a step %d outside the bar", f.name, s)
			}
		}
	}
	// gate on notes that are there: a part of a step, or up to the next note
	base := []lab.Note{{Tick: 0, Vel: 100, Len: 120}, {Tick: 480, Vel: 100, Len: 120}, {Tick: 1200, Vel: 100, Len: 120}}
	g := gateNotes(base, 1920, 0)
	if g[0].Len != 480 || g[1].Len != 720 || g[2].Len != 720 {
		t.Fatalf("legato: %d %d %d", g[0].Len, g[1].Len, g[2].Len)
	}
	if g := gateNotes(base, 1920, 25); g[1].Len != 30 {
		t.Fatalf("25%% of a step: %d", g[1].Len)
	}
	// decay counts from the first hit of each bar, whatever the order they were given in
	d := decayNotes([]lab.Note{{Tick: 2040, Vel: 100}, {Tick: 0, Vel: 100}, {Tick: 240, Vel: 100}, {Tick: 1920, Vel: 100}}, 50)
	got := map[uint32]int{}
	for _, n := range d {
		got[n.Tick] = n.Vel
	}
	if got[0] != 100 || got[240] != 50 || got[1920] != 100 || got[2040] != 50 {
		t.Fatalf("decay: %v", got)
	}
	// euclid takes the decay as well
	e := decayNotes(euclidNotes(0, 1920, 120, 16, 4, 0, 50, 100), 20)
	if e[0].Vel != 100 || e[1].Vel != 80 || e[3].Vel != 51 {
		t.Fatalf("euclid with decay: %d %d %d", e[0].Vel, e[1].Vel, e[3].Vel)
	}
	// in the pattern: fill, then shape the notes that are there
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.seqMenu()
	a.trackDefaults()
	a.tt.fillPat, a.tt.fillDecay = 3, 3 // off-beats, 15% decay
	chooseMenu(t, a, "Fill...")
	notes := a.trackNotes(0)
	if len(notes) != 16 || notes[0].Tick != 240 || notes[1].Vel != 85 {
		t.Fatalf("off-beats over 4 bars with decay: %d notes, first %+v, second vel %d", len(notes), notes[0], notes[1].Vel)
	}
	chooseMenuContaining(t, a, "Keep it")
	keys(t, a, "B")
	a.seqMenu()
	a.tt.shapeGate, a.tt.shapeDecay = 0, 0 // to the next note
	chooseMenu(t, a, "Shape")
	notes = a.trackNotes(0)
	if notes[0].Len != 480 {
		t.Fatalf("legato to the next note: %d", notes[0].Len)
	}
}

func TestRatchetUsesTheUnitsSubsteps(t *testing.T) {
	base := []lab.Note{{Tick: 240, Pad: 3, Vel: 100, Len: 120}, {Tick: 480, Pad: 3, Vel: 90, Len: 120}}
	// 3 parts: three notes on the parts of the step, each a third of a step long
	r := ratchetNotes(base, 0, 3, 0, 0, 1)
	if len(r) != 6 {
		t.Fatalf("2 notes x 3 parts: %d", len(r))
	}
	want := []uint32{240, 280, 320, 480, 520, 560}
	for i, n := range r {
		if n.Tick != want[i] || n.Len != 40 || n.Pad != 3 {
			t.Fatalf("part %d: %+v, want tick %d len 40", i, n, want[i])
		}
	}
	// 2 and 4 parts divide the step evenly too
	if r := ratchetNotes(base[:1], 0, 2, 0, 0, 1); r[1].Tick != 300 || r[1].Len != 60 {
		t.Fatalf("2 parts: %+v", r)
	}
	if r := ratchetNotes(base[:1], 0, 4, 0, 0, 1); len(r) != 4 || r[3].Tick != 330 || r[3].Len != 30 {
		t.Fatalf("4 parts: %+v", r)
	}
	// which notes: every 2nd leaves the others alone
	if r := ratchetNotes(base, 1, 2, 0, 0, 1); len(r) != 3 || r[2].Len != 120 {
		t.Fatalf("every 2nd note: %+v", r)
	}
	// parts that play, and the velocity ramp
	if r := ratchetNotes(base[:1], 0, 3, 1, 0, 1); len(r) != 2 || r[0].Tick != 280 {
		t.Fatalf("skip the first: %+v", r)
	}
	if r := ratchetNotes(base[:1], 0, 4, 3, 0, 1); len(r) != 2 || r[1].Tick != 300 {
		t.Fatalf("every other part: %+v", r)
	}
	up := ratchetNotes(base[:1], 0, 3, 0, 1, 1)
	if !(up[0].Vel < up[1].Vel && up[1].Vel < up[2].Vel && up[2].Vel == 100) {
		t.Fatalf("louder: %+v", up)
	}
	if ac := ratchetNotes(base[:1], 0, 3, 0, 3, 1); ac[0].Vel != 100 || ac[1].Vel != 60 {
		t.Fatalf("accent the first: %+v", ac)
	}
	// the last note of each bar
	two := []lab.Note{{Tick: 0, Vel: 100, Len: 120}, {Tick: 1800, Vel: 100, Len: 120}, {Tick: 1920, Vel: 100, Len: 120}, {Tick: 3720, Vel: 100, Len: 120}}
	if r := ratchetNotes(two, 4, 2, 0, 0, 1); len(r) != 2+2*2 {
		t.Fatalf("2 of 4 notes split: %d", len(r))
	}
	// what the unit's substeps look like to 404tui: a step of n parts has notes a part long, on the parts
	for _, parts := range ratchetParts {
		if stepTicks%parts != 0 {
			t.Fatalf("%d parts do not divide a step", parts)
		}
	}
}

func TestEvolveMutatesEachBarFromTheOneBefore(t *testing.T) {
	var first []lab.Note
	for _, s := range []int{0, 4, 8, 12} { // a kick on the beats in bar 1
		first = append(first, lab.Note{Tick: uint32(s * stepTicks), Pad: 0, Vel: 100, Len: stepTicks})
	}
	out := evolveNotes(first, 4*1920, 40, 0, 0, 5)
	byBar := map[int][]lab.Note{}
	for _, n := range out {
		byBar[int(n.Tick)/1920] = append(byBar[int(n.Tick)/1920], n)
	}
	if len(byBar) != 4 {
		t.Fatalf("notes in %d bars", len(byBar))
	}
	if len(byBar[0]) != 4 || byBar[0][1].Tick != 480 {
		t.Fatalf("bar 1 is the seed, untouched: %+v", byBar[0])
	}
	same := true
	sig := func(ns []lab.Note) string {
		s := ""
		for _, n := range ns {
			s += fmt.Sprintf("%d/%d ", int(n.Tick)%1920, n.Vel)
		}
		return s
	}
	for b := 1; b < 4; b++ {
		same = same && sig(byBar[b]) == sig(byBar[0])
	}
	if same {
		t.Fatal("40% change did not change any bar")
	}
	for _, n := range out {
		if int(n.Tick) >= 4*1920 || n.Pad != 0 || n.Vel < 1 || n.Vel > 127 {
			t.Fatalf("a note out of its bounds: %+v", n)
		}
	}
	// the same seed gives the same result, another seed another
	if sig(evolveNotes(first, 4*1920, 40, 0, 0, 5)) != sig(out) {
		t.Fatal("the same seed must give the same bars")
	}
	if sig(evolveNotes(first, 4*1920, 40, 0, 0, 6)) == sig(out) {
		t.Fatal("another seed should give other bars")
	}
	// velocities only: the rhythm stays, the velocities move
	vo := evolveNotes(first, 2*1920, 100, 4, 0, 9)
	for _, n := range vo {
		if int(n.Tick)%480 != 0 {
			t.Fatalf("velocities only keeps the steps: %+v", n)
		}
	}
	if len(vo) != 8 {
		t.Fatalf("and the notes: %d", len(vo))
	}
	// drops only: never more notes; adds only: never fewer
	if got := len(evolveNotes(first, 4*1920, 60, 3, 0, 11)); got > 16 {
		t.Fatalf("drops only: %d", got)
	}
	if got := len(evolveNotes(first, 4*1920, 60, 2, 0, 11)); got < 16 {
		t.Fatalf("adds only: %d", got)
	}
	// back to the first bar every 2 bars: bar 3 grows from bar 1 again
	rs := evolveNotes(first, 4*1920, 0, 0, 1, 3) // no change at all: all bars equal the first
	if len(rs) != 16 {
		t.Fatalf("no change: %d notes", len(rs))
	}
	// in the pattern, through the menu: a one-bar pattern says so
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.setTrack(0, first)
	a.seqMenu()
	chooseMenu(t, a, "Evolve")
	if len(a.trackNotes(0)) < 8 { // the pattern is 4 bars: the first bar was copied into the later ones with mutations
		t.Fatalf("evolved: %d notes", len(a.trackNotes(0)))
	}
}

func TestAccentsAndGhostsForHats(t *testing.T) {
	var hats []lab.Note
	for s := 0; s < 16; s += 2 { // eighths
		hats = append(hats, lab.Note{Tick: uint32(s * stepTicks), Pad: 2, Vel: 90, Len: 60})
	}
	out := accentGhostNotes(hats, 2, 1920, 1, 120, 70, 0, 30, 100, 1) // the beats accented, the rest at 70
	for _, n := range out {
		want := 70
		if int(n.Tick)%480 == 0 {
			want = 120
		}
		if n.Vel != want {
			t.Fatalf("tick %d: vel %d, want %d", n.Tick, n.Vel, want)
		}
	}
	if len(out) != 8 {
		t.Fatalf("no ghosts, no new notes: %d", len(out))
	}
	// off-beat accents (the ands)
	oa := accentGhostNotes(hats, 2, 1920, 2, 127, 80, 0, 30, 100, 1)
	if oa[1].Vel != 127 || oa[0].Vel != 80 { // tick 240 is an and
		t.Fatalf("off-beats: %d %d", oa[0].Vel, oa[1].Vel)
	}
	// ghosts: soft notes only on empty steps, none where there is a hit
	g := accentGhostNotes(hats, 2, 1920, 0, 0, 0, 4, 30, 100, 1) // every empty step
	if len(g) != 16 {
		t.Fatalf("eighths plus a ghost on each empty 16th: %d", len(g))
	}
	ghosts := 0
	steps := map[int]int{}
	for _, n := range g {
		steps[int(n.Tick)/stepTicks]++
		if n.Vel == 30 {
			ghosts++
			if int(n.Tick)/stepTicks%2 == 0 {
				t.Fatalf("a ghost on a hit: %+v", n)
			}
		}
	}
	for s, c := range steps {
		if c != 1 {
			t.Fatalf("two notes on step %d", s)
		}
	}
	if ghosts != 8 {
		t.Fatalf("8 ghosts: %d", ghosts)
	}
	// before each hit: the step before; the chance thins them
	if b := accentGhostNotes(hats, 2, 1920, 0, 0, 0, 1, 30, 100, 1); len(b) != 8+7 { // the first hit has no step before it
		t.Fatalf("before each hit: %d", len(b))
	}
	half := 0
	for seed := int64(1); seed <= 20; seed++ {
		half += len(accentGhostNotes(hats, 2, 1920, 0, 0, 0, 2, 30, 50, seed)) - 8
	}
	if half < 20*8/4 || half > 20*8*3/4 {
		t.Fatalf("a 50%% chance on 8 candidates, 20 runs: %d ghosts", half)
	}
	// every 3rd note, first of each bar
	if e := accentGhostNotes(hats, 2, 1920, 4, 127, 0, 0, 30, 100, 1); e[0].Vel != 127 || e[1].Vel != 90 || e[3].Vel != 127 {
		t.Fatalf("every 3rd: %d %d %d", e[0].Vel, e[1].Vel, e[3].Vel)
	}
	if f := accentGhostNotes(hats, 2, 3840, 6, 127, 0, 0, 30, 100, 1); f[0].Vel != 127 || f[1].Vel != 90 {
		t.Fatalf("first of the bar: %d %d", f[0].Vel, f[1].Vel)
	}
}

func TestNudgeAndTheSlicerCursor(t *testing.T) {
	ns := nudgeNotes([]lab.Note{{Tick: 480}, {Tick: 3}}, 1920, -10)
	if ns[0].Tick != 470 || ns[1].Tick != 0 {
		t.Fatalf("nudge: %d %d", ns[0].Tick, ns[1].Tick)
	}
	if nudgeNotes([]lab.Note{{Tick: 1900}}, 1920, 40)[0].Tick != 1919 {
		t.Fatal("stays inside the pattern")
	}
	if cursorPixels(0) != 2 || cursorPixels(3) != 8 || cursorPixels(6) != 32 || cursorPixels(10) != 128 {
		t.Fatalf("tiers: %d %d %d %d", cursorPixels(0), cursorPixels(3), cursorPixels(6), cursorPixels(10))
	}
	a, _ := newApp(t)
	keys(t, a, "A A D D R A") // a loop on A1
	a.go_(Slice)
	if a.sl == nil {
		t.Skip("no slicer")
	}
	a.at = 0
	from, to := a.view()
	col := max((to-from)/waveCols, 1)
	a.Press(Right, 0)
	one := a.at
	a.Press(Right, 12) // held for two seconds
	if far := a.at - one; far != col*128 {
		t.Fatalf("a long hold moves 128 pixels at a press: %d frames, want %d", far, col*128)
	}
	if one != col*2 {
		t.Fatalf("the first press is 2 pixels: %d", one)
	}
	a.at = 0
	keys(t, a, "]") // R1 jumps a quarter of the view
	if a.at != max((to-from)/4, 1) {
		t.Fatalf("R1 jumps a quarter of the view: %d", a.at)
	}
	keys(t, a, "[")
	if a.at != 0 {
		t.Fatalf("and L1 back: %d", a.at)
	}
}

func TestFileIndexSearchesFromDisk(t *testing.T) {
	a, _ := newApp(t)
	a.dir = a.Root
	a.startIndex(a.Root)
	for i := 0; i < 200; i++ {
		a.idx.mu.Lock()
		running := a.idx.running
		a.idx.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.indexTick()
	p, root := a.indexFor(a.Root)
	if p == "" || root != filepath.Clean(a.Root) {
		t.Fatalf("index not found: %q %q", p, root)
	}
	// delete the live files: the answer must come from the index alone
	var names []string
	a.searchQ = ""
	a.list()
	for _, e := range a.entries {
		names = append(names, e.name)
	}
	os.RemoveAll(a.Root)
	os.MkdirAll(a.Root, 0o755)
	a.searchQ = "wav"
	a.list()
	if len(a.entries) == 0 && len(names) > 0 {
		t.Fatal("the indexed search finds nothing")
	}
	for _, e := range a.entries {
		if !strings.Contains(strings.ToLower(e.name), "wav") {
			t.Fatalf("%q does not match", e.name)
		}
	}
}

func TestFreezeLoopsOnlyTheBarInFocus(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R") // a kit on A1..A3, then SEQ
	keys(t, a, "A")        // a note on step 1 of bar 1
	a.step = 16 + 4
	keys(t, a, "A") // and one in bar 2
	full, ok := a.render()
	if !ok {
		t.Fatal("nothing rendered")
	}
	a.step = 16
	keys(t, a, "Y+A")
	if !a.freeze {
		t.Fatal("Y+A freezes the bar")
	}
	one, ok := a.render()
	if !ok || one.Frames()*2 > full.Frames()+2 || one.Frames() == 0 {
		t.Fatalf("a frozen bar is half of 2 bars: %d of %d frames", one.Frames(), full.Frames())
	}
	if p := a.playStep(); p != -1 {
		t.Fatalf("not playing: %d", p)
	}
	keys(t, a, "Y+A")
	if a.freeze {
		t.Fatal("Y+A again releases")
	}
}

func TestMuteSoloAndNewPatternFromAudible(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R")
	for p := 0; p < 3; p++ { // a note on each of A1..A3
		a.toggle(p, 0)
	}
	a.autoRows()
	a.row = len(a.rows) - 1
	d, _ := a.doc()
	if len(d.Notes) != 3 {
		t.Fatalf("three tracks of one note: %d", len(d.Notes))
	}
	full, _ := a.render()
	keys(t, a, "}") // this row muted
	if a.trackMode(a.rowPad()) != trackMute || a.audible(a.rowPad()) {
		t.Fatal("R2 mutes the row")
	}
	muted, _ := a.render()
	if len(muted.Samples) != len(full.Samples) {
		t.Fatal("the loop keeps its length")
	}
	keys(t, a, "}") // solo
	if !a.audible(a.rowPad()) || a.audible(a.rows[0]) {
		t.Fatal("solo plays only that row")
	}
	keys(t, a, "}")
	keys(t, a, "}") // muted again
	from := a.pat
	a.newFromAudible()
	if a.pat == from {
		t.Fatal("the new pattern is another one")
	}
	nd, _ := a.doc()
	if len(nd.Notes) != 2 {
		t.Fatalf("the muted track is left out: %d notes", len(nd.Notes))
	}
	a.pat = from
	if od, _ := a.doc(); len(od.Notes) != 3 {
		t.Fatal("the original keeps its notes")
	}
}

func TestBounceWhatYouHear(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R")
	for p := 0; p < 2; p++ {
		a.toggle(p, 0)
	}
	a.autoRows()
	a.row = 0
	keys(t, a, "}") // the first row muted
	before := a.info.Pads[5].File
	a.bounce(a.audible, "mix")
	keys(t, a, "A")
	if before {
		t.Skip("pad 6 used")
	}
	found := false
	for i := 3; i < 160; i++ {
		if a.info.Pads[i].File && strings.HasPrefix(a.info.Pads[i].Name, "bnc_") {
			found = true
		}
	}
	if !found {
		t.Fatal("the bounce is on a pad")
	}
}

func TestSliceEqualPartsAndAutoLimit(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R Y+R Y+R") // a kit, then SLICE
	if a.Screen() != Slice {
		t.Skipf("screen %v", a.Screen())
	}
	a.equalParts(8)
	if len(a.sl.Slices) != 8 {
		t.Fatalf("8 equal parts: %d", len(a.sl.Slices))
	}
	a.autoMax = 4
	a.equalParts(1)
	a.autoSlice()
	t.Log(a.msg)
	if len(a.sl.Slices) > 4 {
		t.Fatalf("auto-cut up to 4: %d", len(a.sl.Slices))
	}
}

func TestSliceShuffleChangesTheOrderShown(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R Y+R Y+R")
	if a.Screen() != Slice {
		t.Skipf("screen %v", a.Screen())
	}
	a.equalParts(8)
	before := append([]int(nil), a.sl.Order...)
	a.shuffleOrder()
	if fmt.Sprint(before) == fmt.Sprint(a.sl.Order) {
		t.Fatalf("shuffle left the order as it was: %v", before)
	}
	if len(a.sl.Order) != 8 {
		t.Fatalf("order %v", a.sl.Order)
	}
	a.randomParts(6)
	if n := len(a.sl.Slices); n < 2 || n > 8 {
		t.Fatalf("random parts: %d", n)
	}
	a.gridParts(0.25)
	if len(a.sl.Slices) < 2 {
		t.Fatalf("grid: %d slices, %s", len(a.sl.Slices), a.msg)
	}
	a.sl.Order = []int{len(a.sl.Slices) - 1, 0}
	a.sl.ResetOrder()
	if len(a.sl.Order) != len(a.sl.Slices) || a.sl.Order[0] != 0 {
		t.Fatalf("order reset %v", a.sl.Order)
	}
	a.Render() // draws the order row without a panic
}

func TestFrozenBarScopesTheTools(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R")
	keys(t, a, "A") // a note on A1, bar 1
	pad := a.rowPad()
	var all []lab.Note
	for s := 0; s < 64; s += 4 { // every beat of the 4 bars
		all = append(all, lab.Note{Tick: uint32(s * stepTicks), Pad: pad, Vel: 100, Len: stepTicks})
	}
	a.step = 16 + 2
	a.freeze = true
	if err := a.setTrack(pad, all); err != nil {
		t.Fatal(err)
	}
	notes := a.trackNotes(pad)
	inBar2, outside := 0, 0
	for _, n := range notes {
		if n.Tick >= 16*stepTicks && n.Tick < 32*stepTicks {
			inBar2++
		} else {
			outside++
		}
	}
	if inBar2 != 4 || outside != 1 {
		t.Fatalf("bar 2 got the beats (%d), the other bars stay as they were (%d)", inBar2, outside)
	}
}

func TestMangleDiceAreFixedByTheRoll(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R Y+R Y+R")
	if a.Screen() != Slice {
		t.Skip()
	}
	a.equalParts(8)
	base := a.sl.Clone()
	m := mangleSet{drop: 2, rev: 3, pitchN: 2, pitchRange: 5, roll: 1, seed: 7}
	x := mangleSlicing(base, m)
	y := mangleSlicing(base, m)
	drops, revs := 0, 0
	for i := range x.Slices {
		if x.Slices[i] != y.Slices[i] {
			t.Fatal("same seed, same slices")
		}
		if x.Slices[i].Chance == 0 {
			drops++
		}
		if x.Slices[i].Reverse {
			revs++
		}
	}
	if drops != 2 || revs != 3 {
		t.Fatalf("drops %d reversed %d", drops, revs)
	}
	a.mangleMenu()
	if a.menu == nil {
		t.Fatal("the dialog is open")
	}
	keys(t, a, "B")
	if len(a.sl.Slices) != 8 || a.sl.Slices[0].Chance != base.Slices[0].Chance {
		t.Fatal("B puts the slices back")
	}
}

func TestLFOModulatesTheTrack(t *testing.T) {
	var notes []lab.Note
	for s := 0; s < 16; s++ {
		notes = append(notes, lab.Note{Tick: uint32(s * stepTicks), Pad: 0, Vel: 64, Len: stepTicks})
	}
	shape := func(name string) int {
		for i, n := range lfoShapes {
			if n == name {
				return i
			}
		}
		t.Fatal(name)
		return 0
	}
	st := lfoState{shape: shape("SIN"), rate: 3, depth: 5} // a sine over 1/1 (16 steps), 100%
	out := lfoNotes(notes, st, 1)
	if out[0].Vel != 64 || out[4].Vel != 127 || out[12].Vel != 1 {
		t.Fatalf("a sine: middle, top at a quarter, bottom at three quarters: %d %d %d", out[0].Vel, out[4].Vel, out[12].Vel)
	}
	st.shape = shape("SAW")
	if o := lfoNotes(notes, st, 1); o[0].Vel >= o[8].Vel || o[8].Vel >= o[15].Vel {
		t.Fatal("a saw rises")
	}
	st.shape = shape("SAW DN")
	if o := lfoNotes(notes, st, 1); o[0].Vel <= o[15].Vel {
		t.Fatal("a falling saw falls")
	}
	st.shape = shape("PUMP")
	if o := lfoNotes(notes, st, 1); o[0].Vel != 1 || o[15].Vel < 60 {
		t.Fatalf("PUMP dips on the beat and swells back: %d ... %d", o[0].Vel, o[15].Vel)
	}
	st.shape, st.shift = shape("SQR"), 2 // half a round later: starts low
	if o := lfoNotes(notes, st, 1); o[0].Vel >= 64 || o[8].Vel <= 64 {
		t.Fatal("shift moves the wave")
	}
	st = lfoState{shape: shape("TRI"), rate: 3, depth: 5, ranged: true, lo: 2, hi: 6} // between 32 and 96
	for _, n := range lfoNotes(notes, st, 1) {
		if n.Vel < 32 || n.Vel > 96 {
			t.Fatalf("the range holds the velocity between 32 and 96: %d", n.Vel)
		}
	}
	st = lfoState{shape: shape("SAW"), rate: 3, depth: 5, target: 1}
	if o := lfoNotes(notes, st, 1); o[0].Len >= o[15].Len {
		t.Fatal("length follows the saw")
	}
	st.target, st.depth = 2, 3
	if o := lfoNotes(notes, st, 1); o[15].Tick <= notes[15].Tick {
		t.Fatal("timing moves late at the top of the saw")
	}
	for _, n := range []string{"RND", "S&H"} {
		st = lfoState{shape: shape(n), rate: 5, depth: 3}
		a, b := lfoNotes(notes, st, 1), lfoNotes(notes, st, 1)
		c := lfoNotes(notes, st, 2)
		same := true
		for i := range a {
			if a[i].Vel != b[i].Vel {
				t.Fatalf("%s: the same seed rolls the same", n)
			}
			if a[i].Vel != c[i].Vel {
				same = false
			}
		}
		if same {
			t.Fatalf("%s: another seed, another roll", n)
		}
	}
}

func TestEnvelopeAndLFOExtras(t *testing.T) {
	// two notes close together, then a long gap: the second follows the first closely, the first follows the second late
	notes := []lab.Note{
		{Tick: 0, Pad: 0, Vel: 100, Len: stepTicks},
		{Tick: 2 * stepTicks, Pad: 0, Vel: 100, Len: stepTicks},
	}
	st := envState{decay: 4, sustain: 0, release: 4, depth: 3, invert: true} // ducks
	out := envNotes(notes, 16*stepTicks, st)
	if out[1].Vel >= out[0].Vel {
		t.Fatalf("the note just after another is ducked more than the one after the long gap: %d vs %d", out[1].Vel, out[0].Vel)
	}
	st.invert = false
	up := envNotes(notes, 16*stepTicks, st)
	if up[1].Vel <= notes[1].Vel-1 && up[1].Vel < 127 {
		t.Fatalf("not inverted it goes up: %d", up[1].Vel)
	}
	// LFO: inverted flips the saw, a centre moves it, DRIFT is smooth and seeded
	var many []lab.Note
	for s := 0; s < 16; s++ {
		many = append(many, lab.Note{Tick: uint32(s * stepTicks), Vel: 64, Len: stepTicks})
	}
	idx := func(name string) int {
		for i, n := range lfoShapes {
			if n == name {
				return i
			}
		}
		return 0
	}
	l := lfoState{shape: idx("SAW"), rate: 3, depth: 5, center: 0}
	a := lfoNotes(many, l, 1)
	l.invert = true
	b := lfoNotes(many, l, 1)
	if a[15].Vel <= a[0].Vel || b[15].Vel >= b[0].Vel {
		t.Fatal("inverted turns the saw round")
	}
	l.invert, l.center = false, 32
	if c := lfoNotes(many, l, 1); c[8].Vel != a[8].Vel+32 {
		t.Fatalf("centre +32: %d vs %d", c[8].Vel, a[8].Vel)
	}
	l = lfoState{shape: idx("DRIFT"), rate: 3, depth: 3, center: 0}
	d1, d2 := lfoNotes(many, l, 5), lfoNotes(many, l, 5)
	for i := range d1 {
		if d1[i].Vel != d2[i].Vel {
			t.Fatal("DRIFT: the same seed, the same wander")
		}
	}
}

func TestArpFromOneSample(t *testing.T) {
	// C major triad over two octaves: C E G, then the same an octave up
	o := onePad{scale: 1, stack: 3, oct: 4}
	if got := fmt.Sprint(onePadSemis(o)); got != "[0 4 7 12 16 19]" {
		t.Fatalf("triad in C major: %s", got)
	}
	// the seventh chord on the second degree of C major is D F A C
	o = onePad{scale: 1, stack: 7, degree: 1, oct: 3}
	if got := fmt.Sprint(onePadSemis(o)); got != "[2 5 9 12]" {
		t.Fatalf("Dm7: %s", got)
	}
	// fifths only, on the key of E
	o = onePad{key: 4, scale: 1, stack: 2, oct: 3}
	if got := fmt.Sprint(onePadSemis(o)); got != "[4 11]" {
		t.Fatalf("E fifth: %s", got)
	}
	// three octaves down: C E G at -36 is out of range for the top, the lowest copies are
	o = onePad{scale: 1, stack: 3, oct: 1}
	if got := fmt.Sprint(onePadSemis(o)); got != "[-24 -20 -17 -12 -8 -5 0 4 7]" {
		t.Fatalf("triad -2 octaves: %s", got)
	}
	a, au := newApp(t)
	keys(t, a, kit)
	a.go_(Pads)
	a.cur = 0
	a.openOnePadMenu(0)
	a.Render() // the keyboard is drawn
	au.Loops = 0
	chooseMenu(t, a, "Hear it / stop it")
	if !a.arpOn || au.Loops != 1 {
		t.Fatalf("one sample loops as an arp: on %v loops %d (%s)", a.arpOn, au.Loops, a.msg)
	}
	a.stop()
	before := padsWithSamples(a)
	a.openOnePadMenu(0)
	chooseMenu(t, a, "Bounce the arp to a new pad")
	if padsWithSamples(a) != before+1 {
		t.Fatalf("bounced to one pad: %s", a.msg)
	}
	a.openOnePadMenu(0)
	chooseMenu(t, a, "Bounce the chord to a new pad")
	if padsWithSamples(a) != before+2 {
		t.Fatalf("the chord on a pad of its own: %s", a.msg)
	}
	a.go_(Pads)
	a.openOnePadMenu(0)
	chooseMenu(t, a, "Print the arp to the pattern")
	if d, ok := a.doc(); a.scr != Seq || !ok || len(d.Notes) == 0 {
		t.Fatalf("keys and notes written: %s", a.msg)
	}
}

func TestL2StepsTheGridOnTheSlicer(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Slice)
	if a.sl == nil {
		t.Skip("no slicer")
	}
	a.sl.Division = 0
	keys(t, a, "{")
	if a.sl.Division != formats.Divisions[1] {
		t.Fatalf("L2 takes the next grid: %d", a.sl.Division)
	}
	for i := 0; i < len(formats.Divisions)-1; i++ {
		keys(t, a, "{")
	}
	if a.sl.Division != 0 {
		t.Fatalf("round to off again: %d", a.sl.Division)
	}
}

// seekMenu puts the cursor on the first item that matches, turning the pages of a menu with tabs; false if none does.
func seekMenu(t *testing.T, a *App, match func(label string) bool) bool {
	t.Helper()
	m := menuOf(a)
	pages := 1
	if m.groups != nil {
		pages = len(m.groups)
	}
	for p := 0; p < pages; p++ {
		if m.groups != nil {
			m.setTab(p)
		}
		for i := 0; i < len(m.items); i++ {
			if !m.item().header && match(m.item().label) {
				return true
			}
			keys(t, a, "D")
		}
	}
	return false
}

// menuLabels is the labels of every item of a menu, on every page.
func menuLabels(m *menu) []string {
	items := m.items
	if m.full != nil {
		items = m.full
	}
	var out []string
	for _, it := range items {
		out = append(out, it.label)
	}
	return out
}

func TestMenuTabsTurnWithL1R1(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.seqMenu()
	m := a.menu
	if len(m.tabs) != 4 || m.tab != 0 || m.tabs[0] != "PATTERN" {
		t.Fatalf("the pattern menu has tabs: %v", m.tabs)
	}
	first := m.items[0].label
	keys(t, a, "]")
	if m.tab != 1 || m.items[0].label == first {
		t.Fatalf("R1 shows the next tab: %d", m.tab)
	}
	keys(t, a, "[ [")
	if m.tab != 3 {
		t.Fatalf("L1 turns back round the ends: %d", m.tab)
	}
	a.Render()
	a.go_(Slice)
	a.sliceMenu()
	if len(a.menu.tabs) < 2 || !strings.HasPrefix(a.menu.tabs[0], "SLICE") {
		t.Fatalf("the slicer menu has tabs: %v", a.menu.tabs)
	}
}

func TestSelectChordsOpenTheTools(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R") // a kit, then SEQ
	keys(t, a, "S")
	if a.menu != nil || a.np != nil {
		t.Fatal("Select alone does nothing")
	}
	keys(t, a, "B")
	if a.np == nil || a.menu != nil {
		t.Fatal("B opens the note settings")
	}
	keys(t, a, "B S+B")
	if a.menu == nil || len(a.menu.tabs) != 4 {
		t.Fatalf("Select+B opens the pattern tools with their tabs: %+v", a.menu)
	}
	keys(t, a, "B Y+R Y+R")
	if a.scr != Slice {
		t.Skip("no slicer")
	}
	keys(t, a, "Y+B")
	if a.menu == nil || !strings.HasPrefix(a.menu.title, "slice settings") {
		t.Fatal("Y+B is the slice settings")
	}
	keys(t, a, "B S+B")
	if a.menu == nil || !strings.HasPrefix(a.menu.title, "slice tools") {
		t.Fatal("Select+B is the slice tools")
	}
	keys(t, a, "B")
	// the first tab of the settings is the slice in front
	a.sliceMenu()
	if !strings.HasPrefix(a.menu.tabs[0], "SLICE") || a.menu.items[0].label != "Pitch" {
		t.Fatalf("the slice in front comes first: %v %q", a.menu.tabs, a.menu.items[0].label)
	}
}

func TestNotePanelFaders(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit+" Y+R")
	keys(t, a, "A") // a note on the first step
	keys(t, a, "B")
	if a.np == nil {
		t.Fatal("B opens the note panel")
	}
	n, ok := a.npNote()
	if !ok {
		t.Fatal("the panel is about the note under the cursor")
	}
	v0 := n.Vel
	keys(t, a, "L")
	if n2, _ := a.npNote(); n2.Vel != v0-1 {
		t.Fatalf("Left lowers the velocity: %d -> %d", v0, n2.Vel)
	}
	keys(t, a, "D R R")
	if n2, _ := a.npNote(); n2.Len != n.Len+30 {
		t.Fatalf("Right on Length: %d -> %d", n.Len, n2.Len)
	}
	keys(t, a, "D R R R")
	n2, _ := a.npNote()
	if gridOff(n2.Tick) != 15 {
		t.Fatalf("Timing moves the note off the grid: %d", gridOff(n2.Tick))
	}
	keys(t, a, "A")
	if n3, _ := a.npNote(); gridOff(n3.Tick) != 0 {
		t.Fatalf("A puts the timing back: %d", gridOff(n3.Tick))
	}
	a.Render()
	keys(t, a, "]")
	if a.np != nil || a.menu == nil {
		t.Fatal("R1 goes to the pattern tools")
	}
}

func TestSlicePlanFollowsTheMenus(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, "A A D D R A") // a loop on A1
	a.go_(Slice)
	keys(t, a, "Y+A")
	if len(a.sl.Slices) < 4 {
		t.Skipf("only %d slices", len(a.sl.Slices))
	}
	plan := a.slicePlan()
	if len(plan) != a.steps2() {
		t.Fatalf("one cell per step: %d of %d", len(plan), a.steps2())
	}
	a.sl.Mode = formats.ModeShuffle
	a.sl.Seed = 5
	shuffled := a.slicePlan()
	same := true
	for i := range plan {
		same = same && plan[i].Slice == shuffled[i].Slice
	}
	if same {
		t.Fatal("the row below shows the order the mode makes, not only the order set")
	}
	a.sl.Division = 8
	if got := a.slicePlan(); len(got) != a.steps2() {
		t.Fatalf("a grid keeps the steps: %d", len(got))
	}
	a.Render() // the row is drawn
	au.Played = 0
	a.playResult(false)
	if len(a.placed) != a.steps2() || a.playingStep() != 0 {
		t.Fatalf("the step that sounds is lit: %d placed, step %d", len(a.placed), a.playingStep())
	}
	if a.sl.Division > 0 && a.sl.BPM != a.tempoOf(a.spad/16) {
		t.Fatalf("the grid runs at the bank's tempo: %g", a.sl.BPM)
	}
}

func TestResultToAPadAsksForBankAndPad(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, "A A D D R A")
	a.go_(Slice)
	keys(t, a, "Y+A")
	a.sliceToolsMenu()
	chooseMenuNoPick(t, a, "Result to a pad")
	if a.picker == nil || !a.picker.pads {
		t.Fatal("the pad dialog is asked")
	}
	keys(t, a, "] D") // bank B, a pad further down
	want := a.picker.bank*16 + a.picker.cur
	keys(t, a, "A")
	if !a.info.Pads[want].File {
		t.Fatalf("the result is on the chosen pad %d", want)
	}
}

func chooseMenuNoPick(t *testing.T, a *App, label string) {
	t.Helper()
	if !seekMenu(t, a, func(l string) bool { return strings.HasPrefix(l, label) }) {
		t.Fatalf("no menu item %q", label)
	}
	keys(t, a, "A")
}

func TestMangleWhichSlicesAndFixedValues(t *testing.T) {
	base := &formats.Slicing{Frames: 48000}
	for i := 0; i < 8; i++ {
		base.Slices = append(base.Slices, formats.Slice{Start: i * 6000, End: (i + 1) * 6000, Gain: 100, Chance: 100})
		base.Order = append(base.Order, i)
	}
	m := mangleSet{rev: 3, pitchN: 2, pitchFix: 11, roll: 1, rollFix: 3, which: 2, seed: 1} // the first ones, a fixed pitch and roll
	sl := mangleSlicing(base, m)
	for i := 0; i < 8; i++ {
		if sl.Slices[i].Reverse != (i < 3) {
			t.Fatalf("the first three are reversed: slice %d", i)
		}
	}
	if sl.Slices[0].Pitch != manglePitchFix[11] || sl.Slices[1].Pitch != manglePitchFix[11] || sl.Slices[2].Pitch != 0 {
		t.Fatalf("a fixed pitch on two slices: %d %d %d", sl.Slices[0].Pitch, sl.Slices[1].Pitch, sl.Slices[2].Pitch)
	}
	if sl.Slices[0].Roll != mangleRollFix[3] {
		t.Fatalf("a fixed roll: %d", sl.Slices[0].Roll)
	}
	m.which = 3 // the last ones
	sl = mangleSlicing(base, m)
	if !sl.Slices[7].Reverse || sl.Slices[0].Reverse {
		t.Fatal("the last ones")
	}
}

func TestRenameFindSetupExportRestorePads(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	first := a.info.Pads[0].Name
	if first == "" {
		t.Fatal("the kit puts a sample on A1")
	}
	// rename
	a.renamePad(0)
	if a.prompt == nil || a.prompt.buf != first {
		t.Fatalf("the prompt starts with the name: %+v", a.prompt)
	}
	a.prompt.buf = "thump"
	a.closePrompt(true)
	if a.info.Pads[0].Name != "thump" {
		t.Fatalf("renamed: %q", a.info.Pads[0].Name)
	}
	keys(t, a, "Y+{") // undo
	if a.info.Pads[0].Name != first {
		t.Fatalf("undo puts the name back: %q", a.info.Pads[0].Name)
	}
	// with two pads marked: a prefix
	a.marks = []int{0, 1}
	a.renamePad(0)
	if a.menu == nil || a.menu.title != "rename 2 marked pads" {
		t.Fatalf("marked pads ask prefix or suffix: %+v", a.menu)
	}
	chooseMenuNoPick(t, a, "Add a prefix")
	a.prompt.buf = "x_"
	a.closePrompt(true)
	if !strings.HasPrefix(a.info.Pads[0].Name, "x_") || !strings.HasPrefix(a.info.Pads[1].Name, "x_") {
		t.Fatalf("both pads got the prefix: %q %q", a.info.Pads[0].Name, a.info.Pads[1].Name)
	}
	a.marks = nil
	// find
	a.cur, a.bank = 5, 3
	a.findPad()
	a.prompt.buf = "x_"
	a.closePrompt(true)
	if a.bank != 0 || a.cur != 0 {
		t.Fatalf("find goes to A1: bank %d pad %d", a.bank, a.cur)
	}
	// set up
	a.lab.SetPadParamRaw(0, 0x78, 0)
	if v, _ := a.lab.PadParam(0, 0x78); v != 0 {
		t.Fatal("the test needs a pad that was never set up")
	}
	a.setupPads(false)
	if v, _ := a.lab.PadParam(0, 0x78); v != 64 {
		t.Fatalf("the pan is back at the centre: %d", v)
	}
	// export, clear the pads, restore
	a.exportBank()
	dir := filepath.Join(a.exportDir(), app.SanitizeName(a.ProjectName())+"_bank_A")
	for _, f := range []string{"samples.json", "A01 x_" + app.SanitizeName(strings.TrimPrefix(a.info.Pads[0].Name, "x_")) + ".wav"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("export has %s: %v", f, err)
		}
	}
	a.lab.ClearPad(0)
	a.lab.ClearPad(1)
	a.refresh()
	if a.info.Pads[0].File {
		t.Fatal("A1 is cleared")
	}
	a.restorePads(dir)
	if !a.info.Pads[0].File || !a.info.Pads[1].File || !a.info.Pads[2].File {
		t.Fatalf("the pads are back: %v %v %v", a.info.Pads[0].File, a.info.Pads[1].File, a.info.Pads[2].File)
	}
	if v, _ := a.lab.PadParam(0, 0x78); v != 64 {
		t.Fatalf("the settings come back too: pan %d", v)
	}
}

func TestPatternSlotsCopyMoveAndRetarget(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	keys(t, a, "Y+R A") // SEQ, a note on A1
	if _, ok := a.doc(); !ok {
		t.Fatal("a pattern with a note")
	}
	from := a.pat
	a.seqMenu()
	chooseMenuNoPick(t, a, "Copy to a slot")
	if a.picker == nil || a.picker.title == "" {
		t.Fatal("the pattern picker opens to choose the slot")
	}
	keys(t, a, "R R A") // two slots on, chosen
	if a.picker != nil {
		t.Fatal("A chooses")
	}
	to := from + 3 // the picker starts on the slot after this one
	if b, err := a.lab.Pattern(to); err != nil || len(b) == 0 {
		t.Fatalf("the copy is in slot %d: %v", to, err)
	}
	if _, err := a.lab.Pattern(from); err != nil {
		t.Fatal("the original stays")
	}
	// move it one slot on
	a.seqMenu()
	chooseMenuNoPick(t, a, "Move to a slot")
	keys(t, a, "R A")
	if a.pat != from+2 {
		t.Fatalf("the pattern in front is the one that moved: %d, wanted %d", a.pat, from+2)
	}
	// retarget the pattern on the move to bank B
	a.seqMenu()
	chooseMenuNoPick(t, a, "Use the pads of another bank")
	chooseMenuNoPick(t, a, "Bank B")
	a.refresh()
	b, _ := a.lab.PatternBytes(a.pat)
	if banks := patternBanks(b); len(banks) != 1 || banks[0] != 1 {
		t.Fatalf("the notes are on the pads of bank B now: %v", banks)
	}
}

func TestUnitProjectsLoadBackupRestoreRenameErase(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	onScreen(t, a, Unit)
	keys(t, a, "A")
	a.Tick()
	if a.link == nil {
		keys(t, a, ".")
	}
	if a.link == nil {
		t.Fatal("the unit was not found")
	}
	// send the kit over project 3, then take it back into a fresh editor
	keys(t, a, "D D A")
	chooseMenuNoPick(t, a, "Send: replace this project")
	keys(t, a, "A ~")
	if _, _, _, err, fin := a.job.state(); !fin || err != nil {
		t.Fatalf("send: finished %v err %v", fin, err)
	}
	a.job = nil
	// back it up
	keys(t, a, "A")
	chooseMenuNoPick(t, a, "Back up: write a copy")
	waitJob(t, a)
	backups, _ := os.ReadDir(filepath.Join(a.dataDir(), "backups"))
	if len(backups) != 1 {
		t.Fatalf("one backup: %v", backups)
	}
	// load it into the editor
	a.lab.ClearPad(0)
	a.refresh()
	a.saved = true // the editor has nothing to lose
	keys(t, a, "A")
	chooseMenuNoPick(t, a, "Open a copy in the editor")
	waitJob(t, a)
	if padsWithSamples(a) != 3 {
		t.Fatalf("the unit's project has the 3 pads of the kit: %d", padsWithSamples(a))
	}
	// rename it
	keys(t, a, "Y+R Y+R Y+R Y+R") // somewhere else and back, to be on UNIT with the link
	keys(t, a, "Y+L Y+L")
	for a.Screen() != Unit {
		keys(t, a, "Y+L")
	}
	keys(t, a, "A")
	chooseMenuNoPick(t, a, "Rename")
	a.prompt.buf = "KIT TWO"
	a.closePrompt(true)
	if names, _ := a.link.Projects(); names[a.unitCur] != "KIT TWO" {
		t.Fatalf("renamed on the unit: %q", names[a.unitCur])
	}
	// erase it
	keys(t, a, "A")
	chooseMenuNoPick(t, a, "Erase it")
	chooseMenuNoPick(t, a, "Yes, erase it")
	if !strings.Contains(a.msg, "erased") {
		t.Fatalf("erased: %q", a.msg)
	}
}

// waitJob lets the job in the background finish and the main loop take what comes of it.
func waitJob(t *testing.T, a *App) {
	t.Helper()
	for i := 0; i < 600; i++ {
		a.Tick()
		if a.job != nil {
			if _, _, _, err, fin := a.job.state(); fin {
				if err != nil {
					t.Fatalf("job: %v", err)
				}
				a.Tick()
				a.job = nil
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the job did not finish")
}

func TestEditingTheUnitLive(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	onScreen(t, a, Unit)
	keys(t, a, "A")
	a.Tick()
	if a.link == nil {
		keys(t, a, ".")
	}
	dev, ok := a.unitDevice()
	if !ok {
		t.Fatal("the unit was not found")
	}
	keys(t, a, "D D A")
	chooseMenuNoPick(t, a, "Send: replace this project")
	keys(t, a, "A ~")
	a.job = nil
	// edit project 3 live
	a.saved = true
	keys(t, a, "A")
	chooseMenuNoPick(t, a, "Edit live")
	waitJob(t, a)
	if a.live == nil || a.live.project != 2 {
		t.Fatalf("live editing of project 3: %+v", a.live)
	}
	waitLive := func() {
		t.Helper()
		for i := 0; i < 600; i++ {
			if w, _, err := a.liveStatus(); w == 0 && err == nil {
				return
			} else if err != nil {
				t.Fatalf("live: %v", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("the unit never got what was changed")
	}
	// a pad is renamed in the editor
	a.go_(Pads)
	a.renameEach([]int{0}, func(string) string { return "live kick" })
	waitLive()
	pad0, _ := proto.NewPadIndex(0)
	blk, err := dev.PadBlock(pad0)
	if err != nil || blk.Name() != "live kick" {
		t.Fatalf("the unit has the new name: %q %v", blk.Name(), err)
	}
	// a setting
	pre := a.lab.Snapshot()
	a.lab.SetPadParam(0, 0x78, 90)
	a.commitUndo(pre, A, 0)
	waitLive()
	blk, _ = dev.PadBlock(pad0)
	if v, _ := blk.Param(0x78); v != 90 {
		t.Fatalf("the unit has the pan: %d", v)
	}
	// undo goes to the unit too
	keys(t, a, "Y+{")
	waitLive()
	blk, _ = dev.PadBlock(pad0)
	if v, _ := blk.Param(0x78); v == 90 {
		t.Fatal("undo reached the unit")
	}
	// a pad is cleared
	pre = a.lab.Snapshot()
	a.lab.ClearPad(1)
	a.refresh()
	a.commitUndo(pre, A, 0)
	waitLive()
	pad1, _ := proto.NewPadIndex(1)
	if blk, _ := dev.PadBlock(pad1); blk.HasSample() {
		t.Fatal("the unit's pad is cleared")
	}
	// a note in a pattern is written to the unit's pattern file
	keys(t, a, "Y+R A")
	waitLive()
	slot, _ := proto.NewPadIndex(uint16(a.pat - 1))
	if b, err := dev.ReadFile(formats.PatternPath(3, slot)); err != nil || len(b) < 16 {
		t.Fatalf("the unit has the pattern: %d bytes, %v", len(b), err)
	}
	// a changed sample (reversed) is imported again
	a.go_(Pads)
	a.transform(0, "reverse", 0)
	waitLive()
	if f, ok := a.lab.PadFile(0); !ok {
		t.Fatal("the editor has the sample")
	} else if got, err := dev.ReadFile(pad0.SamplePath(3)); err != nil || len(got) != len(f) {
		t.Fatalf("the unit has the new sample: %d bytes against %d (%v)", len(got), len(f), err)
	}
	a.stopLive()
	if a.live != nil {
		t.Fatal("stopped")
	}
}

type fakeRecorder struct{ dir string }

func (f fakeRecorder) Start(dir, device string) (*record.Session, error) {
	path := filepath.Join(dir, "take.wav")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(path, make([]byte, 4096), 0o644)
	return record.NewSession(path, "fake", func() error { return nil }), nil
}

func TestRecordingASetWithTheRecorderOfThePlatform(t *testing.T) {
	a, _ := newApp(t)
	a.ToggleRecording()
	if a.rec != nil || !a.msgBad {
		t.Fatalf("no recorder here says so: %q", a.msg)
	}
	a.set.RecordDir = filepath.Join(t.TempDir(), "takes")
	a.SetPlatform(Platform{Name: "test", Recorder: fakeRecorder{}})
	keys(t, a, "S+X") // Select+X records from any screen
	if a.rec == nil {
		t.Fatalf("recording: %q", a.msg)
	}
	if !strings.HasPrefix(a.recText(), "REC ") {
		t.Fatalf("the title bar shows the time: %q", a.recText())
	}
	a.Render()
	keys(t, a, "S+X")
	for i := 0; i < 100 && !strings.HasPrefix(a.msg, "recorded"); i++ {
		a.Tick()
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.HasPrefix(a.msg, "recorded") || a.rec != nil {
		t.Fatalf("recorded: %q", a.msg)
	}
	if _, err := os.Stat(filepath.Join(a.recordDir(), "take.wav")); err != nil {
		t.Fatal(err)
	}
}

// onScreen goes right through the screens until the one is in front.
func onScreen(t *testing.T, a *App, s Screen) {
	t.Helper()
	for i := 0; i < len(screenNames)+1 && a.Screen() != s; i++ {
		keys(t, a, "Y+R")
	}
	if a.Screen() != s {
		t.Fatalf("cannot get to screen %d (at %d)", s, a.Screen())
	}
}

func padNote(pad int) int { // the MIDI note of a pad (1..16)
	order := [16]int{12, 13, 14, 15, 8, 9, 10, 11, 4, 5, 6, 7, 0, 1, 2, 3}
	return 36 + order[pad-1]
}

func TestPerformFollowsTheUnitsMIDI(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	base := time.Now().Add(-10 * time.Second)
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Second / 48) } // 24 ticks to a beat of 0.5 s
	a.midiEvent(midiin.Event{Kind: midiin.Start, At: at(0)})
	for i := 1; i <= 24*4+10; i++ {
		a.midiEvent(midiin.Event{Kind: midiin.Clock, At: at(i)})
	}
	if bar, beat := a.clk.Beat(); bar != 1 || beat != 0 {
		t.Fatalf("the clock is at bar 2 beat 1: %d %d", bar+1, beat+1)
	}
	if a.mon.BPM < 115 || a.mon.BPM > 125 {
		t.Fatalf("the tempo from the clock: %v", a.mon.BPM)
	}
	// a pad hit lights the pad (the clock does not run any more, so it is hit by hand)
	a.clk.Running = false
	a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 3, Data1: padNote(5), Data2: 100, At: time.Now()})
	if g := a.mon.Glow(2, 4, time.Now()); g < 0.5 {
		t.Fatalf("pad 5 of bank C is lit: %v", g)
	}
	// a controller shows up in the list
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 16, Data2: 90, At: time.Now()})
	if a.mon.CCValue(1, 16) != 90 {
		t.Fatal("the knob C1 of BUS 1")
	}
	// the pad code 1 3 3 7 is live mode
	a.go_(Pads)
	for _, p := range []int{1, 3, 3, 7} {
		a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 1, Data1: padNote(p), Data2: 100, At: time.Now()})
	}
	if a.Screen() != Perform {
		t.Fatalf("the code goes to PERF: %v", a.Screen())
	}
	for _, p := range []int{1, 3, 3, 7} {
		a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 1, Data1: padNote(p), Data2: 100, At: time.Now()})
	}
	if a.Screen() != Pads {
		t.Fatalf("the same code goes back: %v", a.Screen())
	}
	// a message of the control channel: note 62 on channel 16 is live mode
	a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 16, Data1: 62, Data2: 100, At: time.Now()})
	if a.Screen() != Perform {
		t.Fatalf("note 62 on channel 16 is live mode: %v", a.Screen())
	}
	// the screen draws, and the patterns are chosen
	a.Render()
	a.RenderCells()
	keys(t, a, "A")
	if a.picker == nil {
		t.Fatal("A chooses the pattern that plays")
	}
	keys(t, a, "A")
	if a.perfNow != 1 {
		t.Fatalf("now: %d", a.perfNow)
	}
	keys(t, a, "X A")
	if a.perfNext != 2 {
		t.Fatalf("next: %d", a.perfNext)
	}
	keys(t, a, "Y+A")
	if a.perfNow != 2 || a.perfNext != 0 {
		t.Fatalf("the next became the one that plays: %d %d", a.perfNow, a.perfNext)
	}
}

type fakeMIDI struct{ ch chan midiin.Event }

func (f fakeMIDI) Ports() ([]string, error) { return []string{"SP-404MKII"}, nil }
func (f fakeMIDI) Open(part string, on func(midiin.Event)) (func(), string, error) {
	go func() {
		for e := range f.ch {
			on(e)
		}
	}()
	return func() {}, "SP-404MKII", nil
}

func TestTheMIDIInputOfThePlatformIsOpenedAndRead(t *testing.T) {
	a, _ := newApp(t)
	f := fakeMIDI{ch: make(chan midiin.Event, 8)}
	a.SetPlatform(Platform{Name: "test", MIDI: f})
	a.Tick()
	if a.midi == nil || !strings.Contains(a.midiStatus(), "SP-404MKII") {
		t.Fatalf("the input is open: %q", a.midiStatus())
	}
	f.ch <- midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 17, Data2: 33}
	for i := 0; i < 100 && a.mon.CCValue(2, 17) != 33; i++ {
		a.Tick()
		time.Sleep(10 * time.Millisecond)
	}
	if a.mon.CCValue(2, 17) != 33 {
		t.Fatal("the controller came through")
	}
	b, _ := newApp(t)
	if b.midiStatus() != "no MIDI in this build" {
		t.Fatalf("without MIDI it says so: %q", b.midiStatus())
	}
}

func TestTheLooperMakesThePatternDeletesAndRemixes(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	onScreen(t, a, Loop)
	keys(t, a, "A")
	b, err := a.lab.PatternBytes(1)
	if err != nil || len(patternPadsOf(b)) != 3 {
		t.Fatalf("the pattern has the 3 loops: %v %v", patternPadsOf(b), err)
	}
	if patternBars(b) != 2 {
		t.Fatalf("2 bars when nothing says otherwise: %d", patternBars(b))
	}
	keys(t, a, "D D D D X") // pad 5 is empty
	if padsWithSamples(a) != 3 {
		t.Fatal("nothing to delete on an empty pad")
	}
	a.loopCur = 1
	keys(t, a, "X")
	if !a.info.Pads[1].File || !a.msgBad {
		t.Fatalf("the first X asks: %q", a.msg)
	}
	keys(t, a, "X")
	if a.info.Pads[1].File {
		t.Fatal("the second X deletes")
	}
	a.loopCur = 0
	keys(t, a, "Y+X")
	n := padsWithSamples(a)
	if n != 3 {
		t.Fatalf("the remix is a new pad on a free pad: %d pads", n)
	}
	if !a.info.Pads[1].File {
		t.Fatal("the remix is on the next free pad, pad 2")
	}
	a.Render()
	a.RenderCells()
}

func connectUnit(t *testing.T, a *App) {
	t.Helper()
	a.go_(Unit)
	keys(t, a, "A")
	a.Tick()
	if a.link == nil {
		keys(t, a, ".")
	}
	if a.link == nil {
		t.Fatal("the unit was not found")
	}
}

func TestTheBrowserReachesTheUnitsCardAndDoesFileOperations(t *testing.T) {
	a, _ := newApp(t)
	connectUnit(t, a)
	a.go_(Files)
	a.cd(app.CardRoot)
	if a.dir != app.CardRoot || a.msgBad {
		t.Fatalf("the card is listed: %q %q", a.dir, a.msg)
	}
	// a new folder on the card
	a.newFolder()
	a.prompt.buf = "takes"
	a.closePrompt(true)
	found := false
	for _, e := range a.entries {
		found = found || (e.dir && e.name == "takes")
	}
	if !found {
		t.Fatalf("the folder is there: %v", a.entries)
	}
	a.cd(joinPath(app.CardRoot, "takes"))
	if a.dir != "SD:takes" {
		t.Fatalf("path of a folder of the card: %q", a.dir)
	}
	// copy a file of the computer into it
	a.fclip = []string{filepath.Join(a.Root, "drums", "kick_808.wav")}
	a.pasteFiles()
	waitJob(t, a)
	if len(a.entries) != 1 || a.entries[0].name != "kick_808.wav" {
		t.Fatalf("the file came over: %v (%s)", a.entries, a.msg)
	}
	// it can be heard and put on a pad from there
	id, err := a.decode(a.entries[0])
	if err != nil || id == 0 {
		t.Fatalf("a file of the card is read: %v", err)
	}
	// rename, then up and delete the folder
	a.renameFile()
	a.prompt.buf = "kick_two.wav"
	a.closePrompt(true)
	if len(a.entries) != 1 || a.entries[0].name != "kick_two.wav" {
		t.Fatalf("renamed: %v", a.entries)
	}
	keys(t, a, "L")
	if a.dir != app.CardRoot {
		t.Fatalf("up goes to the card: %q", a.dir)
	}
	for i, e := range a.entries {
		if e.name == "takes" {
			a.fcur = i
		}
	}
	a.deleteFilesAsk()
	chooseMenuNoPick(t, a, "Yes, delete")
	waitJob(t, a)
	for _, e := range a.entries {
		if e.name == "takes" {
			t.Fatalf("the folder is gone: %v", a.entries)
		}
	}
	// the computer's files are not deleted from here
	a.cd(a.Root)
	a.deleteFilesAsk()
	if a.menu != nil || !a.msgBad {
		t.Fatalf("deleting on the computer is refused: %q", a.msg)
	}
	// marks
	a.cd(filepath.Join(a.Root, "drums"))
	a.selHeld = true
	a.selShifted(A)
	a.selHeld = false
	if len(a.marked()) != 1 {
		t.Fatalf("Select+A marks: %v", a.marked())
	}
}

func TestThePadMenuHasThePadAndBankActions(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.openPadMenu()
	got := strings.Join(menuLabels(a.menu), "|")
	for _, want := range []string{"Rename...", "Find a pad by name...", "Set up the pads that were never set up", "Switches for every pad of the bank...", "Set the buses of the bank by the names...", "Export the bank to a folder", "Restore pads from an export..."} {
		if !strings.Contains(got, want) {
			t.Errorf("the pad menu has %q: %s", want, got)
		}
	}
}

func TestSlicerTapOrderLaneSeveralSamplesAndPatternUnder(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.cur = 0
	a.go_(Slice)
	if a.sl == nil {
		t.Fatal("the slicer is open on the kick")
	}
	// tap: the sample plays, A cuts where it is
	a.startTap()
	a.tapT0 = time.Now().Add(-time.Duration(float64(a.sl.Frames) / formats.SampleRate * float64(time.Second) / 2))
	a.tapTick()
	if a.at < a.sl.Frames/3 {
		t.Fatalf("the cursor follows the sample: %d of %d", a.at, a.sl.Frames)
	}
	keys(t, a, "A")
	if len(a.sl.Slices) != 2 {
		t.Fatalf("A cuts while tapping: %d slices", len(a.sl.Slices))
	}
	keys(t, a, "B")
	if a.tap {
		t.Fatal("B stops the tap")
	}
	// the order lane
	a.openLane()
	keys(t, a, "R U") // step 2: another slice
	if a.sl.Order[1] != 1+1-1 && len(a.sl.Order) != 2 {
		t.Fatalf("order %v", a.sl.Order)
	}
	keys(t, a, "A") // repeat
	if len(a.sl.Order) != 3 {
		t.Fatalf("a step repeated: %v", a.sl.Order)
	}
	keys(t, a, "B") // drop
	if len(a.sl.Order) != 2 {
		t.Fatalf("a step dropped: %v", a.sl.Order)
	}
	keys(t, a, "[")
	keys(t, a, "Y+B")
	if a.lane {
		t.Fatal("Y+B leaves the lane")
	}
	a.Render()
	a.RenderCells()
	// several samples: the sample of pad 2 is added
	frames := a.sl.Frames
	a.addSampleFromPad()
	keys(t, a, "R A") // pad A2
	if a.sl.Frames <= frames || len(a.sl.Slices) != 3 {
		t.Fatalf("a sample was added: %d frames, %d slices", a.sl.Frames, len(a.sl.Slices))
	}
	a.takeLastSampleOut()
	if a.sl.Frames != frames || len(a.sl.Slices) != 2 {
		t.Fatalf("taken out again: %d frames, %d slices", a.sl.Frames, len(a.sl.Slices))
	}
	// the pattern under the slices
	a.patternUnder(2)
	keys(t, a, "A") // the pad dialog: the first free run
	if a.picker == nil {
		t.Fatal("the pattern is asked for")
	}
	keys(t, a, "R R R A")
	if a.picker != nil {
		t.Fatal("chosen")
	}
	if padsWithSamples(a) != 5 {
		t.Fatalf("the 2 slices are on pads: %d pads", padsWithSamples(a))
	}
	b, err := a.lab.PatternBytes(4)
	if err != nil || len(patternPadsOf(b)) < 2 {
		t.Fatalf("the pattern plays the slices: %v %v", patternPadsOf(b), err)
	}
}

func TestTagsAndTheIndexOfTheUnitsFiles(t *testing.T) {
	a, _ := newApp(t)
	// without a unit there is nothing to tag
	a.go_(Files)
	a.filesMenu()
	for _, it := range a.menu.items {
		if strings.Contains(it.label, "Search by tags") {
			t.Fatal("tags are offered without a unit")
		}
	}
	a.menu = nil
	connectUnit(t, a)
	a.cd(app.CardRoot)
	a.filesMenu()
	has := false
	for _, it := range a.menu.items {
		has = has || strings.Contains(it.label, "Search by tags")
	}
	if !has {
		t.Fatal("tags are offered with a unit")
	}
	a.menu = nil
	a.indexUnitFolder(app.CardRoot)
	waitJob(t, a)
	if a.msgBad || !strings.Contains(a.msg, "indexed") {
		t.Fatalf("indexed: %q", a.msg)
	}
	// the index is on the card, and a search by tag finds the file by its name's words
	if _, ok, _ := a.fileStore().Stat(app.TagIndexPath); !ok {
		t.Fatal("the index is not on the card")
	}
	a.tagSearch, a.searchQ = true, "kick"
	a.list()
	if len(a.entries) != 1 || !strings.Contains(a.entries[0].name, "kick 808") {
		t.Fatalf("tag search: %v", a.entries)
	}
	a.tagSearch, a.searchQ = false, ""
	a.cd(joinPath(app.CardRoot, "IMPORT"))
	for i, e := range a.entries {
		if e.name == "kick 808.wav" {
			a.fcur = i
		}
	}
	a.tagFiles(a.fileTargets(), true)
	a.prompt.buf = "wobbly"
	a.closePrompt(true)
	if !strings.Contains(a.tagsOf(joinPath(a.dir, "kick 808.wav")), "wobbly") {
		t.Fatalf("the tag is on the file: %q", a.tagsOf(joinPath(a.dir, "kick 808.wav")))
	}
	// it is saved: a fresh program reads it back
	a.tags = nil
	a.cd(app.CardRoot)
	a.tagSearch, a.searchQ = true, "wobbly"
	a.tagIndex()
	a.list()
	if len(a.entries) != 1 {
		t.Fatalf("the tag was saved: %v", a.entries)
	}
}

func TestOnAComputerBGoesBackToTheMenuItCameFrom(t *testing.T) {
	a, _ := newApp(t)
	Terminal = true
	defer func() { Terminal = false }()
	child := func() { a.menu = &menu{title: "child", items: []menuItem{{label: "x"}}} }
	a.menu = &menu{title: "parent", items: []menuItem{{label: "open it", run: func(int) { child() }}}}
	parent := a.menu
	a.menuKey(A)
	if a.menu == nil || a.menu.title != "child" {
		t.Fatalf("A opens the child: %+v", a.menu)
	}
	a.menuKey(B)
	if a.menu != parent {
		t.Fatal("B goes back to the parent")
	}
	a.menuKey(B)
	if a.menu != nil {
		t.Fatal("B again closes it")
	}
	// the handheld's B closes at once
	Terminal = false
	a.menu = &menu{title: "parent", items: []menuItem{{label: "open it", run: func(int) { child() }}}}
	a.menuKey(A)
	a.menuKey(B)
	if a.menu != nil {
		t.Fatal("on the handheld B closes the child")
	}
}

func TestOnAComputerTheSliceOrderLoops(t *testing.T) {
	a, au := newApp(t)
	keys(t, a, kit)
	Terminal = true
	defer func() { Terminal = false }()
	a.go_(Slice)
	if a.sl == nil {
		t.Fatal("the slicer opens on a pad with a sample")
	}
	base := au.Loops
	a.playResult(false)
	if !a.orderOn || au.Loops != base+1 {
		t.Fatalf("the order plays in a loop: on %v, loops %d (was %d)", a.orderOn, au.Loops, base)
	}
	seed := a.sl.Seed
	a.NewOrder()
	if !a.orderOn || a.sl.Seed == seed {
		t.Fatalf("a new order goes on playing: on %v, seed %d (was %d)", a.orderOn, a.sl.Seed, seed)
	}
	a.playResult(false)
	if a.orderOn {
		t.Fatal("asking again stops the loop")
	}
}

func TestUndoOnEveryScreenLeavesAScreenThatDraws(t *testing.T) {
	for _, term := range []bool{false, true} {
		Terminal = term
		for scr := Screen(0); int(scr) < len(screenNames); scr++ {
			a, _ := newApp(t)
			Terminal = false // the kit goes on with the handheld's keys (a computer asks which pad in a dialog)
			keys(t, a, kit)
			Terminal = term
			a.go_(Seq)
			keys(t, a, "A R R R A") // an edit to undo
			onScreen(t, a, Pads)
			a.go_(scr)
			a.undoStep()
			a.Render()
			a.RenderCells()
			a.redoStep()
			a.Render()
			a.RenderCells()
		}
	}
	Terminal = false
}

func TestUndoWithAPanelOrAModeOpenStillDraws(t *testing.T) {
	for _, term := range []bool{false, true} {
		Terminal = term
		for i, setup := range []string{"A", "S+B", "Y+X", "Y+B"} { // pad panel, tools, add track, pattern picker / settings
			a, _ := newApp(t)
			keys(t, a, kit)
			a.go_(Seq)
			keys(t, a, "A R R R A")
			if i == 0 {
				a.go_(Pads)
			}
			keys(t, a, setup)
			a.undoStep()
			a.Render()
			a.RenderCells()
			keys(t, a, "B")
			a.Render()
		}
		a, _ := newApp(t)
		keys(t, a, kit)
		a.go_(Seq)
		keys(t, a, "A R R R A")
		a.go_(Slice)
		a.startTap()
		a.undoStep()
		a.Render()
		a.RenderCells()
		a.go_(Slice)
		a.openLane()
		a.undoStep()
		a.Render()
		a.RenderCells()
	}
	Terminal = false
}

func TestUndoAndRedoInTheSlicerTakeBackCutsAndJoins(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Slice)
	a.at = a.sl.Frames / 3
	keys(t, a, "A")
	a.at = a.sl.Frames * 2 / 3
	keys(t, a, "A")
	n := len(a.sl.Slices)
	if n < 3 {
		t.Fatalf("two cuts make three slices, got %d", n)
	}
	keys(t, a, "B") // the slice under the cursor is removed
	if len(a.sl.Slices) != n-1 {
		t.Fatalf("B removes a slice: %d of %d", len(a.sl.Slices), n)
	}
	a.undoStep()
	if len(a.sl.Slices) != n {
		t.Fatalf("undo brings the slice back: %d", len(a.sl.Slices))
	}
	a.undoStep()
	if len(a.sl.Slices) != n-1 {
		t.Fatalf("undo takes the last cut back: %d", len(a.sl.Slices))
	}
	a.redoStep()
	if len(a.sl.Slices) != n {
		t.Fatalf("redo makes the cut again: %d", len(a.sl.Slices))
	}
	if a.Screen() != Slice {
		t.Fatalf("the slicer stays in front, got %v", a.Screen())
	}
}

func TestSwapTwoTracks(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)
	a.go_(Seq)
	a.addTrack(0)
	a.seqMenu()
	a.trackDefaults()
	a.tt.fillPat, a.tt.fillDecay = 3, 3
	chooseMenu(t, a, "Fill...")
	chooseMenuContaining(t, a, "Keep it")
	keys(t, a, "B")
	n0 := a.trackNotes(0)
	if len(n0) == 0 {
		t.Fatal("no notes to swap")
	}
	// pad 1 gets two notes by hand
	a.toggle(1, 0)
	a.toggle(1, 4)
	n1 := a.trackNotes(1)
	if len(n1) != 2 {
		t.Fatalf("pad 1 has %d notes", len(n1))
	}
	a.swapTracks(0, 1)
	if got := a.trackNotes(0); len(got) != 2 || got[0].Pad != 0 || got[0].Tick != n1[0].Tick || got[1].Tick != n1[1].Tick {
		t.Fatalf("pad 0 after the swap: %+v", got)
	}
	got1 := a.trackNotes(1)
	if len(got1) != len(n0) || got1[0].Pad != 1 || got1[0].Tick != n0[0].Tick || got1[0].Vel != n0[0].Vel {
		t.Fatalf("pad 1 after the swap: %d notes, first %+v (was %+v)", len(got1), got1[0], n0[0])
	}
	// undo puts both back
	a.undoStep()
	if len(a.trackNotes(0)) != len(n0) || len(a.trackNotes(1)) != 2 {
		t.Fatalf("after undo: %d and %d notes", len(a.trackNotes(0)), len(a.trackNotes(1)))
	}
	a.swapTracks(0, 0)
	if len(a.trackNotes(0)) != len(n0) {
		t.Fatal("swapping a pad with itself changed it")
	}
}

// a favourite folder has a mark at the right of its name in the lists
func TestFavouriteFolderHasAMark(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.go_(Files)
	a.cd(a.Root)
	a.Render()
	var row string
	find := func() string {
		for _, l := range strings.Split(screenStr(a), "\n") {
			if strings.Contains(l, "drums/") {
				return l
			}
		}
		return ""
	}
	if row = find(); row == "" || strings.Contains(row, "[*]") {
		t.Fatalf("a folder that is not a favourite has no mark: %q", row)
	}
	a.toggleFavourite(filepath.Join(a.Root, "drums"))
	if row = find(); !strings.Contains(row, "[*]") {
		t.Fatalf("a favourite folder has a mark: %q", row)
	}
}

// the search by categories: the tags of the indexed samples (kick, hat...), not their names; Tab cycles names, folders, categories
func TestSearchByCategoriesInTheIndex(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.go_(Files)
	a.cd(a.Root)
	a.startIndex(a.Root)
	for i := 0; i < 200; i++ {
		a.idx.mu.Lock()
		running := a.idx.running
		a.idx.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.indexTick()
	a.OpenTagSearch()
	if a.kb == nil || !a.tagSearch {
		t.Fatal("# opens the search by categories")
	}
	all := len(a.entries)
	if all == 0 {
		t.Fatal("with no words every indexed sample is a result")
	}
	a.searchQ = "kick"
	a.list()
	if len(a.entries) == 0 || len(a.entries) >= all {
		t.Fatalf("kick narrows %d to %d", all, len(a.entries))
	}
	for _, e := range a.entries {
		if !strings.Contains(strings.ToLower(e.name), "kick") && !strings.Contains(strings.ToLower(e.name), "bd") {
			t.Logf("kick category: %s", e.name)
		}
	}
	if !strings.Contains(a.Message(), "narrow it with") {
		t.Fatalf("the categories that narrow it are named: %q", a.Message())
	}
	// wherever the folder in view is: the whole index is searched
	other := t.TempDir()
	a.kb = nil
	a.cd(other)
	a.OpenTagSearch()
	a.searchQ = "kick"
	a.list()
	if len(a.entries) == 0 {
		t.Fatal("a folder outside the index still searches it")
	}
	if e := a.entries[0]; e.where == "" || !strings.HasPrefix(e.where, a.Root) {
		t.Fatalf("a result names its folder: %+v", e)
	}
	if _, ok := a.current(); !ok || !strings.HasSuffix(joinPath(a.dir, a.entries[0].name), ".wav") {
		t.Fatalf("the name leads to the file: %s", joinPath(a.dir, a.entries[0].name))
	}
	a.TypeTab() // categories -> names
	if a.tagSearch || a.searchDirs {
		t.Fatal("Tab from categories is the names again")
	}
	a.TypeTab()
	a.TypeTab()
	if !a.tagSearch {
		t.Fatal("names, folders, categories")
	}
}

// your own tags: a file, or a folder for everything below it; the search by categories finds them, and they are kept
func TestLocalTagsAreFoundByTheCategorySearch(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.go_(Files)
	a.cd(a.Root)
	a.startIndex(a.Root)
	for i := 0; i < 200; i++ {
		a.idx.mu.Lock()
		running := a.idx.running
		a.idx.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.indexTick()
	bass := filepath.Join(a.Root, "bass")
	files := filepath.Join(bass, "bass_C1.wav")
	st := a.localTagStore()
	if n := st.change([]string{files}, []string{"Wobbly"}, true); n != 1 {
		t.Fatalf("one file tagged: %d", n)
	}
	if n := st.change([]string{filepath.Join(a.Root, "drums")}, []string{"909"}, true); n != 1 {
		t.Fatalf("one folder tagged: %d", n)
	}
	a.saveLocalTags()
	a.OpenTagSearch()
	a.searchQ = "wobbly"
	a.list()
	if len(a.entries) != 1 || !strings.HasSuffix(a.entries[0].name, "bass_C1.wav") {
		t.Fatalf("the file with the tag: %+v", a.entries)
	}
	a.searchQ = "909"
	a.list()
	if len(a.entries) == 0 {
		t.Fatal("a tag on a folder is on its files")
	}
	for _, e := range a.entries {
		if !strings.HasPrefix(e.name, "drums") {
			t.Fatalf("only the drums have it: %s", e.name)
		}
	}
	if got := a.tagsOf(files); got != "wobbly" {
		t.Fatalf("the panel's tags: %q", got)
	}
	a.ltags = nil
	if n := a.localTagStore().change([]string{files}, []string{"wobbly"}, false); n != 1 {
		t.Fatal("the tags were kept, and can be taken off")
	}
}
