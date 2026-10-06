package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// command is one entry of the palette: a label for the key that does it.
type command struct {
	key   string
	label string
	// sections it applies to; none means all of them.
	sections []int
}

var commands = []command{
	{"enter", "Preview pad on the unit", []int{secPads}},
	{"a", "Assign a file to this pad (opens the file dialog)", []int{secPads}},
	{"n", "Rename pad (prefix/suffix when pads are marked)", []int{secPads}},
	{"v", "Mark / unmark pad", []int{secPads}},
	{"V", "Mark every pad with a sample in the bank", []int{secPads}},
	{"d", "Delete sample (of every marked pad)", []int{secPads}},
	{"S", "Edit this pad's settings (or the marked pads')", []int{secPads}},
	{",", "Pad level down (of the marked pads)", []int{secPads}},
	{".", "Pad level up (of the marked pads)", []int{secPads}},
	{"c", "Copy this pad (or the marked ones) to another pad, in any bank", []int{secPads}},
	{"b", "Edit this bank's tempo and volume", []int{secPads, secPatterns}},
	{"<", "Bank tempo down 1 BPM", []int{secPads, secPatterns}},
	{">", "Bank tempo up 1 BPM", []int{secPads, secPatterns}},
	{"-", "Bank volume down", []int{secPads, secPatterns}},
	{"+", "Bank volume up", []int{secPads, secPatterns}},
	{"x", "Pick up pad to move it", []int{secPads}},
	{"p", "Move the picked-up pad here (overwrite)", []int{secPads}},
	{"s", "Swap the picked-up pad with this one", []int{secPads}},
	{"e", "Export this bank: choose a folder, get WAV files + samples.json", []int{secPads}},
	{"R", "Restore pads from an export: choose its folder", []int{secPads}},
	{"enter", "Open pattern", []int{secPatterns}},
	{"b", "Retarget the open pattern to another bank", []int{secPatterns}},
	{"enter", "Assign this file to a pad", []int{secFiles}},
	{"[", "Target bank for assigns: previous (Files)", []int{secFiles}},
	{"]", "Target bank for assigns: next (Files)", []int{secFiles}},
	{"space", "Listen to this file", []int{secFiles}},
	{"v", "Mark file for a multi-file assign", []int{secFiles}},
	{"tab", "Open / close folder", []int{secFiles}},
	{"n", "Rename file or folder", []int{secFiles}},
	{"m", "Mark file or folder to move", []int{secFiles}},
	{"p", "Move marked entry here", []int{secFiles}},
	{"R", "Restore pads from this folder", []int{secFiles}},
	{"t", "Next source: SD card, unit storage, this computer", []int{secFiles}},
	{"enter", "Select project", []int{secProjects}},
	{"n", "Rename project", []int{secProjects}},
	{"e", "Export this project to the SD card or this computer", []int{secProjects}},
	{"R", "Restore this project from a backup folder", []int{secProjects}},
	{"[", "Previous bank", []int{secPads, secPatterns}},
	{"]", "Next bank", []int{secPads, secPatterns}},
	{"/", "Find a pad or project by name", []int{secPads, secProjects}},
	{"/", "Search every audio file in this source by keyword", []int{secFiles}},
	{"1", "Go to Pads", nil},
	{"2", "Go to Patterns", nil},
	{"3", "Go to Files", nil},
	{"4", "Go to Projects", nil},
	{"T", "Next colour theme", nil},
	{"r", "Refresh from the unit", nil},
	{"?", "Keybindings", nil},
	{"q", "Quit", nil},
}

// matches are the commands of the section in front that contain the palette's filter.
func (m *Model) matches() []command {
	var out []command
	for _, c := range commands {
		here := len(c.sections) == 0
		for _, s := range c.sections {
			here = here || s == m.section
		}
		if here && strings.Contains(strings.ToLower(c.label), strings.ToLower(m.palette.filter)) {
			out = append(out, c)
		}
	}
	return out
}

