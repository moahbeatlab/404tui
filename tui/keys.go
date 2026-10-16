package tui

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"math/rand"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	{"V", "Mark every pad with a sample in the bank (again, or Esc, unmarks them)", []int{secPads}},
	{"d", "Delete sample (of every marked pad)", []int{secPads}},
	{"S", "Edit this pad's settings (or the marked pads')", []int{secPads}},
	{",", "Pad level down (of the marked pads)", []int{secPads}},
	{".", "Pad level up (of the marked pads)", []int{secPads}},
	{"o", "Preview pads on this computer or on the unit (switch; O and 0 work too)", []int{secPads}},
	{"P", "Play this pad once on this computer", []int{secPads}},
	{"C", "Chromatic: play this pad as notes from the keyboard, on this computer", []int{secPads}},
	{"Z", "Slicer: slice this pad's sample (in Files, the file)", []int{secPads, secFiles}},
	{"c", "Copy this pad (or the marked ones) to another pad, in any bank", []int{secPads}},
	{"b", "Edit this bank's tempo and volume", []int{secPads, secPatterns}},
	{"<", "Bank tempo down 1 BPM", []int{secPads, secPatterns}},
	{">", "Bank tempo up 1 BPM", []int{secPads, secPatterns}},
	{"-", "Bank volume down", []int{secPads, secPatterns}},
	{"+", "Bank volume up", []int{secPads, secPatterns}},
	{"x", "Pick up pad to move it", []int{secPads}},
	{"p", "Move the picked-up pad here (overwrite)", []int{secPads}},
	{"s", "Swap the picked-up pad with this one; with none picked up, solo this pad", []int{secPads}},
	{"e", "Export this bank: choose a folder, get WAV files + samples.json", []int{secPads}},
	{"R", "Restore pads from an export: choose its folder, then the bank (p there also makes a pattern)", []int{secPads}},
	{"t", "Set gate, loop, BPM sync, vinyl, one shot, fixed velocity or bus FX on the highlighted pad", []int{secPads}},
	{"T", "Set those pad settings on every pad of the bank at once", []int{secPads}},
	{"m", "Mute this pad in the pattern playing on this computer (again unmutes)", []int{secPads, secPatterns}},
	{"M", "Solo this pad in the pattern playing on this computer: only the soloed pads sound (again unsolos); s does the same", []int{secPads, secPatterns}},
	{"S", "In an open pattern: save it as you hear it into a slot you choose, without the muted pads (or with only the soloed ones)", []int{secPatterns}},
	{"B", "In an open pattern: bounce it as you hear it (m mutes, s solos a track) to one sample on a pad you choose", []int{secPatterns}},
	{"N", "New pattern: pick a template (all pads held, blank, or a genre groove)", []int{secPads, secPatterns}},
	{"F", "Find where the samples on the pads came from, for the pads with no origin noted: names and lengths are matched against the index", []int{secPads}},
	{"W", "Find which folder each bank of this project was filled from, and note it, so the Files view shows the folder as used (needs the folders indexed)", []int{secPads}},
	{"tab", "Move between the pads and the patterns panel under them (in an open pattern: expand / fold the track on the cursor row)", []int{secPads, secPatterns}},
	{"enter", "Open pattern", []int{secPatterns}},
	{"d", "Delete the pattern (or the marked ones)", []int{secPatterns}},
	{"v", "Mark / unmark a pattern for deleting", []int{secPatterns}},
	{"c", "Pick up the pattern to copy it (then p puts it in a slot, any bank)", []int{secPatterns}},
	{"x", "Pick up the pattern to move it (then p puts it in a slot, any bank)", []int{secPatterns}},
	{"p", "Put the picked-up pattern here (twice to replace one)", []int{secPatterns}},
	{"P", "Play the pattern on this computer (again stops it; s stops)", []int{secPatterns}},
	{"b", "Retarget the open pattern to another bank", []int{secPatterns}},
	{"{ }", "In an open pattern: turn to the previous / next page of bars (up to two bars a page)", []int{secPatterns}},
	{"enter", "In an open pattern: expand the track into a big 16-step lane (Tab does too, and folds it again); inside it, note on/off. Esc leaves the pattern", []int{secPatterns}},
	{"x", "In an open pattern: note on/off at the cursor", []int{secPatterns}},
	{"v", "In an open pattern: show, edit and loop only some bars (a bar, or two) — v again for the whole pattern", []int{secPatterns}},
	{"H L", "In an open pattern: nudge the note at the cursor 5 ticks earlier / later (off the grid)", []int{secPatterns}},
	{"( )", "In an open pattern: nudge the note at the cursor 1 tick earlier / later", []int{secPatterns}},
	{"0", "In an open pattern: put the note at the cursor back on the nearest grid step", []int{secPatterns}},
	{"enter", "Assign this file to a pad", []int{secFiles}},
	{"[", "Target bank for assigns: previous (Files)", []int{secFiles}},
	{"]", "Target bank for assigns: next (Files)", []int{secFiles}},
	{"space", "Listen to this file", []int{secFiles}},
	{"v", "Mark a file or folder (V: every file in the folder, J: junk files)", []int{secFiles}},
	{"J", "Mark macOS and Windows junk files in the open folders (._ files, .DS_Store)", []int{secFiles}},
	{"s", "Search tags only (set by i): bass 124, synth house", []int{secFiles}},
	{"*", "Favourite: tag the marked files (or the one in front) favourite; again takes it off", []int{secFiles}},
	{"+", "Tag the marked files with words you type; a word with - in front takes the tag off", []int{secFiles}},
	{"i", "Index the folder in front in the background: tags from file and folder names, saved to SD:404tui-index.json.gz (again stops it)", []int{secFiles}},
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
	{"X", "Erase this project: everything in it, leaving it empty", []int{secProjects}},
	{"[", "Previous bank", []int{secPads, secPatterns}},
	{"]", "Next bank", []int{secPads, secPatterns}},
	{"/", "Find a pad or project by name", []int{secPads, secProjects}},
	{"/", "Search every audio file in this source by keyword", []int{secFiles}},
	{"shift+tab", "Fold every folder in the list", []int{secFiles}},
	{"f", "Find a folder by name, in every indexed folder and the open ones; it is opened for you (h or Tab collapses it)", []int{secFiles}},
	{"ctrl+a", "Autoplay: files play as the cursor rests on them", []int{secFiles}},
	{"w", "Files: switch between three columns (h goes up a folder, l goes into one, j k move along it) and the folder tree", []int{secFiles}},
	{"}", "Jump to the next folder in the list (Alt-n too)", []int{secFiles}},
	{"{", "Jump to the previous folder in the list (Alt-p too)", []int{secFiles}},
	{"h", "Files (three columns): up to the parent folder", []int{secFiles}},
	{"l", "Files (three columns): into the folder in front; Enter and Tab do too", []int{secFiles}},
	{"j", "Files (three columns): the next file or folder in this column; k the one before", []int{secFiles}},
	{"a", "Slicer: choose a pad, or a new sample from the file view, to slice", []int{secSlicer}},
	{"a", "Sequencer: choose a pattern to edit (a again, in the editor, chooses another)", []int{secSequencer}},
	{"N", "Sequencer: make a new pattern", []int{secSequencer}},
	{"1", "Go to Pads", nil},
	{"2", "Go to Files", nil},
	{"3", "Go to Projects", nil},
	{"4", "Go to the Slicer; a there chooses a pad or a new sample to slice", nil},
	{"5", "Go to the Sequencer; a there chooses a pattern to edit", nil},
	{"6", "Go to Config: the words, colours and buses (inside the slicer 6 is an envelope key: use the tab)", nil},
	{"H", "Set up pads that were never set up (hold 1, pan at its left end): pan in the centre, hold 100, time stretch 100%", []int{secPads}},
	{"K", "Check the sample on the pad (or the marked pads) against the file it came from: same length, channels and samples?", []int{secPads}},
	{"B", "Set the bus of the bank's pads (the marked ones; p: the whole project) by their names, as Config says", []int{secPads}},
	{"j", "Config: the next setting; k the one before; h l change it, Enter edits the drum words", []int{secConfig}},
	{"U", "Working offline: leave offline mode and connect to the unit once it is plugged in", nil},
	{"ctrl+r", "Record the unit's audio input to a WAV file in the folder set in Config; again stops it", nil},
	{"ctrl+t", "Next colour theme", nil},
	{"r", "Refresh from the unit", nil},
	{"?", "Keybindings", nil},
	{"q", "Quit", nil},
}

// matches are the commands of the section in front that contain the palette's filter.
func (m *Model) matches() []command {
	var out []command
	if m.palette.items != nil {
		for _, c := range m.palette.items {
			if strings.Contains(strings.ToLower(c.key+" "+c.label), strings.ToLower(m.palette.filter)) {
				out = append(out, c)
			}
		}
		return out
	}
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
	case "ctrl+r":
		return tea.KeyMsg{Type: tea.KeyCtrlR}
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
	case m.slicer != nil:
		return "slicer"
	case m.browser != nil:
		return "browser"
	case m.prompt != nil:
		return fmt.Sprintf("prompt %d", m.prompt.kind)
	case m.picker != nil:
		return "picker"
	case m.toggles != nil:
		return "toggles"
	case m.palette != nil:
		return "palette"
	}
	return ""
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	name := key.String()
	if name == "ctrl+c" {
		m.stopListening()
		m.stopRecordingNow()
		return m, tea.Quit
	}
	// Ctrl-R records the unit's audio from anywhere, in a dialog too.
	if name == "ctrl+r" && !m.typingText() && m.palette == nil {
		return m.toggleRecording()
	}
	// ? lists what can be done with the view or dialog in front, searchable, wherever it is; in a
	// text being typed it is a letter.
	if name == "?" && m.palette == nil && m.unit != nil && m.snap != nil && !m.typingText() && !m.help {
		return m.openContextPalette()
	}
	// While the slicer waits for a sample to add, the main screen is in front: Z takes the one
	// under the cursor, Esc gives up.
	if m.slicer != nil && m.slicer.adding && (name == "esc" || name == "q") {
		m.slicer.adding = false
		m.message = "nothing added"
		return m.goTo(m.slicer.backTo)
	}
	// The help overlay swallows everything except what closes it.
	if m.help {
		if name == "q" || name == "esc" || name == "?" {
			m.help = false
		}
		return m, nil
	}
	switch {
	case m.report != nil:
		// Any key closes a report; the arrows scroll nothing, it is short.
		m.report = nil
		return m, nil
	case m.palette != nil:
		return m.paletteKey(key)
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
	case m.slicer != nil && !m.slicer.adding && m.section == secSlicer:
		// 1 to 5 always change the view; the slicer stays open under its tab.
		if len(name) == 1 && name >= "1" && name <= "5" {
			return m.goTo(railSections[int(name[0]-'1')])
		}
		return m.slicerKey(key)
	case m.toggles != nil:
		return m.togglesKey(key)
	case m.lenPick != nil:
		return m.lengthPickKey(name)
	case m.tmplPicker != nil:
		return m.templatePickerKey(name)
	case m.rangePick != nil:
		return m.rangePickerKey(name)
	case m.laneMap != nil:
		return m.laneAssignKey(name)
	case m.palette != nil:
		return m.paletteKey(key)
	case m.patTool != nil:
		// The groove-tools menu over an open pattern is modal: it takes every key.
		return m.patToolKey(name)
	}
	if m.unit == nil {
		// Nothing to work on until a unit is connected.
		switch name {
		case "q", "esc":
			return m.askQuit()
		case "o":
			if m.openOffline != nil {
				return m.chooseOffline()
			}
		case "?":
			m.help = true
		case "ctrl+t":
			m.themeAt = (m.themeAt + 1) % len(themes)
			m.st = newStyles(themes[m.themeAt])
		}
		return m, nil
	}
	if name == "?" {
		m.help = true
		return m, nil
	}
	if name == "U" && m.offline != "" {
		if !m.unitBack {
			m.message = "no SP-404MKII is plugged in yet: still offline"
			return m, nil
		}
		return m.leaveOffline()
	}
	if name == "q" {
		return m.askQuit()
	}
	if m.busy != "" || m.snap == nil {
		if name == "esc" {
			return m.askQuit()
		}
		return m, nil
	}
	m.message = ""
	if m.addAsk != nil {
		return m.addAskKey(name)
	}
	if m.sliceWant && name == "esc" {
		m.sliceWant = false
		m.message = "nothing chosen"
		return m.goTo(secSlicer)
	}

	switch name {
	case ":":
		m.palette = &palette{}
		return m, nil
	case "esc":
		return m.escape()
	case "1", "2", "3", "4", "5", "6", "7":
		return m.goTo(railSections[int(name[0]-'1')])
	case "shift+tab", "tab":
		switch m.section {
		case secPads:
			// The patterns of the bank are the panel under the pads: Tab moves into it.
			return m.goTo(secPatterns)
		case secPatterns:
			if m.pattern != nil {
				// In an open pattern Tab expands the track on the cursor row into its step
				// lane, and folds it again; Esc leaves the pattern.
				if len(m.pattern.Grid.Rows) > 0 {
					m.patFocus = !m.patFocus
				}
				return m, nil
			}
			return m.goTo(secPads)
		case secFiles:
			if name == "shift+tab" && m.tree != nil {
				m.fileAt = m.tree.CollapseAll(m.fileAt)
				m.message = "folded everything"
				return m, m.wanted()
			}
		default:
			return m.goTo(secPads)
		}
	case "ctrl+t":
		m.themeAt = (m.themeAt + 1) % len(themes)
		m.st = newStyles(themes[m.themeAt])
		m.message = "theme: " + themes[m.themeAt].name
		return m, nil
	case "r":
		m.patterns = map[int][]app.PatternRow{}
		m.patPreview = map[proto.PadIndex]*app.PatternDetail{}
		m.usage, m.usageState = nil, 0
		clear(m.indexes)
		m.tree = nil
		m.infos = map[string]fileInfo{}
		m.pattern, m.patFocus, m.patRange = nil, false, nil
		m.busy = "reading the unit"
		return m, m.load("refreshed")
	case "s":
		if m.section == secFiles {
			return m.openTagSearch(m.root, false, proto.PadIndex{})
		}
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
		if m.seqTab && name == "a" && !m.selecting() {
			// In the Sequencer, a chooses another pattern.
			return m.openPatternAsk()
		}
		return m.patternsKey(name)
	case secSlicer:
		return m.slicerTabKey(name)
	case secSequencer:
		return m.sequencerTabKey(name)
	case secConfig:
		return m.configKey(name)
	case secPerform:
		return m.perfKey(name)
	case secFiles:
		return m.filesKey(name)
	}
	return m.projectsKey(name)
}

