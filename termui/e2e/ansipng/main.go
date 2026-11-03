// Command ansipng turns the text of a terminal capture (tmux capture-pane -e -p), with its colour codes, into a PNG of
// what the terminal showed.
//
//	ansipng in.ansi out.png
package main

import (
	"bufio"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"

	"github.com/moahbeatlab/404tui/handheld"
	"github.com/moahbeatlab/404tui/termui"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: ansipng in.ansi out.png")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var cells [][]termui.Cell
	fg, bg := color.RGBA{217, 223, 230, 255}, color.RGBA{11, 13, 16, 255}
	bold := false
	for sc.Scan() {
		var row []termui.Cell
		line := []rune(sc.Text())
		for i := 0; i < len(line); i++ {
			if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
				j := i + 2
				for j < len(line) && !(line[j] >= '@' && line[j] <= '~') {
					j++
				}
				if j < len(line) && line[j] == 'm' {
					fg, bg, bold = sgr(string(line[i+2:j]), fg, bg, bold)
				}
				i = j
				continue
			}
			row = append(row, termui.Cell{R: line[i], Fg: fg, Bg: bg, Bold: bold})
		}
		cells = append(cells, row)
	}
	w := 0
	for _, r := range cells {
		w = max(w, len(r))
	}
	for i := range cells { // short lines are filled with the ground
		for len(cells[i]) < w {
			cells[i] = append(cells[i], termui.Cell{R: ' ', Fg: fg, Bg: bg})
		}
	}
	if err := handheld.WritePNG(os.Args[2], termui.Image(cells)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// sgr applies a colour code: 0 reset, 1 bold, 22 not bold, 38;2;r;g;b and 48;2;r;g;b, 38;5;n and 48;5;n.
func sgr(s string, fg, bg color.RGBA, bold bool) (color.RGBA, color.RGBA, bool) {
	p := strings.Split(s, ";")
	n := func(i int) int { v, _ := strconv.Atoi(p[i]); return v }
	for i := 0; i < len(p); i++ {
		switch n(i) {
		case 0:
			fg, bg, bold = color.RGBA{217, 223, 230, 255}, color.RGBA{11, 13, 16, 255}, false
		case 1:
			bold = true
		case 22:
			bold = false
		case 38, 48:
			code := n(i)
			var c color.RGBA
			ok := false
			switch {
			case i+4 < len(p) && n(i+1) == 2:
				c, ok = color.RGBA{uint8(n(i + 2)), uint8(n(i + 3)), uint8(n(i + 4)), 255}, true
				i += 4
			case i+2 < len(p) && n(i+1) == 5:
				c, ok = xterm(n(i+2)), true
				i += 2
			}
			if ok && code == 38 {
				fg = c
			} else if ok {
				bg = c
			}
		}
	}
	return fg, bg, bold
}

func xterm(n int) color.RGBA {
	switch {
	case n >= 232:
		v := uint8(8 + 10*(n-232))
		return color.RGBA{v, v, v, 255}
	case n >= 16:
		n -= 16
		lv := []uint8{0, 95, 135, 175, 215, 255}
		return color.RGBA{lv[n/36], lv[n/6%6], lv[n%6], 255}
	}
	return color.RGBA{128, 128, 128, 255}
}
