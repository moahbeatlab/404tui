package tui

// fxNames are the effects a bus of the SP-404MKII can hold, by the number the unit sends in CC 83:
// 0 is none, 1 to 5 are the direct effects (what sits on them is the unit's setting). Buses 1 and 2
// share the table.
var fxNames = []string{"(OFF)", "Direct FX1", "Direct FX2", "Direct FX3", "Direct FX4", "Direct FX5", "Scatter", "Downer",
	"Ha-Dou", "Ko-Da-Ma", "Zan-Zou", "To-Gu-Ro", "SBF", "Stopper", "Tape Echo", "TimeCtrlDly", "Super Filter",
	"WrmSaturator", "303 VinylSim", "404 VinylSim", "Cassette Sim", "Lo-fi", "Reverb", "Chorus", "JUNO Chorus",
	"Flanger", "Phaser", "Wah", "Slicer", "Tremolo/Pan", "Chromatic PS", "Hyper-Reso", "Ring Mod", "Crusher",
	"Overdrive", "Distortion", "Equalizer", "Compressor", "SX Reverb", "SX Delay", "Cloud Delay", "Back Spin",
	"DJFX Delay"}

// The MIDI of a bus's effect: bus 1 and bus 2 send on channel 1 and 2. CC 19 switches the effect
// (0 to 63 off, 64 and up on), CC 83 says which effect it is (an index of fxNames), CC 16 to 18 are
// CTRL 1 to 3.
const (
	ccFxSwitch = 19
	ccFxNumber = 83
)

// fxNames34 is the same for bus 3 and 4, which have another list (and not all effects of buses 1 and 2).
var fxNames34 = []string{"(OFF)", "303 VinylSim", "404 VinylSim", "Cassette Sim", "Lo-fi", "Downer", "Compressor", "Equalizer",
	"Isolator", "Super Filter", "Filter+Drive", "WrmSaturator", "Overdrive", "Distortion", "Crusher", "Ring Mod", "SBF",
	"Resonator", "Hyper-Reso", "Chromatic PS", "Reverb", "Ha-Dou", "Zan-Zou", "Sync Delay", "TimeCtrlDly", "Ko-Da-Ma",
	"Tape Echo", "Chorus", "JUNO Chorus", "Flanger", "Phaser", "Wah", "Slicer", "Tremolo/Pan", "To-Gu-Ro", "DJFX Looper",
	"Scatter", "SX Reverb", "SX Delay", "Cloud Delay", "DJFX Delay"}

// fxTable is the list of effects of bus 0 to 3.
func fxTable(bus int) []string {
	if bus >= 2 {
		return fxNames34
	}
	return fxNames
}

// fxNum is the number of an effect on a bus, -1 when the bus does not have it.
func fxNum(bus int, name string) int {
	for i, n := range fxTable(bus) {
		if n == name {
			return i
		}
	}
	return -1
}

// fxNameOf is the name of an effect number on a bus (0 to 3).
func fxNameOf(bus, num int) string {
	if t := fxTable(bus); num >= 0 && num < len(t) {
		return t[num]
	}
	return "?"
}

// fxName is the name of an effect number.
func fxName(num int) string {
	if num >= 0 && num < len(fxNames) {
		return fxNames[num]
	}
	return "?"
}