// escape backs out of the innermost thing: a pattern, a pick-up, marks; else it quits.
func (m Model) escape() (tea.Model, tea.Cmd) {
	switch {
	case m.selecting() && m.section == secPatterns:
		// Esc ends a selection of notes before it leaves the pattern.
		m.patSel = nil
		m.message = ""
	case m.pattern != nil && m.section == secPatterns:
		// Back to the patterns under the pads, from the grid or from an expanded track; to the
		// Sequencer tab when that is where the pattern was opened.
		m.pattern, m.patRange, m.patFocus, m.patSel = nil, nil, false, nil
		if m.seqTab {
			m.section, m.seqTab, m.seqSlot = secSequencer, false, nil
		}
	case m.section == secPatterns && m.patHolding:
		m.patHolding = false
	case m.section == secPatterns && len(m.patMarks) > 0:
		m.patMarks = map[int]bool{}
	case m.section == secPatterns:
		// Esc leaves the patterns panel for the pads.
		m.section = secPads
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
		return m.askQuit()
	}
	return m, nil
}

// chooseOffline asks which folder of projects on this computer to work on without a unit,
// starting at the one used last time.
func (m Model) chooseOffline() (tea.Model, tea.Cmd) {
	var expanded []string
	at := m.mem.OfflineDir
	for dir := filepath.Dir(at); at != "" && dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		expanded = append([]string{dir}, expanded...)
	}
	return m.openBrowser(browseOffline, localRoot(), expanded, at)
}

// askQuit asks whether to leave the app; y, Enter or q again quits.
func (m Model) askQuit() (tea.Model, tea.Cmd) {
	m.prompt = &prompt{kind: promptQuit}
	return m, nil
}

func (m Model) goTo(section int) (tea.Model, tea.Cmd) {
	if m.section == secPatterns && m.seqTab && m.pattern != nil && section != secPatterns {
		// The pattern of the Sequencer tab stays: it is read again when the tab is back.
		slot := m.pattern.Slot
		m.seqSlot = &slot
		m.pattern, m.patRange, m.patFocus, m.patSel = nil, nil, false, nil
	}
	if m.section == secSlicer && section != secSlicer && m.slicer != nil && m.slicer.playing {
		// The slicer stays open under its tab, but silent.
		m.slicer.playing = false
		m.stopListening()
	}
	if section == secSequencer && m.seqSlot != nil {
		slot := *m.seqSlot
		return m.openSequencerPattern(slot)
	}
	m.section, m.seqTab = section, false
	cmd := m.wanted()
	return m, cmd
}

func (m *Model) stopListening() {
	// A file still being decoded must not start once this has been asked for.
	m.listenGen.Add(1)
	switch {
	case m.inLoopAssign():
		// Choosing a sample for a pad of the pattern that loops: the loop goes on, the next file
		// is swapped into it when it is chosen.
		return
	case m.loopTry != nil:
		// The assign dialog closed without keeping the file: the pad's own sound comes back and
		// the loop goes on.
		m.loopTry, m.loopDirty = nil, true
		return
	}
	if m.player != nil {
		m.player.Stop()
	}
	m.listening = ""
}

// looping reports whether a pattern loops on this computer now.
func (m Model) looping() bool {
	return m.loopSlot != nil && m.player != nil && m.player.Playing() && m.listening == "pattern "+m.loopSlot.String()
}

// assignPad is the pad a file is being chosen for: the assign dialog's, or that of a search
// opened from it.
func (m Model) assignPad() (proto.PadIndex, bool) {
	switch {
	case m.search != nil && m.search.forPad:
		return m.search.pad, true
	case m.browser != nil && m.browser.kind == browseAssign:
		return m.browser.pad, true
	}
	return proto.PadIndex{}, false
}

// inLoopAssign reports whether a file is being chosen for a pad while a pattern loops: then the
// files listened to are tried in the loop, on that pad, instead of played on their own.
func (m Model) inLoopAssign() bool {
	_, ok := m.assignPad()
	return ok && m.looping()
}

// tryInLoop puts a file in place of the dialog pad's sample in the looping pattern, so it is
// heard in the groove before it is assigned; nothing is written to the unit.
func (m Model) tryInLoop(path string) (tea.Model, tea.Cmd) {
	m.listenGen.Add(1)
	pad, _ := m.assignPad()
	m.loopTry, m.loopDirty = &loopTry{pad: pad, path: path}, true
	m.message = fmt.Sprintf("%s on %s in the playing pattern: Enter assigns it, Esc puts the pad's own sound back", app.Base(path), pad)
	return m, nil
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
	case "N":
		return m.openTemplatePicker()
	case "F":
		return m.scanOrigins()
	case "W":
		return m.scanFolderUses()
	case "B":
		return m.askBusScan(false)
	case "K":
		return m.checkSamples()
	case "H":
		return m.askRepairPads()
	case "m":
		return m.toggleMute(row.Pad)
	case "M":
		return m.toggleSolo(row.Pad)
	case "t":
		// Scope the pad-settings dialog to the highlighted pad.
		p := row.Pad
		m.toggles = &toggleMenu{pad: &p}
	case "T":
		// Scope the pad-settings dialog to every pad of the bank.
		m.toggles = &toggleMenu{}
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
		// Again, with every pad already marked, it unmarks them (Esc does too).
		all := true
		for i, r := range m.snap.Bank(m.bank) {
			all = all && (!r.HasSample || m.marked[i])
		}
		if all && len(m.marked) > 0 {
			m.marked = map[int]bool{}
			m.message = "unmarked every pad"
			break
		}
		for i, r := range m.snap.Bank(m.bank) {
			if r.HasSample {
				m.marked[i] = true
			}
		}
		m.message = fmt.Sprintf("marked %s: V again or Esc unmarks them", plural(len(m.marked), "pad"))
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
	case "Z":
		return m.openSlicerPad(row)
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
			if name == "s" {
				// With nothing picked up to swap, s solos the pad.
				return m.toggleSolo(row.Pad)
			}
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
		d := m.pattern
		rows := d.Grid.Rows
		if m.selecting() {
			if next, cmd, ok := m.selectionKey(name); ok {
				return next, cmd
			}
		}
		switch name {
		case "V":
			return m.startSelecting()
		case "D":
			return m.clearTrackInView()
		case "b":
			if !d.HasSourceBank {
				m.message = "its pads span more than one bank; retargeting needs one bank"
				break
			}
			// Default to the next bank over, so a single Enter does something useful.
			m.picker = &picker{retarget: true, slot: d.Slot, sourceBank: d.SourceBank, bank: (d.SourceBank + 1) % proto.Banks}
		case "B":
			return m.patternBounce()
		case "c", "C":
			return m.changeCondition(name == "c")
		case "y":
			return m.rollAgain()
		case "r":
			m.busy = "reading the pattern"
			return m, m.openPattern(m.pattern.Slot, "refreshed")
		case "P":
			return m.playPattern(m.pattern.Slot)
		case "h", "left":
			lo, hi := m.colBounds(d)
			m.patCol = clamp(m.patCol-1, lo, hi-1)
		case "l", "right":
			lo, hi := m.colBounds(d)
			m.patCol = clamp(m.patCol+1, lo, hi-1)
		case "v":
			return m.openRangePicker()
		case "j", "down":
			m.patRow = clamp(m.patRow+1, 0, max(len(rows)-1, 0))
		case "k", "up":
			m.patRow = clamp(m.patRow-1, 0, max(len(rows)-1, 0))
		case "g":
			// Start of the current bar.
			m.patCol = m.patPage() * d.StepsPerPage()
		case "G":
			// End of the current bar (or the pattern, whichever comes first).
			m.patCol = min(m.patPage()*d.StepsPerPage()+d.StepsPerPage()-1, d.Grid.Steps-1)
		case "{", "[":
			if _, ok := m.rangeOf(d); ok {
				return m.slideRange(-1)
			}
			// The page before (the bars shown at once), keeping the step's place in it.
			m.patCol = clamp(m.patCol-m.pageSteps(d), 0, d.Grid.Steps-1)
		case "}", "]":
			if _, ok := m.rangeOf(d); ok {
				return m.slideRange(+1)
			}
			// The next page.
			m.patCol = clamp(m.patCol+m.pageSteps(d), 0, d.Grid.Steps-1)
		case "enter":
			// The first Enter expands the track into its step lane; inside it, Enter sets notes.
			if !m.patFocus {
				if len(rows) > 0 {
					m.patFocus = true
				}
				break
			}
			return m.toggleNote()
		case "x":
			return m.toggleNote()
		case "H", "L", "(", ")":
			// Nudge the note off (or back toward) the grid: H L by 5 ticks, ( ) by 1.
			by := map[string]int{"H": -5, "L": 5, "(": -1, ")": 1}[name]
			return m.nudgeNote(by, false)
		case "0":
			return m.resetNote()
		case " ":
			return m.playPattern(m.pattern.Slot)
		case "u":
			return m.undoPatternEdit()
		case "R":
			return m.redoPatternEdit()
		case "t":
			// Scope the tools to the pad the cursor is on.
			if pad, _, ok := m.cellOfPattern(); ok {
				p := pad
				m.patTool = newPatTool(&p)
				m.message = ""
			}
		case "T":
			// Scope the tools to the whole pattern (every track together).
			m.patTool = newPatTool(nil)
			m.message = ""
		case ",", ".", "<", ">", "*":
			return m.changeVelocity(name)
		case "m", "M", "s":
			if pad, _, ok := m.cellOfPattern(); ok {
				if name == "m" {
					return m.toggleMute(pad)
				}
				return m.toggleSolo(pad)
			}
		case "+", "=", "-":
			return m.changeLength(name != "-")
		case "S":
			return m.chooseHeardSlot()
		}
		return m, nil
	}
	if name != "p" {
		m.patConfirm = -1
	}
	switch name {
	case "N":
		return m.openTemplatePicker()
	case "P", "s", " ":
		rows := m.patterns[m.bank]
		if rows == nil || !rows[m.patternCursor].Exists {
			if name == "s" && m.player != nil {
				m.player.Stop()
				m.listening = ""
			}
			m.message = "that pattern slot is empty"
			break
		}
		return m.playPattern(rows[m.patternCursor].Slot)
	case "v":
		if rows := m.patterns[m.bank]; rows != nil && rows[m.patternCursor].Exists {
			slot := int(rows[m.patternCursor].Slot.Index())
			if m.patMarks[slot] {
				delete(m.patMarks, slot)
			} else {
				m.patMarks[slot] = true
			}
			m.patternCursor = clamp(m.patternCursor+1, 0, proto.PadsPerBank-1)
		}
	case "u":
		m.patMarks = map[int]bool{}
		m.patHolding = false
	case "d":
		return m.deletePatterns()
	case "c", "x":
		rows := m.patterns[m.bank]
		if rows == nil || !rows[m.patternCursor].Exists {
			m.message = "nothing to pick up: that pattern slot is empty"
			break
		}
		m.patHeld, m.patHolding, m.patCut = rows[m.patternCursor].Slot, true, name == "x"
		verb := "copy"
		if m.patCut {
			verb = "move"
		}
		m.message = fmt.Sprintf("%s picked up: go to a slot (and bank, [ ]) and press p to %s it there", m.patHeld, verb)
	case "p":
		return m.putPattern()
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
	case "enter":
		rows := m.patterns[m.bank]
		if rows == nil || !rows[m.patternCursor].Exists {
			m.message = "that pattern slot is empty"
			break
		}
		// A pattern opened from the panel under the pads is the Sequencer's too.
		m.seqTab, m.seqSlot = true, nil
		m.busy = "reading the pattern"
		return m, m.openPattern(rows[m.patternCursor].Slot, "")
	}
	return m, nil
}

