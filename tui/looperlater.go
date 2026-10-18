package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/proto"
)

// What changes the pads while the unit plays (a pad that is deleted, slices that are put on a pad) waits for a beat of the
// pattern: beat 1 of the next round, where everything is triggered again, or (a setting) the last beat before it. The work
// on the unit can make a gap, and this is where it is least heard. The pattern is as long as a loop (the length the Looper has, in bars), counted from the unit's
// start. Without a running clock the work is done at once.

// ticksPerBeat is the clock's ticks in a quarter note.
const ticksPerBeat = 24

// lpJob is work that waits for the last beat of the pattern.
type lpJob struct {
	key   string
	label string
	cmd   tea.Cmd
}

// laterAt is the tick, in the pattern, at which the queued work is done: beat 1 of the next round, or (the setting) the last beat.
func (m Model) laterAt() int {
	if m.mem.LaterLastBeat {
		return m.loopBars()*ticksPerBar - ticksPerBeat
	}
	return 0
}

// laterName says when the queued work is done.
func (m Model) laterName() string {
	if m.mem.LaterLastBeat {
		return "the last beat of the pattern"
	}
	return "beat 1 of the pattern"
}

// beatsToLater says in how many beats that is.
func (m Model) beatsToLater() float64 {
	length := m.loopBars() * ticksPerBar
	left := (m.laterAt() - m.clock.ticks%length + length) % length
	if left == 0 {
		left = length
	}
	return float64(left) / ticksPerBeat
}

// laterToggle switches between beat 1 and the last beat for the queued work.
func (m Model) laterToggle() (tea.Model, tea.Cmd) {
	m.mem.LaterLastBeat = !m.mem.LaterLastBeat
	m.message = "deleting a pad and placing slices wait for " + m.laterName()
	return m, nil
}

// looperLater queues work for the last beat of the pattern. It says false when the clock does not run: the work is not
// queued then, and the caller does it at once.
func (m Model) looperLater(job lpJob) bool {
	if !m.clockRuns() || m.clock == nil {
		return false
	}
	lp := m.looper()
	lp.jobs = append(lp.jobs, job)
	return true
}

// looperUnlater takes a queued job back.
func (m Model) looperUnlater(key string) bool {
	lp := m.looper()
	for i, j := range lp.jobs {
		if j.key == key {
			lp.jobs = append(lp.jobs[:i], lp.jobs[i+1:]...)
			return true
		}
	}
	return false
}

func (m Model) looperQueued(key string) bool {
	if m.lp == nil {
		return false
	}
	for _, j := range m.lp.jobs {
		if j.key == key {
			return true
		}
	}
	return false
}

// looperJobsTick does the queued work when the last beat of the pattern begins.
func (m Model) looperJobsTick() tea.Cmd {
	lp := m.lp
	if lp == nil || len(lp.jobs) == 0 || m.clock == nil {
		return nil
	}
	if m.clock.ticks%(m.loopBars()*ticksPerBar) != m.laterAt() {
		return nil
	}
	var cmds []tea.Cmd
	for _, j := range lp.jobs {
		m.log.Info("looper: doing the queued work", "job", j.key, "tick", m.clock.ticks)
		cmds = append(cmds, j.cmd)
	}
	lp.jobs = nil
	return tea.Batch(cmds...)
}

// looperJobsWatch drops the queued work when the clock has stopped.
func (m Model) looperJobsWatch() Model {
	if m.lp != nil && len(m.lp.jobs) > 0 && !m.clockRuns() {
		m.lp.jobs = nil
		m.message = "the clock stopped: what waited for the last beat was dropped"
	}
	return m
}

// deleteCmd is the work of deleting the samples of pads.
func (m Model) deleteCmd(pads []proto.PadIndex) tea.Cmd {
	unit := m.unit
	return m.edit(func() tea.Msg {
		if failed := app.DeletePads(unit, pads); len(failed) > 0 {
			return doneMsg{message: "not deleted: " + strings.Join(failed, ", "), reload: true}
		}
		names := make([]string, len(pads))
		for i, p := range pads {
			names[i] = p.String()
		}
		return doneMsg{message: strings.Join(names, ", ") + " deleted", reload: true}
	})
}

// laterText says what is queued, for the status line.
func (m Model) laterText() string {
	if m.lp == nil || len(m.lp.jobs) == 0 || m.clock == nil {
		return ""
	}
	var labels []string
	for _, j := range m.lp.jobs {
		labels = append(labels, j.label)
	}
	return fmt.Sprintf("on %s (in %.1f beats): %s", m.laterName(), m.beatsToLater(), strings.Join(labels, ", "))
}