// keyFor is the key press a palette command stands for.
func keyFor(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// key handles a key press, then keeps the state that is remembered between runs.
func (m Model) key(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	model, cmd := m.handleKey(key)
	if updated, ok := model.(Model); ok {
		updated.persist()
		return updated, cmd
	}
	return model, cmd
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	name := key.String()
	if name == "ctrl+c" {
		m.stopListening()
		return m, tea.Quit
	}
	// The help overlay swallows everything except what closes it.
	if m.help {
		if name == "q" || name == "esc" || name == "?" {
			m.help = false
		}
		return m, nil
	}
	switch {
	case m.search != nil:
		return m.searchKey(key)
	case m.editor != nil:
		return m.editorKey(key)
	case m.browser != nil:
		return m.browserKey(key)
	case m.prompt != nil:
		return m.promptKey(key)
	case m.picker != nil:
		return m.pickerKey(key)
	case m.palette != nil:
		return m.paletteKey(key)
	}
	if m.unit == nil {
		// Nothing to work on until a unit is connected.
		switch name {
		case "q", "esc":
			return m, tea.Quit
		case "?":
			m.help = true
		case "T":
			m.themeAt = (m.themeAt + 1) % len(themes)
			m.st = newStyles(themes[m.themeAt])
		}
		return m, nil
	}
	if name == "?" {
		m.help = true
		return m, nil
	}
	if name == "q" {
		m.stopListening()
		return m, tea.Quit
	}
	if m.busy != "" || m.snap == nil {
		if name == "esc" {
			m.stopListening()
			return m, tea.Quit
		}
		return m, nil
	}
	m.message = ""

	switch name {
	case ":":
		m.palette = &palette{}
		return m, nil
	case "esc":
		return m.escape()
	case "1", "2", "3", "4":
		return m.goTo(int(name[0] - '1'))
	case "shift+tab":
		return m.goTo((m.section + secCount - 1) % secCount)
	case "tab":
		if m.section != secFiles {
			return m.goTo((m.section + 1) % secCount)
		}
	case "T":
		m.themeAt = (m.themeAt + 1) % len(themes)
		m.st = newStyles(themes[m.themeAt])
		m.message = "theme: " + themes[m.themeAt].name
		return m, nil
	case "r":
		m.patterns = map[int][]app.PatternRow{}
		m.usage, m.usageState = nil, 0
		clear(m.indexes)
		m.tree = nil
		m.infos = map[string]fileInfo{}
		m.pattern = nil
		m.busy = "reading the unit"
		return m, m.load("refreshed")
	case "/":
		switch m.section {
		case secFiles:
			return m.openSearch(m.root, false, proto.PadIndex{})
		case secPads, secProjects:
			m.prompt = &prompt{kind: promptSearch}
		}
		return m, nil
	}
	switch m.section {
	case secPads:
		return m.padsKey(name)
	case secPatterns:
		return m.patternsKey(name)
	case secFiles:
		return m.filesKey(name)
	}
	return m.projectsKey(name)
}

// escape backs out of the innermost thing: a pattern, a pick-up, marks; else it quits.
func (m Model) escape() (tea.Model, tea.Cmd) {
	switch {
	case m.pattern != nil && m.section == secPatterns:
		m.pattern = nil
	case m.hasMoving:
		m.hasMoving = false
	case len(m.marked) > 0:
		m.marked = map[int]bool{}
	case len(m.fileMarks) > 0:
		m.fileMarks = map[string]bool{}
	case m.fileMoved != "":
		m.fileMoved = ""
	case m.listening != "":
		m.stopListening()
	default:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) goTo(section int) (tea.Model, tea.Cmd) {
	m.section = section
	cmd := m.wanted()
	return m, cmd
}

func (m *Model) stopListening() {
	if m.player != nil {
		m.player.Stop()
	}
	m.listening = ""
}

func clamp(v, lo, hi int) int {
	return max(min(v, hi), lo)
}

// ---- Pads ----

// gridMove moves the cursor on the 4×4 pad grid, laid out like the unit and the desktop
// app: pads 1-4 on the top row, 13-16 on the bottom one.
func (m *Model) gridMove(dx, dy int) {
	col, row := m.pad%4, m.pad/4
	col = clamp(col+dx, 0, 3)
	row = clamp(row+dy, 0, 3)
	m.pad = row*4 + col
}

func (m *Model) switchBank(by int) tea.Cmd {
	m.bank = (m.bank + proto.Banks + by) % proto.Banks
	// Marks are positions in a bank; they mean nothing in another one.
	m.marked = map[int]bool{}
	// The unit selects the first pad of a bank it is moved to; expect that, so the cursor
	// does not jump when it reports it.
	m.selected, m.hasSelected = m.padAt(0), true
	bank, unit := m.bank, m.unit
	show := m.edit(func() tea.Msg {
		if err := app.ShowBank(unit, bank); err != nil {
			return noteMsg("the unit did not follow the bank: " + err.Error())
		}
		return nil
	})
	return tea.Batch(show, m.wanted())
}

// targets are the pads an edit applies to: the marked ones, or else the one under the
// cursor.
func (m *Model) targets() []app.PadRow {
	bank := m.snap.Bank(m.bank)
	if len(m.marked) == 0 {
		return bank[m.pad : m.pad+1]
	}
	var rows []app.PadRow
	for i, row := range bank {
		if m.marked[i] {
			rows = append(rows, row)
		}
	}
	return rows
}

func withSamples(rows []app.PadRow) []proto.PadIndex {
	var pads []proto.PadIndex
	for _, row := range rows {
		if row.HasSample {
			pads = append(pads, row.Pad)
		}
	}
	return pads
}

func (m Model) padsKey(name string) (tea.Model, tea.Cmd) {
	row := m.padRow()
	switch name {
	case "h", "left":
		m.gridMove(-1, 0)
	case "l", "right":
		m.gridMove(1, 0)
	case "j", "down":
		m.gridMove(0, 1)
	case "k", "up":
		m.gridMove(0, -1)
	case "g":
		m.pad = 0
	case "G":
		m.pad = 15
	case "[":
		cmd := m.switchBank(-1)
		return m, cmd
	case "]":
		cmd := m.switchBank(1)
		return m, cmd
	case "v":
		if m.marked[m.pad] {
			delete(m.marked, m.pad)
		} else {
			m.marked[m.pad] = true
		}
	case "V":
		for i, r := range m.snap.Bank(m.bank) {
			if r.HasSample {
				m.marked[i] = true
			}
		}
	case "enter", " ":
		if !row.HasSample {
			m.message = fmt.Sprintf("%s is empty; a assigns a file to it", row.Pad)
			break
		}
		m.message = fmt.Sprintf("previewing %s", row.Pad)
		unit := m.unit
		// Not a wait: a preview holds the note for its whole length.
		return m, func() tea.Msg {
			if err := unit.Preview(row.Pad, app.PreviewLength); err != nil {
				return noteMsg(fmt.Sprintf("preview %s failed: %v", row.Pad, err))
			}
			return nil
		}
	case "n":
		pads := withSamples(m.targets())
		switch {
		case len(m.marked) >= 2 && len(pads) > 0:
			m.prompt = &prompt{kind: promptBulkRename, pads: pads, rows: m.targets()}
		case len(m.marked) < 2 && row.HasSample:
			m.prompt = &prompt{kind: promptRenamePad, pads: []proto.PadIndex{row.Pad}, buffer: row.Name}
		default:
			m.message = "nothing to rename: no sample there"
		}
	case "d":
		if pads := withSamples(m.targets()); len(pads) > 0 {
			m.prompt = &prompt{kind: promptDelete, pads: pads}
		} else {
			m.message = "nothing to delete: no sample there"
		}
	case "a":
		return m.openAssign(row.Pad)
	case "e":
		return m.openBrowser(browseExport, app.CardRoot, []string{"SD:exports"}, "")
	case "R":
		return m.openBrowser(browseRestore, app.CardRoot, []string{"SD:exports"}, "")
	case "c":
		pads := withSamples(m.targets())
		if len(pads) == 0 {
			m.message = "nothing to copy: no sample there"
			break
		}
		// It opens on the next free pad after this one, in the same bank.
		m.picker = &picker{copyFrom: pads, bank: m.bank, pad: m.freeAfter(m.bank, m.pad)}
	case "<", ">", "-", "+", "=", "b":
		return m.bankKey(name)
	case "S":
		return m.openEditor()
	case ",":
		return m.nudge("level", -1)
	case ".":
		return m.nudge("level", 1)
	case "x":
		m.moving, m.hasMoving = row.Pad, true
		if !row.HasSample {
			m.hasMoving = false
			m.message = "nothing to pick up: no sample there"
		} else {
			m.message = fmt.Sprintf("%s picked up: p moves it to a pad, s swaps", row.Pad)
		}
	case "p", "s":
		if !m.hasMoving {
			m.message = "nothing picked up: x picks up a pad's sample"
			break
		}
		from, to := m.moving, row.Pad
		if from == to {
			m.message = "that is the pad itself"
			break
		}
		mode := proto.MoveOverwrite
		if name == "s" {
			mode = proto.MoveExchange
		}
		m.hasMoving = false
		m.busy = "moving"
		unit := m.unit
		return m, m.edit(func() tea.Msg {
			if err := unit.MoveSample(from, to, mode); err != nil {
				return doneMsg{message: err.Error()}
			}
			verb := "moved to"
			if mode == proto.MoveExchange {
				verb = "swapped with"
			}
			return doneMsg{message: fmt.Sprintf("%s %s %s", from, verb, to), reload: true, focus: &to}
		})
	}
	cmd := m.wanted()
	return m, cmd
}

// bankKey changes the tempo and volume of the bank in view: < > a BPM at a time, - + in
// steps of 5, and b opens a dialog to type them. The change shows at once and is sent to the
// unit behind it.
func (m Model) bankKey(name string) (tea.Model, tea.Cmd) {
	bank, unit := m.bank, m.unit
	info := &m.snap.Banks[bank]
	if name == "b" {
		m.prompt = &prompt{kind: promptBank, fields: [2]string{fmt.Sprintf("%.2f", info.Tempo), fmt.Sprint(info.Volume)}}
		return m, nil
	}
	if name == "<" || name == ">" {
		delta := 1.0
		if name == "<" {
			delta = -1
		}
		tempo := info.Tempo + delta
		if tempo < app.MinBankBPM || tempo > app.MaxBankBPM {
			m.message = fmt.Sprintf("bank tempo stays between %.0f and %.0f BPM", app.MinBankBPM, app.MaxBankBPM)
			return m, nil
		}
		info.Tempo = tempo
		return m, m.edit(func() tea.Msg {
			if err := app.SetBankTempo(unit, bank, tempo); err != nil {
				return doneMsg{message: "bank tempo: " + err.Error(), reload: true}
			}
			return noteMsg(fmt.Sprintf("bank %c tempo %.2f BPM", 'A'+bank, tempo))
		})
	}
	delta := 5
	if name == "-" {
		delta = -5
	}
	volume := clamp(info.Volume+delta, 0, app.MaxBankVolume)
	if volume == info.Volume {
		m.message = fmt.Sprintf("bank %c volume is at its limit", 'A'+bank)
		return m, nil
	}
	info.Volume = volume
	return m, m.edit(func() tea.Msg {
		if err := app.SetBankVolume(unit, bank, volume); err != nil {
			return doneMsg{message: "bank volume: " + err.Error(), reload: true}
		}
		return noteMsg(fmt.Sprintf("bank %c volume %d", 'A'+bank, volume))
	})
}

// bankPromptKey types into the bank dialog.
func (m Model) bankPromptKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.prompt
	switch name := key.String(); {
	case name == "esc":
		m.prompt = nil
	case name == "tab" || name == "shift+tab" || name == "down" || name == "up":
		p.field = 1 - p.field
	case name == "enter":
		return m.applyBank()
	case name == "backspace":
		if f := p.fields[p.field]; f != "" {
			p.fields[p.field] = f[:len(f)-1]
		}
	case name == "ctrl+u":
		p.fields[p.field] = ""
	case key.Type == tea.KeyRunes:
		for _, r := range key.Runes {
			if (r >= '0' && r <= '9') || (r == '.' && p.field == 0) {
				p.fields[p.field] += string(r)
			}
		}
	}
	return m, nil
}

// applyBank sends the values typed in the bank dialog, the ones that changed.
func (m Model) applyBank() (tea.Model, tea.Cmd) {
	p, bank, unit := m.prompt, m.bank, m.unit
	m.prompt = nil
	tempo, terr := strconv.ParseFloat(strings.TrimSpace(p.fields[0]), 64)
	volume, verr := strconv.Atoi(strings.TrimSpace(p.fields[1]))
	if terr != nil || verr != nil {
		m.message = "bank settings: tempo and volume have to be numbers"
		return m, nil
	}
	cur := m.snap.Banks[bank]
	m.busy = "setting the bank"
	return m, m.edit(func() tea.Msg {
		if tempo != cur.Tempo {
			if err := app.SetBankTempo(unit, bank, tempo); err != nil {
				return doneMsg{message: err.Error(), reload: true}
			}
		}
		if volume != cur.Volume {
			if err := app.SetBankVolume(unit, bank, volume); err != nil {
				return doneMsg{message: err.Error(), reload: true}
			}
		}
		return doneMsg{message: fmt.Sprintf("bank %c: tempo %.2f BPM, volume %d", 'A'+bank, tempo, volume), reload: true}
	})
}

// ---- Patterns ----

func (m Model) patternsKey(name string) (tea.Model, tea.Cmd) {
	if m.pattern != nil {
		switch name {
		case "b":
			d := m.pattern
			if !d.HasSourceBank {
				m.message = "its pads span more than one bank; retargeting needs one bank"
				break
			}
			// Default to the next bank over, so a single Enter does something useful.
			m.picker = &picker{retarget: true, slot: d.Slot, sourceBank: d.SourceBank, bank: (d.SourceBank + 1) % proto.Banks}
		case "r":
			m.busy = "reading the pattern"
			return m, m.openPattern(m.pattern.Slot, "refreshed")
		}
		return m, nil
	}
	switch name {
	case "<", ">", "-", "+", "=", "b":
		return m.bankKey(name)
	case "h", "left":
		m.patternCursor = clamp(m.patternCursor%4-1, 0, 3) + m.patternCursor/4*4
	case "l", "right":
		m.patternCursor = clamp(m.patternCursor%4+1, 0, 3) + m.patternCursor/4*4
	case "j", "down":
		m.patternCursor = clamp(m.patternCursor/4+1, 0, 3)*4 + m.patternCursor%4
	case "k", "up":
		m.patternCursor = clamp(m.patternCursor/4-1, 0, 3)*4 + m.patternCursor%4
	case "[":
		m.bank = (m.bank + proto.Banks - 1) % proto.Banks
		cmd := m.wanted()
		return m, cmd
	case "]":
		m.bank = (m.bank + 1) % proto.Banks
		cmd := m.wanted()
		return m, cmd
	case "enter", " ":
		rows := m.patterns[m.bank]
		if rows == nil || !rows[m.patternCursor].Exists {
			m.message = "that pattern slot is empty"
			break
		}
		m.busy = "reading the pattern"
		return m, m.openPattern(rows[m.patternCursor].Slot, "")
	}
	return m, nil
}

// ---- Projects ----

func (m Model) projectsKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "j", "down":
		m.projectAt = clamp(m.projectAt+1, 0, len(m.snap.Projects)-1)
	case "k", "up":
		m.projectAt = clamp(m.projectAt-1, 0, len(m.snap.Projects)-1)
	case "g":
		m.projectAt = 0
	case "G":
		m.projectAt = len(m.snap.Projects) - 1
	case "enter", " ":
		project := uint8(m.projectAt)
		m.busy = fmt.Sprintf("loading project %d", project+1)
		unit := m.unit
		return m, m.edit(func() tea.Msg {
			if err := unit.SelectProject(project); err != nil {
				return doneMsg{message: fmt.Sprintf("project %d: %v", project+1, err)}
			}
			return doneMsg{message: fmt.Sprintf("project %d selected", project+1), reload: true}
		})
	case "n":
		m.prompt = &prompt{kind: promptRenameProject, project: uint8(m.projectAt), buffer: m.snap.Projects[m.projectAt]}
	case "e":
		return m.openProjectBrowser(browseProjectExport, uint8(m.projectAt))
	case "R", "i":
		return m.openProjectBrowser(browseProjectRestore, uint8(m.projectAt))
	}
	return m, nil
}

