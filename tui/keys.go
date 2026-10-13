package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
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
	{"o", "Preview pads on this computer or on the unit (switch; O and 0 work too)", []int{secPads}},
	{"P", "Play this pad once on this computer", []int{secPads}},
	{"C", "Chromatic: play this pad as notes from the keyboard, on this computer", []int{secPads}},
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
	{"R", "Restore pads from an export: choose its folder, then the bank", []int{secPads}},
	{"enter", "Open pattern", []int{secPatterns}},
	{"b", "Retarget the open pattern to another bank", []int{secPatterns}},
	{"enter", "Assign this file to a pad", []int{secFiles}},
	{"[", "Target bank for assigns: previous (Files)", []int{secFiles}},
	{"]", "Target bank for assigns: next (Files)", []int{secFiles}},
	{"space", "Listen to this file", []int{secFiles}},
	{"v", "Mark a file or folder (V: every file in the folder, J: junk files)", []int{secFiles}},
	{"J", "Mark macOS and Windows junk files in the open folders (._ files, .DS_Store)", []int{secFiles}},
	{"d", "Delete the marked files and folders from the SD card", []int{secFiles}},
	{"c", "Copy the marked files and folders (then p pastes)", []int{secFiles}},
	{"x", "Cut the marked files and folders to move them (then p pastes)", []int{secFiles}},
	{"p", "Paste into the folder in front (also from another source: t)", []int{secFiles}},
	{"N", "New folder in the folder in front", []int{secFiles}},
	{"tab", "Open / close folder", []int{secFiles}},
	{"n", "Rename file or folder", []int{secFiles}},
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
	{"shift+tab", "Fold every folder in the list", []int{secFiles}},
	{"f", "Find a folder in the list by name, then h or Tab collapses it", []int{secFiles}},
	{"ctrl+a", "Autoplay: files play as the cursor rests on them", []int{secFiles}},
	{"}", "Jump to the next folder in the list (Alt-n too)", []int{secFiles}},
	{"{", "Jump to the previous folder in the list (Alt-p too)", []int{secFiles}},
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
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// key handles a key press, then keeps the state that is remembered between runs.
func (m Model) key(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.log.Debug("key", "key", key.String(), "section", sectionNames[m.section], "overlay", m.overlayName(), "busy", m.busy)
	model, cmd := m.handleKey(key)
	if updated, ok := model.(Model); ok {
		updated.persist()
		return updated, cmd
	}
	return model, cmd
}

// overlayName names the dialog in front, for the log.
func (m Model) overlayName() string {
	switch {
	case m.help:
		return "help"
	case m.search != nil:
		return "search"
	case m.editor != nil:
		return "editor"
	case m.chromatic != nil:
		return "chromatic"
	case m.browser != nil:
		return "browser"
	case m.prompt != nil:
		return fmt.Sprintf("prompt %d", m.prompt.kind)
	case m.picker != nil:
		return "picker"
	case m.palette != nil:
		return "palette"
	}
	return ""
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
	case m.chromatic != nil:
		return m.chromaticKey(key)
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
		if m.section == secFiles && m.tree != nil {
			m.fileAt = m.tree.CollapseAll(m.fileAt)
			m.message = "folded everything"
			return m, m.wanted()
		}
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
	case len(m.clip) > 0:
		m.clip = nil
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
	// A file still being decoded must not start once this has been asked for.
	m.listenGen.Add(1)
	if m.player != nil {
		m.player.Stop()
	}
	m.listening = ""
}

// pageStep is how far a paging key moves a list that shows height rows: Ctrl-D and Ctrl-U go
// half a page down and up, as in vim, Page Down and Page Up a whole page.
func (m Model) pageStep(key string, height int) int {
	half := max(height/2, 1)
	switch key {
	case "ctrl+d":
		return half
	case "ctrl+u":
		return -half
	case "pgdown":
		return max(height-1, 1)
	}
	return -max(height-1, 1)
}

// folderRow is the row of the next folder below row at (step 1) or the previous one above it
// (step -1), at any depth; ok is false when there is none that way.
func folderRow(rows []app.Node, at, step int) (row int, ok bool) {
	for i := at + step; i >= 0 && i < len(rows); i += step {
		if rows[i].Dir {
			return i, true
		}
	}
	return at, false
}