// playPattern plays a pattern through this computer's speakers, mixed from the samples on its
// pads at the bank's tempo, over and over; pressing it again while it sounds stops it, and s stops it.
// This is the pads as stored, not what the unit would play: the unit has no command in this
// protocol to start its own sequencer.
func (m Model) playPattern(slot proto.PadIndex) (tea.Model, tea.Cmd) {
	if m.player == nil {
		m.message = "no audio output to play on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	label := "pattern " + slot.String()
	if m.listening == label && m.player.Playing() {
		m.player.Stop()
		m.listening = ""
		m.message = label + " stopped"
		return m, nil
	}
	return m.startLoop(slot)
}

// startLoop mixes a pattern (only its range, when one is set) and loops it from the top,
// whatever played before: a new range is a loop of another length.
func (m Model) startLoop(slot proto.PadIndex) (tea.Model, tea.Cmd) {
	label := "pattern " + slot.String()
	m.player.Stop()
	id := m.listenGen.Add(1)
	m.listening = label
	m.loopSlot = &slot
	m.message = "mixing " + label + "…"
	player, gen, mix := m.player, m.listenGen, m.patternMix(slot)
	return m, func() tea.Msg {
		sample, loops, err := mix()
		if err != nil {
			return listenMsg{path: label, err: err}
		}
		if gen.Load() != id {
			return listenMsg{path: label, stale: true}
		}
		// A pattern repeats until it is stopped, so that it can be listened to while the
		// samples of its pads are changed; a player that cannot loop plays it once.
		round := time.Duration(sample.Frames()) * time.Second / formats.SampleRate
		if looper, ok := player.(app.Looper); ok {
			return listenMsg{path: label, err: looper.Loop(sample), round: round, loops: loops}
		}
		return listenMsg{path: label, err: player.Play(sample)}
	}
}

// patternMixer reads a pattern from the unit and mixes it, at the tempo of its bank and with
// each pad at its level, as things are now: the pads are copied, so the mix is not changed by
// what happens on the screen while it is made.
func (m Model) patternMixer(slot proto.PadIndex) func() (formats.Sample, error) {
	mix := m.patternMix(slot)
	return func() (formats.Sample, error) {
		sample, _, err := mix()
		return sample, err
	}
}

// condRolls is how many loops a pattern with chances in it is rolled over at least.
const condRolls = 4

// condKey names a pattern for its conditions: the source, the project and the slot.
func (m Model) condKey(slot proto.PadIndex) string {
	source, project := "unit", 0
	if m.offline != "" {
		source = m.offline
	}
	if m.snap != nil {
		project = int(m.snap.Project)
	}
	return fmt.Sprintf("%s|%d|%s", source, project, slot)
}

// patternConditions are what is kept for the pattern in a slot: its conditions and the seed.
func (m Model) patternConditions(slot proto.PadIndex) (app.Conditions, int64) {
	if m.conds == nil {
		return nil, 0
	}
	p := m.conds.load(m.condKey(slot))
	return p.conditions(), p.Seed
}

// patternMix is patternMixer that also says how many loops of the pattern the mix has: one,
// unless conditions make it a cycle (and then each note is where its condition puts it).
func (m Model) patternMix(slot proto.PadIndex) func() (formats.Sample, int, error) {
	// With a range set on this pattern, only its bars are mixed: the loop is that long.
	var rng *barRange
	if m.pattern != nil && m.pattern.Slot == slot {
		if r, ok := m.rangeOf(m.pattern); ok {
			rng = &r
		}
	}
	return m.patternMixIn(slot, rng)
}

// onlyHeard is the conditions of the pads that are heard in the mix: those of a muted track, or of
// a track that is not soloed, change nothing in the sound, so they do not make the cycle longer.
func onlyHeard(cs app.Conditions, muted, soloed map[proto.PadIndex]bool) app.Conditions {
	out := app.Conditions{}
	for k, c := range cs {
		if heard(k.Pad, muted, soloed) {
			out[k] = c
		}
	}
	return out
}

// patternMixIn is patternMix over the bars of rng, the whole pattern when it is nil.
func (m Model) patternMixIn(slot proto.PadIndex, rng *barRange) func() (formats.Sample, int, error) {
	bpm := m.snap.Banks[slot.Bank()].Tempo
	if m.snap.UsesProjectTempo {
		bpm = m.snap.ProjectTempo
	}
	unit, ops, cache := m.unit, m.ops, m.padAudio
	muted, soloed := maps.Clone(m.muted), maps.Clone(m.soloed)
	// The pads are copied with their settings: the screen changes those while the mix is made.
	pads := slices.Clone(m.snap.Pads)
	for i := range pads {
		pads[i].Params = maps.Clone(pads[i].Params)
	}
	var try loopTry
	if m.loopTry != nil {
		try = *m.loopTry
	}
	// Conditions are read now, with the pads: the screen changes them while the mix is made.
	conds, seed := m.patternConditions(slot)
	return func() (formats.Sample, int, error) {
		ops.Lock()
		defer ops.Unlock()
		current, err := unit.CurrentProject()
		if err != nil {
			return formats.Sample{}, 0, err
		}
		data, err := unit.ReadFile(formats.PatternPath(current+1, slot))
		if err != nil {
			return formats.Sample{}, 0, err
		}
		pattern, err := formats.ParsePattern(data)
		if err != nil {
			return formats.Sample{}, 0, err
		}
		if rng != nil {
			bar := uint32(pattern.BeatsPerBar()) * formats.PPQ
			pattern = pattern.Slice(uint32(rng.first)*bar, min(uint32(rng.last+1)*bar, pattern.Length))
		}
		// Conditions make it a cycle of loops, each note on the loops its condition says.
		loops := 1
		if live := onlyHeard(conds.Pruned(pattern), muted, soloed); len(live) > 0 {
			loops = live.Loops(condRolls)
			pattern = app.Unroll(pattern, live, loops, seed)
		}
		// A file tried on a pad plays in its place, the whole file, at the pad's settings.
		var tried *formats.Sample
		if try.path != "" {
			imported, err := app.LoadSample(unit, try.path)
			if err != nil {
				return formats.Sample{}, 0, err
			}
			tried = &imported.Sample
		}
		sample, err := app.MixPatternVoices(pattern, bpm, func(pad proto.PadIndex) (app.Voice, error) {
			row := pads[pad.Index()]
			if tried != nil && pad == try.pad && heard(pad, muted, soloed) {
				return app.PadVoice(*tried, row.Level, row.Params), nil
			}
			if !row.HasSample || !heard(pad, muted, soloed) {
				return app.Voice{Sample: formats.Sample{Channels: 2}}, nil
			}
			key := factKey{row.Pad, row.Size, row.Name}
			sample, ok := cache.get(key)
			if !ok {
				if sample, err = app.PadAudio(unit, row); err != nil {
					return app.Voice{}, err
				}
				cache.put(key, sample)
			}
			// The pad as its settings say: level, pan, pitch, direction, gate and loop.
			return app.PadVoice(sample, row.Level, row.Params), nil
		})
		return sample, loops, err
	}
}

// loopQueuedMsg ends the mixing of a pattern again after a change while it loops.
type loopQueuedMsg struct {
	err   error
	loops int
	round time.Duration
}

// requeueLoop mixes the looping pattern again, so that a change is heard at once:
// a note on or off, a velocity, a pad's level. It does nothing when no pattern loops.
func (m Model) requeueLoop() tea.Cmd {
	if m.loopSlot == nil || m.player == nil || !m.player.Playing() || m.listening != "pattern "+m.loopSlot.String() || m.snap == nil {
		return nil
	}
	player, mix := m.player, m.patternMix(*m.loopSlot)
	return func() tea.Msg {
		sample, loops, err := mix()
		if err != nil {
			return loopQueuedMsg{err: err}
		}
		round := time.Duration(sample.Frames()) * time.Second / formats.SampleRate
		// At once, from the place the loop has reached; else from its next round.
		if replacer, ok := player.(app.LoopReplacer); ok {
			return loopQueuedMsg{err: replacer.ReplaceLoop(sample), loops: loops, round: round}
		}
		if swapper, ok := player.(app.LoopSwapper); ok {
			return loopQueuedMsg{err: swapper.NextLoop(sample), loops: loops, round: round}
		}
		if looper, ok := player.(app.Looper); ok {
			return loopQueuedMsg{err: looper.Loop(sample), loops: loops, round: round}
		}
		return loopQueuedMsg{}
	}
}

// deletePatterns asks to delete the marked patterns, or the one in front.
func (m Model) deletePatterns() (tea.Model, tea.Cmd) {
	rows := m.patterns[m.bank]
	if rows == nil {
		return m, nil
	}
	var slots []proto.PadIndex
	for _, r := range m.patterns[m.bank] {
		if m.patMarks[int(r.Slot.Index())] && r.Exists {
			slots = append(slots, r.Slot)
		}
	}
	if len(slots) == 0 {
		if !rows[m.patternCursor].Exists {
			m.message = "that pattern slot is empty"
			return m, nil
		}
		slots = []proto.PadIndex{rows[m.patternCursor].Slot}
	}
	m.prompt = &prompt{kind: promptDeletePatterns, pads: slots}
	return m, nil
}

// putPattern drops the picked-up pattern into the slot in front: a copy, or a move. A slot
// that holds a pattern is replaced only on a second p.
func (m Model) putPattern() (tea.Model, tea.Cmd) {
	rows := m.patterns[m.bank]
	if !m.patHolding || rows == nil {
		m.message = "nothing picked up: c copies a pattern, x moves it"
		return m, nil
	}
	from, to := m.patHeld, rows[m.patternCursor].Slot
	if from == to {
		m.message = "that is the slot itself"
		return m, nil
	}
	if rows[m.patternCursor].Exists && m.patConfirm != int(to.Index()) {
		m.patConfirm = int(to.Index())
		m.message = fmt.Sprintf("%s holds a pattern: p again replaces it", to)
		return m, nil
	}
	cut, unit := m.patCut, m.unit
	m.patHolding, m.patConfirm = false, -1
	m.busy = "copying the pattern"
	if cut {
		m.busy = "moving the pattern"
	}
	return m, m.edit(func() tea.Msg {
		if err := app.CopyPattern(unit, from, to, cut); err != nil {
			return doneMsg{message: err.Error(), patterns: true, reload: true}
		}
		verb := "copied to"
		if cut {
			verb = "moved to"
		}
		return doneMsg{message: fmt.Sprintf("pattern %s %s %s", from, verb, to), patterns: true, reload: true}
	})
}

// bankToggle is a pad setting that is on or off, and that t switches for a whole bank.
// A setting kept in one bit of the mode flags has that bit as mask.
type bankToggle struct {
	name, label string
	mask        int32
	key         string
	// values is how many values a setting with more than two has (bus FX: off, BUS 1,
	// BUS 2); 0 for an on/off setting.
	values int32
}

var bankToggles = []bankToggle{
	{"gate", "Gate", 0, "g", 0}, {"loop", "Loop", 0, "l", 0}, {"bpm-sync", "BPM sync", 0, "b", 0}, {"vinyl", "Vinyl", 0, "v", 0},
	{"mode-flags", "One shot", 1 << 5, "o", 0}, {"mode-flags", "Fixed velocity", 1 << 0, "f", 0},
	{"bus-fx", "Bus FX", 0, "x", 3},
}

// isOn says whether the setting is on in a pad's parameters.
func (t bankToggle) isOn(params map[string]int32) bool {
	if t.mask != 0 {
		return params[t.name]&t.mask != 0
	}
	return params[t.name] != 0
}

// with is the parameter's value with the setting turned on or off.
func (t bankToggle) with(current int32, on bool) int32 {
	switch {
	case t.mask == 0 && on:
		return 1
	case t.mask == 0:
		return 0
	case on:
		return current | t.mask
	}
	return current &^ t.mask
}

// toggleMenu is the pad-settings dialog. pad, when set, scopes it to that one pad; otherwise it
// changes every pad with a sample in the bank.
type toggleMenu struct {
	cursor int
	pad    *proto.PadIndex
}

// toggleTargets are the pads the menu changes: one pad when scoped to it, else every pad with a
// sample in the bank on show.
func (m Model) toggleTargets() []proto.PadIndex {
	if t := m.toggles; t != nil && t.pad != nil {
		if m.snap.Pads[t.pad.Index()].HasSample {
			return []proto.PadIndex{*t.pad}
		}
		return nil
	}
	var pads []proto.PadIndex
	for _, pad := range proto.AllPads()[m.bank*proto.PadsPerBank : (m.bank+1)*proto.PadsPerBank] {
		if m.snap.Pads[pad.Index()].HasSample {
			pads = append(pads, pad)
		}
	}
	return pads
}

// toggleState counts the target pads that have the setting on.
func (m Model) toggleState(t bankToggle) (on, total int) {
	for _, pad := range m.toggleTargets() {
		total++
		if t.isOn(m.snap.Pads[pad.Index()].Params) {
			on++
		}
	}
	return on, total
}

// toggleValue is the value every target pad has for a setting with several values, and
// whether they all have it.
func (m Model) toggleValue(t bankToggle) (value int32, same bool) {
	pads := m.toggleTargets()
	for i, pad := range pads {
		v := m.snap.Pads[pad.Index()].Params[t.name]
		if i == 0 {
			value = v
		} else if v != value {
			return value, false
		}
	}
	return value, len(pads) > 0
}

func (m Model) togglesKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.toggles
	bus := bankToggles[len(bankToggles)-1]
	switch name := key.String(); name {
	case "esc", "q", "t":
		m.toggles = nil
	case "up", "k", "ctrl+p":
		t.cursor = max(t.cursor-1, 0)
	case "down", "j", "ctrl+n":
		t.cursor = min(t.cursor+1, len(bankToggles)-1)
	case "enter", " ":
		return m.toggleTargetsBy(bankToggles[t.cursor], -1)
	case "0", "1", "2":
		// Digits set the bus directly: 0 off, 1 BUS 1, 2 BUS 2.
		return m.toggleTargetsBy(bus, int32(name[0]-'0'))
	default:
		// A letter picks a setting: g, l, b, v, o, f, x.
		for _, b := range bankToggles {
			if name == b.key {
				return m.toggleTargetsBy(b, -1)
			}
		}
	}
	return m, nil
}

