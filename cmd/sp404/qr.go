package main

import (
	"os"
	"strings"

	"rsc.io/qr"
)

// qrText draws text as a QR code for a terminal: two rows of modules per line with half blocks, black on white with a
// quiet zone, so a phone camera reads it on dark and light terminals alike.
func qrText(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 2
	n := code.Size
	dark := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < n && y < n && code.Black(x, y)
	}
	var b strings.Builder
	for y := -quiet; y < n+quiet; y += 2 {
		b.WriteString("\x1b[30;47m")
		for x := -quiet; x < n+quiet; x++ {
			switch top, bottom := dark(x, y), dark(x, y+1); {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String(), nil
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
