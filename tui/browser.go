package tui

import (
	"fmt"
	"sort"
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
	// browseOffline chooses the folder of projects on this computer to work on offline.
	browseOffline
	// browseScan chooses the folder whose indexed files a scan looks in for where the samples
	// of the pads came from.
	browseScan
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
	// finding is set while a folder name is being typed (f); findText is what is typed and
	// findNote says what the last search did.
	// scanBanks is set when the scan also notes which folder is on which bank (W, not F).
	scanBanks bool
	finding   bool
	findText  string
	findAt    int
	findNote  string
}

// openBrowser opens the chooser on root with the given folders open, and the cursor on
// selectPath when that is on screen.
func (m Model) openBrowser(kind browseKind, root string, expanded []string, selectPath string) (tea.Model, tea.Cmd) {
	// On this computer, root at the filesystem but open down to the home folder and land there,
	// so the default is unchanged while /Volumes and the rest of the disk are a step up.
	if root == localRoot() && len(expanded) == 0 && selectPath == "" {
		expanded = app.Ancestors(root, localHome())
		expanded = append(expanded, localHome())
		selectPath = localHome()
	}
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
	var unit app.Storage = m.unit
	if m.unit == nil {
		// No unit yet (choosing a folder to work offline on): this computer only.
		unit = app.Local{}
	}
	return func() tea.Msg {
		m.ops.Lock()
		defer m.ops.Unlock()
		tree, err := app.LoadTree(unit, root, expanded)
		return browseMsg{root: root, tree: tree, at: at, err: err}
	}
}

