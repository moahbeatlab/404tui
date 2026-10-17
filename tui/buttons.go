package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The Perform view is meant to be used with a mouse or a touch screen: what can be done is drawn as
// big buttons, as tall as the screen allows (3, 5 or 7 rows), and the places they were drawn at are kept
// for the clicks.

// btnRows is how tall the buttons are for a sidebar h rows high: the other panels need about 41 rows
// and the buttons use 5 times their height, so a taller screen makes bigger buttons.
func btnRows(h int) int {
	n := clamp((h-41)/5, 3, 7)
	if n%2 == 0 {
		n-- // an odd height has a middle row for the label
	}
	return n
}

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

// rowBuilder makes a band of blocks side by side, each n rows tall, and notes where the buttons are
// in it.
type rowBuilder struct {
	st    styles
	n     int
	lines []strings.Builder
	w     int
	btns  []btn
}

func newRowBuilder(st styles, n int) *rowBuilder {
	return &rowBuilder{st: st, n: n, lines: make([]strings.Builder, n)}
}

// add puts a block of n lines of width w at the end of the band.
func (b *rowBuilder) add(block []string, w int) {
	for i := range block {
		b.lines[i].WriteString(block[i])
	}
	b.w += w
}

// filled is a block of n copies of a line, with the middle one replaced when mid is not empty.
func (b *rowBuilder) filled(line, mid string) []string {
	blk := make([]string, b.n)
	for i := range blk {
		blk[i] = line
	}
	if mid != "" {
		blk[b.n/2] = mid
	}
	return blk
}

// gap leaves n columns empty.
func (b *rowBuilder) gap(n int) {
	b.add(b.filled(strings.Repeat(" ", n), ""), n)
}

// text puts a label in the middle row of the band.
func (b *rowBuilder) text(s string) {
	w := ansi.StringWidth(s)
	b.add(b.filled(strings.Repeat(" ", w), s), w)
}

// button draws a label as a big button, lit when on, at least minW wide.
func (b *rowBuilder) button(label string, on bool, act string, arg, minW int) {
	th := b.st.th
	style := lipgloss.NewStyle().Background(th.tile).Foreground(th.text).Bold(true)
	if on {
		style = lipgloss.NewStyle().Background(th.accent).Foreground(th.cursorFg).Bold(true)
	}
	w := max(ansi.StringWidth(label)+6, minW)
	left := (w - ansi.StringWidth(label)) / 2
	empty := style.Render(strings.Repeat(" ", w))
	mid := style.Render(strings.Repeat(" ", left) + label + strings.Repeat(" ", w-left-ansi.StringWidth(label)))
	b.btns = append(b.btns, btn{col: b.w, w: w, h: b.n, act: act, arg: arg})
	b.add(b.filled(empty, mid), w)
}

// bar draws a bar w wide that is filled from the left to frac, and is a button: a tap sets where.
func (b *rowBuilder) bar(w int, frac float64, colour lipgloss.Color, act string, arg int) {
	filled := clamp(int(frac*float64(w)+0.5), 0, w)
	line := lipgloss.NewStyle().Foreground(colour).Render(strings.Repeat("█", filled)) + b.st.faint.Render(strings.Repeat("░", w-filled))
	b.btns = append(b.btns, btn{col: b.w, w: w, h: b.n, act: act, arg: arg})
	b.add(b.filled(line, ""), w)
}

// toggle draws a switch: off it is a dark pill with the knob on the left, on it is lit with the knob
// on the right; the label stands beside it.
func (b *rowBuilder) toggle(on bool, label, act string, arg int) {
	th := b.st.th
	var style lipgloss.Style
	var mid string
	if on {
		style = lipgloss.NewStyle().Background(th.good).Foreground(th.cursorFg).Bold(true)
		mid = "          ON  ●"
	} else {
		style = lipgloss.NewStyle().Background(th.tile).Foreground(th.faint).Bold(true)
		mid = "●  off          "
	}
	w := ansi.StringWidth(mid)
	text := b.st.bold.Render("  " + label)
	tw := ansi.StringWidth(text)
	total := w + tw
	blank := style.Render(strings.Repeat(" ", w)) + strings.Repeat(" ", tw)
	b.btns = append(b.btns, btn{col: b.w, w: total, h: b.n, act: act, arg: arg})
	b.add(b.filled(blank, style.Render(mid)+text), total)
}

