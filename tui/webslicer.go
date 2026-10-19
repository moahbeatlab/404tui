package tui

import (
	"fmt"
	"math/rand"
	"slices"
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
	slDivChoices   = []int{0, 2, 4, 8, 16}
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
	mangle     mangleOpts
	// seed fixes every roll (the cuts, the order, the dice) so that a change of one setting leaves the rest as it was; 0 rolls anew.
	seed int64
}

// mangleOpts are the dice: how many of the slices drop out, play backwards, get a pitch (up or down by up to pitch semitones)
// or a roll, how much the loudness and the timing wander (percent), and the fades at the ends of the slices (ms).
type mangleOpts struct {
	drop, rev, pitchN, roll, human, fade int
	pitch                                int // the most semitones a pitched slice moves
}

// The choices of the dice that are not a number of slices, in the order of the page's list.
var (
	manglePitch = []int{2, 5, 12}
	mangleHuman = []int{0, 25, 50, 100}
	mangleFade  = []int{0, 10, 50, 200}
)

// values are the dice in the page's order, the last one the pitch range.
func (o mangleOpts) values() []int {
	return []int{o.drop, o.rev, o.pitchN, o.roll, o.human, o.fade, o.pitch}
}

func (o mangleOpts) active() int {
	n := 0
	for _, v := range o.values()[:6] {
		if v != 0 {
			n++
		}
	}
	return n
}

// sliceSample cuts a sample and puts the slices back together. Without a grid it is as long as the sample, with a grid
// it has as many steps as it takes to be as long. Whatever the dice do, the result has the length of the sample, so that it
// still fits the pattern. It also says in which order the slices play.
func sliceSample(src formats.Sample, o sliceOpts) (formats.Sample, []int) {
	seed := o.seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	// The dice have a generator of their own that draws the same numbers for every slice whatever they are set to, so that
	// turning one die does not change what the others did.
	dice := rand.New(rand.NewSource(seed ^ 0x2545f4914f6cdd1d))
	n := max(o.n, 1)
	sl := formats.NewSlicing(src.Frames(), n)
	if o.randomCut {
		sl.DivideRandom(n, rng)
	}
	sl.Seed = rng.Int63()
	switch o.order {
	case slOrderShuffle:
		for try := 0; try < 8; try++ {
			shuffleOrderSeed(sl, seed+int64(try))
			if !inOrder(sl.Order) {
				break
			}
		}
	case slOrderRandom:
		// Picks by chance, made here so that the dice below work in every order.
		sl.Order = make([]int, len(sl.Slices))
		for i := range sl.Order {
			sl.Order[i] = rng.Intn(len(sl.Slices))
		}
	}
	mg := o.mangle
	mod := src
	if mg.pitchN > 0 {
		// A pitched slice is as long as its slot again: shorter ones end in silence, longer ones are cut, so that the
		// rhythm stays where it is.
		mod = formats.Sample{Channels: src.Channels, Samples: slices.Clone(src.Samples)}
	}
	ch := max(src.Channels, 1)
	// Each effect has its own order of the slices, fixed by the seed, and takes the first few of it: one more slice added
	// to an effect leaves the ones it had.
	chosen := func(k int) []bool {
		is := make([]bool, len(sl.Slices))
		for _, i := range dice.Perm(len(sl.Slices))[:min(max(k, 0), len(sl.Slices))] {
			is[i] = true
		}
		return is
	}
	drop, rev, pitched, roll := chosen(mg.drop), chosen(mg.rev), chosen(mg.pitchN), chosen(mg.roll)
	for i := range sl.Slices {
		sc := &sl.Slices[i]
		dRollPick, dPitch := dice.Intn(len(formats.Rolls)-1), dice.Float64()
		sc.Chance = 100
		if drop[i] {
			sc.Chance = 0
		}
		sc.Reverse = rev[i]
		sc.Attack, sc.Release = mg.fade, mg.fade
		if roll[i] {
			sc.Roll = formats.Rolls[1+dRollPick]
		}
		if pitched[i] {
			if p := int(dPitch*float64(2*mg.pitch+1)) - mg.pitch; p != 0 {
				from, to := min(sc.Start, src.Frames()), min(sc.End, src.Frames())
				part := formats.Sample{Channels: src.Channels, Samples: src.Samples[from*ch : to*ch]}.Pitched(p)
				region := mod.Samples[from*ch : to*ch]
				clear(region)
				copy(region, part.Samples)
			}
		}
	}
	sl.Humanize = mg.human
	sl.Mode = formats.ModeOrder
	steps := len(sl.Order)
	if o.div > 0 && o.bpm > 0 {
		sl.Division, sl.BPM, sl.Swing = o.div, o.bpm, o.swing
		if step := sl.StepFrames(); step > 0 {
			steps = max((src.Frames()+step/2)/step, 1)
		}
	}
	out, placed := formats.Render(mod, sl, formats.NewSequencer(sl), steps)
	order := make([]int, 0, len(placed))
	for _, pl := range placed {
		order = append(order, pl.Slice+1)
	}
	return fitFrames(out, src.Frames()), order
}

