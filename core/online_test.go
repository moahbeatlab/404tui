package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/dbg"
	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/record"
	"github.com/moahbeatlab/404tui/sim"
)

func termOn(t *testing.T) {
	t.Helper()
	Terminal = true
	t.Cleanup(func() { Terminal = false; SetSize(MinCols, MinRows) })
	SetSize(120, 40)
}

// kitKeys puts the kit on the pads with the handheld's keys (a computer asks which pad in a dialog, the kit script does not).
func kitKeys(t *testing.T, a *App) {
	t.Helper()
	was := Terminal
	Terminal = false
	defer func() { Terminal = was }()
	keys(t, a, kit)
}

func screenStr(a *App) string {
	s := a.RenderCells()
	var sb strings.Builder
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			sb.WriteRune(s.Cells[r*s.Cols+c].R)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// waitLink ticks until the unit has been found (a computer looks in the background).
func waitLink(t *testing.T, a *App) {
	t.Helper()
	for i := 0; i < 300 && a.link == nil; i++ {
		a.Tick()
		time.Sleep(10 * time.Millisecond)
	}
	if a.link == nil {
		t.Fatal("the unit was not found")
	}
}

func TestTerminalScreensAreSixAndInOrder(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	for i, want := range []string{"PADS", "SEQ", "UNIT", "FILES", "SLICE", "CONFIG"} {
		a.GoScreen(i)
		if i == 4 && a.ScreenName() == "PADS" { // the slicer bounces back to the pads when the pad is empty
			continue
		}
		if a.ScreenName() != want {
			t.Errorf("%d is %s, got %s", i+1, want, a.ScreenName())
		}
	}
	a.GoScreen(6)
	if a.ScreenName() == "PERF" || a.ScreenName() == "LOOP" || a.ScreenName() == "SAVE" {
		t.Errorf("7 goes to %s: PERF, LOOP and SAVE are not on a computer", a.ScreenName())
	}
	txt := screenStr(a)
	for _, gone := range []string{"LOOP", "PERF", "SAVE"} {
		if strings.Contains(txt, gone) {
			t.Errorf("the screen map still shows %s", gone)
		}
	}
}

func TestOfflineProjectsAreListedAndOpened(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	a.slot = 3
	if _, err := a.exportTo(a.offlineDir()); err != nil {
		t.Fatal(err)
	}
	a.GoScreen(2)
	txt := screenStr(a)
	if !strings.Contains(txt, "OFFLINE") || !strings.Contains(txt, "PROJECT_03") || !strings.Contains(txt, "PROJECTS") {
		t.Fatalf("PROJECTS offline lists the folder's projects:\n%s", txt)
	}
	a.unitCur = 2
	a.Dispatch(Event{Key: A, Down: true})
	a.Dispatch(Event{Key: A, Down: false})
	if a.menu == nil || !strings.Contains(screenStr(a), "Open it in the editor") {
		t.Fatalf("A opens the project's menu:\n%s", screenStr(a))
	}
	a.menu = nil
	a.openOffline(2)
	if a.ScreenName() != "PADS" || a.set.LastOffline != 3 {
		t.Fatalf("opening goes to the pads and remembers the project: %s, last %d", a.ScreenName(), a.set.LastOffline)
	}
	if n, ok := a.lastOffline(); !ok || n != 2 {
		t.Fatalf("the last project offline is 3: %d %v", n, ok)
	}
}

func TestOnlineWhenTheUnitIsThereOfflineWhenChosen(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.GoScreen(2)
	if !strings.Contains(screenStr(a), "OFFLINE") {
		t.Fatal("without a unit PROJECTS is offline")
	}
	waitLink(t, a)
	if !a.onlineNow() || !strings.Contains(screenStr(a), "ONLINE") {
		t.Fatalf("with a unit PROJECTS is online:\n%s", screenStr(a))
	}
	a.offlineChosen = true
	if !strings.Contains(screenStr(a), "OFFLINE") || !strings.Contains(screenStr(a), "t goes online") {
		t.Fatalf("offline chosen: the screen says so and how to go back:\n%s", screenStr(a))
	}
	a.ToolsMenu()
	if a.menu == nil || !strings.Contains(screenStr(a), "Online: the projects of the unit") {
		t.Fatalf("t on PROJECTS offers online and offline:\n%s", screenStr(a))
	}
}

func TestSplashOffersOnlineAndOffline(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	a.slot = 2
	a.exportTo(a.offlineDir())
	a.set.LastOffline = 2
	waitLink(t, a)
	a.ShowSplash(time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	a.Tick()
	if a.splash == nil {
		t.Fatal("with a unit the splash waits for a choice")
	}
	txt := screenStr(a)
	for _, want := range []string{"UNIT FOUND", "u  work online", "o  work offline: PROJECT_02"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the splash says %q:\n%s", want, txt)
		}
	}
	if !a.SplashText("o") || a.splash != nil || a.ScreenName() != "PADS" || a.slot != 2 || !a.offlineChosen {
		t.Fatalf("o opens the last project offline: %s slot %d", a.ScreenName(), a.slot)
	}
	a.ShowSplash(time.Hour)
	if !a.SplashText("u") || a.offlineChosen || a.ScreenName() != "UNIT" || !a.onlineNow() {
		t.Fatalf("u goes online, to PROJECTS: %s", a.ScreenName())
	}
	for i := 0; i < 300 && a.live == nil; i++ {
		a.Tick()
		time.Sleep(10 * time.Millisecond)
	}
	if a.live == nil || a.ScreenName() != "PADS" {
		t.Fatalf("online loads the unit's project and edits it live, then shows the pads: live %v, %s", a.live != nil, a.ScreenName())
	}
	a.stopLive()
	if a.SplashText("o") {
		t.Fatal("the letters are the splash's only while it is in front")
	}
}

func TestSeqMuteAndSolo(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	a.GoScreen(1)
	keys(t, a, "A")
	if len(a.rows) == 0 {
		t.Skip("no track")
	}
	pad := a.rowPad()
	a.SeqTrack(1)
	if a.trackMode(pad) != trackMute || a.audible(pad) {
		t.Fatal("m mutes the track")
	}
	a.SeqTrack(1)
	if a.trackMode(pad) != trackOn {
		t.Fatal("m again plays it")
	}
	a.SeqTrack(2)
	if a.trackMode(pad) != trackSolo {
		t.Fatal("s makes it solo")
	}
}

func TestNamesAreTypedWithoutAKeyboardOnScreen(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.askText("rename A1", "", "kick", func(string) {})
	txt := screenStr(a)
	if strings.Contains(txt, "abcdefghij") || !strings.Contains(txt, "Enter done") || !strings.Contains(txt, "kick_") {
		t.Fatalf("a text prompt is one line to type in:\n%s", txt)
	}
}

func TestFilesTChoosesWhereFilesComeFrom(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.GoScreen(3)
	a.ToolsMenu()
	txt := strings.ToLower(screenStr(a))
	for _, want := range []string{"load files from where", "computer", "samples", "copy, cut, rename, delete"} {
		if !strings.Contains(txt, want) {
			t.Errorf("t on FILES offers %q:\n%s", want, txt)
		}
	}
}

func TestLevelMeterIsInThePanelWhileAUnitIsThere(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	if strings.Contains(screenStr(a), "level:") {
		t.Fatal("no meter without a unit")
	}
	waitLink(t, a)
	a.Tick()
	if txt := screenStr(a); !strings.Contains(txt, "level: listening") {
		t.Fatalf("with a unit the panel has a meter (listening at first):\n%s", txt)
	}
	l := a.lvl
	if l == nil {
		l = newLevelMeter()
		a.lvl = l
	}
	l.ok = true
	l.hold = [2]float64{-6, -20}
	l.clipped = time.Now()
	txt := screenStr(a)
	if !strings.Contains(txt, "-6.0 dB") || !strings.Contains(txt, "-20.0 dB") || !strings.Contains(txt, "CLIP") {
		t.Fatalf("the meter shows both channels and the clip light:\n%s", txt)
	}
}

func TestProjectsAreExportedIntoAChosenFolder(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	dir := t.TempDir()
	a.exportProjectAsk()
	if a.dp == nil || !strings.Contains(screenStr(a), "EXPORT PROJECT_01 INTO THE FOLDER") {
		t.Fatalf("the export opens the folder chooser:\n%s", screenStr(a))
	}
	a.dp.dir = dir
	a.dpList("")
	a.dpKey(A)
	if _, err := os.Stat(filepath.Join(dir, "PROJECT_01", "PADCONF.BIN")); err != nil {
		t.Fatalf("the project is in the folder that was typed: %v", err)
	}
	// a project of the unit goes into a folder too
	waitLink(t, a)
	a.GoScreen(2)
	a.unitCur = 0
	a.unitMenu()
	if !strings.Contains(screenStr(a), "Back up: write a copy into a folder") {
		t.Fatalf("the unit's project menu offers a folder:\n%s", screenStr(a))
	}
	a.menu = nil
	dev, _ := a.unitDevice()
	root := filepath.Join(t.TempDir(), "backups")
	os.MkdirAll(root, 0o755)
	a.unitBackupAsk(dev, 0)
	if a.dp == nil {
		t.Fatal("the backup opens the folder chooser")
	}
	a.dp.dir = root
	a.dpList("")
	a.dpKey(A)
	for i := 0; i < 500 && a.job != nil; i++ {
		a.Tick()
		if _, _, _, _, fin := a.job.state(); fin {
			a.Tick()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	made, _ := filepath.Glob(filepath.Join(root, "*", "PADCONF.BIN"))
	if len(made) != 1 {
		t.Fatalf("the backup is a folder of its own in the folder that was typed: %v", made)
	}
	if a.set.BackupDir != root || !strings.Contains(a.msg, root) {
		t.Fatalf("the folder is kept and the message says where the backup is: %q %q", a.set.BackupDir, a.msg)
	}
}

func TestLiveChangesAreCoalesced(t *testing.T) {
	q := []liveChange{{kind: "param", pad: 2, id: 0x69, value: 100}, {kind: "param", pad: 2, id: 0x78, value: 5}, {kind: "pattern", pad: 3}, {kind: "sample", pad: 4}, {kind: "param", pad: 4, id: 0x69}}
	q = coalesce(q, []liveChange{{kind: "param", pad: 2, id: 0x69, value: 101}, {kind: "pattern", pad: 3}, {kind: "clear", pad: 4}})
	if len(q) != 4 {
		t.Fatalf("a setting and a pattern that are overtaken are not sent, a cleared pad takes its waiting changes: %+v", q)
	}
	for _, c := range q {
		if c.kind == "param" && c.pad == 2 && c.id == 0x69 && c.value != 101 {
			t.Fatalf("the last value is the one that is sent: %+v", c)
		}
		if c.pad == 4 && c.kind != "clear" {
			t.Fatalf("nothing else waits for a cleared pad: %+v", c)
		}
	}
}

func TestPadsChangedOnTheUnitComeIntoTheEditor(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	dev := device.New(sim.Demo(), "sim")
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	a.live.poll() // a difference has to be seen twice
	ps := a.live.poll()
	if len(ps) == 0 {
		t.Skip("the simulated unit has the same pads as the editor")
	}
	n := ps[0].n
	a.applyPulls(ps)
	if got := a.info.Pads[n].File; got != (ps[0].smp != nil) {
		t.Fatalf("the pad %d is in the editor as the unit has it: %v", n, got)
	}
	if again := a.live.poll(); len(again) != 0 {
		t.Fatalf("what came in is not pulled again: %d pads", len(again))
	}
	if !strings.Contains(a.msg, "changed on the unit") {
		t.Fatalf("it says so: %q", a.msg)
	}
	// a change made on the unit after that comes in too
	pad, _ := proto.NewPadIndex(uint16(n))
	if ps[0].smp != nil {
		if err := dev.SetParam(0x69, proto.TargetPad(pad), 50); err != nil {
			t.Fatal(err)
		}
		a.live.poll()
		next := a.live.poll()
		if len(next) != 1 || next[0].n != n {
			t.Fatalf("a setting changed on the unit is noticed: %d", len(next))
		}
		a.applyPulls(next)
		if v, _ := a.lab.PadParam(n, 0x69); v != 50 {
			t.Fatalf("the level is the unit's: %d", v)
		}
	}
}

func TestSpectrumIsDrawnUnderTheMeterInATallWindow(t *testing.T) {
	termOn(t)
	SetSize(120, 44)
	a, _ := newApp(t)
	waitLink(t, a)
	a.Tick()
	if a.lvl == nil {
		a.lvl = newLevelMeter()
	}
	a.lvl.ok = true
	a.lvl.hold = [2]float64{-12, -12}
	a.lvl.spec = make([]float64, 64)
	for i := range a.lvl.spec {
		a.lvl.spec[i] = float64(i) / 63
	}
	if txt := screenStr(a); !strings.ContainsAny(txt, "⣿⣷⣾⣶⣤⣀⣄⣠⢀⡀") {
		t.Fatalf("the spectrum is drawn:\n%s", txt)
	}
	a.lvl.spec = nil
	SetSize(120, 30)
}

func TestFolderChooserWalksTheFolders(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	top := t.TempDir()
	os.MkdirAll(filepath.Join(top, "music", "kits"), 0o755)
	os.MkdirAll(filepath.Join(top, "backups"), 0o755)
	os.MkdirAll(filepath.Join(top, ".hidden"), 0o755)
	var got string
	a.pickFolder("where to", top, func(d string) { got = d })
	if a.dp == nil || !a.Modal() {
		t.Fatal("the chooser is a dialog")
	}
	txt := screenStr(a)
	for _, want := range []string{"WHERE TO", "use this folder", "../", "backups/", "music/", "type a path"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the chooser shows %q:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, ".hidden") {
		t.Error("hidden folders are not listed")
	}
	a.dpKey(Down) // ..
	a.dpKey(Down) // backups
	a.dpKey(Down) // music
	a.dpKey(Right)
	if filepath.Base(a.dp.dir) != "music" || len(a.dp.names) != 2 { // .. and kits
		t.Fatalf("l opens the folder in front: %s %v", a.dp.dir, a.dp.names)
	}
	a.dpKey(Left)
	if a.dp.dir != top || a.dp.names[a.dp.cur-1] != "music" {
		t.Fatalf("h goes up and stands on the folder it came from: %s %d", a.dp.dir, a.dp.cur)
	}
	a.dpKey(Right)
	a.dpKey(Down) // ..
	a.dpKey(Down) // kits
	a.dpKey(A)
	if got != filepath.Join(top, "music", "kits") || a.dp != nil {
		t.Fatalf("Enter uses the folder in front: %q", got)
	}
	// the row ".." goes up, and Enter on it uses the folder above
	a.pickFolder("up", filepath.Join(top, "music"), func(d string) { got = d })
	a.dpKey(Down)
	a.dpKey(Right)
	if a.dp.dir != top {
		t.Fatalf("l on .. goes up: %s", a.dp.dir)
	}
	a.dpKey(Left)
	a.dpKey(Left) // up and up: the way to / is always there
	if a.dp.dir != filepath.Dir(filepath.Dir(top)) {
		t.Fatalf("h goes up: %s", a.dp.dir)
	}
	// a path is typed to go anywhere
	if !a.DirPickerType() || a.prompt == nil {
		t.Fatal("/ asks for a path")
	}
	a.prompt.buf = top
	a.TypeEnter()
	if a.dp.dir != top {
		t.Fatalf("the typed folder is where the chooser goes: %s", a.dp.dir)
	}
	a.DirPickerType()
	a.prompt.buf = filepath.Join(top, "nothing")
	a.TypeEnter()
	if a.dp.dir != top || !a.msgBad {
		t.Fatal("a path that is not a folder is refused")
	}
	a.dpKey(B)
	a.pickFolder("again", top, func(d string) { got = d })
	a.dpKey(X)
	if a.prompt == nil {
		t.Fatal("x asks for the name of a new folder")
	}
	a.prompt.buf = "fresh"
	a.TypeEnter()
	if _, err := os.Stat(filepath.Join(top, "fresh")); err != nil || a.dp.names[a.dp.cur-1] != "fresh" {
		t.Fatalf("the new folder is made and under the cursor: %v", err)
	}
	a.dpKey(B)
	if a.dp != nil {
		t.Fatal("Esc closes it")
	}
	// a folder that is not there starts at the nearest one that is
	a.pickFolder("gone", filepath.Join(top, "nothing", "here"), nil)
	if a.dp.dir != top {
		t.Fatalf("starts at %s", a.dp.dir)
	}
}

func TestFilesLeftColumnShowsTheSourceAtTheTopOfOne(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	waitLink(t, a)
	a.GoScreen(3)
	a.cd("/")
	txt := screenStr(a)
	for _, want := range []string{"sources", "computer", "samples", "the top", "SD card", "Unit storage"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the left column lists %q:\n%s", want, txt)
		}
	}
	a.cd(app.CardRoot)
	if txt := screenStr(a); !strings.Contains(txt, "sources") {
		t.Errorf("the top of the card shows the sources too:\n%s", txt)
	}
	a.cd(a.Root)
	if txt := screenStr(a); strings.Contains(txt, "sources") {
		t.Errorf("below the top the left column is the folder above:\n%s", txt)
	}
}

func TestIndexingShowsItsProgressInThePanel(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.idx.mu.Lock()
	a.idx.running, a.idx.root = true, "/Volumes/UNTITLED/IMPORT"
	a.idx.mu.Unlock()
	atomic.StoreInt64(&a.idx.count, 1234)
	txt := screenStr(a)
	for _, want := range []string{"INDEXING", "1234 files", "IMPORT"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the panel shows %q while it indexes:\n%s", want, txt)
		}
	}
	a.idx.mu.Lock()
	a.idx.running = false
	a.idx.mu.Unlock()
	if strings.Contains(screenStr(a), "INDEXING") {
		t.Error("the progress goes when it is done")
	}
}

func TestSearchShowsTheFolderAndCanLookForFoldersOnly(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	SetSize(160, 48)
	a.GoScreen(3)
	a.openSearch()
	a.TypeText("c1")
	if len(a.entries) == 0 || a.entries[0].dir {
		t.Fatalf("a search finds files: %+v", a.entries)
	}
	txt := screenStr(a)
	if !strings.Contains(txt, "bass_C1.wav") || !strings.Contains(txt, "‹ bass") {
		t.Fatalf("a result shows its file and the folder it is in:\n%s", txt)
	}
	a.TypeText("") // keep the query
	a.searchQ = "bass"
	a.TypeTab()
	if !a.searchDirs || len(a.entries) != 1 || !a.entries[0].dir || a.entries[0].name != "bass" {
		t.Fatalf("Tab: folders only: %+v", a.entries)
	}
	if txt := screenStr(a); !strings.Contains(txt, "FOLDERS") || !strings.Contains(txt, "Tab files") {
		t.Fatalf("the search line says it looks for folders:\n%s", txt)
	}
	a.TypeTab()
	if a.searchDirs || a.entries[0].dir {
		t.Fatalf("Tab again: files: %+v", a.entries)
	}
}

func TestBackupFolderIsNamedLikeTheProjectAndAsksWhenItIsThere(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	root := t.TempDir()
	var got string
	a.freeTarget(root, "House", true, func(d string) { got = d })
	if got != filepath.Join(root, "House") || a.menu != nil {
		t.Fatalf("a free name goes on at once: %q", got)
	}
	os.MkdirAll(filepath.Join(root, "House", "old"), 0o755)
	got = ""
	a.freeTarget(root, "House", true, func(d string) { got = d })
	if got != "" || a.menu == nil || !strings.Contains(strings.ToLower(screenStr(a)), "is there already") {
		t.Fatalf("a taken name asks:\n%s", screenStr(a))
	}
	txt := screenStr(a)
	for _, want := range []string{"Overwrite it", "Use a different name", "Cancel"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the question offers %q:\n%s", want, txt)
		}
	}
	// a different name
	a.menu.cur = 1
	a.menu.items[1].run(0)
	if a.prompt == nil || a.prompt.buf != "House_2" {
		t.Fatal("it asks for a different name, House_2 to start with")
	}
	a.TypeEnter()
	if got != filepath.Join(root, "House_2") {
		t.Fatalf("the new name is used: %q", got)
	}
	// overwrite
	a.freeTarget(root, "House", true, func(d string) { got = d })
	a.menu.items[0].run(0)
	if _, err := os.Stat(filepath.Join(root, "House", "old")); err == nil {
		t.Fatal("overwriting empties the folder first")
	}
	if got != filepath.Join(root, "House") {
		t.Fatalf("overwrite goes on with the same folder: %q", got)
	}
}

