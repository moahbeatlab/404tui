package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// ONLINE and OFFLINE, on a computer. Online the PROJECTS screen lists the 16 projects of the unit that is plugged in; offline it lists
// the projects of a folder on this computer (PROJECT_01 .. PROJECT_16, each a folder with PADCONF.BIN: what SAVE writes, and what a
// backup of the unit is). The program looks for the unit all the time, and says so at the start; the project that was open last
// offline is remembered, so that it can be opened again.

// onlineNow says whether PROJECTS shows the unit: one is plugged in, and offline was not chosen.
func (a *App) onlineNow() bool { return a.link != nil && !a.offlineChosen }

// offlineDir is the folder of projects worked on without the unit: the one chosen, else the card folder SAVE writes to.
func (a *App) offlineDir() string {
	if a.set.OfflineDir != "" {
		return expandHome(a.set.OfflineDir)
	}
	return filepath.Join(a.OutDir, cardFolder)
}

// offlineProject is the folder of project n (0..15) of the offline folder, when it holds one.
func (a *App) offlineProject(n int) (string, bool) {
	dir := filepath.Join(a.offlineDir(), fmt.Sprintf("PROJECT_%02d", n+1))
	if _, err := os.Stat(filepath.Join(dir, "PADCONF.BIN")); err != nil {
		return "", false
	}
	return dir, true
}

// openOffline opens project n (0..15) of the offline folder in the editor, and remembers it as the last one.
func (a *App) openOffline(n int) {
	dir, ok := a.offlineProject(n)
	if !ok {
		a.say(fmt.Sprintf("no project %d in %s", n+1, short2(a.offlineDir(), 30)), true)
		return
	}
	if err := a.OpenProject(dir); err != nil {
		a.say("cannot open it: "+err.Error(), true)
		return
	}
	a.slot = n + 1
	a.sl, a.pat, a.step = nil, 1, 0
	a.bank, a.cur = 0, 0
	a.saved = true
	a.changed = false
	a.set.LastOffline = n + 1
	a.saveSettings()
	a.say(fmt.Sprintf("opened %s offline", filepath.Base(dir)), false)
	a.go_(Pads)
}

// lastOffline is the project that was open last offline (0..15), and whether it is still there.
func (a *App) lastOffline() (int, bool) {
	n := a.set.LastOffline - 1
	if n < 0 || n > 15 {
		return 0, false
	}
	_, ok := a.offlineProject(n)
	return n, ok
}

// offlineMenu is what can be done with a project of the offline folder.
func (a *App) offlineMenu() {
	n := a.unitCur
	dir, ok := a.offlineProject(n)
	m := &menu{title: fmt.Sprintf("project %d  %s", n+1, filepath.Base(dir))}
	if !ok {
		m.title = fmt.Sprintf("project %d  (empty)", n+1)
	}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	if ok {
		add("Open it in the editor", func(int) { a.openOffline(n) })
	}
	add("Open a project from a folder...", func(int) { a.openProjectFolder() })
	add("Write the project of the editor here", func(int) {
		a.slot = n + 1
		names, err := a.exportTo(a.offlineDir())
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		a.wrote, a.saved = names, true
		a.set.LastOffline = n + 1
		a.saveSettings()
		a.say(fmt.Sprintf("wrote %d files as PROJECT_%02d in %s", len(names), n+1, short2(a.offlineDir(), 30)), false)
	})
	add("Export the project of the editor into a folder...", func(int) { a.exportProjectAsk() })
	a.menu = m
}

