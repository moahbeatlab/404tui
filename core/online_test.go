package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func termOn(t *testing.T) {
	t.Helper()
	Terminal = true
	t.Cleanup(func() { Terminal = false; SetSize(MinCols, MinRows) })
	SetSize(120, 40)
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
	keys(t, a, kit)
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
	keys(t, a, kit)
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
	if a.SplashText("o") {
		t.Fatal("the letters are the splash's only while it is in front")
	}
}

func TestSeqMuteAndSolo(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	keys(t, a, kit)
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
	keys(t, a, kit)
	dir := t.TempDir()
	a.exportProjectAsk()
	if a.prompt == nil || !strings.Contains(screenStr(a), "EXPORT THE PROJECT INTO THE FOLDER") {
		t.Fatal("the export asks for a folder")
	}
	a.prompt.buf = dir
	a.TypeEnter()
	if _, err := os.Stat(filepath.Join(dir, "PROJECT_01", "PADCONF.BIN")); err != nil {
		t.Fatalf("the project is in the folder that was typed: %v", err)
	}
	// a project of the unit goes into a folder too
	waitLink(t, a)
	a.GoScreen(2)
	a.unitCur = 0
	a.unitMenu()
	if !strings.Contains(screenStr(a), "Save it into a folder") {
		t.Fatalf("the unit's project menu offers a folder:\n%s", screenStr(a))
	}
	a.menu = nil
	dev, _ := a.unitDevice()
	out := filepath.Join(t.TempDir(), "backup")
	a.unitBackupAsk(dev, 0)
	if a.prompt == nil {
		t.Fatal("the backup asks for a folder")
	}
	a.prompt.buf = out
	a.TypeEnter()
	for i := 0; i < 500 && a.job != nil; i++ {
		a.Tick()
		if _, _, _, _, fin := a.job.state(); fin {
			a.Tick()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(out, "PADCONF.BIN")); err != nil {
		t.Fatalf("the unit's project is in the folder: %v", err)
	}
}