func TestChromaticNotesOfTheLivePerformanceProject(t *testing.T) {
	dirs, _ := filepath.Glob("/home/hermes/test-projects/07_LIVE*")
	if len(dirs) == 0 {
		t.Skip("the project is not here")
	}
	a, _ := newApp(t)
	if err := a.OpenProject(dirs[0]); err != nil {
		t.Fatal(err)
	}
	a.pat = 1
	d, ok := a.doc()
	if !ok {
		t.Fatal("pattern 1 is there")
	}
	var got []int
	for _, n := range d.Notes {
		if n.Pad == 4 { // A5
			got = append(got, formats.PitchSemitones(uint8(n.Pitch)))
		}
	}
	want := []int{0, 12, 0, -12, -6, 6, 11, 12}
	if len(got) != len(want) {
		t.Fatalf("A5 has %d notes: %v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the pitches of A5 are %v, wanted %v", got, want)
		}
	}
	// the note panel changes a pitch, and the pattern plays it up or down
	a.go_(Seq)
	pad := 4
	tick := d.Notes[0].Tick
	p := 7
	if _, ok := a.editNote(pad, tick, d.Notes[0].Vel, d.Notes[0].Len, 0, &p); !ok {
		t.Fatal("edit")
	}
	d, _ = a.doc()
	if n, _ := noteAt(d, pad, int(tick)/stepTicks); formats.PitchSemitones(uint8(n.Pitch)) != 7 {
		t.Fatalf("the pitch of the note is +7: %d", n.Pitch)
	}
	if _, ok := a.render(); !ok {
		t.Fatal("the pattern renders with pitched notes")
	}
	plain, _ := a.noteSample(pad, 100, 0, 0)
	up, _ := a.noteSample(pad, 100, 0, int(formats.PitchByte(12)))
	down, _ := a.noteSample(pad, 100, 0, int(formats.PitchByte(-12)))
	if !(up.Frames() < plain.Frames() && down.Frames() > plain.Frames()) {
		t.Fatalf("up is shorter and down longer: %d %d %d", up.Frames(), plain.Frames(), down.Frames())
	}
}

