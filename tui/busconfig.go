package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/record"
)

// busRule is the rule that sends a pad to a bus by its name: the one set in the Config view, or
// the default.
func (m Model) busRule() app.BusRule {
	if m.mem.Buses != nil && m.mem.Buses.Version == app.BusRuleVersion {
		return *m.mem.Buses
	}
	return app.DefaultBusRule()
}

// colourOfBus is the 256-colour code of a named bus colour, "" for the theme's own.
func colourOfBus(name string) lipgloss.Color {
	for _, c := range app.BusColours {
		if c.Name == name {
			return lipgloss.Color(c.Code)
		}
	}
	return ""
}

// withBusColours gives the theme the colours the rule names for the buses, so that the pads, the
// Config view and everything else draw a bus in the colour chosen.
func (m Model) withBusColours() Model {
	rule := m.busRule()
	if c := colourOfBus(rule.Bus1Colour); c != "" {
		m.st.th.bus1 = c
	}
	if c := colourOfBus(rule.Bus2Colour); c != "" {
		m.st.th.bus2 = c
	}
	return m
}

// setBusRule keeps a rule; it is saved with the rest of what is remembered.
func (m *Model) setBusRule(r app.BusRule) {
	m.mem.Buses = &r
}

// splitWords reads a list of words typed with commas or spaces between them: lower case, once each.
func splitWords(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// ---- the Config view ----

const (
	cfgBus1Words = iota
	cfgBus2Words
	cfgBus1Colour
	cfgBus2Colour
	cfgOther
	cfgOnAssign
	cfgReset
	cfgRecDir
	cfgRecDevice
	cfgRows
)

// cycleColour is the colour after (or before) the named one, the theme's own ("") at one end.
func cycleColour(name string, by int) string {
	names := []string{""}
	for _, c := range app.BusColours {
		names = append(names, c.Name)
	}
	i := slices.Index(names, name)
	return names[(max(i, 0)+by+len(names))%len(names)]
}

func (m Model) configKey(name string) (tea.Model, tea.Cmd) {
	rule := m.busRule()
	switch name {
	case "j", "down":
		m.cfgAt = min(m.cfgAt+1, cfgRows-1)
	case "k", "up":
		m.cfgAt = max(m.cfgAt-1, 0)
	case "g":
		m.cfgAt = 0
	case "G":
		m.cfgAt = cfgRows - 1
	case "h", "left", "l", "right", " ", "enter":
		by := 1
		if name == "h" || name == "left" {
			by = -1
		}
		switch m.cfgAt {
		case cfgBus1Words, cfgBus2Words:
			if name == " " || name == "enter" {
				bus, words := 1, rule.Bus1Words
				if m.cfgAt == cfgBus2Words {
					bus, words = 2, rule.Bus2Words
				}
				m.prompt = &prompt{kind: promptBusWords, busOf: bus, buffer: strings.Join(words, ", ")}
			}
			return m, nil
		case cfgBus1Colour:
			rule.Bus1Colour = cycleColour(rule.Bus1Colour, by)
		case cfgBus2Colour:
			rule.Bus2Colour = cycleColour(rule.Bus2Colour, by)
		case cfgOther:
			rule.Other = (rule.Other + by + 3) % 3
		case cfgOnAssign:
			rule.OnAssign = !rule.OnAssign
		case cfgReset:
			if name != "enter" && name != " " {
				return m, nil
			}
			rule = app.DefaultBusRule()
			m.message = "the bus rule and colours are back to their defaults"
		case cfgRecDir:
			if name == " " || name == "enter" {
				m.prompt = &prompt{kind: promptRecDir, buffer: m.mem.RecordDir}
			}
			return m, nil
		case cfgRecDevice:
			if name == " " || name == "enter" {
				m.prompt = &prompt{kind: promptRecDevice, buffer: m.recordDevice()}
			}
			return m, nil
		}
		m.setBusRule(rule)
	}
	return m, nil
}

// configBody draws the Config view: the settings of the bus rule, and what it makes of the pads of
// the bank in front.
func (m Model) configBody(g geom) string {
	st, rule := m.st, m.busRule()
	const labelW = 36
	busStyle := func(bus int32) lipgloss.Style {
		c, _ := st.th.busColour(bus)
		return lipgloss.NewStyle().Foreground(c).Bold(true)
	}
	row := func(i int, label, value, hint string) string {
		// The label can hold coloured text, so its room is counted in cells, not bytes.
		pad := strings.Repeat(" ", max(labelW-ansi.StringWidth(label), 2))
		marker, name := "  ", st.text.Render(label)
		if i == m.cfgAt {
			marker, name = st.accent.Render("▶ "), st.bold.Render(label)
		}
		return marker + name + pad + value + "  " + st.dim.Render(hint)
	}
	valueW := max(g.bodyW-labelW-24, 20)
	words := func(list []string) string {
		return st.text.Render(ansi.Truncate(strings.Join(list, ", "), valueW, "…"))
	}
	colour := func(bus int32, name string) string {
		label := name
		if label == "" {
			label = "the theme's"
		}
		return busStyle(bus).Render("██ " + label)
	}
	onOff := map[bool]string{true: "on", false: "off"}
	other := busName(int32(rule.Other))
	if rule.Other == 1 || rule.Other == 2 {
		other = busStyle(int32(rule.Other)).Render(other)
	}
	lines := []string{
		st.bold.Render("Buses by pad name"), "",
		row(cfgBus1Words, "Words for samples on "+busStyle(1).Render("BUS 1"), words(rule.Bus1Words), "Enter edits"),
		row(cfgBus2Words, "Words for samples on "+busStyle(2).Render("BUS 2"), words(rule.Bus2Words), "Enter edits"),
		"",
		row(cfgBus1Colour, "Colour of "+busStyle(1).Render("BUS 1"), colour(1, rule.Bus1Colour), "h l change"),
		row(cfgBus2Colour, "Colour of "+busStyle(2).Render("BUS 2"), colour(2, rule.Bus2Colour), "h l change"),
		"",
		row(cfgOther, "A name with none of the words", other, "h l change"),
		row(cfgOnAssign, "Set the bus when a file is assigned", st.text.Render(onOff[rule.OnAssign]), "Space toggles"),
		row(cfgReset, "Back to the defaults", "", "Enter"),
		"",
		st.bold.Render("Recorder"),
		row(cfgRecDir, "Record into folder", st.text.Render(display(m.recordDir())), "Enter edits"),
		row(cfgRecDevice, "Input with this in its name", st.text.Render(m.recordDevice()), "Enter edits"),
		st.dim.Render(m.recorderLine()),
		"",
		st.dim.Render("A word matches a word of the pad's name (Kick_125, OHat, DrumLoop, 14_Perc1) and a longer one by its start;"),
		st.dim.Render("where a name has words of both, BUS 1 wins. B in Pads sets the buses of the bank, the marked pads or the project."),
		"",
		st.bold.Render(fmt.Sprintf("What it makes of bank %c", 'A'+m.bank)),
	}
	bank := m.snap.Bank(m.bank)
	colW := max((g.bodyW-4)/2, 30)
	for i := 0; i < len(bank); i += 2 {
		var cells []string
		for _, r := range bank[i:min(i+2, len(bank))] {
			cells = append(cells, lipgloss.NewStyle().Width(colW).Render(m.busPreview(rule, r, colW)))
		}
		lines = append(lines, strings.Join(cells, ""))
	}
	return m.box("Config", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}

// busStateText is a bus drawn in its colour.
func (m Model) busStateText(bus int32) string {
	if c, ok := m.st.th.busColour(bus); ok {
		return lipgloss.NewStyle().Foreground(c).Bold(true).Render(busName(bus))
	}
	return m.st.dim.Render(busName(bus))
}

// busPreview is a line about a pad: its name, the word that decides it, and the bus it is on,
// and what the rule would make of that.
func (m Model) busPreview(rule app.BusRule, r app.PadRow, width int) string {
	st := m.st
	if !r.HasSample {
		return st.faint.Render(fmt.Sprintf("%-4s", r.Pad))
	}
	want, now := rule.BusFor(r.Name), r.Params["bus-fx"]
	why := map[int]string{0: "no word", 1: "BUS 1 word", 2: "BUS 2 word"}[rule.Match(r.Name)]
	state := m.busStateText(now)
	if now != want {
		state = m.busStateText(now) + st.warn.Render(" → ") + m.busStateText(want)
	}
	w := max(width-38, 10)
	name := ansi.Truncate(r.Name, w, "…")
	return fmt.Sprintf("%-4s %-*s %s  %s", r.Pad, w, name, st.dim.Render(fmt.Sprintf("%-10s", why)), state)
}

// ---- scanning the pads ----

// busChanges are the pads of the bank (the marked ones among them, when some are marked; or the
// whole project) that are not on the bus their name says, each with that bus, and how many of
// the pads that were looked at are drums and not.
func (m Model) busChanges(all bool) (changes map[proto.PadIndex]int32, words1, words2, none, right int) {
	rule := m.busRule()
	changes = map[proto.PadIndex]int32{}
	rows := m.snap.Bank(m.bank)
	if all {
		rows = m.snap.Pads[:]
	}
	for i, r := range rows {
		if !r.HasSample || (!all && len(m.marked) > 0 && !m.marked[i]) {
			continue
		}
		want := rule.BusFor(r.Name)
		switch rule.Match(r.Name) {
		case 1:
			words1++
		case 2:
			words2++
		default:
			none++
		}
		if r.Params["bus-fx"] == want {
			right++
			continue
		}
		changes[r.Pad] = want
	}
	return changes, words1, words2, none, right
}

// askBusScan looks at the pads and asks before setting the bus of those that are not on the one
// their name says.
func (m Model) askBusScan(all bool) (tea.Model, tea.Cmd) {
	changes, words1, words2, none, right := m.busChanges(all)
	if len(changes) == 0 {
		where := fmt.Sprintf("bank %c", 'A'+m.bank)
		if all {
			where = "the project"
		}
		m.prompt = nil
		m.message = fmt.Sprintf("every pad of %s with a sample is on the bus its name says (%d by BUS 1 words, %d by BUS 2 words, %d by neither)", where, words1, words2, none)
		if right == 0 {
			m.message = fmt.Sprintf("no pad with a sample in %s", where)
		}
		return m, nil
	}
	m.prompt = &prompt{kind: promptBusScan, buses: changes, scanAll: all}
	return m, nil
}

func (m Model) busScanBox(p *prompt) string {
	st := m.st
	rule := m.busRule()
	_, w1, w2, none, right := m.busChanges(p.scanAll)
	scope := fmt.Sprintf("bank %c", 'A'+m.bank)
	switch {
	case p.scanAll:
		scope = "the whole project"
	case len(m.marked) > 0:
		scope = fmt.Sprintf("the %d marked pads of bank %c", len(m.marked), 'A'+m.bank)
	}
	to := map[int32]int{}
	var examples []string
	pads := make([]proto.PadIndex, 0, len(p.buses))
	for pad := range p.buses {
		pads = append(pads, pad)
	}
	slices.SortFunc(pads, func(a, b proto.PadIndex) int { return int(a.Index()) - int(b.Index()) })
	for _, pad := range pads {
		to[p.buses[pad]]++
		if len(examples) < 6 {
			examples = append(examples, fmt.Sprintf("%s %s → %s", pad, ansi.Truncate(m.snap.Pads[pad.Index()].Name, 22, "…"), m.busStateText(p.buses[pad])))
		}
	}
	lines := []string{
		st.bold.Render(fmt.Sprintf("Set the bus of %s by their names?", plural(len(p.buses), "pad"))),
		st.dim.Render(scope + fmt.Sprintf("  ·  %d by BUS 1 words, %d by BUS 2 words, %d by neither (→ %s), %d already right", w1, w2, none, busName(int32(rule.Other)), right)), "",
		fmt.Sprintf("%d → %s    %d → %s    %d → %s", to[1], m.busStateText(1), to[2], m.busStateText(2), to[0], m.busStateText(0)),
	}
	for _, e := range examples {
		lines = append(lines, st.dim.Render("  ")+e)
	}
	if len(pads) > len(examples) {
		lines = append(lines, st.dim.Render(fmt.Sprintf("  … and %d more", len(pads)-len(examples))))
	}
	other := "the whole project"
	if p.scanAll {
		other = "this bank"
	}
	lines = append(lines, "", st.dim.Render("y apply   p "+other+" instead   n/Esc leave them   (the words are in Config, 6)"))
	return st.modal.Render(strings.Join(lines, "\n"))
}

// recorderLine says how recording works, and whether this computer can do it.
func (m Model) recorderLine() string {
	tool := record.Tool(m.recorder)
	if tool == "" {
		return "Ctrl-R records the unit's audio to a WAV file, from anywhere. No recorder is installed: install ffmpeg (or sox)."
	}
	return "Ctrl-R starts and stops a recording of the unit's audio (as 24-bit 48 kHz WAV, with " + tool + ") from anywhere; the header shows it."
}