// openProjectBrowser opens the chooser for exporting a project or restoring one.
func (m Model) openProjectBrowser(kind browseKind, project uint8) (tea.Model, tea.Cmd) {
	model, cmd := m.openBrowser(kind, app.CardRoot, nil, "")
	model.(Model).browser.project = project
	return model, cmd
}

// planProjectRestore reads the backup in dir and checks it, then asks before it overwrites.
func (m Model) planProjectRestore(project uint8, dir string) (tea.Model, tea.Cmd) {
	unit := m.unit
	m.busy = "reading the backup"
	return m, m.edit(func() tea.Msg {
		plan, err := app.PlanProjectRestore(unit, project, dir)
		return planMsg{plan: plan, err: err}
	})
}

// ---- Files ----

func (m Model) filesKey(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "[":
		m.target = (m.target + proto.Banks - 1) % proto.Banks
		return m, nil
	case "]":
		m.target = (m.target + 1) % proto.Banks
		return m, nil
	case "t":
		m.root = nextRoot(m.root)
		m.tree, m.fileMoved, m.fileAt = nil, "", 0
		m.fileMarks = map[string]bool{}
		cmd := m.wanted()
		return m, cmd
	}
	if m.tree == nil {
		return m, nil
	}
	rows := m.tree.Rows()
	at := m.fileAt
	if name == "p" {
		return m.paste(m.tree.Folder(at))
	}
	if len(rows) == 0 {
		return m, nil
	}
	node := rows[at]
	switch name {
	case "j", "down":
		m.fileAt = clamp(at+1, 0, len(rows)-1)
	case "k", "up":
		m.fileAt = clamp(at-1, 0, len(rows)-1)
	case "g":
		m.fileAt = 0
	case "G":
		m.fileAt = len(rows) - 1
	case "l", "right":
		if node.Dir && !node.Expanded {
			m.busy = "listing files"
			return m, m.loadDir(node.Path, at)
		}
	case "tab":
		if node.Expanded {
			m.tree.Collapse(at)
		} else if node.Dir {
			m.busy = "listing files"
			return m, m.loadDir(node.Path, at)
		}
	case "enter":
		switch {
		case node.Dir && !node.Expanded:
			m.busy = "listing files"
			return m, m.loadDir(node.Path, at)
		case node.Dir:
			m.tree.Collapse(at)
		case isAudio(node.Name):
			return m.startAssign(node)
		case node.Name == app.Sidecar:
			return m.restoreFrom(app.Dir(node.Path))
		}
	case "h", "left", "backspace":
		if node.Expanded {
			m.tree.Collapse(at)
		} else if parent := m.tree.Parent(at); parent >= 0 {
			m.fileAt = parent
		}
	case "a":
		if isAudio(node.Name) {
			return m.startAssign(node)
		}
		m.message = "that is not an audio file"
	case "v":
		if !isAudio(node.Name) {
			m.message = "only audio files can be marked for assigning"
			break
		}
		if m.fileMarks[node.Path] {
			delete(m.fileMarks, node.Path)
		} else {
			m.fileMarks[node.Path] = true
		}
		m.fileAt = clamp(at+1, 0, len(rows)-1)
	case "n":
		m.prompt = &prompt{kind: promptRenameFile, path: node.Path, buffer: node.Name}
	case "m":
		m.fileMoved = node.Path
		m.message = fmt.Sprintf("marked %s; p on a folder moves it there", display(node.Path))
	case "R":
		return m.openBrowser(browseRestore, m.root, m.tree.Expanded(), node.Path)
	case " ":
		if !isAudio(node.Name) {
			break
		}
		return m.listen(node.Path)
	}
	cmd := m.wanted()
	return m, cmd
}

