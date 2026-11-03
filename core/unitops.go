package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
)

// What the UNIT screen does with the projects of a plugged-in SP-404MKII besides sending one: load it into the editor, back it
// up to this computer, restore a backup over it, rename it, make it the current one, erase it.

// unitDevice is the device session of the link, when it is one.
func (a *App) unitDevice() (*device.Device, bool) {
	l, ok := a.link.(DeviceLink)
	if !ok {
		return nil, false
	}
	return l.Dev, true
}

// startJob runs work in the background with its progress on the UNIT screen, and after (on the main loop) when it is finished.
func (a *App) startJob(project int, work func(progress func(done, total int, name string)) error, after func(err error)) {
	j := &job{project: project}
	a.job = j
	a.jobAfter = after
	go func() {
		err := work(func(done, total int, name string) {
			j.mu.Lock()
			j.done, j.total, j.name = done, total, name
			j.mu.Unlock()
		})
		j.mu.Lock()
		j.err, j.finished = err, true
		j.mu.Unlock()
	}()
}

// jobTick calls what was waiting for a job that has finished.
func (a *App) jobTick() {
	if a.job == nil || a.jobAfter == nil {
		return
	}
	if _, _, _, err, fin := a.job.state(); fin {
		f := a.jobAfter
		a.jobAfter = nil
		f(err)
	}
}

func (a *App) unitProjectName(n int) string {
	names, err := a.link.Projects()
	if err != nil || n >= len(names) {
		return ""
	}
	return names[n]
}

// unitMenu is what can be done with the project of the unit under the cursor.
func (a *App) unitMenu() {
	dev, ok := a.unitDevice()
	n := a.unitCur
	name := a.unitProjectName(n)
	m := &menu{title: fmt.Sprintf("unit project %d  %s", n+1, name)}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	add("Send the project of the editor here...", func(int) { a.confirm = true })
	if ok {
		if a.live != nil && a.live.project == n {
			add("Stop editing it live", func(int) { a.stopLive() })
		} else {
			add("Edit it live (changes go to the unit)", func(int) { a.unitLoadThen(dev, n, true) })
		}
		add("Load it into the editor", func(int) { a.unitLoad(dev, n) })
		add("Back it up to this computer", func(int) { a.unitBackup(dev, n) })
		add("Restore it from a backup...", func(int) { a.unitRestoreMenu(dev, n) })
		add("Rename...", func(int) { a.unitRename(dev, n, name) })
		add("Make it the unit's project", func(int) {
			if err := dev.SelectProject(uint8(n)); err != nil {
				a.say(err.Error(), true)
				return
			}
			a.say(fmt.Sprintf("project %d is the unit's project now", n+1), false)
		})
		add("Erase it...", func(int) { a.unitEraseAsk(dev, n) })
	}
	a.menu = m
}

// unitCache is where a project taken from the unit is kept.
func (a *App) unitCache(n int) string {
	return filepath.Join(a.dataDir(), "unit", fmt.Sprintf("PROJECT_%02d", n+1))
}

// unitLoad copies the project from the unit and opens it in the editor.
func (a *App) unitLoad(dev *device.Device, n int) { a.unitLoadThen(dev, n, false) }

// unitLoadThen loads the project and, when live is set, makes it the one that is edited live.
func (a *App) unitLoadThen(dev *device.Device, n int, live bool) {
	if a.hasWork() && !a.saved {
		m := &menu{title: "the editor has work that is not saved"}
		m.items = append(m.items,
			menuItem{label: "Replace it with the unit's project " + fmt.Sprint(n+1), run: func(int) { a.unitLoadNow(dev, n, live) }},
			menuItem{label: "No", run: func(int) {}})
		a.menu = m
		return
	}
	a.unitLoadNow(dev, n, live)
}