// rows is the band as lines, with its buttons put on the row they start at.
func (b *rowBuilder) rows(first int) ([]string, []btn) {
	out := make([]string, b.n)
	for i := range out {
		out[i] = b.lines[i].String()
	}
	btns := make([]btn, len(b.btns))
	for i, x := range b.btns {
		x.row += first
		btns[i] = x
	}
	return out, btns
}

// btnSpec is a button of two lines: a label and a line of detail under it.
type btnSpec struct {
	label, detail string
	on            bool
	act           string
	arg           int
}

// twoLine draws a button rows tall with its label and, under it, a line of detail.
func (b *rowBuilder) twoLine(rows, w int, sp btnSpec) []string {
	th := b.st.th
	style := lipgloss.NewStyle().Background(th.tile).Foreground(th.text).Bold(true)
	faint := lipgloss.NewStyle().Background(th.tile).Foreground(th.faint)
	if sp.on {
		style = lipgloss.NewStyle().Background(th.accent).Foreground(th.cursorFg).Bold(true)
		faint = lipgloss.NewStyle().Background(th.accent).Foreground(th.cursorFg)
	}
	centre := func(st lipgloss.Style, text string) string {
		text = ansi.Truncate(text, w, "…")
		left := (w - ansi.StringWidth(text)) / 2
		return st.Render(strings.Repeat(" ", left) + text + strings.Repeat(" ", w-left-ansi.StringWidth(text)))
	}
	out := make([]string, rows)
	for i := range out {
		out[i] = style.Render(strings.Repeat(" ", w))
	}
	if rows < 2 || sp.detail == "" {
		out[(rows-1)/2] = centre(style, sp.label)
		return out
	}
	at := (rows - 2) / 2
	out[at] = centre(style, sp.label)
	out[at+1] = centre(faint, sp.detail)
	return out
}

// stack puts two two-line buttons one above the other in a column w wide: each is half the height of
// the band.
func (b *rowBuilder) stack(w int, top, bottom btnSpec) {
	half := b.n / 2
	blk := append(b.twoLine(half, w, top), b.twoLine(b.n-half, w, bottom)...)
	b.btns = append(b.btns,
		btn{col: b.w, w: w, h: half, act: top.act, arg: top.arg},
		btn{row: half, col: b.w, w: w, h: b.n - half, act: bottom.act, arg: bottom.arg})
	b.add(blk, w)
}

// row puts a run of two-line buttons side by side, each w wide, with a gap of one between.
func (b *rowBuilder) row(w int, specs ...btnSpec) {
	for i, sp := range specs {
		if i > 0 {
			b.gap(1)
		}
		b.btns = append(b.btns, btn{col: b.w, w: w, h: b.n, act: sp.act, arg: sp.arg})
		b.add(b.twoLine(b.n, w, sp), w)
	}
}

// motionRows is how tall the buttons of the MOTION panel are for a sidebar h rows high: they take what
// the other panels leave, in odd numbers so that a label has a middle row.
func motionRows(h int) int {
	n := clamp((h-33)/3, 3, 11)
	if n%2 == 0 {
		n--
	}
	return n
}

// stackN puts buttons one above the other in a column w wide, each about the same height; their labels
// are in the middle of their part.
func (b *rowBuilder) stackN(w int, specs ...btnSpec) {
	var blk []string
	row := 0
	for i, sp := range specs {
		h := b.n/len(specs) + b2i(i < b.n%len(specs))
		blk = append(blk, b.twoLine(h, w, sp)...)
		b.btns = append(b.btns, btn{row: row, col: b.w, w: w, h: h, act: sp.act, arg: sp.arg})
		row += h
	}
	b.add(blk, w)
}
