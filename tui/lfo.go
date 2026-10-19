package tui

import (
	"fmt"
	"math"
	"math/rand"
)

// LFOs: four low-frequency oscillators that turn a knob of a bus (or switch its effect on and off) in time with the
// unit's MIDI clock. They are sent as the controllers of the knobs, like Punch, so they move the effects the unit has,
// no others. An LFO swings around where the knob was when it was started, and puts it back when it stops. There is
// no sound in it: it is a hand on the knob that never tires, and the clock decides where in the bar it is.

const lfoCount = 4

// The shapes of an LFO.
const (
	lfoSine = iota
	lfoTriangle
	lfoSawUp
	lfoSawDown
	lfoSquare
	lfoRandom
	lfoPump
	lfoShapes
)

// lfoShapeNames are the shapes for the page.
var lfoShapeNames = [lfoShapes]string{"sine", "triangle", "saw up", "saw down", "square", "random", "pump"}

// lfoRates are the lengths of a cycle in beats (a quarter note is one): 1/16, 1/8, 1/4, 1/2, then 1, 2, 4 and 8 bars.
var lfoRates = [8]float64{0.25, 0.5, 1, 2, 4, 8, 16, 32}

// lfoRateNames are the same for the page.
var lfoRateNames = [8]string{"1/16", "1/8", "1/4", "1/2", "1 bar", "2 bars", "4 bars", "8 bars"}

// lfoSwitch is the knob number of the effect switch.
const lfoSwitch = 6

// lfoSetting is the settings of an LFO, the defaults when it has not been changed: bus 1, CTRL 1, sine, a bar, half depth.
func (m Model) lfoSetting(i int) LfoSet {
	s := m.mem.Lfo[i]
	if !s.Set {
		s = LfoSet{Rate: 4, Depth: 50}
	}
	return s
}

// lfoRun is what an LFO is doing: running, the value of the knob to swing around and to go back to, the last value sent.
type lfoRun struct {
	on     bool
	center int
	wasOn  int // the effect switch before, 1 or 0, -1 if the unit had not said
	last   int // the last value sent, -1 for none
	period int // for the random shape: the cycle the value was drawn for
	rv     float64
	value  int // what is on the knob, for the page
}

// lfoWave is the shape at phase p (0 to 1): from -1 to 1, the pump from -1 (dipped) to 0.
func lfoWave(shape int, p, rv float64) float64 {
	switch shape {
	case lfoSine:
		return math.Sin(2 * math.Pi * p)
	case lfoTriangle:
		switch {
		case p < 0.25:
			return 4 * p
		case p < 0.75:
			return 2 - 4*p
		}
		return 4*p - 4
	case lfoSawUp:
		return 2*p - 1
	case lfoSawDown:
		return 1 - 2*p
	case lfoSquare:
		if p < 0.5 {
			return 1
		}
		return -1
	case lfoRandom:
		return rv
	case lfoPump:
		// Dips at the start of the cycle and comes back, quickly at first.
		return -math.Pow(1-p, 2)
	}
	return 0
}

// lfoValue is the value an LFO sends at a clock tick (0 to 127; for the switch 127 or 0).
func lfoValue(set LfoSet, center, ticks int, run *lfoRun) int {
	period := int(lfoRates[clamp(set.Rate, 0, len(lfoRates)-1)] * 24)
	p := float64(ticks%period)/float64(period) + float64(set.Shift)*0.25
	p -= math.Floor(p)
	cycle := ticks/period + set.Shift
	if cycle != run.period || run.rv == 0 {
		run.period, run.rv = cycle, rand.Float64()*2-1
	}
	w := lfoWave(set.Shape, p, run.rv)
	if set.Knob == lfoSwitch {
		on := w > 0
		if set.Shape == lfoPump {
			on = p < 0.3
		}
		if on {
			return 127
		}
		return 0
	}
	amp := float64(set.Depth) / 100 * 127 / 2
	if set.Shape == lfoPump {
		amp = float64(set.Depth) / 100 * 127
	}
	return clamp(int(math.Round(float64(center)+w*amp)), 0, 127)
}

// lfoSet changes one setting of an LFO: bus, knob, shape, rate, depth or shift.
func (m Model) lfoSet(i int, name string, n int) Model {
	if i < 0 || i >= lfoCount {
		return m
	}
	s := m.lfoSetting(i)
	s.Set = true
	switch name {
	case "bus":
		s.Bus = clamp(n, 0, 3)
	case "knob":
		s.Knob = clamp(n, 0, lfoSwitch)
	case "shape":
		s.Shape = clamp(n, 0, lfoShapes-1)
	case "rate":
		s.Rate = clamp(n, 0, len(lfoRates)-1)
	case "depth":
		s.Depth = clamp(n, 0, 100)
	case "shift":
		s.Shift = clamp(n, 0, 3)
	default:
		return m
	}
	m.sc = m.scenes()
	run := &m.sc.lfo[i]
	if run.on && (name == "bus" || name == "knob") {
		// The knob it moved goes back before it moves another.
		m = m.lfoRestore(i)
		m.mem.Lfo[i] = s
		return m.lfoBegin(i)
	}
	m.mem.Lfo[i] = s
	return m
}

