package app

import (
	"slices"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// Role guessing turns a bank's sample names into a first guess of which pad plays which drum
// voice, so a genre groove can be pre-filled before the user confirms it. A pad has no role of
// its own, only a free-text sample name, so this is a substring match on the name — a starting
// point the confirm dialog lets the user correct, not the last word.

// roleKeyword maps a lane to the lower-case substrings that name it. The lanes are tried in
// order, so a name matching two lanes takes the first; closed hat is last among the hats so a
// generic "hat" falls to it. Each pad is given to at most one lane (its first match).
var roleKeywords = []struct {
	lane     formats.Lane
	keywords []string
}{
	{formats.LaneKick, []string{"kick", "bassdrum", "bass drum", "bd", "bd_", "_bd"}},
	{formats.LaneSnare, []string{"snare", "snr", "sd", "sd_", "_sd"}},
	{formats.LaneClap, []string{"clap", "clp", "hclap", "handclap"}},
	{formats.LaneOpenHat, []string{"ohat", "open hat", "openhat", "ohh", "oh_", "_oh", "=oh"}},
	{formats.LaneRim, []string{"rim", "snap", "stick", "click"}},
	{formats.LaneRide, []string{"ride", "cymbal"}},
	{formats.LaneShaker, []string{"shaker", "shkr", "shake", "tamb", "maraca"}},
	{formats.LaneTom, []string{"tom"}},
	{formats.LaneBongoLo, []string{"bongo lo", "bongo low", "low bongo", "hembra"}},
	{formats.LaneBongoHi, []string{"bongo hi", "bongo high", "high bongo", "macho"}},
	{formats.LaneCongaSlap, []string{"conga slap", "slap"}},
	{formats.LaneCongaOpen, []string{"conga open", "conga hi", "conga high", "open tone"}},
	{formats.LaneCongaTip, []string{"conga tip", "conga mute", "conga heel"}},
	{formats.LaneCongaBass, []string{"conga bass", "conga lo", "conga low"}},
	{formats.LanePerc, []string{"perc", "conga", "bongo", "tabla", "cowbell", "clave", "block", "tom"}},
	{formats.LaneClosedHat, []string{"chat", "closed hat", "closedhat", "chh", "ch_", "_ch", "=ch", "hat", "hh", "hihat", "hi-hat"}},
}

// GuessRoles maps groove lanes to pads by matching the sample names of pads. Only pads with a
// sample are considered; each pad fills at most one lane and each lane takes the first (lowest
// pad) matching name. Lanes with no match are left out of the map.
func GuessRoles(pads []PadRow) map[formats.Lane]proto.PadIndex {
	out := map[formats.Lane]proto.PadIndex{}
	used := map[proto.PadIndex]bool{}
	for _, rk := range roleKeywords {
		for _, row := range pads {
			if !row.HasSample || used[row.Pad] {
				continue
			}
			if nameMatches(row.Name, rk.keywords) {
				out[rk.lane] = row.Pad
				used[row.Pad] = true
				break
			}
		}
	}
	return out
}

// GuessAlternates gives a lane a second pad: the next pad (with a sample, not in main) whose name fits the lane too, like "909 CH 2" beside
// "909 CH", or "909 KICK LONG" beside "909 KICK". The generator plays the two in turn, so the whole kit is heard.
func GuessAlternates(pads []PadRow, main map[formats.Lane]proto.PadIndex) map[formats.Lane]proto.PadIndex {
	out := map[formats.Lane]proto.PadIndex{}
	used := map[proto.PadIndex]bool{}
	for _, p := range main {
		used[p] = true
	}
	for _, rk := range roleKeywords {
		if _, ok := main[rk.lane]; !ok {
			continue
		}
		for _, row := range pads {
			if !row.HasSample || used[row.Pad] || !nameMatches(row.Name, rk.keywords) {
				continue
			}
			out[rk.lane] = row.Pad
			used[row.Pad] = true
			break
		}
	}
	return out
}

// nameMatches reports whether name (case-insensitively) contains any of the keywords.
func nameMatches(name string, keywords []string) bool {
	low := strings.ToLower(name)
	words := strings.FieldsFunc(low, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
	for _, k := range keywords {
		if w, ok := strings.CutPrefix(k, "="); ok { // a keyword with = is a whole word: "909 CH" is a closed hat, "punch" is not
			if slices.Contains(words, w) {
				return true
			}
			continue
		}
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}
