package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The Perform view is meant to be used with a mouse or a touch screen: what can be done is drawn as
// buttons, and the places they were drawn at are kept for the clicks.

// btn is a place of a panel's line that does something when it is clicked: its column and width
// counted from the start of the line, and the row of the line in the panel.
type btn struct {
	row, col, w int
	act         string
	arg         int
}

// hitBtn is a button placed on the view, counted from the corner of its frame.
type hitBtn struct {
	x, y, w int
	act     string
	arg     int
}

// lineBuilder makes a line of text and buttons and notes where each button is.
type lineBuilder struct {
	st   styles
	sb   strings.Builder
	w    int
	btns []btn
}

func (b *lineBuilder) text(s string) {
	b.sb.WriteString(s)
	b.w += ansi.StringWidth(s)
}

func (b *lineBuilder) String() string { return b.sb.String() }

// button draws a label as a button, lit when on.
func (b *lineBuilder) button(label string, on bool, act string, arg int) {
	th := b.st.th
	style := lipgloss.NewStyle().Background(th.tile).Foreground(th.text)
	if on {
		style = lipgloss.NewStyle().Background(th.accent).Foreground(th.cursorFg).Bold(true)
	}
	cell := " " + label + " "
	b.btns = append(b.btns, btn{col: b.w, w: ansi.StringWidth(cell), act: act, arg: arg})
	b.text(style.Render(cell))
}

// region puts already drawn text of width w on the line as a button (a bar to click on).
func (b *lineBuilder) region(s string, w int, act string, arg int) {
	b.btns = append(b.btns, btn{col: b.w, w: w, act: act, arg: arg})
	b.text(s)
}

// toggle draws a switch: off it is a dark pill with the knob on the left, on it is lit with the knob
// on the right.
func (b *lineBuilder) toggle(on bool, label, act string, arg int) {
	th := b.st.th
	var pill string
	if on {
		pill = lipgloss.NewStyle().Background(th.good).Foreground(th.cursorFg).Bold(true).Render("   ON ●")
	} else {
		pill = lipgloss.NewStyle().Background(th.tile).Foreground(th.faint).Render("● off   ")
	}
	w := ansi.StringWidth(pill) + 1 + ansi.StringWidth(label)
	b.btns = append(b.btns, btn{col: b.w, w: w, act: act, arg: arg})
	b.text(pill + " " + b.st.text.Render(label))
}
