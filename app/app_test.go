package app_test

import (
	"slices"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
	"github.com/moahbeatlab/404tui/sim"
)

func connect(t *testing.T) (*device.Device, *sim.Sim) {
	t.Helper()
	s := sim.New()
	d := device.New(s, "sim")
	t.Cleanup(func() { d.Close() })
	return d, s
}

func pad(t *testing.T, name string) proto.PadIndex {
	t.Helper()
	p, err := proto.ParsePad(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAndPadEdits(t *testing.T) {
	d, s := connect(t)
	s.SetSample(pad(t, "A1"), "kick", 1000)
	s.SetSample(pad(t, "A3"), "hat", 1000)

	snap, err := app.Load(d)
	if err != nil {
		t.Fatal(err)
	}
	bank := snap.Bank(0)
	if snap.Port != "sim" || snap.ProjectName != "PROJECT_01" || len(snap.Projects) != 16 || len(snap.Pads) != 160 {
		t.Errorf("snapshot = %+v", snap)
	}
	if !bank[0].HasSample || bank[0].Name != "kick" || bank[1].HasSample || bank[2].Name != "hat" {
		t.Errorf("bank A = %+v", bank[:3])
	}

	// The empty pad in between is skipped, not failed.
	renamed, failed := app.BulkRename(d, bank[:3], "x_", false)
	if renamed != 2 || failed != nil {
		t.Errorf("renamed %d, failed %v", renamed, failed)
	}
	renamed, _ = app.BulkRename(d, bank[:1], "_1", true)
	snap, _ = app.Load(d)
	if renamed != 1 || snap.Bank(0)[0].Name != "kick_1" || snap.Bank(0)[2].Name != "x_hat" {
		t.Errorf("after rename: %q, %q", snap.Bank(0)[0].Name, snap.Bank(0)[2].Name)
	}

	failed = app.DeletePads(d, []proto.PadIndex{pad(t, "A1"), pad(t, "A2")})
	snap, _ = app.Load(d)
	if len(failed) != 1 || snap.Bank(0)[0].HasSample {
		t.Errorf("delete: failed %v, A1 has sample %v", failed, snap.Bank(0)[0].HasSample)
	}

	if err := app.ShowBank(d, 4); err != nil {
		t.Fatal(err)
	}
	if bank, _ := s.Selected(); bank != 4 {
		t.Errorf("unit shows bank %d", bank)
	}
}

func TestProjectChangeIsNoticed(t *testing.T) {
	d, s := connect(t)
	if app.ProjectChanged(d.TakeEvents()) {
		t.Error("a quiet unit reported a change")
	}
	s.Notify([]byte{proto.ReplyChanged, 2, 0, 0, 0})
	d.CurrentProject()
	if !app.ProjectChanged(d.TakeEvents()) {
		t.Error("the change notification was missed")
	}
	// The two-byte acknowledgement of a project select is not a change by itself.
	if app.ProjectChanged([]proto.Message{proto.Short(proto.ChannelControl, []byte{proto.ReplyChanged, 0})}) {
		t.Error("an acknowledgement counted as a change")
	}
}

func TestPatterns(t *testing.T) {
	d, s := connect(t)
	s.SetPattern(pad(t, "B2"), pad(t, "A1"), pad(t, "C8"))
	rows, err := app.LoadPatterns(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 16 || rows[0].Exists || !rows[1].Exists || !slices.Equal(rows[1].Pads, []proto.PadIndex{pad(t, "A1"), pad(t, "C8")}) {
		t.Errorf("patterns = %+v", rows[:2])
	}
}

func names(t *app.Tree) []string {
	var out []string
	for _, n := range t.Rows() {
		out = append(out, n.Path)
	}
	return out
}

func TestTree(t *testing.T) {
	dir, file := fileapi.DtDir, fileapi.DtReg
	tree := app.NewTree(app.CardRoot, []fileapi.DirEntry{{Name: "readme.txt", Kind: file}, {Name: "IMPORT", Kind: dir}, {Name: "EXPORT", Kind: dir}})
	// Folders first, then by name.
	if got := names(tree); !slices.Equal(got, []string{"SD:EXPORT", "SD:IMPORT", "SD:readme.txt"}) {
		t.Fatalf("rows = %v", got)
	}
	tree.Expand(1, []fileapi.DirEntry{{Name: "kick.wav", Kind: file}, {Name: "pack", Kind: dir}})
	tree.Expand(2, []fileapi.DirEntry{{Name: "snare.wav", Kind: file}})
	want := []string{"SD:EXPORT", "SD:IMPORT", "SD:IMPORT/pack", "SD:IMPORT/pack/snare.wav", "SD:IMPORT/kick.wav", "SD:readme.txt"}
	if got := names(tree); !slices.Equal(got, want) {
		t.Fatalf("expanded rows = %v", got)
	}
	if tree.Parent(3) != 2 || tree.Parent(4) != 1 || tree.Parent(0) != -1 {
		t.Errorf("parents = %d %d %d", tree.Parent(3), tree.Parent(4), tree.Parent(0))
	}
	if tree.Folder(3) != "SD:IMPORT/pack" || tree.Folder(1) != "SD:IMPORT" || tree.Folder(5) != "SD:" {
		t.Errorf("folders = %q %q %q", tree.Folder(3), tree.Folder(1), tree.Folder(5))
	}
	// Closing a folder hides everything under it, however deep.
	tree.Collapse(1)
	if got := names(tree); !slices.Equal(got, []string{"SD:EXPORT", "SD:IMPORT", "SD:readme.txt"}) {
		t.Errorf("collapsed rows = %v", got)
	}

	for path, want := range map[string][2]string{
		"SD:IMPORT/pack/a.wav": {"SD:IMPORT/pack", "a.wav"},
		"SD:IMPORT":            {"SD:", "IMPORT"},
		"ROLAND/SP-404MKII":    {"ROLAND", "SP-404MKII"},
		"ROLAND":               {"", "ROLAND"},
	} {
		if app.Dir(path) != want[0] || app.Base(path) != want[1] {
			t.Errorf("%q splits into %q and %q", path, app.Dir(path), app.Base(path))
		}
	}
}

func TestMoveAndRenameOnTheCard(t *testing.T) {
	d, s := connect(t)
	s.AddFile("SD:IMPORT/kick.wav", []byte("k"))
	s.AddFile("SD:IMPORT/pack/snare.wav", []byte("s"))

	to, err := app.MoveInto(d, "SD:IMPORT/kick.wav", "SD:IMPORT/pack")
	if err != nil || to != "SD:IMPORT/pack/kick.wav" {
		t.Fatalf("moved to %q (%v)", to, err)
	}
	to, err = app.RenamePath(d, to, "bd.wav")
	if err != nil || to != "SD:IMPORT/pack/bd.wav" {
		t.Fatalf("renamed to %q (%v)", to, err)
	}
	if data, ok := s.File("SD:IMPORT/pack/bd.wav"); !ok || string(data) != "k" {
		t.Errorf("file after move and rename: %q, %v", data, ok)
	}
	if _, err := app.MoveInto(d, "SD:IMPORT/pack/bd.wav", "SD:IMPORT/pack"); err == nil {
		t.Error("moving a file onto itself succeeded")
	}
}

func TestTreeSiblingsAndChildren(t *testing.T) {
	dir, file := fileapi.DtDir, fileapi.DtReg
	tree := app.NewTree(app.CardRoot, []fileapi.DirEntry{{Name: "readme.txt", Kind: file}, {Name: "IMPORT", Kind: dir}, {Name: "EXPORT", Kind: dir}})
	tree.Expand(1, []fileapi.DirEntry{{Name: "kick.wav", Kind: file}, {Name: "pack", Kind: dir}})
	tree.Expand(2, []fileapi.DirEntry{{Name: "snare.wav", Kind: file}})
	// Rows: EXPORT, IMPORT, pack, snare.wav, kick.wav, readme.txt.
	if got := tree.Siblings(4); !slices.Equal(got, []int{2, 4}) {
		t.Errorf("siblings of kick.wav = %v", got)
	}
	if got := tree.Siblings(0); !slices.Equal(got, []int{0, 1, 5}) {
		t.Errorf("siblings of EXPORT = %v (what is inside IMPORT is not one)", got)
	}
	if got := tree.Siblings(3); !slices.Equal(got, []int{3}) {
		t.Errorf("siblings of snare.wav = %v", got)
	}
	if got := tree.Children(1); !slices.Equal(got, []int{2, 4}) {
		t.Errorf("children of IMPORT = %v", got)
	}
	if got := tree.Children(0); got != nil {
		t.Errorf("children of a closed folder = %v", got)
	}
	if tree.Siblings(-1) != nil || tree.Siblings(9) != nil {
		t.Error("no such row, no siblings")
	}
}

func TestIndexOfAllHoldsTheFilesOfEverySource(t *testing.T) {
	ix := app.IndexOfAll(&app.TagIndex{Files: map[string]string{
		"SD:IMPORT/a.wav":         "a",
		"/Volumes/X/Kit/b.wav":    "b",
		"ROLAND/SP-404MKII/c.wav": "c",
	}})
	if len(ix.Files) != 3 {
		t.Fatalf("%d files, want 3", len(ix.Files))
	}
	for _, f := range ix.Files {
		if f.Path == "" || f.Rel != f.Path {
			t.Errorf("bad entry %+v", f)
		}
	}
	if got := app.IndexOfAll(nil); len(got.Files) != 0 {
		t.Error("a missing index has files")
	}
}

func TestTheBusRuleSendsDrumsToOneBusAndTheRestToAnother(t *testing.T) {
	r := app.DefaultBusRule()
	drums := []string{"Kick_125_Cm", "14_Kick_125_G_Klezmer", "09_OHat_125_G", "10_Perc1_125", "HTHF_125_Percussion_Loop", "12_Clap_125", "DrumLoop_124", "snare 03", "hihat_closed", "TomHigh"}
	others := []string{"01_Bass1_125_G", "02_Synth_125", "06_Lead_125", "07_FX1_125", "ptn I4 mix", "vocal chop", "Pad_warm", "pastrami"}
	for _, n := range drums {
		if !r.IsDrum(n) || r.BusFor(n) != 2 {
			t.Errorf("%q is not a drum pad going to BUS 2", n)
		}
	}
	for _, n := range others {
		if r.IsDrum(n) || r.BusFor(n) != 1 {
			t.Errorf("%q is a drum pad", n)
		}
	}
	// The words are the user's: a custom list and other buses.
	r = app.BusRule{DrumWords: []string{"boom"}, DrumBus: 1, OtherBus: 0}
	if r.BusFor("boom_01") != 1 || r.BusFor("kick") != 0 {
		t.Error("the custom words and buses are not used")
	}
}
