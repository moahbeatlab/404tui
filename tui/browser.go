package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// browseKind is what the folder or file browser is for.
type browseKind int

const (
	// browseExport chooses the folder a bank goes into.
	browseExport browseKind = iota
	// browseRestore chooses the export to restore pads from.
	browseRestore
	// browseAssign chooses an audio file for a pad.
	browseAssign
	// browseProjectExport and browseProjectRestore choose where a project is saved to, or
	// the backup folder it is restored from.
	browseProjectExport
	browseProjectRestore
)

// browser is the dialog built on the same tree as the Files section, over the SD card, the
// unit's storage or this computer.
type browser struct {
	kind    browseKind
	root    string
	tree    *app.Tree
	at      int
	loading bool
	// pad is the pad an assign is for; marks are the files picked to fill free pads.
	pad   proto.PadIndex
	marks map[string]bool
	// confirm is set once the pad holds a sample and the user has been told it will be
	// replaced.
	confirm bool
	// project is the project (0-based) a project export or restore is for.
	project uint8
}

// openBrowser opens the chooser on root with the given folders open, and the cursor on
// selectPath when that is on screen.
func (m Model) openBrowser(kind browseKind, root string, expanded []string, selectPath string) (tea.Model, tea.Cmd) {
	m.browser = &browser{kind: kind, root: root, loading: true, marks: map[string]bool{}}
	return m, m.loadBrowser(root, expanded, selectPath)
}

// openAssign opens the file dialog for a pad, where it was left last time.
func (m Model) openAssign(pad proto.PadIndex) (tea.Model, tea.Cmd) {
	root := app.CardRoot
	if r, ok := rootFromKey(m.mem.AssignRoot); ok {
		root = r
	}
	model, cmd := m.openBrowser(browseAssign, root, m.mem.AssignExpanded, m.mem.AssignAt)
	model.(Model).browser.pad = pad
	return model, cmd
}

func (m Model) loadBrowser(root string, expanded []string, at string) tea.Cmd {
	unit := m.unit
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		tree, err := app.LoadTree(unit, root, expanded)
		return browseMsg{root: root, tree: tree, at: at, err: err}
	}
}

func (m Model) loadBrowserDir(root, path string, row int) tea.Cmd {
	unit := m.unit
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		entries, err := unit.ListDir(path)
		return browseDirMsg{root: root, path: path, row: row, entries: entries, err: err}
	}
}

// remember keeps where the assign dialog is, to open it there next time.
func (m *Model) remember(b *browser) {
	if b.kind != browseAssign || b.tree == nil {
		return
	}
	m.mem.AssignRoot = rootKey(b.root)
	m.mem.AssignExpanded = b.tree.Expanded()
	if rows := b.tree.Rows(); b.at < len(rows) {
		m.mem.AssignAt = rows[b.at].Path
	}
}

