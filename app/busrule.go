package app

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/moahbeatlab/404tui/proto"
)

// BusRule says which bus a pad goes to, by its name: a pad whose name holds one of the drum words
// goes to the drum bus, any other pad to the other bus (0 is no bus). The words and the buses
// are the user's to change.
type BusRule struct {
	// DrumWords are the words that make a pad a drum pad, lower case.
	DrumWords []string `json:"drumWords"`
	// DrumBus and OtherBus are the bus of a drum pad and of any other (0 none, 1, 2).
	DrumBus  int `json:"drumBus"`
	OtherBus int `json:"otherBus"`
	// OnAssign sets the bus of a pad when a file is assigned to it.
	OnAssign bool `json:"onAssign"`
}

// DefaultDrumWords are the words of a drum kit: its voices, and what they are called short.
var DefaultDrumWords = []string{
	"kick", "bd", "snare", "sd", "clap", "hat", "hihat", "hh", "ohat", "chat", "cymbal", "crash",
	"ride", "tom", "perc", "percussion", "rim", "rimshot", "shaker", "conga", "bongo", "clave",
	"cowbell", "tambourine", "drum", "drums", "break", "snap",
}

// DefaultBusRule sends drums to BUS 2 and everything else to BUS 1, when a file is assigned too.
func DefaultBusRule() BusRule {
	return BusRule{DrumWords: append([]string(nil), DefaultDrumWords...), DrumBus: 2, OtherBus: 1, OnAssign: true}
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

// IsDrum reports whether a pad name holds a drum word: a word of the name that is one of the
// words, or that starts with one of four letters or more ("percussion" for "perc").
func (r BusRule) IsDrum(name string) bool {
	words := nameWords(name)
	// "OHat" also reads as the one word, so the whole name stripped of its separators is tried.
	words = append(words, strings.ToLower(strings.Map(func(c rune) rune {
		if unicode.IsLetter(c) {
			return c
		}
		return -1
	}, name)))
	for _, dw := range r.DrumWords {
		dw = strings.ToLower(strings.TrimSpace(dw))
		if dw == "" {
			continue
		}
		for i, w := range words {
			last := i == len(words)-1
			if w == dw || (len(dw) >= 4 && strings.HasPrefix(w, dw) && !last) || (len(dw) >= 4 && last && strings.Contains(w, dw)) {
				return true
			}
		}
	}
	return false
}

// BusFor is the bus a pad of that name goes to.
func (r BusRule) BusFor(name string) int32 {
	if r.IsDrum(name) {
		return int32(r.DrumBus)
	}
	return int32(r.OtherBus)
}

// SetPadBus sets the bus of a pad (0 none, 1, 2).
func SetPadBus(p Pads, pad proto.PadIndex, bus int32) error {
	info, ok := proto.ParamByName("bus-fx")
	if !ok {
		return fmt.Errorf("this version does not know the pad bus")
	}
	return p.SetParam(info.ID, proto.TargetPad(pad), bus)
}
