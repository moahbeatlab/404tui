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
func sliceSample(src formats.Sample, o sliceOpts) (formats.Sample, []int) {
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
	out, placed := formats.Render(src, sl, formats.NewSequencer(sl), steps)
	order := make([]int, 0, len(placed))
	for _, pl := range placed {
		order = append(order, pl.Slice+1)
	}
	return out, order
}

// orderText says in which order the slices play, by their numbers (1 is the first of the original), as far as it fits.
func orderText(order []int) string {
	var b strings.Builder
	for i, n := range order {
		if i == 32 {
			b.WriteString(" …")
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprint(&b, n)
	}
	return b.String()
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
		out, _ := sliceSample(sample, o)
		name := "SLC " + time.Now().Format("15:04:05")
		log.Info("slicer: result", "from", row.Pad.String(), "to", target.String(), "slices", o.n, "random_cut", o.randomCut,
			"order", o.order, "div", o.div, "bpm", o.bpm, "frames", out.Frames(), "frames_before", sample.Frames())
		return edit(func() tea.Msg {
			if _, err := app.PutSampleLive(unit, target, out, name); err != nil {
				return doneMsg{message: "the slices were lost: " + err.Error(), reload: true}
			}
			if info, ok := proto.ParamByName("gate"); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(target), 0)
			}
			return doneMsg{message: fmt.Sprintf("%s in %d slices → %s (%s)", row.Pad, o.n, target, strings.TrimSpace(name)), reload: true}
		})()
	}
}

// slicerDoneMsg is a slicing that was rendered and waits for a pad to go on.
type slicerDoneMsg struct {
	out  formats.Sample
	from string
	n    int
	err  error
	// order is how the slices play, by number.
	order string
}

// sliceRender cuts the sample of a pad; the result waits for the pad it goes on.
func (m Model) sliceRender(row app.PadRow, o sliceOpts) (tea.Model, tea.Cmd) {
	if !row.HasSample {
		m.message = fmt.Sprintf("%s holds no sample to slice", row.Pad)
		return m, nil
	}
	unit, ops, log, cache := m.unit, m.ops, m.log, m.padAudio
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
				return slicerDoneMsg{err: fmt.Errorf("could not read %s: %w", row.Pad, err)}
			}
			cache.put(key, sample)
		}
		out, order := sliceSample(sample, o)
		log.Info("slicer: rendered", "from", row.Pad.String(), "slices", o.n, "random_cut", o.randomCut, "mode", o.order, "played", orderText(order),
			"div", o.div, "bpm", o.bpm, "frames", out.Frames(), "frames_before", sample.Frames())
		return slicerDoneMsg{out: out, from: row.Pad.String(), n: o.n, order: orderText(order)}
	}
}

// slicerDone takes the rendered slices: they wait, and the page asks for the pad.
func (m Model) slicerDone(msg slicerDoneMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		m.message = msg.err.Error()
		return m, nil
	}
	lp := m.looper()
	lp.slOut, lp.slHave, lp.slFrom, lp.slN, lp.slOrder = msg.out, true, msg.from, msg.n, msg.order
	lp.slDest = -1
	if free, ok := m.slFreePad(m.slTargetBank()); ok {
		lp.slDest = int(free.Pad())
	}
	m.message = fmt.Sprintf("%s in %d slices, order %s: choose the pad it goes on, then PLACE", msg.from, msg.n, msg.order)
	return m, nil
}

