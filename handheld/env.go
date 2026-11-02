package handheld

import (
	"fmt"
	"math"
	"sort"

	"github.com/moahbeatlab/404tui/lab"
)

// An envelope on a track, as the ENV slot of Loop Cartographer: every note starts an envelope (attack, decay to a sustain
// level while the note is held, release after), and the next note is moved by how far that envelope has got when it comes
// (the pattern is a loop, so the first note follows the last). A note shortly after another is moved a lot, one long after
// it is not: ducking, accents on the offbeat, a build-up after a gap.

var (
	envSteps    = []float64{0, 0.5, 1, 2, 4, 8} // attack, decay and release, in 16ths
	envSustains = []int{0, 25, 50, 75, 100}
)

type envState struct {
	target, attack, decay, sustain, release, depth, center int
	invert                                                 bool
}

// envLevel is the envelope of a note that started dt steps ago and was held for gate steps, 0..1.
func envLevel(dt, gate float64, st envState) float64 {
	a, d, r := envSteps[st.attack], envSteps[st.decay], envSteps[st.release]
	s := float64(envSustains[st.sustain]) / 100
	held := func(t float64) float64 {
		switch {
		case a > 0 && t < a:
			return t / a
		case d > 0 && t < a+d:
			return 1 - (1-s)*(t-a)/d
		case t < a:
			return 1
		}
		return s
	}
	if dt < 0 {
		return 0
	}
	if dt <= gate {
		return held(dt)
	}
	if r <= 0 || dt >= gate+r {
		return 0
	}
	return held(gate) * (1 - (dt-gate)/r)
}

// envNotes moves every note by the envelope of the note before it.
func envNotes(notes []lab.Note, length int, st envState) []lab.Note {
	out := append([]lab.Note(nil), notes...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	if len(out) == 0 {
		return out
	}
	depth := float64(lfoDepths[st.depth]) / 100
	if st.invert {
		depth = -depth
	}
	ctr := float64(st.center)
	prev := append([]lab.Note(nil), out...)
	for i := range out {
		p := prev[(i+len(prev)-1)%len(prev)]
		dt := float64(int(prev[i].Tick)-int(p.Tick)) / stepTicks
		if dt <= 0 {
			dt += float64(length) / stepTicks
		}
		w := envLevel(dt, float64(p.Len)/stepTicks, st)
		n := &out[i]
		switch st.target {
		case 0:
			n.Vel = min(max(n.Vel+int(math.Round(ctr+w*depth*127)), 1), 127)
		case 1:
			n.Len = min(max(int(math.Round(float64(n.Len)*(1+ctr/64+w*depth*0.9))), 1), 65535)
		default:
			n.Tick = uint32(max(int(n.Tick)+int(math.Round((ctr/64+w*depth)*float64(stepTicks)/3)), 0))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

func envStepNames() []string {
	out := make([]string, len(envSteps))
	for i, v := range envSteps {
		out[i] = fmt.Sprintf("%g steps", v)
	}
	return out
}

func (a *App) envMenu() {
	pad := a.rowPad()
	st := &a.env
	if !a.envInit {
		*st = envState{attack: 0, decay: 2, sustain: 1, release: 2, depth: 3, center: 0, invert: true}
		a.envInit = true
	}
	a.toolMenu(fmt.Sprintf("ENV on %s", a.label(pad)), []menuItem{
		val("Moves the notes'", lfoTargets, st.target, func(v int) { st.target = v }),
		val("Attack", envStepNames(), st.attack, func(v int) { st.attack = v }),
		val("Decay", envStepNames(), st.decay, func(v int) { st.decay = v }),
		val("Sustain", names(envSustains, func(v int) string { return fmt.Sprintf("%d%%", v) }), st.sustain, func(v int) { st.sustain = v }),
		val("Release", envStepNames(), st.release, func(v int) { st.release = v }),
		val("Direction", []string{"as it is (up)", "inverted (ducks)"}, b2i(st.invert), func(v int) { st.invert = v == 1 }),
		val("Center (shifts the middle)", names(lfoCenters, signed), near(lfoCenters, st.center), func(v int) { st.center = lfoCenters[v] }),
		val("Depth", names(lfoDepths, func(v int) string { return fmt.Sprintf("%d%%", v) }), st.depth, func(v int) { st.depth = v }),
	}, "Shape them by the envelope", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		notes := a.trackNotes(pad)
		if len(notes) < 2 {
			a.say("an envelope needs two notes on the track at least", true)
			return
		}
		a.finishTrack(pad, envNotes(notes, int(d.Length), *st), "ENV on the "+lfoTargets[st.target])
	})
}
