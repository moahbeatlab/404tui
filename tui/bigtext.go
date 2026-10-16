package tui

import "strings"

// pixelFont is a 3x5 font of capitals and digits, '#' for a lit pixel. Drawn with half blocks two
// pixel rows to a terminal row, a text is three rows tall.
var pixelFont = map[rune][5]string{
	'A': {".#.", "#.#", "###", "#.#", "#.#"}, 'B': {"##.", "#.#", "##.", "#.#", "##."},
	'C': {".##", "#..", "#..", "#..", ".##"}, 'D': {"##.", "#.#", "#.#", "#.#", "##."},
	'E': {"###", "#..", "##.", "#..", "###"}, 'F': {"###", "#..", "##.", "#..", "#.."},
	'G': {".##", "#..", "#.#", "#.#", ".##"}, 'H': {"#.#", "#.#", "###", "#.#", "#.#"},
	'I': {"###", ".#.", ".#.", ".#.", "###"}, 'J': {"..#", "..#", "..#", "#.#", ".#."},
	'K': {"#.#", "#.#", "##.", "#.#", "#.#"}, 'L': {"#..", "#..", "#..", "#..", "###"},
	'M': {"#.#", "###", "###", "#.#", "#.#"}, 'N': {"##.", "#.#", "#.#", "#.#", "#.#"},
	'O': {".#.", "#.#", "#.#", "#.#", ".#."}, 'P': {"##.", "#.#", "##.", "#..", "#.."},
	'Q': {".#.", "#.#", "#.#", "###", ".##"}, 'R': {"##.", "#.#", "##.", "#.#", "#.#"},
	'S': {".##", "#..", ".#.", "..#", "##."}, 'T': {"###", ".#.", ".#.", ".#.", ".#."},
	'U': {"#.#", "#.#", "#.#", "#.#", "###"}, 'V': {"#.#", "#.#", "#.#", "#.#", ".#."},
	'W': {"#.#", "#.#", "###", "###", "#.#"}, 'X': {"#.#", "#.#", ".#.", "#.#", "#.#"},
	'Y': {"#.#", "#.#", ".#.", ".#.", ".#."}, 'Z': {"###", "..#", ".#.", "#..", "###"},
	'0': {"###", "#.#", "#.#", "#.#", "###"}, '1': {".#.", "##.", ".#.", ".#.", "###"},
	'2': {"##.", "..#", ".#.", "#..", "###"}, '3': {"##.", "..#", ".#.", "..#", "##."},
	'4': {"#.#", "#.#", "###", "..#", "..#"}, '5': {"###", "#..", "##.", "..#", "##."},
	'6': {".##", "#..", "###", "#.#", "###"}, '7': {"###", "..#", ".#.", ".#.", ".#."},
	'8': {"###", "#.#", "###", "#.#", "###"}, '9': {"###", "#.#", "###", "..#", "##."},
	'-': {"...", "...", "###", "...", "..."}, '_': {"...", "...", "...", "...", "###"},
	'.': {"...", "...", "...", "...", ".#."}, '+': {"...", ".#.", "###", ".#.", "..."},
	'#': {"#.#", "###", "#.#", "###", "#.#"}, '/': {"..#", "..#", ".#.", "#..", "#.."},
	' ': {"...", "...", "...", "...", "..."},
}

// bigWidth is how many cells wide a text is drawn at a scale (cells per pixel, 1 or 2).
func bigWidth(text string, scale int) int {
	n := len([]rune(text))
	if n == 0 {
		return 0
	}
	return n*(3*scale+scale) - scale
}

// bigText draws text in capitals three rows tall, each pixel scale cells wide. A character the
// font has not got is a space.
func bigText(text string, scale int) [3]string {
	var out [3]strings.Builder
	runes := []rune(strings.ToUpper(text))
	for i, r := range runes {
		g, ok := pixelFont[r]
		if !ok {
			g = pixelFont[' ']
		}
		for row := 0; row < 3; row++ {
			for col := 0; col < 3; col++ {
				top := g[2*row][col] == '#'
				bottom := 2*row+1 < 5 && g[2*row+1][col] == '#'
				c := " "
				switch {
				case top && bottom:
					c = "█"
				case top:
					c = "▀"
				case bottom:
					c = "▄"
				}
				out[row].WriteString(strings.Repeat(c, scale))
			}
			if i < len(runes)-1 {
				out[row].WriteString(strings.Repeat(" ", scale))
			}
		}
	}
	return [3]string{out[0].String(), out[1].String(), out[2].String()}
}
