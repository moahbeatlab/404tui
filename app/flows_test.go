package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func wav(frames int, seed int16) []byte {
	s := formats.Sample{Channels: 1, Samples: make([]int16, frames)}
	for i := range s.Samples {
		s.Samples[i] = seed + int16(i%50)
	}
	return s.WAV()
}

func TestAssignFromTheCardAndTheComputer(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouter(d)
	s.AddFile("SD:IMPORT/pack/Kick Drum 01.wav", wav(4800, 100))
	dir := t.TempDir()
	local := filepath.Join(dir, "snare.wav")
	if err := os.WriteFile(local, wav(2400, 200), 0o644); err != nil {
		t.Fatal(err)
	}

	name, err := app.Assign(d, router, pad(t, "C5"), "SD:IMPORT/pack/Kick Drum 01.wav")
	if err != nil || name != "Kick Drum 01" {
		t.Fatalf("assigned %q (%v)", name, err)
	}
	if _, err := app.Assign(d, router, pad(t, "C6"), local); err != nil {
		t.Fatal(err)
	}
	if s.PadName(pad(t, "C5")) != "Kick Drum 01" || s.PadName(pad(t, "C6")) != "snare" {
		t.Errorf("pads are named %q and %q", s.PadName(pad(t, "C5")), s.PadName(pad(t, "C6")))
	}
	stored, _ := s.File(pad(t, "C5").SamplePath(1))
	if sample, err := formats.ParseSMP(stored); err != nil || sample.Frames() != 4800 {
		t.Errorf("the unit holds %+v (%v)", sample.Frames(), err)
	}

	if _, err := app.Assign(d, router, pad(t, "C7"), "SD:IMPORT/missing.wav"); err == nil {
		t.Error("assigning a missing file succeeded")
	}
	s.AddFile("SD:IMPORT/junk.wav", []byte("not audio"))
	if _, err := app.Assign(d, router, pad(t, "C7"), "SD:IMPORT/junk.wav"); err == nil || !strings.Contains(err.Error(), "junk.wav") {
		t.Errorf("assigning junk: %v", err)
	}
}

