package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/formats"
)

// patBounceMsg is the open pattern, mixed as it is heard, ready to go onto a pad.
type patBounceMsg struct {
	sample formats.Sample
	name   string
	err    error
}

// patternBounce resamples the open pattern: the same mix that plays on this computer, so what is
// muted (m) is left out, and when some tracks are soloed (s) only those are in it, and only the
// bars of the range when one is set. It is one sample, as long as the pattern, that goes onto a
// pad of the bank that the picker asks for.
func (m Model) patternBounce() (tea.Model, tea.Cmd) {
	d := m.pattern
	if m.unit == nil || m.snap == nil || d == nil {
		return m, nil
	}
	if len(d.PadsUsed) == 0 {
		m.message = "the pattern plays nothing"
		return m, nil
	}
	mix := m.patternMixer(d.Slot)
	name := fmt.Sprintf("ptn %s mix", d.Slot)
	m.busy = "mixing the pattern"
	return m, func() tea.Msg {
		sample, err := mix()
		return patBounceMsg{sample: sample, name: name, err: err}
	}
}

// patternBounced asks which pad the mix goes onto.
func (m Model) patternBounced(msg patBounceMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		m.message = "could not mix the pattern: " + msg.err.Error()
		return m, nil
	}
	if msg.sample.Frames() == 0 {
		m.message = "nothing to bounce: every track is muted or silent"
		return m, nil
	}
	m.picker = &picker{bank: m.bank, pad: m.nextFree(m.bank), bounces: []formats.Sample{msg.sample}, bounceNames: []string{msg.name}, bounceWhat: "pattern"}
	return m, nil
}

// bouncedMessage is what is said once a bounce is on its pad.
func bouncedMessage(what string, sample formats.Sample, target, name string) string {
	seconds := float64(sample.Frames()) / formats.SampleRate
	if what == "pattern" {
		return fmt.Sprintf("bounced the pattern, %.1f s as you hear it, to %s as %q", seconds, target, name)
	}
	if what == "harmony" {
		return fmt.Sprintf("put %.1f s of chords on %s as %q", seconds, target, name)
	}
	if what == "stretch" {
		return fmt.Sprintf("stretched to %.1f s on %s as %q: slice it there, at the tempo", seconds, target, name)
	}
	return fmt.Sprintf("bounced %.1f s of slices to %s as %q", seconds, target, name)
}
