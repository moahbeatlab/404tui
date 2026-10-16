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
)

// busRule is the rule that sends a pad to a bus by its name: the one set in the Config view, or
// the default.
func (m Model) busRule() app.BusRule {
	if m.mem.Buses != nil {
		return *m.mem.Buses
	}
	return app.DefaultBusRule()
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
	cfgWords = iota
	cfgDrumBus
	cfgOtherBus
	cfgOnAssign
	cfgReset
	cfgRows
)

func (m Model) configKey(name string) (tea.Model, tea.Cmd) {
	rule := m.busRule()
	step := func(bus, by int) int { return (bus + by + 3) % 3 }
	switch name {
	case "j", "down":
		m.cfgAt = min(m.cfgAt+1, cfgRows-1)
	case "k", "up":
		m.cfgAt = max(m.cfgAt-1, 0)
	case "g":
		m.cfgAt = 0
	case "G":
		m.cfgAt = cfgRows - 1
	case "h", "left", "l", "right":
		by := 1
		if name == "h" || name == "left" {
			by = -1
		}
		switch m.cfgAt {
		case cfgDrumBus:
			rule.DrumBus = step(rule.DrumBus, by)
		case cfgOtherBus:
			rule.OtherBus = step(rule.OtherBus, by)
		case cfgOnAssign:
			rule.OnAssign = !rule.OnAssign
		default:
			return m, nil
		}
		m.setBusRule(rule)
	case " ", "enter":
		switch m.cfgAt {
		case cfgWords:
			m.prompt = &prompt{kind: promptDrumWords, buffer: strings.Join(rule.DrumWords, ", ")}
		case cfgDrumBus:
			rule.DrumBus = step(rule.DrumBus, 1)
			m.setBusRule(rule)
		case cfgOtherBus:
			rule.OtherBus = step(rule.OtherBus, 1)
			m.setBusRule(rule)
		case cfgOnAssign:
			rule.OnAssign = !rule.OnAssign
			m.setBusRule(rule)
		case cfgReset:
			m.setBusRule(app.DefaultBusRule())
			m.message = "the bus rule is back to its defaults"
		}
	}
	return m, nil
}

