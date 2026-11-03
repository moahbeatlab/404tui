package handheld

import "image/color"

// Gfx is what the screens draw with. The screens are laid out in cells and say what they want drawn (a filled block, a text,
// a bar, a waveform, a frame around a dialog, the selected row); a backend decides how that looks. The handheld's backend
// draws pixels (the frame is a picture); the terminal's draws characters in the style of the terminal program: flat tiles,
// an amber accent, rounded dialogs, braille waveforms. Positions are in cells; where a fraction is given (x of a width) it
// runs 0..1 over the width in cells.
type Gfx interface {
	// Fill paints a block of cells.
	Fill(col, row, cols, rows int, c color.RGBA)
	// Text writes s from a cell; bold for emphasis.
	Text(col, row int, s string, c color.RGBA, bold bool)
	// HRule is a thin line along the top of a row, VRule one along the left of a column; they leave letters alone.
	HRule(col, row, cols int, c color.RGBA)
	VRule(col, row, rows int, c color.RGBA)
	// Frame is the frame of a dialog, along the outer ring of cells of the rectangle.
	Frame(col, row, cols, rows int, c color.RGBA)
	// Highlight is the selected row (or block): what the cursor is on.
	Highlight(col, row, cols, rows int)
	// Cursor marks a tile as the one the cursor is on; it is drawn last, over the tile and its contents.
	Cursor(col, row, cols, rows int)
	// Bar is a bar of a row's width, filled from the left up to frac.
	Bar(col, row, cols int, frac float64, fg, track color.RGBA)
	// Fader is a setting: a bar from the left (or, bipolar, from where the default is) to frac, the default marked.
	Fader(col, row, cols int, frac, def float64, bipolar bool)
	// BarsPerCell is how many values a cell takes in Bars (a pixel backend has more than a text one).
	BarsPerCell() int
	// Bars draws h (0..1, cols*BarsPerCell() values) as columns: mirrored around the middle (a waveform) or from the bottom.
	Bars(col, row, cols, rows int, h []float64, mirrored bool, c color.RGBA)
	// Mark is a vertical line at x (0..1 of the width) over rows rows.
	Mark(col, row, cols, rows int, x float64, c color.RGBA)
	// Region paints the part x0..x1 of the width, rows rows high, as the selected part of a waveform.
	Region(col, row, cols, rows int, x0, x1 float64)
	// Badge is a switch's letter, two cells wide: lit when on.
	Badge(col, row int, letter string, on bool)
	// Piano is a keyboard of two octaves over cols cells and 3 rows, the semitones in lit coloured.
	Piano(col, row, cols int, lit map[int]bool)
	// StepCell is a step of the sequencer's grid; Note draws a note in it (start, length: 0..1 of the step; vel 0..1) and Tie
	// the part of a long note that runs on over the next step (frac of the step).
	StepCell(col, row, cols, rows int, bg color.RGBA)
	Note(col, row, cols, rows int, start, length, vel float64, c color.RGBA)
	Tie(col, row, cols, rows int, frac float64, c color.RGBA)
	// Logo is the name in big letters made of cells.
	Logo(col, row int, s string, c color.RGBA)
}
