package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/proto"
)

// The unit's own Looper (a mode of the SP-404MKII) records the sound of a bus; the sample is copied to a pad on the
// unit. Watching it, the program ungates a pad that got a new sample and stops the looper (REC, controller 88, value
// 0 on channel 1) when the pattern comes round to its end, so the loop plays once in the pattern, and not on and on.

// ccLooperRec is the REC button of the unit's Looper mode: 127 starts, 0 stops.
const ccLooperRec = 88

// looperWatchEvery is how often the unit is read while watching, in case it does not announce the change.
const looperWatchEvery = 5 * time.Second

// looperWatchToggle switches watching the unit's looper on or off.
func (m Model) looperWatchToggle() (tea.Model, tea.Cmd) {
	m.mem.LooperWatch = !m.mem.LooperWatch
	lp := m.looper()
	lp.stopTick = 0
	if m.mem.LooperWatch {
		m.message = fmt.Sprintf("watching: a new sample on a pad is ungated, the unit's looper stops after %s of the pattern", plural(m.loopBars(), "bar"))
	} else {
		m.message = "no longer watching the unit's looper"
	}
	return m, nil
}

// looperFound looks at the pads that were read again and takes care of each new sample: it is ungated, and the looper
// is stopped when the pattern ends. old is what was known before.
func (m *Model) looperFound(old, cur *app.Snapshot) tea.Cmd {
	lp := m.looper()
	lp.loadedAt = time.Now()
	if !m.mem.LooperWatch || old == nil || cur == nil || old.Project != cur.Project || m.unit == nil || len(old.Pads) != len(cur.Pads) {
		return nil
	}
	var ungate []proto.PadIndex
	var names []string
	found := false
	for i, n := range cur.Pads {
		o := old.Pads[i]
		if !n.HasSample || (o.HasSample && o.Size == n.Size && o.Name == n.Name) {
			continue
		}
		found = true
		m.log.Info("looper: new sample on the unit", "pad", n.Pad.String(), "name", n.Name, "bytes", n.Size, "gate", n.Gate)
		names = append(names, n.Pad.String())
		if n.Gate {
			ungate = append(ungate, n.Pad)
			cur.Pads[i].Gate = false
		}
	}
	if !found {
		return nil
	}
	msg := fmt.Sprintf("new sample on %s: ", joinNames(names))
	if len(ungate) > 0 {
		msg += "ungated"
	} else {
		msg += "was not gated"
	}
	if m.clockRuns() && m.clock != nil {
		length := m.loopBars() * ticksPerBar
		lp.stopTick = m.clock.ticks - m.clock.ticks%length + length
		msg += fmt.Sprintf(", the looper stops at the end of the pattern in %.1f beats", float64(lp.stopTick-m.clock.ticks)/24)
	} else {
		m.looperStop()
		msg += ", looper stopped (the clock is not running)"
	}
	m.message = msg
	if len(ungate) == 0 {
		return nil
	}
	unit, log := m.unit, m.log
	return m.edit(func() tea.Msg {
		info, ok := proto.ParamByName("gate")
		if !ok {
			return doneMsg{message: msg}
		}
		for _, p := range ungate {
			if err := unit.SetParam(info.ID, proto.TargetPad(p), 0); err != nil {
				log.Error("looper: ungating failed", "pad", p.String(), "err", err)
				return doneMsg{message: fmt.Sprintf("could not ungate %s: %v", p, err)}
			}
		}
		return doneMsg{message: msg}
	})
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// looperUnitTick stops the unit's looper when the clock has come to the end of the pattern.
func (m Model) looperUnitTick() {
	lp := m.lp
	if lp == nil || lp.stopTick == 0 || m.clock == nil || m.clock.ticks < lp.stopTick {
		return
	}
	lp.stopTick = 0
	m.looperStop()
}

// looperStop sends the stop of the unit's looper (REC off, controller 88, channel 1).
func (m Model) looperStop() {
	lp := m.looper()
	if lp.out == nil {
		out, ok := m.midiSrc.(midiin.Outputs)
		if !ok || time.Since(lp.tried) < 3*time.Second {
			return
		}
		lp.tried = time.Now()
		o, name, err := out.OpenOut(m.midiPort())
		if err != nil {
			m.log.Error("looper: no MIDI output to stop the unit's looper", "err", err)
			return
		}
		lp.out = o
		m.log.Info("looper: sending to", "output", name)
	}
	m.log.Info("looper: stopping the unit's looper", "tick", m.clock.ticks)
	if err := lp.out.Send([]byte{0xB0, ccLooperRec, 0}); err != nil {
		m.log.Error("looper: stop not sent", "err", err)
		lp.out.Close()
		lp.out = nil
	}
}
