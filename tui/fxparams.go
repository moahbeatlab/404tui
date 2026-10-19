// The names and ranges of the six knobs of each effect, to show their values as the unit does: the parameter lists of the
// SP-404MKII reference manual, as 404jam has them.

package tui

import (
	"fmt"
	"math"
	"strconv"
)

var fxParams = map[string][6]fxParam{
	"303 VinylSim": {{name: "COMP", kind: "c", lo: 0.0, hi: 100.0}, {name: "NOISE", kind: "c", lo: 0.0, hi: 100.0}, {name: "WOW FLUT", kind: "c", lo: 0.0, hi: 100.0}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {}, {}},
	"404 VinylSim": {{name: "FREQUENCY", kind: "c", lo: 0.0, hi: 100.0}, {name: "NOISE", kind: "c", lo: 0.0, hi: 100.0}, {name: "WOW FLUT", kind: "c", lo: 0.0, hi: 100.0}, {}, {}, {}},
	"Auto Pitch":   {{name: "PITCH", kind: "c", lo: -100.0, hi: 100.0}, {name: "FORMANT", kind: "c", lo: -100.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "AT PITCH", kind: "c", lo: 0.0, hi: 100.0}, {name: "KEY", kind: "e", opts: []string{"CHROMA", "A", "Bb", "B", "C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab"}}, {name: "ROBOT", kind: "b"}},
	"Back Spin":    {{name: "LENGTH", kind: "e", opts: []string{"1/1", "1/2", "1/4", "1/8", "1/16"}}, {name: "SPEED", kind: "c", lo: 0.0, hi: 100.0}, {name: "BACK SW", kind: "b"}, {}, {}, {}},
	"Cassette Sim": {{name: "TONE", kind: "c", lo: 0.0, hi: 100.0}, {name: "HISS", kind: "c", lo: 0.0, hi: 100.0}, {name: "AGE", kind: "c", lo: 0.0, hi: 60.0, unit: "J"}, {name: "DRIVE", kind: "c", lo: 0.0, hi: 100.0}, {name: "WOW FLUT", kind: "c", lo: 0.0, hi: 100.0}, {name: "Catch", kind: "c", lo: 0.0, hi: 100.0}},
	"Chorus":       {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.33, hi: 2.3, unit: "sec"}, {name: "BALANCE", kind: "bal"}, {name: "EQ LOW", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}, {name: "EQ HIGH", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}},
	"Chromatic PS": {{name: "PITCH1", kind: "c", lo: -24.0, hi: 12.0, unit: "st"}, {name: "PITCH2", kind: "c", lo: -24.0, hi: 12.0, unit: "st"}, {name: "BALANCE", kind: "bal"}, {name: "PAN1", kind: "pan"}, {name: "PAN2", kind: "pan"}, {}},
	"Cloud Delay":  {{name: "WINDOW", kind: "c", lo: 0.0, hi: 100.0}, {name: "PITCH", kind: "c", lo: -12.0, hi: 12.0}, {name: "BALANCE", kind: "bal"}, {name: "FEEDBACK", kind: "c", lo: -100.0, hi: 100.0}, {name: "CLOUDY", kind: "c", lo: 0.0, hi: 100.0}, {name: "LOFI", kind: "b"}},
	"Compressor":   {{name: "SUSTAIN", kind: "c", lo: 0.0, hi: 100.0}, {name: "ATTACK", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATIO", kind: "c", lo: 0.0, hi: 100.0}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {}, {}},
	"Crusher":      {{name: "FILTER", kind: "c", lo: 331.0, hi: 15392.0, unit: "Hz"}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {}, {}, {}},
	"DJFX Delay":   {{name: "LENGTH", kind: "c", lo: 0.23, hi: 0.012, unit: "sec"}, {name: "TIME", kind: "c", lo: 0.0, hi: 100.0}, {name: "LOOP SW", kind: "b"}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "SYNC", kind: "b"}},
	"DJFX Looper":  {{name: "LENGTH", kind: "c", lo: 0.23, hi: 0.012, unit: "sec"}, {name: "SPEED", kind: "c", lo: -100.0, hi: 100.0}, {name: "LOOP SW", kind: "b"}, {}, {}, {}},
	"Direct FX":    {{name: "CTRL 1", kind: "c", lo: 0.0, hi: 127.0}, {name: "CTRL 2", kind: "c", lo: 0.0, hi: 127.0}, {name: "CTRL 3", kind: "c", lo: 0.0, hi: 127.0}, {name: "CTRL 4", kind: "c", lo: 0.0, hi: 127.0}, {name: "CTRL 5", kind: "c", lo: 0.0, hi: 127.0}, {name: "CTRL 6", kind: "c", lo: 0.0, hi: 127.0}},
	"Distortion":   {{name: "DRIVE", kind: "c", lo: 0.0, hi: 100.0}, {name: "TONE", kind: "c", lo: -100.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {}, {}},
	"Downer":       {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "e", opts: []string{"2/1", "1/1", "1/2", "1/4", "1/8", "1/16", "1/32"}}, {name: "FILTER", kind: "c", lo: 0.0, hi: 100.0}, {name: "PITCH", kind: "b"}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {}},
	"Equalizer":    {{name: "LOW GAIN", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}, {name: "MID GAIN", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}, {name: "HIGH GAIN", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}, {name: "LOW FREQ", kind: "e", opts: []string{"20", "25", "31", "40", "50", "63", "80", "100", "125", "160", "200", "250", "315", "400"}}, {name: "MID FREQ", kind: "e", opts: []string{"200", "250", "315", "400", "500", "630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000"}}, {name: "HIGH FREQ", kind: "e", opts: []string{"2000", "2500", "3150", "4000", "5000", "6300", "8000", "10000", "12500", "16000"}}},
	"Filter+Drive": {{name: "CUTOFF", kind: "c", lo: 20.0, hi: 16000.0, unit: "Hz"}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {name: "DRIVE", kind: "c", lo: 0.0, hi: 100.0}, {name: "FLT TYPE", kind: "e", opts: []string{"HPF", "LPF"}}, {name: "LOW FREQ", kind: "c", lo: 20.0, hi: 16000.0, unit: "Hz"}, {name: "LOW GAIN", kind: "c", lo: -24.0, hi: 24.0, unit: "dB"}},
	"Flanger":      {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "MANUAL", kind: "c", lo: 0.0, hi: 100.0}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "SYNC", kind: "b"}},
	"Gt Amp Sim":   {{name: "AMP TYPE", kind: "e", opts: []string{"JC", "TWIN", "BG", "MATCH", "MS", "SLDN"}}, {name: "DRIVE", kind: "c", lo: 0.0, hi: 100.0}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "BASS", kind: "c", lo: -100.0, hi: 100.0}, {name: "MIDDLE", kind: "c", lo: -100.0, hi: 100.0}, {name: "TREBLE", kind: "c", lo: -100.0, hi: 100.0}},
	"Ha-Dou":       {{name: "MOD DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "TIME", kind: "c", lo: 0.0, hi: 100.0}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "LOW CUT", kind: "e", opts: []string{"FLAT", "20", "25", "31", "40", "50", "63", "80", "100", "125", "160", "200", "250", "315", "400", "500", "630", "800"}}, {name: "HIGH CUT", kind: "e", opts: []string{"630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000", "10000", "12500", "FLAT"}}, {name: "PRE DELAY", kind: "c", lo: 0.0, hi: 100.0, unit: "msec"}},
	"Harmony":      {{name: "PITCH", kind: "c", lo: -100.0, hi: 100.0}, {name: "FORMANT", kind: "c", lo: -100.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "AT PITCH", kind: "c", lo: 0.0, hi: 100.0}, {name: "KEY", kind: "e", opts: []string{"CHROMA", "A", "Bb", "B", "C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab"}}, {name: "HARMONY", kind: "e", opts: []string{"Root", "P5", "Oct", "UpDn", "UpDnP5", "3rd", "5thUp", "5thDn", "7thUp", "7thDn"}}},
	"Hyper-Reso":   {{name: "NOTE", kind: "e", opts: []string{"-17", "-16", "-15", "-14", "-13", "-12", "-11", "-10", "-9", "-8", "-7", "-6", "-5", "-4", "-3", "-2", "-1", "+1", "+2", "+3", "+4", "+5", "+6", "+7", "+8", "+9", "+10", "+11", "+12", "+13", "+14", "+15", "+16", "+17", "+18"}}, {name: "SPREAD", kind: "e", opts: []string{"UNISON", "TINY", "SMALL", "MEDIUM", "HUGE"}}, {name: "CHARACTER", kind: "c", lo: 0.0, hi: 100.0}, {name: "SCALE", kind: "e", opts: []string{"C Maj", "Db Maj", "D Maj", "Eb Maj", "E Maj", "F Maj", "Gb Maj", "G Maj", "Ab Maj", "A Maj", "Bb Maj", "B Maj", "C min", "Db min", "D min", "Eb min", "E min", "F min", "Gb min", "G min", "Ab min", "A min", "Bb min", "B min"}}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "ENV MOD", kind: "c", lo: 0.0, hi: 100.0}},
	"Isolator":     {{name: "LOW", kind: "c", lo: -42.0, hi: 12.0, unit: "dB", inf: true}, {name: "MID", kind: "c", lo: -42.0, hi: 12.0, unit: "dB", inf: true}, {name: "HIGH", kind: "c", lo: -42.0, hi: 12.0, unit: "dB", inf: true}, {}, {}, {}},
	"JUNO Chorus":  {{name: "MODE", kind: "e", opts: []string{"JUNO 1", "JUNO 2", "JUNO12", "JX-1 1", "JX-1 2"}}, {name: "NOISE", kind: "c", lo: 0.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {}, {}, {}},
	"Ko-Da-Ma":     {{name: "TIME", kind: "e", opts: []string{"1/32", "1/16T", "1/32D", "1/16", "1/8T", "1/16D", "1/8", "1/4T", "1/8D", "1/4", "1/2T", "1/4D", "1/2", "1/1T", "1/2D", "1/1"}}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "SEND", kind: "c", lo: 0.0, hi: 100.0}, {name: "L DAMP F", kind: "e", opts: []string{"FLAT", "80", "100", "125", "160", "200", "250", "315", "400", "500", "630", "800"}}, {name: "H DAMP F", kind: "e", opts: []string{"630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000", "10000", "12500", "FLAT"}}, {name: "MODE", kind: "e", opts: []string{"SINGLE", "PAN"}}},
	"Lo-fi":        {{name: "PRE FILT", kind: "e", opts: []string{"1", "2", "3", "4", "5", "6"}}, {name: "LOFI TYPE", kind: "e", opts: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}}, {name: "TONE", kind: "c", lo: -100.0, hi: 100.0}, {name: "CUTOFF", kind: "e", opts: []string{"200", "250", "315", "400", "500", "630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000"}}, {name: "BALANCE", kind: "bal"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}},
	"Overdrive":    {{name: "DRIVE", kind: "c", lo: 0.0, hi: 100.0}, {name: "TONE", kind: "c", lo: -100.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {}, {}},
	"Phaser":       {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "MANUAL", kind: "c", lo: 0.0, hi: 100.0}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "SYNC", kind: "b"}},
	"Resonator":    {{name: "ROOT", kind: "nt", lo: 0.0, hi: 127.0}, {name: "BRIGHT", kind: "c", lo: 0.0, hi: 100.0}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "CHORD", kind: "e", opts: []string{"Root", "Oct", "UpDn", "P5", "m3", "m5", "m7", "m7oct", "m0", "m11", "M3", "M5", "M7", "M7oct", "M9", "M11"}}, {name: "PANNING", kind: "c", lo: 0.0, hi: 100.0}, {name: "ENV MOD", kind: "c", lo: 0.0, hi: 100.0}},
	"Reverb":       {{name: "TYPE", kind: "e", opts: []string{"AMBI", "ROOM", "HALL1", "HALL2"}}, {name: "TIME", kind: "c", lo: 0.0, hi: 100.0}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "LOW CUT", kind: "e", opts: []string{"FLAT", "20", "25", "31", "40", "50", "63", "80", "100", "125", "160", "200", "250", "315", "400", "500", "630", "800"}}, {name: "HIGH CUT", kind: "e", opts: []string{"630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000", "10000", "12500", "FLAT"}}, {name: "PRE DELAY", kind: "c", lo: 0.0, hi: 100.0, unit: "ms"}},
	"Ring Mod":     {{name: "FREQUENCY", kind: "c", lo: 0.0, hi: 100.0}, {name: "SENS", kind: "c", lo: 0.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "POLARITY", kind: "b"}, {name: "EQ LOW", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}, {name: "EQ HIGH", kind: "c", lo: -15.0, hi: 15.0, unit: "dB"}},
	"SBF":          {{name: "INTERVAL", kind: "c", lo: 0.0, hi: 100.0}, {name: "WIDTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {name: "TYPE", kind: "e", opts: []string{"SBF1", "SBF2", "SBF3", "SBF4", "SBF5", "SBF6"}}, {name: "GAIN", kind: "c", lo: -52.3, hi: 10.0, unit: "dB", inf: true}, {}},
	"SX Delay":     {{name: "TIME", kind: "e", opts: []string{"1/32", "1/16T", "1/32D", "1/16", "1/8T", "1/16D", "1/8", "1/4T", "1/8D", "1/4", "1/2T", "1/4D", "1/2", "1/1T", "1/2D", "1/1"}}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "BALANCE", kind: "bal"}, {}, {}, {}},
	"SX Reverb":    {{name: "TIME", kind: "c", lo: 0.0, hi: 100.0}, {name: "TONE", kind: "c", lo: -100.0, hi: 100.0}, {name: "BALANCE", kind: "bal"}, {}, {}, {}},
	"Scatter":      {{name: "TYPE", kind: "e", opts: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}}, {name: "DEPTH", kind: "e", opts: []string{"10", "20", "30", "40", "50", "60", "70", "80", "90", "100"}}, {name: "SCATTER", kind: "b"}, {name: "SPEED", kind: "e", opts: []string{"SINGLE", "DOUBLE"}}, {}, {}},
	"Slicer":       {{name: "PATTERN", kind: "c", lo: 1.0, hi: 32.0}, {name: "SPEED", kind: "c", lo: 0.0, hi: 100.0}, {name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "SHUFFLE", kind: "c", lo: 0.0, hi: 100.0}, {name: "MODE", kind: "e", opts: []string{"LEGATO", "SLASH"}}, {name: "SYNC", kind: "b"}},
	"Stopper":      {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "e", opts: []string{"4/1", "2/1", "1/1", "1/2", "1/4", "1/8", "1/16", "1/32", "1/64"}}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {name: "FLT MOD", kind: "c", lo: 0.0, hi: 100.0}, {name: "AMP MOD", kind: "c", lo: 0.0, hi: 100.0}, {}},
	"Super Filter": {{name: "CUTOFF", kind: "c", lo: 0.0, hi: 100.0}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {name: "FLT TYPE", kind: "e", opts: []string{"LPF", "BPF", "HPF"}}, {name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "SYNC", kind: "b"}},
	"Sync Delay":   {{name: "TIME", kind: "e", opts: []string{"1/32", "1/16T", "1/32D", "1/16", "1/8T", "1/16D", "1/8", "1/4T", "1/8D", "1/4", "1/2T", "1/4D", "1/2", "1/1T", "1/2D", "1/1"}}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "L DAMP F", kind: "e", opts: []string{"FLAT", "80", "100", "125", "160", "200", "250", "315", "400", "500", "630", "800"}}, {name: "H DAMP F", kind: "e", opts: []string{"630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000", "10000", "12500", "FLAT"}}, {}},
	"Tape Echo":    {{name: "TIME", kind: "c", lo: 10.0, hi: 800.0, unit: "msec"}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "MODE", kind: "e", opts: []string{"S", "M", "L", "S+M", "S+L", "M+L", "S+M+L"}}, {name: "W/F RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "W/F DEPTH", kind: "c", lo: 0.0, hi: 100.0}},
	"TimeCtrlDly":  {{name: "TIME", kind: "c", lo: 0.0, hi: 100.0}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0, unit: "%"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "L DAMP F", kind: "e", opts: []string{"FLAT", "80", "100", "125", "160", "200", "250", "315", "400", "500", "630", "800"}}, {name: "H DAMP F", kind: "e", opts: []string{"630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000", "10000", "12500", "FLAT"}}, {name: "SYNC", kind: "b"}},
	"To-Gu-Ro":     {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "RESONANCE", kind: "c", lo: 0.0, hi: 100.0}, {name: "FLT MOD", kind: "c", lo: 0.0, hi: 100.0}, {name: "AMP MOD", kind: "c", lo: 0.0, hi: 100.0}, {name: "SYNC", kind: "b"}},
	"Tremolo/Pan":  {{name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "TYPE", kind: "e", opts: []string{"TRE", "PAN"}}, {name: "WAVE", kind: "e", opts: []string{"TRI", "SQR", "SIN", "SAW1", "SAW2", "TRP"}}, {name: "SYNC", kind: "b"}, {}},
	"Vocoder":      {{name: "NOTE", kind: "e", opts: []string{"-17", "-16", "-15", "-14", "-13", "-12", "-11", "-10", "-9", "-8", "-7", "-6", "-5", "-4", "-3", "-2", "-1", "+1", "+2", "+3", "+4", "+5", "+6", "+7", "+8", "+9", "+10", "+11", "+12", "+13", "+14", "+15", "+16", "+17", "+18"}}, {name: "FORMANT", kind: "c", lo: -100.0, hi: 100.0}, {name: "TONE", kind: "c", lo: -100.0, hi: 100.0}, {name: "SCALE", kind: "e", opts: []string{"C Maj", "Db Maj", "D Maj", "Eb Maj", "E Maj", "F Maj", "Gb Maj", "G Maj", "Ab Maj", "A Maj", "Bb Maj", "B Maj", "C min", "Db min", "D min", "Eb min", "E min", "F min", "Gb min", "G min", "Ab min", "A min", "Bb min", "B min"}}, {name: "CHORD", kind: "e", opts: []string{"Root", "P5", "Oct", "UpDn", "UpDnP5", "3rd", "5thUp", "5thDn", "7thUp", "7thDn"}}, {name: "BALANCE", kind: "bal"}},
	"Wah":          {{name: "PEAK", kind: "c", lo: 0.0, hi: 100.0}, {name: "RATE", kind: "c", lo: 0.0, hi: 100.0}, {name: "MANUAL", kind: "c", lo: 0.0, hi: 100.0}, {name: "DEPTH", kind: "c", lo: 0.0, hi: 100.0}, {name: "FLT TYPE", kind: "e", opts: []string{"LPF", "BPF"}}, {name: "SYNC", kind: "b"}},
	"WrmSaturator": {{name: "DRIVE", kind: "c", lo: 0.0, hi: 48.0, unit: "dB"}, {name: "Eq LOW", kind: "c", lo: -24.0, hi: 24.0, unit: "dB"}, {name: "Eq HIGH", kind: "c", lo: -24.0, hi: 24.0, unit: "dB"}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {}, {}},
	"Zan-Zou":      {{name: "TIME", kind: "c", lo: 0.0, hi: 100.0}, {name: "FEEDBACK", kind: "c", lo: 0.0, hi: 99.0}, {name: "HF DAMP", kind: "e", opts: []string{"200", "250", "315", "400", "500", "630", "800", "1000", "1250", "1600", "2000", "2500", "3150", "4000", "5000", "6300", "8000", "OFF"}}, {name: "LEVEL", kind: "c", lo: 0.0, hi: 100.0}, {name: "MODE", kind: "e", opts: []string{"2TAP", "3TAP", "4TAP"}}, {name: "SYNC", kind: "b"}},
}

