package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
)

// The ranger view shows the folder tree as three columns, as the ranger file manager does: the
// folder above, the folder in front, and a preview of what the cursor is on (the inside of a
// folder, the details of a file). It is the same tree as the plain view, so everything that
// works on the row under the cursor works here too; only moving about differs: j k go along
// the folder in front, h goes up a folder and l goes into one.

// rangerKey handles the keys that move about in the ranger view, and says whether it knew
// the key.
func (m Model) rangerKey(name string) (Model, tea.Cmd, bool) {
	rows := m.tree.Rows()
	at := m.fileAt
	node := rows[at]
	sib := m.tree.Siblings(at)
	pos := max(slices.Index(sib, at), 0)
	move := func(to int) (Model, tea.Cmd, bool) {
		m.fileAt = sib[clamp(to, 0, len(sib)-1)]
		return m, nil, true
	}
	switch name {
	case "j", "down":
		return move(pos + 1)
	case "k", "up":
		return move(pos - 1)
	case "g":
		return move(0)
	case "G":
		return move(len(sib) - 1)
	case "ctrl+d", "ctrl+u", "pgdown", "pgup":
		return move(pos + m.pageStep(name, m.geom().bodyH-2))
	case "h", "left", "backspace":
		if parent := m.tree.Parent(at); parent >= 0 {
			m.fileAt = parent
		}
		return m, nil, true
	case "l", "right", "tab", "enter":
		if !node.Dir {
			return m, nil, false
		}
		delete(m.autoOpen, node.Path)
		if !node.Expanded {
			m.busy = "listing files"
			return m, m.loadDirInto(node.Path, at, false), true
		}
		if kids := m.tree.Children(at); len(kids) > 0 {
			m.fileAt = kids[0]
		} else {
			m.message = node.Name + " is empty"
		}
		return m, nil, true
	}
	return m, nil, false
}

// rangerTidy closes the folders that were only opened to be previewed, once the cursor is
// not on them any more, so that looking about does not leave the tree open everywhere.
func (m *Model) rangerTidy() {
	rows := m.tree.Rows()
	if len(rows) == 0 || m.fileAt >= len(rows) {
		return
	}
	cur := rows[m.fileAt].Path
	for path := range m.autoOpen {
		if cur == path || strings.HasPrefix(cur, path+"/") {
			continue
		}
		if i := m.tree.Index(path); i >= 0 {
			m.tree.Collapse(i)
		}
		delete(m.autoOpen, path)
	}
	m.fileAt = max(m.tree.Index(cur), 0)
}

// rangerPreview reads the folder the cursor is on, if it was not read yet, so that its inside
// can be shown in the third column.
func (m *Model) rangerPreview() tea.Cmd {
	if !m.ranger || m.tree == nil || m.dirPreviewBusy {
		return nil
	}
	rows := m.tree.Rows()
	if m.fileAt >= len(rows) {
		return nil
	}
	node := rows[m.fileAt]
	if !node.Dir || node.Expanded || m.previewSkip[node.Path] {
		return nil
	}
	m.dirPreviewBusy = true
	return m.loadDirInto(node.Path, m.fileAt, true)
}

// loadDirInto lists a folder to open it. With preview it is only to be looked into (and is
// closed again when the cursor leaves); without, the cursor goes in once it is open.
func (m Model) loadDirInto(path string, row int, preview bool) tea.Cmd {
	root := m.root
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		entries, err := m.unit.ListDir(path)
		return dirMsg{root: root, path: path, row: row, entries: entries, err: err, preview: preview, into: !preview}
	}
}

// ---- drawing ----

