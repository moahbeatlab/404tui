package app

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/moahbeatlab/404tui/proto"
)

// BusRule says which bus a pad goes to, by its name: a name that holds one of the words of BUS 1
// goes to BUS 1, else one that holds a word of BUS 2 to BUS 2, else to Other. The words, the
// bus of the rest and the colours of the buses are the user's to change.
type BusRule struct {
	// Version tells the rule from older ones that were saved in another shape.
	Version int `json:"version"`
	// Bus1Words and Bus2Words are the words (lower case) that send a pad to each bus. A word of
	// the first list wins where a name holds words of both.
	Bus1Words []string `json:"bus1Words"`
	Bus2Words []string `json:"bus2Words"`
	// Other is the bus of a pad whose name holds none of the words (0 none, 1, 2).
	Other int `json:"other"`
	// OnAssign sets the bus of a pad when a file is assigned to it.
	OnAssign bool `json:"onAssign"`
	// Bus1Colour and Bus2Colour name the colours the buses are drawn in ("" for the theme's);
	// see BusColours.
	Bus1Colour string `json:"bus1Colour,omitempty"`
	Bus2Colour string `json:"bus2Colour,omitempty"`
}

// BusRuleVersion is the shape of the rule that is saved now.
const BusRuleVersion = 2

// BusColours are the colours a bus can be given, by name, as 256-colour numbers.
var BusColours = []struct{ Name, Code string }{
	{"blue", "75"}, {"red", "203"}, {"green", "114"}, {"orange", "214"}, {"purple", "177"},
	{"cyan", "80"}, {"pink", "211"}, {"yellow", "221"}, {"white", "252"},
}

// DefaultDrumWords are the words of a drum kit: its voices, and what they are called short.
var DefaultDrumWords = []string{
	"kick", "bd", "snare", "sd", "clap", "hat", "hihat", "hh", "ohat", "chat", "cymbal", "crash",
	"ride", "tom", "perc", "percussion", "rim", "rimshot", "shaker", "conga", "bongo", "clave",
	"cowbell", "tambourine", "drum", "drums", "break", "snap",
}

// DefaultBusRule sends drums to BUS 2 and everything else to BUS 1, when a file is assigned too.
func DefaultBusRule() BusRule {
	return BusRule{Version: BusRuleVersion, Bus2Words: append([]string(nil), DefaultDrumWords...), Other: 1, OnAssign: true}
}

// nameWords splits a pad name into lower-case words: at anything that is not a letter, at digits,
// and between a small and a capital letter (OHat, DrumLoop → o, hat; drum, loop).
func nameWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case !unicode.IsLetter(r):
			flush()
		case unicode.IsUpper(r) && i > 0 && unicode.IsLower(runes[i-1]):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// HasWord reports whether a pad name holds one of the words: a word of the name that is one of
// them, or that starts with one of four letters or more ("percussion" for "perc").
func HasWord(words []string, name string) bool {
	parts := nameWords(name)
	// "OHat" also reads as the one word, so the whole name stripped of its separators is tried.
	parts = append(parts, strings.ToLower(strings.Map(func(c rune) rune {
		if unicode.IsLetter(c) {
			return c
		}
		return -1
	}, name)))
	for _, dw := range words {
		dw = strings.ToLower(strings.TrimSpace(dw))
		if dw == "" {
			continue
		}
		for i, w := range parts {
			last := i == len(parts)-1
			if w == dw || (len(dw) >= 4 && strings.HasPrefix(w, dw) && !last) || (len(dw) >= 4 && last && strings.Contains(w, dw)) {
				return true
			}
		}
	}
	return false
}

// Match says which list a name is in: 1, 2, or 0 for neither.
func (r BusRule) Match(name string) int {
	switch {
	case HasWord(r.Bus1Words, name):
		return 1
	case HasWord(r.Bus2Words, name):
		return 2
	}
	return 0
}

// BusFor is the bus a pad of that name goes to.
func (r BusRule) BusFor(name string) int32 {
	if m := r.Match(name); m != 0 {
		return int32(m)
	}
	return int32(r.Other)
}

// SetPadBus sets the bus of a pad (0 none, 1, 2).
func SetPadBus(p Pads, pad proto.PadIndex, bus int32) error {
	info, ok := proto.ParamByName("bus-fx")
	if !ok {
		return fmt.Errorf("this version does not know the pad bus")
	}
	return p.SetParam(info.ID, proto.TargetPad(pad), bus)
}