// jumpFolder moves a list cursor to the next or previous folder.
func jumpFolder(rows []app.Node, at int, key string) (int, string) {
	step := 1
	if key == "{" || key == "alt+p" {
		step = -1
	}
	if row, ok := folderRow(rows, at, step); ok {
		return row, ""
	}
	if step > 0 {
		return at, "no more folders below"
	}
	return at, "no more folders above"
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
		if m.previewLocal {
			return m.previewPadLocal(row)
		}
		return m.previewPad(row)
	case "P":
		if !row.HasSample {
			m.message = fmt.Sprintf("%s is empty; a assigns a file to it", row.Pad)
			break
		}
		return m.previewPadLocal(row)
	case "C":
		return m.openChromatic(row)
	case "o", "O", "0":
		m.previewLocal = !m.previewLocal
		where := "the unit"
		if m.previewLocal {
			where = "this computer"
			if m.audioOut != "" {
				where += " (" + m.audioOut + ")"
			}
		}
		m.message = "pads play on " + where + " when previewed (o switches, P plays once on the computer)"
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

// previewPad plays a pad on the unit. The pad is selected first and checked, because the
// unit plays the previously selected pad on the first press otherwise; the preview then stops
// by itself after the sample's length, and pressing again restarts it.
func (m Model) previewPad(row app.PadRow) (tea.Model, tea.Cmd) {
	m.previewSeq++
	seq, pad := m.previewSeq, row.Pad
	previous := m.previewing
	m.previewing = &pad
	m.message = fmt.Sprintf("previewing %s", pad)
	duration := m.previewLength(row)
	unit, log := m.unit, m.log
	return m, m.edit(func() tea.Msg {
		// Another pad that is still playing is stopped first.
		if previous != nil && *previous != pad {
			_ = app.StopPreview(unit, *previous)
		}
		err := app.StartPreview(unit, pad, func(msg string, args ...any) { log.Debug(msg, args...) })
		return previewMsg{pad: pad, seq: seq, err: err, duration: duration}
	})
}

// previewPadLocal plays a pad's sample on this computer, the way a file is auditioned: it is
// read from the unit once, cut to the part the pad plays, and played through the computer's
// audio output. Pressing again restarts it, and another pad replaces it. This is the sample as
// stored: the unit's pitch, envelope and effects are not applied.
func (m Model) previewPadLocal(row app.PadRow) (tea.Model, tea.Cmd) {
	if !row.HasSample {
		m.message = fmt.Sprintf("%s is empty; a assigns a file to it", row.Pad)
		return m, nil
	}
	if m.player == nil {
		m.message = "no audio output to play on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	// The sound that is playing stops now, and a file still being read must not start later.
	m.player.Stop()
	id := m.listenGen.Add(1)
	label := "pad " + row.Pad.String()
	m.listening = label
	m.message = "reading " + row.Pad.String() + " from the unit…"
	key := factKey{row.Pad, row.Size, row.Name}
	player, unit, ops, log, gen, cache := m.player, m.unit, m.ops, m.log, m.listenGen, m.padAudio
	return m, func() tea.Msg {
		start := time.Now()
		sample, cached := cache.get(key)
		if !cached {
			ops.Lock()
			var err error
			sample, err = app.PadAudio(unit, row)
			ops.Unlock()
			if err != nil {
				log.Warn("could not read the pad to play it on this computer", "pad", row.Pad.String(), "err", err)
				return listenMsg{path: label, err: err}
			}
			cache.put(key, sample)
		}
		if gen.Load() != id {
			return listenMsg{path: label, stale: true}
		}
		err := player.Play(sample)
		log.Debug("playing a pad on this computer", "pad", row.Pad.String(), "frames", sample.Frames(),
			"from the unit", !cached, "took", time.Since(start).Round(time.Millisecond), "err", err)
		return listenMsg{path: label, err: err}
	}
}

// previewLength is how long a preview plays: the sample's length when it is known, at most
// eight seconds (a looping pad would go on forever).
func (m Model) previewLength(row app.PadRow) time.Duration {
	if res, ok := m.facts[factKey{row.Pad, row.Size, row.Name}]; ok && res.err == nil && res.facts.Seconds > 0 {
		return time.Duration(min(max(res.facts.Seconds, 0.3)+0.25, 8) * float64(time.Second))
	}
	return 2 * time.Second
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
	case "ctrl+d", "ctrl+u", "pgdown", "pgup":
		m.projectAt = clamp(m.projectAt+m.pageStep(name, m.geom().bodyH-2), 0, len(m.snap.Projects)-1)
	case "g":
		m.projectAt = 0
	case "G":
		m.projectAt = len(m.snap.Projects) - 1
	case "enter", " ":
		project := uint8(m.projectAt)
		if project == m.snap.Project {
			m.message = fmt.Sprintf("project %d is already the current project", project+1)
			break
		}
		m.busy = fmt.Sprintf("loading project %d", project+1)
		unit := m.unit
		return m, m.edit(func() tea.Msg {
			// Read the unit again either way: a switch that was reported late or failed may
			// have happened, and the screen should show where the unit really is.
			if err := unit.SelectProject(project); err != nil {
				return doneMsg{message: fmt.Sprintf("project %d: %v", project+1, err), reload: true}
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
	before := m.fileAt
	switch name {
	case "ctrl+a":
		return m.toggleAutoplay()
	case "[":
		m.target = (m.target + proto.Banks - 1) % proto.Banks
		return m, nil
	case "]":
		m.target = (m.target + 1) % proto.Banks
		return m, nil
	case "t":
		m.root = nextRoot(m.root)
		// The clipboard stays, so that a paste can land on the other side.
		m.tree, m.fileAt = nil, 0
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
		return m.paste()
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
	case "ctrl+d", "ctrl+u", "pgdown", "pgup":
		m.fileAt = clamp(at+m.pageStep(name, m.geom().bodyH-2), 0, len(rows)-1)
	case "}", "{", "alt+n", "alt+p":
		m.fileAt, m.message = jumpFolder(rows, at, name)
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
	case "f":
		m.prompt = &prompt{kind: promptFindFolder}
		return m, nil
	case "a":
		if isAudio(node.Name) {
			return m.startAssign(node)
		}
		m.message = "that is not an audio file"
	case "v":
		m = m.toggleMark(node)
	case "V":
		m = m.markFolder()
	case "J":
		m = m.markJunk()
	case "u":
		m.fileMarks = map[string]bool{}
		m.message = "unmarked everything"
	case "c":
		m = m.copyFiles(false)
	case "x", "m":
		m = m.copyFiles(true)
	case "d":
		return m.deleteFiles()
	case "N":
		return m.newFolder()
	case "n":
		m.prompt = &prompt{kind: promptRenameFile, path: node.Path, buffer: node.Name}
	case "R":
		return m.openBrowser(browseRestore, m.root, m.tree.Expanded(), node.Path)
	case " ":
		if !isAudio(node.Name) {
			break
		}
		return m.listen(node.Path)
	}
	cmd := m.wanted()
	// Moving on to another row plays it, if autoplay is on, and stops the old sound at once.
	if m.fileAt != before {
		cmd = tea.Batch(cmd, m.scheduleAutoplay())
	}
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
			if m.fileMarks[r.Path] && !r.Dir && isAudio(r.Name) {
				files = append(files, r.Path)
			}
		}
		if len(files) == 0 {
			m.message = "none of the marked items is an audio file"
			return m, nil
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
	// Whatever is playing stops now, not when the new file has been decoded.
	m.player.Stop()
	id := m.listenGen.Add(1)
	m.listening = path
	m.message = "decoding " + app.Base(path) + "…"
	player, unit, log, gen := m.player, m.unit, m.log, m.listenGen
	return m, func() tea.Msg {
		start := time.Now()
		imported, err := app.LoadSample(unit, path)
		if err != nil {
			log.Warn("could not decode the file to listen to it", "path", path, "err", err)
			return listenMsg{path: path, err: err}
		}
		if gen.Load() != id {
			log.Debug("dropped a file that was still decoding when something else was asked for", "path", path)
			return listenMsg{path: path, stale: true}
		}
		err = player.Play(imported.Sample)
		log.Debug("playing on this computer", "path", path, "decoded in", time.Since(start).Round(time.Millisecond),
			"seconds", float64(imported.Sample.Frames())/formats.SampleRate, "source rate", imported.SourceRate, "err", err)
		return listenMsg{path: path, err: err}
	}
}

// toggleAutoplay turns autoplay on or off.
func (m Model) toggleAutoplay() (tea.Model, tea.Cmd) {
	m.autoplay = !m.autoplay
	switch {
	case m.autoplay && m.player == nil:
		m.autoplay = false
		m.message = "no audio output to autoplay on"
		return m, nil
	case !m.autoplay:
		m.stopListening()
		m.message = "autoplay off"
		return m, nil
	}
	m.message = "autoplay on: files play as the cursor rests on them"
	cmd := m.scheduleAutoplay()
	return m, cmd
}

// autoplayPath is the audio file the search or the assign dialog has under the cursor.
func (m Model) autoplayPath() string {
	switch {
	case m.search == nil && m.browser == nil && m.picker == nil && m.prompt == nil && m.section == secFiles:
		if node, ok := m.fileNode(); ok && !node.Dir && isAudio(node.Name) {
			return node.Path
		}
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
var autoplayDelay = 250 * time.Millisecond

// scheduleAutoplay plays the file under the cursor shortly, if autoplay is on.
//
// The sound that is playing stops at once, at the first move: waiting for the next file to be
// decoded would leave the old one playing for most of a second.
func (m *Model) scheduleAutoplay() tea.Cmd {
	if !m.autoplay || m.player == nil {
		return nil
	}
	m.stopListening()
	m.autoSeq++
	seq := m.autoSeq
	return tea.Tick(autoplayDelay, func(time.Time) tea.Msg { return autoplayMsg{seq: seq} })
}

// afterFiles runs a change to the files and then lists the volume again, with the same
// folders open.
func (m *Model) afterFiles(change func() string) tea.Cmd {
	return m.afterFilesAt("", change)
}

// afterFilesAt is afterFiles, with the cursor put on the row at land afterwards.
func (m *Model) afterFilesAt(land string, change func() string) tea.Cmd {
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
		return treeMsg{root: root, tree: tree, err: err, note: note, at: land}
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
	if !p.retarget && p.restorePlan == nil && len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
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
		if p.retarget || p.restorePlan != nil {
			p.bank, p.confirm = (p.bank+proto.Banks-1)%proto.Banks, false
		} else {
			step(-1, 0)
		}
	case "l", "right":
		if p.retarget || p.restorePlan != nil {
			p.bank, p.confirm = (p.bank+1)%proto.Banks, false
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
	case p.retarget || p.restorePlan != nil:
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
	if p.restorePlan != nil {
		return m.restoreEnter()
	}
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
		if p.reopen != nil {
			return m.openAssign(*p.reopen)
		}
		return m, nil
	}
	if p.kind == promptDelete || p.kind == promptConfirmRestore || p.kind == promptDeleteFiles {
		switch name {
		case "y", "enter":
			return m.apply()
		case "n", "q":
			m.prompt = nil
			if p.reopen != nil {
				return m.openAssign(*p.reopen)
			}
		}
		return m, nil
	}
	if p.kind == promptFindFolder && m.tree != nil {
		switch name {
		case "up", "down", "ctrl+n", "ctrl+p", "tab", "shift+tab":
			p.choice = stepChoice(p.choice, len(folderMatches(m.tree.Rows(), p.buffer)), name)
			return m, nil
		}
		if name != "enter" {
			p.choice = 0 // typing starts the list over
		}
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
			if r >= ' ' && (r <= '~' || p.kind == promptExport || p.kind == promptRestore || p.kind == promptSearch || p.kind == promptExportName || p.kind == promptAssignFile || p.kind == promptMkdir || p.kind == promptFindFolder || p.kind == promptProjectExport || p.kind == promptProjectExportName || p.kind == promptProjectRestore) {
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
	if p.kind == promptDeleteFiles {
		paths := p.paths
		m.fileMarks = map[string]bool{}
		// What was deleted cannot be pasted.
		m.clip = slices.DeleteFunc(slices.Clone(m.clip), func(c string) bool { return slices.Contains(paths, c) })
		m.busy = "deleting"
		deleteAll := func() string {
			deleted, failed := app.DeletePaths(unit, paths, nil)
			note := fmt.Sprintf("%s deleted", plural(deleted, "item"))
			if len(failed) > 0 {
				note += "; not deleted: " + strings.Join(failed, ", ")
			}
			return note
		}
		if pad := p.reopen; pad != nil {
			// From the assign dialog: its file list is read afresh when it opens again, and
			// what was indexed for searching may have changed. The cursor stays where it was,
			// or on the folder that held what is gone.
			clear(m.indexes)
			m.mem.AssignAt = survivor(m.mem.AssignAt, paths)
			return m, m.edit(func() tea.Msg { return reopenAssignMsg{pad: *pad, note: deleteAll()} })
		}
		land := ""
		if n, ok := m.fileNode(); ok {
			land = survivor(n.Path, paths)
		}
		cmd := m.afterFilesAt(land, deleteAll)
		return m, cmd
	}
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
	if p.kind != promptDelete && p.kind != promptFindFolder && text == "" {
		m.message = "nothing typed"
		return m, nil
	}
	switch p.kind {
	case promptSearch:
		return m.find(text)
	case promptFindFolder:
		return m.findFolder(text, p.choice), nil
	case promptAssignFile:
		return m.assign([]string{text}, p.pads[0])
	case promptMkdir:
		dir := app.Join(p.path, text)
		m.busy = "creating the folder"
		cmd := m.afterFiles(func() string {
			if err := unit.Mkdir(dir); err != nil {
				return fmt.Sprintf("creating %s failed: %v", display(dir), err)
			}
			return "created " + display(dir)
		})
		return m, cmd
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
		to := app.Join(app.Dir(p.path), text)
		rename := func() string {
			if _, err := app.RenamePath(unit, p.path, text); err != nil {
				return fmt.Sprintf("renaming %s failed: %v", display(p.path), err)
			}
			return fmt.Sprintf("renamed to %s", display(to))
		}
		if pad := p.reopen; pad != nil {
			// From the assign dialog: it opens again on the renamed row, with the folders
			// that were open (the renamed one under its new name).
			clear(m.indexes)
			from := p.path
			m.mem.AssignAt = to
			for i, e := range m.mem.AssignExpanded {
				if e == from || strings.HasPrefix(e, from+"/") {
					m.mem.AssignExpanded[i] = to + strings.TrimPrefix(e, from)
				}
			}
			return m, m.edit(func() tea.Msg { return reopenAssignMsg{pad: *pad, note: rename()} })
		}
		cmd := m.afterFilesAt(to, rename)
		return m, cmd
	}
	return m, nil
}

// restoreFrom reads the export in dir; the bank to restore it onto is asked next.
func (m Model) restoreFrom(dir string) (tea.Model, tea.Cmd) {
	unit := m.unit
	m.busy = "reading the export"
	return m, m.edit(func() tea.Msg {
		plan, err := app.PlanPadRestore(unit, dir)
		return padRestoreMsg{plan: plan, err: err}
	})
}

// runRestore puts an export onto a bank (-1: back where each pad came from).
func (m Model) runRestore(plan *app.PadRestore, bank int) (tea.Model, tea.Cmd) {
	unit := m.unit
	targets, err := plan.Targets(bank)
	if err != nil {
		m.message = err.Error()
		return m, nil
	}
	first := targets[0]
	m.busy = fmt.Sprintf("restoring %d pads", len(plan.Pads))
	return m, m.edit(func() tea.Msg {
		summary, err := app.RestorePadsTo(unit, plan, bank, nil)
		if err != nil {
			return doneMsg{message: "restore failed: " + err.Error(), reload: true}
		}
		return doneMsg{message: summary, reload: true, focus: &first}
	})
}

// restoreEnter restores the picker's export onto the bank chosen; pads that hold a sample
// are replaced only on a second Enter.
func (m Model) restoreEnter() (tea.Model, tea.Cmd) {
	p := m.picker
	targets, err := p.restorePlan.Targets(p.bank)
	if err != nil {
		m.picker = nil
		m.message = err.Error()
		return m, nil
	}
	held := func(pad proto.PadIndex) bool { return m.snap.Pads[pad.Index()].HasSample }
	occupied := 0
	for _, t := range targets {
		if held(t) {
			occupied++
		}
	}
	if occupied > 0 && !p.confirm {
		p.confirm = true
		return m, nil
	}
	plan, bank := p.restorePlan, p.bank
	m.picker = nil
	return m.runRestore(plan, bank)
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