func (m Model) browserKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := m.browser
	name := key.String()
	m.remember(b)
	switch name {
	case "ctrl+a":
		if b.kind == browseAssign {
			m.autoplay = !m.autoplay
			if m.autoplay && m.player == nil {
				m.autoplay = false
				m.message = "no audio output to autoplay on"
			}
			return m, m.scheduleAutoplay()
		}
	case "esc", "q":
		m.browser = nil
		m.stopListening()
		return m, nil
	case "t":
		b.root = nextRoot(b.root)
		b.tree, b.at, b.loading, b.confirm = nil, 0, true, false
		return m, m.loadBrowser(b.root, nil, "")
	}
	if b.tree == nil || b.loading {
		return m, nil
	}
	rows := b.tree.Rows()
	here := b.tree.Folder(b.at)
	if name == "/" && b.kind == browseAssign {
		return m.openSearch(b.root, true, b.pad)
	}
	if name == "/" || (name == "p" && b.kind == browseAssign) {
		// Typing a path stays possible, starting from the folder in front.
		m.browser = nil
		m.stopListening()
		kind := promptRestore
		switch b.kind {
		case browseExport:
			kind = promptExport
		case browseAssign:
			kind = promptAssignFile
		case browseProjectExport:
			kind = promptProjectExport
		case browseProjectRestore:
			kind = promptProjectRestore
		}
		m.prompt = &prompt{kind: kind, buffer: here, pads: []proto.PadIndex{b.pad}, project: b.project}
		return m, nil
	}
	if len(rows) == 0 {
		if name == "enter" && b.kind != browseAssign {
			return m.chosen(b, here)
		}
		return m, nil
	}
	node := rows[b.at]
	moved := true
	switch name {
	case "j", "down":
		b.at = clamp(b.at+1, 0, len(rows)-1)
	case "k", "up":
		b.at = clamp(b.at-1, 0, len(rows)-1)
	case "g":
		b.at = 0
	case "G":
		b.at = len(rows) - 1
	case "l", "right", "tab":
		moved = false
		switch {
		case node.Dir && !node.Expanded:
			b.loading = true
			return m, m.loadBrowserDir(b.root, node.Path, b.at)
		case node.Dir && name == "tab":
			b.tree.Collapse(b.at)
		}
	case "h", "left", "backspace":
		if node.Expanded {
			b.tree.Collapse(b.at)
			moved = false
		} else if parent := b.tree.Parent(b.at); parent >= 0 {
			b.at = parent
		}
	case "v":
		if b.kind == browseAssign && isAudio(node.Name) {
			if b.marks[node.Path] {
				delete(b.marks, node.Path)
			} else {
				b.marks[node.Path] = true
			}
			b.at = clamp(b.at+1, 0, len(rows)-1)
		}
	case " ":
		if b.kind == browseAssign && isAudio(node.Name) {
			return m.listen(node.Path)
		}
		moved = false
	case "enter":
		if b.kind == browseAssign {
			return m.assignChosen(b, node)
		}
		return m.chosen(b, here)
	default:
		moved = false
	}
	if moved {
		b.confirm = false
		return m, tea.Batch(m.wanted(), m.scheduleAutoplay())
	}
	return m, m.wanted()
}

// assignChosen handles Enter in the assign dialog: a folder opens or closes, an audio file
// (or every marked one) goes onto the pad.
func (m Model) assignChosen(b *browser, node app.Node) (tea.Model, tea.Cmd) {
	switch {
	case node.Dir && !node.Expanded:
		b.loading = true
		return m, m.loadBrowserDir(b.root, node.Path, b.at)
	case node.Dir:
		b.tree.Collapse(b.at)
		return m, nil
	}
	files := []string{node.Path}
	if len(b.marks) > 0 {
		files = files[:0]
		for _, r := range b.tree.Rows() {
			if b.marks[r.Path] {
				files = append(files, r.Path)
			}
		}
	} else if !isAudio(node.Name) {
		m.message = "that is not an audio file"
		return m, nil
	}
	held := m.snap.Pads[b.pad.Index()]
	// A pad that holds a sample is replaced only on a second Enter.
	if held.HasSample && len(files) == 1 && !b.confirm {
		b.confirm = true
		return m, nil
	}
	pad := b.pad
	m.browser = nil
	m.stopListening()
	return m.assign(files, pad)
}

// chosen carries out the choice of a folder: a restore starts at once; an export asks what
// to call the folder it makes inside it.
func (m Model) chosen(b *browser, dir string) (tea.Model, tea.Cmd) {
	m.browser = nil
	switch b.kind {
	case browseRestore:
		return m.restoreFrom(dir)
	case browseProjectRestore:
		return m.planProjectRestore(b.project, dir)
	case browseProjectExport:
		name := fmt.Sprintf("PROJECT_%02d", b.project+1)
		if n := app.SanitizeName(m.snap.Projects[b.project]); m.snap.Projects[b.project] != "" {
			name = n
		}
		m.prompt = &prompt{kind: promptProjectExportName, path: dir, project: b.project, buffer: name}
		return m, nil
	}
	name := app.SanitizeName(m.snap.ProjectName)
	if name == "Sample" {
		name = fmt.Sprintf("PROJECT_%02d", m.snap.Project+1)
	}
	m.prompt = &prompt{kind: promptExportName, path: dir, buffer: fmt.Sprintf("%s-%c", name, 'A'+m.bank)}
	return m, nil
}