func nextRoot(root string) string {
	switch root {
	case app.CardRoot:
		return app.StorageRoot
	case app.StorageRoot:
		return localRoot()
	}
	return app.CardRoot
}

// startAssign opens the pad picker for a file, or for every marked file. It opens on the
// target bank, at the next free pad, so a plain Enter puts the file where you would want it.
func (m Model) startAssign(node app.Node) (tea.Model, tea.Cmd) {
	files := []string{node.Path}
	if len(m.fileMarks) > 0 {
		files = files[:0]
		for _, r := range m.tree.Rows() {
			if m.fileMarks[r.Path] {
				files = append(files, r.Path)
			}
		}
	}
	m.stopListening()
	m.picker = &picker{files: files, bank: m.target, pad: m.nextFree(m.target)}
	return m, nil
}

// assign carries out an assign: one file onto a pad, or several onto the free pads from it on.
func (m Model) assign(files []string, start proto.PadIndex) (tea.Model, tea.Cmd) {
	unit, snap := m.unit, m.snap
	m.fileMarks = map[string]bool{}
	m.busy = fmt.Sprintf("assigning %d file(s)", len(files))
	if len(files) == 1 {
		return m, m.edit(func() tea.Msg {
			name, err := app.Assign(unit, unit, start, files[0])
			if err != nil {
				return doneMsg{message: err.Error()}
			}
			return doneMsg{message: fmt.Sprintf("%s: assigned to %s as %q", app.Base(files[0]), start, name), reload: true, focus: &start, assigned: true}
		})
	}
	taken := func(p proto.PadIndex) bool { return snap.Pads[p.Index()].HasSample }
	pads := app.FreePads(start, len(files), taken)
	return m, m.edit(func() tea.Msg {
		placed := app.AssignMany(unit, unit, pads, files, nil)
		ok, failed := 0, []string{}
		var first *proto.PadIndex
		for _, p := range placed {
			if p.Err != nil {
				failed = append(failed, p.Err.Error())
				continue
			}
			if first == nil {
				pad := p.Pad
				first = &pad
			}
			ok++
		}
		message := fmt.Sprintf("%d files assigned from %s", ok, start)
		if len(failed) > 0 {
			message += "; failed: " + strings.Join(failed, "; ")
		}
		return doneMsg{message: message, reload: true, focus: first, assigned: first != nil}
	})
}