// lfoToggle starts an LFO, or stops it.
func (m Model) lfoToggle(i int) Model {
	if i < 0 || i >= lfoCount {
		return m
	}
	m.sc = m.scenes()
	if m.sc.lfo[i].on {
		m = m.lfoRestore(i)
		m.sc.lfo[i].on = false
		m.message = fmt.Sprintf("LFO %d off", i+1)
		return m
	}
	return m.lfoBegin(i)
}

// lfoBegin starts an LFO from where its knob is now.
func (m Model) lfoBegin(i int) Model {
	set := m.lfoSetting(i)
	run := &m.sc.lfo[i]
	if m.punchBusy(set.Bus) {
		m.message = fmt.Sprintf("BUS %d has a motion: stop it first", set.Bus+1)
		return m
	}
	*run = lfoRun{on: true, center: 64, wasOn: -1, last: -1}
	if set.Knob == lfoSwitch {
		if f := m.fx[set.Bus]; f.onSeen {
			run.wasOn = 0
			if f.on {
				run.wasOn = 1
			}
		}
	} else if c := m.ccOf(set.Bus+1, punchCCs[set.Knob]); c != nil {
		run.center = c.val
	} else {
		m.message = fmt.Sprintf("LFO %d: BUS %d CTRL %d is not known yet, so it swings around the middle (turn it once on the unit to start from there)", i+1, set.Bus+1, set.Knob+1)
		return m
	}
	m.message = fmt.Sprintf("LFO %d on: BUS %d %s, %s", i+1, set.Bus+1, lfoKnobName(set.Knob), lfoRateNames[set.Rate])
	if m.clock == nil || !m.clock.running {
		m.message += " — it moves with the unit's clock: start its sequencer"
	}
	return m
}

func lfoKnobName(k int) string {
	if k == lfoSwitch {
		return "effect on/off"
	}
	return fmt.Sprintf("CTRL %d", k+1)
}

// lfoSend puts a value on the knob an LFO moves, when it is not there already.
func (m Model) lfoSend(i, v int) Model {
	set, run := m.lfoSetting(i), &m.sc.lfo[i]
	run.value = v
	if v == run.last {
		return m
	}
	run.last = v
	if set.Knob == lfoSwitch {
		return m.punchSend(set.Bus, PunchBus{On: v / 127, Num: -1, Knobs: [6]int{-1, -1, -1, -1, -1, -1}}, nil)
	}
	p := PunchBus{On: -1, Num: -1, Knobs: [6]int{-1, -1, -1, -1, -1, -1}}
	p.Knobs[set.Knob] = v
	return m.punchSend(set.Bus, p, nil)
}

// lfoRestore puts the knob of an LFO back to where it was.
func (m Model) lfoRestore(i int) Model {
	set, run := m.lfoSetting(i), &m.sc.lfo[i]
	if !run.on {
		return m
	}
	if set.Knob == lfoSwitch {
		// A switch the unit never said anything about goes back off, not left as the gate stopped.
		return m.lfoSend(i, max(run.wasOn, 0)*127)
	}
	return m.lfoSend(i, run.center)
}

// lfoTick moves the LFOs on one tick of the clock.
func (m Model) lfoTick() Model {
	if m.sc == nil || m.clock == nil {
		return m
	}
	for i := range lfoCount {
		run := &m.sc.lfo[i]
		if !run.on {
			continue
		}
		set := m.lfoSetting(i)
		if m.punchBusy(set.Bus) {
			continue
		}
		m = m.lfoSend(i, lfoValue(set, run.center, m.clock.ticks, run))
	}
	return m
}

// lfoStop puts the knobs back when the clock stops; the LFOs stay on and go on with the next start.
func (m Model) lfoStop() Model {
	if m.sc == nil {
		return m
	}
	for i := range lfoCount {
		if m.sc.lfo[i].on {
			m = m.lfoRestore(i)
			m.sc.lfo[i].last = -1
		}
	}
	return m
}

// webLfo is the LFOs for the page.
type webLfo struct {
	Bus   int  `json:"bus"`
	Knob  int  `json:"knob"`
	Shape int  `json:"shape"`
	Rate  int  `json:"rate"`
	Depth int  `json:"depth"`
	Shift int  `json:"shift"`
	On    bool `json:"on"`
	Value int  `json:"value"`
}

func (m Model) webLfo() []webLfo {
	out := make([]webLfo, lfoCount)
	for i := range out {
		s := m.lfoSetting(i)
		out[i] = webLfo{Bus: s.Bus, Knob: s.Knob, Shape: s.Shape, Rate: s.Rate, Depth: s.Depth, Shift: s.Shift}
		if m.sc != nil {
			out[i].On, out[i].Value = m.sc.lfo[i].on, m.sc.lfo[i].value
		}
	}
	return out
}
