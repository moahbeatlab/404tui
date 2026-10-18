package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// The basic Slicer of the web page: a pad of any bank is cut in a number of slices, played in an order, and the result goes
// on the next free pad of a bank, ungated. No waveform: the number of slices, how they are cut, their order and their timing.

const (
	slOrderShuffle = iota // every slice once, in a random order
	slOrderInOrder        // the slices as they are
	slOrderRandom         // each step a random slice (some come twice, some not at all)
	slOrders
)

var (
	slOrderNames   = []string{"shuffle", "in order", "random"}
	slSliceChoices = []int{2, 4, 8, 16, 32}
	slDivChoices   = []int{0, 4, 8, 16}
	slSwingChoices = []int{0, 54, 58, 62, 66}
)

// sliceOpts is what a slicing does with a sample.
type sliceOpts struct {
	n         int
	randomCut bool // cut at random places, not in equal slices
	order     int
	// div puts the steps on a grid of notes to the bar at bpm (0: the slices play at their own length); swing is 0 or
	// the percent of the late steps.
	div, swing int
	bpm        float64
}

// sliceSample cuts a sample and puts the slices back together. Without a grid it is as long as the sample, with a grid
// it has as many steps as it takes to be as long.
func sliceSample(src formats.Sample, o sliceOpts) formats.Sample {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	sl := formats.NewSlicing(src.Frames(), max(o.n, 1))
	if o.randomCut {
		sl.DivideRandom(max(o.n, 1), rng)
	}
	sl.Seed = rng.Int63()
	switch o.order {
	case slOrderShuffle:
		for try := 0; try < 8; try++ {
			shuffleOrder(sl)
			if !inOrder(sl.Order) {
				break
			}
		}
	case slOrderRandom:
		sl.Mode = formats.ModeRandom
	}
	steps := len(sl.Order)
	if o.div > 0 && o.bpm > 0 {
		sl.Division, sl.BPM, sl.Swing = o.div, o.bpm, o.swing
		if step := sl.StepFrames(); step > 0 {
			steps = max((src.Frames()+step/2)/step, 1)
		}
	}
	out, _ := formats.Render(src, sl, formats.NewSequencer(sl), steps)
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

// sliceJob cuts the sample of a pad and puts the result on target, ungated.
func (m Model) sliceJob(row app.PadRow, target proto.PadIndex, o sliceOpts) (tea.Model, tea.Cmd) {
	if !row.HasSample {
		m.message = fmt.Sprintf("%s holds no sample to slice", row.Pad)
		return m, nil
	}
	unit, ops, log, cache, edit := m.unit, m.ops, m.log, m.padAudio, m.edit
	key := factKey{row.Pad, row.Size, row.Name}
	m.busy = "slicing " + row.Pad.String()
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
		out := sliceSample(sample, o)
		name := "SLC " + time.Now().Format("15:04:05")
		log.Info("slicer: result", "from", row.Pad.String(), "to", target.String(), "slices", o.n, "random_cut", o.randomCut,
			"order", o.order, "div", o.div, "bpm", o.bpm, "frames", out.Frames(), "frames_before", sample.Frames())
		return edit(func() tea.Msg {
			if _, err := app.PutSample(unit, target, out, name); err != nil {
				return doneMsg{message: "the slices were lost: " + err.Error(), reload: true}
			}
			if info, ok := proto.ParamByName("gate"); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(target), 0)
			}
			return doneMsg{message: fmt.Sprintf("%s in %d slices → %s (%s)", row.Pad, o.n, target, strings.TrimSpace(name)), reload: true}
		})()
	}
}

// ---- the page's Slicer ----

func (m Model) slSlices() int {
	for _, n := range slSliceChoices {
		if n == m.mem.SlicerSlices {
			return n
		}
	}
	return 8
}

func (m Model) slOrder() int {
	if m.mem.SlicerOrder < 0 || m.mem.SlicerOrder >= slOrders {
		return slOrderShuffle
	}
	return m.mem.SlicerOrder
}

// slTargetBank is the bank the result goes to: the Looper's unless one was chosen.
func (m Model) slTargetBank() int {
	if m.mem.SlicerTarget >= 1 && m.mem.SlicerTarget <= proto.Banks {
		return m.mem.SlicerTarget - 1
	}
	return m.loopBank()
}

// slSource is the bank the pad comes from.
func (m Model) slSource() (bank, pad int) {
	lp := m.looper()
	if !lp.slSet {
		lp.slSet, lp.slBank, lp.slPad = true, m.loopBank(), -1
	}
	return lp.slBank, lp.slPad
}

