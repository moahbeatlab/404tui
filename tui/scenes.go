package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/midiin"
)

// Scenes (Perform): the effect of each bus is saved as scene A and as scene B (on or off, which effect and
// the three knobs, as the unit last said them over MIDI), and the fade between them is sent back to
// the unit as MIDI controllers: by hand with h and l, or as a glide over some bars that begins on the
// next bar of the unit's clock. A bus that has not both scenes saved is left alone.

// fxScene is the effect of one bus as it was saved; a knob that was not known is -1.
type fxScene struct {
	set     bool
	on      bool
	onKnown bool // the unit had said whether the effect was on
	num     int
	knobs   [3]int
}

// busScene is the scenes of one bus and where its fade is: each bus has its own, so one can glide
// while the other stays.
type busScene struct {
	a, b  fxScene
	mix   float64
	glide *glide
	sent  fxScene
	mo    [motionSlots]motion
	sel   int // the slot of the motion tapped last
}

// sceneState is the scenes of both buses and the output they are sent on. It is shared by the copies
// of the model, as the cache of the drawing is.
type sceneState struct {
	bus  [2]busScene
	bars int // the length of a glide
	// overdub: a knob moved while a motion plays is written into it.
	overdub bool
	out     midiin.Out
	outErr  string
	tried   time.Time
	// punch is the snapshots that are held (see punch.go).
	punch punchRun
	// lfo is what the LFOs are doing (see lfo.go).
	lfo [4]lfoRun
	// last is the last message sent to the unit, as text, for the panel and the log.
	last string
}

// glide is a fade that runs with the clock.
type glide struct {
	from, to float64
	waiting  bool // for the next bar
	ticks    int  // clock ticks since it began (a new Start of the unit does not disturb it)
	total    int  // how many it takes, fixed when it began
}

const ticksPerBar = 24 * 4

func (m Model) scenes() *sceneState {
	if m.sc == nil {
		return &sceneState{bars: 2}
	}
	return m.sc
}

