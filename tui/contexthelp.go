package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// openContextPalette opens the searchable list of what can be done with the view or dialog in
// front, which ? asks for everywhere. A dialog with no list of its own gets the older help.
func (m Model) openContextPalette() (tea.Model, tea.Cmd) {
	items, title, ok := m.contextCommands()
	if !ok {
		m.help = true
		return m, nil
	}
	m.palette = &palette{items: items, title: title}
	return m, nil
}

// typingText reports whether a text is being typed, where ? is a letter like the others.
func (m Model) typingText() bool {
	return m.prompt != nil || m.search != nil || (m.browser != nil && m.browser.finding)
}

// contextCommands are the commands of what is in front: its keys, each with what it does. For
// a view with no items of its own (items nil, ok true) they are the commands of the section.
func (m Model) contextCommands() (items []command, title string, ok bool) {
	switch {
	case m.addAsk != nil && m.addAsk.pads != nil:
		return listCommands("Pad", "arrows move", "[ ] bank", "Enter slice it", "Esc back"), "Choose a pad", true
	case m.addAsk != nil && m.addAsk.patterns != nil:
		return listCommands("Pattern", "arrows move", "[ ] bank", "Enter open it", "Esc back"), "Choose a pattern", true
	case m.addAsk != nil:
		return listCommands("p a pad", "f a new sample from the file view", "Esc cancel"), "Slicer", true
	case m.browser != nil && m.browser.kind == browseAssign:
		var cmds []command
		for _, k := range m.assignKeyList(m.browser) {
			cmds = append(cmds, command{key: k[0], label: k[1]})
		}
		return cmds, "Assign a file", true
	case m.browser != nil:
		return listCommands("Enter use the folder in front", "Tab / l open a folder", "h close it", "t next source", "p type a path", "f find a folder", "Esc cancel"), "Choose a folder", true
	case m.picker != nil && !m.picker.slots:
		return listCommands("A-J bank", "1-16 pad", "arrows move", "[ ] bank", "Enter choose", "Esc back"), "Choose a pad", true
	case m.slicer != nil && !m.slicer.adding && m.section == secSlicer:
		return groupCommands(m.slicer.slicerHelp()), "Slicer", true
	case m.lenPick != nil:
		return listCommands("h l fewer or more bars", "1-8 that many bars", "Enter make the pattern", "Esc back to the templates"), "How long", true
	case m.editor != nil, m.chromatic != nil, m.toggles != nil, m.tmplPicker != nil, m.rangePick != nil, m.laneMap != nil, m.patTool != nil:
		return nil, "", false
	case m.section == secPatterns && m.pattern != nil:
		return groupCommands(m.patternHelp(m.pattern)), "Pattern " + m.pattern.Slot.String(), true
	}
	return nil, "Commands", true
}

// groupCommands turns the groups of keys a dialog shows under it into commands.
func groupCommands(groups []helpGroup) []command {
	var out []command
	for _, g := range groups {
		for _, k := range g.keys {
			if k.key == "" {
				continue
			}
			out = append(out, command{key: k.key, label: g.name + ": " + k.what})
		}
	}
	return out
}

// listCommands makes commands of "key what" lines.
func listCommands(lines ...string) []command {
	var out []command
	for _, l := range lines {
		key, what, _ := strings.Cut(l, " ")
		out = append(out, command{key: key, label: what})
	}
	return out
}

// runnableKey is the key press a command stands for when it is a single key that can be sent
// again to the dialog under the palette; the ones with several keys are only for reading.
func runnableKey(spec string) (string, bool) {
	switch strings.ToLower(spec) {
	case "enter":
		return "enter", true
	case "esc":
		return "esc", true
	case "space":
		return "space", true
	case "tab":
		return "tab", true
	case "ctrl-a", "ctrl+a":
		return "ctrl+a", true
	}
	if len([]rune(spec)) == 1 {
		return spec, true
	}
	return "", false
}