// slicerPlace puts the waiting slices on the pad chosen, ungated.
func (m Model) slicerPlace() (tea.Model, tea.Cmd) {
	lp := m.looper()
	tb := m.slTargetBank()
	if !lp.slHave || lp.slDest < 0 || lp.slDest > 15 {
		m.message = "tap the pad the slices go on first"
		return m, nil
	}
	target := m.snap.Bank(tb)[lp.slDest].Pad
	out, from, n := lp.slOut, lp.slFrom, lp.slN
	lp.slHave, lp.slOut = false, formats.Sample{}
	unit, edit := m.unit, m.edit
	name := "SLC " + time.Now().Format("15:04:05")
	m.busy = "putting the slices on " + target.String()
	return m, edit(func() tea.Msg {
		if _, err := app.PutSampleLive(unit, target, out, name); err != nil {
			return doneMsg{message: "the slices were lost: " + err.Error(), reload: true}
		}
		if info, ok := proto.ParamByName("gate"); ok {
			_ = unit.SetParam(info.ID, proto.TargetPad(target), 0)
		}
		return doneMsg{message: fmt.Sprintf("%s in %d slices → %s (%s)", from, n, target, name), reload: true}
	})
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
	if act == "sldel" {
		// Holding a pad deletes it, in the bank the grid shows.
		grid := bank
		if lp.slHave {
			grid = m.slTargetBank()
		}
		if !lp.slHave && i == pad {
			lp.slPad = -1
		}
		return m.deletePadIn(grid, i)
	}
	if lp.slHave {
		// The slices wait: the grid is the bank they go to.
		switch act {
		case "slpad":
			if i >= 0 && i <= 15 {
				lp.slDest = i
			}
		case "sltbank":
			cur := m.slTargetBank()
			m.mem.SlicerTarget = (cur+n+proto.Banks)%proto.Banks + 1
			lp.slDest = -1
			if free, ok := m.slFreePad(m.slTargetBank()); ok {
				lp.slDest = int(free.Pad())
			}
		case "slgo":
			return m.slicerPlace()
		case "slagain":
			return m.sliceRender(m.snap.Bank(bank)[pad], m.sliceOptions(bank))
		case "slcancel":
			lp.slHave, lp.slOut = false, formats.Sample{}
			m.message = "the slices were dropped"
		}
		return m, nil
	}
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
		return m.sliceRender(m.snap.Bank(bank)[pad], m.sliceOptions(bank))
	}
	return m, nil
}

// sliceOptions are the settings of the page's Slicer, for a pad of a bank.
func (m Model) sliceOptions(bank int) sliceOpts {
	return sliceOpts{n: m.slSlices(), randomCut: m.mem.SlicerRandomCut, order: m.slOrder(), div: m.mem.SlicerDiv, swing: m.mem.SlicerSwing, bpm: m.slBPM(bank)}
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
	// Phase is pick (the grid is the bank the pad comes from) or ready (the slices wait, and the grid is the bank they go to,
	// Dest the pad chosen, -1 for none, DestName what it holds now).
	Phase    string `json:"phase"`
	Dest     int    `json:"dest"`
	DestName string `json:"destName"`
	// Order is how the waiting slices play, by number.
	Played string `json:"played"`
}

func (m Model) webSlicer() webSlicer {
	bank, pad := m.slSource()
	tb := m.slTargetBank()
	s := webSlicer{Bank: string(rune('A' + bank)), Pad: pad, Pads: []webLoopPad{}, Slices: m.slSlices(), Random: m.mem.SlicerRandomCut,
		Order: slOrderNames[m.slOrder()], Div: m.mem.SlicerDiv, Swing: m.mem.SlicerSwing, Target: string(rune('A' + tb)), Busy: m.busy != ""}
	s.Phase, s.Dest = "pick", -1
	if free, ok := m.slFreePad(tb); ok {
		s.Free = free.String()
	}
	for i, row := range m.snap.Bank(bank) {
		if i == pad {
			s.Src = row.Pad.String() + " " + row.Name
		}
	}
	grid := bank
	if lp := m.lp; lp != nil && lp.slHave {
		s.Phase, s.Dest, grid, s.Free = "ready", lp.slDest, tb, ""
		s.Src = lp.slFrom + " in " + fmt.Sprint(lp.slN) + " slices"
		s.Played = lp.slOrder
	}
	for i, row := range m.snap.Bank(grid) {
		s.Pads = append(s.Pads, webLoopPad{N: i + 1, Name: row.Name, Has: row.HasSample})
		if s.Phase == "ready" && i == s.Dest {
			s.Free, s.DestName = row.Pad.String(), ""
			if row.HasSample {
				s.DestName = row.Name
			}
		}
	}
	return s
}
