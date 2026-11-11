package core

import (
	"fmt"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The kit of the generator: which pad plays the kick, the snare, the hats... and which second pad takes over some of the hits (a long kick
// beside the kick, "CH 2" beside "CH"). What the names of the pads say is the start; the Generator kit menu changes any of it.

// genKit is what was chosen by hand, by voice: a pad number (0..159), or -1 for none. A voice that is not in the map is left to the names.
type genKit struct {
	pad, alt map[formats.Lane]int
}

// genVoices are the voices the generator plays, in the order the menu lists them.
var genVoices = []struct {
	lane formats.Lane
	name string
	alt  bool // it can have a second pad
}{
	{formats.LaneKick, "Kick", true},
	{formats.LaneSnare, "Snare", true},
	{formats.LaneClap, "Clap", true},
	{formats.LaneClosedHat, "Closed hat", true},
	{formats.LaneOpenHat, "Open hat", true},
	{formats.LaneRim, "Rim", false},
	{formats.LanePerc, "Percussion", true},
	{formats.LaneTom, "Tom", true},
	{formats.LaneShaker, "Shaker", true},
	{formats.LaneRide, "Ride", true},
}

func (a *App) padRows(bank int) []app.PadRow {
	var rows []app.PadRow
	for i := bank * 16; i < bank*16+16; i++ {
		if idx, ok := proto.NewPadIndex(uint16(i)); ok {
			rows = append(rows, app.PadRow{Pad: idx, HasSample: a.info.Pads[i].File, Name: a.info.Pads[i].Name})
		}
	}
	return rows
}

// generatorKit is the bank of the kit, the pad of each voice and the second pad of some: guessed from the names, then what was chosen.
func (a *App) generatorKit() (int, map[formats.Lane]proto.PadIndex, map[formats.Lane]proto.PadIndex) {
	bank, pads := a.kitBank()
	if bank < 0 {
		bank, pads = 0, map[formats.Lane]proto.PadIndex{}
	}
	for l, v := range a.gk.pad {
		if idx, ok := proto.NewPadIndex(uint16(v)); ok && v >= 0 {
			pads[l] = idx
		} else if v < 0 {
			delete(pads, l)
		}
	}
	alts := app.GuessAlternates(a.padRows(bank), pads)
	for l, v := range a.gk.alt {
		if idx, ok := proto.NewPadIndex(uint16(v)); ok && v >= 0 {
			alts[l] = idx
		} else if v < 0 {
			delete(alts, l)
		}
	}
	for l := range alts { // a second pad needs a first
		if _, ok := pads[l]; !ok {
			delete(alts, l)
		}
	}
	return bank, pads, alts
}

// genKitMenu is the Generator kit menu: every voice with its pad (left and right choose another), and a second pad for the voices that have one.
func (a *App) genKitMenu() {
	bank, _, _ := a.generatorKit()
	name := func(i int) string { return fmt.Sprintf("%s %s", a.label(i), short(a.info.Pads[i].Name, 18)) }
	padVals := func(auto string) []string {
		vals := []string{auto, "none"}
		for i := bank * 16; i < bank*16+16; i++ {
			vals = append(vals, name(i))
		}
		return vals
	}
	autoName := func(m map[formats.Lane]proto.PadIndex, l formats.Lane, none string) string {
		if p, ok := m[l]; ok {
			return "auto " + name(int(p.Index()))
		}
		return none
	}
	gb, gpads := a.kitBank()
	if gb < 0 {
		gpads = map[formats.Lane]proto.PadIndex{}
	}
	galts := app.GuessAlternates(a.padRows(bank), gpads)
	m := &menu{title: "generator kit: which pad plays what"}
	m.head("VOICES (the pads of bank " + string(rune('A'+bank)) + ")")
	if a.gk.pad == nil {
		a.gk = genKit{pad: map[formats.Lane]int{}, alt: map[formats.Lane]int{}}
	}
	cur := func(set map[formats.Lane]int, l formats.Lane) int {
		v, ok := set[l]
		switch {
		case !ok:
			return 0
		case v < 0:
			return 1
		}
		return 2 + v - bank*16
	}
	put := func(set map[formats.Lane]int, l formats.Lane, val int) {
		switch val {
		case 0:
			delete(set, l)
		case 1:
			set[l] = -1
		default:
			set[l] = bank*16 + val - 2
		}
	}
	for _, v := range genVoices {
		v := v
		m.items = append(m.items, menuItem{label: v.name, vals: padVals(autoName(gpads, v.lane, "auto: none")), val: max(cur(a.gk.pad, v.lane), 0),
			onChange: func(val int) { put(a.gk.pad, v.lane, val) }})
	}
	m.head("SECOND PADS: take over some hits")
	for _, v := range genVoices {
		v := v
		if !v.alt {
			continue
		}
		m.items = append(m.items, menuItem{label: v.name + " 2", vals: padVals(autoName(galts, v.lane, "auto: none")), val: max(cur(a.gk.alt, v.lane), 0),
			onChange: func(val int) { put(a.gk.alt, v.lane, val) }})
	}
	m.head("")
	m.items = append(m.items, menuItem{label: "Back to the names of the pads", run: func(int) {
		a.gk = genKit{}
		a.say("the generator takes its pads from their names again", false)
	}})
	a.menu = m
}
