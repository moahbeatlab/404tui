package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/midiin"
)

// Scenes (Perform): the effect of each bus is saved as scene A and as scene B (on or off, which effect and
// the three knobs, as the unit last said them over MIDI), and the fade between them is sent back to
// the unit as MIDI controllers: by hand with h and l, or as a glide over some bars that begins on the
// next bar of the unit's clock. A bus that has not both scenes saved is left alone.

// fxScene is the effect of one bus as it was saved; a knob that was not known is -1.
type fxScene struct {
	set   bool
	on    bool
	num   int
	knobs [3]int
}

// sceneState is the scenes and the output they are sent on. It is shared by the copies of the
// model, as the cache of the drawing is.
type sceneState struct {
	a, b   [2]fxScene
	mix    float64
	off    [2]bool // a bus that does not follow the fade
	bars   int     // the length of a glide
	out    midiin.Out
	outErr string
	tried  time.Time
	sent   [2]fxScene
	glide  *glide
}

// glide is a fade that runs with the clock.
type glide struct {
	from, to float64
	waiting  bool // for the next bar
	start    int  // the tick it began on
}

const ticksPerBar = 24 * 4

func (m Model) scenes() *sceneState {
	if m.sc == nil {
		return &sceneState{bars: 2}
	}
	return m.sc
}

// capture reads the effect of a bus as the unit last said it.
func (m Model) captureScene(bus int) fxScene {
	f := m.fx[bus]
	s := fxScene{set: true, on: f.on, num: -1, knobs: [3]int{-1, -1, -1}}
	if f.numSeen {
		s.num = f.num
	}
	for k := range s.knobs {
		if c := m.ccOf(bus+1, 16+k); c != nil {
			s.knobs[k] = c.val
		}
	}
	return s
}

// sceneKey handles the keys of the scenes.
func (m Model) sceneKey(name string) (Model, bool) {
	sc := m.scenes()
	switch name {
	case "A", "B":
		for bus := range 2 {
			if sc.off[bus] {
				continue
			}
			if name == "A" {
				sc.a[bus] = m.captureScene(bus)
			} else {
				sc.b[bus] = m.captureScene(bus)
			}
		}
		m.message = "scene " + name + " saved from the effects as the unit last said them"
	case "h", "left":
		sc.mix = math.Max(math.Round(sc.mix*20-1)/20, 0)
		sc.glide = nil
		m = m.sceneApply(sc)
	case "l", "right":
		sc.mix = math.Min(math.Round(sc.mix*20+1)/20, 1)
		sc.glide = nil
		m = m.sceneApply(sc)
	case "H":
		sc.mix, sc.glide = 0, nil
		m = m.sceneApply(sc)
	case "L":
		sc.mix, sc.glide = 1, nil
		m = m.sceneApply(sc)
	case "t", "T":
		bus := 0
		if name == "T" {
			bus = 1
		}
		sc.off[bus] = !sc.off[bus]
	case "G":
		sc.bars = map[int]int{1: 2, 2: 4, 4: 8, 8: 1}[sc.bars]
		if sc.bars == 0 {
			sc.bars = 2
		}
		m.message = fmt.Sprintf("a glide takes %s", plural(sc.bars, "bar"))
	case "g":
		if sc.glide != nil {
			sc.glide = nil
			m.message = "glide stopped where it is"
			break
		}
		if m.clock == nil || !m.clock.running {
			m.message = "a glide runs with the unit's MIDI clock: start its sequencer (h l fade by hand)"
			break
		}
		to := 1.0
		if sc.mix >= 0.5 {
			to = 0
		}
		sc.glide = &glide{from: sc.mix, to: to, waiting: true}
		m.message = fmt.Sprintf("glide to %s over %s, from the next bar", map[bool]string{true: "B", false: "A"}[to == 1], plural(sc.bars, "bar"))
	default:
		return m, false
	}
	m.sc = sc
	return m, true
}

// sceneTick moves a glide on with the clock.
func (m Model) sceneTick() Model {
	sc := m.sc
	if sc == nil || sc.glide == nil || m.clock == nil {
		return m
	}
	g := sc.glide
	if g.waiting {
		if m.clock.ticks%ticksPerBar != 0 {
			return m
		}
		g.waiting, g.start = false, m.clock.ticks
	}
	p := float64(m.clock.ticks-g.start) / float64(sc.bars*ticksPerBar)
	if p >= 1 {
		p = 1
		sc.glide = nil
	}
	sc.mix = g.from + (g.to-g.from)*p
	return m.sceneApply(sc)
}

