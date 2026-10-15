package tui

import (
	"strings"
	"unicode"
)

// A prompt's buffer is edited at a cursor that sits back runes from the end of it, so a prompt
// opened with text already in it starts with the cursor at the end.

// edit handles the emacs-style keys of a one-line field and says whether it knew the key.
func (p *prompt) edit(name string) bool {
	r := []rune(p.buffer)
	at := len(r) - clamp(p.back, 0, len(r))
	switch name {
	case "left", "ctrl+b":
		at--
	case "right", "ctrl+f":
		at++
	case "home", "ctrl+a":
		at = 0
	case "end", "ctrl+e":
		at = len(r)
	case "alt+b", "alt+left", "ctrl+left":
		at = wordStart(r, at)
	case "alt+f", "alt+right", "ctrl+right":
		at = wordEnd(r, at)
	case "backspace", "ctrl+h":
		if at > 0 {
			r = append(r[:at-1], r[at:]...)
			at--
		}
	case "delete", "ctrl+d":
		if at < len(r) {
			r = append(r[:at], r[at+1:]...)
		}
	case "ctrl+u":
		r, at = r[at:], 0
	case "ctrl+k":
		r = r[:at]
	case "ctrl+w", "alt+backspace":
		from := wordStart(r, at)
		r = append(r[:from], r[at:]...)
		at = from
	case "alt+d":
		to := wordEnd(r, at)
		r = append(r[:at], r[to:]...)
	default:
		return false
	}
	at = clamp(at, 0, len(r))
	p.buffer, p.back = string(r), len(r)-at
	return true
}

// insert puts typed text at the cursor.
func (p *prompt) insert(s string) {
	r := []rune(p.buffer)
	at := len(r) - clamp(p.back, 0, len(r))
	p.buffer = string(r[:at]) + s + string(r[at:])
}

// shown is the buffer for the screen, with the cursor in it.
func (p *prompt) shown() string {
	r := []rune(p.buffer)
	at := len(r) - clamp(p.back, 0, len(r))
	if at == len(r) {
		return p.buffer + "█"
	}
	return string(r[:at]) + "\x1b[7m" + string(r[at]) + "\x1b[27m" + string(r[at+1:])
}

func wordStart(r []rune, at int) int {
	for at > 0 && !isWordRune(r[at-1]) {
		at--
	}
	for at > 0 && isWordRune(r[at-1]) {
		at--
	}
	return at
}

func wordEnd(r []rune, at int) int {
	for at < len(r) && !isWordRune(r[at]) {
		at++
	}
	for at < len(r) && isWordRune(r[at]) {
		at++
	}
	return at
}

func isWordRune(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c) || strings.ContainsRune("_", c)
}
