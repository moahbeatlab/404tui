package formats

import (
	"errors"
	"fmt"
	"math"

	"github.com/moahbeatlab/404tui/proto"
)

// Grooves are the genre templates behind the "new pattern" menu. A groove names a set of lanes
// (kick, clap, hats and the like) and, for each, the 16th-note steps it strikes over a phrase of
// one or more bars and how hard, with the lane's own swing, push or pull off the grid and note
// length. BuildGroove turns a lane->pad mapping into a playable PTNnnnnn.BIN. The pad a lane
// plays is chosen by the caller (guessed from the sample names, then confirmed), so the groove
// tables stay about rhythm, not any one kit.
//
// The house and techno grooves follow what Attack Magazine's "Beat Dissected" breakdowns say
// of each style (named on each template): where the voices sit, their swing and the small
// variations at the end of a phrase that keep a loop moving.

// Lane is a drum voice a groove writes to.
type Lane int

const (
	LaneKick Lane = iota
	LaneSnare
	LaneClosedHat
	LaneOpenHat
	LaneClap
	LanePerc
	LaneRide
	LaneShaker
	LaneRim
	LaneTom
)

// LaneName is the lane's name for the confirm dialog.
func (l Lane) LaneName() string {
	switch l {
	case LaneKick:
		return "Kick"
	case LaneSnare:
		return "Snare"
	case LaneClosedHat:
		return "Closed hat"
	case LaneOpenHat:
		return "Open hat"
	case LaneClap:
		return "Clap"
	case LanePerc:
		return "Perc"
	case LaneRide:
		return "Ride"
	case LaneShaker:
		return "Shaker"
	case LaneRim:
		return "Rim"
	case LaneTom:
		return "Tom"
	}
	return "?"
}

// The marks of a lane's hits, one per 16th step: how hard it is struck, or a rest. Spaces and a
// | between bars are there to be read and carry no time.
var hitVelocity = map[rune]uint8{
	'X': 120, // accent
	'x': 100, // a plain hit
	'o': 80,  // soft
	'g': 50,  // ghost
}

// laneSpec is one lane of a groove as written in the tables below.
type laneSpec struct {
	lane Lane
	// hits are the lane's steps over its phrase, 16 marks a bar (see hitVelocity); a lane with
	// fewer bars than the groove repeats its phrase.
	hits string
	// swing, 50 to 75, pushes the off-beat 16ths later the MPC way, as the SWING tool does; 0 or
	// 50 is straight.
	swing int
	// nudge moves every hit of the lane by that many ticks: later (laid back) or earlier (pushed).
	nudge int
	// gate is how long each hit is held, in ticks; 0 is a 16th.
	gate uint16
}

// GrooveTemplate is a named genre groove: the lanes it writes and how each plays. Lanes a
// template names but the caller leaves unmapped are skipped, so a kit without an open hat still
// gets the kick and clap.
type GrooveTemplate struct {
	Name string
	// Tempo is the range of tempos the style is played at, e.g. "122–127 BPM".
	Tempo string
	// Lanes are the voices this groove uses, in the order the confirm dialog shows them.
	Lanes []Lane
	specs []laneSpec
}

// groove puts a template together from its lanes, in the order given.
func groove(name, tempo string, specs ...laneSpec) GrooveTemplate {
	g := GrooveTemplate{Name: name, Tempo: tempo, specs: specs}
	for _, s := range specs {
		g.Lanes = append(g.Lanes, s.lane)
	}
	return g
}

// parseHits reads a lane's hit marks into one velocity per step (0: a rest).
func parseHits(hits string) ([]uint8, error) {
	var out []uint8
	for _, r := range hits {
		switch {
		case r == ' ' || r == '|':
		case r == '.':
			out = append(out, 0)
		default:
			v, ok := hitVelocity[r]
			if !ok {
				return nil, fmt.Errorf("%q is not a hit mark", r)
			}
			out = append(out, v)
		}
	}
	if len(out) == 0 || len(out)%16 != 0 {
		return nil, fmt.Errorf("%d steps is not a whole number of bars", len(out))
	}
	return out, nil
}

// Bars is how long the groove's phrase is: the bars of its longest lane.
func (g GrooveTemplate) Bars() int {
	bars := 1
	for _, s := range g.specs {
		if steps, err := parseHits(s.hits); err == nil {
			bars = max(bars, len(steps)/16)
		}
	}
	return bars
}