func (m Model) browserBox(g geom) string {
	st, b := m.st, m.browser
	if b.kind == browseAssign {
		return m.assignBox(g)
	}
	title, enter := "", ""
	switch b.kind {
	case browseRestore:
		title = "Restore pads from the export in…"
		enter = "Enter restores from the folder in front (or its samples.json)"
	case browseExport:
		title = fmt.Sprintf("Export bank %c into…", 'A'+m.bank)
		enter = "Enter exports into the folder in front"
	case browseProjectExport:
		title = fmt.Sprintf("Export project %d into…", b.project+1)
		enter = "Enter saves the project into the folder in front"
	case browseProjectRestore:
		title = fmt.Sprintf("Restore project %d from the backup folder…", b.project+1)
		enter = "Enter reads the backup in the folder in front (the one holding PADCONF.BIN)"
	}
	height := max(min(g.bodyH-10, 20), 5)
	tree := m.treeRender(b.tree, b.at, height, func(app.Node) string { return " " })
	if b.loading && b.tree == nil {
		tree = st.dim.Render(" listing…")
	}
	where := ""
	if b.tree != nil {
		where = display(b.tree.Folder(b.at))
	}
	lines := []string{
		st.bold.Render(title), "",
		m.sourceTabs(b.root), "",
		fit(tree, height), "",
		st.dim.Render("Folder  ") + st.accent.Render(where), "",
		st.dim.Render(enter),
		st.dim.Render("Tab/l open  ·  h close  ·  t source  ·  / type a path  ·  Esc cancel"),
	}
	return st.modal.Width(min(m.width-6, 84)).Render(strings.Join(lines, "\n"))
}

func (m Model) sourceTabs(root string) string {
	st := m.st
	var tabs strings.Builder
	for _, r := range roots() {
		label := " " + rootName(r) + " "
		if r == root {
			tabs.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(label))
		} else {
			tabs.WriteString(st.dim.Render(label))
		}
		tabs.WriteString(" ")
	}
	return tabs.String()
}

// assignBox is the file dialog of an assign: the tree on the left, the file in front on the
// right, and the pad it will go onto at the top.
func (m Model) assignBox(g geom) string {
	st, b := m.st, m.browser
	held := m.snap.Pads[b.pad.Index()]
	target := st.dim.Render(fmt.Sprintf("%s is empty", b.pad))
	if held.HasSample {
		target = st.warn.Render(fmt.Sprintf("%s holds %q — it will be replaced", b.pad, held.Name))
		if b.confirm {
			target = st.bad.Render("Press Enter again to replace it")
		}
	}
	if n := len(b.marks); n > 0 {
		target = st.good.Render(fmt.Sprintf("%d files marked — Enter fills free pads from %s on", n, b.pad))
	}
	width := min(m.width-6, 110)
	leftW := width * 11 / 20
	height := max(min(g.bodyH-10, 22), 6)
	mark := func(n app.Node) string {
		switch {
		case b.marks[n.Path]:
			return st.good.Render("✓")
		case n.Path == m.listening:
			return st.hot.Render("♪")
		}
		return " "
	}
	left := lipgloss.NewStyle().Width(leftW).Render(fit(m.treeRender(b.tree, b.at, height, mark), height))
	right := ""
	if b.tree != nil && b.at < len(b.tree.Rows()) {
		node := b.tree.Rows()[b.at]
		lines := []string{st.accent.Render(node.Name)}
		switch {
		case isAudio(node.Name):
			lines = append(lines, "")
			lines = append(lines, m.audioInfo(node, width-leftW-6)...)
		case node.Dir:
			lines = append(lines, st.dim.Render("Folder"))
		}
		right = strings.Join(lines, "\n")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", lipgloss.NewStyle().Width(width-leftW-4).Render(right))
	lines := []string{
		st.bold.Render(fmt.Sprintf("Assign a file to %s", b.pad)), target, "",
		m.sourceTabs(b.root), "",
		body, "",
		st.dim.Render("Enter assign  ·  Space listen  ·  Ctrl-A autoplay " + onOff(m.autoplay) + "  ·  v mark  ·  Tab/l open  ·  t source  ·  / search  ·  p type a path  ·  Esc"),
	}
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}
