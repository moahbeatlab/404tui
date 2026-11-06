package core

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// markedAudio are the marked audio files, in the order of their names as people count them (a folder at a time, clap_2 before clap_10).
func (a *App) markedAudio() []string {
	var out []string
	for _, p := range a.marked() {
		if audioExts[strings.ToLower(pathExt(p))] {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return app.NaturalLess(strings.ToLower(out[i]), strings.ToLower(out[j])) })
	return out
}

var leadingNumber = regexp.MustCompile(`^(\d+)[_\- .]`)

// padNumberOf is the number a file name starts with (03_Loop.wav is 3), and whether it has one.
func padNumberOf(path string) (int, bool) {
	m := leadingNumber.FindStringSubmatch(baseName(path))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// emptyName says whether a file is named as an empty slot of a kit (01_EMPTY.wav): it is left out.
func emptyName(path string) bool { return strings.Contains(strings.ToLower(baseName(path)), "empty") }

// silent says whether a decoded sample has no sound in it.
func (a *App) silent(id int) bool {
	pcm, err := a.lab.PCM(id)
	if err != nil {
		return false
	}
	for _, v := range pcm {
		if v > 8 || v < -8 {
			return false
		}
	}
	return true
}

// assignMarked puts the marked files on pads. When every file starts with a number (a kit: 03_Loop.wav, 12_Clap.wav) it offers to
// put each on the pad of that number in a bank; otherwise, or when asked, they go one after the other from a pad, in the order of their
// names. Files named EMPTY, and files with no sound in them, are left out either way.
func (a *App) assignMarked() {
	files := a.markedAudio()
	if len(files) == 0 {
		a.say("none of the marked items is an audio file", true)
		return
	}
	numbered := true
	for _, p := range files {
		if _, ok := padNumberOf(p); !ok && !emptyName(p) {
			numbered = false
		}
	}
	if !numbered {
		a.assignInOrder(files)
		return
	}
	m := &menu{title: fmt.Sprintf("%d marked files: where do they go?", len(files))}
	m.items = append(m.items,
		menuItem{label: "By their numbers: 03_... goes on pad 3 of a bank", run: func(int) { a.assignByNumber(files) }},
		menuItem{label: "One after the other, from a pad", run: func(int) { a.assignInOrder(files) }})
	a.menu = m
}

// assignByNumber asks for a bank (the pad dialog: any pad of it) and puts each file on the pad its number says.
func (a *App) assignByNumber(files []string) {
	a.pickPad("the bank for the numbered files (03_ is pad 3)", a.padN(), false, func(p int) {
		bank := p / 16
		var skippedEmpty, noPad []string
		done := 0
		for _, f := range files {
			n, ok := padNumberOf(f)
			if emptyName(f) || !ok {
				skippedEmpty = append(skippedEmpty, f)
				continue
			}
			if n < 1 || bank*16+n-1 > 159 {
				noPad = append(noPad, baseName(f))
				continue
			}
			id, err := a.decodePath(f, pathExt(f))
			if err != nil {
				a.say(baseName(f)+": "+err.Error(), true)
				continue
			}
			if a.silent(id) {
				skippedEmpty = append(skippedEmpty, f)
				continue
			}
			pad := bank*16 + n - 1
			name := strings.TrimSuffix(baseName(f), filepath.Ext(baseName(f)))
			if err := a.assignPad(pad, id, name); err != nil {
				a.say(err.Error(), true)
				continue
			}
			delete(a.pcm, pad)
			done++
		}
		a.finishAssign(bank*16, done)
		msg := fmt.Sprintf("%d files on bank %c by their numbers, %d empty left out", done, 'A'+bank, len(skippedEmpty))
		if len(noPad) > 0 {
			msg += fmt.Sprintf("; no pad for %s", strings.Join(noPad, ", "))
		}
		a.say(msg, false)
	})
}

// assignInOrder asks which pad the first file goes to (the pad dialog), then puts the files on that pad and the ones after it.
func (a *App) assignInOrder(all []string) {
	var files []string
	for _, f := range all {
		if !emptyName(f) {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		a.say("every marked file is an empty one", true)
		return
	}
	start := a.freeRun(len(files), a.padN())
	if start < 0 {
		start = a.padN()
	}
	title := fmt.Sprintf("put %d marked files on the pads from", len(files))
	a.pickPad(title, start, false, func(first int) {
		if first+len(files) > 160 {
			a.say(fmt.Sprintf("%d files do not fit from %s: the last pad is J16", len(files), a.label(first)), true)
			return
		}
		done, at := 0, first
		for _, p := range files {
			id, err := a.decodePath(p, pathExt(p))
			if err != nil {
				a.say(baseName(p)+": "+err.Error(), true)
				continue
			}
			if a.silent(id) { // no sound in it: no pad is used for it
				continue
			}
			name := strings.TrimSuffix(baseName(p), filepath.Ext(baseName(p)))
			if err := a.assignPad(at, id, name); err != nil {
				a.say(err.Error(), true)
				continue
			}
			delete(a.pcm, at)
			at++
			done++
		}
		a.finishAssign(first, done)
		a.say(fmt.Sprintf("%d files on %s to %s, in the order of their names", done, a.label(first), a.label(max(at-1, first))), false)
	})
}

// finishAssign is what follows putting marked files on pads: the marks go, the pads are shown from the bank of the first.
func (a *App) finishAssign(first, done int) {
	a.fmarks = nil
	a.aud = nil
	a.sl = nil
	a.refresh()
	a.stopPreview()
	a.replay()
	a.bank, a.cur = first/16, first%16
	a.go_(Pads)
}

// markAllHere marks every audio file of the folder in front; when they are all marked already it takes every mark away.
func (a *App) markAllHere() {
	all, any := true, false
	for _, e := range a.entries {
		if !e.dir {
			any = true
			if !a.fmarks[joinPath(a.dir, e.name)] {
				all = false
			}
		}
	}
	if any && all { // everything here is marked already: M takes every mark away again
		a.fmarks = nil
		a.say("nothing marked", false)
		return
	}
	if a.fmarks == nil {
		a.fmarks = map[string]bool{}
	}
	for _, e := range a.entries {
		if !e.dir {
			a.fmarks[joinPath(a.dir, e.name)] = true
		}
	}
	a.say(fmt.Sprintf("%d marked", len(a.fmarks)), false)
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
	a.tagItems(m)
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
