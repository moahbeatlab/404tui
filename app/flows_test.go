package app_test

import (
	"bytes"
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
