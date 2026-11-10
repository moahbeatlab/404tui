package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// A style is a groove written as text lanes (one character a 16th step: X accent, x hit, o soft, g ghost, . rest; | between bars) and
// the ways it may be varied. Each lane has the energy it enters at (From) and the one it leaves at (Until, 0: never), so one style
// reaches from a bare intro to a full drop. These are the generator's own patterns, written for it, not taken from anywhere.

// SL is one lane of a style.
type SL struct {
	Lane        formats.Lane
	Hits        string
	From, Until float64
	Swing       int
	Gate        uint16
}

// Style is a genre.
type Style struct {
	Name  string
	Tempo string // the range it is played at
	BPM   int    // a usual tempo
	Bars  int    // the phrase of its lanes
	Lanes []SL
	// What may change from one pattern to the next. The core (the kick and the claps) stays where the genre has it; this is the room around it.
	HatKinds map[string]int // the hat patterns it may use, with how often
	PercHits [2]int         // the hits of the euclidean percussion line, from and to (of 16 steps)
	Fills    []string       // the fills that suit it
	Swing    [2]int         // the swing it may be played with, 50 is straight
	Ghosts   float64        // how many ghost notes it likes (0..1)
	Poly     float64        // the chance of a polymeter line when the pattern's length has a cycle that fits
	Echo     float64        // the chance of an echo on the claps, rim, perc or open hat
	Chance   float64        // the chance that the hats get a chance-with-memory
}

var styles = []Style{
	{
		Name: "House", Tempo: "120-126 BPM", BPM: 124, Bars: 2,
		Lanes: []SL{
			{Lane: formats.LaneKick, Hits: "X...X...X...X..."},
			{Lane: formats.LaneClosedHat, Hits: "o.x.o.x.o.x.o.x.", Until: 0.45, Gate: 60},
			{Lane: formats.LaneClosedHat, Hits: "ogxg ogxg ogxg ogxo | ogxg ogxg ogxg oxxo", From: 0.45, Swing: 54, Gate: 60},
			{Lane: formats.LaneOpenHat, Hits: "..x...x...x...x.", From: 0.15, Gate: 180},
			{Lane: formats.LaneClap, Hits: "....x.......x...", From: 0.25},
			{Lane: formats.LaneRim, Hits: "......g.......g. | ....g.g.......g.", From: 0.7},
			{Lane: formats.LaneShaker, Hits: "g.og g.og g.og g.og", From: 0.6, Swing: 56, Gate: 60},
		},
		HatKinds: map[string]int{"8th": 3, "16th": 4, "euclid": 2},
		PercHits: [2]int{5, 7}, Fills: []string{"snare roll", "clap stack", "open hat swell", "ghost shuffle", "kick stutter", "hat stop"},
		Swing: [2]int{50, 58}, Ghosts: 0.3, Poly: 0.2, Echo: 0.3, Chance: 0.4,
	},
	{
		Name: "Tech house", Tempo: "124-128 BPM", BPM: 126, Bars: 2,
		Lanes: []SL{
			{Lane: formats.LaneKick, Hits: "X...X...X...X..."},
			{Lane: formats.LaneClosedHat, Hits: "x.o.x.o.x.o.x.o.", Until: 0.35, Gate: 60},
			{Lane: formats.LaneClosedHat, Hits: "xogo xogo xogo xogo | xogo xogx xogo xoxo", From: 0.35, Swing: 56, Gate: 60},
			{Lane: formats.LaneOpenHat, Hits: "..x...x...x...x. | ..x...x...x..xx.", From: 0.1, Gate: 160},
			{Lane: formats.LaneClap, Hits: "....x.......x...", From: 0.2},
			{Lane: formats.LanePerc, Hits: "x..x..x...x..x.. | x..x..x..x..x.x.", From: 0.3, Gate: 100},
			{Lane: formats.LaneRim, Hits: "..g..o..g..o..g.", From: 0.5, Swing: 56},
			{Lane: formats.LaneShaker, Hits: "gogo gogo gogo gogo", From: 0.55, Swing: 58, Gate: 60},
			{Lane: formats.LaneRide, Hits: "..o...o...o...o.", From: 0.85, Gate: 120},
		},
		HatKinds: map[string]int{"16th": 5, "euclid": 4, "8th": 1},
		PercHits: [2]int{6, 9}, Fills: []string{"snare roll", "rim run", "tom cascade", "ghost shuffle", "clap stack", "hat stop", "kick stutter"},
		Swing: [2]int{54, 62}, Ghosts: 0.45, Poly: 0.5, Echo: 0.45, Chance: 0.6,
	},
	{
		Name: "Techno", Tempo: "126-135 BPM", BPM: 130, Bars: 2,
		Lanes: []SL{
			{Lane: formats.LaneKick, Hits: "X...X...X...X..."},
			{Lane: formats.LaneOpenHat, Hits: "..x...x...x...x.", From: 0.1, Gate: 200},
			{Lane: formats.LaneClosedHat, Hits: "x.x.x.x.x.x.x.x.", Until: 0.3, Gate: 60},
			{Lane: formats.LaneClosedHat, Hits: "ogogogogogogogog | ogogogogogogogxg", From: 0.3, Gate: 60},
			{Lane: formats.LaneClap, Hits: "....x.......x...", From: 0.3},
			{Lane: formats.LaneRim, Hits: "..x..x..x..x..x. | ..x..x..x..x.x..", From: 0.5},
			{Lane: formats.LanePerc, Hits: "x...x..x..x..x.. | x...x..x..x..xx.", From: 0.6, Gate: 100},
			{Lane: formats.LaneRide, Hits: "x.x.x.x.x.x.x.x.", From: 0.85, Gate: 120},
			{Lane: formats.LaneTom, Hits: "................ | ...........g.g.o", From: 0.9},
		},
		HatKinds: map[string]int{"16th": 4, "euclid": 5, "8th": 1},
		PercHits: [2]int{5, 8}, Fills: []string{"snare roll", "tom cascade", "rim run", "clap stack", "hat stop", "kick stutter"},
		Swing: [2]int{50, 54}, Ghosts: 0.2, Poly: 0.5, Echo: 0.35, Chance: 0.5,
	},
}

// Styles are the styles, in order.
func Styles() []Style { return styles }

// StyleByName finds a style by name, ignoring case and spaces.
func StyleByName(name string) (Style, error) {
	key := func(s string) string {
		return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s))
	}
	for _, s := range styles {
		if key(s.Name) == key(name) {
			return s, nil
		}
	}
	var names []string
	for _, s := range styles {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return Style{}, fmt.Errorf("no style %q (there are: %s)", name, strings.Join(names, ", "))
}