func TestSeqBracketsChangeTheBankAndBracesTheBar(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.GoScreen(1)
	a.setPattern(3)
	a.SeqBank(1)
	if a.pat != 19 {
		t.Fatalf("] goes to the pattern at the same place in bank B: %d", a.pat)
	}
	a.SeqBank(-1)
	a.SeqBank(-1)
	if a.pat != 3+9*16 {
		t.Fatalf("[ goes round to bank J: %d", a.pat)
	}
	if txt := screenStr(a); !strings.Contains(txt, "{ }") {
		t.Fatalf("the panel names the keys for the bar:\n%s", txt)
	}
}

func TestEditingLiveLoadsOnlyWhatIsNeeded(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	waitLink(t, a)
	dev, _ := a.unitDevice()
	a.unitLoadNow(dev, 0, true)
	for i := 0; i < 500 && a.live == nil; i++ {
		a.Tick()
		time.Sleep(10 * time.Millisecond)
	}
	if a.live == nil {
		t.Fatal("the project was not loaded")
	}
	t.Cleanup(func() { a.stopLive() })
	remote := a.lab.RemoteCount()
	if remote == 0 {
		t.Skip("the simulated project has no samples")
	}
	pad := -1
	for n := 0; n < 160; n++ {
		if a.lab.IsRemote(n) {
			pad = n
			break
		}
	}
	if !a.info.Pads[pad].File || a.info.Pads[pad].Frames == 0 {
		t.Fatalf("a pad on the unit shows with its length: %+v", a.info.Pads[pad])
	}
	// asked for in the background, not waited for
	if _, ok := a.padSampleSoft(pad); ok {
		t.Fatal("drawing does not wait for the unit")
	}
	for i := 0; i < 500 && a.lab.IsRemote(pad); i++ {
		a.Tick()
		time.Sleep(10 * time.Millisecond)
	}
	if a.lab.IsRemote(pad) {
		t.Fatal("the sample came in the background")
	}
	if _, ok := a.padSample(pad); !ok {
		t.Fatal("and it plays")
	}
	// a pad that is used at once is fetched at once, and nothing of it goes back to the unit
	var other int
	for n := 0; n < 160; n++ {
		if a.lab.IsRemote(n) {
			other = n
			break
		}
	}
	if a.lab.RemoteCount() > 0 {
		if _, ok := a.padSample(other); !ok || a.lab.IsRemote(other) {
			t.Fatal("a sample that is needed is fetched on the spot")
		}
	}
	a.liveSync()
	if w, _, _ := a.liveStatus(); w != 0 {
		t.Fatalf("a fetched sample is not sent back: %d changes waiting", w)
	}
}

