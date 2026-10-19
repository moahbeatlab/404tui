package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The Slicer's preview: the slices are written to the pad they will end on, with gate and loop on, and started with a MIDI note
// on the next beat 1, so they are heard as a loop in the mix while the settings are worked on. Each change cuts the slices
// again and writes them again (a second or so, in which the loop is gone; the next beat 1 starts it again). APPLY puts the
// pad back to a normal one (ungated, not looping, muted as a new pad is): the sample is on it already, nothing is sent
// again. DISCARD stops it and deletes the pad.

// slNote sends the note of a pad to the unit: on starts it (held, as a finger would), off lets go. The channel is the bank, the
// notes are the ones the unit sends (see padOfNote).
func (m Model) slNote(pad proto.PadIndex, on bool) Model {
	order := [16]int{12, 13, 14, 15, 8, 9, 10, 11, 4, 5, 6, 7, 0, 1, 2, 3}
	sc := m.scenes()
	m.sc = sc
	status, vel := byte(0x80), byte(0)
	if on {
		status, vel = 0x90, 127
	}
	m.sceneSend(sc, []byte{status | byte(pad.Bank()), byte(36 + order[pad.Pad()]), vel})
	return m
}

// slPreviewStop lets go of the pad that plays, and cancels a start that waits.
func (m Model) slPreviewStop() Model {
	lp := m.looper()
	if lp.slPlay {
		m = m.slNote(lp.slPPad, false)
	}
	lp.slPlay, lp.slArm = false, false
	return m
}

// slPreviewToggle turns the preview on or off before slicing.
func (m Model) slPreviewToggle() (tea.Model, tea.Cmd) {
	lp := m.looper()
	if lp.slHave {
		return m, nil
	}
	lp.slPrev = !lp.slPrev
	if lp.slPrev {
		m.message = "PREVIEW: SLICE makes the slices and plays them as a loop on the free pad; change the settings and they follow"
	} else {
		m.message = "PREVIEW off"
	}
	return m, nil
}

// slPreviewWrite writes the slices that wait to the pad chosen, with gate and loop on.
func (m Model) slPreviewWrite() (tea.Model, tea.Cmd) {
	lp := m.looper()
	tb := m.slTargetBank()
	if lp.slDest < 0 || lp.slDest > 15 {
		lp.slPrev, lp.slHave = false, false
		m.message = fmt.Sprintf("bank %c is full: no pad for the preview", 'A'+tb)
		return m, nil
	}
	m = m.slPreviewStop()
	target := m.snap.Bank(tb)[lp.slDest].Pad
	lp.slPPad, lp.slDirty = target, false
	out, from, n := lp.slOut, lp.slFrom, lp.slN
	unit, edit := m.unit, m.edit
	name := "SLC " + time.Now().Format("15:04:05")
	m.busy = "sending the slices to " + target.String()
	return m, edit(func() tea.Msg {
		if _, err := app.PutSampleLive(unit, target, out, name); err != nil {
			return doneMsg{message: "the preview was lost: " + err.Error(), reload: true}
		}
		for _, p := range []string{"gate", "loop"} {
			if info, ok := proto.ParamByName(p); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(target), 1)
			}
		}
		return doneMsg{message: fmt.Sprintf("%s in %d slices on %s, looping", from, n, target), reload: true, previewed: &target}
	})
}

// slPreviewed is the preview written: it starts on the next beat 1 (at once without a clock). A setting that changed
// meanwhile cuts and writes the slices again.
func (m Model) slPreviewed(pad proto.PadIndex) (Model, tea.Cmd) {
	lp := m.looper()
	if !lp.slPrev {
		return m, nil
	}
	lp.slWritten, lp.slPPad = true, pad
	if m.clock != nil && m.clock.running {
		lp.slArm = true
	} else {
		m = m.slNote(pad, true)
		lp.slPlay = true
	}
	if lp.slDirty {
		lp.slDirty = false
		nm, cmd := m.slPreviewRefresh()
		return nm.(Model), cmd
	}
	return m, nil
}

// slPreviewTick starts the preview on beat 1.
func (m Model) slPreviewTick() Model {
	lp := m.lp
	if lp == nil || !lp.slArm || m.clock == nil || m.clock.ticks%ticksPerBar != 0 {
		return m
	}
	lp.slArm, lp.slPlay = false, true
	return m.slNote(lp.slPPad, true)
}

// slPreviewRefresh cuts the slices again with the settings now and writes them, or notes that it must when the pad is busy.
func (m Model) slPreviewRefresh() (tea.Model, tea.Cmd) {
	lp := m.looper()
	if m.busy != "" {
		lp.slDirty = true
		return m, nil
	}
	bank, pad := m.slSource()
	if pad < 0 {
		return m, nil
	}
	return m.sliceRender(m.snap.Bank(bank)[pad], m.sliceOptions(bank))
}

// slPreviewCommand is a command of the page while the slices are heard.
func (m Model) slPreviewCommand(act string, n, i int) (tea.Model, tea.Cmd) {
	lp := m.looper()
	switch act {
	case "slslices", "slcut", "slorder", "sldiv", "slswing", "slmang":
		return m.slParam(act, n, i).slPreviewRefresh()
	case "slagain":
		return m.slPreviewRefresh()
	case "slpad", "sltbank", "sltbankset":
		m.message = fmt.Sprintf("the preview is on %s: APPLY it, or DISCARD to choose another pad", lp.slPPad)
	case "slgo":
		return m.slPreviewApply()
	case "slcancel":
		return m.slPreviewDiscard()
	}
	return m, nil
}

// slPreviewApply lets the pad go back to a normal one: ungated, not looping, muted like a new pad.
func (m Model) slPreviewApply() (tea.Model, tea.Cmd) {
	lp := m.looper()
	if !lp.slWritten {
		m.message = "the preview is not on the pad yet"
		return m, nil
	}
	m = m.slPreviewStop()
	target, from, n := lp.slPPad, lp.slFrom, lp.slN
	lp.slPrev, lp.slHave, lp.slWritten, lp.slOut = false, false, false, formats.Sample{}
	unit, edit := m.unit, m.edit
	m.busy = "putting " + target.String() + " in order"
	return m, edit(func() tea.Msg {
		for _, p := range []string{"gate", "loop"} {
			if info, ok := proto.ParamByName(p); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(target), 0)
			}
		}
		msg := doneMsg{message: fmt.Sprintf("%s in %d slices → %s, muted: hold it to bring it in", from, n, target), reload: true}
		if muteNewPad(unit, target) {
			msg.muted = []proto.PadIndex{target}
		} else {
			msg.message = fmt.Sprintf("%s in %d slices → %s", from, n, target)
		}
		return msg
	})
}

// slPreviewDiscard stops the preview and deletes the pad.
func (m Model) slPreviewDiscard() (tea.Model, tea.Cmd) {
	lp := m.looper()
	m = m.slPreviewStop()
	target, written := lp.slPPad, lp.slWritten
	lp.slPrev, lp.slHave, lp.slWritten, lp.slOut = false, false, false, formats.Sample{}
	if !written {
		m.message = "the preview was dropped"
		return m, nil
	}
	m.busy = "deleting " + target.String()
	return m, m.deleteCmd([]proto.PadIndex{target})
}