func (m Model) togglesBox() string {
	st := m.st
	scope := fmt.Sprintf("every pad of bank %c", 'A'+m.bank)
	if t := m.toggles; t != nil && t.pad != nil {
		scope = "pad " + t.pad.String()
	}
	lines := []string{st.bold.Render("Set for " + scope), ""}
	for i, t := range bankToggles {
		on, total := m.toggleState(t)
		state := fmt.Sprintf("off (0 of %d)", total)
		switch {
		case total == 0:
			state = "no samples"
		case t.values > 0:
			if v, same := m.toggleValue(t); same {
				state = fmt.Sprintf("%s (all %d)", busName(v), total)
			} else {
				state = "mixed"
			}
		case on == total:
			state = fmt.Sprintf("on (all %d)", total)
		case on > 0:
			state = fmt.Sprintf("mixed (%d of %d on)", on, total)
		}
		line := fmt.Sprintf("%-15s %s", t.label, state)
		if i == m.toggles.cursor {
			line = lipgloss.NewStyle().Background(st.th.cursorBg).Foreground(st.th.cursorFg).Bold(true).Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "",
		st.dim.Render("Enter switches it: on for all, or off when all are on  ·  g l b v o f x pick"),
		st.dim.Render("Bus FX: Enter steps through off, BUS 1, BUS 2  ·  0 1 2 set it  ·  Esc closes"),
		st.dim.Render("One shot can clear gate and loop on the pads it is turned on for."))
	return st.modal.Width(min(m.width-4, 80)).Render(strings.Join(lines, "\n"))
}

// toggleTargetsBy changes a setting on the target pads. An on/off setting goes on for all of
// them, or off when all are on already. A setting with several values (bus FX) takes value,
// or with -1 goes to the next one.
func (m Model) toggleTargetsBy(t bankToggle, value int32) (tea.Model, tea.Cmd) {
	pads := m.toggleTargets()
	padScope := m.toggles != nil && m.toggles.pad != nil
	if len(pads) == 0 {
		m.toggles = nil
		m.message = "no pads with a sample to change"
		return m, nil
	}
	info, ok := proto.ParamByName(t.name)
	if !ok {
		return m, nil
	}
	var turnOn bool
	var what string
	if t.values > 0 {
		if value < 0 {
			cur, same := m.toggleValue(t)
			value = 1
			if same {
				value = (cur + 1) % t.values
			}
		}
		what = busName(value)
	} else {
		on, total := m.toggleState(t)
		turnOn = on != total
		what = onOffText(map[bool]int32{true: 1, false: 0}[turnOn])
	}
	values := map[proto.PadIndex]int32{}
	for _, pad := range pads {
		row := &m.snap.Pads[pad.Index()]
		if row.Params == nil {
			row.Params = map[string]int32{}
		}
		if t.values > 0 {
			row.Params[t.name] = value
		} else {
			row.Params[t.name] = t.with(row.Params[t.name], turnOn)
		}
		values[pad] = row.Params[t.name]
	}
	unit, id := m.unit, info.ID
	scope := fmt.Sprintf("in bank %c", 'A'+m.bank)
	if padScope && len(pads) == 1 {
		scope = "on " + pads[0].String()
	}
	m.toggles = nil
	m.marked = map[int]bool{}
	prog := m.prog
	prog.begin(len(pads))
	m.busy = "setting " + strings.ToLower(t.label)
	return m, m.edit(func() tea.Msg {
		defer prog.begin(0)
		for i, pad := range pads {
			line := t.label + " " + what + " on " + pad.String()
			prog.step(i, line)
			prog.logLine(line)
			if err := unit.SetParam(id, proto.TargetPad(pad), values[pad]); err != nil {
				return doneMsg{message: fmt.Sprintf("%s on %s: %v", strings.ToLower(t.label), pad, err), reload: true}
			}
		}
		return doneMsg{message: fmt.Sprintf("%s %s on %s %s", strings.ToLower(t.label), what, plural(len(pads), "pad"), scope), reload: true}
	})
}

// ---- New pattern: templates ----

// templatePicker is the list N opens: pick the shape of the new pattern. The first two entries
// are special (all pads held, and a blank pattern); the rest are the genre grooves from
// formats.GrooveTemplates, in order.
type templatePicker struct {
	cursor int
}

// templateItems are the picker's rows: the two built-ins first, then one per genre groove.
func templateItems() []string {
	items := []string{"All pads held", "Blank"}
	for _, g := range formats.GrooveTemplates() {
		items = append(items, fmt.Sprintf("%-16s %s  ·  %s", g.Name, plural(g.Bars(), "bar"), g.Tempo))
	}
	return items
}

// grooveAt is the genre groove a picker row stands for, and whether the row is a groove at all
// (the first two rows are the built-ins and return ok false).
func grooveAt(row int) (formats.GrooveTemplate, bool) {
	grooves := formats.GrooveTemplates()
	i := row - 2
	if i < 0 || i >= len(grooves) {
		return formats.GrooveTemplate{}, false
	}
	return grooves[i], true
}

// openTemplatePicker opens the new-pattern template list, when the bank has a free slot and, for
// the genre grooves, some samples to play.
func (m Model) openTemplatePicker() (tea.Model, tea.Cmd) {
	m.tmplPicker = &templatePicker{}
	return m, nil
}

func (m Model) templatePickerKey(name string) (tea.Model, tea.Cmd) {
	p := m.tmplPicker
	items := templateItems()
	switch name {
	case "esc", "q", "N":
		m.tmplPicker = nil
	case "up", "k", "ctrl+p":
		p.cursor = max(p.cursor-1, 0)
	case "down", "j", "ctrl+n":
		p.cursor = min(p.cursor+1, len(items)-1)
	case "g":
		p.cursor = 0
	case "G":
		p.cursor = len(items) - 1
	case "enter", " ":
		return m.chooseTemplate(p.cursor)
	}
	return m, nil
}

// chooseTemplate acts on the picked row: "All pads held" and "Blank" write at once; a genre
// groove opens the lane confirm dialog.
func (m Model) chooseTemplate(row int) (tea.Model, tea.Cmd) {
	m.tmplPicker = nil
	bars := app.StartBars
	if g, ok := grooveAt(row); ok {
		bars = g.Bars()
	} else if row > 1 {
		return m, nil
	}
	return m.askPatternLength(row, bars)
}

// buildPattern makes what the template row stands for, bars long.
func (m Model) buildPattern(row, bars int) (tea.Model, tea.Cmd) {
	switch row {
	case 0:
		return m.newStartPattern(bars)
	case 1:
		return m.newBlankPattern(bars)
	}
	g, ok := grooveAt(row)
	if !ok {
		return m, nil
	}
	next, cmd := m.openLaneAssign(g)
	if nm, ok := next.(Model); ok && nm.laneMap != nil {
		nm.laneMap.bars = bars
		return nm, cmd
	}
	return next, cmd
}

// bankSampledPads are the pads of the bank on show that hold a sample, in pad order.
func (m Model) bankSampledPads() []proto.PadIndex {
	var pads []proto.PadIndex
	for _, pad := range proto.AllPads()[m.bank*proto.PadsPerBank : (m.bank+1)*proto.PadsPerBank] {
		if m.snap.Pads[pad.Index()].HasSample {
			pads = append(pads, pad)
		}
	}
	return pads
}

// newBlankPattern writes an empty pattern into the first free slot of the bank on show, to be
// programmed by hand.
func (m Model) newBlankPattern(bars int) (tea.Model, tea.Cmd) {
	unit, bank := m.unit, m.bank
	m.busy = "making a pattern"
	return m, m.edit(func() tea.Msg {
		slot, err := firstFreePatternSlot(unit, bank)
		if err != nil {
			return doneMsg{message: err.Error()}
		}
		built, err := formats.BlankPattern(bars)
		if err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		if err := app.WriteGroove(unit, slot, built); err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		return doneMsg{
			message:  fmt.Sprintf("pattern %s made: blank, %d bars", slot, bars),
			reload:   true,
			patterns: true,
		}
	})
}

// firstFreePatternSlot is the first empty pattern slot of a bank, or an error naming why there
// is none (reading failed, or every slot is taken).
func firstFreePatternSlot(unit app.Unit, bank int) (proto.PadIndex, error) {
	rows, err := app.LoadPatterns(unit, bank)
	if err != nil {
		return proto.PadIndex{}, fmt.Errorf("reading the patterns failed: %s", err)
	}
	free := slices.IndexFunc(rows, func(r app.PatternRow) bool { return !r.Exists })
	if free < 0 {
		return proto.PadIndex{}, fmt.Errorf("every pattern slot of bank %c is taken", 'A'+bank)
	}
	return rows[free].Slot, nil
}

// laneAssign is the dialog that confirms which pad plays each lane of a genre groove before it is
// written. cursor is the lane row in front; lane maps each of the template's lanes to the pad
// chosen for it (nil = skip the lane); pads are the bank's sampled pads the h/l keys cycle
// through, with a "skip" past the end. When choosing is true the dialog is in its pad chooser:
// chooseAt is the row of the chooser (0..len(pads) is skip) picking the pad for the lane cursor
// is on.
type laneAssign struct {
	tmpl     formats.GrooveTemplate
	cursor   int
	lane     map[formats.Lane]*proto.PadIndex
	pads     []proto.PadIndex
	choosing bool
	chooseAt int
	// bars is how long the pattern is (0: the groove's own phrase).
	bars int
}

// openLaneAssign opens the lane confirm dialog for a genre groove, pre-filled by guessing the
// roles from the bank's sample names.
func (m Model) openLaneAssign(g formats.GrooveTemplate) (tea.Model, tea.Cmd) {
	pads := m.bankSampledPads()
	if len(pads) == 0 {
		m.message = fmt.Sprintf("bank %c has no samples to put in a pattern", 'A'+m.bank)
		return m, nil
	}
	// Guess roles over just this bank's rows.
	var rows []app.PadRow
	for _, pad := range pads {
		rows = append(rows, m.snap.Pads[pad.Index()])
	}
	guess := app.GuessRoles(rows)
	lane := map[formats.Lane]*proto.PadIndex{}
	for _, l := range g.Lanes {
		if p, ok := guess[l]; ok {
			pp := p
			lane[l] = &pp
		} else {
			lane[l] = nil
		}
	}
	m.laneMap = &laneAssign{tmpl: g, lane: lane, pads: pads}
	return m, nil
}

func (m Model) laneAssignKey(name string) (tea.Model, tea.Cmd) {
	a := m.laneMap
	lanes := a.tmpl.Lanes
	if a.choosing {
		return m.laneChooseKey(name)
	}
	switch name {
	case "esc", "q":
		m.laneMap = nil
	case "up", "k", "ctrl+p":
		a.cursor = max(a.cursor-1, 0)
	case "down", "j", "ctrl+n":
		a.cursor = min(a.cursor+1, len(lanes)-1)
	case "left", "h":
		a.cycleLane(lanes[a.cursor], -1)
	case "right", "l", "tab":
		a.cycleLane(lanes[a.cursor], +1)
	case "enter", " ":
		// Open the pad chooser for the lane in front, starting on its current choice.
		a.choosing = true
		a.chooseAt = a.choiceIndex(lanes[a.cursor])
	case "w":
		return m.writeGroovePattern()
	}
	return m, nil
}

// laneChooseKey handles keys while the lane dialog is in its pad chooser: a list of the bank's
// sampled pads with "skip" at the top. Enter sets the lane to the picked pad and comes back to
// the lane list; Esc comes back without changing it.
func (m Model) laneChooseKey(name string) (tea.Model, tea.Cmd) {
	a := m.laneMap
	lane := a.tmpl.Lanes[a.cursor]
	last := len(a.pads) // the chooser has len(pads)+1 rows: skip, then each pad
	switch name {
	case "esc", "q":
		a.choosing = false
	case "up", "k", "ctrl+p":
		a.chooseAt = max(a.chooseAt-1, 0)
	case "down", "j", "ctrl+n":
		a.chooseAt = min(a.chooseAt+1, last)
	case "g":
		a.chooseAt = 0
	case "G":
		a.chooseAt = last
	case "enter", " ":
		if a.chooseAt == last {
			a.lane[lane] = nil
		} else {
			pp := a.pads[a.chooseAt]
			a.lane[lane] = &pp
		}
		a.choosing = false
	}
	return m, nil
}

// choiceIndex is the chooser row the lane's current pad sits on: len(pads) (the skip row) when
// the lane is skipped, else the pad's position among the bank's sampled pads.
func (a *laneAssign) choiceIndex(lane formats.Lane) int {
	if p := a.lane[lane]; p != nil {
		for i, pad := range a.pads {
			if pad == *p {
				return i
			}
		}
	}
	return len(a.pads)
}

// cycleLane steps the pad a lane plays through the bank's sampled pads and a "skip" past the end:
// the choices are pads[0..n-1] then nil (skip), wrapping. dir is +1 or -1.
func (a *laneAssign) cycleLane(lane formats.Lane, dir int) {
	// Current index in [0, len(pads)]: len(pads) means skip (nil).
	cur := len(a.pads)
	if p := a.lane[lane]; p != nil {
		for i, pad := range a.pads {
			if pad == *p {
				cur = i
				break
			}
		}
	}
	n := len(a.pads) + 1
	next := ((cur+dir)%n + n) % n
	if next == len(a.pads) {
		a.lane[lane] = nil
		return
	}
	pp := a.pads[next]
	a.lane[lane] = &pp
}

// writeGroovePattern builds the confirmed groove and writes it into the first free slot of the
// bank on show.
func (m Model) writeGroovePattern() (tea.Model, tea.Cmd) {
	a := m.laneMap
	pads := map[formats.Lane]proto.PadIndex{}
	for lane, p := range a.lane {
		if p != nil {
			pads[lane] = *p
		}
	}
	if len(pads) == 0 {
		m.message = "assign at least one lane to a pad first"
		return m, nil
	}
	g, bars := a.tmpl, a.bars
	if bars == 0 {
		bars = g.Bars()
	}
	m.laneMap = nil
	unit, bank := m.unit, m.bank
	m.busy = "making a pattern"
	return m, m.edit(func() tea.Msg {
		slot, err := firstFreePatternSlot(unit, bank)
		if err != nil {
			return doneMsg{message: err.Error()}
		}
		// The phrase plays over and over to the length asked for.
		built, err := g.BuildGroove(pads, bars)
		if err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		if err := app.WriteGroove(unit, slot, built); err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		return doneMsg{
			message:  fmt.Sprintf("pattern %s made: %s, %d lanes, %d bars", slot, g.Name, len(pads), bars),
			reload:   true,
			patterns: true,
		}
	})
}

// newStartPattern makes a pattern in the first free slot of the bank on show: every pad of
// the bank that holds a sample starts on beat 1 and is held for app.StartBars bars.
func (m Model) newStartPattern(bars int) (tea.Model, tea.Cmd) {
	var pads []proto.PadIndex
	for _, pad := range proto.AllPads()[m.bank*proto.PadsPerBank : (m.bank+1)*proto.PadsPerBank] {
		if m.snap.Pads[pad.Index()].HasSample {
			pads = append(pads, pad)
		}
	}
	if len(pads) == 0 {
		m.message = fmt.Sprintf("bank %c has no samples to put in a pattern", 'A'+m.bank)
		return m, nil
	}
	unit, bank := m.unit, m.bank
	m.busy = "making a pattern"
	return m, m.edit(func() tea.Msg {
		rows, err := app.LoadPatterns(unit, bank)
		if err != nil {
			return doneMsg{message: "reading the patterns failed: " + err.Error()}
		}
		free := slices.IndexFunc(rows, func(r app.PatternRow) bool { return !r.Exists })
		if free < 0 {
			return doneMsg{message: fmt.Sprintf("every pattern slot of bank %c is taken", 'A'+bank)}
		}
		slot := rows[free].Slot
		if err := app.WriteStartPattern(unit, slot, pads, bars); err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		return doneMsg{
			message:  fmt.Sprintf("pattern %s made: %d pads on beat 1, %d bars", slot, len(pads), bars),
			reload:   true,
			patterns: true,
		}
	})
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
	case "X":
		m.prompt = &prompt{kind: promptEraseProject, project: uint8(m.projectAt)}
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
	prog := m.prog
	prog.begin(0)
	return m, m.edit(func() tea.Msg {
		defer prog.begin(0)
		plan, err := app.PlanProjectRestore(unit, project, dir, prog.stepTotal)
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
	if name == "w" {
		m.ranger = !m.ranger
		m.message = map[bool]string{true: "files: three columns", false: "files: folder tree"}[m.ranger]
		return m, m.wanted()
	}
	if m.ranger {
		if next, cmd, ok := m.rangerKey(name); ok {
			m = next
			if cmd != nil {
				return m, cmd
			}
			m.rangerTidy()
			return m.filesTail(before)
		}
	}
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
	case "Z":
		if node.Dir || !isAudio(node.Name) {
			m.message = "Z slices an audio file: put the cursor on one"
			return m, nil
		}
		return m.openSlicerFile(node.Path)
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
	case "*":
		return m.toggleFavourite(m.filesToTag(), nil)
	case "+":
		return m.askTags(m.filesToTag(), nil)
	case "i":
		return m.startTagIndex(m.tree.Folder(m.fileAt))
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
	return m.filesTail(before)
}

// filesTail is what follows a key in the files: what the row under the cursor needs is read,
// and moving on to another row plays it, if autoplay is on, and stops the old sound at once.
func (m Model) filesTail(before int) (tea.Model, tea.Cmd) {
	cmd := m.wanted()
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
	unit, snap, rule := m.unit, m.snap, m.busRule()
	m.fileMarks = map[string]bool{}
	m.busy = fmt.Sprintf("assigning %d file(s)", len(files))
	if len(files) == 1 {
		prog := m.prog
		plan := app.PlanTempos(files, false)
		prog.begin(1 + plan.Steps())
		return m, m.edit(func() tea.Msg {
			defer prog.begin(0)
			prog.step(0, app.Base(files[0]))
			name, err := app.Assign(unit, unit, start, files[0])
			if err != nil {
				return doneMsg{message: err.Error()}
			}
			if rule.OnAssign {
				_ = app.SetPadBus(unit, start, rule.BusFor(name))
			}
			message := fmt.Sprintf("%s: assigned to %s as %q, from %s", app.Base(files[0]), start, name, display(app.Dir(files[0])))
			prog.logLine(app.Base(files[0]) + " assigned to " + start.String())
			// Where it came from is kept on the card: the pad's name has no room for it.
			origins, oerr := app.RecordOrigins(unit, snap.Project, []app.OriginRecord{{Pad: start, Path: files[0], Name: name}})
			if oerr != nil {
				message += fmt.Sprintf("; its origin was not saved: %v", oerr)
			}
			if bpm, ok := plan.Files[files[0]]; ok {
				prog.step(1, fmt.Sprintf("tempo %g BPM on %s", bpm, start))
				if err := app.SetPadTempo(unit, start, bpm); err != nil {
					message += fmt.Sprintf("; its tempo was not set: %v", err)
				} else {
					message += fmt.Sprintf("; tempo %g BPM", bpm)
				}
			}
			return doneMsg{message: message, reload: true, focus: &start, assigned: true, origins: origins}
		})
	}
	taken := func(p proto.PadIndex) bool { return snap.Pads[p.Index()].HasSample }
	// Several files fill the free pads of the bank the first one goes to, and no further:
	// what does not fit is reported, not put in another bank.
	pads := slices.DeleteFunc(app.FreePads(start, len(files), taken), func(p proto.PadIndex) bool { return p.Bank() != start.Bank() })
	pads = pads[:min(len(pads), proto.PadsPerBank)]
	// Files numbered like "09_Snare.wav" go where the number says: 09 is pad 9 of the bank,
	// whatever pads came before. A file that would land on a pad that holds a sample is left
	// out (a placeholder named EMPTY quietly), not put over it.
	numbered := false
	var held []string
	orderNote := ""
	want, ok := app.NumberedPads(files, int(start.Bank()), taken)
	if m.byNumber && !ok {
		orderNote = "; not every name starts with a pad number, so the free pads were filled in order"
	}
	if m.byNumber && ok {
		numbered, pads = true, nil
		kept := files[:0:0]
		for i, f := range files {
			if taken(want[i]) {
				if !strings.Contains(strings.ToUpper(app.Base(f)), "EMPTY") {
					held = append(held, fmt.Sprintf("%s (pad %s is taken)", app.Base(f), want[i]))
				}
				continue
			}
			kept = append(kept, f)
			pads = append(pads, want[i])
		}
		files = kept
	}
	prog := m.prog
	// The bar counts the files and then the tempos read from their names: each pad's, and
	// the bank's when the files agree on one.
	plan := app.PlanTempos(files, true)
	prog.begin(len(pads) + plan.Steps())
	return m, m.edit(func() tea.Msg {
		defer prog.begin(0)
		placed := app.AssignManyReport(unit, unit, pads, files, prog.step, func(p app.Placement) {
			switch {
			case p.Blank:
				prog.logLine(app.Base(p.File) + " is empty: " + p.Pad.String() + " left empty")
			case errors.Is(p.Err, app.ErrNoFreePad):
				prog.logLine(app.Base(p.File) + " found no free pad")
			case p.Err != nil:
				prog.logLine(app.Base(p.File) + " failed: " + p.Err.Error())
			default:
				prog.logLine(app.Base(p.File) + " assigned to " + p.Pad.String())
			}
		})
		if rule.OnAssign {
			for _, p := range placed {
				if p.Err == nil && !p.Blank {
					_ = app.SetPadBus(unit, p.Pad, rule.BusFor(app.PadName(p.File)))
				}
			}
		}
		tempos := app.ApplyTempos(unit, placed, plan, func(done int, what string) {
			prog.step(len(pads)+done, what)
			prog.logLine(what)
		})
		ok, failed := 0, []string{}
		var first *proto.PadIndex
		noRoom, blank := 0, 0
		for _, p := range placed {
			if p.Blank {
				blank++
				continue
			}
			if p.Err != nil {
				if errors.Is(p.Err, app.ErrNoFreePad) {
					noRoom++
					continue
				}
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
		if numbered {
			message = fmt.Sprintf("%d files assigned to the pads their numbers name", ok)
			failed = append(failed, held...)
		}
		if blank > 0 {
			message += fmt.Sprintf("; %s left empty", plural(blank, "empty placeholder"))
		}
		message += orderNote
		if tempos.Pads > 0 {
			message += fmt.Sprintf("; %s given a tempo", plural(tempos.Pads, "pad"))
		}
		if tempos.BankSet {
			message += fmt.Sprintf("; bank %c set to %g BPM", 'A'+int(start.Bank()), plan.Bank)
		}
		failed = append(failed, tempos.Failures...)
		if noRoom > 0 {
			message += fmt.Sprintf("; %d did not fit: bank %s is full", noRoom, start.String()[:1])
		}
		if len(failed) > 0 {
			message += "; failed: " + strings.Join(failed, "; ")
		}
		var records []app.OriginRecord
		for _, p := range placed {
			if p.Err == nil && !p.Blank {
				records = append(records, app.OriginRecord{Pad: p.Pad, Path: p.File, Name: app.PadName(p.File)})
			}
		}
		var origins *app.Origins
		if len(records) > 0 {
			var oerr error
			if origins, oerr = app.RecordOrigins(unit, snap.Project, records); oerr != nil {
				message += fmt.Sprintf("; the origins were not saved: %v", oerr)
			}
		}
		return doneMsg{message: message, reload: true, focus: first, assigned: first != nil, origins: origins}
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
	if m.inLoopAssign() {
		// A pattern loops: the file is heard on the pad, in the pattern, not on its own.
		return m.tryInLoop(path)
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
	case name == "?" && p.filter == "":
		m.palette = nil
	case name == "enter":
		m.palette = nil
		if p.cursor < len(found) {
			if p.items != nil {
				if k, ok := runnableKey(found[p.cursor].key); ok {
					return m.key(keyFor(k))
				}
				return m, nil
			}
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
	if p.slots {
		return m.patternSlotKey(name)
	}
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
	case "p":
		if p.restorePlan != nil {
			p.withPattern = !p.withPattern
		}
	case "enter":
		return m.pickerEnter()
	}
	return m, nil
}

// patternSlotKey is the second step of a restore that also makes a pattern: which slot
// the pattern goes into. Esc goes back to the choice of bank.
func (m Model) patternSlotKey(name string) (tea.Model, tea.Cmd) {
	p := m.picker
	bank := func(b int) (tea.Model, tea.Cmd) {
		p.ptnBank, p.confirm = (b+proto.Banks)%proto.Banks, false
		return m.patternSlots()
	}
	step := func(dx, dy int) {
		col, row := p.pad%4, p.pad/4
		p.pad = clamp(row+dy, 0, 3)*4 + clamp(col+dx, 0, 3)
		p.confirm = false
	}
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'J' {
		return bank(int(name[0] - 'A'))
	}
	switch name {
	case "esc", "q":
		if p.saveFrom != nil {
			// Saving a pattern has no step before this one.
			m.picker = nil
			break
		}
		p.slots, p.confirm, p.pad = false, false, 0
	case "[", "H":
		return bank(p.ptnBank - 1)
	case "]", "L":
		return bank(p.ptnBank + 1)
	case "h", "left":
		step(-1, 0)
	case "l", "right":
		step(1, 0)
	case "j", "down":
		step(0, 1)
	case "k", "up":
		step(0, -1)
	case "enter":
		rows := m.patterns[p.ptnBank]
		if rows == nil {
			return m.patternSlots()
		}
		if p.saveFrom != nil && rows[p.pad].Slot == *p.saveFrom {
			m.message = "that is the pattern itself: choose another slot"
			return m, nil
		}
		// A slot that holds a pattern is replaced only on a second Enter.
		if rows[p.pad].Exists && !p.confirm {
			p.confirm = true
			return m, nil
		}
		if p.saveFrom != nil {
			from, to := *p.saveFrom, rows[p.pad].Slot
			m.picker = nil
			return m.saveHeardPattern(from, to)
		}
		plan, to, slot := p.restorePlan, p.bank, rows[p.pad].Slot
		m.picker = nil
		return m.runRestore(plan, to, &slot)
	}
	return m, nil
}

// patternSlots rests the slot chooser on the first free slot of its bank, reading the
// bank's patterns first when they are not in hand.
func (m Model) patternSlots() (tea.Model, tea.Cmd) {
	p := m.picker
	rows := m.patterns[p.ptnBank]
	if rows == nil {
		m.busy = "reading patterns"
		return m, m.loadPatterns(p.ptnBank)
	}
	p.pad = firstFreeSlot(rows)
	return m, nil
}

// firstFreeSlot is the first slot without a pattern; the first slot when all are taken.
func firstFreeSlot(rows []app.PatternRow) int {
	for i, row := range rows {
		if !row.Exists {
			return i
		}
	}
	return 0
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
	if len(p.bounces) > 0 {
		return m.bounceEnter()
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
	if p.kind == promptQuit {
		m.prompt = nil
		if name == "y" || name == "enter" || name == "q" {
			m.stopListening()
			m.stopRecordingNow()
			return m, tea.Quit
		}
		return m, nil
	}
	if name == "esc" {
		m.prompt = nil
		if p.reopen != nil {
			return m.openAssign(*p.reopen)
		}
		return m, nil
	}
	if p.kind == promptBusScan {
		switch name {
		case "y", "enter":
			return m.apply()
		case "p":
			return m.askBusScan(!p.scanAll)
		case "n", "q":
			m.prompt = nil
		}
		return m, nil
	}
	if p.kind == promptDelete || p.kind == promptRepairPads || p.kind == promptConfirmRestore || p.kind == promptDeleteFiles || p.kind == promptEraseProject || p.kind == promptDeletePatterns {
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
			p.choice = stepChoice(p.choice, len(folderChoices(m.tree.Rows(), m.indexes[m.root], m.tags, p.buffer)), name)
			return m, nil
		}
		if name != "enter" {
			p.choice = 0 // typing starts the list over
		}
	}
	switch {
	case name == "enter":
		return m.apply()
	case p.edit(name):
	case name == "tab" && p.kind == promptBulkRename:
		p.suffix = !p.suffix
	case key.Type == tea.KeyRunes || key.Type == tea.KeySpace:
		for _, r := range key.Runes {
			// The unit only accepts printable ASCII in names; paths and searches can hold more.
			if r >= ' ' && (r <= '~' || p.kind == promptExport || p.kind == promptRestore || p.kind == promptSearch || p.kind == promptExportName || p.kind == promptAssignFile || p.kind == promptMkdir || p.kind == promptFindFolder || p.kind == promptProjectExport || p.kind == promptProjectExportName || p.kind == promptProjectRestore || p.kind == promptRecDir) {
				p.insert(string(r))
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
		prog := m.prog
		prog.begin(len(paths))
		deleteAll := func() string {
			defer prog.begin(0)
			deleted, failed := app.DeletePaths(unit, paths, prog.step)
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
	if p.kind == promptDeletePatterns {
		slots := p.pads
		m.patMarks = map[int]bool{}
		m.busy = "deleting"
		return m, m.edit(func() tea.Msg {
			deleted, failed := app.DeletePatterns(unit, slots)
			message := fmt.Sprintf("%s deleted", plural(deleted, "pattern"))
			if len(failed) > 0 {
				message += "; not deleted: " + strings.Join(failed, ", ")
			}
			return doneMsg{message: message, patterns: true, reload: true}
		})
	}
	if p.kind == promptEraseProject {
		project := p.project
		m.busy = fmt.Sprintf("erasing project %d", project+1)
		prog := m.prog
		prog.beginSweep()
		return m, m.edit(func() tea.Msg {
			defer prog.begin(0)
			if err := app.EraseProject(unit, project); err != nil {
				return doneMsg{message: "erase failed: " + err.Error(), reload: true}
			}
			return doneMsg{message: fmt.Sprintf("project %d erased", project+1), reload: true}
		})
	}
	if p.kind == promptConfirmRestore {
		plan := p.plan
		m.busy = fmt.Sprintf("restoring project %d", plan.Project+1)
		prog := m.prog
		prog.begin(len(plan.Files))
		return m, m.edit(func() tea.Msg {
			defer prog.begin(0)
			if err := app.RestoreProject(unit, plan, func(i int, name string) { prog.step(i, name) }); err != nil {
				return doneMsg{message: "restore failed: " + err.Error(), reload: true}
			}
			return doneMsg{message: fmt.Sprintf("project %d restored from %s", plan.Project+1, plan.Dir), reload: true}
		})
	}
	if p.kind != promptDelete && p.kind != promptFindFolder && p.kind != promptBusWords && p.kind != promptBusScan && p.kind != promptRepairPads && p.kind != promptRecDir && p.kind != promptRecDevice && p.kind != promptBusNote && text == "" {
		m.message = "nothing typed"
		return m, nil
	}
	switch p.kind {
	case promptSearch:
		return m.find(text)
	case promptFindFolder:
		return m.findFolder(text, p.choice)
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
		prog := m.prog
		return m, m.edit(func() tea.Msg {
			defer prog.begin(0)
			prog.begin(0)
			if n, err := app.ProjectFileCount(unit, project); err == nil {
				prog.begin(n)
			}
			summary, err := app.ExportProject(unit, project, dir, prog.step)
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
	case promptBusNote:
		m.mem.BusNotes[p.busOf-1] = text
		m.message = fmt.Sprintf("BUS %d: %s", p.busOf, text)
		return m, nil
	case promptRecDir:
		m.mem.RecordDir = text
		m.message = "recordings go to " + display(m.recordDir())
		return m, nil
	case promptRecDevice:
		m.mem.RecordDevice = text
		m.message = fmt.Sprintf("the input to record has %q in its name", m.recordDevice())
		return m, nil
	case promptRepairPads:
		pads := p.pads
		m.busy = "setting the pads up"
		return m, m.edit(func() tea.Msg {
			failed := 0
			for _, pad := range pads {
				if err := app.SetUpPad(unit, pad); err != nil {
					failed++
				}
			}
			message := fmt.Sprintf("%s set up: pan in the centre, hold 100, time stretch 100%%", plural(len(pads)-failed, "pad"))
			if failed > 0 {
				message += fmt.Sprintf(", %d failed", failed)
			}
			return doneMsg{message: message, reload: true}
		})
	case promptBusScan:
		buses := p.buses
		m.busy = "setting the buses"
		return m, m.edit(func() tea.Msg {
			failed := 0
			for pad, bus := range buses {
				if err := app.SetPadBus(unit, pad, bus); err != nil {
					failed++
				}
			}
			message := fmt.Sprintf("the bus of %s set by their names", plural(len(buses)-failed, "pad"))
			if failed > 0 {
				message += fmt.Sprintf(", %d failed", failed)
			}
			return doneMsg{message: message, reload: true}
		})
	case promptBusWords:
		rule := m.busRule()
		if p.busOf == 1 {
			rule.Bus1Words = splitWords(text)
		} else {
			rule.Bus2Words = splitWords(text)
		}
		m.setBusRule(rule)
		m.message = fmt.Sprintf("%s kept for BUS %d", plural(len(splitWords(text)), "word"), p.busOf)
		return m, nil
	case promptDelete:
		m.marked = map[int]bool{}
		m.busy = "deleting"
		project := m.snap.Project
		return m, m.edit(func() tea.Msg {
			failed := app.DeletePads(unit, p.pads)
			// What was noted of where these samples came from goes with them.
			origins, _ := app.ForgetOrigins(unit, project, p.pads)
			message := fmt.Sprintf("%d pads deleted", len(p.pads))
			switch {
			case len(failed) > 0:
				message = "delete failed: " + strings.Join(failed, ", ")
			case len(p.pads) == 1:
				message = fmt.Sprintf("%s: sample deleted", p.pads[0])
			}
			return doneMsg{message: message, reload: true, origins: origins}
		})
	case promptExport, promptExportName:
		dir := text
		if p.kind == promptExportName {
			dir = app.Join(p.path, text)
		}
		pads := proto.AllPads()[m.bank*proto.PadsPerBank : (m.bank+1)*proto.PadsPerBank]
		m.busy = "exporting"
		prog := m.prog
		prog.begin(0)
		return m, m.edit(func() tea.Msg {
			defer prog.begin(0)
			summary, err := app.ExportPads(unit, pads, dir, prog.stepTotal)
			if err != nil {
				return doneMsg{message: "export failed: " + err.Error()}
			}
			return doneMsg{message: summary}
		})
	case promptRestore:
		return m.restoreFrom(text)
	case promptTags:
		return m.tagsAnswered(p, text)
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

// runRestore puts an export onto a bank (-1: back where each pad came from). With a slot,
// it then writes a pattern there that starts every restored pad together.
func (m Model) runRestore(plan *app.PadRestore, bank int, slot *proto.PadIndex) (tea.Model, tea.Cmd) {
	unit := m.unit
	targets, err := plan.Targets(bank)
	if err != nil {
		m.message = err.Error()
		return m, nil
	}
	first := targets[0]
	project := m.snap.Project
	m.busy = fmt.Sprintf("restoring %d pads", len(plan.Pads))
	prog := m.prog
	prog.begin(0)
	return m, m.edit(func() tea.Msg {
		defer prog.begin(0)
		summary, err := app.RestorePadsTo(unit, plan, bank, prog.stepTotal)
		if err != nil {
			return doneMsg{message: "restore failed: " + err.Error(), reload: true}
		}
		var records []app.FolderUseRecord
		for b, n := range app.RestoredBanks(targets) {
			records = append(records, app.FolderUseRecord{Bank: b, Folder: plan.Dir, Pads: n})
		}
		uses, err := app.RecordFolderUses(unit, project, records)
		if err != nil {
			summary += "; the folder could not be noted as used: " + err.Error()
		}
		if slot == nil {
			return doneMsg{message: summary, reload: true, focus: &first, folderUses: uses}
		}
		if err := app.WriteStartPattern(unit, *slot, targets, app.StartBars); err != nil {
			return doneMsg{message: summary + "; the pattern was not made: " + err.Error(), reload: true, focus: &first, folderUses: uses}
		}
		summary += fmt.Sprintf("; pattern %s starts them together (%d bars)", *slot, app.StartBars)
		return doneMsg{message: summary, reload: true, focus: &first, patterns: true, folderUses: uses}
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
	if p.withPattern {
		// Next: which slot the pattern goes into.
		p.slots, p.ptnBank, p.confirm = true, p.bank, false
		return m.patternSlots()
	}
	plan, bank := p.restorePlan, p.bank
	m.picker = nil
	return m.runRestore(plan, bank, nil)
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

// cellOfPattern is the pad and the notes under the cursor of the open pattern.
func (m Model) cellOfPattern() (proto.PadIndex, []formats.NoteRecord, bool) {
	d := m.pattern
	if d == nil || len(d.Grid.Rows) == 0 {
		return proto.PadIndex{}, nil, false
	}
	pad := d.Grid.Rows[clamp(m.patRow, 0, len(d.Grid.Rows)-1)].Pad
	return pad, d.NotesAt(pad, clamp(m.patCol, 0, d.Grid.Steps-1)), true
}

// rangeOf is the range set on d, when there is one that fits it.
func (m Model) rangeOf(d *app.PatternDetail) (barRange, bool) {
	if r := m.patRange; r != nil && d != nil && r.slot == d.Slot && r.last < d.Bars() {
		return *r, true
	}
	return barRange{}, false
}

// colBounds are the columns of the open pattern the cursor may be on: [lo, hi), the range's
// bars when one is set, else the whole pattern.
func (m Model) colBounds(d *app.PatternDetail) (lo, hi int) {
	if r, ok := m.rangeOf(d); ok {
		spp := d.StepsPerPage()
		return r.first * spp, min((r.last+1)*spp, d.Grid.Steps)
	}
	return 0, d.Grid.Steps
}

// openRangePicker opens the list of ranges for the open pattern, on the range set now (or the
// bar the cursor is on).
func (m Model) openRangePicker() (tea.Model, tea.Cmd) {
	d := m.pattern
	items := []barRange{{slot: d.Slot, first: -1, last: -1}}
	for b := range d.Bars() {
		items = append(items, barRange{slot: d.Slot, first: b, last: b})
	}
	for b := 0; b+1 < d.Bars(); b++ {
		items = append(items, barRange{slot: d.Slot, first: b, last: b + 1})
	}
	cursor := 0
	if r, ok := m.rangeOf(d); ok {
		for i, it := range items {
			if it == r {
				cursor = i
			}
		}
	}
	m.rangePick = &rangePicker{cursor: cursor, items: items}
	return m, nil
}

// rangePickerKey drives the range list: j/k move, Enter sets the range, Esc closes.
func (m Model) rangePickerKey(name string) (tea.Model, tea.Cmd) {
	p := m.rangePick
	switch name {
	case "esc", "q":
		m.rangePick = nil
	case "j", "down":
		p.cursor = clamp(p.cursor+1, 0, len(p.items)-1)
	case "k", "up":
		p.cursor = clamp(p.cursor-1, 0, len(p.items)-1)
	case "g":
		p.cursor = 0
	case "G":
		p.cursor = len(p.items) - 1
	case "enter":
		r := p.items[p.cursor]
		m.rangePick = nil
		if r.first < 0 {
			return m.setRange(nil)
		}
		return m.setRange(&r)
	}
	return m, nil
}

// setRange limits the open pattern to r (nil: the whole pattern again): the cursor moves into
// it, and a loop of this pattern starts over on it.
func (m Model) setRange(r *barRange) (tea.Model, tea.Cmd) {
	d := m.pattern
	m.patRange = r
	lo, hi := m.colBounds(d)
	if m.patCol < lo || m.patCol >= hi {
		// Into the range, on the same step of the bar.
		m.patCol = lo + m.patCol%d.StepsPerPage()
		m.patCol = clamp(m.patCol, lo, hi-1)
	}
	m.message = "the whole pattern is shown and played"
	if r != nil {
		m.message = r.label() + " only: shown, edited and looped  ·  { } move it  ·  v changes it"
	}
	if m.looping() && *m.loopSlot == d.Slot {
		model, cmd := m.startLoop(d.Slot)
		mm := model.(Model)
		mm.message = m.message
		return mm, cmd
	}
	return m, nil
}

// slideRange moves the range by its own length, earlier (dir -1) or later, staying inside the
// pattern; the cursor goes with it.
func (m Model) slideRange(dir int) (tea.Model, tea.Cmd) {
	d := m.pattern
	r, _ := m.rangeOf(d)
	n := r.bars()
	first := clamp(r.first+dir*n, 0, d.Bars()-n)
	if first == r.first {
		m.message = "the range is at the " + map[int]string{-1: "start", 1: "end"}[dir] + " of the pattern"
		return m, nil
	}
	m.patCol = clamp(m.patCol+(first-r.first)*d.StepsPerPage(), 0, d.Grid.Steps-1)
	return m.setRange(&barRange{slot: d.Slot, first: first, last: first + n - 1})
}

// pageSteps is how many steps one page of the open pattern's grid shows: the bars that fit the
// body (at most two), so { and } turn the page the header says comes next.
func (m Model) pageSteps(d *app.PatternDetail) int {
	sp := m.patSpan(d, m.geom().bodyW-2)
	return max(sp.hi-sp.lo, d.StepsPerPage())
}

// patStepLabel names the cursor's place as "bar N step M", where M is the 16th within the bar.
func (m Model) patStepLabel() string {
	spp := m.pattern.StepsPerPage()
	return fmt.Sprintf("bar %d step %d", m.patCol/spp+1, m.patCol%spp+1)
}

// editPatternBytes changes the open pattern's file on the unit with edit, then reads it again
// so that the grid shows what was written; note is what is said afterwards.
func (m Model) editPatternBytes(edit func(b []byte) ([]byte, error), note string) (tea.Model, tea.Cmd) {
	unit, ops, slot := m.unit, m.ops, m.pattern.Slot
	return m, func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		before, err := app.EditPattern(unit, slot, edit)
		if err != nil {
			return patternMsg{err: err}
		}
		detail, err := app.OpenPattern(unit, slot)
		return patternMsg{detail: detail, err: err, note: note, undo: before}
	}
}

// toggleNote puts a note at the cursor, a whole step long and at full velocity, or takes the
// notes there out.
func (m Model) toggleNote() (tea.Model, tea.Cmd) {
	pad, _, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	d := m.pattern
	// Only the notes that start at the step are taken out; under a longer note from before, a
	// new one is put.
	notes := d.CellNotes(pad, m.patCol)
	if len(notes) > 0 {
		offsets := make([]int, len(notes))
		for i, n := range notes {
			offsets[i] = n.Offset
		}
		return m.editPatternBytes(func(b []byte) ([]byte, error) {
			for _, at := range offsets {
				if err := formats.RemoveNote(b, at); err != nil {
					return nil, err
				}
			}
			return b, nil
		}, fmt.Sprintf("%s %s: %s taken out", d.Slot, m.patStepLabel(), plural(len(notes), "note")))
	}
	tick, length := d.StepTick(m.patCol), uint16(min(d.StepTicks(), 65535))
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		return formats.InsertNote(b, tick, pad, 127, length)
	}, fmt.Sprintf("%s %s: note on %s at velocity 127", d.Slot, m.patStepLabel(), pad))
}

// condKeys are the keys of the notes at the cursor step: the sixteenth each starts in.
func (m Model) condKeysAt() (pad proto.PadIndex, keys []app.NoteKey, ok bool) {
	pad, _, ok = m.cellOfPattern()
	if !ok {
		return pad, nil, false
	}
	// Only the notes that start in the step: one from before that goes on into it is not this one.
	notes := m.pattern.CellNotes(pad, m.patCol)
	seen := map[app.NoteKey]bool{}
	for _, n := range notes {
		k := app.NoteKey{Pad: n.Pad, Step: int(n.Tick) / (formats.PPQ / 4)}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return pad, keys, len(keys) > 0
}

// changeCondition steps the condition of the note at the cursor through the list (c forward, C
// back): always, 1:2 2:2, 1:3 2:3 3:3 and so on, then the chances. It says at once how long the
// bounce of the pattern is now, since the conditions make it a cycle of loops.
func (m Model) changeCondition(forward bool) (tea.Model, tea.Cmd) {
	d := m.pattern
	pad, keys, ok := m.condKeysAt()
	if !ok {
		m.message = "no note here: Enter puts one, then c gives it a condition"
		return m, nil
	}
	dir := 1
	if !forward {
		dir = -1
	}
	next := app.StepCond(m.patConds[keys[0]], dir)
	conds := maps.Clone(m.patConds)
	if conds == nil {
		conds = app.Conditions{}
	}
	for _, k := range keys {
		conds[k] = next
	}
	m.patConds = conds.PrunedTo(d.Notes)
	m.saveConditions()
	m.loopDirty = true
	m.message = fmt.Sprintf("%s %s: %s%s", pad, m.patStepLabel(), next, m.cycleNote())
	return m, nil
}

// clearTrackInView takes out every note of the track under the cursor that starts in the bars on
// show (the page, or the range), leaving the track's notes in the other bars and the other
// tracks as they are. u brings them back.
func (m Model) clearTrackInView() (tea.Model, tea.Cmd) {
	pad, _, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	d := m.pattern
	sp := m.patSpan(d, m.geom().bodyW-2)
	ticks := map[uint32]bool{}
	for _, n := range d.Notes {
		if c := noteCol(d, n.Tick); n.Pad == pad && c >= sp.lo && c < sp.hi {
			ticks[n.Tick] = true
		}
	}
	if len(ticks) == 0 {
		m.message = fmt.Sprintf("no note of %s starts in %s", pad, spanLabel(d, sp))
		return m, nil
	}
	note := fmt.Sprintf("%s: the notes of %s in %s are taken out (u brings them back)", d.Slot, pad, strings.ToLower(spanLabel(d, sp)))
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		out, _, err := formats.EditNotes(b, pad, ticks, formats.NoteEdit{Delete: true})
		return out, err
	}, note)
}

// patternsKeyMove is a key of the open pattern that moves the cursor, done as it is without a
// selection (the selection is kept).
func (m Model) patternsKeyMove(name string) (tea.Model, tea.Cmd) {
	sel := m.patSel
	m.patSel = nil
	next, cmd := m.patternsKey(name)
	nm := next.(Model)
	nm.patSel = sel
	return nm, cmd
}

// rollAgain picks new dice for the chances of the pattern: another seed.
func (m Model) rollAgain() (tea.Model, tea.Cmd) {
	if !m.patConds.Chancy() {
		m.message = "no note has a chance: c steps a note's condition on to one (50% and so on)"
		return m, nil
	}
	m.patSeed = time.Now().UnixNano()
	m.saveConditions()
	m.loopDirty = true
	m.message = "the dice are rolled again" + m.cycleNote()
	return m, nil
}

// saveConditions keeps the conditions of the open pattern.
func (m Model) saveConditions() {
	if m.conds != nil && m.pattern != nil {
		m.conds.save(m.condKey(m.pattern.Slot), condsOf(m.patConds, m.patSeed))
	}
}

// cycleNote says, for the open pattern, how long it is when bounced, or "" with no conditions:
// ", bounces as 6 loops = 12 bars = 24.0 s".
func (m Model) cycleNote() string {
	if line := m.cycleLine(); line != "" {
		return "; bounced: " + line
	}
	return ""
}

// cycleLine is how long the open pattern is once bounced, with its conditions at the tempo of
// its bank: "6 loops = 12 bars = 24.0 s", "" when no condition makes it longer than a loop.
func (m Model) cycleLine() string {
	d := m.pattern
	if d == nil {
		return ""
	}
	live := onlyHeard(m.patConds.PrunedTo(d.Notes), m.muted, m.soloed)
	if len(live) == 0 {
		return ""
	}
	loops, bars, seconds := app.CycleOf(d.Length, d.Beats, d.BankTempo, live, condRolls)
	line := fmt.Sprintf("%s = %s = %.1f s", plural(loops, "loop"), plural(bars, "bar"), seconds)
	if live.Chancy() {
		line += " (chances rolled over those loops)"
	}
	return line
}

// cycleExplain says in plain words what the conditions line means: how long the pattern is,
// how many loops the bounce needs before it repeats, and what the note at the cursor does.
func (m Model) cycleExplain() []string {
	d := m.pattern
	live := onlyHeard(m.patConds.PrunedTo(d.Notes), m.muted, m.soloed)
	if len(live) == 0 {
		return nil
	}
	loops, bars, seconds := app.CycleOf(d.Length, d.Beats, d.BankTempo, live, condRolls)
	out := []string{fmt.Sprintf("the pattern is %s long; with these conditions it only repeats exactly after %s, so B bounces %s (%.1f s) to a pad",
		plural(max(bars/max(loops, 1), 1), "bar"), plural(loops, "loop"), plural(bars, "bar"), seconds)}
	// Several spans only meet again after their least common multiple: say why the bounce is long.
	if spans := live.Spans(); len(spans) > 1 && live.CycleLoops() > spans[len(spans)-1] {
		var words []string
		for _, n := range spans {
			words = append(words, fmt.Sprint(n))
		}
		out = append(out, fmt.Sprintf("conditions over %s loops only meet again after %s: that is why it is long. With the same span (all x:2, or 2 and 4) the cycle is just the longest one",
			strings.Join(words, " and "), plural(live.CycleLoops(), "loop")))
	}
	if seconds > 20 {
		out = append(out, fmt.Sprintf("%.0f s is long for a pad: fewer different spans make it shorter, and chances alone are 4 loops", seconds))
	}
	if _, keys, ok := m.condKeysAt(); ok {
		out = append([]string{"the note here plays " + m.patConds[keys[0]].Explain()}, out...)
	}
	return out
}

// nudgeNote moves the note at the cursor by a number of ticks, or onto the nearest grid step
// when snap is set, keeping everything else about it. The cursor follows the note into the step
// it lands in, so the offset shown beside it stays the one of the note just moved.
func (m Model) nudgeNote(by int, snap bool) (tea.Model, tea.Cmd) {
	pad, notes, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	if len(notes) == 0 {
		m.message = "no note here to move"
		return m, nil
	}
	d := m.pattern
	from := notes[0].Tick
	if snap {
		if by = -d.GridOffset(from); by == 0 {
			m.message = "that note is on the grid already"
			return m, nil
		}
	}
	to := clamp(int(from)+by, 0, int(d.Length)-1)
	if to == int(from) {
		m.message = "the note cannot move further that way"
		return m, nil
	}
	m.patCol = clamp(int(uint64(to)*uint64(d.Grid.Steps)/uint64(max(d.Length, 1))), 0, d.Grid.Steps-1)
	note := fmt.Sprintf("%s: the note of %s moved %+d ticks, now %s", d.Slot, pad, to-int(from), offsetText(d, uint32(to)))
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		return formats.MoveNote(b, pad, from, by)
	}, note)
}

// resetNote puts the notes at the cursor step back as a new note is: one step long, on the grid.
func (m Model) resetNote() (tea.Model, tea.Cmd) {
	pad, notes, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	if len(notes) == 0 {
		m.message = "no note here to reset"
		return m, nil
	}
	d := m.pattern
	step := int(max(d.StepTicks(), 1))
	set := map[formats.PadTick]bool{}
	plain := true
	for _, n := range notes {
		set[formats.PadTick{Pad: pad, Tick: n.Tick}] = true
		plain = plain && d.GridOffset(n.Tick) == 0 && int(n.Length) == step
	}
	if plain {
		m.message = "that note is one step long and on the grid already"
		return m, nil
	}
	if to := int64(notes[0].Tick) + int64(-d.GridOffset(notes[0].Tick)); to >= 0 {
		m.patCol = clamp(int(uint64(to)*uint64(d.Grid.Steps)/uint64(max(d.Length, 1))), 0, d.Grid.Steps-1)
	}
	note := fmt.Sprintf("%s: the note of %s reset: one step long, on the grid", d.Slot, pad)
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		out, _, err := formats.EditNoteSet(b, set, formats.NoteEdit{Snap: uint32(step), SetLength: step})
		return out, err
	}, note)
}

// offsetText says how far a tick is off the grid in words: "on the grid", "24 ticks late".
func offsetText(d *app.PatternDetail, tick uint32) string {
	switch off := d.GridOffset(tick); {
	case off > 0:
		return fmt.Sprintf("%d ticks late", off)
	case off < 0:
		return fmt.Sprintf("%d ticks early", -off)
	}
	return "on the grid"
}

// changeVelocity sets the velocity of the notes at the cursor: , and . by 1, < and > by 10,
// * to 127, where the note plays at the pad's own level.
func (m Model) changeVelocity(key string) (tea.Model, tea.Cmd) {
	pad, notes, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	if len(notes) == 0 {
		m.message = "no note here: Enter puts one"
		return m, nil
	}
	loudest := 0
	offsets := make([]int, len(notes))
	for i, n := range notes {
		offsets[i] = n.Offset
		loudest = max(loudest, int(n.Velocity))
	}
	next := loudest
	switch key {
	case ",":
		next--
	case ".":
		next++
	case "<":
		next -= 10
	case ">":
		next += 10
	case "*":
		next = 127
	}
	next = clamp(next, 1, 127)
	if next == loudest {
		m.message = fmt.Sprintf("velocity is %d already", loudest)
		return m, nil
	}
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		for _, at := range offsets {
			if err := formats.SetVelocity(b, at, uint8(next)); err != nil {
				return nil, err
			}
		}
		return b, nil
	}, fmt.Sprintf("%s: velocity of %s %s is %d", m.pattern.Slot, pad, m.patStepLabel(), next))
}

// undoPatternEdit puts the open pattern back one edit, moving the state it replaces onto the
// redo stack so redoPatternEdit can bring it back. Up to patHistoryMax edits are kept.
func (m Model) undoPatternEdit() (tea.Model, tea.Cmd) {
	h := m.patHist
	if h == nil || m.pattern == nil || h.slot != m.pattern.Slot || len(h.undo) == 0 {
		m.message = "nothing to undo in this pattern"
		return m, nil
	}
	// Pop the newest undo snapshot and write it back; the handler pushes the replaced bytes onto
	// the redo stack.
	snap := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	note := fmt.Sprintf("undo: step %d/%d (R redoes)", len(h.undo), len(h.undo)+len(h.redo)+1)
	return m.applyPatternSnapshot(snap, note, editUndo)
}

// redoPatternEdit brings back an edit undone with undoPatternEdit, moving the state it replaces
// back onto the undo stack.
func (m Model) redoPatternEdit() (tea.Model, tea.Cmd) {
	h := m.patHist
	if h == nil || m.pattern == nil || h.slot != m.pattern.Slot || len(h.redo) == 0 {
		m.message = "nothing to redo in this pattern"
		return m, nil
	}
	snap := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	note := fmt.Sprintf("redo: step %d/%d (u undoes)", len(h.undo)+1, len(h.undo)+len(h.redo)+1)
	return m.applyPatternSnapshot(snap, note, editRedo)
}

// applyPatternSnapshot writes a saved pattern back to the unit. op marks it as an undo or redo so
// the message handler moves the replaced bytes to the other stack instead of recording a fresh
// edit.
func (m Model) applyPatternSnapshot(snap []byte, note string, op patEditOp) (tea.Model, tea.Cmd) {
	unit, ops, slot := m.unit, m.ops, m.pattern.Slot
	saved := append([]byte(nil), snap...)
	return m, func() tea.Msg {
		ops.Lock()
		defer ops.Unlock()
		before, err := app.EditPattern(unit, slot, func([]byte) ([]byte, error) { return saved, nil })
		if err != nil {
			return patternMsg{err: err}
		}
		detail, err := app.OpenPattern(unit, slot)
		return patternMsg{detail: detail, err: err, note: note, undo: before, op: op}
	}
}

// patToolCount is how many groove tools the menu offers.
const patToolCount = 8

// newPatTool starts the groove-tools menu with sensible amounts. pad scopes it to one track; nil
// is the whole pattern.
func newPatTool(pad *proto.PadIndex) *patTool {
	return &patTool{pad: pad, swing: 54, quantStrength: 100, shift: 1, human: 20}
}

// toolScope is the formats scope the tools run with: one pad, or the whole pattern.
func (t *patTool) toolScope() formats.Scope {
	if t.pad != nil {
		return formats.OnlyPad(*t.pad)
	}
	return formats.WholePattern
}

// patToolKey drives the groove-tools menu over the open pattern: j/k pick a tool, h/l (or -/+)
// change its amount, e toggles the eighth/sixteenth grid, Enter applies it to the whole pattern,
// Esc closes the menu.
func (m Model) patToolKey(name string) (tea.Model, tea.Cmd) {
	t := m.patTool
	switch name {
	case "esc":
		m.patTool = nil
	case "j", "down":
		t.kind = patToolKind((int(t.kind) + 1) % patToolCount)
	case "k", "up":
		t.kind = patToolKind((int(t.kind) + patToolCount - 1) % patToolCount)
	case "e":
		t.eighths = !t.eighths
	case "l", "right", "+", "=":
		t.adjust(+1)
	case "h", "left", "-":
		t.adjust(-1)
	case "enter":
		return m.applyPatTool()
	}
	return m, nil
}

// adjust nudges the amount of the chosen tool, keeping it in range.
func (t *patTool) adjust(dir int) {
	switch t.kind {
	case toolSwing:
		t.swing = clamp(t.swing+dir*2, 50, 75)
	case toolQuantize:
		t.quantStrength = clamp(t.quantStrength+dir*10, 0, 100)
	case toolShift:
		t.shift = clamp(t.shift+dir, -16, 16)
	case toolHumanize:
		t.human = clamp(t.human+dir*5, 0, 100)
	}
}

// toolGrid is the resolution the swing/quantize/shift tools work on: an eighth note or a
// sixteenth.
func (t *patTool) toolGrid() uint32 {
	if t.eighths {
		return formats.PPQ / 2
	}
	return formats.PPQ / 4
}

// applyPatTool runs the chosen tool over its scope (one pad or the whole pattern) and writes the
// result back, pushing the edit onto the undo stack. Swing is MPC-style (off-beats pushed).
func (m Model) applyPatTool() (tea.Model, tea.Cmd) {
	t := *m.patTool
	grid := t.toolGrid()
	scope := t.toolScope()
	res := "16ths"
	if t.eighths {
		res = "8ths"
	}
	where := "all pads"
	if t.pad != nil {
		where = t.pad.String()
	}
	var edit func(b []byte) ([]byte, error)
	var note string
	switch t.kind {
	case toolSwing:
		edit = func(b []byte) ([]byte, error) { return formats.Swing(b, grid, float64(t.swing), scope) }
		note = fmt.Sprintf("swing %d%% on %s", t.swing, res)
	case toolQuantize:
		edit = func(b []byte) ([]byte, error) {
			return formats.Quantize(b, grid, float64(t.quantStrength)/100, scope)
		}
		note = fmt.Sprintf("quantize %d%% to %s", t.quantStrength, res)
	case toolShift:
		edit = func(b []byte) ([]byte, error) { return formats.Shift(b, grid, t.shift, scope) }
		note = fmt.Sprintf("shift %+d %s", t.shift, res)
	case toolReverse:
		edit = func(b []byte) ([]byte, error) { return formats.Reverse(b, scope) }
		note = "reverse"
	case toolHumanize:
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		amt := t.human
		edit = func(b []byte) ([]byte, error) {
			return formats.Humanize(b, uint32(amt), uint8(amt), nil, scope, rng)
		}
		note = fmt.Sprintf("humanize %d", t.human)
	case toolDouble, toolExtend, toolHalve:
		// A pattern is one length for all its tracks, so these work on all of it.
		d := m.pattern
		edit, where = formats.DoublePattern, "all pads"
		note = fmt.Sprintf("doubled: %s → %s", plural(d.Bars(), "bar"), plural(d.Bars()*2, "bar"))
		if t.kind == toolExtend {
			edit = formats.ExtendPattern
			note = fmt.Sprintf("extended: %s → %s, the new half empty", plural(d.Bars(), "bar"), plural(d.Bars()*2, "bar"))
		}
		if t.kind == toolHalve {
			edit = formats.HalvePattern
			note = fmt.Sprintf("halved: %s → %s, the second half gone", plural(d.Bars(), "bar"), plural(max(d.Bars()/2, 1), "bar"))
		}
	default:
		return m, nil
	}
	m.patTool = nil
	return m.editPatternBytes(edit, note+"  ·  "+where)
}

func (m Model) changeLength(longer bool) (tea.Model, tea.Cmd) {
	pad, notes, ok := m.cellOfPattern()
	if !ok {
		return m, nil
	}
	if len(notes) == 0 {
		m.message = "no note here: Enter puts one"
		return m, nil
	}
	d := m.pattern
	step := max(d.StepTicks(), 1)
	type change struct {
		offset int
		length uint16
	}
	var changes []change
	for _, n := range notes {
		length := uint32(n.Length)
		if longer {
			length = min(length+step, d.Length-n.Tick, 65535)
		} else {
			length = max(length-min(length, step), min(step, uint32(n.Length)))
		}
		if length != uint32(n.Length) {
			changes = append(changes, change{n.Offset, uint16(length)})
		}
	}
	if len(changes) == 0 {
		if longer {
			m.message = "the note lasts to the end of the pattern already"
		} else {
			m.message = "the note is one step long already"
		}
		return m, nil
	}
	steps := float64(changes[0].length) / float64(step)
	return m.editPatternBytes(func(b []byte) ([]byte, error) {
		for _, c := range changes {
			if err := formats.SetLength(b, c.offset, c.length); err != nil {
				return nil, err
			}
		}
		return b, nil
	}, fmt.Sprintf("%s: the note of %s lasts %g steps", d.Slot, pad, math.Round(steps*10)/10))
}

// toggleMute silences a pad in the mix this computer plays, or lets it sound again. The unit is
// not told: its own mute is on its pads.
func (m Model) toggleMute(pad proto.PadIndex) (tea.Model, tea.Cmd) {
	if m.muted[pad] {
		delete(m.muted, pad)
		m.message = pad.String() + " sounds again in the mix on this computer"
	} else {
		m.muted[pad] = true
		m.message = pad.String() + " is muted in the mix on this computer (m again unmutes)"
	}
	m.loopDirty = true
	return m, nil
}

// toggleSolo makes a pad one of the few that sound in the mix this computer plays, or takes
// it out of them; with none soloed, every pad sounds.
func (m Model) toggleSolo(pad proto.PadIndex) (tea.Model, tea.Cmd) {
	if m.soloed[pad] {
		delete(m.soloed, pad)
		m.message = pad.String() + " is no longer soloed"
	} else {
		m.soloed[pad] = true
		m.message = pad.String() + " is soloed: only the soloed pads sound on this computer (M again unsolos)"
	}
	m.loopDirty = true
	return m, nil
}

// chooseHeardSlot asks which slot the open pattern is saved into as it is heard; the chooser
// rests on the first free slot of its bank.
func (m Model) chooseHeardSlot() (tea.Model, tea.Cmd) {
	d := m.pattern
	if len(d.PadsUsed) > 0 && !slices.ContainsFunc(d.PadsUsed, func(pad proto.PadIndex) bool { return heard(pad, m.muted, m.soloed) }) {
		m.message = "every pad of the pattern is muted: nothing to save"
		return m, nil
	}
	from := d.Slot
	m.picker = &picker{slots: true, ptnBank: int(from.Index()) / proto.PadsPerBank, saveFrom: &from}
	return m.patternSlots()
}

// saveHeardPattern copies pattern from into slot to with only the notes of the pads heard in
// the mix on this computer: muted pads, and the pads not soloed when some are, are left out.
// The new pattern plays the same on the unit.
func (m Model) saveHeardPattern(from, to proto.PadIndex) (tea.Model, tea.Cmd) {
	var left []string
	if d := m.pattern; d != nil && d.Slot == from {
		for _, pad := range d.PadsUsed {
			if !heard(pad, m.muted, m.soloed) {
				left = append(left, pad.String())
			}
		}
	}
	unit := m.unit
	muted, soloed := maps.Clone(m.muted), maps.Clone(m.soloed)
	m.busy = "saving what you hear as " + to.String()
	return m, m.edit(func() tea.Msg {
		dropped, err := app.CopyPatternKeeping(unit, from, to, func(pad proto.PadIndex) bool { return heard(pad, muted, soloed) })
		if err != nil {
			return doneMsg{message: "the pattern was not saved: " + err.Error(), patterns: true}
		}
		message := fmt.Sprintf("%s saved as %s, as you hear it", from, to)
		if len(left) > 0 {
			message += fmt.Sprintf(": without %s (%s left out)", strings.Join(left, " "), plural(dropped, "note"))
		} else {
			message += ": nothing muted, so a full copy"
		}
		return doneMsg{message: message, patterns: true}
	})
}

// heard says whether a pad sounds in the mix on this computer: not muted, and soloed when any is.
func heard(pad proto.PadIndex, muted, soloed map[proto.PadIndex]bool) bool {
	if muted[pad] {
		return false
	}
	return len(soloed) == 0 || soloed[pad]
}