func TestToolsKeepAndMakePitch(t *testing.T) {
	st := func(n lab.Note) int { return formats.PitchSemitones(uint8(n.Pitch)) }
	base := []lab.Note{{Tick: 0, Pad: 4, Vel: 100, Len: 120, Pitch: int(formats.PitchByte(7))}, {Tick: 480, Pad: 4, Vel: 90, Len: 120}}
	// an echo climbs from the pitch of the note
	e := echoNotesPitch(base[:1], 1920, 240, 3, 70, 50, 2)
	if len(e) != 4 || st(e[0]) != 7 || st(e[1]) != 9 || st(e[2]) != 11 || st(e[3]) != 12 {
		t.Fatalf("an echo climbs by 2 st a repeat from +7, up to the top: %v", e)
	}
	if got := echoNotes(base[:1], 1920, 240, 1, 70, 50); st(got[1]) != 7 {
		t.Fatalf("an echo without a pitch step repeats the pitch: %d", st(got[1]))
	}
	// a ratchet rises part by part
	r := ratchetNotesPitch(base[1:], 0, 4, 0, 0, 3, 1)
	if len(r) != 4 || st(r[0]) != 0 || st(r[3]) != 9 {
		t.Fatalf("the parts of a ratchet rise by 3 st: %v", r)
	}
	if r := ratchetNotes(base[:1], 0, 2, 0, 0, 1); st(r[0]) != 7 || st(r[1]) != 7 {
		t.Fatal("the parts keep the pitch of the note")
	}
	// made notes get a pitch
	ns := withPitch(euclidNotes(0, 1920, 120, 16, 4, 0, 50, 100), -5)
	for _, n := range ns {
		if st(n) != -5 {
			t.Fatalf("every note made is at -5: %v", ns)
		}
	}
	if len(ns) == 0 || len(withPitch(euclidNotes(0, 1920, 120, 16, 4, 0, 50, 100), 0)) == 0 {
		t.Fatal("notes")
	}
	// randomize
	rp := randomizePitch(append([]lab.Note(nil), base...), 3, 5)
	for i, n := range rp {
		if d := st(n) - st(base[i]); d < -3 || d > 3 {
			t.Fatalf("the spread is 3 st: %d", d)
		}
	}
	// the Pitch tool
	pn := pitchNotes(base, 0, -9, 1)
	if st(pn[0]) != -2 || st(pn[1]) != -9 {
		t.Fatalf("transpose: %d %d", st(pn[0]), st(pn[1]))
	}
	if pn := pitchNotes(base, 1, 5, 1); st(pn[0]) != 5 || st(pn[1]) != 5 {
		t.Fatal("set every note")
	}
	if pn := pitchNotes(base, 5, 0, 1); pn[0].Pitch != 0 || pn[1].Pitch != 0 {
		t.Fatal("back to plain")
	}
	run := pitchNotes(make([]lab.Note, 6), 3, 2, 1)
	if st(run[0]) != 0 || st(run[1]) != 2 || st(run[2]) != 4 || st(run[5]) != 10 {
		t.Fatalf("a rising run: %v", run)
	}
	// an LFO on the pitch
	l := lfoNotes([]lab.Note{{Tick: 0, Pad: 4, Vel: 100, Len: 120}, {Tick: 240, Pad: 4, Vel: 100, Len: 120}}, lfoState{target: 3, shape: 0, rate: 3, depth: 3}, 1)
	if l[0].Pitch == 0 && l[1].Pitch == 0 {
		t.Fatal("an LFO on the pitch moves it")
	}
}

