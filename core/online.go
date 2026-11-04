package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	add("Offline: the projects in "+short2(a.offlineDir(), 30), func(int) {
		a.offlineChosen = true
		a.say("offline: the projects of "+short2(a.offlineDir(), 30), false)
	})
	m.head("OFFLINE FOLDER")
	add("Choose the folder of the offline projects...", func(int) {
		a.askText("folder of the offline projects", "PROJECT_01 .. PROJECT_16, each with PADCONF.BIN in it", a.offlineDir(), func(s string) {
			if s == "" {
				return
			}
			a.set.OfflineDir = strings.TrimSpace(s)
			a.offlineChosen = true
			a.saveSettings()
		})
	})
	if _, ok := a.lastOffline(); ok {
		add(fmt.Sprintf("Open PROJECT_%02d again (the last one offline)", a.set.LastOffline), func(int) { a.openOffline(a.set.LastOffline - 1) })
	}
	a.menu = m
}

// exportProjectAsk asks for the folder the project of the editor is written into, as PROJECT_nn with PADCONF.BIN, PTN and SMPL:
// the card of the unit, a drive, or a folder on this computer to send to someone.
func (a *App) exportProjectAsk() {
	def := a.offlineDir()
	a.askText("export the project into the folder", fmt.Sprintf("%s is made in it; Enter exports", a.ProjectName()), def, func(s string) {
		if s == "" {
			return
		}
		dir := expandHome(s)
		names, err := a.exportTo(dir)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		a.say(fmt.Sprintf("wrote %d files to %s", len(names), short2(filepath.Join(dir, a.ProjectName()), 40)), false)
	})
}
