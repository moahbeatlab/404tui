package core

import (
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
)

// Sample tools (from the PADS menu): what is done with the sample of a pad. The chord and the arp are in onepad.go; the keyboard
// puts pitched copies on a run of pads, so that the pads can be played, and sequenced, like keys.

func arpNames[T any](list []T, name func(T) string) []string {
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = name(v)
	}
	return out
}

var keyboardSizes = []int{4, 5, 7, 8, 12, 16}

// openKeyboardMenu spreads pitched copies of pad n's sample over a run of free pads.
func (a *App) openKeyboardMenu(n int) {
	scales := append([]string{"chromatic"}, arpNames(formats.ArpScales, func(s struct {
		Name string
		Iv   []int
	}) string {
		return s.Name
	})...)
	size, scale := 4, 0 // 8 keys, chromatic
	m := &menu{title: "keyboard from " + a.label(n)}
	m.items = append(m.items,
		menuItem{label: "Keys", vals: names(keyboardSizes, func(v int) string { return fmt.Sprintf("%d pads", v) }), val: size, keepOpen: true, onChange: func(v int) { size = v }},
		menuItem{label: "Scale", vals: scales, val: scale, keepOpen: true, onChange: func(v int) { scale = v }},
		menuItem{label: "Put them on free pads", run: func(int) { a.keyboard(n, keyboardSizes[size], scale-1) }},
	)
	a.menu = m
}

// keySemis is the pitch of key i in semitones from the sample, for a scale (-1: chromatic).
func keySemis(i, scale int) int {
	if scale < 0 || scale >= len(formats.ArpScales) {
		return i
	}
	iv := formats.ArpScales[scale].Iv
	return iv[i%len(iv)] + 12*(i/len(iv))
}

func (a *App) keyboard(n, keys, scale int) {
	id, ok := a.padSample(n)
	if !ok {
		a.say("this pad has no sample", true)
		return
	}
	first := a.freeRun(keys, n+1)
	if first < 0 {
		a.say(fmt.Sprintf("no room: %d empty pads in a row are needed", keys), true)
		return
	}
	base := fit(a.info.Pads[n].Name, 9)
	for i := 0; i < keys; i++ {
		semi := keySemis(i, scale)
		pid, err := a.lab.Pitched(id, semi)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		err = a.assignPad(first+i, pid, fmt.Sprintf("%s_%+d", base, semi))
		a.lab.Free(pid)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, first+i)
	}
	a.refresh()
	a.bank, a.cur = first/16, first%16
	a.say(fmt.Sprintf("%d keys on %s...%s", keys, a.label(first), a.label(first+keys-1)), false)
}
