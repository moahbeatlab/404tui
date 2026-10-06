package proto

import (
	"math"
	"strings"
)

// Parameter ids for the set-parameter command (01 id 00 target value).

// Target of a parameter: one pad, or the global scope (FFFF).
type Target uint16

const TargetGlobal Target = 0xFFFF

func TargetPad(p PadIndex) Target {
	return Target(p.Index())
}

type Scope int

const (
	ScopePad Scope = iota
	ScopeGlobal
)

type ParamInfo struct {
	Name  string
	ID    byte
	Scope Scope
	Min   int32
	Max   int32
	Help  string
}

func pad(name string, id byte, min, max int32, help string) ParamInfo {
	return ParamInfo{Name: name, ID: id, Scope: ScopePad, Min: min, Max: max, Help: help}
}

func global(name string, id byte, min, max int32, help string) ParamInfo {
	return ParamInfo{Name: name, ID: id, Scope: ScopeGlobal, Min: min, Max: max, Help: help}
}

// Params are the parameters mapped by differential captures (docs/re/03-parameters.md).
var Params = []ParamInfo{
	pad("start", 0x67, 0, math.MaxInt32, "start point, sample frames"),
	pad("end", 0x68, 0, math.MaxInt32, "end point, sample frames"),
	pad("level", 0x69, 0, 127, ""),
	pad("gate", 0x6A, 0, 1, ""),
	pad("loop", 0x6B, 0, 1, ""),
	pad("mute-group", 0x6D, 0, 10, "0 = off"),
	pad("bpm-sync", 0x6E, 0, 1, ""),
	pad("bpm", 0x6F, 4000, 20000, "BPM x 100"),
	pad("mode-flags", 0x70, 0, 255, "bit0 fixed velocity, bits 3/4 chromatic mode, bit5 one shot"),
	pad("loop-top", 0x71, 0, math.MaxInt32, "loop top, sample frames"),
	pad("pitch-coarse", 0x73, -12, 12, "semitones (Vinyl off)"),
	pad("pitch-fine", 0x74, -100, 100, "cents (Vinyl off)"),
	pad("play-mode", 0x75, 0, 3, "0 forward, 1 reverse, 2 fwd ping-pong, 3 rev ping-pong"),
	pad("time-stretch", 0x76, 5000, 15000, "percent x 100"),
	pad("vinyl", 0x77, 0, 1, ""),
	pad("balance", 0x78, 13, 115, "64 = centre"),
	pad("pad-link", 0x79, 0, 10, "0 = off"),
	pad("bus-fx", 0x7A, 0, 2, "1 = BUS 1"),
	pad("roll", 0x7B, 0, 10, "2 = 1/4"),
	pad("attack", 0x7D, 0, 127, ""),
	pad("hold", 0x7E, 1, 100, ""),
	pad("release", 0x7F, 0, 127, ""),
	pad("groove", 0x8A, 0, 8, "0 = off"),
	pad("rate", 0x8B, -7, 7, "0 = default"),
	pad("humanize", 0x8C, 0, 3, "0 = off"),
	global("selected-bank", 0x00, 0, 9, "UI sync"),
	global("selected-pad", 0x01, 0, 15, "UI sync"),
	global("project-tempo", 0x0A, 4000, 20000, "BPM x 100"),
	global("tempo-select", 0x0B, 0, 1, "0 bank, 1 project"),
}

const (
	// ChopPointFirst: chop point slots 1–16 are ids 8E–9D, one slot each (8D is a separate
	// 0–15 value).
	ChopPointFirst  byte = 0x8E
	BankProtectBase byte = 0x17
	BankTempoBase   byte = 0x21
	BankVolumeBase  byte = 0x35
)

func ParamByName(name string) (ParamInfo, bool) {
	for _, p := range Params {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return ParamInfo{}, false
}

// BankParam resolves bank-level global parameters: bank-protect-a, bank-tempo-j,
// bank-volume-c. It returns the id and whether the value is stored inverted (127 − value).
func BankParam(name string) (id byte, inverted bool, ok bool) {
	name = strings.ToLower(name)
	at := strings.LastIndexByte(name, '-')
	if at < 0 {
		return 0, false, false
	}
	kind, bank := name[:at], name[at+1:]
	if len(bank) != 1 || bank[0] < 'a' || bank[0] > 'j' {
		return 0, false, false
	}
	b := bank[0] - 'a'
	switch kind {
	case "bank-protect":
		return BankProtectBase + b, false, true
	case "bank-tempo":
		return BankTempoBase + b, false, true
	case "bank-volume":
		return BankVolumeBase + b, true, true
	}
	return 0, false, false
}

// blockField is the position of a parameter inside the pad block, for ids verified
// against known values.
func blockField(id byte) (int, bool) {
	switch {
	// Contiguous run: field n holds id 0x66 + n.
	case id >= 0x67 && id <= 0x7B:
		return int(id - 0x66), true
	// After Roll there is no field for 0x7C; Attack starts one field earlier.
	case id >= 0x7D && id <= 0x7F:
		return int(id - 0x67), true
	}
	return 0, false
}