// fxParam is one knob of an effect: its name, the kind ("c" a range, "e" a list, "b" on or off, "bal" a balance,
// "pan", "nt" a note) and what it needs to say its value.
type fxParam struct {
	name, kind string
	lo, hi     float64
	opts       []string
	unit       string
	inf        bool
}

// fxKnobs is the six knobs of an effect on a bus: its parameters, or CTRL 1 to 6 where it is not known (an effect not
// named, one of the Direct FX, or no effect known).
func fxKnobs(bus, num int) [6]fxParam {
	name := fxNameOf(bus, num)
	if p, ok := fxParams[name]; ok {
		return p
	}
	var out [6]fxParam
	for k := range out {
		out[k] = fxParam{name: fmt.Sprintf("CTRL %d", k+1), kind: "c", hi: 127}
	}
	return out
}

// fxValueText says the value of a knob as the unit shows it; v is the controller value (0 to 127), -1 for not known.
func (p fxParam) valueText(v int) string {
	if v < 0 {
		return "–"
	}
	switch p.kind {
	case "b":
		if v >= 64 {
			return "ON"
		}
		return "OFF"
	case "e":
		if len(p.opts) > 0 {
			return p.opts[min(len(p.opts)-1, v*len(p.opts)/128)]
		}
	case "bal":
		w := int(math.Round(float64(v) * 100 / 127))
		return fmt.Sprintf("%d:%d", 100-w, w)
	case "pan":
		n := int(math.Round(float64(v)*100/127)) - 50
		switch {
		case n < 0:
			return fmt.Sprintf("L%d", -n)
		case n > 0:
			return fmt.Sprintf("R%d", n)
		}
		return "CTR"
	case "nt":
		return fmt.Sprintf("%s%d", []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}[v%12], v/12-1)
	}
	if p.inf && v == 0 {
		return "-INF"
	}
	x := p.lo + (p.hi-p.lo)*float64(v)/127
	span := math.Abs(p.hi - p.lo)
	dec := 3
	switch {
	case span >= 20:
		dec = 0
	case span >= 2:
		dec = 1
	case span >= 0.5:
		dec = 2
	}
	f := math.Pow(10, float64(dec))
	s := strconv.FormatFloat(math.Round(x*f)/f, 'f', -1, 64)
	if p.unit != "" {
		s += " " + p.unit
	}
	return s
}
