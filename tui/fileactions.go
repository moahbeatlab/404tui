package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// Actions on the files marked in the Files list (or the one in front when none is marked):
// delete, copy, cut and paste, new folder. Marking is v for one, V for every file in the
// folder in front, J for the junk operating systems leave on a card.

// fileTargets are the paths an action applies to: the marked ones, or else the one in front.
func (m Model) fileTargets() []string {
	if len(m.fileMarks) > 0 {
		paths := make([]string, 0, len(m.fileMarks))
		for p := range m.fileMarks {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		return paths
	}
	if node, ok := m.fileNode(); ok {
		return []string{node.Path}
	}
	return nil
}

// toggleMark marks or unmarks the row in front, and moves down.
func (m Model) toggleMark(node app.Node) Model {
	if m.fileMarks[node.Path] {
		delete(m.fileMarks, node.Path)
	} else {
		m.fileMarks[node.Path] = true
	}
	m.fileAt = clamp(m.fileAt+1, 0, len(m.tree.Rows())-1)
	return m
}

// markFolder marks every file in the folder in front (the open ones below it too).
func (m Model) markFolder() Model {
	folder := m.tree.Folder(m.fileAt)
	n := 0
	for _, r := range m.tree.Rows() {
		if r.Dir || !strings.HasPrefix(r.Path, folder) || app.IsJunk(r.Name) {
			continue
		}
		m.fileMarks[r.Path] = true
		n++
	}
	m.message = fmt.Sprintf("marked %d files in %s", n, display(folder))
	return m
}

// markJunk marks the files and folders in the open folders that operating systems leave on
// a card: macOS's ._ companions and .DS_Store, Windows' Thumbs.db and desktop.ini.
func (m Model) markJunk() Model {
	n := 0
	for _, r := range m.tree.Rows() {
		if app.IsJunk(r.Name) {
			m.fileMarks[r.Path] = true
			n++
		}
	}
	if n == 0 {
		m.message = "no junk files in the open folders (open more folders with Tab)"
	} else {
		m.message = fmt.Sprintf("marked %d junk files; d deletes them", n)
	}
	return m
}

// copyFiles puts the targets on the clipboard, to copy or (cut) to move; p pastes them.
func (m Model) copyFiles(cut bool) Model {
	targets := m.fileTargets()
	if len(targets) == 0 {
		return m
	}
	if cut {
		for _, p := range targets {
			if app.VolumeOf(p) != app.VolumeCard {
				m.message = fmt.Sprintf("%s is on %s: only files on the SD card can be moved; copy them instead (c)", app.Base(p), app.VolumeOf(p))
				return m
			}
		}
	}
	m.clip, m.clipCut = targets, cut
	m.fileMarks = map[string]bool{}
	verb := "to copy"
	if cut {
		verb = "to move"
	}
	m.message = fmt.Sprintf("%s %s: open a folder, on this source or another (t), and press p", plural(len(targets), "item"), verb)
	return m
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// paste puts the clipboard into the folder in front. Nothing is overwritten.
func (m Model) paste() (tea.Model, tea.Cmd) {
	if len(m.clip) == 0 {
		m.message = "nothing on the clipboard: c copies and x moves the marked files"
		return m, nil
	}
	dest := m.tree.Folder(m.fileAt)
	if app.VolumeOf(dest) == app.VolumeUnit {
		m.message = "files go to the SD card or this computer: the unit's own storage holds your projects"
		return m, nil
	}
	sources, cut, unit := m.clip, m.clipCut, m.unit
	if cut {
		m.clip = nil
	}
	m.busy = "copying"
	if cut {
		m.busy = "moving"
	}
	cmd := m.afterFiles(func() string {
		res := app.CopyPaths(unit, sources, dest, cut, nil)
		verb := "copied"
		if cut {
			verb = "moved"
		}
		var note string
		switch len(res.Done) {
		case 0:
			note = "nothing " + verb
		case 1:
			note = fmt.Sprintf("%s to %s", verb, display(res.Done[0]))
		default:
			note = fmt.Sprintf("%d %s into %s", len(res.Done), verb, display(dest))
		}
		if len(res.Failed) > 0 {
			note += "; not " + verb + ": " + strings.Join(res.Failed, ", ")
		}
		return note
	})
	return m, cmd
}

// deleteFiles asks to delete the targets, after counting what is in them.
func (m Model) deleteFiles() (tea.Model, tea.Cmd) {
	targets := m.fileTargets()
	if len(targets) == 0 {
		return m, nil
	}
	for _, p := range targets {
		switch app.VolumeOf(p) {
		case app.VolumeCard:
		case app.VolumeComputer:
			m.message = "files on this computer are not deleted from here; use Finder"
			return m, nil
		default:
			m.message = "the unit's own storage holds your projects: delete samples from pads instead"
			return m, nil
		}
	}
	unit := m.unit
	m.busy = "counting"
	return m, m.edit(func() tea.Msg {
		count, err := app.CountPaths(unit, targets)
		return deletePlanMsg{paths: targets, count: count, err: err}
	})
}

// deletePlanMsg carries what a delete would remove, to be confirmed first.
type deletePlanMsg struct {
	paths []string
	count app.Count
	err   error
	// reopen is the pad whose assign dialog the question came from, to open it again after.
	reopen *proto.PadIndex
}

// reopenAssignMsg opens the assign dialog again after something done from it.
type reopenAssignMsg struct {
	pad  proto.PadIndex
	note string
}

// deleteBox is the question before files are deleted.
func (m Model) deleteBox() string {
	st, p := m.st, m.prompt
	lines := []string{st.bad.Render(fmt.Sprintf("Delete %s from the SD card?", plural(len(p.paths), "item"))), ""}
	for _, path := range p.paths[:min(len(p.paths), 8)] {
		lines = append(lines, "  "+st.text.Render(app.Base(path)))
	}
	if len(p.paths) > 8 {
		lines = append(lines, st.dim.Render(fmt.Sprintf("  … and %d more", len(p.paths)-8)))
	}
	what := plural(p.count.Files, "file")
	if p.count.Folders > 0 {
		what += " and " + plural(p.count.Folders, "folder")
	}
	lines = append(lines, "", st.warn.Render(what+", with everything in them; this cannot be undone."), "",
		st.dim.Render("y delete   n/Esc keep"))
	return st.modal.Render(strings.Join(lines, "\n"))
}

// newFolder asks for the name of a folder in the folder in front.
func (m Model) newFolder() (tea.Model, tea.Cmd) {
	dir := m.tree.Folder(m.fileAt)
	if app.VolumeOf(dir) == app.VolumeUnit {
		m.message = "folders are made on the SD card or this computer, not in the unit's own storage"
		return m, nil
	}
	m.prompt = &prompt{kind: promptMkdir, path: dir}
	return m, nil
}

// clipMarker is the symbol shown beside a file on the clipboard.
func (m Model) clipMarker(path string) string {
	if !slices.Contains(m.clip, path) {
		return ""
	}
	if m.clipCut {
		return "↗"
	}
	return "⧉"
}

// folderMatches is the rows of the open folders whose name contains text, in list order;
// empty text matches them all.
func folderMatches(rows []app.Node, text string) []int {
	want := strings.ToLower(strings.TrimSpace(text))
	var out []int
	for i, r := range rows {
		if r.Dir && strings.Contains(strings.ToLower(r.Name), want) {
			out = append(out, i)
		}
	}
	return out
}

// stepChoice moves a choice in a list of n by the key's direction, wrapping round.
func stepChoice(choice, n int, key string) int {
	if n == 0 {
		return 0
	}
	switch key {
	case "down", "ctrl+n", "tab":
		return (choice + 1) % n
	case "up", "ctrl+p", "shift+tab":
		return (choice + n - 1) % n
	}
	return clamp(choice, 0, n-1)
}

// findFolder moves the file cursor to the chosen one of the folders matching text.
func (m Model) findFolder(text string, choice int) Model {
	rows := m.tree.Rows()
	matches := folderMatches(rows, text)
	if len(matches) == 0 {
		m.message = fmt.Sprintf("no open folder named like %q (open more with Tab)", text)
		return m
	}
	at := matches[clamp(choice, 0, len(matches)-1)]
	m.fileAt = at
	m.message = rows[at].Name + ": Tab or h collapses it"
	return m
}

// survivor is where a cursor on path should land once deleted is gone: the path itself, or
// when it was deleted (or sat inside a deleted folder), the nearest folder above that stays.
func survivor(path string, deleted []string) string {
	gone := func(p string) bool {
		for _, d := range deleted {
			if p == d || strings.HasPrefix(p, d+"/") {
				return true
			}
		}
		return false
	}
	for path != "" && gone(path) {
		up := app.Dir(path)
		if up == path {
			return ""
		}
		path = up
	}
	return path
}
