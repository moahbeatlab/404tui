package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// patternBed is a pattern, mixed from its pads, that plays under the slices: every round of the
// slicer is as long as the pattern (or a whole number of times that), the slice steps lie on the
// pattern's grid, and the two are added into one sample, so they cannot drift apart.
type patternBed struct {
	slot   proto.PadIndex
	sample formats.Sample
	bpm    float64
}

// slicerBedMsg is a pattern mixed for the slicer, or the end of the pattern (none).
type slicerBedMsg struct {
	slot   proto.PadIndex
	sample formats.Sample
	bpm    float64
	none   bool
	err    error
}

// slicerNextPattern moves P on to the next pattern of the bank that has notes, and after the
// last one to none. The pattern is mixed in the background.
func (m Model) slicerNextPattern() (tea.Model, tea.Cmd) {
	s := m.slicer
	if m.unit == nil || m.snap == nil {
		m.message = "no unit to read patterns from"
		return m, nil
	}
	rows := m.patterns[m.bank]
	if !slices.ContainsFunc(rows, func(r app.PatternRow) bool { return r.Exists }) {
		// Nothing known yet, or a list that may be out of date: read the slots.
		m.ops.Lock()
		var err error
		rows, err = app.LoadPatterns(m.unit, m.bank)
		m.ops.Unlock()
		if err != nil {
			m.message = "patterns: " + err.Error()
			return m, nil
		}
	}
	var slots []proto.PadIndex
	for _, r := range rows {
		if r.Exists {
			slots = append(slots, r.Slot)
		}
	}
	next := 0
	if s.bed != nil {
		for i, slot := range slots {
			if slot == s.bed.slot {
				next = i + 1
			}
		}
	}
	if next >= len(slots) {
		if s.bed == nil {
			m.message = fmt.Sprintf("no pattern in bank %c", 'A'+m.bank)
			return m, nil
		}
		s.bed, s.dirty = nil, true
		m.message = "the slices play alone again"
		return m, nil
	}
	slot := slots[next]
	m.message = "mixing pattern " + slot.String() + "…"
	mix, bpm := m.patternMixer(slot), m.slicerBankTempo()
	return m, func() tea.Msg {
		sample, err := mix()
		return slicerBedMsg{slot: slot, sample: sample, bpm: bpm, err: err}
	}
}

// slicerBedMixed puts a mixed pattern under the slices: the grid takes the pattern's tempo
// (and a 1/16 division when there was none), and the next round plays both.
func (m Model) slicerBedMixed(msg slicerBedMsg) (tea.Model, tea.Cmd) {
	s := m.slicer
	if s == nil {
		return m, nil
	}
	if msg.err != nil {
		m.message = "pattern " + msg.slot.String() + ": " + msg.err.Error()
		return m, nil
	}
	s.bed = &patternBed{slot: msg.slot, sample: msg.sample, bpm: msg.bpm}
	s.sl.BPM = msg.bpm
	if s.sl.Division == 0 {
		s.sl.Division = 16
	}
	s.settle(m)
	m.message = fmt.Sprintf("pattern %s plays with the slices at %g BPM; the tempo follows the pattern", msg.slot, msg.bpm)
	return m, nil
}

// withBed adds the pattern, repeated passes times, to the slices' audio: the result is exactly
// the bed long, in stereo; what the slices play past the end is cut, and a gap is silence.
func withBed(slices, bed formats.Sample, passes int) formats.Sample {
	total := bed.Frames() * passes
	mix := make([]int32, total*2)
	ch := max(slices.Channels, 1)
	for f := 0; f < min(slices.Frames(), total); f++ {
		l := int32(slices.Samples[f*ch])
		r := l
		if ch > 1 {
			r = int32(slices.Samples[f*ch+1])
		}
		mix[f*2], mix[f*2+1] = l, r
	}
	bch := max(bed.Channels, 1)
	for f := 0; f < total; f++ {
		b := f % bed.Frames()
		l := int32(bed.Samples[b*bch])
		r := l
		if bch > 1 {
			r = int32(bed.Samples[b*bch+1])
		}
		mix[f*2] += l
		mix[f*2+1] += r
	}
	out := formats.Sample{Channels: 2, Samples: make([]int16, len(mix))}
	for i, v := range mix {
		out.Samples[i] = int16(max(min(v, 32767), -32768))
	}
	return out
}