// sceneApply sends the controllers that the fade position changes.
func (m Model) sceneApply(sc *sceneState) Model {
	for bus := range 2 {
		a, b := sc.a[bus], sc.b[bus]
		if sc.off[bus] || !a.set || !b.set {
			continue
		}
		pick := a
		if sc.mix >= 0.5 {
			pick = b
		}
		last := &sc.sent[bus]
		ch := byte(bus)
		if pick.num >= 0 && (!last.set || pick.num != last.num) {
			m.sceneSend(sc, []byte{0xB0 | ch, ccFxNumber, byte(pick.num)})
			last.num = pick.num
		}
		for k := range 3 {
			if a.knobs[k] < 0 || b.knobs[k] < 0 {
				continue
			}
			v := int(math.Round(float64(a.knobs[k]) + float64(b.knobs[k]-a.knobs[k])*sc.mix))
			if !last.set || v != last.knobs[k] {
				m.sceneSend(sc, []byte{0xB0 | ch, byte(16 + k), byte(v)})
				last.knobs[k] = v
			}
		}
		if !last.set || pick.on != last.on {
			v := byte(0)
			if pick.on {
				v = 127
			}
			m.sceneSend(sc, []byte{0xB0 | ch, ccFxSwitch, v})
			last.on = pick.on
		}
		last.set = true
	}
	return m
}

// sceneSend sends one message to the unit, opening the output the first time (and again now and then
// when it was not there).
func (m Model) sceneSend(sc *sceneState, msg []byte) {
	if sc.out == nil {
		out, ok := m.midiSrc.(midiin.Outputs)
		if !ok || time.Since(sc.tried) < 3*time.Second {
			return
		}
		sc.tried = time.Now()
		o, name, err := out.OpenOut(m.midiPort())
		if err != nil {
			sc.outErr = err.Error()
			m.log.Debug("midi: output not open", "err", err)
			return
		}
		sc.out, sc.outErr = o, ""
		m.log.Info("midi: sending to", "output", name)
	}
	if err := sc.out.Send(msg); err != nil {
		sc.outErr = err.Error()
		sc.out.Close()
		sc.out = nil
	}
}

// closeScenes lets go of the output.
func (m Model) closeScenes() {
	if m.sc != nil && m.sc.out != nil {
		m.sc.out.Close()
		m.sc.out = nil
	}
}

// sceneLines are the SCENES panel of the Perform sidebar.
func (m Model) sceneLines(width int) []string {
	st := m.st
	sc := m.scenes()
	barW := max(width-4-14, 10)
	filled := int(math.Round(sc.mix * float64(barW)))
	bar := lipgloss.NewStyle().Foreground(st.th.accent).Render(strings.Repeat("█", filled)) + st.faint.Render(strings.Repeat("░", barW-filled))
	lines := []string{st.bold.Render("A ") + bar + st.bold.Render(" B") + st.text.Render(fmt.Sprintf(" %3d%%", int(math.Round(sc.mix*100))))}
	for bus := range 2 {
		colour, _ := st.th.busColour(int32(bus + 1))
		head := lipgloss.NewStyle().Foreground(colour).Bold(true).Render(fmt.Sprintf("BUS %d", bus+1))
		state := fmt.Sprintf("A %s  B %s", sceneText(sc.a[bus]), sceneText(sc.b[bus]))
		if sc.off[bus] {
			state = "does not follow (t T)"
		}
		lines = append(lines, head+"  "+st.dim.Render(state))
	}
	switch {
	case sc.glide != nil && sc.glide.waiting:
		lines = append(lines, st.hot.Render("glide waits for the next bar")+st.dim.Render("  g stops"))
	case sc.glide != nil:
		lines = append(lines, st.hot.Render("gliding")+st.dim.Render(fmt.Sprintf(" over %s  g stops", plural(sc.bars, "bar"))))
	default:
		lines = append(lines, st.dim.Render(fmt.Sprintf("A B save  h l fade  H L jump  g glide %s (G)", plural(sc.bars, "bar"))))
	}
	if sc.outErr != "" {
		lines = append(lines, st.warn.Render("cannot send to the unit: "+sc.outErr))
	} else if _, ok := m.midiSrc.(midiin.Outputs); !ok {
		lines = append(lines, st.warn.Render("no MIDI output in this build"))
	}
	return lines
}

func sceneText(s fxScene) string {
	if !s.set {
		return "—"
	}
	t := fxName(max(s.num, 0))
	if s.num < 0 {
		t = "?"
	}
	if !s.on {
		t += " off"
	}
	return t
}