func (a *App) unitLoadNow(dev *device.Device, n int, live bool) {
	dir := a.unitCache(n)
	os.RemoveAll(dir)
	st := app.NewRouter(dev)
	a.startJob(n, func(progress func(done, total int, name string)) error {
		total, _ := app.ProjectFileCount(dev, uint8(n))
		_, err := app.ExportProject(st, uint8(n), dir, func(done int, name string) { progress(done, total, name) })
		return err
	}, func(err error) {
		if err != nil {
			a.say("load failed: "+err.Error(), true)
			return
		}
		if err := a.OpenProject(dir); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.live = nil
		a.go_(Pads)
		if live {
			a.goLive(dev, n)
			return
		}
		a.say(fmt.Sprintf("unit project %d is in the editor", n+1), false)
	})
}

// unitBackup copies the project from the unit into a folder of its own below the backups folder.
func (a *App) unitBackup(dev *device.Device, n int) {
	name := app.SanitizeName(a.unitProjectName(n))
	dir := filepath.Join(a.dataDir(), "backups", fmt.Sprintf("%02d_%s_%s", n+1, name, a.clock().Format("20060102_150405")))
	st := app.NewRouter(dev)
	a.startJob(n, func(progress func(done, total int, name string)) error {
		total, _ := app.ProjectFileCount(dev, uint8(n))
		_, err := app.ExportProject(st, uint8(n), dir, func(done int, name string) { progress(done, total, name) })
		return err
	}, func(err error) {
		if err != nil {
			a.say("backup failed: "+err.Error(), true)
			return
		}
		a.say("backed up to "+short2(dir, 40), false)
	})
}

// unitRestoreMenu lists the backups there are, and a folder to type, and asks before it writes over the project.
func (a *App) unitRestoreMenu(dev *device.Device, n int) {
	root := filepath.Join(a.dataDir(), "backups")
	ents, _ := os.ReadDir(root)
	var dirs []string
	for _, e := range ents {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	m := &menu{title: fmt.Sprintf("restore over unit project %d", n+1)}
	for _, d := range dirs {
		d := d
		m.items = append(m.items, menuItem{label: d, run: func(int) { a.unitRestoreAsk(dev, n, filepath.Join(root, d)) }})
	}
	m.items = append(m.items, menuItem{label: "A folder somewhere else...", run: func(int) {
		a.askText("folder of a project backup", "the folder with PADCONF.BIN in it", "", func(s string) { a.unitRestoreAsk(dev, n, expandHome(s)) })
	}})
	a.menu = m
}

func (a *App) unitRestoreAsk(dev *device.Device, n int, dir string) {
	plan, err := app.PlanProjectRestore(restoreSource{dev: dev}, uint8(n), dir, nil)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	m := &menu{title: fmt.Sprintf("replace unit project %d with %s?", n+1, short2(dir, 24))}
	m.items = append(m.items,
		menuItem{label: fmt.Sprintf("Yes: %d files, %.1f MB", len(plan.Files), float64(plan.Total)/1e6), run: func(int) {
			a.startJob(n, func(progress func(done, total int, name string)) error {
				total := len(plan.Files)
				return app.RestoreProject(dev, plan, func(i int, name string) { progress(i, total, name) })
			}, func(err error) {
				if err != nil {
					a.say("restore failed: "+err.Error(), true)
					return
				}
				a.say(fmt.Sprintf("unit project %d restored", n+1), false)
			})
		}},
		menuItem{label: "No", run: func(int) {}})
	a.menu = m
}

func (a *App) unitRename(dev *device.Device, n int, old string) {
	a.askText(fmt.Sprintf("name of unit project %d", n+1), "", old, func(s string) {
		if s == "" {
			return
		}
		if err := dev.SetProjectName(uint8(n), s); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.say(fmt.Sprintf("unit project %d is called %s", n+1, s), false)
	})
}

func (a *App) unitEraseAsk(dev *device.Device, n int) {
	m := &menu{title: fmt.Sprintf("erase unit project %d: samples, patterns and settings?", n+1)}
	m.items = append(m.items,
		menuItem{label: "Yes, erase it", run: func(int) {
			if err := app.EraseProject(dev, uint8(n)); err != nil {
				a.say(err.Error(), true)
				return
			}
			a.say(fmt.Sprintf("unit project %d erased", n+1), false)
		}},
		menuItem{label: "No", run: func(int) {}})
	a.menu = m
}

var _ = strings.TrimSpace
var _ = time.Second
