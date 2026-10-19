package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
)

// The Slicer's preview: the slices stay in memory, on no pad, and are heard as a loop on the computer's audio output, as the
// Slicer of the terminal does. Send that output to the unit (a USB cable and its EXT SOURCE button, or the computer's own
// speakers) and the loop is in the mix. The loop starts on beat 1 of the unit's clock and has the length of the sample, so it
// stays in step with a pattern of that length. A setting that changes cuts the slices again and the new ones take over from the
// next round, without a break. PLACE puts the slices on a pad as ever and stops the loop; DISCARD stops it.

// slPreviewToggle turns the preview on or off.
func (m Model) slPreviewToggle() (tea.Model, tea.Cmd) {
	lp := m.looper()
	if lp.slPrev {
		lp.slPrev = false
		m = m.slPreviewStop()
		m.message = "PREVIEW off"
		return m, nil
	}
	if m.player == nil {
		m.message = "no audio output to play the preview on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	lp.slPrev = true
	m.message = "PREVIEW: the slices loop on the computer's audio output (send it to the unit); settings change the loop"
	if lp.slHave {
		return m.slPreviewPlay()
	}
	return m, nil
}

// slPreviewStop stops the loop and a start that waits.
func (m Model) slPreviewStop() Model {
	lp := m.looper()
	if lp.slPlay && m.player != nil {
		m.player.Stop()
	}
	lp.slPlay, lp.slArm = false, false
	return m
}

// slPreviewPlay plays the slices that wait: the first time on the next beat 1 (at once without a clock), later as the loop's next
// round.
func (m Model) slPreviewPlay() (tea.Model, tea.Cmd) {
	lp := m.looper()
	if !lp.slPrev || !lp.slHave || m.player == nil {
		return m, nil
	}
	if lp.slPlay {
		if swapper, ok := m.player.(app.LoopSwapper); ok {
			_ = swapper.NextLoop(lp.slOut)
		} else if looper, ok := m.player.(app.Looper); ok {
			_ = looper.Loop(lp.slOut)
		}
		return m, nil
	}
	if m.clock != nil && m.clock.running {
		lp.slArm = true
		return m, nil
	}
	lp.slPlay = true
	m.slPreviewLoop()
	return m, nil
}

// slPreviewLoop starts the loop.
func (m Model) slPreviewLoop() {
	lp := m.lp
	if looper, ok := m.player.(app.Looper); ok {
		_ = looper.Loop(lp.slOut)
	} else {
		_ = m.player.Play(lp.slOut)
	}
}

// slPreviewTick starts the preview on beat 1.
func (m Model) slPreviewTick() Model {
	lp := m.lp
	if lp == nil || !lp.slArm || m.player == nil || m.clock == nil || m.clock.ticks%ticksPerBar != 0 {
		return m
	}
	lp.slArm, lp.slPlay = false, true
	m.slPreviewLoop()
	return m
}

// slPreviewRefresh cuts the slices again with the settings now, or notes that it must when that is going on.
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

// slPreviewState says how the preview is going, for the page.
func (m Model) slPreviewState() string {
	lp := m.lp
	switch {
	case lp == nil || !lp.slPrev:
		return ""
	case m.busy != "":
		return "cutting…"
	case lp.slArm:
		return "starts on beat 1"
	case lp.slPlay:
		return "looping"
	case lp.slHave:
		return "ready"
	}
	return "on"
}