func (m Model) rangerBody(g geom) string {
	st := m.st
	w := g.bodyW
	parentW := clamp(w*20/100, 14, 40)
	curW := clamp(w*34/100, 22, 70)
	prevW := w - parentW - curW - 2
	if prevW < 16 {
		return m.box(rootName(m.root), m.treeLines(g.bodyH-2), w, g.bodyH, true)
	}
	inner := g.bodyH - 2
	if m.tree == nil {
		return m.box(rootName(m.root), st.dim.Render(" listing…"), w, g.bodyH, true)
	}
	rows := m.tree.Rows()
	if len(rows) == 0 || m.fileAt >= len(rows) {
		return m.box(rootName(m.root), st.dim.Render(" (empty)"), w, g.bodyH, true)
	}
	at := m.fileAt
	node := rows[at]

	mark := func(n app.Node) string {
		switch {
		case m.fileMarks[n.Path]:
			return st.good.Render("✓")
		case m.clipMarker(n.Path) != "":
			return st.warn.Render(m.clipMarker(n.Path))
		case n.Path == m.listening:
			return st.hot.Render("♪")
		case m.favouriteMark(n.Path) != "":
			return m.favouriteMark(n.Path)
		case n.Dir && m.folderUsed(n.Path):
			return m.usedMark()
		}
		return " "
	}

	parent := m.tree.Parent(at)
	parentTitle, parentBody := rootName(m.root), st.dim.Render(" ")
	if parent >= 0 {
		if gp := m.tree.Parent(parent); gp >= 0 {
			parentTitle = rows[gp].Name
		}
		parentBody = m.rangerColumn(m.tree.Siblings(parent), parent, inner, parentW-2, mark)
	} else if len(rows) > 0 {
		parentBody = st.dim.Render(" " + display(m.tree.Root))
	}
	here := rootName(m.root)
	if parent >= 0 {
		here = rows[parent].Name
	}
	if n := len(m.fileMarks); n > 0 {
		here += fmt.Sprintf("  ·  %d marked", n)
	}
	cur := m.rangerColumn(m.tree.Siblings(at), at, inner, curW-2, mark)

	var preview []string
	title := node.Name
	switch {
	case node.Dir && node.Expanded:
		preview = append(preview, m.folderUseLines(node.Path, prevW-2)...)
		preview = append(preview, m.folderPadLines(node.Path, prevW-2)...)
		preview = append(preview, m.folderPadLines(node.Path, prevW-2)...)
		kids := m.tree.Children(at)
		if len(kids) == 0 {
			preview = append(preview, st.dim.Render(" (empty)"))
		} else {
			room := max(inner-len(preview), 1)
			preview = append(preview, strings.Split(m.rangerColumn(kids, -1, room, prevW-2, mark), "\n")...)
		}
	case node.Dir:
		preview = []string{st.dim.Render(" listing…")}
	default:
		preview = strings.Split(m.fileDetail(inner, prevW-2), "\n")
		title = "Details"
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.box(parentTitle, parentBody, parentW, g.bodyH, false), " ",
		m.box(here, cur, curW, g.bodyH, true), " ",
		m.box(title, m.withKeyGuide(strings.Join(preview, "\n"), rangerKeyGuide, inner, prevW-2), prevW, g.bodyH, false))
}

// rangerKeyGuide is the Files key guide with the keys of the column view in the View line.
var rangerKeyGuide = func() []keyGroup {
	g := slices.Clone(filesKeyGuide)
	g[len(g)-1] = keyGroup{"View", [][2]string{{"Space", "listen"}, {"^A", "autoplay"}, {"[ ]", "target bank"}, {"t", "source"},
		{"h", "up"}, {"l", "into"}, {"w", "tree view"}}}
	return g
}()

// rangerColumn draws some rows of the tree one under the other, with the cursor on row at.
func (m Model) rangerColumn(idx []int, at, height, width int, markOf func(app.Node) string) string {
	st := m.st
	nodes := m.tree.Rows()
	pos := slices.Index(idx, at)
	from, to := window(len(idx), max(pos, 0), height)
	lines := make([]string, 0, to-from)
	for k := from; k < to; k++ {
		n := nodes[idx[k]]
		name := n.Name
		style := st.dim
		switch {
		case n.Dir:
			name += "/"
			style = st.accent
		case app.IsJunk(n.Name):
			style = st.faint
		case isAudio(n.Name):
			style = st.text
		case n.Name == app.Sidecar:
			style = st.good
		}
		mark := markOf(n)
		room := max(width-3, 4)
		name = ansi.Truncate(name, room, "…")
		line := " " + mark + " " + style.Render(name)
		if idx[k] == at {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Width(width).Render(
				" " + ansi.Strip(mark) + " " + name)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return st.dim.Render(" (empty)")
	}
	return strings.Join(lines, "\n")
}