// listen plays an audio file through the computer's speakers; pressing it again while it is
// still sounding stops it, and once it has finished it plays again.
func (m Model) listen(path string) (tea.Model, tea.Cmd) {
	if m.player == nil {
		m.message = "no audio output to listen on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	if m.listening == path && m.player.Playing() {
		m.stopListening()
		return m, nil
	}
	return m.playFile(path)
}

// playFile starts a file playing, whatever is playing now.
func (m Model) playFile(path string) (tea.Model, tea.Cmd) {
	if m.player == nil {
		return m.listen(path)
	}
	m.listening = path
	m.message = "decoding " + app.Base(path) + "…"
	player, unit := m.player, m.unit
	return m, func() tea.Msg {
		imported, err := app.LoadSample(unit, path)
		if err != nil {
			return listenMsg{path: path, err: err}
		}
		return listenMsg{path: path, err: player.Play(imported.Sample)}
	}
}

// autoplayPath is the audio file the search or the assign dialog has under the cursor.
func (m Model) autoplayPath() string {
	switch {
	case m.search != nil:
		if f, ok := m.search.current(); ok {
			return f.Path
		}
	case m.browser != nil && m.browser.kind == browseAssign && m.browser.tree != nil && m.browser.at < len(m.browser.tree.Rows()):
		if node := m.browser.tree.Rows()[m.browser.at]; !node.Dir && isAudio(node.Name) {
			return node.Path
		}
	}
	return ""
}

