package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/formats"
)

// A remix takes the sample of a pad of the Looper's bank, cuts it in a number of equal slices, plays them in a random
// order (every slice once, so the loop is as long as before) and puts the result on the next free pad of the bank, ungated.

// loopSliceChoices are the numbers of slices the s key goes through.
var loopSliceChoices = []int{4, 8, 16, 32}

// loopSlices is how many slices a remix cuts.
func (m Model) loopSlices() int {
	for _, n := range loopSliceChoices {
		if n == m.mem.LooperSlices {
			return n
		}
	}
	return 8
}

// looperSlicesNext goes to the next number of slices.
func (m Model) looperSlicesNext() (tea.Model, tea.Cmd) {
	n := m.loopSlices()
	next := loopSliceChoices[0]
	for i, c := range loopSliceChoices {
		if c == n {
			next = loopSliceChoices[(i+1)%len(loopSliceChoices)]
		}
	}
	m.mem.LooperSlices = next
	m.message = fmt.Sprintf("a remix cuts a loop in %d slices and plays them in a random order (x remixes the pad in front)", next)
	return m, nil
}

// remixSample cuts a sample in n equal slices and puts them in a random order, each once.
func remixSample(src formats.Sample, n int) formats.Sample {
	out, _ := sliceSample(src, sliceOpts{n: n, order: slOrderShuffle})
	return out
}

// looperRemix remixes the pad i (0 to 15) of the bank onto the next free pad of it.
func (m Model) looperRemix(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i > 15 || m.snap == nil || m.unit == nil {
		return m, nil
	}
	row := m.snap.Bank(m.loopBank())[i]
	target, ok := m.loopFreePad()
	if !ok {
		m.message = fmt.Sprintf("bank %c is full: d deletes a loop", 'A'+m.loopBank())
		return m, nil
	}
	return m.sliceJob(row, target, sliceOpts{n: m.loopSlices(), order: slOrderShuffle})
}
