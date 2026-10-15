package app_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
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

func TestWriteStartPatternMakesAPatternTheUnitLists(t *testing.T) {
	d, s := connect(t)
	c3, pads := pad(t, "C3"), []proto.PadIndex{pad(t, "B1"), pad(t, "B4")}
	if rows, _ := app.LoadPatterns(d, 2); rows[2].Exists {
		t.Fatal("C3 holds a pattern before one is written")
	}
	if err := app.WriteStartPattern(d, c3, pads, app.StartBars); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.File(formats.PatternPath(1, c3)); !ok {
		t.Fatal("no pattern file")
	}
	rows, err := app.LoadPatterns(d, 2)
	if err != nil || !rows[2].Exists || !slices.Equal(rows[2].Pads, pads) {
		t.Fatalf("C3 = %+v (%v)", rows[2], err)
	}
	detail, err := app.OpenPattern(d, c3)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Beats != 4 || detail.Length != 3840 || detail.SourceBank != 1 || len(detail.Grid.Rows) != 2 {
		t.Errorf("detail = %+v", detail)
	}
	for _, row := range detail.Grid.Rows {
		if !row.Hits[0] || slices.Contains(row.Hits[1:], true) {
			t.Errorf("%s hits = %v", row.Pad, row.Hits)
		}
	}
	if err := app.WriteStartPattern(d, c3, nil, app.StartBars); err == nil {
		t.Error("a pattern without pads was written")
	}
}

