package tui

import "github.com/charmbracelet/lipgloss"

// theme is one colour scheme. Every colour the UI uses comes from here.
type theme struct {
	name string
	// Text colours.
	text, dim, faint lipgloss.Color
	// accent marks what is in focus; hot is the pad that is sounding.
	accent, hot, good, warn, bad lipgloss.Color
	// Tile backgrounds.
	tile, tileEmpty, cursorBg, markedBg lipgloss.Color
	// cursorFg is the text on the cursor tile.
	cursorFg lipgloss.Color
	// bus1 and bus2 colour the pads that go to BUS 1 (blue) and BUS 2 (red).
	bus1, bus2 lipgloss.Color
	// cond marks the notes that have a condition: a colour nothing else in the grid has.
	cond lipgloss.Color
}

var themes = []theme{
	{
		name: "Hardware",
		text: "252", dim: "245", faint: "240",
		accent: "214", hot: "203", good: "114", warn: "221", bad: "203",
		tile: "237", tileEmpty: "235", cursorBg: "214", markedBg: "65", cursorFg: "232",
		bus1: "75", bus2: "196", cond: "177",
	},
	{
		name: "Ocean",
		text: "252", dim: "245", faint: "240",
		accent: "80", hot: "212", good: "114", warn: "221", bad: "203",
		tile: "236", tileEmpty: "234", cursorBg: "80", markedBg: "65", cursorFg: "232",
		bus1: "69", bus2: "196", cond: "215",
	},
	{
		name: "Paper",
		text: "236", dim: "243", faint: "250",
		accent: "25", hot: "160", good: "28", warn: "130", bad: "160",
		tile: "254", tileEmpty: "255", cursorBg: "25", markedBg: "151", cursorFg: "255",
		bus1: "27", bus2: "124", cond: "91",
	},
}

// styles are the lipgloss styles built from a theme.
type styles struct {
	th                                                   theme
	text, dim, faint, accent, hot, good, warn, bad, bold lipgloss.Style
	modal                                                lipgloss.Style
}

func newStyles(th theme) styles {
	fg := func(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return styles{
		th:     th,
		text:   fg(th.text),
		dim:    fg(th.dim),
		faint:  fg(th.faint),
		accent: fg(th.accent).Bold(true),
		hot:    fg(th.hot).Bold(true),
		good:   fg(th.good),
		warn:   fg(th.warn),
		bad:    fg(th.bad),
		bold:   lipgloss.NewStyle().Bold(true).Foreground(th.text),
		modal: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(th.accent).Padding(0, 1),
	}
}

// busColour is the colour of the pads that go to a bus: blue for BUS 1, red for BUS 2, none
// for the others.
func (th theme) busColour(bus int32) (lipgloss.Color, bool) {
	switch bus {
	case 1:
		return th.bus1, true
	case 2:
		return th.bus2, true
	}
	return "", false
}

// busBadge is the small mark for a bus, a shape that tells them apart without colour.
func busBadge(bus int32) string {
	switch bus {
	case 1:
		return "¹"
	case 2:
		return "²"
	}
	return ""
}