func TestTracksKeepTheirPitchesThroughATool(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	dirs, _ := filepath.Glob("/home/hermes/test-projects/07_LIVE*")
	if len(dirs) == 0 {
		t.Skip("the project is not here")
	}
	if err := a.OpenProject(dirs[0]); err != nil {
		t.Fatal(err)
	}
	a.pat = 1
	a.go_(Seq)
	pad := 4
	before := a.trackNotes(pad)
	a.finishTrack(pad, cycleNotes(before, 3840, 3840), "cycle")
	after := a.trackNotes(pad)
	if len(after) != len(before) {
		t.Fatalf("%d notes became %d", len(before), len(after))
	}
	for i := range before {
		if after[i].Pitch != before[i].Pitch {
			t.Fatalf("note %d: pitch byte %d became %d", i, before[i].Pitch, after[i].Pitch)
		}
	}
}

func TestAFileGoesToThePadChosenInTheDialog(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.GoScreen(3)
	a.Dispatch(Event{Key: Right, Down: true})
	a.Dispatch(Event{Key: Right, Down: false})
	press := func(k Key) {
		a.Dispatch(Event{Key: k, Down: true})
		a.Dispatch(Event{Key: k, Down: false})
	}
	for a.entries[a.fcur].dir {
		press(Right)
	}
	press(A)
	if a.picker == nil || !a.picker.pads || !strings.Contains(strings.ToLower(screenStr(a)), "on a pad") {
		t.Fatalf("A on a file opens the pad dialog:\n%s", screenStr(a))
	}
	press(Right)
	press(Right) // A3
	press(A)
	if a.picker != nil || !a.info.Pads[2].File || a.info.Pads[0].File || a.ScreenName() != "PADS" {
		t.Fatalf("the file is on the pad that was chosen, A3: %v %v %s", a.info.Pads[2].File, a.info.Pads[0].File, a.ScreenName())
	}
	if a.bank != 0 || a.cur != 2 {
		t.Fatalf("the cursor is on that pad: bank %d pad %d", a.bank, a.cur)
	}
}

func TestFolderSearchMatchesTheNameOfTheFolderOnly(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	for _, d := range []string{"deep_house/bass_loops/A", "deep_house/drums", "house/deep_house_session/alone", "other/stuff"} {
		os.MkdirAll(filepath.Join(a.Root, d), 0o755)
	}
	a.GoScreen(3)
	a.openSearch()
	a.TypeText("deep house")
	a.TypeTab()
	var got []string
	for _, e := range a.entries {
		if !e.dir {
			t.Fatalf("folders only: %+v", e)
		}
		got = append(got, filepath.ToSlash(e.name))
	}
	want := map[string]bool{"deep_house": true, "house/deep_house_session": true}
	if len(got) != len(want) {
		t.Fatalf("only the folders whose own name has the words: %v", got)
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("%q does not have the words in its own name: %v", g, got)
		}
	}
}

func TestMarkedFilesGoOnConsecutivePadsInTheOrderOfTheirNames(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	src, err := os.ReadFile(filepath.Join(a.Root, "bass", "bass_C1.wav"))
	if err != nil {
		t.Fatal(err)
	}
	set := filepath.Join(a.Root, "set")
	os.MkdirAll(set, 0o755)
	for _, n := range []string{"kick_10.wav", "kick_2.wav", "kick_1.wav", "snare_1.wav"} {
		os.WriteFile(filepath.Join(set, n), src, 0o644)
	}
	a.GoScreen(3)
	a.cd(set)
	var order []string
	for _, e := range a.entries {
		order = append(order, e.name)
	}
	if strings.Join(order, " ") != "kick_1.wav kick_2.wav kick_10.wav snare_1.wav" {
		t.Fatalf("the list counts the way people do: %v", order)
	}
	a.fcur = 0
	a.MarkFile() // kick_1
	a.MarkFile() // kick_2
	a.MarkFile() // kick_10
	if len(a.fmarks) != 3 || a.fcur != 3 {
		t.Fatalf("m marks and moves on: %d marked, cursor %d", len(a.fmarks), a.fcur)
	}
	a.fcur = 0
	press := func(k Key) {
		a.Dispatch(Event{Key: k, Down: true})
		a.Dispatch(Event{Key: k, Down: false})
	}
	press(A)
	if a.picker == nil || !strings.Contains(strings.ToLower(screenStr(a)), "3 marked files") {
		t.Fatalf("A with marks opens the pad dialog:\n%s", screenStr(a))
	}
	press(A) // the pad it opens on
	var got []string
	start := -1
	for n, p := range a.info.Pads {
		if p.File {
			if start < 0 {
				start = n
			}
			got = append(got, p.Name)
		}
	}
	if strings.Join(got, " ") != "kick_1 kick_2 kick_10" || start != a.bank*16+a.cur {
		t.Fatalf("the files are on consecutive pads in order: %v from %d", got, start)
	}
	if len(a.fmarks) != 0 || a.ScreenName() != "PADS" {
		t.Fatal("the marks are gone and the pads are shown")
	}
}

func TestWhatWasJustSentIsNotPulledBack(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	dev := device.New(sim.Demo(), "sim")
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	kitKeys(t, a) // pads put in the editor: the unit does not have them yet, and is being given them
	a.liveSync()
	for i := 0; i < 3; i++ {
		if ps := a.live.poll(); len(ps) != 0 {
			t.Fatalf("while the unit is being given something, and for a while after, it is not looked at: %d pads", len(ps))
		}
	}
	if !a.info.Pads[0].File {
		t.Fatal("the pad is still in the editor")
	}
	// later, with nothing on its way, what differs counts once it is seen twice
	a.live.mu.Lock()
	a.live.queue, a.live.running, a.live.quietUntil = nil, false, time.Time{}
	a.live.mu.Unlock()
	a.live.poll()
	if ps := a.live.poll(); len(ps) == 0 {
		t.Skip("the simulated unit shows the same pads")
	}
}