func TestPatternGridHasSixteenStepsPerBar(t *testing.T) {
	d, _ := connect(t)
	c3, a1 := pad(t, "C3"), pad(t, "A1")
	// A 2-bar pattern with an A1 note on the first beat of each bar (tick 0 and tick 1920).
	file, err := formats.BlankPattern(2)
	if err != nil {
		t.Fatal(err)
	}
	if file, err = formats.InsertNote(file, 0, a1, 127, 120); err != nil {
		t.Fatal(err)
	}
	if file, err = formats.InsertNote(file, 1920, a1, 127, 120); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteFile(formats.PatternPath(1, c3), file); err != nil {
		t.Fatal(err)
	}
	detail, err := app.OpenPattern(d, c3)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Bars() != 2 {
		t.Errorf("Bars() = %d, want 2", detail.Bars())
	}
	if detail.Grid.Steps != 2*app.GridStepsPerBar {
		t.Errorf("Grid.Steps = %d, want %d", detail.Grid.Steps, 2*app.GridStepsPerBar)
	}
	if got := detail.StepTick(16); got != 1920 {
		t.Errorf("StepTick(16) = %d, want 1920 (start of bar 2)", got)
	}
	if got := detail.StepTicks(); got != 120 {
		t.Errorf("StepTicks() = %d, want 120 (a 16th)", got)
	}
	// The two A1 hits land on col 0 and col 16: one per bar.
	if len(detail.Grid.Rows) != 1 {
		t.Fatalf("grid rows = %d, want 1", len(detail.Grid.Rows))
	}
	if hits := detail.Grid.Rows[0].Hits; !hits[0] || !hits[16] {
		t.Errorf("A1 hits = %v, want col 0 and col 16 set", hits)
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
	plan, err := app.PlanProjectRestore(router, 3, dir, nil)
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
	if _, err := app.PlanProjectRestore(router, 3, filepath.Join(dir, "SMPL"), nil); err == nil || !strings.Contains(err.Error(), "PADCONF.BIN") {
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

func TestCopyPadsKeepsSampleAndSettings(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouted(d)
	s.AddFile("SD:IMPORT/kick.wav", wav(4800, 10))
	s.AddFile("SD:IMPORT/hat.wav", wav(2400, 20))
	a2, a3, c5 := pad(t, "A2"), pad(t, "A3"), pad(t, "C5")
	app.Assign(d, router, a2, "SD:IMPORT/kick.wav")
	app.Assign(d, router, a3, "SD:IMPORT/hat.wav")
	level, _ := proto.ParamByName("level")
	loop, _ := proto.ParamByName("loop")
	d.SetParam(level.ID, proto.TargetPad(a2), 99)
	d.SetParam(loop.ID, proto.TargetPad(a2), 1)
	d.SetChopPoints(a2, []uint32{0, 1200})

	copies, err := app.PlanCopy([]proto.PadIndex{a3, a2}, c5)
	if err != nil || len(copies) != 2 || copies[0] != (app.Copy{From: a2, To: c5}) || copies[1].To != pad(t, "C6") {
		t.Fatalf("plan = %+v (%v)", copies, err)
	}
	n, failed := app.CopyPads(d, copies)
	if n != 2 || failed != nil {
		t.Fatalf("copied %d, failed %v", n, failed)
	}
	block, _ := d.PadBlock(c5)
	if v, _ := block.Param(level.ID); block.Name() != "kick" || v != 99 {
		t.Errorf("C5: %q, level %d", block.Name(), v)
	}
	if v, _ := block.Param(loop.ID); v != 1 {
		t.Errorf("C5 loop = %d", v)
	}
	if chops := block.ChopPoints(); chops[0] != 0 || chops[1] != 1200 || chops[2] >= 0 {
		t.Errorf("C5 chop points = %v", chops)
	}
	copied, _ := s.File(c5.SamplePath(1))
	original, _ := s.File(a2.SamplePath(1))
	if !bytes.Equal(copied, original) {
		t.Error("the copied sample differs from the original")
	}
	if s.PadName(pad(t, "C6")) != "hat" || s.PadName(a2) != "kick" {
		t.Errorf("C6 %q, A2 %q", s.PadName(pad(t, "C6")), s.PadName(a2))
	}
}

func TestPlanCopyAndOverlappingCopies(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouted(d)
	for i, name := range []string{"one", "two", "three"} {
		s.AddFile("SD:IMPORT/"+name+".wav", wav(480, int16(i)))
		app.Assign(d, router, proto.AllPads()[i], "SD:IMPORT/"+name+".wav")
	}
	// Shifting A1-A3 up one pad copies A2 onto A3 while A3 is still needed for A4.
	pads := proto.AllPads()[:3]
	copies, _ := app.PlanCopy(pads, pad(t, "A2"))
	if n, failed := app.CopyPads(d, copies); n != 3 || failed != nil {
		t.Fatalf("copied %d, failed %v", n, failed)
	}
	for pad, want := range map[string]string{"A1": "one", "A2": "one", "A3": "two", "A4": "three"} {
		if got := s.PadName(proto.AllPads()[map[string]int{"A1": 0, "A2": 1, "A3": 2, "A4": 3}[pad]]); got != want {
			t.Errorf("%s is %q, want %q", pad, got, want)
		}
	}

	if _, err := app.PlanCopy(pads, pad(t, "J15")); err == nil || !strings.Contains(err.Error(), "past J16") {
		t.Errorf("a plan past the end: %v", err)
	}
	if copies, _ := app.PlanCopy(pads[:1], pad(t, "A1")); len(copies) != 0 {
		t.Errorf("copying a pad onto itself planned %v", copies)
	}
	if _, err := app.PlanCopy(nil, pad(t, "A1")); err == nil {
		t.Error("planning nothing succeeded")
	}
	// An empty source fails alone and the rest still copy.
	copies = []app.Copy{{From: pad(t, "B1"), To: pad(t, "B2")}, {From: pad(t, "A1"), To: pad(t, "B3")}}
	if n, failed := app.CopyPads(d, copies); n != 1 || len(failed) != 1 || !strings.Contains(failed[0], "no sample") {
		t.Errorf("copied %d, failed %v", n, failed)
	}
}

func TestBankTempoAndVolume(t *testing.T) {
	d, _ := connect(t)
	snap, err := app.Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Banks[2].Tempo != 120 || snap.Banks[2].Volume != 127 || snap.UsesProjectTempo {
		t.Errorf("defaults: %+v project tempo %v", snap.Banks[2], snap.UsesProjectTempo)
	}
	if err := app.SetBankTempo(d, 2, 98.5); err != nil {
		t.Fatal(err)
	}
	// The unit stores the volume as 127 minus what is shown; it comes back as set.
	if err := app.SetBankVolume(d, 2, 90); err != nil {
		t.Fatal(err)
	}
	snap, _ = app.Load(d)
	if snap.Banks[2].Tempo != 98.5 || snap.Banks[2].Volume != 90 {
		t.Errorf("bank C: %+v", snap.Banks[2])
	}
	if snap.Banks[1].Tempo != 120 || snap.Banks[3].Volume != 127 {
		t.Errorf("another bank changed: %+v %+v", snap.Banks[1], snap.Banks[3])
	}
	for _, bad := range []float64{39.9, 200.1, -1} {
		if err := app.SetBankTempo(d, 0, bad); err == nil {
			t.Errorf("tempo %v accepted", bad)
		}
	}
	for _, bad := range []int{-1, 128} {
		if err := app.SetBankVolume(d, 0, bad); err == nil {
			t.Errorf("volume %d accepted", bad)
		}
	}
	if err := app.SetBankTempo(d, 10, 100); err == nil {
		t.Error("bank 11 accepted")
	}
	if err := app.SetBankVolume(d, 9, 0); err != nil {
		t.Error(err)
	}
	if snap, _ = app.Load(d); snap.Banks[9].Volume != 0 {
		t.Errorf("bank J volume %d", snap.Banks[9].Volume)
	}
}

func TestIndexSearchAcrossFolders(t *testing.T) {
	d, s := connect(t)
	for _, f := range []string{
		"SD:IMPORT/Drums/808 Kick.wav", "SD:IMPORT/Drums/Snare.wav", "SD:IMPORT/Drums/hats/Closed Hat.aif",
		"SD:IMPORT/Kicks/deep.flac", "SD:IMPORT/notes.txt", "SD:EXPORT/old kick take 2.wav",
	} {
		s.AddFile(f, []byte("x"))
	}
	ix, err := app.BuildIndex(app.NewRouted(d), app.CardRoot, app.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Files) != 5 || ix.Truncated {
		t.Fatalf("indexed %d files, truncated %v", len(ix.Files), ix.Truncated)
	}
	rels := func(q string) []string {
		var out []string
		for _, f := range ix.Search(q, 0) {
			out = append(out, f.Rel)
		}
		return out
	}
	// A match in the file's own name comes before one only in a folder name.
	if got := rels("kick"); !slices.Equal(got, []string{"EXPORT/old kick take 2.wav", "IMPORT/Drums/808 Kick.wav", "IMPORT/Kicks/deep.flac"}) {
		t.Errorf("kick: %v", got)
	}
	// Every word has to match, case ignored, folders included.
	if got := rels("DRUMS hat"); !slices.Equal(got, []string{"IMPORT/Drums/hats/Closed Hat.aif"}) {
		t.Errorf("drums hat: %v", got)
	}
	if got := rels("  "); got != nil {
		t.Errorf("an empty query listed %v", got)
	}
	if got := rels("nothing"); len(got) != 0 {
		t.Errorf("nothing: %v", got)
	}
	if got := ix.Search("kick", 2); len(got) != 2 {
		t.Errorf("limit: %d", len(got))
	}
	if !slices.Equal(app.Ancestors(app.CardRoot, "SD:IMPORT/Drums/hats/Closed Hat.aif"), []string{"SD:IMPORT", "SD:IMPORT/Drums", "SD:IMPORT/Drums/hats"}) {
		t.Errorf("ancestors: %v", app.Ancestors(app.CardRoot, "SD:IMPORT/Drums/hats/Closed Hat.aif"))
	}

	// Limits stop the walk and say so.
	small, _ := app.BuildIndex(app.NewRouted(d), app.CardRoot, app.IndexLimits{MaxFolders: 2, MaxFiles: 100, MaxDepth: 10})
	if !small.Truncated {
		t.Error("a walk cut short by MaxFolders was not marked truncated")
	}
	shallow, _ := app.BuildIndex(app.NewRouted(d), app.CardRoot, app.IndexLimits{MaxFolders: 100, MaxFiles: 100, MaxDepth: 1})
	if !shallow.Truncated || len(shallow.Files) >= 5 {
		t.Errorf("depth limit: %d files, truncated %v", len(shallow.Files), shallow.Truncated)
	}
	if _, err := app.BuildIndex(app.NewRouted(d), "SD:nowhere", app.DefaultLimits); err == nil {
		t.Error("indexing a missing folder succeeded")
	}
}

func TestPadFactsAndPatternUsage(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouted(d)
	s.AddFile("SD:IMPORT/loop.wav", func() []byte {
		x := formats.Sample{Channels: 2, Samples: make([]int16, 2*24000)}
		return x.WAV()
	}())
	a5 := pad(t, "A5")
	if _, err := app.Assign(d, router, a5, "SD:IMPORT/loop.wav"); err != nil {
		t.Fatal(err)
	}
	snap, _ := app.Load(d)
	row := snap.Pads[a5.Index()]
	if row.Params == nil || len(row.ChopFrames) != 0 || row.StartBytes != 512 {
		t.Fatalf("row = %+v", row)
	}
	if _, ok := row.Params["level"]; !ok {
		t.Errorf("params = %v", row.Params)
	}
	facts, err := app.ReadPadFacts(router, row)
	if err != nil {
		t.Fatal(err)
	}
	// 24000 stereo frames at 48 kHz: half a second, the whole sample selected.
	if facts.Channels != 2 || facts.Frames != 24000 || facts.Seconds != 0.5 || facts.File != "BANK1-05.SMP" ||
		facts.StartFrame != 0 || facts.EndFrame != 24000 {
		t.Errorf("facts = %+v", facts)
	}
	if _, err := app.ReadPadFacts(router, snap.Pads[0]); err == nil {
		t.Error("facts of a pad with no sample succeeded")
	}

	s.SetPattern(pad(t, "B2"), a5, pad(t, "A1"))
	s.SetPattern(pad(t, "C3"), a5)
	usage, err := app.PatternUsage(d)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(usage[a5], []proto.PadIndex{pad(t, "B2"), pad(t, "C3")}) || !slices.Equal(usage[pad(t, "A1")], []proto.PadIndex{pad(t, "B2")}) || len(usage[pad(t, "A2")]) != 0 {
		t.Errorf("usage = %v", usage)
	}
}

func TestStartPreviewSelectsFirstAndSurvivesTheWrongPadQuirk(t *testing.T) {
	d, s := connect(t)
	a1, a6 := pad(t, "A1"), pad(t, "A6")
	s.WrongPadOnFirstPreview = true
	s.Select(0, 0) // the unit has A1 selected

	var notes []string
	logf := func(msg string, args ...any) { notes = append(notes, msg) }
	// Selecting the pad first means the quirk never happens: A6 sounds on the first try.
	if err := app.StartPreview(d, a6, logf); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.PreviewStarts, []uint16{a6.Index()}) || len(s.PreviewStops) != 0 {
		t.Errorf("starts %v, stops %v (the quirk should not have been triggered)", s.PreviewStarts, s.PreviewStops)
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "selecting the pad") {
		t.Errorf("log = %v", notes)
	}
	if bank, p := s.Selected(); bank != 0 || p != 5 {
		t.Errorf("selected %d %d", bank, p)
	}
	// The same pad again needs no selecting.
	notes = nil
	s.PreviewStarts = nil
	if err := app.StartPreview(d, a6, logf); err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 || len(s.PreviewStarts) != 1 {
		t.Errorf("log %v, starts %v", notes, s.PreviewStarts)
	}
	_ = a1
}

// If selecting first does not help, the notification that says which pad sounded catches it.
func TestStartPreviewRetriesWhenTheWrongPadSounded(t *testing.T) {
	d, s := connect(t)
	a1, a6 := pad(t, "A1"), pad(t, "A6")
	s.WrongPadOnFirstPreview = true
	s.Select(0, 0)
	// A previewer whose SelectPad does nothing, as if the unit ignored it.
	p := &blindSelect{Previewer: d}
	var notes []string
	if err := app.StartPreview(p, a6, func(msg string, args ...any) { notes = append(notes, msg) }); err != nil {
		t.Fatal(err)
	}
	// The first start made A1 sound; it was stopped and A6 started again.
	if !slices.Equal(s.PreviewStarts, []uint16{a6.Index(), a6.Index()}) || !slices.Equal(s.PreviewStops, []uint16{a1.Index()}) {
		t.Errorf("starts %v, stops %v", s.PreviewStarts, s.PreviewStops)
	}
	found := false
	for _, n := range notes {
		found = found || strings.Contains(n, "another pad than asked for")
	}
	if !found {
		t.Errorf("the retry was not logged: %v", notes)
	}
}

type blindSelect struct{ app.Previewer }

func (blindSelect) SelectPad(proto.PadIndex) error { return nil }

func TestRestoreAnExportOntoAnotherBank(t *testing.T) {
	d, s := connect(t)
	router := app.NewRouted(d)
	s.AddFile("SD:IMPORT/kick.wav", wav(480, 3))
	s.AddFile("SD:IMPORT/hat.wav", wav(240, 9))
	level, _ := proto.ParamByName("level")
	c1, c5 := pad(t, "C1"), pad(t, "C5")
	app.Assign(d, router, c1, "SD:IMPORT/kick.wav")
	app.Assign(d, router, c5, "SD:IMPORT/hat.wav")
	d.SetParam(level.ID, proto.TargetPad(c5), 77)
	if _, err := app.ExportPads(router, proto.AllPads()[32:48], "SD:exports/C-bank", nil); err != nil {
		t.Fatal(err)
	}

	plan, err := app.PlanPadRestore(router, "SD:exports/C-bank")
	if err != nil {
		t.Fatal(err)
	}
	if bank, ok := plan.SingleBank(); !ok || bank != 2 || len(plan.Pads) != 2 || plan.Pads[1].Name != "hat" {
		t.Fatalf("plan = %+v", plan)
	}
	targets, err := plan.Targets(6) // bank G
	if err != nil || targets[0] != pad(t, "G1") || targets[1] != pad(t, "G5") {
		t.Fatalf("targets = %v (%v)", targets, err)
	}
	summary, err := app.RestorePadsTo(router, plan, 6, nil)
	if err != nil || summary != "2 pads restored onto bank G from SD:exports/C-bank" {
		t.Fatalf("restore: %q (%v)", summary, err)
	}
	block, _ := d.PadBlock(pad(t, "G5"))
	if v, _ := block.Param(level.ID); block.Name() != "hat" || v != 77 {
		t.Errorf("G5: %q, level %d", block.Name(), v)
	}
	if s.PadName(pad(t, "G1")) != "kick" || s.PadName(c5) != "hat" {
		t.Errorf("G1 %q, C5 %q (the original bank is left as it was)", s.PadName(pad(t, "G1")), s.PadName(c5))
	}

	// -1 puts every pad back where it was.
	app.DeletePads(d, []proto.PadIndex{c1, c5})
	if _, err := app.RestorePadsTo(router, plan, -1, nil); err != nil || s.PadName(c1) != "kick" {
		t.Errorf("restore in place: %v, C1 %q", err, s.PadName(c1))
	}

	// An export that spans banks can only go back where it was.
	app.Assign(d, router, pad(t, "A1"), "SD:IMPORT/kick.wav")
	if _, err := app.ExportPads(router, []proto.PadIndex{pad(t, "A1"), c1}, "SD:exports/two", nil); err != nil {
		t.Fatal(err)
	}
	spans, err := app.PlanPadRestore(router, "SD:exports/two")
	if err != nil {
		t.Fatal(err)
	}
	if _, single := spans.SingleBank(); single || len(spans.Banks) != 2 {
		t.Errorf("banks = %v", spans.Banks)
	}
	if _, err := spans.Targets(3); err == nil || !strings.Contains(err.Error(), "spans banks AC") {
		t.Errorf("remapping a two-bank export: %v", err)
	}
	if _, err := app.PlanPadRestore(router, "SD:exports/nothing"); err == nil {
		t.Error("planning from a folder with no export succeeded")
	}
}

func TestBlankPlaceholders(t *testing.T) {
	quiet := formats.Sample{Channels: 1, Samples: make([]int16, 480)}
	loud := formats.Sample{Channels: 1, Samples: []int16{0, 900, -900}}
	for _, c := range []struct {
		file   string
		sample formats.Sample
		want   bool
	}{
		{"SD:kit/05_EMPTY.wav", quiet, true},
		{"SD:kit/05_empty.wav", formats.Sample{Channels: 1}, true},
		{"SD:kit/05_EMPTY.wav", loud, false},
		{"SD:kit/03_Loop1.wav", quiet, false},
	} {
		if got := app.IsBlankPlaceholder(c.file, c.sample); got != c.want {
			t.Errorf("%s: %v, want %v", c.file, got, c.want)
		}
	}
}

func TestNumberedPadsFollowTheNumber(t *testing.T) {
	names := []string{"00_DrumLoop_x.wav", "01_EMPTY.wav", "02_EMPTY.wav", "05_EMPTY.wav", "09_OHat_x.wav", "10_Perc1_x.wav", "14_Kick_x.wav", "16_EMPTY.wav"}
	var files []string
	for _, n := range names {
		files = append(files, "SD:kit/"+n)
	}
	pads, ok := app.NumberedPads(files, 0, func(proto.PadIndex) bool { return false })
	if !ok {
		t.Fatal("numbered names were not recognised")
	}
	want := map[string]string{"09_OHat_x.wav": "A9", "10_Perc1_x.wav": "A10", "14_Kick_x.wav": "A14", "16_EMPTY.wav": "A16", "00_DrumLoop_x.wav": "A1"}
	for i, n := range names {
		if w, has := want[n]; has && pads[i].String() != w {
			t.Errorf("%s on %s, want %s", n, pads[i], w)
		}
	}
	if _, ok := app.NumberedPads([]string{"SD:a/09_x.wav", "SD:a/snare.wav"}, 0, func(proto.PadIndex) bool { return false }); ok {
		t.Error("a file without a number was taken as numbered")
	}
	if pads, _ := app.NumberedPads([]string{"SD:a/03_x.wav"}, 2, func(proto.PadIndex) bool { return false }); pads[0].String() != "C3" {
		t.Errorf("bank C: %s", pads[0])
	}
}

func TestTemposFromNames(t *testing.T) {
	for name, want := range map[string]float64{
		"09_OHat_124_HDK_02.wav": 124, "124_Kit02": 124, "loop 90bpm.wav": 90, "Kick_808.wav": 0,
		"05_Snare.wav": 0, "x_44100.wav": 0, "03_Loop1_124_HDK_02.wav": 124,
	} {
		if got, ok := app.TempoFromName(name); got != want || ok != (want != 0) {
			t.Errorf("%s: %v %v, want %v", name, got, ok, want)
		}
	}
	files := []string{"SD:k/124_Kit02/00_Loop_124_a.wav", "SD:k/124_Kit02/05_EMPTY.wav", "SD:k/124_Kit02/09_OHat_124_a.wav"}
	plan := app.PlanTempos(files, true)
	if !plan.HasBank || plan.Bank != 124 || len(plan.Files) != 2 || plan.Steps() != 3 {
		t.Errorf("plan %+v", plan)
	}
	if plan := app.PlanTempos([]string{"SD:k/a_90.wav", "SD:k/b_124.wav"}, true); plan.HasBank {
		t.Error("files that disagree set a bank tempo")
	}
	if plan := app.PlanTempos([]string{"SD:k/90_Kit/01_kick.wav"}, true); !plan.HasBank || plan.Bank != 90 {
		t.Errorf("the folder name was not used: %+v", plan)
	}
}

func TestMixPatternPlacesNotesAtTheirTicks(t *testing.T) {
	a1, _ := proto.NewPadIndex(0)
	click := formats.Sample{Channels: 1, Samples: []int16{1000, 1000}}
	p := formats.Pattern{Length: formats.PPQ, Events: []formats.Event{{Tick: 0, Pad: a1, Velocity: 127}, {Tick: formats.PPQ / 2, Pad: a1, Velocity: 127}}}
	mix, err := app.MixPattern(p, 120, func(proto.PadIndex) (formats.Sample, error) { return click, nil })
	if err != nil {
		t.Fatal(err)
	}
	// One beat at 120 BPM is half a second; the second note starts a quarter second in.
	if mix.Channels != 2 || mix.Frames() != 24000 {
		t.Errorf("%d frames", mix.Frames())
	}
	at := 12000
	if mix.Samples[0] != 1000 || mix.Samples[at*2] != 1000 || mix.Samples[(at-1)*2] != 0 {
		t.Errorf("notes are not where their ticks put them")
	}
}

func TestTagsFromNamesAndFolders(t *testing.T) {
	got := app.TagsFor("SD:Samples/Synth/House/124_Bass_Wobble1.wav", "SD:Samples")
	want := []string{"synth", "house", "124", "bass", "wobble1", "wobble"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("tags %v, want %v", got, want)
	}
	got = app.TagsFor("SD:Kits/124_Kit05/09_CHat_124_HDK_05.wav", "SD:")
	for _, w := range []string{"124", "chat", "hat"} {
		if !slices.Contains(got, w) {
			t.Errorf("%v lacks %q", got, w)
		}
	}
	if slices.Contains(got, "09") || slices.Contains(got, "05") {
		t.Errorf("counting numbers kept: %v", got)
	}
	if slices.Contains(app.TagsFor("SD:a/Cmin_pad.wav", "SD:"), "cmin") == false || !app.IsKeyTag("f#m") {
		t.Error("keys are not tagged")
	}
}

func TestTagsAreCleanedUp(t *testing.T) {
	has := func(path string, want ...string) {
		t.Helper()
		got := app.TagsFor(path, "SD:")
		for _, w := range want {
			if !slices.Contains(got, w) {
				t.Errorf("%s: %v lacks %q", path, got, w)
			}
		}
	}
	lacks := func(path string, not ...string) {
		t.Helper()
		got := app.TagsFor(path, "SD:")
		for _, w := range not {
			if slices.Contains(got, w) {
				t.Errorf("%s: %v has %q", path, got, w)
			}
		}
	}
	has("SD:IMPORT/09 tech house/h2-house-chords-2/Chord One Shots/Fs_035.wav", "oneshot", "f#", "chords", "tech", "house")
	lacks("SD:IMPORT/09 tech house/h2-house-chords-2/Chord One Shots/Fs_035.wav", "import", "h2", "one", "shots", "fs")
	has("SD:IMPORT/_collections/bass_loops/x.wav", "loop", "bass")
	lacks("SD:IMPORT/_collections/bass_loops/x.wav", "loops", "collections")
	has("SD:IMPORT/h/Hits/crash_09.wav", "oneshot", "crash")
	lacks("SD:IMPORT/x/h2-core_series_vol2/128_Kit01.wav", "h2", "vol2", "kit01")
	has("SD:IMPORT/x/h2-core_series_vol2/128_Kit01.wav", "128", "core", "series")
	// A song's name is an artist and a title: only its folders are tags.
	has("SD:IMPORT/Songs/Defected/Daft Punk -- One More Time.mp3", "songs", "defected")
	lacks("SD:IMPORT/Songs/Defected/Daft Punk -- One More Time.mp3", "daft", "punk", "time")
}

func TestIndexTagsWalksAFolderAndKeepsTheRest(t *testing.T) {
	tree := map[string][]fileapi.DirEntry{
		"SD:Packs":       {{Name: "House", Kind: fileapi.DtDir}, {Name: "readme.txt"}},
		"SD:Packs/House": {{Name: "124_bass_a.wav"}, {Name: "snare.wav"}},
	}
	list := func(dir string) ([]fileapi.DirEntry, error) { return tree[dir], nil }
	ix := app.NewTagIndex()
	ix.Files["SD:Other/old.wav"] = "old"
	ix.Files["SD:Packs/House/gone.wav"] = "stale"
	var last app.TagProgress
	n, done, err := app.IndexTags(list, "SD:Packs", ix, app.DefaultLimits, func(p app.TagProgress) { last = p }, nil)
	if err != nil || !done || n != 2 {
		t.Fatalf("%d files, done %v, %v", n, done, err)
	}
	if last.Folders != 2 || last.Queued != 0 {
		t.Errorf("progress %+v", last)
	}
	if _, stale := ix.Files["SD:Packs/House/gone.wav"]; stale {
		t.Error("a file that is gone stayed in the index")
	}
	if ix.Files["SD:Other/old.wav"] != "old" {
		t.Error("a folder that was not walked lost its entries")
	}
	if hits := ix.Search("bass 124"); len(hits) != 1 || hits[0] != "SD:Packs/House/124_bass_a.wav" {
		t.Errorf("search %v", hits)
	}
	if len(ix.Folders) != 1 || ix.Folders[0] != "SD:Packs" {
		t.Errorf("folders %v", ix.Folders)
	}
}

func TestAFolderTheUnitFailsOnIsTriedAgainAndNeverEmptiesTheIndex(t *testing.T) {
	calls := map[string]int{}
	list := func(dir string) ([]fileapi.DirEntry, error) {
		calls[dir]++
		switch dir {
		case "SD:Packs":
			return []fileapi.DirEntry{{Name: "Flaky", Kind: fileapi.DtDir}, {Name: "Dead", Kind: fileapi.DtDir}}, nil
		case "SD:Packs/Flaky":
			if calls[dir] < 3 {
				return nil, errors.New("media busy")
			}
			return []fileapi.DirEntry{{Name: "a.wav"}}, nil
		}
		return nil, errors.New("media busy")
	}
	ix := app.NewTagIndex()
	ix.Files["SD:Packs/Dead/old.wav"] = "old"
	var last app.TagProgress
	n, done, err := app.IndexTags(list, "SD:Packs", ix, app.DefaultLimits, func(p app.TagProgress) { last = p }, nil)
	if err != nil || !done || n != 1 {
		t.Fatalf("%d files, done %v, %v", n, done, err)
	}
	if calls["SD:Packs/Flaky"] != 3 {
		t.Errorf("the flaky folder was read %d times", calls["SD:Packs/Flaky"])
	}
	if last.Unreadable != 1 {
		t.Errorf("unreadable %d", last.Unreadable)
	}
	if ix.Files["SD:Packs/Dead/old.wav"] != "old" {
		t.Error("what was known of a folder that failed was dropped")
	}
}

func TestTagsAreWorkedOutAgainWhenTheIndexIsLoaded(t *testing.T) {
	ix := app.NewTagIndex()
	ix.Folders = []string{"SD:IMPORT"}
	ix.Files["SD:IMPORT/Kit/_Loops/128_Bass.wav"] = "old tags from an old version"
	ix.Files["SD:IMPORT/Kit/05_EMPTY.wav"] = "empty"
	ix.Files["SD:ELSEWHERE/a.wav"] = "kept as it is"
	ix.Retag()
	if got := ix.Files["SD:IMPORT/Kit/_Loops/128_Bass.wav"]; got != "kit loops loop 128 bass" && got != "loop 128 bass kit" && !strings.Contains(got, "128") {
		t.Errorf("tags %q", got)
	}
	if strings.Contains(ix.Files["SD:IMPORT/Kit/_Loops/128_Bass.wav"], "old") {
		t.Error("the old tags stayed")
	}
	if _, ok := ix.Files["SD:IMPORT/Kit/05_EMPTY.wav"]; ok {
		t.Error("a placeholder stayed in the index")
	}
	if ix.Files["SD:ELSEWHERE/a.wav"] != "kept as it is" {
		t.Error("a file outside the indexed folders was changed")
	}
}

func TestSavedIndexIsSmallAndGivesTheSearchIndexBack(t *testing.T) {
	s := &memStorage{files: map[string][]byte{}}
	ix := app.NewTagIndex()
	ix.Folders = []string{"SD:Packs"}
	for i := 0; i < 500; i++ {
		ix.Files[fmt.Sprintf("SD:Packs/House/124_Kit%03d/%02d_bass_loop_124.wav", i/20, i%20)] = "x"
	}
	if err := ix.Save(s); err != nil {
		t.Fatal(err)
	}
	if n := len(s.files[app.TagIndexPath]); n == 0 || n > 6000 {
		t.Errorf("the saved index is %d bytes for 500 files", n)
	}
	back, err := app.LoadTagIndex(s)
	if err != nil || len(back.Files) != 500 || back.Updated == "" {
		t.Fatalf("%d files back, updated %q, %v", len(back.Files), back.Updated, err)
	}
	if hits := app.IndexFromTags(back, "SD:Packs").Search("bass 124", 10); len(hits) != 10 {
		t.Errorf("search found %d", len(hits))
	}
	if got := app.IndexFromTags(back, "SD:Packs"); got.Saved != back.Updated || got.Folders != 25 {
		t.Errorf("saved %q, folders %d", got.Saved, got.Folders)
	}
}

// memStorage is a card in memory, enough for the index file.
type memStorage struct{ files map[string][]byte }

func (m *memStorage) ListDir(string) ([]fileapi.DirEntry, error) { return nil, nil }
func (m *memStorage) Rename(from, to string) error {
	m.files[to] = m.files[from]
	delete(m.files, from)
	return nil
}
func (m *memStorage) ReadFile(p string) ([]byte, error)  { return m.files[p], nil }
func (m *memStorage) WriteFile(p string, d []byte) error { m.files[p] = d; return nil }
func (m *memStorage) Mkdir(string) error                 { return nil }
func (m *memStorage) Unlink(p string) error              { delete(m.files, p); return nil }
func (m *memStorage) Rmdir(string) error                 { return nil }
func (m *memStorage) Stat(p string) (fileapi.FileStat, bool, error) {
	_, ok := m.files[p]
	return fileapi.FileStat{}, ok, nil
}
func (m *memStorage) ReadPrefix(p string, n uint32) ([]byte, bool, error) {
	d, ok := m.files[p]
	return d[:min(len(d), int(n))], ok, nil
}

func TestUserTagsAreKeptApartAndSurviveIndexing(t *testing.T) {
	ix := app.NewTagIndex()
	ix.Folders = []string{"SD:Packs"}
	ix.Files["SD:Packs/a/kick.wav"] = "a kick"
	ix.Files["SD:Packs/a/snare.wav"] = "a snare"
	if n := ix.AddUserTags([]string{"SD:Packs/a/kick.wav", "SD:Elsewhere/wow.wav"}, []string{"Favourite", "very punchy"}); n != 2 {
		t.Fatalf("%d files changed", n)
	}
	if got := ix.Files["SD:Packs/a/kick.wav"]; !strings.Contains(got, "favourite") || !strings.Contains(got, "very-punchy") || !strings.Contains(got, "kick") {
		t.Errorf("tags %q", got)
	}
	// A file outside the indexed folders can be found by what was given it.
	if hits := ix.Search("favourite"); len(hits) != 2 {
		t.Errorf("found %v", hits)
	}
	// Indexing again works the names out again and keeps what the user added.
	list := func(dir string) ([]fileapi.DirEntry, error) {
		if dir == "SD:Packs" {
			return []fileapi.DirEntry{{Name: "a", Kind: fileapi.DtDir}}, nil
		}
		return []fileapi.DirEntry{{Name: "kick.wav"}, {Name: "snare.wav"}}, nil
	}
	if _, _, err := app.IndexTags(list, "SD:Packs", ix, app.DefaultLimits, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !ix.HasUserTag("SD:Packs/a/kick.wav", "favourite") || !strings.Contains(ix.Files["SD:Packs/a/kick.wav"], "favourite") {
		t.Errorf("a tag was lost by indexing: %q", ix.Files["SD:Packs/a/kick.wav"])
	}
	// They are saved and read back, and removing one takes it off the file.
	s := &memStorage{files: map[string][]byte{}}
	if err := ix.Save(s); err != nil {
		t.Fatal(err)
	}
	back, err := app.LoadTagIndex(s)
	if err != nil || !back.HasUserTag("SD:Elsewhere/wow.wav", "favourite") {
		t.Fatalf("not read back: %v", err)
	}
	if n := back.RemoveUserTags([]string{"SD:Packs/a/kick.wav"}, []string{"favourite"}); n != 1 {
		t.Errorf("%d removed", n)
	}
	if got := back.Files["SD:Packs/a/kick.wav"]; strings.Contains(got, "favourite") || !strings.Contains(got, "very-punchy") || !strings.Contains(got, "kick") {
		t.Errorf("after removing: %q", got)
	}
	if app.CleanTag("  Fav Sound! ") != "fav-sound" {
		t.Errorf("clean tag %q", app.CleanTag("  Fav Sound! "))
	}
}

func TestCloneSharesNothingWithTheIndexOnShow(t *testing.T) {
	ix := app.NewTagIndex()
	ix.Folders = []string{"SD:Packs"}
	ix.Files["SD:Packs/a.wav"] = "a"
	ix.AddUserTags([]string{"SD:Packs/a.wav"}, []string{"favourite"})
	c := ix.Clone()
	c.AddUserTags([]string{"SD:Packs/a.wav"}, []string{"loopy"})
	c.RemoveUserTags([]string{"SD:Packs/a.wav"}, []string{"favourite"})
	c.Files["SD:Packs/new.wav"] = "new"
	c.Folders[0] = "SD:Changed"
	if !ix.HasUserTag("SD:Packs/a.wav", "favourite") || ix.HasUserTag("SD:Packs/a.wav", "loopy") {
		t.Errorf("the original was changed through the copy: %v", ix.User)
	}
	if _, ok := ix.Files["SD:Packs/new.wav"]; ok || ix.Folders[0] != "SD:Packs" {
		t.Error("the original's files or folders were changed through the copy")
	}
}

func TestTaggingManyFilesAtOnceKeepsEachFilesOwnTags(t *testing.T) {
	ix := app.NewTagIndex()
	ix.Folders = []string{"SD:Packs"}
	var paths []string
	for i := 0; i < 200; i++ {
		p := fmt.Sprintf("SD:Packs/kit/%03d_loop_124.wav", i)
		paths = append(paths, p)
		ix.Files[p] = "kit loop 124"
	}
	if n := ix.AddUserTags(paths, []string{"favourite", "favourite", "", "Wobbly Bass"}); n != 200 {
		t.Fatalf("%d changed", n)
	}
	// The same words again change nothing; a file already tagged is not counted.
	if n := ix.AddUserTags(paths, []string{"favourite"}); n != 0 {
		t.Errorf("%d changed the second time", n)
	}
	for _, p := range paths {
		got := ix.Files[p]
		if !strings.Contains(got, "kit loop 124") || !strings.Contains(got, "favourite") || !strings.Contains(got, "wobbly-bass") {
			t.Fatalf("%s: %q", p, got)
		}
	}
	if n := ix.RemoveUserTags(paths[:100], []string{"favourite"}); n != 100 {
		t.Errorf("%d removed", n)
	}
	if got := ix.UserTagCounts(); len(got) != 2 || got[0].Count != 200 && got[1].Count != 200 {
		t.Errorf("counts %v", got)
	}
	// Removing a tag a file never had, or from files the index does not know, is harmless.
	if n := ix.RemoveUserTags([]string{"SD:Nowhere/x.wav"}, []string{"favourite"}); n != 0 {
		t.Errorf("%d removed from an unknown file", n)
	}
}

func TestAWalkKeepsTheTagsGivenWhileItRan(t *testing.T) {
	walked := app.NewTagIndex()
	walked.Folders = []string{"SD:Packs"}
	walked.Files["SD:Packs/a.wav"] = "a"
	newest := walked.Clone()
	newest.AddUserTags([]string{"SD:Packs/a.wav"}, []string{"favourite"})
	walked.UseUserTagsOf(newest)
	if !walked.HasUserTag("SD:Packs/a.wav", "favourite") || !strings.Contains(walked.Files["SD:Packs/a.wav"], "favourite") {
		t.Errorf("the tag given meanwhile was lost: %v %q", walked.User, walked.Files["SD:Packs/a.wav"])
	}
	// Taking one off meanwhile counts too.
	newest.RemoveUserTags([]string{"SD:Packs/a.wav"}, []string{"favourite"})
	walked.UseUserTagsOf(newest)
	if walked.HasUserTag("SD:Packs/a.wav", "favourite") || strings.Contains(walked.Files["SD:Packs/a.wav"], "favourite") {
		t.Errorf("a tag taken off meanwhile came back: %q", walked.Files["SD:Packs/a.wav"])
	}
}

func TestFolderTagsReachEveryFileBelowAndGoWhenTakenOff(t *testing.T) {
	ix := app.NewTagIndex()
	ix.Folders = []string{"SD:Packs"}
	for _, p := range []string{"SD:Packs/a/x/1.wav", "SD:Packs/a/2.wav", "SD:Packs/b/3.wav"} {
		ix.Files[p] = "own"
	}
	if n := ix.AddFolderTags([]string{"SD:Packs/a"}, []string{"Best Of"}); n != 1 {
		t.Fatalf("%d changed", n)
	}
	if hits := ix.Search("best-of"); len(hits) != 2 {
		t.Errorf("found %v", hits)
	}
	if !ix.HasUserTag("SD:Packs/a", "best-of") || ix.HasUserTag("SD:Packs/b", "best-of") {
		t.Error("the folder marker is wrong")
	}
	// Written and read back, the folder's tag still reaches its files (and a new file there).
	s := &memStorage{files: map[string][]byte{}}
	if err := ix.Save(s); err != nil {
		t.Fatal(err)
	}
	back, err := app.LoadTagIndex(s)
	if err != nil || len(back.Search("best-of")) != 2 {
		t.Fatalf("read back: %v %v", err, back.Search("best-of"))
	}
	back.Files["SD:Packs/a/new.wav"] = "new"
	back.Retag()
	if len(back.Search("best-of")) != 3 {
		t.Errorf("a file added later has no tag: %v", back.Search("best-of"))
	}
	if n := back.RemoveFolderTags([]string{"SD:Packs/a"}, []string{"best-of"}); n != 1 {
		t.Errorf("%d removed", n)
	}
	if hits := back.Search("best-of"); len(hits) != 0 {
		t.Errorf("still found %v", hits)
	}
	// A clone keeps folder tags apart too.
	c := ix.Clone()
	c.RemoveFolderTags([]string{"SD:Packs/a"}, []string{"best-of"})
	if len(ix.Search("best-of")) != 2 {
		t.Error("the original lost the folder tag through its copy")
	}
}

func TestWAVSecondsReadsTheLengthFromTheFirstBytes(t *testing.T) {
	s := formats.Sample{Channels: 2, Samples: make([]int16, 2*48000)}
	got, ok := app.WAVSeconds(s.WAV()[:256])
	if !ok || got < 0.999 || got > 1.001 {
		t.Errorf("%v %v", got, ok)
	}
	if _, ok := app.WAVSeconds([]byte("ID3 not a wav at all")); ok {
		t.Error("something that is not a WAV had a length")
	}
}

func TestScanOriginsMatchesNamesThenLengths(t *testing.T) {
	pad := func(name string) proto.PadIndex { p, _ := proto.ParsePad(name); return p }
	files := []app.IndexedFile{}
	for _, p := range []string{
		"SD:Kits/One/09_OHat_124_HDK_01.wav", "SD:Kits/Two/09_OHat_124_HDK_02.wav", // the same pad name
		"SD:Kits/One/14_Kick.wav", "SD:Other/unrelated.wav",
	} {
		files = append(files, app.IndexedFile{Path: p})
	}
	rows := []app.PadRow{
		{Pad: pad("A1"), HasSample: true, Name: app.PadName("09_OHat_124_HDK_01.wav")},
		{Pad: pad("A2"), HasSample: true, Name: "14_Kick"},
		{Pad: pad("A3"), HasSample: true, Name: "nothing like it"},
		{Pad: pad("A4"), HasSample: true, Name: "14_Kick"},
		{Pad: pad("A5")},
	}
	have := &app.Origins{Pads: map[string]app.Origin{}}
	have.Pads["1/A4"] = app.Origin{Path: "SD:Kits/One/14_Kick.wav", Name: "14_Kick"}
	lengths := map[string]float64{"SD:Kits/One/09_OHat_124_HDK_01.wav": 7.74, "SD:Kits/Two/09_OHat_124_HDK_02.wav": 3.0}
	seconds := func(p string) (float64, bool) { s, ok := lengths[p]; return s, ok }
	padSeconds := func(r app.PadRow) (float64, bool) { return 7.74, true }
	var lines []string
	scan := app.ScanOrigins(rows, 0, have, files, seconds, padSeconds, nil, func(done, total int, line string) { lines = append(lines, line) })
	// A1: two files have its name, and only one has its length. A2: one file has the name.
	// A3: nothing. A4: already known. A5: no sample.
	if len(scan.Found) != 2 || scan.Found[0].Path != "SD:Kits/One/09_OHat_124_HDK_01.wav" || scan.Found[1].Path != "SD:Kits/One/14_Kick.wav" || !scan.Found[0].Matched {
		t.Errorf("found %+v", scan.Found)
	}
	if len(scan.Unknown) != 1 || scan.Unknown[0] != pad("A3") || scan.Known != 1 || len(scan.Ambiguous) != 0 {
		t.Errorf("unknown %v known %d ambiguous %v", scan.Unknown, scan.Known, scan.Ambiguous)
	}
	if len(lines) != 3 {
		t.Errorf("%d progress lines: %v", len(lines), lines)
	}
	// Two files of one name and one length cannot be told apart: the pad is left, with both.
	lengths["SD:Kits/Two/09_OHat_124_HDK_02.wav"] = 7.74
	rows[0].Name = app.PadName("09_OHat_124_HDK_02.wav")
	scan = app.ScanOrigins(rows[:1], 0, have, files, seconds, padSeconds, nil, nil)
	if len(scan.Found) != 0 || len(scan.Ambiguous) != 1 || len(scan.Ambiguous[0].Candidates) != 2 {
		t.Errorf("ambiguous %+v found %+v", scan.Ambiguous, scan.Found)
	}
}

// cardWithProject is a card in memory that also answers for the current project, enough for
// the functions that read a pad's sample and files by their paths.
type cardWithProject struct{ *memStorage }

func (cardWithProject) CurrentProject() (uint8, error) { return 0, nil }

func TestMatchByContentTellsFilesOfTheSameNameApart(t *testing.T) {
	noise := func(seed int) formats.Sample {
		s := formats.Sample{Channels: 2, Samples: make([]int16, 2*20000)}
		x := uint32(seed)*2654435761 + 1
		for i := range s.Samples {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			s.Samples[i] = int16(x>>16) / 4
		}
		return s
	}
	card := cardWithProject{&memStorage{files: map[string][]byte{}}}
	candidates := []string{"SD:Kits/One/Loop.wav", "SD:Kits/Two/Loop.wav", "SD:Kits/Three/Loop.wav"}
	for i, c := range candidates {
		card.files[c] = noise(i + 1).WAV()
	}
	pad, _ := proto.ParsePad("A3")
	row := app.PadRow{Pad: pad, HasSample: true, Name: "Loop"}
	// The pad holds the second file, as an assign makes it: converted, and written as an SMP.
	imported, err := formats.DecodeAudio(card.files[candidates[1]], ".wav")
	if err != nil {
		t.Fatal(err)
	}
	card.files[pad.SamplePath(1)] = imported.Sample.SMP()
	got := app.MatchByContent(card, row, candidates)
	if len(got) != 1 || got[0] != candidates[1] {
		t.Errorf("matched %v, want %v", got, candidates[1])
	}
	// A pad that holds something else (edited on the unit) leaves every candidate standing.
	card.files[pad.SamplePath(1)] = noise(99).SMP()
	if got := app.MatchByContent(card, row, candidates); len(got) != 3 {
		t.Errorf("an unknown sample narrowed to %v", got)
	}
	// A silent start cannot be told apart either.
	card.files[pad.SamplePath(1)] = formats.Sample{Channels: 2, Samples: make([]int16, 2*20000)}.SMP()
	if got := app.MatchByContent(card, row, candidates); len(got) != 3 {
		t.Errorf("silence narrowed to %v", got)
	}
	// Through the scan: three files of one name, one length, settled by what they hold.
	card.files[pad.SamplePath(1)] = imported.Sample.SMP()
	var files []app.IndexedFile
	for _, c := range candidates {
		files = append(files, app.IndexedFile{Path: c})
	}
	scan := app.ScanOrigins([]app.PadRow{row}, 0, &app.Origins{Pads: map[string]app.Origin{}}, files, nil, nil,
		func(r app.PadRow, c []string) []string { return app.MatchByContent(card, r, c) }, nil)
	if len(scan.Found) != 1 || scan.Found[0].Path != candidates[1] || len(scan.Ambiguous) != 0 {
		t.Errorf("scan %+v", scan)
	}
}

func TestAMixIsExactlyOneRoundAndTailsWrapToTheStart(t *testing.T) {
	a1, _ := proto.NewPadIndex(0)
	long := formats.Sample{Channels: 1, Samples: make([]int16, 20000)}
	for i := range long.Samples {
		long.Samples[i] = 1000
	}
	// One beat at 120 BPM is 24000 frames; a note half way rings on past the end.
	p := formats.Pattern{Length: formats.PPQ, Events: []formats.Event{{Tick: formats.PPQ / 2, Pad: a1, Velocity: 127}}}
	mix, err := app.MixPattern(p, 120, func(proto.PadIndex) (formats.Sample, error) { return long, nil })
	if err != nil {
		t.Fatal(err)
	}
	if mix.Frames() != 24000 {
		t.Fatalf("%d frames, want one round of 24000", mix.Frames())
	}
	// 20000 frames from 12000: to the end of the round, and its last 8000 frames at the start.
	at := func(f int) int16 { return mix.Samples[2*f] }
	if at(0) != 1000 || at(7999) != 1000 || at(8000) != 0 || at(11999) != 0 || at(12000) != 1000 || at(23999) != 1000 {
		t.Errorf("the tail did not wrap: %d %d %d %d %d %d", at(0), at(7999), at(8000), at(11999), at(12000), at(23999))
	}
}

func TestAPadVoiceFollowsThePadsSettings(t *testing.T) {
	ramp := formats.Sample{Channels: 1, Samples: make([]int16, 1000)}
	for i := range ramp.Samples {
		ramp.Samples[i] = int16(i * 10)
	}
	// As it is: stereo, the same both sides, at full level.
	v := app.PadVoice(ramp, 127, map[string]int32{"balance": 64})
	if v.Sample.Channels != 2 || v.Sample.Frames() != 1000 || v.Sample.Samples[2*500] != 5000 || v.Sample.Samples[2*500+1] != 5000 {
		t.Fatalf("plain voice: %d channels, %d frames, %d/%d", v.Sample.Channels, v.Sample.Frames(), v.Sample.Samples[1000], v.Sample.Samples[1001])
	}
	// Level halves it.
	if v := app.PadVoice(ramp, 64, nil); v.Sample.Samples[2*500] > 2600 || v.Sample.Samples[2*500] < 2400 {
		t.Errorf("at level 64: %d", v.Sample.Samples[2*500])
	}
	// Panned hard left, the right side is silent.
	v = app.PadVoice(ramp, 127, map[string]int32{"balance": 13})
	if v.Sample.Samples[2*500] != 5000 || v.Sample.Samples[2*500+1] != 0 {
		t.Errorf("panned left: %d/%d", v.Sample.Samples[1000], v.Sample.Samples[1001])
	}
	// Reverse: the last frame comes first.
	v = app.PadVoice(ramp, 127, map[string]int32{"play-mode": 1})
	if v.Sample.Samples[0] != 9990 || v.Sample.Samples[2*999] != 0 {
		t.Errorf("reversed: first %d last %d", v.Sample.Samples[0], v.Sample.Samples[2*999])
	}
	// An octave up is half as long; an octave down twice.
	if v := app.PadVoice(ramp, 127, map[string]int32{"pitch-coarse": 12}); v.Sample.Frames() != 500 {
		t.Errorf("an octave up has %d frames", v.Sample.Frames())
	}
	if v := app.PadVoice(ramp, 127, map[string]int32{"pitch-coarse": -12}); v.Sample.Frames() != 2000 {
		t.Errorf("an octave down has %d frames", v.Sample.Frames())
	}
	if v := app.PadVoice(ramp, 127, map[string]int32{"gate": 1, "loop": 1}); !v.Gate || !v.Loop {
		t.Error("gate and loop did not come along")
	}
}

func TestGatedPadsStopAndLoopingPadsRepeatForTheNote(t *testing.T) {
	a1, _ := proto.NewPadIndex(0)
	short := formats.Sample{Channels: 2, Samples: make([]int16, 2*6000)}
	for i := range short.Samples {
		short.Samples[i] = 1000
	}
	// One beat at 120 BPM is 24000 frames; the note lasts a quarter of it, 6000 frames... of a
	// 12000 frame sample for the gate, and the 6000 frame sample repeated over a half for the loop.
	long := formats.Sample{Channels: 2, Samples: make([]int16, 2*12000)}
	for i := range long.Samples {
		long.Samples[i] = 1000
	}
	p := formats.Pattern{Length: formats.PPQ, Events: []formats.Event{{Tick: 0, Pad: a1, Velocity: 127, Length: formats.PPQ / 4}}}
	gated, _ := app.MixPatternVoices(p, 120, func(proto.PadIndex) (app.Voice, error) { return app.Voice{Sample: long, Gate: true}, nil })
	if gated.Samples[2*5999] != 1000 || gated.Samples[2*6000] != 0 {
		t.Errorf("gated: %d then %d", gated.Samples[2*5999], gated.Samples[2*6000])
	}
	p.Events[0].Length = formats.PPQ / 2
	looped, _ := app.MixPatternVoices(p, 120, func(proto.PadIndex) (app.Voice, error) { return app.Voice{Sample: short, Loop: true}, nil })
	if looped.Samples[2*11999] != 1000 || looped.Samples[2*12000] != 0 {
		t.Errorf("looped: %d then %d", looped.Samples[2*11999], looped.Samples[2*12000])
	}
	free, _ := app.MixPatternVoices(p, 120, func(proto.PadIndex) (app.Voice, error) { return app.Voice{Sample: long}, nil })
	if free.Samples[2*11999] != 1000 {
		t.Error("a pad that is neither gated nor looping was cut")
	}
}

func TestGridOffsetIsTheDistanceToTheNearestStep(t *testing.T) {
	// A 2-bar pattern at 16ths: a step is 120 ticks.
	d := &app.PatternDetail{Length: 3840, Beats: 4, BankTempo: 120, Grid: app.Grid{Steps: 32}}
	for tick, want := range map[uint32]int{0: 0, 24: 24, 100: -20, 1920 + 60: 60, 3839: -1} {
		if got := d.GridOffset(tick); got != want {
			t.Errorf("GridOffset(%d) = %d, want %d", tick, got, want)
		}
	}
	// A 16th at 120 BPM is 125 ms.
	if got := d.TicksToMillis(120); got != 125 {
		t.Errorf("TicksToMillis(120) = %v, want 125", got)
	}
}

func TestAPutSampleGetsFullLevel(t *testing.T) {
	d, _ := connect(t)
	a1 := pad(t, "A1")
	level, _ := proto.ParamByName("level")
	if err := d.SetParam(level.ID, proto.TargetPad(a1), 20); err != nil {
		t.Fatal(err)
	}
	sample := formats.Sample{Channels: 2, Samples: make([]int16, 4000)}
	if _, err := app.PutSample(d, a1, sample, "bounce"); err != nil {
		t.Fatal(err)
	}
	block, err := d.PadBlock(a1)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := block.Param(level.ID)
	if got != 127 {
		t.Errorf("the pad is at level %d after a put, want 127", got)
	}
}

func TestFolderUsesAreWorkedOutFromOriginsAndRolledUp(t *testing.T) {
	o := &app.Origins{Pads: map[string]app.Origin{}}
	var rows []app.PadRow
	for i, name := range []string{"a", "b", "c", "d"} {
		p := proto.AllPads()[32+i] // bank C
		rows = append(rows, app.PadRow{Pad: p, HasSample: true, Name: name})
		folder := "SD:IMPORT/kit-one"
		if i == 3 {
			folder = "SD:IMPORT/other"
		}
		o.Pads[fmt.Sprintf("1/%s", p)] = app.Origin{Path: folder + "/" + name + ".wav", Name: name}
	}
	// One lone sample on bank D is not a folder.
	lone := proto.AllPads()[48]
	rows = append(rows, app.PadRow{Pad: lone, HasSample: true, Name: "x"})
	o.Pads[fmt.Sprintf("1/%s", lone)] = app.Origin{Path: "SD:IMPORT/solo/x.wav", Name: "x"}

	found := app.FolderUsesFromOrigins(o, 0, rows)
	if len(found) != 1 || found[0].Bank != 2 || found[0].Folder != "SD:IMPORT/kit-one" || found[0].Pads != 3 {
		t.Fatalf("found %+v", found)
	}

	f := &app.FolderUses{Banks: map[string]app.FolderUse{
		"1/C": {Folder: "SD:IMPORT/house/kit-one"},
		"2/A": {Folder: "SD:IMPORT/house"},
	}}
	own, inside := f.Of("SD:IMPORT/house")
	if len(own) != 1 || own[0].Project != 2 || len(inside) != 1 || inside[0].Bank != 2 {
		t.Fatalf("own %+v inside %+v", own, inside)
	}
	if f.Used("SD:IMPORT/hou") || !f.Used("SD:IMPORT") || f.Used("SD:IMPORT/house/kit-two") {
		t.Error("a folder is used by its own name or a folder inside it, not by a name that starts the same")
	}
	var none *app.FolderUses
	if none.Used("SD:IMPORT") {
		t.Error("no notes, nothing used")
	}
}