// autoplayDelay is how long the cursor has to rest before autoplay plays what it is on, so
// scrolling through a list does not play every file it passes.
const autoplayDelay = 250 * time.Millisecond

// scheduleAutoplay plays the file under the cursor shortly, if autoplay is on.
func (m *Model) scheduleAutoplay() tea.Cmd {
	if !m.autoplay || m.player == nil {
		return nil
	}
	m.autoSeq++
	seq := m.autoSeq
	return tea.Tick(autoplayDelay, func(time.Time) tea.Msg { return autoplayMsg{seq: seq} })
}

// paste moves the marked path into dir.
func (m Model) paste(dir string) (tea.Model, tea.Cmd) {
	if m.fileMoved == "" {
		m.message = "nothing marked: m marks a file or folder to move"
		return m, nil
	}
	from, unit := m.fileMoved, m.unit
	m.fileMoved = ""
	m.busy = "moving"
	cmd := m.afterFiles(func() string {
		to, err := app.MoveInto(unit, from, dir)
		if err != nil {
			return fmt.Sprintf("moving %s failed: %v", display(from), err)
		}
		return fmt.Sprintf("moved to %s", display(to))
	})
	return m, cmd
}

// afterFiles runs a change to the files and then lists the volume again, with the same
// folders open.
func (m *Model) afterFiles(change func() string) tea.Cmd {
	// What was indexed for searching may have changed.
	clear(m.indexes)
	root, unit := m.root, m.unit
	var open []string
	if m.tree != nil {
		open = m.tree.Expanded()
	}
	return m.edit(func() tea.Msg {
		note := change()
		tree, err := app.LoadTree(unit, root, open)
		return treeMsg{root: root, tree: tree, err: err, note: note}
	})
}

// ---- overlays ----

func (m Model) paletteKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.palette
	found := m.matches()
	switch name := key.String(); {
	case name == "esc":
		m.palette = nil
	case name == "enter":
		m.palette = nil
		if p.cursor < len(found) {
			return m.key(keyFor(found[p.cursor].key))
		}
	case name == "up" || name == "ctrl+p":
		p.cursor = max(p.cursor-1, 0)
	case name == "down" || name == "ctrl+n":
		p.cursor = max(min(p.cursor+1, len(found)-1), 0)
	case name == "backspace":
		if p.filter != "" {
			p.filter = p.filter[:len(p.filter)-1]
		}
		p.cursor = 0
	case key.Type == tea.KeyRunes || key.Type == tea.KeySpace:
		p.filter += string(key.Runes)
		p.cursor = 0
	}
	return m, nil
}

func (m Model) pickerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.picker
	name := key.String()
	step := func(dx, dy int) {
		col, row := p.pad%4, p.pad/4
		p.pad = clamp(row+dy, 0, 3)*4 + clamp(col+dx, 0, 3)
		p.confirm = false
	}
	// A capital letter names a bank, and digits type a pad number: C, 7, Enter puts the
	// file on C7.
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'J' {
		p.bank, p.confirm, p.digits = int(name[0]-'A'), false, ""
		p.pad = m.pickBankPad(p)
		return m, nil
	}
	if !p.retarget && len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
		digits := p.digits + name
		if n := atoi(digits); n < 1 || n > proto.PadsPerBank {
			digits = name
		}
		if n := atoi(digits); n >= 1 && n <= proto.PadsPerBank {
			p.pad, p.confirm = n-1, false
		}
		p.digits = digits
		if atoi(digits)*10 > proto.PadsPerBank {
			p.digits = ""
		}
		return m, nil
	}
	p.digits = ""
	switch name {
	case "esc", "q":
		m.picker = nil
	case "[", "H":
		p.bank, p.confirm = (p.bank+proto.Banks-1)%proto.Banks, false
		p.pad = m.pickBankPad(p)
	case "]", "L":
		p.bank, p.confirm = (p.bank+1)%proto.Banks, false
		p.pad = m.pickBankPad(p)
	case "h", "left":
		if p.retarget {
			p.bank = (p.bank + proto.Banks - 1) % proto.Banks
		} else {
			step(-1, 0)
		}
	case "l", "right":
		if p.retarget {
			p.bank = (p.bank + 1) % proto.Banks
		} else {
			step(1, 0)
		}
	case "j", "down":
		step(0, 1)
	case "k", "up":
		step(0, -1)
	case "enter":
		return m.pickerEnter()
	}
	return m, nil
}