func TestNumberedKitFilesGoOnThePadOfTheirNumberAndEmptyOnesAreLeftOut(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	src, _ := os.ReadFile(filepath.Join(a.Root, "bass", "bass_C1.wav"))
	set := filepath.Join(a.Root, "kit")
	os.MkdirAll(set, 0o755)
	for _, n := range []string{"00_DrumLoop_01.wav", "01_EMPTY.wav", "03_Loop1_01.wav", "12_Clap_01.wav", "14_Kick_01.wav", "16_EMPTY.wav"} {
		os.WriteFile(filepath.Join(set, n), src, 0o644)
	}
	a.GoScreen(3)
	a.cd(set)
	a.MarkAll()
	if len(a.fmarks) != 6 {
		t.Fatalf("M marks every file: %d", len(a.fmarks))
	}
	a.MarkAll()
	if len(a.fmarks) != 0 {
		t.Fatalf("M again takes every mark away: %d", len(a.fmarks))
	}
	a.MarkAll()
	press := func(k Key) {
		a.Dispatch(Event{Key: k, Down: true})
		a.Dispatch(Event{Key: k, Down: false})
	}
	press(A)
	if a.menu == nil || !strings.Contains(screenStr(a), "By their numbers") {
		t.Fatalf("numbered files offer to go by their numbers:\\n%s", screenStr(a))
	}
	a.menu.items[0].run(0)
	a.pickerKey(Down) // bank A stays: any pad of it
	a.pickerKey(Up)
	a.pickerKey(A)
	for n, want := range map[int]string{2: "03_Loop1_01", 11: "12_Clap_01", 13: "14_Kick_01"} {
		if p := a.info.Pads[n]; !p.File || p.Name != want {
			t.Errorf("pad %d is %q, wanted %q", n+1, p.Name, want)
		}
	}
	for _, n := range []int{0, 1, 15} {
		if a.info.Pads[n].File {
			t.Errorf("pad %d is empty: the EMPTY files and number 0 have no pad", n+1)
		}
	}
	if !strings.Contains(a.msg, "no pad for 00_DrumLoop_01.wav") || !strings.Contains(a.msg, "2 empty left out") {
		t.Errorf("it says what it left out: %q", a.msg)
	}
}

func TestTheDebugLogSaysWhatHappened(t *testing.T) {
	termOn(t)
	path := filepath.Join(t.TempDir(), "debug.log")
	if _, err := dbg.Set(dbg.Basic, path); err != nil {
		t.Fatal(err)
	}
	defer dbg.Set(dbg.Off, "")
	a, _ := newApp(t)
	waitLink(t, a)
	dev, _ := a.unitDevice()
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	kitKeys(t, a)
	a.liveSync()
	a.Dispatch(Event{Key: Right, Down: true})
	a.say("a message", true)
	time.Sleep(50 * time.Millisecond)
	b, _ := os.ReadFile(path)
	txt := string(b)
	for _, want := range []string{"unit: found", "live: start", "pad assigned", "live: changes found", "msg=\"a message\"", "level=WARN", "msg=key"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the debug log has %q:\n%s", want, txt)
		}
	}
	a.GoScreen(5)
	for i, it := range a.cfgItems() {
		if strings.HasPrefix(it.label, "Debug log") {
			a.cfgAt = i
		}
	}
	if txt := screenStr(a); !strings.Contains(txt, "Debug log") || !strings.Contains(txt, "The log is written to") {
		t.Fatalf("CONFIG shows the log and where it is written:\n%s", txt)
	}
}

func TestASampleAssignedWhileEditingLiveReachesTheUnit(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	dev := device.New(sim.Demo(), "sim")
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	// a pad that is empty on the unit gets a sample in the editor
	pad := -1
	for n := 0; n < 16; n++ {
		pi, _ := proto.NewPadIndex(uint16(n))
		if blk, err := dev.PadBlock(pi); err == nil && !blk.HasSample() {
			pad = n
			break
		}
	}
	if pad < 0 {
		t.Skip("the simulated unit has every pad of bank A in use")
	}
	id := a.lab.Decode
	_ = id
	kitKeys(t, a)
	for n := range a.info.Pads {
		if a.info.Pads[n].File && n != pad && pad > 2 {
			break
		}
	}
	a.GoScreen(0)
	src, _ := os.ReadFile(filepath.Join(a.Root, "bass", "bass_C1.wav"))
	mb, err := a.lab.Decode(src, ".wav")
	if err != nil {
		t.Fatal(err)
	}
	var m lab.Meta
	json.Unmarshal(mb, &m)
	if err := a.assignPad(pad, m.ID, "fresh"); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	a.liveSync()
	for i := 0; i < 500; i++ {
		if w, _, _ := a.liveStatus(); w == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.liveTick()
	if _, _, err := a.liveStatus(); err != nil {
		t.Fatalf("sending the sample failed: %v", err)
	}
	if strings.Contains(a.msg, "live:") && a.msgBad {
		t.Fatalf("the unit refused: %s", a.msg)
	}
	pi, _ := proto.NewPadIndex(uint16(pad))
	blk, err := dev.PadBlock(pi)
	if err != nil || !blk.HasSample() {
		t.Fatalf("the unit has the sample on pad %d: %v", pad+1, err)
	}
}

func TestMOnPadsMarksAndMUnmarksAll(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a) // A1..A3 have samples
	a.GoScreen(0)
	a.bank, a.cur = 0, 0
	a.MarkPad()
	if len(a.marks) != 1 || a.marks[0] != 0 {
		t.Fatalf("m marks the pad: %v", a.marks)
	}
	a.MarkPad()
	if len(a.marks) != 0 {
		t.Fatalf("m again unmarks it: %v", a.marks)
	}
	a.MarkAllPads()
	if len(a.marks) != 3 {
		t.Fatalf("M marks every pad of the bank with a sample: %v", a.marks)
	}
	a.MarkAllPads()
	if len(a.marks) != 0 {
		t.Fatalf("M again unmarks them all: %v", a.marks)
	}
	if txt := screenStr(a); !strings.Contains(txt, "mark all / none") {
		t.Fatalf("the panel names the keys:\n%s", txt)
	}
}

func TestRKeyRenamesThePad(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	a.GoScreen(0)
	a.bank, a.cur = 0, 1
	old := a.info.Pads[1].Name
	a.RenamePadKey()
	if a.prompt == nil || a.prompt.buf != old || !strings.Contains(strings.ToLower(screenStr(a)), "rename "+strings.ToLower(a.label(1))) {
		t.Fatalf("r asks for the name, with the old one in it:\n%s", screenStr(a))
	}
	a.prompt.buf = "my snare"
	a.TypeEnter()
	if a.info.Pads[1].Name != "my snare" {
		t.Fatalf("the pad has the new name: %q", a.info.Pads[1].Name)
	}
	a.GoScreen(1)
	a.RenamePadKey() // the track in front on SEQ
	if a.scr != Seq {
		t.Fatal("still SEQ")
	}
	a.prompt = nil
	a.cur = 5 // an empty pad
	a.GoScreen(0)
	a.RenamePadKey()
	if a.prompt != nil {
		t.Fatal("a pad with no sample has no name to change")
	}
}

