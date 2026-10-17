package tui

import (
	"fmt"
	"strings"
)

// The map of the Faderfox EC4 for 404tui: one group of 16 encoders, in four rows of four, whose pushes
// send a note on the control channel (16 by default), 60 for the first and 75 for the last. Everything
// that can be done in Perform without the mouse is on it. The map is printed by sp404 midimap and
// described in docs/ec4.md.

// ec4FirstNote is the note of the first encoder's push.
const ec4FirstNote = 60

// ec4Entry is one push of the group: what it does, as a line for the cheat sheet.
type ec4Entry struct {
	name string
	// config is the index (0 to 3) of the message of Config this one is, or -1 when it is fixed.
	config int
}

// ec4Map is the group, in the order of the encoders (and so of the notes).
var ec4Map = [16]ec4Entry{
	{"record on / off", 1}, {"focus on / off", -1}, {"live mode (the Perform tab)", 0}, {"length: 1, 2, 3, 4, 8 bars", -1},
	{"BUS 1 motion slot 1", -1}, {"BUS 1 motion slot 2", -1}, {"BUS 1 motion slot 3", -1}, {"BUS 1 motion slot 4", -1},
	{"BUS 2 motion slot 1", -1}, {"BUS 2 motion slot 2", -1}, {"BUS 2 motion slot 3", -1}, {"BUS 2 motion slot 4", -1},
	{"clear BUS 1 motion", -1}, {"clear BUS 2 motion", -1}, {"next pattern becomes now", 2}, {"view: Stage / Levels", -1},
}

// ec4Action does the fixed pushes of the map (the others are the messages of Config, which
// midiEvent has handled by then); false for a note that is not one of them.
func (m Model) ec4Action(i int) (Model, bool) {
	if i < 0 || i >= len(ec4Map) || ec4Map[i].config >= 0 {
		return m, false
	}
	switch {
	case i == 1:
		nm, _ := m.isolateSwitch()
		return nm.(Model), true
	case i == 3:
		next := map[int]int{1: 2, 2: 3, 3: 4, 4: 8, 8: 1}[m.scenes().bars]
		if next == 0 {
			next = 2
		}
		return m.sceneAct("bars", next, 0), true
	case i >= 4 && i <= 11:
		return m.motionAct((i-4)/motionSlots, (i-4)%motionSlots), true
	case i == 15:
		return m.toggleLevels(), true
	case i == 12 || i == 13:
		return m.motionClear(i - 12), true
	}
	return m, false
}

// EC4Sheet is the cheat sheet of the map with its defaults (channel 16, the notes of Config as they
// are before they are changed there): what each encoder's push does, and the note it sends.
func EC4Sheet() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Faderfox EC4, one group named 404tui. The push of each encoder sends a note on channel %d.\n", defaultMidiChan)
	for i, e := range ec4Map {
		if i%4 == 0 {
			b.WriteString("\n")
		}
		note := ec4FirstNote + i
		if e.config >= 0 {
			_, n, _ := parseMidiMap(defaultMidiMap[e.config])
			note = n
		}
		fmt.Fprintf(&b, "  E%-2d  note %-3d  %s\n", i+1, note, e.name)
	}
	fmt.Fprintf(&b, "\nThe turn of each encoder (relative, 64 is none) sends CC %d to %d on the same channel: E1 is CC %d, E16 is CC %d.\n",
		ec4FirstCC, ec4FirstCC+15, ec4FirstCC, ec4FirstCC+15)
	b.WriteString("In the Levels view (E16) the turns are the volumes of pad 1 to 16 and the pushes E1 to E15 mute a pad; in Stage the turns do nothing.\n")
	return b.String()
}