func (m Model) loadBrowserDir(root, path string, row int) tea.Cmd {
	var unit app.Storage = m.unit
	if m.unit == nil {
		unit = app.Local{}
	}
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
	if b.finding {
		return m.browserFind(b, key)
	}
	switch name {
	case "ctrl+a":
		if b.kind == browseAssign {
			return m.toggleAutoplay()
		}
	case "esc", "q":
		m.browser = nil
		m.stopListening()
		return m, nil
	case "t":
		if m.unit == nil {
			return m, nil // only this computer without a unit
		}
		b.root = nextRoot(b.root)
		b.tree, b.at, b.loading, b.confirm = nil, 0, true, false
		return m, m.loadBrowser(b.root, nil, "")
	}
	if b.tree == nil || b.loading {
		return m, nil
	}
	rows := b.tree.Rows()
	here := b.tree.Folder(b.at)
	if name == "s" && b.kind == browseAssign {
		return m.openTagSearch(b.root, true, b.pad)
	}
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
	case "ctrl+d", "ctrl+u", "pgdown", "pgup":
		height := max(min(m.geom().bodyH-10, 20), 5)
		if b.kind == browseAssign {
			height = max(m.geom().bodyH-9, 8)
		}
		b.at = clamp(b.at+m.pageStep(name, height), 0, len(rows)-1)
	case "}", "{", "alt+n", "alt+p":
		b.at, m.message = jumpFolder(rows, b.at, name)
	case "shift+tab":
		b.at = b.tree.CollapseAll(b.at)
		moved = false
	case "f":
		b.finding, b.findText, b.findAt, b.findNote = true, "", 0, ""
		moved = false
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
		if b.kind == browseAssign {
			// Anything can be marked: Enter assigns the audio files among them, d deletes them.
			if b.marks[node.Path] {
				delete(b.marks, node.Path)
			} else {
				b.marks[node.Path] = true
			}
			b.at = clamp(b.at+1, 0, len(rows)-1)
		}
	case "u":
		if b.kind == browseAssign && len(b.marks) > 0 {
			b.marks = map[string]bool{}
			m.message = "unmarked everything"
		}
	case "n":
		if b.kind == browseAssign {
			pad := b.pad
			m.browser = nil
			m.stopListening()
			m.prompt = &prompt{kind: promptRenameFile, path: node.Path, buffer: node.Name, reopen: &pad}
			return m, nil
		}
	case "#":
		if b.kind == browseAssign {
			m.byNumber = !m.byNumber
			if m.byNumber {
				m.message = "assign by file number: 09_… goes on pad 9 of the bank"
			} else {
				m.message = "assign in order: each file takes the next free pad"
			}
		}
	case "*":
		if b.kind == browseAssign {
			// The dialog stays open, with its marks and cursor: nothing to reopen.
			return m.toggleFavourite(m.dialogFilesToTag(b), nil)
		}
	case "+":
		if b.kind == browseAssign {
			pad := b.pad
			files := m.dialogFilesToTag(b)
			m.browser = nil
			m.stopListening()
			return m.askTags(files, &pad)
		}
	case "i":
		if b.kind == browseAssign {
			return m.startTagIndex(b.tree.Folder(b.at))
		}
	case "V":
		if b.kind == browseAssign {
			folder := b.tree.Folder(b.at)
			n := 0
			for _, r := range rows {
				if r.Dir || !strings.HasPrefix(r.Path, folder+"/") || !isAudio(r.Name) || app.IsJunk(r.Name) {
					continue
				}
				b.marks[r.Path] = true
				n++
			}
			if n == 0 {
				m.message = "no audio files in " + display(folder) + " (open the folder first with Tab)"
			} else {
				m.message = fmt.Sprintf("marked %d files in %s; Enter assigns them to the free pads of %s", n, display(folder), "bank "+b.pad.String()[:1])
			}
		}
	case "J":
		if b.kind == browseAssign {
			n := 0
			for _, r := range rows {
				if app.IsJunk(r.Name) {
					b.marks[r.Path] = true
					n++
				}
			}
			if n == 0 {
				m.message = "no junk files in the open folders (open more folders with Tab)"
			} else {
				m.message = fmt.Sprintf("marked %d junk files; d deletes them", n)
			}
		}
	case "d":
		if b.kind == browseAssign {
			return m.dialogDelete(b, node)
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
		wanted := m.wanted()
		play := m.scheduleAutoplay()
		return m, tea.Batch(wanted, play)
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
			if b.marks[r.Path] && !r.Dir && isAudio(r.Name) {
				files = append(files, r.Path)
			}
		}
		if len(files) == 0 {
			m.message = "none of the marked items is an audio file"
			return m, nil
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
	if m.looping() {
		// The pattern plays on: the file tried in it stays until the pad is read again with the
		// sample written to it, and the loop takes that up.
		m.listenGen.Add(1)
	} else {
		m.stopListening()
	}
	return m.assign(files, pad)
}

// dialogDelete asks to delete the marked items of the assign dialog (or the one in front):
// the dialog closes for the question and opens again, where it was, afterwards.
func (m Model) dialogDelete(b *browser, node app.Node) (tea.Model, tea.Cmd) {
	targets := []string{node.Path}
	if len(b.marks) > 0 {
		targets = targets[:0]
		for p := range b.marks {
			targets = append(targets, p)
		}
		sort.Strings(targets)
	}
	for _, p := range targets {
		if app.VolumeOf(p) != app.VolumeCard {
			m.message = "only files on the SD card are deleted from here"
			return m, nil
		}
	}
	pad, unit := b.pad, m.unit
	m.browser = nil
	m.stopListening()
	m.busy = "counting"
	return m, m.edit(func() tea.Msg {
		count, err := app.CountPaths(unit, targets)
		return deletePlanMsg{paths: targets, count: count, err: err, reopen: &pad}
	})
}

// chosen carries out the choice of a folder: a restore starts at once; an export asks what
// to call the folder it makes inside it.
func (m Model) chosen(b *browser, dir string) (tea.Model, tea.Cmd) {
	m.browser = nil
	switch b.kind {
	case browseScan:
		m.mem.ScanDir = dir
		return m.scanOriginsFor(b.scanBanks, dir)
	case browseRestore:
		return m.restoreFrom(dir)
	case browseProjectRestore:
		return m.planProjectRestore(b.project, dir)
	case browseOffline:
		m.message = "opening " + display(dir) + " to work offline"
		return m, m.goOffline(dir)
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
	if b.finding && b.tree != nil {
		return m.folderFinderBox(g, b.tree.Rows(), m.indexes[b.root], b.findText, b.findAt, "Find a folder in "+rootName(b.root))
	}
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
	case browseScan:
		title = "Look for where the samples came from in…"
		enter = "Enter looks in the files of the folder in front, as far as they are indexed (i in the Files view indexes a folder)"
	case browseOffline:
		title = "Work offline on the projects in…"
		enter = "Enter opens the folder in front: one holding exported projects (each a folder with PADCONF.BIN), or one project.\nAn empty folder works too: new projects are made in it."
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
	target := st.good.Render(fmt.Sprintf("%s is empty", b.pad))
	if held.HasSample {
		target = st.warn.Render(fmt.Sprintf("%s holds %q — it will be replaced", b.pad, held.Name))
		if b.confirm {
			target = st.bad.Render("Press Enter again to replace it")
		}
	}
	if n := len(b.marks); n > 0 {
		target = st.good.Render(fmt.Sprintf("%d files marked — Enter %s", n, m.assignHow(b.pad)))
	}
	// The dialog is as large as the page lets it be, always: a size that followed what is in
	// front made it jump as the cursor moved.
	width := clamp(m.width*94/100, 70, 170)
	leftW := width * 11 / 20
	height := max(g.bodyH-9, 8)
	mark := func(n app.Node) string {
		switch {
		case b.marks[n.Path]:
			return st.good.Render("✓")
		case n.Path == m.listening:
			return st.hot.Render("♪")
		case m.favouriteMark(n.Path) != "":
			return m.favouriteMark(n.Path)
		case n.Dir && m.folderUsed(n.Path):
			return m.usedMark()
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
			lines = append(lines, st.dim.Render("Folder"), "")
			// Whether its samples were used already, and where.
			lines = append(lines, m.folderUsage(node.Path, width-leftW-6)...)
		}
		right = strings.Join(lines, "\n")
	}
	rightW := width - leftW - 4
	// The keys keep the foot of the column; the file in front has the room above them.
	keys := m.assignKeys(b, rightW)
	room := max(height-strings.Count(keys, "\n")-2, 1)
	right = fit(right, room) + "\n\n" + keys
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", lipgloss.NewStyle().Width(rightW).Render(fit(right, height)))
	// The title is a bar in the accent colour with the pad in a lighter block, at the top of the
	// frame above the sources: it says where the files go, and it should not be missed.
	th := st.th
	bar := lipgloss.NewStyle().Background(th.accent).Foreground(th.cursorFg).Bold(true)
	padBlock := lipgloss.NewStyle().Background(th.text).Foreground(th.cursorFg).Bold(true)
	title := bar.Render(" Assign a file to ") + padBlock.Render(b.pad.String()+" ")
	// What the dialog is for and what the pad holds are in its top right corner.
	corner := func(s string) string { return lipgloss.PlaceHorizontal(width-2, lipgloss.Right, s) }
	lines := []string{corner(title), corner(target), "", m.sourceTabs(b.root), "", body}
	return st.modal.Width(width).Render(strings.Join(lines, "\n"))
}

// assignKeys lists the dialog's keys in two columns under the file details, where there is
// room; with files marked it leads with how many and offers what can be done to them.
func (m Model) assignKeys(b *browser, width int) string {
	st := m.st
	keys := [][2]string{
		{"Enter", "assign"}, {"Space", "listen"},
		{"Ctrl-A", "autoplay " + onOff(m.autoplay)}, {"v", "mark"},
		{"V", "mark folder"}, {"J", "mark junk"},
		{"*", "favourite"}, {"+", "add tags"},
		{"i", "index folder"}, {"#", "by number " + onOff(m.byNumber)}, {"d", "delete"}, {"n", "rename"},
		{"Tab / l", "open folder"},
		{"S-Tab", "fold all"}, {"t", "source"},
		{"/", "search"}, {"s", "search tags"}, {"f", "find folder"},
		{"p", "type a path"}, {"Esc", "close"},
	}
	var out []string
	if b.findNote != "" {
		out = append(out, st.warn.Render(b.findNote))
	}
	if n := len(b.marks); n > 0 {
		out = append(out, st.good.Render(plural(n, "item")+" marked"))
		keys = [][2]string{
			{"d", "delete"}, {"u", "unmark all"},
			{"Enter", "assign audio"}, {"v", "mark"},
			{"V", "mark folder"}, {"J", "mark junk"},
			{"*", "favourite"}, {"+", "add tags"},
			{"#", "by number " + onOff(m.byNumber)}, {"f", "find folder"}, {"Esc", "close"},
		}
	}
	colW := max(width/2, 20)
	for i := 0; i < len(keys); i += 2 {
		row := ""
		for _, k := range keys[i:min(i+2, len(keys))] {
			cell := st.accent.Render(fmt.Sprintf("%-8s", k[0])) + st.dim.Render(k[1])
			row += lipgloss.NewStyle().Width(colW).Render(cell)
		}
		out = append(out, row)
	}
	return strings.Join(out, "\n")
}

// browserFind takes the keys while a folder name is typed: the matching open folders are
// listed as it grows, up and down choose one, Enter moves the cursor there, Esc gives up.
func (m Model) browserFind(b *browser, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	name := key.String()
	switch name {
	case "esc":
		b.finding = false
	case "up", "down", "ctrl+n", "ctrl+p", "tab", "shift+tab":
		b.findAt = stepChoice(b.findAt, len(folderChoices(b.tree.Rows(), m.indexes[b.root], m.tags, b.findText)), name)
	case "backspace":
		if r := []rune(b.findText); len(r) > 0 {
			b.findText, b.findAt = string(r[:len(r)-1]), 0
		}
	case "enter":
		b.finding = false
		matches := folderChoices(b.tree.Rows(), m.indexes[b.root], m.tags, b.findText)
		if len(matches) == 0 {
			b.findNote = fmt.Sprintf("no folder named like %q", b.findText)
			break
		}
		c := matches[clamp(b.findAt, 0, len(matches)-1)]
		for i, r := range b.tree.Rows() {
			if r.Path == c.Path {
				b.at = i
				b.findNote = c.Name + ": h or Tab collapses it"
				return m, nil
			}
		}
		// Not open yet: the folders above it are listed and the cursor lands on it.
		b.tree, b.loading, b.confirm = nil, true, false
		return m, m.loadBrowser(b.root, append(app.Ancestors(b.root, c.Path), c.Path), c.Path)
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			b.findText += string(key.Runes)
			b.findAt = 0
		}
	}
	return m, nil
}

// assignHow says where Enter puts the marked files.
func (m Model) assignHow(pad proto.PadIndex) string {
	if m.byNumber {
		return fmt.Sprintf("puts each on the pad its number names (09_ on pad 9 of bank %s)", pad.String()[:1])
	}
	return fmt.Sprintf("fills free pads from %s on  ·  # puts each on the pad its number names", pad)
}