func TestProjectsCanBeOpenedFromAnyFolder(t *testing.T) {
	termOn(t)
	src, _ := newApp(t)
	kitKeys(t, src)
	src.slot = 4
	parent := t.TempDir()
	if _, err := src.exportTo(parent); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t)
	a.GoScreen(2)
	press := func(k Key) {
		a.Dispatch(Event{Key: k, Down: true})
		a.Dispatch(Event{Key: k, Down: false})
	}
	// the folder of a project: opened in the editor
	a.OpenProjectKey()
	if a.dp == nil {
		t.Fatal("o opens the folder chooser")
	}
	a.dp.dir = parent
	a.dpList("")
	if txt := screenStr(a); !strings.Contains(txt, "PROJECT_04") || !strings.Contains(txt, "project") {
		t.Fatalf("the chooser marks the project folder:\n%s", txt)
	}
	for i, n := range a.dp.names {
		if n == "PROJECT_04" {
			a.dp.cur = i + 1
		}
	}
	press(A)
	if a.ScreenName() != "PADS" || !a.info.Pads[0].File || a.slot != 4 {
		t.Fatalf("the project is open: %s, pad 1 %v, slot %d", a.ScreenName(), a.info.Pads[0].File, a.slot)
	}
	// a folder of projects: it is the offline folder, PROJECTS lists them
	a.GoScreen(2)
	a.openProjectFolder()
	os.Rename(filepath.Join(parent, "PROJECT_04"), filepath.Join(parent, "PROJECT_02"))
	a.dp.dir = parent
	a.dpList("")
	press(A) // use this folder
	if a.set.OfflineDir != parent || !a.offlineChosen || a.ScreenName() != "UNIT" {
		t.Fatalf("a folder of projects becomes the offline folder: %q %v %s", a.set.OfflineDir, a.offlineChosen, a.ScreenName())
	}
	if txt := screenStr(a); !strings.Contains(txt, "PROJECT_02") {
		t.Fatalf("PROJECTS lists them:\n%s", txt)
	}
	// a folder with no project in it
	empty := t.TempDir()
	a.openProjectFolder()
	a.dp.dir = empty
	a.dpList("")
	press(A)
	if !a.msgBad || !strings.Contains(a.msg, "is not a project") {
		t.Fatalf("it says so: %q", a.msg)
	}
}

