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
	// ClapNudge moves the claps by ticks (a tick is about a millisecond): early (negative) for a clap that locks to the kick, late for a laid-back one
	ClapNudge int
	// BrokenKick is a style whose kick is not four on the floor
	BrokenKick bool
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
		Swing: [2]int{52, 60}, Ghosts: 0.3, Poly: 0.2, Echo: 0.3, Chance: 0.4, ClapNudge: -6,
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
		Swing: [2]int{54, 58}, Ghosts: 0.45, Poly: 0.5, Echo: 0.45, Chance: 0.6, ClapNudge: -8,
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
		Swing: [2]int{50, 52}, Ghosts: 0.2, Poly: 0.5, Echo: 0.35, Chance: 0.5, ClapNudge: -5,
	},
}

// the styles of the sub-genres (written from the idioms the genres share, in the generator's own text lanes)
func init() {
	styles = append(styles,
		Style{
			Name: "Chicago house", Tempo: "120-124 BPM", BPM: 122, Bars: 2,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "X...X...X...X..."},
				{Lane: formats.LaneClosedHat, Hits: ".o.o.o.o.o.o.o.o", Until: 0.5, Swing: 60, Gate: 60},
				{Lane: formats.LaneClosedHat, Hits: ".o.x.o.o.o.x.o.o | .o.o.o.x.o.o.o.x", From: 0.5, Swing: 60, Gate: 60},
				{Lane: formats.LaneOpenHat, Hits: "..X...X...X...X.", From: 0.1, Gate: 150},
				{Lane: formats.LaneClap, Hits: "....X.......X...", From: 0.2},
				{Lane: formats.LaneRim, Hits: "..x..x..x..x.... | ..x..x..x..x..x.", From: 0.6},
				{Lane: formats.LaneShaker, Hits: "g.og g.og g.og g.og", From: 0.8, Swing: 60, Gate: 60},
			},
			HatKinds: map[string]int{"8th": 3, "16th": 4, "euclid": 1},
			PercHits: [2]int{4, 6}, Fills: []string{"snare roll", "clap stack", "ghost shuffle", "kick stutter", "hat stop", "rim run"},
			Swing: [2]int{58, 62}, Ghosts: 0.3, Poly: 0.2, Echo: 0.2, Chance: 0.3, ClapNudge: -5,
		},
		Style{
			Name: "Deep house", Tempo: "118-124 BPM", BPM: 120, Bars: 2,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "x...x...x...x..."},
				{Lane: formats.LaneClosedHat, Hits: "x.g.x.g.x.g.x.g.", Until: 0.6, Swing: 60, Gate: 60},
				{Lane: formats.LaneClosedHat, Hits: "xgogxgogxgogxgog", From: 0.6, Swing: 62, Gate: 60},
				{Lane: formats.LaneClap, Hits: "....o.......o...", From: 0.2},
				{Lane: formats.LaneShaker, Hits: "oxoxoxoxoxoxoxox", From: 0.55, Swing: 62, Gate: 60},
				{Lane: formats.LanePerc, Hits: "...x.x....x...x. | ...x.x.....x..x.", From: 0.55, Gate: 100},
				{Lane: formats.LaneOpenHat, Hits: "..o...o...o...o.", From: 0.7, Gate: 150},
				{Lane: formats.LaneRim, Hits: ".......g.......g", From: 0.6},
			},
			HatKinds: map[string]int{"8th": 5, "euclid": 2, "16th": 1},
			PercHits: [2]int{4, 6}, Fills: []string{"ghost shuffle", "clap stack", "rim run", "hat stop", "open hat swell"},
			Swing: [2]int{58, 64}, Ghosts: 0.4, Poly: 0.3, Echo: 0.4, Chance: 0.3, ClapNudge: 12, // a laid-back clap
		},
		Style{
			Name: "Minimal tech house", Tempo: "124-128 BPM", BPM: 126, Bars: 2,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "x...x...x...x..."},
				{Lane: formats.LaneClosedHat, Hits: "...o...o...o...o", Until: 0.4, Gate: 60},
				{Lane: formats.LaneClosedHat, Hits: "...o...o...o...o | ..go...o..go...o", From: 0.4, Swing: 52, Gate: 60},
				{Lane: formats.LaneRim, Hits: "....x.........x. | ....x...........x", From: 0.15},
				{Lane: formats.LanePerc, Hits: "x..x..x..x..x..x | x..x..x..x..x.x.", From: 0.4, Gate: 80},
				{Lane: formats.LaneOpenHat, Hits: "..o...........o.", From: 0.7, Gate: 120},
				{Lane: formats.LaneClap, Hits: "....o.......o...", From: 0.85},
			},
			HatKinds: map[string]int{"euclid": 5, "16th": 2, "8th": 2},
			PercHits: [2]int{3, 6}, Fills: []string{"rim run", "hat stop", "ghost shuffle", "kick stutter"},
			Swing: [2]int{50, 54}, Ghosts: 0.25, Poly: 0.8, Echo: 0.4, Chance: 0.7, ClapNudge: -6,
		},
		Style{
			Name: "Dub techno", Tempo: "120-128 BPM", BPM: 124, Bars: 2,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "x...x...x...x..."},
				{Lane: formats.LaneClosedHat, Hits: "..o...o...o...o.", Gate: 80},
				{Lane: formats.LaneRim, Hits: "...x............", From: 0.2, Gate: 80}, // the chord stab, that the echo repeats
				{Lane: formats.LaneOpenHat, Hits: "......o.......o.", From: 0.5, Gate: 150},
				{Lane: formats.LanePerc, Hits: "x....x....x....x", From: 0.45, Gate: 90},
				{Lane: formats.LaneClap, Hits: "....o.......o...", From: 0.7},
			},
			HatKinds: map[string]int{"8th": 3, "euclid": 4, "16th": 1},
			PercHits: [2]int{3, 5}, Fills: []string{"hat stop", "rim run", "open hat swell"},
			Swing: [2]int{50, 54}, Ghosts: 0.15, Poly: 0.9, Echo: 0.9, Chance: 0.4, ClapNudge: -4,
		},
		Style{
			Name: "Industrial techno", Tempo: "135-145 BPM", BPM: 140, Bars: 2, BrokenKick: true,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "X..X..X.X..X..X."},
				{Lane: formats.LaneClap, Hits: "....X.......X...", From: 0.2},
				{Lane: formats.LaneSnare, Hits: "....X.......X...", From: 0.4},
				{Lane: formats.LaneClosedHat, Hits: "x.x.x.x.x.x.x.x.", From: 0.1, Gate: 60},
				{Lane: formats.LaneRide, Hits: "X.......X.......", From: 0.5, Gate: 200},
				{Lane: formats.LanePerc, Hits: "x..x..x..x..x... | x..x..x..x..x.x.", From: 0.6, Gate: 100},
				{Lane: formats.LaneTom, Hits: "..............x. | ...........x..x.", From: 0.9},
			},
			HatKinds: map[string]int{"8th": 3, "euclid": 3, "16th": 2},
			PercHits: [2]int{5, 8}, Fills: []string{"snare roll", "kick stutter", "tom cascade", "hat stop"},
			Swing: [2]int{50, 50}, Ghosts: 0.05, Poly: 0.5, Echo: 0.2, Chance: 0.3, ClapNudge: -5,
		},
		Style{
			Name: "Acid", Tempo: "126-135 BPM", BPM: 130, Bars: 2,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "X...X...X...X..."},
				{Lane: formats.LaneClosedHat, Hits: "x.x.x.x.x.x.x.x.", Until: 0.4, Gate: 60},
				{Lane: formats.LaneClosedHat, Hits: "xoxoxoxoxoxoxoxo", From: 0.4, Gate: 60},
				{Lane: formats.LaneOpenHat, Hits: "..x...x...x...x.", From: 0.1, Gate: 150},
				{Lane: formats.LaneSnare, Hits: "....x.......x...", From: 0.2},
				{Lane: formats.LaneClap, Hits: "....x.......x...", From: 0.4},
				{Lane: formats.LaneTom, Hits: "...x.....x...x.. | ...x.....x.....x", From: 0.7},
			},
			HatKinds: map[string]int{"16th": 5, "8th": 2, "euclid": 2},
			PercHits: [2]int{5, 7}, Fills: []string{"snare roll", "tom cascade", "kick stutter", "hat stop", "clap stack"},
			Swing: [2]int{50, 54}, Ghosts: 0.2, Poly: 0.3, Echo: 0.3, Chance: 0.4, ClapNudge: -6,
		},
		Style{
			Name: "90s house", Tempo: "122-128 BPM", BPM: 125, Bars: 2,
			Lanes: []SL{
				{Lane: formats.LaneKick, Hits: "X...X...X...X..."},
				{Lane: formats.LaneOpenHat, Hits: "..X...X...X...X.", Gate: 170}, // the offbeat open hat of the 909 house records
				{Lane: formats.LaneClosedHat, Hits: "x.x.x.x.x.x.x.x.", Until: 0.4, Swing: 56, Gate: 60},
				{Lane: formats.LaneClosedHat, Hits: "xoxoxoxoxoxoxoxo | xoxoxoxoxoxoxoxx", From: 0.4, Swing: 58, Gate: 60},
				{Lane: formats.LaneClap, Hits: "....X.......X..."},
				{Lane: formats.LaneSnare, Hits: "....x.......x... | ....x.......x..g", From: 0.35},          // layered under the clap
				{Lane: formats.LaneShaker, Hits: "o.o.o.o.o.o.o.o.", From: 0.5, Swing: 56, Gate: 60},        // the tambourine
				{Lane: formats.LanePerc, Hits: "...x..x....x..x. | ..x...x....x...x", From: 0.6, Gate: 100}, // congas
				{Lane: formats.LaneRide, Hits: "x.......x.......", From: 0.85, Gate: 200},
			},
			HatKinds: map[string]int{"8th": 3, "16th": 4, "euclid": 1},
			PercHits: [2]int{4, 7}, Fills: []string{"snare roll", "clap stack", "ghost shuffle", "kick stutter", "tom cascade", "hat stop"},
			Swing: [2]int{55, 60}, Ghosts: 0.3, Poly: 0.2, Echo: 0.2, Chance: 0.3, ClapNudge: 3,
		},
	)
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
