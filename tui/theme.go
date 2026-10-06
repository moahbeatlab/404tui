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
}

var themes = []theme{
	{
		name: "Hardware",
		text: "252", dim: "245", faint: "240",
		accent: "214", hot: "203", good: "114", warn: "221", bad: "203",
		tile: "237", tileEmpty: "235", cursorBg: "214", markedBg: "65", cursorFg: "232",
	},
	{
		name: "Ocean",
		text: "252", dim: "245", faint: "240",
		accent: "80", hot: "212", good: "114", warn: "221", bad: "203",
		tile: "236", tileEmpty: "234", cursorBg: "80", markedBg: "65", cursorFg: "232",
	},
	{
		name: "Paper",
		text: "236", dim: "243", faint: "250",
		accent: "25", hot: "160", good: "28", warn: "130", bad: "160",
		tile: "254", tileEmpty: "255", cursorBg: "25", markedBg: "151", cursorFg: "255",
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