func TestWhatATrackToolKeepsReachesTheUnitWhenEditingLive(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	dev := device.New(sim.Demo(), "sim")
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	kitKeys(t, a)
	a.liveSync()
	drain := func() {
		for i := 0; i < 500; i++ {
			if w, _, _ := a.liveStatus(); w == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	drain()
	a.GoScreen(1)
	a.pat = 5
	a.rows = nil
	a.ensurePattern()
	a.autoRows()
	pad := a.rowPad()
	a.euclidMenu()
	var keep *menuItem
	for i := range a.menu.items {
		if strings.HasPrefix(a.menu.items[i].label, "Keep it") {
			keep = &a.menu.items[i]
		}
	}
	if keep == nil {
		t.Fatal("the tool has a Keep it")
	}
	if w, _, _ := a.liveStatus(); w != 0 {
		t.Fatalf("a trial is not sent: %d waiting", w)
	}
	keep.run(0)
	drain()
	want, err := a.lab.PatternBytes(a.pat)
	if err != nil || len(want) == 0 {
		t.Fatalf("the pattern the tool made: %v", err)
	}
	slot, _ := proto.NewPadIndex(uint16(a.pat - 1))
	got, err := dev.ReadFile(formats.PatternPath(1, slot))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("the unit has the pattern that was kept: %v (%d bytes, wanted %d) pad %d", err, len(got), len(want), pad)
	}
	a.menu = nil
}

func TestEveryFillPatternIsAValidBarOfSixteenthSteps(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range fillPresets {
		if f.name == "" || seen[f.name] {
			t.Errorf("a name that is empty or used twice: %q", f.name)
		}
		seen[f.name] = true
		last := -1
		for _, s := range f.mask {
			if s < 0 || s > 15 || s <= last {
				t.Errorf("%s: the steps go 0 to 15 in order: %v", f.name, f.mask)
			}
			last = s
		}
		if len(f.mask) == 0 {
			t.Errorf("%s has no step", f.name)
		}
	}
	for _, want := range []string{"son clave 3-2", "son clave 2-3", "rumba clave 2-3", "cinquillo", "habanera / dembow"} {
		if !seen[want] {
			t.Errorf("the pattern %q is there", want)
		}
	}
	// the other direction of a clave is the first one half a bar later
	rot := func(mask []int) []int {
		var out []int
		for _, s := range mask {
			out = append(out, (s+8)%16)
		}
		sort.Ints(out)
		return out
	}
	byName := map[string][]int{}
	for _, f := range fillPresets {
		byName[f.name] = f.mask
	}
	for a, b := range map[string]string{"son clave 3-2": "son clave 2-3", "rumba clave 3-2": "rumba clave 2-3"} {
		if fmt.Sprint(rot(byName[a])) != fmt.Sprint(byName[b]) {
			t.Errorf("%s is %s half a bar later: %v", b, a, rot(byName[a]))
		}
	}
}

func TestReloadOnTheUnitAndTheHintAfterAPatternIsSent(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	waitLink(t, a)
	dev, _ := a.unitDevice()
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	// a pattern is written to the unit: the status says that it shows after a reload
	a.GoScreen(1)
	a.pat = 7
	a.ensurePattern()
	a.autoRows()
	a.toggle(a.rowPad(), 3)
	a.liveSync()
	for i := 0; i < 500; i++ {
		a.liveTick()
		if strings.Contains(a.msg, "shows there after a reload") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(a.msg, "shows there after a reload") {
		t.Fatalf("it says the pattern is written and where the reload is: %q", a.msg)
	}
	// the menu of the project has the reload, and it works
	a.GoScreen(2)
	a.unitCur = 0
	a.unitMenu()
	if !strings.Contains(screenStr(a), "Reload on the unit") {
		t.Fatalf("the project's menu offers the reload while it is edited live:\n%s", screenStr(a))
	}
	a.menu = nil
	a.reloadLive()
	for i := 0; i < 500; i++ {
		a.live.mu.Lock()
		done := a.live.reloaded
		a.live.mu.Unlock()
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the unit reloaded the project")
}

func TestCtrlLReloadsTheUnitFromAnyScreen(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	a.ReloadUnit()
	if !a.msgBad || !strings.Contains(a.msg, "no project is edited live") {
		t.Fatalf("without live editing it says so: %q", a.msg)
	}
	if strings.Contains(screenStr(a), "reload unit") {
		t.Fatal("the key is not listed when it does nothing")
	}
	waitLink(t, a)
	dev, _ := a.unitDevice()
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	if !strings.Contains(screenStr(a), "reload unit") {
		t.Fatalf("while editing live the panel lists it under ALWAYS:\n%s", screenStr(a))
	}
	a.GoScreen(4) // any screen
	a.ReloadUnit()
	if !strings.Contains(a.msg, "reloading") {
		t.Fatalf("it starts: %q", a.msg)
	}
}

func TestTheAppNoticesWhatTheUnitDoesByItself(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	waitLink(t, a)
	dev, _ := a.unitDevice()
	a.goLive(dev, 0)
	t.Cleanup(func() { a.stopLive() })
	l := a.live
	l.dev = dev

	// a pattern written on the unit comes into the editor (it has to be seen twice)
	a.GoScreen(1)
	a.pat = 9
	a.ensurePattern()
	a.autoRows()
	a.toggle(a.rowPad(), 2)
	a.liveSync()
	for i := 0; i < 500; i++ {
		if w, _, _ := a.liveStatus(); w == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	l.mu.Lock()
	l.quietUntil = time.Time{}
	l.mu.Unlock()
	mine, _ := a.lab.PatternBytes(9)
	other := append([]byte(nil), mine...)
	// a second note makes it another file: the unit's pattern 9 is not the editor's
	a.toggle(a.rowPad(), 9)
	other, _ = a.lab.PatternBytes(9)
	a.lab.SetPatternBytes(9, mine) // the editor has its own
	a.refresh()
	slot, _ := proto.NewPadIndex(8)
	if err := dev.WriteFile(formats.PatternPath(1, slot), other); err != nil {
		t.Fatal(err)
	}
	l.watchPatterns()
	l.watchPatterns()
	for i := 0; i < 20; i++ { // one event is taken in each round of the program
		a.liveTick()
	}
	got, _ := a.lab.PatternBytes(9)
	if !bytes.Equal(got, other) {
		t.Fatalf("the pattern the unit has is in the editor: %d bytes (unit's %d), %q", len(got), len(other), a.msg)
	}
	if w, _, _ := a.liveStatus(); w != 0 {
		t.Fatalf("and it is not sent back: %d waiting", w)
	}

	// another project opened on the unit: live editing of this one stops, and nothing more is sent to it
	if err := dev.SelectProject(2); err != nil {
		t.Fatal(err)
	}
	l.watchUnit()
	for i := 0; i < 5; i++ {
		a.liveTick()
	}
	if a.live != nil || !a.msgBad || !strings.Contains(a.msg, "has project 3 open now") {
		t.Fatalf("live editing stops when the unit opens another project: live %v %q", a.live != nil, a.msg)
	}
}

func TestSwitchingPatternsWhilePlayingLeavesTheLoopAlone(t *testing.T) {
	termOn(t)
	a, au := newApp(t)
	kitKeys(t, a)
	a.GoScreen(1)
	a.pat = 1
	a.ensurePattern()
	a.autoRows()
	a.toggle(a.rowPad(), 0)
	a.togglePlay()
	if !a.seqOn || a.playPat != 1 {
		t.Fatalf("pattern 1 plays: on %v, playing %d", a.seqOn, a.playPat)
	}
	loops := au.Loops
	a.SeqBank(1) // bank B: the view goes to pattern 17
	if a.pat != 17 || !a.seqOn || a.playPat != 1 {
		t.Fatalf("the view is on 17 and 1 still plays: pat %d, on %v, playing %d", a.pat, a.seqOn, a.playPat)
	}
	if au.Loops != loops {
		t.Fatalf("the loop was not replaced: %d loops, was %d", au.Loops, loops)
	}
	if a.playStep() != -1 {
		t.Fatal("no playhead in a pattern that does not play")
	}
	// an edit of the pattern in view does not reach the loop
	a.ensurePattern()
	a.autoRows()
	a.toggle(a.rowPad(), 3)
	a.replay()
	if au.Loops != loops {
		t.Fatalf("an edit of another pattern leaves the loop alone: %d, was %d", au.Loops, loops)
	}
	// Space stops what plays, Space plays the pattern in view
	a.togglePlay()
	if a.seqOn || a.playPat != 0 {
		t.Fatal("Space stops it")
	}
	a.togglePlay()
	if !a.seqOn || a.playPat != 17 || au.Loops != loops+1 {
		t.Fatalf("Space again plays the new pattern: on %v, playing %d, loops %d", a.seqOn, a.playPat, au.Loops)
	}
	// back on the playing pattern, an edit goes into the loop again
	a.SeqBank(-1)
	a.SeqBank(1)
	before := au.Loops
	a.toggle(a.rowPad(), 7)
	if au.Loops <= before {
		t.Fatalf("an edit of the pattern that plays is heard: %d, was %d", au.Loops, before)
	}
}

func TestTheEventLogBesideARecording(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	a.GoScreen(1)
	a.pat = 3
	a.ensurePattern()
	a.autoRows()
	a.toggle(a.rowPad(), 4)
	dir := t.TempDir()
	started := time.Now()
	s := &record.Session{Path: filepath.Join(dir, "take.wav"), Started: started, Tool: "the program itself"}
	a.startEventLog(s)
	at := func(ms int) time.Time { return started.Add(time.Duration(ms) * time.Millisecond) }
	a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 1, Data1: 48, Data2: 100, At: at(500)}) // pad 1 of bank A
	a.midiEvent(midiin.Event{Kind: midiin.NoteOff, Channel: 1, Data1: 48, At: at(700)})
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 16, Data2: 90, At: at(900)}) // CTRL 1 of bus 1
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 83, Data2: 12, At: at(1000)})
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 19, Data2: 127, At: at(1100)})
	a.midiEvent(midiin.Event{Kind: midiin.Start, At: at(1200)})
	a.perfNow = 3
	a.eventLogTick()
	a.stopEventLog()
	if a.evlog != nil {
		t.Fatal("the log is closed")
	}
	es, err := perflog.Read(perflog.PathFor(s.Path))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	var proj perflog.Entry
	for _, e := range es {
		kinds[e["k"].(string)]++
		if e["k"] == "project" {
			proj = e
		}
	}
	for _, k := range []string{"start", "project", "on", "off", "cc", "fx", "start", "pattern", "end"} {
		if kinds[k] == 0 {
			t.Errorf("the log has %q lines: %v", k, kinds)
		}
	}
	if kinds["fx"] != 2 || kinds["cc"] != 3 {
		t.Errorf("an effect and its switch are cc and fx: %v", kinds)
	}
	for _, e := range es {
		if e["k"] == "on" && (e["bank"] != "A" || e["pad"].(float64) != 1 || e["t"].(float64) < 0.49 || e["t"].(float64) > 0.51) {
			t.Errorf("the pad that was hit: %v", e)
		}
	}
	data, _ := proj["data"].(map[string]any)
	pads, _ := data["pads"].([]any)
	pats, _ := data["patterns"].([]any)
	if len(pads) != 3 || len(pats) == 0 {
		t.Fatalf("the project line has the pads and the patterns: %d pads, %d patterns", len(pads), len(pats))
	}
	if first := pads[0].(map[string]any); first["label"] != "A1" || first["name"] == "" || len(first["peaks"].([]any)) != 32 {
		t.Errorf("a pad has its name and a small picture: %v", first)
	}
	if p := pats[0].(map[string]any); p["label"] != "A3" || len(p["notes"].([]any)) != 1 {
		t.Errorf("the pattern with its notes: %v", p)
	}
}
