package core

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// the launcher of a computer: the doors, the projects opened last, Enter goes on with the last one, 1-4 open one, f a folder
func TestLauncherDoorsAndRecentProjects(t *testing.T) {
	termOn(t)
	a, _ := newApp(t)
	kitKeys(t, a)
	a.slot = 3
	dir := t.TempDir()
	if _, err := a.exportTo(dir); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, "PROJECT_03")
	a.openFolderProject(proj)
	if rs := a.recent(); len(rs) != 1 || rs[0].Dir != proj || rs[0].Pads == 0 {
		t.Fatalf("an opened folder is a recent project: %+v", rs)
	}
	a.RememberProject(filepath.Join(a.autosaveRoot(), "PROJECT_03"))
	if len(a.set.Recent) != 1 {
		t.Fatal("an autosave is not a recent project")
	}

	// a new start, nothing open: the launcher waits, and Enter continues with the project opened last
	c := New(a.Root, a.OutDir, nil)
	c.ShowSplash(time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	c.Tick()
	if c.splash == nil {
		t.Fatal("the launcher waits for a choice")
	}
	txt := screenStr(c)
	for _, want := range []string{"Continue", "PROJECT_03", "Unit, online", "Offline card folder", "Open a folder", "RECENT", "1  recent"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the launcher says %q:\n%s", want, txt)
		}
	}
	c.Press(A, 0)
	if c.splash != nil || padsWithSamples(c) == 0 || c.slot != 3 || c.ScreenName() != "PADS" {
		t.Fatalf("Enter opens the last project: %d pads, slot %d, %s", padsWithSamples(c), c.slot, c.ScreenName())
	}

	// with the project open, Enter only carries on
	c.ShowSplash(time.Hour)
	if txt := screenStr(c); !strings.Contains(txt, "PROJECT_03  ") || !strings.Contains(txt, "pads with samples") {
		t.Errorf("Continue says the open project:\n%s", txt)
	}
	c.Press(A, 0)
	if c.splash != nil || c.Modal() {
		t.Fatal("Enter carries on with what is open")
	}

	// 1 opens the first recent project; a number with no project does nothing
	c.ShowSplash(time.Hour)
	if !c.SplashText("2") || c.splash == nil {
		t.Fatal("2 with one recent project leaves the launcher where it is")
	}
	if !c.SplashText("1") || c.splash != nil || c.ScreenName() != "PADS" {
		t.Fatal("1 opens the first recent project")
	}
	// f opens the folder chooser
	c.ShowSplash(time.Hour)
	if !c.SplashText("f") || c.splash != nil || !c.Modal() {
		t.Fatal("f opens a project from a folder")
	}
}
