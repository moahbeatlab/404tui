package handheld

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/lab"
)

// An LFO on a track: a wave, one cycle long over a number of steps, moves the velocity, the length or the timing of every
// note by where the note sits in the wave. The result is written into the pattern (baked), like the other tools.

// The shapes, the rates (note values) and the shift are those of the LFOs in 404jam (PERF > LFO), in the same order.
var (
	lfoTargets = []string{"velocity", "length", "timing"}
	lfoShapes  = []string{"SIN", "TRI", "SQR", "SAW", "RND", "PUMP", "SAW DN", "S&H"}
	lfoRates   = []struct {
		name  string
		steps int // 16ths for one round
	}{{"8/1", 128}, {"4/1", 64}, {"2/1", 32}, {"1/1", 16}, {"1/2", 8}, {"1/4", 4}, {"1/8", 2}, {"1/16", 1}}
	lfoDepths = []int{10, 20, 30, 50, 75, 100}
	lfoShifts = []string{"0", "1/4", "1/2", "3/4"} // the wave starts that much of a round later
)

type lfoState struct{ target, shape, rate, depth, shift int }

// lfoAnchor is the random value of round k (-1..1), the same for the same seed.
func lfoAnchor(seed int64, k int) float64 {
	return rand.New(rand.NewSource(seed+int64(k)*7919)).Float64()*2 - 1
}

// lfoWave is one round of the shape at a position in rounds (the whole part is the round), -1..1. PUMP only dips: full
// at the beat, back to the middle by the next. RND glides between a random value for each round, S&H holds it.
func lfoWave(shape int, pos float64, seed int64) float64 {
	k := int(math.Floor(pos))
	ph := pos - float64(k)
	switch lfoShapes[shape] {
	case "SIN":
		return math.Sin(2 * math.Pi * ph)
	case "TRI":
		if ph < 0.5 {
			return ph*4 - 1
		}
		return 3 - ph*4
	case "SQR":
		if ph < 0.5 {
			return 1
		}
		return -1
	case "SAW":
		return ph*2 - 1
	case "SAW DN":
		return 1 - ph*2
	case "PUMP":
		return -math.Pow(1-ph, 2)
	case "S&H":
		return lfoAnchor(seed, k)
	}
	a, b := lfoAnchor(seed, k-1), lfoAnchor(seed, k)
	return a + (b-a)*ph
}

// lfoNotes moves each note of the track by the wave at its tick.
func lfoNotes(notes []lab.Note, st lfoState, seed int64) []lab.Note {
	out := append([]lab.Note(nil), notes...)
	depth := float64(lfoDepths[st.depth]) / 100
	cycle := float64(lfoRates[st.rate].steps * stepTicks)
	phase := float64(st.shift) / 4
	for i := range out {
		n := &out[i]
		w := lfoWave(st.shape, float64(n.Tick)/cycle+phase, seed)
		switch st.target {
		case 0:
			amp := 63.5
			if lfoShapes[st.shape] == "PUMP" {
				amp = 127
			}
			n.Vel = min(max(n.Vel+int(math.Round(w*depth*amp)), 1), 127)
		case 1:
			n.Len = min(max(int(math.Round(float64(n.Len)*(1+w*depth*0.9))), 1), 65535)
		default:
			n.Tick = uint32(max(int(n.Tick)+int(math.Round(w*depth*float64(stepTicks)/3)), 0))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

func (a *App) lfoMenu() {
	pad := a.rowPad()
	st := &a.lfo
	if st.depth == 0 && st.rate == 0 && st.shape == 0 && st.target == 0 {
		*st = lfoState{rate: 3, depth: 3}
	}
	a.toolMenu(fmt.Sprintf("LFO on %s", a.label(pad)), []menuItem{
		val("Moves the notes'", lfoTargets, st.target, func(v int) { st.target = v }),
		val("Wave", lfoShapes, st.shape, func(v int) { st.shape = v }),
		val("Rate (one round)", lfoRateNames(), st.rate, func(v int) { st.rate = v }),
		val("Depth", names(lfoDepths, func(v int) string { return fmt.Sprintf("%d%%", v) }), st.depth, func(v int) { st.depth = v }),
		val("Shift (starts later by)", lfoShifts, st.shift, func(v int) { st.shift = v }),
	}, "Modulate them", func() {
		if _, err := a.ensurePattern(); err != nil {
			a.say(err.Error(), true)
			return
		}
		notes := a.trackNotes(pad)
		if len(notes) == 0 {
			a.say("this track has no notes", true)
			return
		}
		a.finishTrack(pad, lfoNotes(notes, *st, a.tt.seed), "LFO on the "+lfoTargets[st.target])
	})
}

func lfoRateNames() []string {
	out := make([]string, len(lfoRates))
	for i, r := range lfoRates {
		out[i] = r.name
	}
	return out
}