// captureScene reads the effect of a bus as the unit last said it.
func (m Model) captureScene(bus int) fxScene {
	f := m.fx[bus]
	s := fxScene{set: true, on: f.on, onKnown: f.onSeen, num: -1, knobs: [3]int{-1, -1, -1}}
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

// sceneAct does what a button of the SCENES panel is for: "a" and "b" save the bus's effect as that
// scene, "fade" puts the fade where the bar was clicked (pos, 0 to 1), "glide" starts a glide to the other
// scene (or stops the one that runs), "bars" sets the length of a glide, "jumpa" and "jumpb" go to a scene.
func (m Model) sceneAct(act string, bus int, pos float64) Model {
	sc := m.scenes()
	if act == "bars" {
		// Here bus is the number of bars, not a bus.
		if bus < 1 || bus > 8 {
			return m
		}
		sc.bars = bus
		m.sc = sc
		m.message = fmt.Sprintf("a glide takes %s", plural(sc.bars, "bar"))
		return m
	}
	if bus < 0 || bus > 1 {
		return m
	}
	bs := &sc.bus[bus]
	switch act {
	case "a", "b":
		scene := m.captureScene(bus)
		if act == "a" {
			bs.a = scene
		} else {
			bs.b = scene
		}
		m.message = fmt.Sprintf("BUS %d: scene %s saved: %s", bus+1, strings.ToUpper(act), sceneDetail(scene))
	case "fade":
		bs.mix, bs.glide = math.Max(math.Min(math.Round(pos*20)/20, 1), 0), nil
		m = m.sceneApply(sc, bus)
	case "jumpa", "jumpb":
		bs.mix, bs.glide = map[string]float64{"jumpa": 0, "jumpb": 1}[act], nil
		m = m.sceneApply(sc, bus)
	case "glide":
		switch {
		case bs.motionBusy():
			m.message = fmt.Sprintf("BUS %d: a motion is on: stop it first", bus+1)
		case bs.glide != nil:
			bs.glide = nil
			m.message = fmt.Sprintf("BUS %d: glide stopped where it is", bus+1)
		case !bs.a.set || !bs.b.set:
			m.message = fmt.Sprintf("BUS %d: save scene A and scene B first", bus+1)
		case !fadeable(bs.a, bs.b):
			m.message = fmt.Sprintf("BUS %d: nothing to fade: the unit has not said a knob value in both scenes. Turn C1 C2 C3 on the unit once, then save A and B again", bus+1)
		case m.clock == nil || !m.clock.running:
			m.message = "a glide runs with the unit's MIDI clock: start its sequencer (the fade bar works without)"
		default:
			to := 1.0
			if bs.mix >= 0.5 {
				to = 0
			}
			bs.glide = &glide{from: bs.mix, to: to, waiting: true, total: sc.bars * ticksPerBar}
			m.message = fmt.Sprintf("BUS %d: glide to %s over %s, from the next bar", bus+1, map[bool]string{true: "B", false: "A"}[to == 1], plural(sc.bars, "bar"))
		}
	}
	m.sc = sc
	return m
}

// sceneTick moves the glides on with the clock.
func (m Model) sceneTick() Model {
	sc := m.sc
	if sc == nil || m.clock == nil {
		return m
	}
	for bus := range 2 {
		bs := &sc.bus[bus]
		g := bs.glide
		if g == nil {
			continue
		}
		if g.waiting {
			if m.clock.ticks%ticksPerBar != 0 {
				continue
			}
			g.waiting, g.ticks = false, 0
		} else {
			g.ticks++
		}
		p := math.Min(float64(g.ticks)/float64(g.total), 1)
		if p >= 1 {
			bs.glide = nil
		}
		bs.mix = math.Max(math.Min(g.from+(g.to-g.from)*p, 1), 0)
		m = m.sceneApply(sc, bus)
	}
	return m
}

// sceneApply sends the controllers that the fade position of a bus changes.
func (m Model) sceneApply(sc *sceneState, bus int) Model {
	bs := &sc.bus[bus]
	a, b := bs.a, bs.b
	if !a.set || !b.set || bs.motionBusy() {
		return m
	}
	pick := a
	if bs.mix >= 0.5 {
		pick = b
	}
	last := &bs.sent
	ch := byte(bus)
	if pick.num >= 0 && (!last.set || pick.num != last.num) {
		m.sceneSend(sc, []byte{0xB0 | ch, ccFxNumber, byte(pick.num)})
		last.num = pick.num
	}
	for k := range 3 {
		if a.knobs[k] < 0 || b.knobs[k] < 0 {
			continue
		}
		v := clamp(int(math.Round(float64(a.knobs[k])+float64(b.knobs[k]-a.knobs[k])*bs.mix)), 0, 127)
		if !last.set || v != last.knobs[k] {
			m.sceneSend(sc, []byte{0xB0 | ch, byte(16 + k), byte(v)})
			last.knobs[k] = v
		}
	}
	if a.onKnown && b.onKnown && (!last.set || pick.on != last.on) {
		v := byte(0)
		if pick.on {
			v = 127
		}
		m.sceneSend(sc, []byte{0xB0 | ch, ccFxSwitch, v})
		last.on = pick.on
	}
	last.set = true
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
	if len(msg) == 3 && msg[0]&0xF0 != 0xB0 {
		sc.last = fmt.Sprintf("channel %d, note %d, velocity %d", int(msg[0]&0x0F)+1, msg[1], msg[2])
		m.log.Info("midi: sent", "channel", int(msg[0]&0x0F)+1, "note", msg[1], "velocity", msg[2], "on", msg[0]&0xF0 == 0x90)
	} else if len(msg) == 3 {
		if m.mon != nil && ((msg[1] >= 16 && msg[1] <= 18) || (msg[1] >= 80 && msg[1] <= 82)) {
			m.mon.setCC(int(msg[0]&0x0F)+1, int(msg[1]), int(msg[2]), time.Now())
		}
		sc.last = fmt.Sprintf("channel %d, controller %d = %d", int(msg[0]&0x0F)+1, msg[1], msg[2])
		m.log.Debug("midi: sent", "channel", int(msg[0]&0x0F)+1, "controller", msg[1], "value", msg[2])
	} else {
		sc.last = fmt.Sprintf("channel %d, program %d", int(msg[0]&0x0F)+1, msg[1])
		m.log.Info("midi: sent", "channel", int(msg[0]&0x0F)+1, "program", msg[1])
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

// sceneLines are the SCENES panel of the Perform sidebar, with its big buttons: for each bus the two
// snapshots one above the other, each showing what it holds, then the fade bar and the glide button.
func (m Model) sceneLines(width int) ([]string, []btn) {
	st := m.st
	sc := m.scenes()
	var lines []string
	var btns []btn
	band := func(b *rowBuilder) {
		l, bt := b.rows(len(lines))
		lines = append(lines, l...)
		btns = append(btns, bt...)
	}
	for bus := range 2 {
		bs := sc.bus[bus]
		colour, _ := st.th.busColour(int32(bus + 1))
		lines = append(lines, lipgloss.NewStyle().Foreground(colour).Bold(true).Render(fmt.Sprintf("BUS %d", bus+1)))
		b := newRowBuilder(st, 2*m.btnH)
		snapW := max(width/2, 22)
		b.stack(snapW, snapshotSpec("A", bs.a, "a", bus), snapshotSpec("B", bs.b, "b", bus))
		b.gap(1)
		label := "glide →B"
		if bs.mix >= 0.5 {
			label = "glide →A"
		}
		switch {
		case bs.glide != nil && bs.glide.waiting:
			label = "waiting… stop"
		case bs.glide != nil:
			label = "gliding… stop"
		}
		glideW := max(ansi.StringWidth(label)+4, 16)
		barW := max(width-snapW-glideW-2, 6)
		b.bar(barW, bs.mix, colour, "fade", bus)
		b.gap(1)
		b.button(label, bs.glide != nil, "glide", bus, glideW)
		band(b)
	}
	return lines, btns
}

// snapshotSpec is the button of a snapshot: lit when something is saved, showing the effect and the
// three knobs.
func snapshotSpec(name string, s fxScene, act string, bus int) btnSpec {
	sp := btnSpec{label: name + "  ·  tap to save", detail: "empty", act: act, arg: bus}
	if !s.set {
		return sp
	}
	sp.on = true
	sp.label = name + "  ✓  tap to save again"
	var parts []string
	if s.num >= 0 {
		parts = append(parts, fxName(s.num))
	}
	if s.onKnown && !s.on {
		parts = append(parts, "off")
	}
	knobs := make([]string, 3)
	for k, v := range s.knobs {
		knobs[k] = "–"
		if v >= 0 {
			knobs[k] = fmt.Sprint(v)
		}
	}
	parts = append(parts, strings.Join(knobs, " · "))
	sp.detail = strings.Join(parts, "   ")
	return sp
}

func sceneText(s fxScene) string {
	if !s.set {
		return "—"
	}
	var parts []string
	if s.num >= 0 {
		parts = append(parts, fxName(s.num))
	}
	if s.onKnown && !s.on {
		parts = append(parts, "off")
	}
	known := 0
	for _, v := range s.knobs {
		if v >= 0 {
			known++
		}
	}
	parts = append(parts, fmt.Sprintf("%d/3 knobs", known))
	return strings.Join(parts, " ")
}

// fadeable says whether the two scenes have anything in common to fade: a knob known in both, or the
// on/off switch or the effect known in both.
func fadeable(a, b fxScene) bool {
	for k := range a.knobs {
		if a.knobs[k] >= 0 && b.knobs[k] >= 0 {
			return true
		}
	}
	return a.onKnown && b.onKnown || a.num >= 0 && b.num >= 0
}

// sceneDetail says what a scene holds, and what the unit had not said.
func sceneDetail(s fxScene) string {
	var parts []string
	if s.num >= 0 {
		parts = append(parts, fxName(s.num))
	}
	if s.onKnown {
		parts = append(parts, map[bool]string{true: "on", false: "off"}[s.on])
	}
	var missing []string
	for k, v := range s.knobs {
		if v >= 0 {
			parts = append(parts, fmt.Sprintf("C%d %d", k+1, v))
		} else {
			missing = append(missing, fmt.Sprintf("C%d", k+1))
		}
	}
	text := strings.Join(parts, ", ")
	if text == "" {
		text = "nothing known yet"
	}
	if len(missing) > 0 {
		text += " (the unit has not sent " + strings.Join(missing, " ") + ": turn it once on the unit)"
	}
	return text
}

// scenesOn says whether the scenes (the snapshots A and B of a bus and the glide between them) are
// shown. They are switched off for now: the motions are what is used.
var scenesOn = false