// grooveTemplates are the genre grooves, in the order the picker lists them.
var grooveTemplates = []GrooveTemplate{
	// "Bumpin' House" and "Punchy House": a 4/4 kick that skips at the end of bar 2, the clap on
	// the upbeats with pickups at the end of bar 2, an off-beat hat over a 16th hat of varied
	// velocity, light swing, and one ride to open the loop.
	groove("House", "122–127 BPM",
		laneSpec{lane: LaneKick, hits: "X...x...X...x... | X...x...X...x.o."},
		laneSpec{lane: LaneClap, hits: "....X.......X... | ....X.......X.go"},
		laneSpec{lane: LaneOpenHat, hits: "..x...x...x...x. | ..x...x...x...X.", gate: 180},
		laneSpec{lane: LaneClosedHat, hits: "og.g og.g og.g og.g | og.g og.g og.g ogog", swing: 54, gate: 60},
		laneSpec{lane: LaneRide, hits: "o............... | ................", gate: 480},
	),
	// "Deep House – Stripped Workout": 4/4 kick, the clap on 2 and 4 sitting back, an off-beat
	// open hat, and closed hats and a shaker carrying the 8th and 16th shuffle (swing 50-60).
	// A rim plays around the beat.
	groove("Deep house", "120–125 BPM",
		laneSpec{lane: LaneKick, hits: "x...x...x...x... | x...x...x...x..."},
		laneSpec{lane: LaneClap, hits: "....x.......x... | ....x.......x...", nudge: 8},
		laneSpec{lane: LaneOpenHat, hits: "..o...o...o...o. | ..o...o...o...o.", gate: 240},
		laneSpec{lane: LaneClosedHat, hits: ".g.g .g.g .g.g .g.g | .g.g .g.g .g.g .g.o", swing: 60, gate: 60},
		laneSpec{lane: LaneShaker, hits: "ggog ggog ggog ggog", swing: 58, gate: 60},
		laneSpec{lane: LaneRim, hits: "...g...o..g....g | ...g...o..g..o..", swing: 58},
	),
	// "Dirty Tech House" and Classmatic's "Catuca": a hard 4/4 kick with a syncopated one at the
	// end of the phrase, the clap on the upbeat pushed early, a snare double hit to close every
	// two bars, off-beat open hats with an extra one to close four, the ride in 8ths, a tom
	// every two bars and percussion that changes from bar to bar.
	groove("Tech house", "122–130 BPM",
		laneSpec{lane: LaneKick, hits: "X...X...X...X... | X...X...X...X... | X...X...X...X... | X...X...X...X.g."},
		laneSpec{lane: LaneClap, hits: "....X.......X...", nudge: -6},
		laneSpec{lane: LaneSnare, hits: "................ | ..............ox"},
		laneSpec{lane: LaneOpenHat, hits: "..x...x...x...x. | ..x...x...x...x. | ..x...x...x...x. | ..x...x...x...xo", gate: 180},
		laneSpec{lane: LaneClosedHat, hits: "og.g og.g og.g og.g", swing: 54, gate: 60},
		laneSpec{lane: LaneRide, hits: "x.o.x.o.x.o.x.o.", gate: 240},
		laneSpec{lane: LaneTom, hits: "................ | ..........o....."},
		laneSpec{lane: LanePerc, hits: "...o..x.....o... | ...o..x...x...o.", swing: 54},
	),
	// "Rolling Techno" and "Dark Techno Rumble": a 4/4 kick with ghost kicks just before beats 2
	// and 4 (the pendulum), claps on 2 and 4 with ghost 16ths around them, an off-beat hat over
	// fast 16ths, rolling toms, open hats with a few left out, and a ride in 8ths whose every
	// second hit is quieter (swing 50-54).
	groove("Techno", "128–135 BPM",
		laneSpec{lane: LaneKick, hits: "X..gX...X..gX..."},
		laneSpec{lane: LaneClap, hits: "....X..g....X.g. | ....X..g....X.gg"},
		laneSpec{lane: LaneClosedHat, hits: "ggxg ggxg ggxg ggxg", swing: 53, gate: 60},
		laneSpec{lane: LaneOpenHat, hits: "..x...x...x..... | ..x.......x...x.", gate: 240},
		laneSpec{lane: LaneTom, hits: "...o...g...o...g", swing: 53},
		laneSpec{lane: LaneRide, hits: "x.o.x.o.x.o.x.o.", gate: 240},
	),
	// Dave Clarke and Silent Servant style: near-straight, a 4/4 kick, the clap on 2 and 4, a
	// rolling 16th closed hat, a short off-beat open hat, a shaker doubling it every other bar
	// and a galloping conga.
	groove("Hypnotic techno", "135–140 BPM",
		laneSpec{lane: LaneKick, hits: "X...X...X...X..."},
		laneSpec{lane: LaneClap, hits: "....X.......X..."},
		laneSpec{lane: LaneClosedHat, hits: "ogxg ogxg ogxg ogxg", swing: 52, gate: 60},
		laneSpec{lane: LaneOpenHat, hits: "..x...x...x...x."},
		laneSpec{lane: LaneShaker, hits: "..o...o...o...o. | ................", gate: 60},
		laneSpec{lane: LanePerc, hits: "..o.go....o.go.. | ..o.go....o.gog."},
	),
	groove("Hip-hop", "85–95 BPM",
		laneSpec{lane: LaneKick, hits: "X.........X....."},
		laneSpec{lane: LaneSnare, hits: "....X.......X..."},
		laneSpec{lane: LaneClosedHat, hits: "x.o.o.o.x.o.o.o."},
	),
}