func TestPadNames(t *testing.T) {
	for file, want := range map[string]string{
		"SD:IMPORT/kick.wav":                  "kick",
		"/Users/me/Loops/Amen Break 170.aif":  "Amen Break 170",
		"a/very long sample name indeed.flac": "very long sample",
		"a/ü.wav":                             "Sample",
		"x/..wav":                             ".",
	} {
		if got := app.PadName(file); got != want {
			t.Errorf("PadName(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestAssignManyFillsFreePads(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouter(d)
	s.SetSample(pad(t, "A2"), "held", 100)
	for _, name := range []string{"one", "two", "three"} {
		s.AddFile("SD:IMPORT/"+name+".wav", wav(480, 1))
	}
	snap, _ := app.Load(d)
	taken := func(p proto.PadIndex) bool { return snap.Pads[p.Index()].HasSample }
	pads := app.FreePads(pad(t, "A1"), 3, taken)
	if !slices.Equal(pads, []proto.PadIndex{pad(t, "A1"), pad(t, "A3"), pad(t, "A4")}) {
		t.Fatalf("free pads = %v", pads)
	}
	var seen []string
	placed := app.AssignMany(d, router, pads, []string{"SD:IMPORT/one.wav", "SD:IMPORT/two.wav", "SD:IMPORT/three.wav", "SD:IMPORT/four.wav"},
		func(i int, f string) { seen = append(seen, f) })
	if len(placed) != 4 || placed[0].Err != nil || placed[2].Pad != pad(t, "A4") || placed[3].Err == nil {
		t.Errorf("placements = %+v", placed)
	}
	if len(seen) != 3 || s.PadName(pad(t, "A2")) != "held" || s.PadName(pad(t, "A3")) != "two" {
		t.Errorf("seen %v, A2 %q, A3 %q", seen, s.PadName(pad(t, "A2")), s.PadName(pad(t, "A3")))
	}
	// The end of the last bank stops the fill.
	if got := app.FreePads(pad(t, "J15"), 5, func(proto.PadIndex) bool { return false }); len(got) != 2 {
		t.Errorf("free pads at the end = %v", got)
	}
}

func TestExportAndRestoreBringTheBankBack(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouter(d)
	s.AddFile("SD:IMPORT/kick.wav", wav(4800, 10))
	s.AddFile("SD:IMPORT/hat.wav", wav(2400, 20))
	a1, a3 := pad(t, "A1"), pad(t, "A3")
	for p, f := range map[proto.PadIndex]string{a1: "kick", a3: "hat"} {
		if _, err := app.Assign(d, router, p, "SD:IMPORT/"+f+".wav"); err != nil {
			t.Fatal(err)
		}
	}
	level, _ := proto.ParamByName("level")
	gate, _ := proto.ParamByName("gate")
	loop, _ := proto.ParamByName("loop")
	d.SetParam(level.ID, proto.TargetPad(a1), 99)
	d.SetParam(gate.ID, proto.TargetPad(a1), 1)
	d.SetParam(loop.ID, proto.TargetPad(a3), 1)
	d.SetChopPoints(a1, []uint32{0, 1200})

	pads := proto.AllPads()[:16]
	var steps int
	summary, err := app.ExportPads(d, pads, "SD:exports/Live Set-A", func(done, total int) { steps++ })
	if err != nil || summary != "2 pads exported, 14 empty, to SD:exports/Live Set-A" || steps != 16 {
		t.Fatalf("export: %q (%v), %d steps", summary, err, steps)
	}
	for _, name := range []string{"A01 kick.wav", "A03 hat.wav", app.Sidecar} {
		if _, ok := s.File("SD:exports/Live Set-A/" + name); !ok {
			t.Errorf("%s was not written", name)
		}
	}
	exported, _ := s.File("SD:exports/Live Set-A/A01 kick.wav")
	if got, err := formats.DecodeAudio(exported, "wav"); err != nil || got.Sample.Frames() != 4800 {
		t.Errorf("exported WAV: %v frames (%v)", got.Sample.Frames(), err)
	}
	if !app.HasSidecar(router, "SD:exports/Live Set-A") || app.HasSidecar(router, "SD:exports") {
		t.Error("HasSidecar misjudges the folders")
	}

	// Wipe the bank, then restore it.
	app.DeletePads(d, []proto.PadIndex{a1, a3})
	d.SetParam(level.ID, proto.TargetPad(a1), 5)
	if blocks, _ := d.PadBlocks(); blocks[0].HasSample() {
		t.Fatal("pad survived the delete")
	}
	summary, err = app.RestorePads(d, "SD:exports/Live Set-A", nil)
	if err != nil || summary != "2 pads restored from SD:exports/Live Set-A" {
		t.Fatalf("restore: %q (%v)", summary, err)
	}
	block, _ := d.PadBlock(a1)
	if v, _ := block.Param(level.ID); block.Name() != "kick" || v != 99 {
		t.Errorf("A1 after restore: %q, level %d", block.Name(), v)
	}
	if v, _ := block.Param(gate.ID); v != 1 {
		t.Errorf("A1 gate = %d", v)
	}
	if chops := block.ChopPoints(); chops[0] != 0 || chops[1] != 1200 || chops[2] >= 0 {
		t.Errorf("A1 chop points = %v", chops)
	}
	if block, _ = d.PadBlock(a3); block.Name() != "hat" {
		t.Errorf("A3 is %q", block.Name())
	}
}

func TestExportToTheComputerAndRestoreFromIt(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouter(d)
	s.AddFile("SD:IMPORT/kick.wav", wav(480, 3))
	if _, err := app.Assign(d, router, pad(t, "B1"), "SD:IMPORT/kick.wav"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "out", "bank-B")
	routed := app.NewRouted(d)
	if _, err := app.ExportPads(routed, proto.AllPads()[16:32], dir, nil); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("the folder holds %d files", len(entries))
	}
	app.DeletePads(d, []proto.PadIndex{pad(t, "B1")})
	if _, err := app.RestorePads(routed, dir, nil); err != nil {
		t.Fatal(err)
	}
	if s.PadName(pad(t, "B1")) != "kick" {
		t.Errorf("B1 is %q", s.PadName(pad(t, "B1")))
	}
}

func TestExportAndRestoreRefuse(t *testing.T) {
	d, s := connect(t)
	if _, err := app.ExportPads(d, proto.AllPads()[:16], "SD:exports/none", nil); err == nil || !strings.Contains(err.Error(), "no pads") {
		t.Errorf("exporting an empty bank: %v", err)
	}
	if _, err := app.RestorePads(d, "SD:exports/none", nil); err == nil {
		t.Error("restoring from a folder with no sidecar succeeded")
	}
	s.AddFile("SD:other/"+app.Sidecar, []byte(`{"format":"x","version":1,"project":"","pads":[]}`))
	if _, err := app.RestorePads(d, "SD:other", nil); err == nil || !strings.Contains(err.Error(), "not a pad export") {
		t.Errorf("restoring someone else's JSON: %v", err)
	}
	s.AddFile("SD:bad/"+app.Sidecar, []byte("{"))
	if _, err := app.RestorePads(d, "SD:bad", nil); err == nil {
		t.Error("restoring unreadable JSON succeeded")
	}
	if got := app.SanitizeName("Kick/Snare: 90?"); got != "Kick_Snare_ 90_" {
		t.Errorf("sanitized = %q", got)
	}
	if app.SanitizeName("   ") != "Sample" || app.SanitizeName("loop.") != "loop" {
		t.Error("sanitize edge cases")
	}
	if got := app.PadLabel(pad(t, "J16")); got != "J16" || app.PadLabel(pad(t, "C5")) != "C05" {
		t.Errorf("labels = %q %q", got, app.PadLabel(pad(t, "C5")))
	}
}

func TestPatternDetailAndRetarget(t *testing.T) {
	d, s := connect(t)
	a1, a2, b2 := pad(t, "A1"), pad(t, "A2"), pad(t, "B2")
	s.SetPattern(b2, a1, a2)
	rec := func(delta byte, pad proto.PadIndex) []byte {
		return []byte{delta, 0x2F + byte(pad.Index()%80), byte(pad.Index() / 80), 0, 0x7F, 0x40, 0x10, 0}
	}
	// Each record's delta applies after it: A1 at 0 and 240, A2 at 120, the end at 480.
	file := slices.Concat(rec(120, a1), rec(120, a2), rec(240, a1), []byte{0, 0x8C, 0, 0, 0, 0, 0, 0}, []byte{4, 0, 0, 0, 0x04, 0, 0, 0})
	s.AddFile(formats.PatternPath(1, b2), file)

	detail, err := app.OpenPattern(d, b2)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Beats != 5 || detail.Length != 480 || detail.BankTempo != 120 || !detail.HasSourceBank || detail.SourceBank != 0 {
		t.Errorf("detail = %+v", detail)
	}
	if len(detail.Grid.Rows) != 2 || detail.Grid.Steps != 16 {
		t.Fatalf("grid = %+v", detail.Grid)
	}
	if row := detail.Grid.Rows[0]; row.Pad != a1 || !row.Hits[0] || !row.Hits[8] || row.Hits[1] {
		t.Errorf("A1 row = %v", row.Hits)
	}
	if got := app.StepGlyphs(detail.Grid.Rows[1].Hits, "x", "."); got != "....  x...  ....  ...." && got != ".... x... .... ...." {
		t.Errorf("A2 glyphs = %q", got)
	}

	if _, err := app.RetargetPattern(d, b2, 0, 0); err == nil {
		t.Error("retargeting onto the same bank succeeded")
	}
	n, err := app.RetargetPattern(d, b2, 0, 3)
	if err != nil || n != 3 {
		t.Fatalf("retargeted %d (%v)", n, err)
	}
	detail, _ = app.OpenPattern(d, b2)
	if detail.SourceBank != 3 || detail.PadsUsed[0] != pad(t, "D1") {
		t.Errorf("after retarget: bank %d, pads %v", detail.SourceBank, detail.PadsUsed)
	}
	if _, err := app.RetargetPattern(d, b2, 0, 3); err == nil {
		t.Error("retargeting an already moved pattern succeeded")
	}
	if _, err := app.OpenPattern(d, pad(t, "C3")); err == nil {
		t.Error("opening a missing pattern succeeded")
	}
	if bank, ok := app.CommonBank([]proto.PadIndex{a1, b2}); ok {
		t.Errorf("pads in two banks share bank %d", bank)
	}
}

func TestLocalStorage(t *testing.T) {
	dir := t.TempDir()
	r := app.NewRouter(nil)
	if err := app.MkdirAll(r, filepath.Join(dir, "a", "b")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "a", "b", "x.wav")
	if err := r.WriteFile(file, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)
	entries, err := r.ListDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name != "a" || !entries[0].IsDir() {
		t.Errorf("listing = %+v (%v)", entries, err)
	}
	st, exists, err := r.Stat(file)
	if err != nil || !exists || st.Size != 5 || st.IsDir() {
		t.Errorf("stat = %+v %v %v", st, exists, err)
	}
	if _, exists, _ := r.Stat(file + "x"); exists {
		t.Error("a missing file exists")
	}
	head, _, _ := r.ReadPrefix(file, 3)
	if string(head) != "hel" {
		t.Errorf("prefix = %q", head)
	}
	if err := r.Rename(file, filepath.Join(dir, "a", "b", "y.wav")); err != nil {
		t.Error(err)
	}
	r.WriteFile(file, []byte("again"))
	if err := r.Rename(file, filepath.Join(dir, "a", "b", "y.wav")); err == nil {
		t.Error("renaming onto an existing file succeeded")
	}
	for path, local := range map[string]bool{"/tmp/x": true, "./x": true, "~/x": true, "SD:IMPORT": false, "ROLAND/x": false} {
		if app.IsLocal(path) != local {
			t.Errorf("IsLocal(%q) = %v", path, !local)
		}
	}
}

func TestProjectExportAndPlannedRestore(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouted(d)
	padconf := make([]byte, 0xC0)
	padconf[8] = 3
	copy(padconf[0x80:], "MY BACKUP")
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PADCONF.BIN", padconf)
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PTN/PATTERNCHAIN_00.CHN", []byte("chn"))
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN", []byte("ptn"))
	s.AddFile("ROLAND/SP-404MKII/PROJECT_01/SMPL/BANK1-01.SMP", []byte("smp"))

	if _, err := app.ExportProject(router, 5, "SD:bk", nil); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("exporting an empty project: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "backup")
	var names []string
	summary, err := app.ExportProject(router, 0, dir, func(done int, name string) { names = append(names, name) })
	if err != nil || !strings.HasPrefix(summary, "project 1 exported: 4 files") || len(names) != 4 {
		t.Fatalf("export: %q (%v) %v", summary, err, names)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "SMPL", "BANK1-01.SMP")); err != nil || string(data) != "smp" {
		t.Errorf("exported sample: %q (%v)", data, err)
	}

	// Plan from the computer: PADCONF first, then pictures, the chain before patterns, samples.
	plan, err := app.PlanProjectRestore(router, 3, dir)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, f := range plan.Files {
		order = append(order, f.Name)
	}
	want := []string{"PADCONF.BIN", "PTN/PATTERNCHAIN_00.CHN", "PTN/PTN00001.BIN", "SMPL/BANK1-01.SMP"}
	if !slices.Equal(order, want) || plan.Name != "MY BACKUP" || plan.Project != 3 || plan.Total != 0xC0+9 {
		t.Errorf("plan = %v, name %q, total %d", order, plan.Name, plan.Total)
	}
	if _, err := app.PlanProjectRestore(router, 3, filepath.Join(dir, "SMPL")); err == nil || !strings.Contains(err.Error(), "PADCONF.BIN") {
		t.Errorf("planning from a folder that is not a backup: %v", err)
	}

	// Writing selects the target project first.
	if err := app.RestoreProject(router, plan, nil); err != nil {
		t.Fatal(err)
	}
	if current, _ := d.CurrentProject(); current != 3 {
		t.Errorf("current project %d", current+1)
	}
	if data, ok := s.File("ROLAND/SP-404MKII/PROJECT_04/SMPL/BANK1-01.SMP"); !ok || string(data) != "smp" {
		t.Errorf("restored sample: %q %v", data, ok)
	}
	if err := app.RestoreProject(router, nil, nil); err == nil {
		t.Error("restoring nothing succeeded")
	}
}