// pickBankPad is the pad to rest on in the bank the picker moved to: the next free one
// (for a copy, the first free one).
func (m Model) pickBankPad(p *picker) int {
	switch {
	case p.retarget:
		return 0
	case len(p.copyFrom) > 0:
		return m.freeAfter(p.bank, -1)
	}
	return m.nextFree(p.bank)
}

// freeAfter is the first pad without a sample after position from in a bank, wrapping
// round; from itself when the bank is full.
func (m Model) freeAfter(bank, from int) int {
	rows := m.snap.Bank(bank)
	for i := 1; i <= proto.PadsPerBank; i++ {
		if p := (from + i + proto.PadsPerBank) % proto.PadsPerBank; !rows[p].HasSample {
			return p
		}
	}
	return max(from, 0)
}

// copyEnter copies the picker's pads to the chosen pad; pads that hold a sample are
// replaced only on a second Enter.
func (m Model) copyEnter() (tea.Model, tea.Cmd) {
	p := m.picker
	target, _ := proto.PadFromBankPad(uint16(p.bank), uint16(p.pad))
	copies, err := app.PlanCopy(p.copyFrom, target)
	if err != nil {
		m.picker = nil
		m.message = err.Error()
		return m, nil
	}
	if len(copies) == 0 {
		m.picker = nil
		m.message = "that is the pad itself"
		return m, nil
	}
	held := func(pad proto.PadIndex) bool { return m.snap.Pads[pad.Index()].HasSample }
	if app.Occupied(copies, held) > 0 && !p.confirm {
		p.confirm = true
		return m, nil
	}
	m.picker = nil
	m.marked = map[int]bool{}
	m.busy = "copying"
	unit, first := m.unit, copies[0].To
	return m, m.edit(func() tea.Msg {
		n, failed := app.CopyPads(unit, copies)
		message := fmt.Sprintf("%d pads copied, from %s to %s on", n, copies[0].From, first)
		if len(copies) == 1 {
			message = fmt.Sprintf("%s copied to %s", copies[0].From, first)
		}
		if len(failed) > 0 {
			message += "; failed: " + strings.Join(failed, ", ")
		}
		return doneMsg{message: message, reload: true, focus: &first}
	})
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func (m Model) pickerEnter() (tea.Model, tea.Cmd) {
	p := m.picker
	if p.retarget {
		m.picker = nil
		slot, from, to, unit := p.slot, p.sourceBank, p.bank, m.unit
		if from == to {
			m.message = "target bank is the same as the source; nothing to do"
			return m, nil
		}
		m.busy = "retargeting"
		return m, m.edit(func() tea.Msg {
			n, err := app.RetargetPattern(unit, slot, from, to)
			if err != nil {
				return patternMsg{err: err}
			}
			note := fmt.Sprintf("%s: %d note(s) moved from bank %c to bank %c", slot, n, 'A'+from, 'A'+to)
			detail, err := app.OpenPattern(unit, slot)
			return patternMsg{detail: detail, err: err, note: note}
		})
	}
	if len(p.copyFrom) > 0 {
		return m.copyEnter()
	}
	target, _ := proto.PadFromBankPad(uint16(p.bank), uint16(p.pad))
	held := m.snap.Pads[target.Index()].HasSample
	// A pad that holds a sample is replaced only on a second Enter.
	if held && len(p.files) == 1 && !p.confirm {
		p.confirm = true
		return m, nil
	}
	files := p.files
	m.picker = nil
	return m.assign(files, target)
}

func (m Model) promptKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.prompt
	if p.kind == promptBank {
		return m.bankPromptKey(key)
	}
	name := key.String()
	if name == "esc" {
		m.prompt = nil
		return m, nil
	}
	if p.kind == promptDelete || p.kind == promptConfirmRestore {
		switch name {
		case "y", "enter":
			return m.apply()
		case "n", "q":
			m.prompt = nil
		}
		return m, nil
	}
	switch {
	case name == "enter":
		return m.apply()
	case name == "backspace":
		if p.buffer != "" {
			r := []rune(p.buffer)
			p.buffer = string(r[:len(r)-1])
		}
	case name == "ctrl+u":
		p.buffer = ""
	case name == "tab" && p.kind == promptBulkRename:
		p.suffix = !p.suffix
	case key.Type == tea.KeyRunes || key.Type == tea.KeySpace:
		for _, r := range key.Runes {
			// The unit only accepts printable ASCII in names; paths and searches can hold more.
			if r >= ' ' && (r <= '~' || p.kind == promptExport || p.kind == promptRestore || p.kind == promptSearch || p.kind == promptExportName || p.kind == promptAssignFile || p.kind == promptProjectExport || p.kind == promptProjectExportName || p.kind == promptProjectRestore) {
				p.buffer += string(r)
			}
		}
	}
	return m, nil
}

