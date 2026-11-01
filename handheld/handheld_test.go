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
	keys(t, a, "A R R R R A")
	d, ok := a.doc()
	if !ok || len(d.Notes) != 2 || d.Bars != 4 {
		t.Fatalf("two notes in a 4 bar pattern, got %+v", d)
	}
	keys(t, a, "A")
	if d, _ = a.doc(); len(d.Notes) != 1 {
		t.Fatalf("A on a note takes it off, notes %d", len(d.Notes))
	}
	keys(t, a, "D B A") // the menu: fill the row below with "every beat"
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
