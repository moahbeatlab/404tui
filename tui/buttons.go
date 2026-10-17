package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The Perform view is meant to be used with a mouse or a touch screen: what can be done is drawn as
// big buttons, three rows tall, and the places they were drawn at are kept for the clicks.

// btnH is the height of a button, in rows of the terminal.
const btnH = 3

// btn is a place of a panel that does something when it is clicked: its column, width and height,
// counted from the start of the panel's text, and the row where it starts.
type btn struct {
	row, col, w, h int
	act            string
	arg            int
}

// hitBtn is a button placed on the view, counted from the corner of its frame.
type hitBtn struct {
	x, y, w, h int
	act        string
	arg        int
}

// rowBuilder makes a band of blocks side by side, each btnH rows tall, and notes where the buttons
// are in it.
type rowBuilder struct {
	st    styles
	lines [btnH]strings.Builder
	w     int
	btns  []btn
}

// add puts a block of btnH lines of width w at the end of the band.
func (b *rowBuilder) add(block [btnH]string, w int) {
	for i := range block {
		b.lines[i].WriteString(block[i])
	}
	b.w += w
}

// gap leaves n columns empty.
func (b *rowBuilder) gap(n int) {
	b.add([btnH]string{strings.Repeat(" ", n), strings.Repeat(" ", n), strings.Repeat(" ", n)}, n)
}

// text puts a label in the middle row of the band.
func (b *rowBuilder) text(s string) {
	w := ansi.StringWidth(s)
	b.add([btnH]string{strings.Repeat(" ", w), s, strings.Repeat(" ", w)}, w)
}

// block draws a filled block with a centred label, in the colours given.
func (b *rowBuilder) block(label string, w int, style lipgloss.Style) [btnH]string {
	w = max(w, ansi.StringWidth(label)+2)
	empty := style.Render(strings.Repeat(" ", w))
	left := (w - ansi.StringWidth(label)) / 2
	mid := style.Render(strings.Repeat(" ", left) + label + strings.Repeat(" ", w-left-ansi.StringWidth(label)))
	return [btnH]string{empty, mid, empty}
}

// button draws a label as a big button, lit when on, at least minW wide.
func (b *rowBuilder) button(label string, on bool, act string, arg, minW int) {
	th := b.st.th
	style := lipgloss.NewStyle().Background(th.tile).Foreground(th.text).Bold(true)
	if on {
		style = lipgloss.NewStyle().Background(th.accent).Foreground(th.cursorFg).Bold(true)
	}
	w := max(ansi.StringWidth(label)+6, minW)
	b.btns = append(b.btns, btn{col: b.w, w: w, h: btnH, act: act, arg: arg})
	b.add(b.block(label, w, style), w)
}

// bar draws a bar w wide that is filled from the left to frac, and is a button: a tap sets where.
func (b *rowBuilder) bar(w int, frac float64, colour lipgloss.Color, act string, arg int) {
	filled := int(frac*float64(w) + 0.5)
	fill := lipgloss.NewStyle().Foreground(colour).Render(strings.Repeat("█", filled))
	rest := b.st.faint.Render(strings.Repeat("░", w-filled))
	line := fill + rest
	b.btns = append(b.btns, btn{col: b.w, w: w, h: btnH, act: act, arg: arg})
	b.add([btnH]string{line, line, line}, w)
}

// toggle draws a switch: off it is a dark pill with the knob on the left, on it is lit with the knob
// on the right; the label stands beside it.
func (b *rowBuilder) toggle(on bool, label, act string, arg int) {
	th := b.st.th
	var style lipgloss.Style
	var mid string
	if on {
		style = lipgloss.NewStyle().Background(th.good).Foreground(th.cursorFg).Bold(true)
		mid = "      ON  ●"
	} else {
		style = lipgloss.NewStyle().Background(th.tile).Foreground(th.faint).Bold(true)
		mid = "●  off      "
	}
	w := ansi.StringWidth(mid)
	blank := style.Render(strings.Repeat(" ", w))
	text := b.st.bold.Render("  " + label)
	total := w + ansi.StringWidth(text)
	b.btns = append(b.btns, btn{col: b.w, w: total, h: btnH, act: act, arg: arg})
	b.add([btnH]string{blank + strings.Repeat(" ", ansi.StringWidth(text)), style.Render(mid) + text, blank + strings.Repeat(" ", ansi.StringWidth(text))}, total)
}

// rows is the band as lines, with its buttons put on the row they start at.
func (b *rowBuilder) rows(first int) ([]string, []btn) {
	out := make([]string, btnH)
	for i := range out {
		out[i] = b.lines[i].String()
	}
	btns := make([]btn, len(b.btns))
	for i, x := range b.btns {
		x.row = first
		btns[i] = x
	}
	return out, btns
}
