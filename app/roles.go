package app

import (
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
	{formats.LaneOpenHat, []string{"ohat", "open hat", "openhat", "ohh", "oh_", "_oh"}},
	{formats.LaneRim, []string{"rim", "snap", "stick", "click"}},
	{formats.LaneRide, []string{"ride", "cymbal"}},
	{formats.LaneShaker, []string{"shaker", "shkr", "shake", "tamb", "maraca"}},
	{formats.LaneTom, []string{"tom"}},
	{formats.LanePerc, []string{"perc", "conga", "bongo", "tabla", "crash", "cowbell", "clave", "block"}},
	{formats.LaneClosedHat, []string{"chat", "closed hat", "closedhat", "chh", "ch_", "_ch", "hat", "hh", "hihat", "hi-hat"}},
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

// nameMatches reports whether name (case-insensitively) contains any of the keywords.
func nameMatches(name string, keywords []string) bool {
	low := strings.ToLower(name)
	for _, k := range keywords {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}