// apply carries out the prompt's answer.
func (m Model) apply() (tea.Model, tea.Cmd) {
	p, unit := m.prompt, m.unit
	m.prompt = nil
	text := strings.TrimSpace(p.buffer)
	if p.kind == promptConfirmRestore {
		plan := p.plan
		m.busy = fmt.Sprintf("restoring project %d", plan.Project+1)
		return m, m.edit(func() tea.Msg {
			if err := app.RestoreProject(unit, plan, nil); err != nil {
				return doneMsg{message: "restore failed: " + err.Error(), reload: true}
			}
			return doneMsg{message: fmt.Sprintf("project %d restored from %s", plan.Project+1, plan.Dir), reload: true}
		})
	}
	if p.kind != promptDelete && text == "" {
		m.message = "nothing typed"
		return m, nil
	}
	switch p.kind {
	case promptSearch:
		return m.find(text)
	case promptAssignFile:
		return m.assign([]string{text}, p.pads[0])
	case promptProjectRestore:
		return m.planProjectRestore(p.project, text)
	case promptProjectExport, promptProjectExportName:
		dir := text
		if p.kind == promptProjectExportName {
			dir = app.Join(p.path, text)
		}
		project := p.project
		m.busy = fmt.Sprintf("exporting project %d", project+1)
		return m, m.edit(func() tea.Msg {
			summary, err := app.ExportProject(unit, project, dir, nil)
			if err != nil {
				return doneMsg{message: "export failed: " + err.Error()}
			}
			return doneMsg{message: summary}
		})
	case promptRenamePad:
		pad := p.pads[0]
		m.busy = "renaming"
		return m, m.edit(func() tea.Msg {
			if err := unit.SetSampleName(pad, text); err != nil {
				return doneMsg{message: fmt.Sprintf("renaming %s failed: %v", pad, err)}
			}
			return doneMsg{message: fmt.Sprintf("%s renamed to %s", pad, text), reload: true}
		})
	case promptRenameProject:
		m.busy = "renaming"
		return m, m.edit(func() tea.Msg {
			if err := unit.SetProjectName(p.project, text); err != nil {
				return doneMsg{message: err.Error()}
			}
			return doneMsg{message: fmt.Sprintf("project %d renamed to %s", p.project+1, text), reload: true}
		})
	case promptBulkRename:
		rows := p.rows
		m.marked = map[int]bool{}
		m.busy = "renaming"
		return m, m.edit(func() tea.Msg {
			renamed, failed := app.BulkRename(unit, rows, text, p.suffix)
			message := fmt.Sprintf("%d pads renamed", renamed)
			if len(failed) > 0 {
				message = fmt.Sprintf("%d renamed, failed: %s", renamed, strings.Join(failed, ", "))
			}
			return doneMsg{message: message, reload: true}
		})
	case promptDelete:
		m.marked = map[int]bool{}
		m.busy = "deleting"
		return m, m.edit(func() tea.Msg {
			failed := app.DeletePads(unit, p.pads)
			message := fmt.Sprintf("%d pads deleted", len(p.pads))
			switch {
			case len(failed) > 0:
				message = "delete failed: " + strings.Join(failed, ", ")
			case len(p.pads) == 1:
				message = fmt.Sprintf("%s: sample deleted", p.pads[0])
			}
			return doneMsg{message: message, reload: true}
		})
	case promptExport, promptExportName:
		dir := text
		if p.kind == promptExportName {
			dir = app.Join(p.path, text)
		}
		pads := proto.AllPads()[m.bank*proto.PadsPerBank : (m.bank+1)*proto.PadsPerBank]
		m.busy = "exporting"
		return m, m.edit(func() tea.Msg {
			summary, err := app.ExportPads(unit, pads, dir, nil)
			if err != nil {
				return doneMsg{message: "export failed: " + err.Error()}
			}
			return doneMsg{message: summary}
		})
	case promptRestore:
		return m.restoreFrom(text)
	case promptRenameFile:
		m.busy = "renaming"
		cmd := m.afterFiles(func() string {
			to, err := app.RenamePath(unit, p.path, text)
			if err != nil {
				return fmt.Sprintf("renaming %s failed: %v", display(p.path), err)
			}
			return fmt.Sprintf("renamed to %s", display(to))
		})
		return m, cmd
	}
	return m, nil
}

// restoreFrom puts the pads of the export in dir back.
func (m Model) restoreFrom(dir string) (tea.Model, tea.Cmd) {
	unit := m.unit
	m.busy = "restoring"
	return m, m.edit(func() tea.Msg {
		summary, err := app.RestorePads(unit, dir, nil)
		if err != nil {
			return doneMsg{message: "restore failed: " + err.Error(), reload: true}
		}
		return doneMsg{message: summary, reload: true}
	})
}

// find moves the cursor to the next thing whose name contains text, wrapping around.
func (m Model) find(text string) (tea.Model, tea.Cmd) {
	needle := strings.ToLower(text)
	has := func(name string) bool { return strings.Contains(strings.ToLower(name), needle) }
	switch m.section {
	case secPads:
		here := int(m.padRow().Pad.Index())
		for i := 1; i <= proto.PadCount; i++ {
			row := m.snap.Pads[(here+i)%proto.PadCount]
			if row.HasSample && has(row.Name) {
				m.bank, m.pad = int(row.Pad.Bank()), int(row.Pad.Pad())
				m.message = fmt.Sprintf("%s: %s", row.Pad, row.Name)
				cmd := m.wanted()
				return m, cmd
			}
		}
	case secFiles:
		if m.tree != nil {
			rows := m.tree.Rows()
			for i := 1; i <= len(rows); i++ {
				if at := (m.fileAt + i) % len(rows); has(rows[at].Name) {
					m.fileAt = at
					cmd := m.wanted()
					return m, cmd
				}
			}
		}
	case secProjects:
		for i := 1; i <= len(m.snap.Projects); i++ {
			if at := (m.projectAt + i) % len(m.snap.Projects); has(m.snap.Projects[at]) {
				m.projectAt = at
				return m, nil
			}
		}
	}
	m.message = fmt.Sprintf("nothing matches %q", text)
	return m, nil
}