// configBody draws the Config view: the settings of the bus rule, and what it makes of the pads of
// the bank in front.
func (m Model) configBody(g geom) string {
	st, rule := m.st, m.busRule()
	row := func(i int, label, value, hint string) string {
		marker, name := "  ", st.text.Render(fmt.Sprintf("%-26s", label))
		if i == m.cfgAt {
			marker, name = st.accent.Render("▶ "), st.bold.Render(fmt.Sprintf("%-26s", label))
			value = st.accent.Render(value)
		} else {
			value = st.text.Render(value)
		}
		return marker + name + value + "  " + st.dim.Render(hint)
	}
	words := strings.Join(rule.DrumWords, ", ")
	onOff := map[bool]string{true: "on", false: "off"}
	lines := []string{
		st.bold.Render("Buses by pad name"), "",
		row(cfgWords, "Drum words", ansi.Truncate(words, max(g.bodyW-60, 20), "…"), "Enter edits"),
		row(cfgDrumBus, "Pads with a drum word go to", busName(int32(rule.DrumBus)), "h l change"),
		row(cfgOtherBus, "Every other pad goes to", busName(int32(rule.OtherBus)), "h l change"),
		row(cfgOnAssign, "Set the bus when a file is assigned", onOff[rule.OnAssign], "Space toggles"),
		row(cfgReset, "Back to the defaults", "", "Enter"),
		"",
		st.dim.Render("B in Pads sets the bus of the pads of the bank (or the marked ones, or the whole project) by this rule;"),
		st.dim.Render("a word matches a word of the pad's name: Kick_125, OHat, DrumLoop, 14_Perc1 — and a longer one by its start."),
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

// busPreview is a line about a pad: its name, what the rule makes of it, and the bus it is on.
func (m Model) busPreview(rule app.BusRule, r app.PadRow, width int) string {
	st := m.st
	if !r.HasSample {
		return st.faint.Render(fmt.Sprintf("%-4s", r.Pad))
	}
	want, now := rule.BusFor(r.Name), r.Params["bus-fx"]
	kind := "other"
	if rule.IsDrum(r.Name) {
		kind = "drum"
	}
	state := st.dim.Render("on " + busName(now))
	if now != want {
		state = st.warn.Render("on " + busName(now) + " → " + busName(want))
	}
	name := ansi.Truncate(r.Name, max(width-34, 10), "…")
	return fmt.Sprintf("%-4s %-*s %s  %s", r.Pad, max(width-34, 10), name, st.dim.Render(fmt.Sprintf("%-5s", kind)), state)
}

// ---- scanning the pads ----

// busChanges are the pads of the bank (the marked ones among them, when some are marked; or the
// whole project) that are not on the bus their name says, each with that bus, and how many of
// the pads that were looked at are drums and not.
func (m Model) busChanges(all bool) (changes map[proto.PadIndex]int32, drums, others, right int) {
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
		if rule.IsDrum(r.Name) {
			drums++
		} else {
			others++
		}
		if r.Params["bus-fx"] == want {
			right++
			continue
		}
		changes[r.Pad] = want
	}
	return changes, drums, others, right
}

// askBusScan looks at the pads and asks before setting the bus of those that are not on the one
// their name says.
func (m Model) askBusScan(all bool) (tea.Model, tea.Cmd) {
	changes, drums, others, right := m.busChanges(all)
	if len(changes) == 0 {
		where := fmt.Sprintf("bank %c", 'A'+m.bank)
		if all {
			where = "the project"
		}
		m.prompt = nil
		m.message = fmt.Sprintf("every pad of %s with a sample is on the bus its name says (%d drums, %d others)", where, drums, others)
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
	_, drums, others, right := m.busChanges(p.scanAll)
	scope := fmt.Sprintf("bank %c", 'A'+m.bank)
	switch {
	case p.scanAll:
		scope = "the whole project"
	case len(m.marked) > 0:
		scope = fmt.Sprintf("the %d marked pads of bank %c", len(m.marked), 'A'+m.bank)
	}
	toDrum, toOther := 0, 0
	var examples []string
	pads := make([]proto.PadIndex, 0, len(p.buses))
	for pad := range p.buses {
		pads = append(pads, pad)
	}
	slices.SortFunc(pads, func(a, b proto.PadIndex) int { return int(a.Index()) - int(b.Index()) })
	for _, pad := range pads {
		if rule.IsDrum(m.snap.Pads[pad.Index()].Name) {
			toDrum++
		} else {
			toOther++
		}
		if len(examples) < 6 {
			examples = append(examples, fmt.Sprintf("%s %s → %s", pad, ansi.Truncate(m.snap.Pads[pad.Index()].Name, 22, "…"), busName(p.buses[pad])))
		}
	}
	lines := []string{
		st.bold.Render(fmt.Sprintf("Set the bus of %s by their names?", plural(len(p.buses), "pad"))),
		st.dim.Render(scope + fmt.Sprintf("  ·  %d drums, %d others, %d already right", drums, others, right)), "",
		st.text.Render(fmt.Sprintf("%d drum pads → %s", toDrum, busName(int32(rule.DrumBus))) + fmt.Sprintf("   %d others → %s", toOther, busName(int32(rule.OtherBus)))),
	}
	for _, e := range examples {
		lines = append(lines, st.dim.Render("  "+e))
	}
	if len(pads) > len(examples) {
		lines = append(lines, st.dim.Render(fmt.Sprintf("  … and %d more", len(pads)-len(examples))))
	}
	other := "the whole project"
	if p.scanAll {
		other = "this bank"
	}
	lines = append(lines, "", st.dim.Render("y apply   p "+other+" instead   n/Esc leave them   (the words and buses are in Config, 6)"))
	return st.modal.Render(strings.Join(lines, "\n"))
}
