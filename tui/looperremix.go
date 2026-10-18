package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
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
	sl := formats.NewSlicing(src.Frames(), n)
	for try := 0; try < 8; try++ {
		shuffleOrder(sl)
		if !inOrder(sl.Order) {
			break
		}
	}
	out, _ := formats.Render(src, sl, formats.NewSequencer(sl), len(sl.Order))
	return out
}

func inOrder(order []int) bool {
	for i, v := range order {
		if v != i {
			return false
		}
	}
	return true
}

// looperRemix remixes the pad i (0 to 15) of the bank onto the next free pad of it.
func (m Model) looperRemix(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i > 15 || m.snap == nil || m.unit == nil {
		return m, nil
	}
	row := m.snap.Bank(m.loopBank())[i]
	if !row.HasSample {
		m.message = fmt.Sprintf("%s holds no sample to remix", row.Pad)
		return m, nil
	}
	target, ok := m.loopFreePad()
	if !ok {
		m.message = fmt.Sprintf("bank %c is full: d deletes a loop", 'A'+m.loopBank())
		return m, nil
	}
	n := m.loopSlices()
	unit, ops, log, cache, edit := m.unit, m.ops, m.log, m.padAudio, m.edit
	key := factKey{row.Pad, row.Size, row.Name}
	m.busy = "remixing " + row.Pad.String()
	return m, func() tea.Msg {
		sample, ok := cache.get(key)
		if !ok {
			ops.Lock()
			var err error
			sample, err = app.PadAudio(unit, row)
			ops.Unlock()
			if err != nil {
				return doneMsg{message: "could not read " + row.Pad.String() + ": " + err.Error(), reload: true}
			}
			cache.put(key, sample)
		}
		out := remixSample(sample, n)
		name := "RMX " + time.Now().Format("15:04:05")
		log.Info("looper: remix", "from", row.Pad.String(), "to", target.String(), "slices", n, "frames", out.Frames(), "frames_before", sample.Frames())
		return edit(func() tea.Msg {
			if _, err := app.PutSample(unit, target, out, name); err != nil {
				return doneMsg{message: "the remix was lost: " + err.Error(), reload: true}
			}
			if info, ok := proto.ParamByName("gate"); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(target), 0)
			}
			return doneMsg{message: fmt.Sprintf("%s remixed in %d slices → %s (%s): it plays from the next round of the pattern", row.Pad, n, target, strings.TrimSpace(name)), reload: true}
		})()
	}
}