func (m Model) slFreePad(bank int) (proto.PadIndex, bool) {
	for _, row := range m.snap.Bank(bank) {
		if !row.HasSample {
			return row.Pad, true
		}
	}
	return proto.PadIndex{}, false
}

// slBPM is the tempo of a bank for a grid: the bank's, the project's, or the clock's.
func (m Model) slBPM(bank int) float64 {
	if m.snap.UsesProjectTempo && m.snap.ProjectTempo > 0 {
		return m.snap.ProjectTempo
	}
	if t := m.snap.Banks[bank].Tempo; t > 0 {
		return t
	}
	if m.mon != nil && m.mon.bpm > 0 {
		return m.mon.bpm
	}
	return 120
}

func cycleInt(list []int, cur, dir int) int {
	at := 0
	for i, v := range list {
		if v == cur {
			at = i
		}
	}
	return list[(at+dir+len(list))%len(list)]
}

// slicerCommand handles the buttons of the page's Slicer.
func (m Model) slicerCommand(act string, n, i int) (tea.Model, tea.Cmd) {
	if m.snap == nil || m.busy != "" {
		return m, nil
	}
	bank, pad := m.slSource()
	lp := m.looper()
	switch act {
	case "slbank":
		lp.slBank = (bank + n + proto.Banks) % proto.Banks
		lp.slPad = -1
	case "slpad":
		if i < 0 || i > 15 {
			return m, nil
		}
		if i == pad {
			i = -1
		}
		lp.slPad = i
	case "sltbank":
		cur := m.slTargetBank()
		m.mem.SlicerTarget = (cur+n+proto.Banks)%proto.Banks + 1
	case "slslices":
		m.mem.SlicerSlices = cycleInt(slSliceChoices, m.slSlices(), 1)
	case "slcut":
		m.mem.SlicerRandomCut = !m.mem.SlicerRandomCut
	case "slorder":
		m.mem.SlicerOrder = (m.slOrder() + 1) % slOrders
	case "sldiv":
		m.mem.SlicerDiv = cycleInt(slDivChoices, m.mem.SlicerDiv, 1)
	case "slswing":
		m.mem.SlicerSwing = cycleInt(slSwingChoices, m.mem.SlicerSwing, 1)
	case "slgo":
		if pad < 0 {
			m.message = "tap the pad to slice first"
			return m, nil
		}
		tb := m.slTargetBank()
		target, ok := m.slFreePad(tb)
		if !ok {
			m.message = fmt.Sprintf("bank %c is full: delete a pad first", 'A'+tb)
			return m, nil
		}
		o := sliceOpts{n: m.slSlices(), randomCut: m.mem.SlicerRandomCut, order: m.slOrder(), div: m.mem.SlicerDiv, swing: m.mem.SlicerSwing}
		o.bpm = m.slBPM(bank)
		return m.sliceJob(m.snap.Bank(bank)[pad], target, o)
	}
	return m, nil
}

// webSlicer is the Slicer for the page.
type webSlicer struct {
	Bank   string       `json:"bank"`
	Pad    int          `json:"pad"` // -1 for none
	Pads   []webLoopPad `json:"pads"`
	Slices int          `json:"slices"`
	Random bool         `json:"random"` // cut at random places
	Order  string       `json:"order"`
	Div    int          `json:"div"`
	Swing  int          `json:"swing"`
	Target string       `json:"target"` // the bank
	Free   string       `json:"free"`   // its next free pad, empty when full
	Src    string       `json:"src"`    // the pad that is cut
	Busy   bool         `json:"busy"`
}

func (m Model) webSlicer() webSlicer {
	bank, pad := m.slSource()
	tb := m.slTargetBank()
	s := webSlicer{Bank: string(rune('A' + bank)), Pad: pad, Pads: []webLoopPad{}, Slices: m.slSlices(), Random: m.mem.SlicerRandomCut,
		Order: slOrderNames[m.slOrder()], Div: m.mem.SlicerDiv, Swing: m.mem.SlicerSwing, Target: string(rune('A' + tb)), Busy: m.busy != ""}
	if free, ok := m.slFreePad(tb); ok {
		s.Free = free.String()
	}
	for i, row := range m.snap.Bank(bank) {
		s.Pads = append(s.Pads, webLoopPad{N: i + 1, Name: row.Name, Has: row.HasSample})
		if i == pad {
			s.Src = row.Pad.String() + " " + row.Name
		}
	}
	return s
}
