package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/app"
)

// File operations in the browser: marking files and folders, copy, cut and paste (also between the computer and the unit's card),
// a new folder, rename, delete (on the card and the unit; on the computer only what the program put there is deleted by hand).

func (a *App) marked() []string {
	var out []string
	for p := range a.fmarks {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// toggleFileMark marks the entry under the cursor, or takes the mark off.
func (a *App) toggleFileMark() {
	e, ok := a.current()
	if !ok {
		return
	}
	p := joinPath(a.dir, e.name)
	if a.fmarks == nil {
		a.fmarks = map[string]bool{}
	}
	if a.fmarks[p] {
		delete(a.fmarks, p)
	} else {
		a.fmarks[p] = true
	}
	a.fcur = min(a.fcur+1, max(len(a.entries)-1, 0))
}

// fileTargets are the marked paths, or the one under the cursor.
func (a *App) fileTargets() []string {
	if m := a.marked(); len(m) > 0 {
		return m
	}
	if e, ok := a.current(); ok {
		return []string{joinPath(a.dir, e.name)}
	}
	return nil
}

// filesMenu is what can be done with the files: mark, copy, cut, paste, a new folder, rename, delete.
func (a *App) filesMenu() {
	n := len(a.fileTargets())
	m := &menu{title: "files"}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	m.head("MARKS")
	add("Mark every file here", func(int) {
		if a.fmarks == nil {
			a.fmarks = map[string]bool{}
		}
		for _, e := range a.entries {
			a.fmarks[joinPath(a.dir, e.name)] = true
		}
		a.say(fmt.Sprintf("%d marked", len(a.fmarks)), false)
	})
	add("Unmark everything", func(int) { a.fmarks = nil; a.say("nothing marked", false) })
	m.head(fmt.Sprintf("THE %s", map[bool]string{true: "MARKED", false: "FILE"}[len(a.fmarks) > 0]))
	add(fmt.Sprintf("Copy (%d)", n), func(int) {
		a.fclip, a.fcut = a.fileTargets(), false
		a.say(fmt.Sprintf("%d copied: paste in another folder", len(a.fclip)), false)
	})
	add(fmt.Sprintf("Cut (%d)", n), func(int) {
		a.fclip, a.fcut = a.fileTargets(), true
		a.say(fmt.Sprintf("%d cut: paste in another folder", len(a.fclip)), false)
	})
	add(fmt.Sprintf("Paste here (%d)", len(a.fclip)), func(int) { a.pasteFiles() })
	add("Delete...", func(int) { a.deleteFilesAsk() })
	add("Rename...", func(int) { a.renameFile() })
	m.head("THIS FOLDER")
	add("New folder...", func(int) { a.newFolder() })
	a.menu = m
}

func (a *App) pasteFiles() {
	if len(a.fclip) == 0 {
		a.say("nothing copied or cut", true)
		return
	}
	src, cut, dest := a.fclip, a.fcut, a.dir
	st := a.fileStore()
	var res app.Moved
	a.startJob(0, func(progress func(done, total int, name string)) error {
		res = app.CopyPaths(st, src, dest, cut, func(done int, p string) { progress(done, len(src), baseName(p)) })
		return nil
	}, func(error) {
		a.fclip = nil
		a.list()
		msg := fmt.Sprintf("%d %s", len(res.Done), map[bool]string{true: "moved", false: "copied"}[cut])
		if len(res.Failed) > 0 {
			msg += fmt.Sprintf(", %d failed: %s", len(res.Failed), res.Failed[0])
		}
		a.say(msg, len(res.Failed) > 0)
	})
}

func (a *App) deleteFilesAsk() {
	t := a.fileTargets()
	if len(t) == 0 {
		return
	}
	for _, p := range t {
		if app.VolumeOf(p) == app.VolumeComputer {
			a.say("deleting is done on the unit and its card; on the computer use the computer's own tools", true)
			return
		}
	}
	m := &menu{title: fmt.Sprintf("delete %d: %s?", len(t), fit(baseName(t[0]), 20))}
	m.items = append(m.items,
		menuItem{label: "Yes, delete them (folders with everything in them)", run: func(int) {
			st := a.fileStore()
			var del int
			var failed []string
			a.startJob(0, func(progress func(done, total int, name string)) error {
				del, failed = app.DeletePaths(st, t, func(done int, p string) { progress(done, len(t), baseName(p)) })
				return nil
			}, func(error) {
				a.fmarks = nil
				a.list()
				msg := fmt.Sprintf("%d deleted", del)
				if len(failed) > 0 {
					msg += ", failed: " + failed[0]
				}
				a.say(msg, len(failed) > 0)
			})
		}},
		menuItem{label: "No", run: func(int) {}})
	a.menu = m
}

func (a *App) renameFile() {
	e, ok := a.current()
	if !ok {
		return
	}
	from := joinPath(a.dir, e.name)
	a.askText("new name of "+fit(e.name, 24), "", e.name, func(s string) {
		if s == "" || s == e.name {
			return
		}
		if err := a.fileStore().Rename(from, joinPath(a.dir, s)); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.list()
		a.say("renamed to "+s, false)
	})
}

func (a *App) newFolder() {
	a.askText("name of the new folder", "", "", func(s string) {
		if s == "" || strings.ContainsAny(s, "/\\") {
			return
		}
		if err := a.fileStore().Mkdir(joinPath(a.dir, s)); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.list()
		a.say("folder "+s+" made", false)
	})
}