// fitFrames makes a sample exactly frames long: the end is cut (with a short fade) or silence is added.
func fitFrames(s formats.Sample, frames int) formats.Sample {
	ch := max(s.Channels, 1)
	switch have := s.Frames(); {
	case have == frames:
		return s
	case have > frames:
		s.Samples = s.Samples[:frames*ch]
		fade := min(64, frames)
		for f := 0; f < fade; f++ {
			g := float64(f) / float64(fade)
			for c := 0; c < ch; c++ {
				i := (frames-1-f)*ch + c
				s.Samples[i] = int16(float64(s.Samples[i]) * g)
			}
		}
	default:
		s.Samples = append(s.Samples, make([]int16, (frames-have)*ch)...)
	}
	return s
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
			msg := doneMsg{message: fmt.Sprintf("%s in %d slices → %s (%s), muted: hold it to bring it in", row.Pad, o.n, target, strings.TrimSpace(name)), reload: true}
			if muteNewPad(unit, target) {
				msg.muted = []proto.PadIndex{target}
			} else {
				msg.message = fmt.Sprintf("%s in %d slices → %s (%s)", row.Pad, o.n, target, strings.TrimSpace(name))
			}
			return msg
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
	had := lp.slHave
	lp.slOut, lp.slHave, lp.slFrom, lp.slN, lp.slOrder = msg.out, true, msg.from, msg.n, msg.order
	if !had || lp.slDest < 0 {
		lp.slDest = -1
		if free, ok := m.slFreePad(m.slTargetBank()); ok {
			lp.slDest = int(free.Pad())
		}
	}
	m.message = fmt.Sprintf("%s in %d slices, order %s: choose the pad it goes on, then PLACE", msg.from, msg.n, msg.order)
	if lp.slPrev {
		m.message = fmt.Sprintf("%s in %d slices, order %s: looping; change the settings, then PLACE", msg.from, msg.n, msg.order)
		if lp.slDirty {
			lp.slDirty = false
			return m.slPreviewRefresh()
		}
		return m.slPreviewPlay()
	}
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
	lp.slPrev = false
	m = m.slPreviewStop()
	unit, edit := m.unit, m.edit
	name := "SLC " + time.Now().Format("15:04:05")
	cmd := edit(func() tea.Msg {
		if _, err := app.PutSampleLive(unit, target, out, name); err != nil {
			return doneMsg{message: "the slices were lost: " + err.Error(), reload: true}
		}
		if info, ok := proto.ParamByName("gate"); ok {
			_ = unit.SetParam(info.ID, proto.TargetPad(target), 0)
		}
		msg := doneMsg{message: fmt.Sprintf("%s in %d slices → %s (%s), muted: hold it to bring it in", from, n, target, name), reload: true}
		if muteNewPad(unit, target) {
			msg.muted = []proto.PadIndex{target}
		} else {
			msg.message = fmt.Sprintf("%s in %d slices → %s (%s)", from, n, target, name)
		}
		return msg
	})
	if m.looperLater(lpJob{key: "place:" + target.String(), label: "slices → " + target.String(), cmd: cmd}) {
		m.message = fmt.Sprintf("the slices go on %s on %s, in %.1f beats", target, m.laterName(), m.beatsToLater())
		return m, nil
	}
	m.busy = "putting the slices on " + target.String()
	return m, cmd
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

// pickInt is the choice number i of the list, or the one after cur when i is not one (the button of the terminal-like cycle).
func pickInt(list []int, cur, i int) int {
	if i >= 0 && i < len(list) {
		return list[i]
	}
	return cycleInt(list, cur, 1)
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
	if m.snap == nil {
		return m, nil
	}
	if m.busy != "" && !lpSettings[act] && act != "slmute" {
		m.message = "busy (" + m.busy + "): try again in a moment"
		return m, nil
	}
	m.looper() // made here, so that what is chosen stays with the model that goes back
	bank, pad := m.slSource()
	lp := m.looper()
	if act == "slmute" {
		// Holding a pad mutes it (again brings it back), in the bank the grid shows.
		grid := bank
		if lp.slHave {
			grid = m.slTargetBank()
		}
		if i < 0 || i > 15 || !m.snap.Bank(grid)[i].HasSample {
			return m, nil
		}
		pad := m.snap.Bank(grid)[i].Pad
		was, silent := m.lvlMuted(pad), m.snap.Pads[pad.Index()].Level == 0
		nm, cmd := m.levelMutePad(pad)
		switch {
		case nm.lvlMuted(pad):
			nm.message = fmt.Sprintf("%s is muted: hold it again to bring it back", pad)
		case was:
			nm.message = fmt.Sprintf("%s is back", pad)
		case silent:
			nm.message = fmt.Sprintf("%s is silent already (level 0)", pad)
		}
		return nm, cmd
	}
	if lp.slHave {
		// The slices wait: the grid is the bank they go to.
		switch act {
		case "slslices", "slcut", "slorder", "sldiv", "slswing", "slmang":
			if lp.slPrev {
				return m.slParam(act, n, i).slPreviewRefresh()
			}
		case "slprev":
			return m.slPreviewToggle()
		case "slpad":
			if i >= 0 && i <= 15 {
				lp.slDest = i
			}
		case "sltbank", "sltbankset":
			cur := m.slTargetBank()
			if act == "sltbankset" {
				n = i - cur
			}
			m.mem.SlicerTarget = (cur+n+proto.Banks)%proto.Banks + 1
			lp.slDest = -1
			if free, ok := m.slFreePad(m.slTargetBank()); ok {
				lp.slDest = int(free.Pad())
			}
		case "slgo":
			return m.slicerPlace()
		case "slagain":
			lp.slSeed = time.Now().UnixNano()
			return m.sliceRender(m.snap.Bank(bank)[pad], m.sliceOptions(bank))
		case "slcancel":
			lp.slHave, lp.slOut = false, formats.Sample{}
			lp.slPrev = false
			m = m.slPreviewStop()
			m.message = "the slices were dropped"
		}
		return m, nil
	}
	switch act {
	case "slbank", "slbankset":
		if act == "slbankset" {
			n = i - bank
		}
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
	case "sltbank", "sltbankset":
		cur := m.slTargetBank()
		if act == "sltbankset" {
			n = i - cur
		}
		m.mem.SlicerTarget = (cur+n+proto.Banks)%proto.Banks + 1
	case "slslices", "slmang", "slcut", "slorder", "sldiv", "slswing":
		m = m.slParam(act, n, i)
	case "slprev":
		return m.slPreviewToggle()
	case "slgo":
		if pad < 0 {
			m.message = "tap the pad to slice first"
			return m, nil
		}
		lp.slSeed = time.Now().UnixNano()
		return m.sliceRender(m.snap.Bank(bank)[pad], m.sliceOptions(bank))
	}
	return m, nil
}

// slParam changes one setting of the Slicer.
func (m Model) slParam(act string, n, i int) Model {
	switch act {
	case "slslices":
		m.mem.SlicerSlices = pickInt(slSliceChoices, m.slSlices(), i)
	case "slmang":
		m = m.slMangle(i, n)
	case "slcut":
		m.mem.SlicerRandomCut = !m.mem.SlicerRandomCut
		if i >= 0 {
			m.mem.SlicerRandomCut = i == 1
		}
	case "slorder":
		m.mem.SlicerOrder = (m.slOrder() + 1) % slOrders
		if i >= 0 && i < slOrders {
			m.mem.SlicerOrder = i
		}
	case "sldiv":
		m.mem.SlicerDiv = pickInt(slDivChoices, m.mem.SlicerDiv, i)
	case "slswing":
		m.mem.SlicerSwing = pickInt(slSwingChoices, m.mem.SlicerSwing, i)
	}
	return m
}

// sliceOptions are the settings of the page's Slicer, for a pad of a bank.
func (m Model) sliceOptions(bank int) sliceOpts {
	return sliceOpts{n: m.slSlices(), randomCut: m.mem.SlicerRandomCut, order: m.slOrder(), div: m.mem.SlicerDiv, swing: m.mem.SlicerSwing, bpm: m.slBPM(bank),
		mangle: m.mangleOptions(), seed: m.looper().slSeed}
}

// mangleOptions are the dice as they are set: the counts cannot be more than the slices.
func (m Model) mangleOptions() mangleOpts {
	pick := func(list []int, v int) int {
		if slices.Contains(list, v) {
			return v
		}
		return list[0]
	}
	n := m.slSlices()
	return mangleOpts{drop: min(m.mem.SlicerDrop, n), rev: min(m.mem.SlicerRev, n), pitchN: min(m.mem.SlicerPitchN, n), roll: min(m.mem.SlicerRoll, n),
		human: pick(mangleHuman, m.mem.SlicerHuman), fade: pick(mangleFade, m.mem.SlicerFade), pitch: pickPitch(m.mem.SlicerPitch)}
}

// pickPitch is the pitch range as it is set, 5 semitones to begin with.
func pickPitch(v int) int {
	if slices.Contains(manglePitch, v) {
		return v
	}
	return 5
}

// slMangle steps a die (field 0 to 5, or 7 for the pitch range) up or down; field 6 puts them all back to 0. The first four
// count slices, from 0 to all of them.
func (m Model) slMangle(field, dir int) Model {
	if field == 6 {
		m.mem.SlicerDrop, m.mem.SlicerRev, m.mem.SlicerPitchN, m.mem.SlicerRoll, m.mem.SlicerHuman, m.mem.SlicerFade = 0, 0, 0, 0, 0, 0
		return m
	}
	o := m.mangleOptions()
	n := m.slSlices()
	count := func(cur int) int { return min(max(cur+dir, 0), n) }
	switch field {
	case 0:
		m.mem.SlicerDrop = count(o.drop)
	case 1:
		m.mem.SlicerRev = count(o.rev)
	case 2:
		m.mem.SlicerPitchN = count(o.pitchN)
	case 3:
		m.mem.SlicerRoll = count(o.roll)
	case 4:
		m.mem.SlicerHuman = cycleInt(mangleHuman, o.human, dir)
	case 5:
		m.mem.SlicerFade = cycleInt(mangleFade, o.fade, dir)
	case 7:
		m.mem.SlicerPitch = cycleInt(manglePitch, o.pitch, dir)
	}
	return m
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
	// Later says what waits for the last beat of the pattern.
	Later string `json:"later"`
	// Prev is the preview mode; Loop says how it is going: sending, waiting for beat 1 or looping.
	Prev bool   `json:"prev"`
	Loop string `json:"loop"`
	// Mangle are the dice (drop, reverse, pitch, roll, humanize, fade) and Active how many are not 0.
	Mangle []int `json:"mangle"`
	Active int   `json:"active"`
}

func (m Model) webSlicer() webSlicer {
	bank, pad := m.slSource()
	tb := m.slTargetBank()
	s := webSlicer{Bank: string(rune('A' + bank)), Pad: pad, Pads: []webLoopPad{}, Slices: m.slSlices(), Random: m.mem.SlicerRandomCut,
		Order: slOrderNames[m.slOrder()], Div: m.mem.SlicerDiv, Swing: m.mem.SlicerSwing, Target: string(rune('A' + tb)), Busy: m.busy != ""}
	s.Phase, s.Dest, s.Later = "pick", -1, m.laterText()
	mg := m.mangleOptions()
	s.Mangle, s.Active = mg.values(), mg.active()
	if free, ok := m.slFreePad(tb); ok {
		s.Free = free.String()
	}
	for i, row := range m.snap.Bank(bank) {
		if i == pad {
			s.Src = row.Pad.String() + " " + row.Name
		}
	}
	grid := bank
	if lp := m.lp; lp != nil {
		s.Prev, s.Loop = lp.slPrev, m.slPreviewState()
	}
	if lp := m.lp; lp != nil && lp.slHave {
		s.Phase, s.Dest, grid, s.Free = "ready", lp.slDest, tb, ""
		s.Src = lp.slFrom + " in " + fmt.Sprint(lp.slN) + " slices"
		s.Played = lp.slOrder
	}
	for i, row := range m.snap.Bank(grid) {
		s.Pads = append(s.Pads, webLoopPad{N: i + 1, Name: row.Name, Has: row.HasSample, Muted: m.lvlMuted(row.Pad)})
		if s.Phase == "ready" && i == s.Dest {
			s.Free, s.DestName = row.Pad.String(), ""
			if row.HasSample {
				s.DestName = row.Name
			}
		}
	}
	return s
}