// GrooveTemplates are the genre grooves the picker offers, in order.
func GrooveTemplates() []GrooveTemplate {
	return grooveTemplates
}

// grooveNoteLength is the gate length of a drum hit, one 16th note.
const grooveNoteLength = PPQ / 4

// swingPush is how many ticks swing% pushes an off-beat 16th: at 50 none, at amount% the off-beat
// sits amount% of the way between its on-beats, the same as the SWING tool.
func swingPush(amount int) int {
	if amount <= 50 {
		return 0
	}
	return int(math.Round(float64(min(amount, 75)-50) / 100 * float64(2*grooveNoteLength)))
}

// BuildGroove writes a PTNnnnnn.BIN of bars bars for the template (bars 0: the groove's own
// phrase length), each mapped lane playing its phrase over and over. pads maps a lane to the pad
// that plays it; a lane missing from the map is skipped. A groove with no mapped lane is an
// error, so a pattern is never written empty by mistake. The records follow
// docs/re/04-patterns.md and reuse the same filler and trailer writeRecords uses, so the file
// round-trips through ParsePattern.
func (g GrooveTemplate) BuildGroove(pads map[Lane]proto.PadIndex, bars int) ([]byte, error) {
	if bars == 0 {
		bars = g.Bars()
	}
	if bars < 1 || bars > MaxStartBars {
		return nil, errors.New("a pattern is 1 to 8 bars long")
	}
	barTicks := 4 * PPQ
	length := uint32(bars * barTicks)
	var recs []patRecord
	for _, spec := range g.specs {
		pad, ok := pads[spec.lane]
		if !ok {
			continue
		}
		steps, err := parseHits(spec.hits)
		if err != nil {
			return nil, fmt.Errorf("%s, %s: %w", g.Name, spec.lane.LaneName(), err)
		}
		gate := spec.gate
		if gate == 0 {
			gate = grooveNoteLength
		}
		for i := 0; i < bars*16; i++ {
			vel := steps[i%len(steps)]
			if vel == 0 {
				continue
			}
			tick := i*grooveNoteLength + spec.nudge
			if i%2 == 1 {
				tick += swingPush(spec.swing)
			}
			recs = append(recs, noteRecord(uint32(clamp(tick, 0, int(length)-1)), pad, vel, gate))
		}
	}
	if len(recs) == 0 {
		return nil, errors.New("the groove has no pads to play")
	}
	return writeRecords(recs, length, startTrailer(bars)), nil
}

// clamp keeps v inside [lo, hi].
func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// BlankPattern writes an empty pattern of bars bars: no notes, just the time carried to the end
// record by filler, and the trailer. It is the starting point for programming a pattern by hand.
func BlankPattern(bars int) ([]byte, error) {
	if bars < 1 || bars > MaxStartBars {
		return nil, errors.New("a pattern is 1 to 8 bars long")
	}
	length := uint32(bars) * uint32(4*PPQ)
	return writeRecords(nil, length, startTrailer(bars)), nil
}

// noteRecord is one note at an absolute tick, in the record form readRecords/writeRecords use.
func noteRecord(tick uint32, pad proto.PadIndex, vel uint8, length uint16) patRecord {
	var r patRecord
	r.tick = tick
	r.note = true
	// bytes are record bytes 1..7: pad (two bytes), pitch 0, velocity, off velocity 64, length.
	r.bytes[0] = noteFirst + byte(pad.Index()%groupSize)
	r.bytes[1] = byte(pad.Index() / groupSize)
	r.bytes[2] = 0
	r.bytes[3] = vel
	r.bytes[4] = 64
	r.bytes[5] = byte(length)
	r.bytes[6] = byte(length >> 8)
	return r
}

// startTrailer is the 8-byte trailer StartPattern writes for a pattern of bars bars.
func startTrailer(bars int) []byte {
	return []byte{byte(bars), 0, 0, 0, 0, 0x80, 0x04, 0x01}
}

// LaneBar is one lane's first bar: how hard each of the 16 steps is struck, 0 for a rest.
type LaneBar struct {
	Lane  Lane
	Steps [16]uint8
}

// FirstBar is the first bar of the groove, a row for each of its lanes, for showing what the
// pattern is like before it is made.
func (g GrooveTemplate) FirstBar() []LaneBar {
	var out []LaneBar
	for _, s := range g.specs {
		steps, err := parseHits(s.hits)
		if err != nil {
			continue
		}
		bar := LaneBar{Lane: s.lane}
		copy(bar.Steps[:], steps[:16])
		out = append(out, bar)
	}
	return out
}