// modeMenu is t on PROJECTS: work online or offline, and the folder of the offline projects.
func (a *App) modeMenu() {
	m := &menu{title: "projects: online or offline"}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	m.head("WORK")
	if a.link != nil {
		add("Online: the projects of the unit ("+fit(a.link.Name(), 24)+")", func(int) { a.offlineChosen = false; a.say("online: the projects of the unit", false) })
	} else {
		add("Online: no unit is plugged in (looking for it)", func(int) {})
	}
	if a.link != nil {
		add("Online: load the unit's open project and edit it live", func(int) { a.go_(Unit); a.goOnline() })
	}
	add("Offline: the projects in "+short2(a.offlineDir(), 30), func(int) {
		a.offlineChosen = true
		a.say("offline: the projects of "+short2(a.offlineDir(), 30), false)
	})
	m.head("OTHER PROJECTS")
	add("Open a project from a folder...", func(int) { a.openProjectFolder() })
	m.head("OFFLINE FOLDER")
	add("Choose the folder of the offline projects...", func(int) {
		a.pickFolder("the folder of the offline projects", a.offlineDir(), func(dir string) {
			a.set.OfflineDir = dir
			a.offlineChosen = true
			a.saveSettings()
			a.say("offline: the projects of "+dir, false)
		})
	})
	if _, ok := a.lastOffline(); ok {
		add(fmt.Sprintf("Open PROJECT_%02d again (the last one offline)", a.set.LastOffline), func(int) { a.openOffline(a.set.LastOffline - 1) })
	}
	a.menu = m
}

// exportProjectAsk opens the folder chooser for the folder the project of the editor is written into, as PROJECT_nn with PADCONF.BIN, PTN and SMPL:
// the card of the unit, a drive, or a folder on this computer to send to someone.
func (a *App) exportProjectAsk() {
	a.pickFolder("export "+a.ProjectName()+" into the folder", a.exportRoot(), func(dir string) {
		a.set.ExportDir = dir
		a.saveSettings()
		a.freeTarget(dir, a.ProjectName(), false, func(string) {
			names, err := a.exportTo(dir)
			if err != nil {
				a.say(err.Error(), true)
				return
			}
			a.say(fmt.Sprintf("wrote %d files to %s", len(names), filepath.Join(dir, a.ProjectName())), false)
		})
	})
}

// goOnline loads the project the unit has open into the editor and edits it live: the program is the unit's editor from then on.
// The load runs in the background (its progress shows on PROJECTS) and ends on the pads.
func (a *App) goOnline() {
	a.offlineChosen = false
	dev, ok := a.unitDevice()
	if !ok {
		return
	}
	n := 0
	if cur, err := dev.CurrentProject(); err == nil && cur < 16 {
		n = int(cur)
	}
	a.unitCur = n
	a.say(fmt.Sprintf("online: loading unit project %d, it is edited live", n+1), false)
	a.unitLoadNow(dev, n, true)
}

// openProjectFolder opens the folder chooser to take a project from anywhere: the folder of a project (the one with PADCONF.BIN in it,
// shown as "project") is opened in the editor; a folder that holds projects (PROJECT_01, PROJECT_02...) becomes the offline folder, so
// that PROJECTS lists them.
func (a *App) openProjectFolder() {
	start := a.set.LastProjectDir
	if start == "" {
		start = a.offlineDir()
	}
	a.pickFolder("open a project: its folder, or a folder of projects", start, func(dir string) {
		a.set.LastProjectDir = filepath.Dir(dir)
		if isProjectFolder(dir) {
			if err := a.OpenProject(dir); err != nil {
				a.say("cannot open it: "+err.Error(), true)
				return
			}
			var n int
			if _, err := fmt.Sscanf(filepath.Base(dir), "PROJECT_%d", &n); err == nil && n >= 1 && n <= 99 {
				a.slot = n
			}
			a.sl, a.pat, a.step = nil, 1, 0
			a.bank, a.cur = 0, 0
			a.saved = true
			a.changed = false
			a.saveSettings()
			a.say("opened "+dir, false)
			a.go_(Pads)
			return
		}
		if n := countProjects(dir); n > 0 {
			a.set.OfflineDir = dir
			a.offlineChosen = true
			a.saveSettings()
			a.say(fmt.Sprintf("offline: %d projects in %s", n, dir), false)
			a.go_(Unit)
			return
		}
		a.set.LastProjectDir = dir
		a.say(short2(dir, 40)+" is not a project and has none in it (a project has a PADCONF.BIN)", true)
	})
}

// isProjectFolder says whether a folder is a project of the unit: it has a PADCONF.BIN.
func isProjectFolder(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "PADCONF.BIN"))
	return err == nil
}

// countProjects is how many projects named PROJECT_nn a folder holds.
func countProjects(dir string) int {
	n := 0
	for i := 1; i <= 16; i++ {
		if isProjectFolder(filepath.Join(dir, fmt.Sprintf("PROJECT_%02d", i))) {
			n++
		}
	}
	return n
}
