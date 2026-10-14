package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// slicerTickEvery is how often the slicer moves its playhead and looks whether the next round
// has to be queued; tests shorten it.
var slicerTickEvery = 40 * time.Millisecond

// slicerEqualCounts are the numbers of slices e steps through.
var slicerEqualCounts = []int{4, 8, 16, 32}

// slicerSensitivities are the onset sensitivities t steps through.
var slicerSensitivities = []float64{0.5, 0.7, 0.9, 0.3}

// slicerRounds are the numbers of rounds a bounce can hold.
var slicerRounds = []int{1, 2, 4, 8}

// slicerMinRound is the shortest a round that is looped is made, by playing the pass over
// again: a round is rendered ahead of time, and a short one would leave no time for that.
const slicerMinRound = 1200 * time.Millisecond

const slicerUndoDepth = 100

// slicer is the dialog that cuts a sample into slices and plays them back in an order, or at
// random, on this computer; a bounce renders what it plays onto a pad.
type slicer struct {
	title string
	// name is what a bounce is called on the pad.
	name string
	// key is what the slicing is kept under between runs.
	key string
	src formats.Sample
	sl  *formats.Slicing
	// at is the slice in front; lane is the place in the order that has the cursor.
	at, lane  int
	focusLane bool
	// view is the part of the sample on screen: the whole of it, or one slice and its
	// surroundings.
	zoomed bool
	// snap makes a moved start settle on a zero crossing, so the cut does not click.
	snap   bool
	coarse bool
	sens   int
	equal  int
	rounds int
	undo   []*formats.Slicing
	peak   int

	// Playback: the round sounding and the one queued after it.
	playing  bool
	gen      int
	round    int
	cur, nxt *slicerRound
	dirty    bool
}

// slicerRound is a stretch of the sequence rendered ahead: what the player has, and where each
// step is in it.
type slicerRound struct {
	sample formats.Sample
	placed []formats.Placed
	start  time.Time
	dur    time.Duration
}

type (
	// slicerLoadedMsg is the sample a slicer was waiting for.
	slicerLoadedMsg struct {
		key, title, name string
		sample           formats.Sample
		err              error
	}
	slicerTickMsg struct{ gen int }
)

// ---- opening ----

// openSlicerPad opens the slicer on a pad's sample, reading it from the unit unless that was
// done before.
func (m Model) openSlicerPad(row app.PadRow) (tea.Model, tea.Cmd) {
	if !row.HasSample {
		m.message = fmt.Sprintf("%s is empty; a assigns a file to it", row.Pad)
		return m, nil
	}
	m.stopListening()
	key := factKey{row.Pad, row.Size, row.Name}
	title := fmt.Sprintf("%s  %s", row.Pad, row.Name)
	if sample, ok := m.padAudio.get(key); ok {
		return m.openSlicer("pad|"+row.Name, title, row.Name, sample)
	}
	m.message = "reading " + row.Pad.String() + " from the unit…"
	unit, ops, log, cache := m.unit, m.ops, m.log, m.padAudio
	return m, func() tea.Msg {
		ops.Lock()
		sample, err := app.PadAudio(unit, row)
		ops.Unlock()
		if err != nil {
			log.Warn("could not read the pad to slice it", "pad", row.Pad.String(), "err", err)
			return slicerLoadedMsg{err: err}
		}
		cache.put(key, sample)
		return slicerLoadedMsg{key: "pad|" + row.Name, title: title, name: row.Name, sample: sample}
	}
}

// openSlicerFile opens the slicer on an audio file.
func (m Model) openSlicerFile(path string) (tea.Model, tea.Cmd) {
	m.stopListening()
	m.message = "decoding " + app.Base(path) + "…"
	unit, ops, log := m.unit, m.ops, m.log
	return m, func() tea.Msg {
		ops.Lock()
		imported, err := app.LoadSample(unit, path)
		ops.Unlock()
		if err != nil {
			log.Warn("could not decode the file to slice it", "path", path, "err", err)
			return slicerLoadedMsg{err: err}
		}
		return slicerLoadedMsg{key: "file|" + path, title: app.Base(path), name: app.PadName(path), sample: imported.Sample}
	}
}

func (m Model) slicerLoaded(msg slicerLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.message = "could not read it to slice: " + msg.err.Error()
		return m, nil
	}
	return m.openSlicer(msg.key, msg.title, msg.name, msg.sample)
}

// openSlicer opens the dialog, with the slices that were kept for this sample when there are
// some, else the sample cut in eight.
func (m Model) openSlicer(key, title, name string, sample formats.Sample) (tea.Model, tea.Cmd) {
	if sample.Frames() < formats.MinSliceFrames*2 {
		m.message = "that sample is too short to slice"
		return m, nil
	}
	key = fmt.Sprintf("%s|%d", key, sample.Frames())
	s := &slicer{title: title, name: name, key: key, src: sample, snap: true, rounds: 1}
	if m.slices != nil {
		if saved, ok := m.slices.load(key); ok && saved.Frames == sample.Frames() && saved.Validate() == nil {
			s.sl = saved
		}
	}
	if s.sl == nil {
		s.sl = formats.NewSlicing(sample.Frames(), 8)
		s.equal = 2 // the next e is 16 slices
	}
	for _, v := range sample.Samples {
		s.peak = max(s.peak, int(math.Abs(float64(v))))
	}
	s.peak = max(s.peak, 1)
	m.slicer = s
	m.message = ""
	return m, nil
}

// ---- editing ----

// change applies an edit to the slicing, keeping what it was to undo, and saves it. A round
// that is queued is made again, so the edit is heard from the next round on.
func (m Model) slicerChange(edit func(*formats.Slicing)) {
	s := m.slicer
	s.undo = append(s.undo, s.sl.Clone())
	if len(s.undo) > slicerUndoDepth {
		s.undo = s.undo[1:]
	}
	edit(s.sl)
	s.settle(m)
}

// settle puts the cursors back in range after the slicing changed, saves it, and marks the
// playing round as out of date.
func (s *slicer) settle(m Model) {
	s.at = clamp(s.at, 0, len(s.sl.Slices)-1)
	s.lane = clamp(s.lane, 0, len(s.sl.Order)-1)
	s.dirty = true
	if m.slices != nil {
		m.slices.save(s.key, s.sl)
	}
}

func (s *slicer) cur1() formats.Slice { return s.sl.Slices[s.at] }

// view is the frames on screen.
func (s *slicer) view() (from, to int) {
	if !s.zoomed {
		return 0, s.sl.Frames
	}
	start, end := s.sl.Slices[s.at].Start, s.sl.End(s.at)
	margin := max((end-start)/4, 512)
	return max(start-margin, 0), min(end+margin, s.sl.Frames)
}

// slicerCols is how many dots wide the waveform is.
func (m Model) slicerCells() int { return max(min(m.width-8, 240), 30) }

// slicerNudge moves the start of the slice in front, or its end, by dots on the screen (a dot
// is half a cell; eight when coarse is on).
func (m Model) slicerNudge(end bool, dots int) {
	s := m.slicer
	from, to := s.view()
	per := max((to-from)/(m.slicerCells()*2), 1)
	if s.coarse {
		dots *= 8
	}
	delta := dots * per
	edge := s.cur1().Start
	if end {
		edge = s.cur1().End
	}
	target := edge + delta
	if s.snap {
		if zc := s.src.ZeroCrossing(target, max(per/2, 1)); (zc-edge)*delta > 0 {
			target = zc
		}
	}
	m.slicerChange(func(sl *formats.Slicing) {
		if end {
			sl.SetEnd(s.at, target)
		} else {
			sl.SetStart(s.at, target)
		}
	})
}

func (m Model) slicerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.slicer
	name := key.String()
	if m.busy != "" {
		return m, nil
	}
	switch name {
	case "esc", "q":
		s.playing = false
		m.stopListening()
		m.slicer = nil
		return m, nil
	case " ":
		return m.slicerTogglePlay()
	case "enter":
		return m.slicerAudition()
	case "tab", "shift+tab":
		s.focusLane = !s.focusLane
		if s.focusLane {
			s.lane = clamp(s.lane, 0, len(s.sl.Order)-1)
			s.at = s.sl.Order[s.lane]
		}
		return m, nil
	case "u":
		if n := len(s.undo); n > 0 {
			s.sl, s.undo = s.undo[n-1], s.undo[:n-1]
			s.settle(m)
			m.message = "undone"
		}
		return m, nil
	case "M":
		m.slicerChange(func(sl *formats.Slicing) { sl.Mode = sl.Mode.Next() })
		m.message = "mode: " + s.sl.Mode.String()
		return m, nil
	case "S":
		seed := time.Now().UnixNano() & 0xffff
		m.slicerChange(func(sl *formats.Slicing) { sl.Seed = seed })
		m.message = fmt.Sprintf("new seed %d: the random picks and chances roll again", seed)
		return m, nil
	case "T":
		next := formats.Divisions[(indexOf(formats.Divisions, s.sl.Division)+1)%len(formats.Divisions)]
		m.slicerChange(func(sl *formats.Slicing) {
			sl.Division = next
			if sl.BPM <= 0 {
				sl.BPM = m.slicerBankTempo()
			}
		})
		if next == 0 {
			m.message = "grid off: slices play at their own length"
		} else {
			m.message = fmt.Sprintf("grid: every step is a 1/%d at %g BPM (. and , change the tempo)", next, s.sl.BPM)
		}
		return m, nil
	case ",", ".":
		if s.sl.Division == 0 {
			m.message = "no grid: T puts the steps on one, then , and . change the tempo"
			return m, nil
		}
		d := 1.0
		if name == "," {
			d = -1
		}
		m.slicerChange(func(sl *formats.Slicing) { sl.BPM = min(max(sl.BPM+d, 40), 300) })
		m.message = fmt.Sprintf("%g BPM", s.sl.BPM)
		return m, nil
	case "n":
		s.rounds = slicerRounds[(indexOf(slicerRounds, s.rounds)+1)%len(slicerRounds)]
		m.message = fmt.Sprintf("a bounce holds %s", plural(s.rounds, "round"))
		return m, nil
	case "b":
		return m.slicerBounce()
	case "o":
		m.slicerChange(func(sl *formats.Slicing) { sl.ResetOrder() })
		m.message = "order: the slices as they lie in the sample"
		return m, nil
	case "R":
		m.slicerChange(func(sl *formats.Slicing) { shuffleOrder(sl) })
		m.message = "order shuffled"
		return m, nil
	case "0", "I":
		s.zoomed = false
		return m, nil
	case "i":
		s.zoomed = true
		return m, nil
	case "z":
		s.snap = !s.snap
		m.message = "snap to zero crossings " + onOff(s.snap)
		return m, nil
	}
	if s.focusLane && m.slicerLaneKey(name) {
		return m, nil
	}
	n := len(s.sl.Slices)
	switch name {
	case "h", "left":
		s.at = (s.at + n - 1) % n
	case "l", "right":
		s.at = (s.at + 1) % n
	case "g":
		s.at = 0
	case "G":
		s.at = n - 1
	case "[", "H":
		m.slicerNudge(false, -1)
	case "]", "L":
		m.slicerNudge(false, 1)
	case "{":
		m.slicerNudge(true, -1)
	case "}":
		m.slicerNudge(true, 1)
	case "f":
		s.coarse = !s.coarse
		if s.coarse {
			m.message = "coarse: a nudge moves eight dots"
		} else {
			m.message = "fine: a nudge moves one dot"
		}
	case "c":
		mid := s.cur1().Start + s.sl.Length(s.at)/2
		if s.snap {
			mid = s.src.ZeroCrossing(mid, 256)
		}
		cut := -1
		m.slicerChange(func(sl *formats.Slicing) { cut = sl.Split(s.at, mid) })
		if cut < 0 {
			m.slicerUndoLast()
			m.message = "that slice is too short to cut"
		} else {
			s.at = cut
			m.message = fmt.Sprintf("cut: slice %d is new; [ ] move its start, { } its end", cut+1)
		}
	case "d":
		removed := false
		m.slicerChange(func(sl *formats.Slicing) { removed = sl.Remove(s.at) })
		if !removed {
			m.slicerUndoLast()
			m.message = "the last slice cannot be removed"
		}
	case "e":
		count := slicerEqualCounts[s.equal%len(slicerEqualCounts)]
		s.equal++
		m.slicerChange(func(sl *formats.Slicing) { sl.Divide(count) })
		s.at = 0
		m.message = fmt.Sprintf("cut in %d equal slices (u undoes)", len(s.sl.Slices))
	case "t":
		sens := slicerSensitivities[s.sens%len(slicerSensitivities)]
		s.sens++
		starts := s.src.Transients(32, sens)
		if len(starts) == 0 {
			m.message = fmt.Sprintf("no onsets found at sensitivity %.1f; t tries another", sens)
			break
		}
		m.slicerChange(func(sl *formats.Slicing) { sl.SetStarts(starts) })
		s.at = 0
		m.message = fmt.Sprintf("%d slices from the onsets, sensitivity %.1f (t again changes it, u undoes)", len(s.sl.Slices), sens)
	case "r":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Reverse = !sl.Slices[s.at].Reverse })
	case "m":
		m.slicerChange(func(sl *formats.Slicing) {
			if sl.Slices[s.at].Chance == 0 {
				sl.Slices[s.at].Chance = 100
			} else {
				sl.Slices[s.at].Chance = 0
			}
		})
	case "(":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Chance = max(sl.Slices[s.at].Chance-10, 0) })
	case ")":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Chance = min(sl.Slices[s.at].Chance+10, 100) })
	case "<":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Pitch = max(sl.Slices[s.at].Pitch-1, -24) })
	case ">":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Pitch = min(sl.Slices[s.at].Pitch+1, 24) })
	case "-":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Gain = max(sl.Slices[s.at].Gain-10, 0) })
	case "+", "=":
		m.slicerChange(func(sl *formats.Slicing) { sl.Slices[s.at].Gain = min(sl.Slices[s.at].Gain+10, 400) })
	case "X":
		m.slicerChange(func(sl *formats.Slicing) {
			sl.Slices[s.at] = formats.Slice{Start: sl.Slices[s.at].Start, Chance: 100, Gain: 100}
		})
		m.message = fmt.Sprintf("slice %d back to its plain settings", s.at+1)
	}
	return m, nil
}

// slicerBankTempo is the tempo of the bank in front, for a grid to start from.
func (m Model) slicerBankTempo() float64 {
	if m.snap == nil {
		return 120
	}
	bpm := m.snap.Banks[m.bank].Tempo
	if m.snap.UsesProjectTempo {
		bpm = m.snap.ProjectTempo
	}
	if bpm < 40 {
		return 120
	}
	return bpm
}

// slicerUndoLast drops the undo step of an edit that turned out to change nothing.
func (m Model) slicerUndoLast() {
	s := m.slicer
	if n := len(s.undo); n > 0 {
		s.sl, s.undo = s.undo[n-1], s.undo[:n-1]
	}
}

// slicerLaneKey handles the keys of the order lane; false for keys that are not its own.
func (m Model) slicerLaneKey(name string) bool {
	s := m.slicer
	order := s.sl.Order
	pick := func() { s.at = s.sl.Order[s.lane] }
	switch name {
	case "h", "left":
		s.lane = (s.lane + len(order) - 1) % len(order)
		pick()
	case "l", "right":
		s.lane = (s.lane + 1) % len(order)
		pick()
	case "H", "L":
		to := s.lane + 1
		if name == "H" {
			to = s.lane - 1
		}
		if to < 0 || to >= len(order) {
			return true
		}
		m.slicerChange(func(sl *formats.Slicing) { sl.Order[s.lane], sl.Order[to] = sl.Order[to], sl.Order[s.lane] })
		s.lane = to
	case "j", "down", "k", "up":
		d := 1
		if name == "k" || name == "up" {
			d = -1
		}
		n := len(s.sl.Slices)
		m.slicerChange(func(sl *formats.Slicing) { sl.Order[s.lane] = (sl.Order[s.lane] + d + n) % n })
		pick()
	case "a":
		m.slicerChange(func(sl *formats.Slicing) {
			sl.Order = append(sl.Order[:s.lane+1], append([]int{sl.Order[s.lane]}, sl.Order[s.lane+1:]...)...)
		})
		s.lane++
	case "d":
		if len(order) < 2 {
			m.message = "the order needs at least one step"
			return true
		}
		m.slicerChange(func(sl *formats.Slicing) { sl.Order = append(sl.Order[:s.lane], sl.Order[s.lane+1:]...) })
		pick()
	default:
		return false
	}
	return true
}

// shuffleOrder puts the steps of the order in a random order.
func shuffleOrder(sl *formats.Slicing) {
	q := formats.NewSequencer(&formats.Slicing{Frames: sl.Frames, Slices: sl.Slices, Order: sl.Order, Mode: formats.ModeShuffle, Seed: time.Now().UnixNano()})
	n := len(sl.Order)
	// One shuffled pass of every slice, then the repeats of the old order kept in length.
	out := make([]int, 0, n)
	for len(out) < n {
		out = append(out, q.Next().Slice)
	}
	sl.Order = out
}

func indexOf(list []int, v int) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}

// ---- playing ----

// roundSteps is how many steps make one pass of the sequence.
func (s *slicer) roundSteps() int {
	switch s.sl.Mode {
	case formats.ModeRandom, formats.ModeNoRepeat:
		return len(s.sl.Slices)
	case formats.ModeShuffle:
		n := 0
		for _, sl := range s.sl.Slices {
			if sl.Chance > 0 {
				n++
			}
		}
		return max(n, 1)
	}
	return len(s.sl.Order)
}

// render makes round i of the sequence as the slicing is now. Each round has its own seed, so
// the random modes do not play the same stretch twice.
func (s *slicer) render(i, passes int) *slicerRound {
	sl := s.sl.Clone()
	sl.Seed += int64(i)
	audio, placed := formats.Render(s.src, sl, formats.NewSequencer(sl), s.roundSteps()*passes)
	return &slicerRound{sample: audio, placed: placed, dur: framesDuration(audio.Frames())}
}

// renderRound is a round long enough to be queued ahead: the pass is played again until it
// is.
func (s *slicer) renderRound(i int) *slicerRound {
	passes := 1
	for {
		r := s.render(i, passes)
		if r.dur >= slicerMinRound || passes >= 32 || r.sample.Frames() == 0 {
			return r
		}
		passes *= 2
	}
}

func framesDuration(frames int) time.Duration {
	return time.Duration(frames) * time.Second / formats.SampleRate
}

func (m Model) slicerTogglePlay() (tea.Model, tea.Cmd) {
	s := m.slicer
	if s.playing {
		s.playing = false
		m.stopListening()
		return m, nil
	}
	if m.player == nil {
		m.message = "no audio output to play on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	m.stopListening()
	s.playing, s.round, s.dirty, s.nxt = true, 0, false, nil
	s.gen++
	s.cur = s.renderRound(0)
	s.cur.start = time.Now()
	if looper, ok := m.player.(app.Looper); ok {
		_ = looper.Loop(s.cur.sample)
	} else {
		_ = m.player.Play(s.cur.sample)
	}
	m.listening = "slicer"
	return m, m.slicerTick()
}

func (m Model) slicerTick() tea.Cmd {
	gen := m.slicer.gen
	return tea.Tick(slicerTickEvery, func(time.Time) tea.Msg { return slicerTickMsg{gen: gen} })
}

// slicerTicked moves the playhead on, and keeps the next round queued: it is rendered once the
// round sounding is under way, so it takes over without a gap.
func (m Model) slicerTicked(msg slicerTickMsg) (tea.Model, tea.Cmd) {
	s := m.slicer
	if s == nil || !s.playing || msg.gen != s.gen {
		return m, nil
	}
	if !m.player.Playing() {
		// Stopped from outside, or a player that plays only once and has finished.
		s.playing = false
		return m, nil
	}
	now := time.Now()
	for now.Sub(s.cur.start) >= s.cur.dur && s.cur.dur > 0 {
		end := s.cur.start.Add(s.cur.dur)
		if s.nxt == nil {
			// Nothing queued in time: the loop went round again.
			s.cur.start = end
			continue
		}
		s.cur, s.nxt = s.nxt, nil
		s.cur.start = end
		s.round++
		if _, ok := m.player.(app.LoopSwapper); !ok {
			if looper, ok := m.player.(app.Looper); ok {
				_ = looper.Loop(s.cur.sample)
			}
		}
	}
	if (s.nxt == nil && now.Sub(s.cur.start) >= s.cur.dur/4) || (s.dirty && s.nxt != nil) {
		s.nxt = s.renderRound(s.round + 1)
		s.dirty = false
		if swapper, ok := m.player.(app.LoopSwapper); ok {
			_ = swapper.NextLoop(s.nxt.sample)
		}
	}
	return m, m.slicerTick()
}

// slicerAudition plays the slice in front once, as it sounds.
func (m Model) slicerAudition() (tea.Model, tea.Cmd) {
	s := m.slicer
	if m.player == nil {
		m.message = "no audio output to play on"
		return m, nil
	}
	s.playing = false
	m.stopListening()
	_ = m.player.Play(formats.SliceAudio(s.src, s.sl, s.at))
	return m, nil
}

// playhead is where the sound is now, as a frame of the source, and the slice it is in; -1
// when nothing plays.
func (s *slicer) playhead() (frame, slice int) {
	if !s.playing || s.cur == nil {
		return -1, -1
	}
	elapsed := int(time.Since(s.cur.start) * formats.SampleRate / time.Second)
	for _, p := range s.cur.placed {
		if elapsed < p.At || elapsed >= p.At+max(p.Frames, 1) {
			continue
		}
		start, length := s.sl.Slices[p.Slice].Start, s.sl.Length(p.Slice)
		// On a grid the slice may sound for less than the step: past that, it is over.
		if elapsed-p.At >= max(p.Audio, 1) {
			return -1, p.Slice
		}
		pos := (elapsed - p.At) * length / max(p.Audio, 1)
		if s.sl.Slices[p.Slice].Reverse {
			pos = length - 1 - pos
		}
		return start + pos, p.Slice
	}
	return -1, -1
}

// ---- bounce ----

// bounceSample renders what the slicer plays, for as many rounds as are set.
func (s *slicer) bounceSample() formats.Sample {
	out := formats.Sample{Channels: s.src.Channels}
	for i := 0; i < s.rounds; i++ {
		part := s.render(i, 1)
		out.Samples = append(out.Samples, part.sample.Samples...)
	}
	return out
}

// slicerBounce asks which pad the rendered sequence goes onto.
func (m Model) slicerBounce() (tea.Model, tea.Cmd) {
	s := m.slicer
	if m.unit == nil || m.snap == nil {
		m.message = "no unit to put it on"
		return m, nil
	}
	sample := s.bounceSample()
	if sample.Frames() == 0 {
		m.message = "nothing to bounce: every slice is silent"
		return m, nil
	}
	// The pad's name has room for 16 characters: the sample's, cut short, and a mark.
	name := []rune(s.name)
	name = name[:min(len(name), app.MaxPadName-4)]
	padName := strings.TrimSpace(string(name)) + " slc"
	bank := m.bank
	m.picker = &picker{bank: bank, pad: m.nextFree(bank), bounce: &sample, bounceName: padName}
	return m, nil
}

// bounceEnter puts the rendered sequence on the pad the picker chose.
func (m Model) bounceEnter() (tea.Model, tea.Cmd) {
	p := m.picker
	target, _ := proto.PadFromBankPad(uint16(p.bank), uint16(p.pad))
	if m.snap.Pads[target.Index()].HasSample && !p.confirm {
		p.confirm = true
		return m, nil
	}
	sample, name := *p.bounce, p.bounceName
	m.picker = nil
	unit := m.unit
	m.busy = "bouncing to " + target.String()
	return m, m.edit(func() tea.Msg {
		if _, err := app.PutSample(unit, target, sample, name); err != nil {
			return doneMsg{message: err.Error()}
		}
		return doneMsg{message: fmt.Sprintf("bounced %.1f s of slices to %s as %q", float64(sample.Frames())/formats.SampleRate, target, name),
			reload: true, focus: &target}
	})
}

// ---- drawing ----

// sliceFG is the colour a slice is drawn in: two alternate, so neighbours can be told apart.
func (st styles) sliceFG(i int, muted bool) lipgloss.Color {
	switch {
	case muted:
		return st.th.faint
	case i%2 == 0:
		return st.th.accent
	}
	return st.th.bus1
}

type cellKind int

const (
	cellSlice cellKind = iota
	cellEdge
	cellHead
)

// cellStyle is how a run of cells is drawn.
type cellStyle struct {
	kind cellKind
	// slice is the slice the cell belongs to, -1 outside all of them.
	slice int
	// selected and playing are the states of that slice; end marks the end edge of a slice.
	selected, playing, end bool
}

// edgeInfo is a slice's edge at a dot of the waveform: slice is its number, 0 for none. Where
// two meet, the edge of the slice in front shows, then a start before an end.
type edgeInfo struct {
	slice int
	end   bool
}

func (e *edgeInfo) put(n edgeInfo, front int) {
	switch {
	case e.slice == 0, n.slice == front+1 && e.slice != front+1, e.slice != front+1 && e.end && !n.end:
		*e = n
	}
}

func (m Model) slicerStyle(c cellStyle) lipgloss.Style {
	st, s := m.st, m.slicer
	switch c.kind {
	case cellHead:
		return lipgloss.NewStyle().Foreground(st.th.text).Bold(true)
	case cellEdge:
		if c.selected && c.end {
			return lipgloss.NewStyle().Foreground(st.th.warn).Bold(true)
		}
		if c.selected {
			return lipgloss.NewStyle().Foreground(st.th.hot).Bold(true)
		}
		return lipgloss.NewStyle().Foreground(st.th.dim)
	}
	if c.slice < 0 {
		return lipgloss.NewStyle().Foreground(st.th.faint)
	}
	style := lipgloss.NewStyle().Foreground(st.sliceFG(c.slice, s.sl.Slices[c.slice].Chance == 0))
	switch {
	case c.playing:
		style = style.Foreground(st.th.hot).Background(st.th.tile)
	case c.selected:
		style = style.Background(st.th.tile)
	}
	return style
}

// runs renders cells in the style of each, joining the neighbours that share one.
func (m Model) slicerRuns(text []rune, styles []cellStyle) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		j := i + 1
		for j < len(text) && styles[j] == styles[i] {
			j++
		}
		b.WriteString(m.slicerStyle(styles[i]).Render(string(text[i:j])))
		i = j
	}
	return b.String()
}

func seconds(frames int) string { return fmt.Sprintf("%.3f s", float64(frames)/formats.SampleRate) }

func (m Model) slicerBox() string {
	st, s := m.st, m.slicer
	sl := s.sl
	cells := m.slicerCells()
	cols := cells * 2
	from, to := s.view()
	span := max(to-from, 1)
	head, headSlice := s.playhead()
	help := m.slicerHelpLines(cells)
	// What is not the waveform takes 15 lines with the border; the waveform gets the rest.
	rows := clamp(m.geom().bodyH-15-len(help), 3, 10)

	// Slices can overlap: the one in front, or the one sounding, shows where they do.
	styleOf := func(frame int) cellStyle {
		i := -1
		for k, x := range sl.Slices {
			if frame >= x.Start && frame < x.End && (i < 0 || k == s.at || (k == headSlice && i != s.at)) {
				i = k
			}
		}
		return cellStyle{kind: cellSlice, slice: i, selected: i == s.at, playing: headSlice == i}
	}
	colOf := func(frame int) int { return int(int64(frame-from) * int64(cols) / int64(span)) }

	// Where the slices start and where the playhead is, in dots.
	edge := make([]edgeInfo, cols+1)
	for i, x := range sl.Slices {
		if x.Start >= from && x.Start < to {
			edge[min(colOf(x.Start), cols-1)].put(edgeInfo{slice: i + 1}, s.at)
		}
		if x.End > from && x.End <= to {
			edge[min(colOf(x.End), cols-1)].put(edgeInfo{slice: i + 1, end: true}, s.at)
		}
	}
	headCol := -1
	if head >= from && head < to {
		headCol = min(colOf(head), cols-1)
	}

	// The waveform: peaks as dots, mirrored about the middle, with the edges as full lines.
	amps := s.src.Peaks(from, to, cols)
	half := rows * 4 / 2
	heights := make([]int, cols)
	for c, a := range amps {
		heights[c] = min(max(1, int(math.Sqrt(float64(a)/float64(s.peak))*float64(half)+0.5)), half)
		if edge[c].slice > 0 || c == headCol {
			heights[c] = half
		}
	}
	wave := make([]string, rows)
	for r := 0; r < rows; r++ {
		text := make([]rune, cells)
		styles := make([]cellStyle, cells)
		for c := 0; c < cells; c++ {
			cell := rune(0x2800)
			for sub := 0; sub < 2; sub++ {
				h := heights[c*2+sub]
				for d := 0; d < 4; d++ {
					if dot := r*4 + d; dot >= half-h && dot < half+h {
						cell |= brailleDots[sub][d]
					}
				}
			}
			text[c] = cell
			frame := from + (c*2+1)*span/cols
			styles[c] = styleOf(frame)
			switch {
			case headCol == c*2 || headCol == c*2+1:
				styles[c] = cellStyle{kind: cellHead}
			case edge[c*2].slice > 0 || edge[c*2+1].slice > 0:
				e := edge[c*2]
				e.put(edge[c*2+1], s.at)
				styles[c] = cellStyle{kind: cellEdge, slice: e.slice - 1, selected: e.slice-1 == s.at, end: e.end}
			}
		}
		wave[r] = m.slicerRuns(text, styles)
	}

	// Above it, the number of each slice at its start.
	numText := []rune(strings.Repeat(" ", cells))
	numStyle := make([]cellStyle, cells)
	for c := range numStyle {
		numStyle[c] = cellStyle{kind: cellSlice, slice: -1}
	}
	for i, x := range sl.Slices {
		if x.Start < from || x.Start >= to {
			continue
		}
		c := min(colOf(x.Start)/2, cells-1)
		label := []rune(fmt.Sprint(i + 1))
		for k, r := range label {
			if c+k < cells {
				numText[c+k] = r
				numStyle[c+k] = cellStyle{kind: cellEdge, slice: i, selected: i == s.at}
			}
		}
	}

	if x := sl.Slices[s.at]; x.End > from && x.End <= to {
		c := min(colOf(x.End)/2, cells-1)
		if numText[c] == ' ' {
			numText[c] = '◂'
			numStyle[c] = cellStyle{kind: cellEdge, slice: s.at, selected: true, end: true}
		}
	}

	// Under it, a bar with a block per slice, with what is set on it.
	barText := []rune(strings.Repeat(" ", cells))
	barKind := make([]int, cells) // slice index + 1; 0 outside
	barOrder := make([]int, 0, len(sl.Slices))
	for i := range sl.Slices {
		if i != s.at {
			barOrder = append(barOrder, i)
		}
	}
	barOrder = append(barOrder, s.at) // the one in front is drawn last, over the others
	for _, i := range barOrder {
		a, b := max(sl.Slices[i].Start, from), min(sl.End(i), to)
		if b <= a {
			continue
		}
		c0, c1 := min(colOf(a)/2, cells-1), min(max(colOf(b)/2, colOf(a)/2+1), cells)
		for c := c0; c < c1; c++ {
			barKind[c] = i + 1
		}
		if c1-c0 > 0 {
			barText[c0] = '▏'
		}
		label := []rune(m.sliceLabel(i, c1-c0-1))
		start := c0 + 1 + max(c1-c0-1-len(label), 0)/2
		for k, r := range label {
			if start+k < c1 {
				barText[start+k] = r
			}
		}
	}
	bar := m.slicerBar(barText, barKind, headSlice)

	// The time axis.
	axis := seconds(from)
	end := seconds(to)
	zoom := ""
	if s.zoomed {
		zoom = fmt.Sprintf("zoom ×%.1f", float64(sl.Frames)/float64(span))
	}
	gap := max(cells-len(axis)-len(end)-len(zoom), 2)
	axisLine := st.faint.Render(axis + strings.Repeat(" ", gap/2) + zoom + strings.Repeat(" ", gap-gap/2) + end)

	// Header.
	playing := st.faint.Render("■ stopped")
	if s.playing {
		playing = st.good.Render("▶ playing")
	}
	title := st.bold.Render("Slicer") + st.dim.Render("  ·  ") + st.text.Render(s.title) + st.dim.Render(fmt.Sprintf("  ·  %s  ·  %s", seconds(sl.Frames), plural(len(sl.Slices), "slice")))
	right := st.dim.Render("mode ") + st.accent.Render(sl.Mode.String()) + "  " + playing
	titleLine := title + strings.Repeat(" ", max(cells-ansi.StringWidth(title)-ansi.StringWidth(right), 2)) + right

	lines := []string{titleLine, "", m.slicerRuns(numText, numStyle)}
	lines = append(lines, wave...)
	lines = append(lines, bar, axisLine, "")
	lines = append(lines, m.sliceInfo(from, to, cols)...)
	lines = append(lines, "", m.orderLane(cells, head >= 0))
	lines = append(lines, m.roundLine(cells))
	lines = append(lines, st.faint.Render(strings.Repeat("─", cells)))
	lines = append(lines, help...)
	// Whatever is wider than the waveform is cut, so the box never outgrows the screen.
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, cells, "…")
	}
	return st.modal.Render(strings.Join(lines, "\n"))
}

// helpKey is a key and what it does, as the keys of the other panels are listed.
type helpKey struct{ key, what string }

// helpGroup is a row of keys under a heading.
type helpGroup struct {
	name string
	keys []helpKey
}

// slicerHelp are the keys the dialog lists under itself, in groups; the order lane's keys take
// the place of the first two while the cursor is in the lane.
func (s *slicer) slicerHelp() []helpGroup {
	groups := []helpGroup{
		{"slices", []helpKey{{"h l", "select"}, {"c", "cut"}, {"d", "delete"}, {"e", "equal slices"}, {"t", "onsets"}, {"u", "undo"}}},
		{"edges", []helpKey{{"[ ]", "start"}, {"{ }", "end"}, {"f", "coarse"}, {"z", "snap"}, {"i I", "zoom"}}},
	}
	if s.focusLane {
		groups = []helpGroup{
			{"order", []helpKey{{"h l", "move"}, {"H L", "carry step"}, {"j k", "other slice"}, {"a", "repeat"}, {"d", "drop"}, {"R", "shuffle"}, {"o", "reset"}}},
		}
	}
	return append(groups,
		helpGroup{"sound", []helpKey{{"r", "reverse"}, {"m", "mute"}, {"( )", "chance"}, {"< >", "pitch"}, {"- +", "gain"}, {"X", "plain"}}},
		helpGroup{"play", []helpKey{{"Space", "play"}, {"Enter", "hear slice"}, {"M", "mode"}, {"T", "grid"}, {", .", "tempo"}, {"S", "seed"}, {"Tab", "order lane"}}},
		helpGroup{"bounce", []helpKey{{"b", "to a pad"}, {"n", "rounds"}, {"Esc", "close"}}},
	)
}

// helpLines draws the groups: a heading, then the keys as the status bar shows them, an accent
// key and a dim word, wrapped under the heading when a row is too long.
func (m Model) slicerHelpLines(width int) []string {
	st := m.st
	const head = 8
	var lines []string
	for _, g := range m.slicer.slicerHelp() {
		line := st.faint.Render(fmt.Sprintf("%-*s", head, g.name))
		used := head
		for i, k := range g.keys {
			chip := st.accent.Render(k.key) + st.dim.Render(" "+k.what)
			w := ansi.StringWidth(k.key) + 1 + ansi.StringWidth(k.what)
			if i > 0 && used+2+w > width {
				lines = append(lines, line)
				line, used = strings.Repeat(" ", head), head
			}
			if used > head {
				line += "  "
				used += 2
			}
			line += chip
			used += w
		}
		lines = append(lines, line)
	}
	return lines
}

// sliceLabel is what the bar shows on slice i: its number and as much of its settings as fit.
func (m Model) sliceLabel(i, width int) string {
	x := m.slicer.sl.Slices[i]
	num := fmt.Sprint(i + 1)
	var flags []string
	switch {
	case x.Chance == 0:
		flags = append(flags, "mute")
	case x.Chance < 100:
		flags = append(flags, fmt.Sprintf("%d%%", x.Chance))
	}
	if x.Reverse {
		flags = append(flags, "rev")
	}
	if x.Pitch != 0 {
		flags = append(flags, fmt.Sprintf("%+d", x.Pitch))
	}
	if x.Gain != 100 {
		flags = append(flags, fmt.Sprintf("g%d", x.Gain))
	}
	label := num
	for _, f := range flags {
		if len(label)+1+len(f) > width {
			break
		}
		label += " " + f
	}
	if len(label) > width {
		return ""
	}
	return label
}

// slicerBar draws the bar of slice blocks: the one in front lit, the one sounding in the
// colour of a sounding pad, muted ones greyed.
func (m Model) slicerBar(text []rune, kind []int, playing int) string {
	st, s := m.st, m.slicer
	var b strings.Builder
	for i := 0; i < len(text); {
		j := i + 1
		for j < len(text) && kind[j] == kind[i] {
			j++
		}
		piece := string(text[i:j])
		switch idx := kind[i] - 1; {
		case idx < 0:
			b.WriteString(piece)
		case idx == playing:
			b.WriteString(lipgloss.NewStyle().Background(st.th.hot).Foreground(st.th.cursorFg).Bold(true).Render(piece))
		case idx == s.at:
			b.WriteString(lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(piece))
		case s.sl.Slices[idx].Chance == 0:
			b.WriteString(lipgloss.NewStyle().Background(st.th.tileEmpty).Foreground(st.th.faint).Render(piece))
		default:
			b.WriteString(lipgloss.NewStyle().Background(st.th.tile).Foreground(st.sliceFG(idx, false)).Render(piece))
		}
		i = j
	}
	return b.String()
}

// meter is a value 0..100 as ten blocks.
func meter(v int) string {
	n := (v + 5) / 10
	return strings.Repeat("▰", min(n, 10)) + strings.Repeat("▱", 10-min(n, 10))
}

// sliceInfo describes the slice in front, and the settings of the whole.
func (m Model) sliceInfo(from, to, cols int) []string {
	st, s := m.st, m.slicer
	x := s.cur1()
	dir := "forward"
	if x.Reverse {
		dir = "reverse"
	}
	chance := fmt.Sprintf("%s %3d%%", meter(x.Chance), x.Chance)
	if s.sl.Mode == formats.ModeOrder {
		chance += st.faint.Render(" plays")
	} else {
		chance += st.faint.Render(" weight")
	}
	first := st.bold.Render(fmt.Sprintf("Slice %d/%d", s.at+1, len(s.sl.Slices))) + "   " +
		st.dim.Render("start ") + st.text.Render(seconds(x.Start)) + st.faint.Render(fmt.Sprintf(" (%d)", x.Start)) + "   " +
		st.dim.Render("end ") + st.text.Render(seconds(x.End)) + st.faint.Render(fmt.Sprintf(" (%d)", x.End)) + "   " +
		st.dim.Render("length ") + st.text.Render(seconds(s.sl.Length(s.at)))
	second := st.dim.Render("chance ") + st.accent.Render(chance) + "   " +
		st.dim.Render("gain ") + st.accent.Render(fmt.Sprintf("%d%%", x.Gain)) + "   " +
		st.dim.Render("pitch ") + st.accent.Render(fmt.Sprintf("%+d", x.Pitch)) + "   " +
		st.dim.Render("play ") + st.accent.Render(dir)
	dot := float64(max((to-from)/cols, 1)) / formats.SampleRate * 1000
	grid := "grid off"
	if s.sl.Division > 0 {
		grid = fmt.Sprintf("grid 1/%d at %g BPM", s.sl.Division, s.sl.BPM)
	}
	step := "one dot"
	if s.coarse {
		step = "eight dots"
		dot *= 8
	}
	third := st.faint.Render(fmt.Sprintf("a nudge moves "+step+", %.2f ms  ·  snap %s  ·  %s  ·  seed %d  ·  a bounce holds %s",
		dot, onOff(s.snap), grid, s.sl.Seed, plural(s.rounds, "round")))
	return []string{first, second, third}
}

// orderLane draws the order the slices play in, as numbered steps with the cursor on one.
func (m Model) orderLane(width int, playing bool) string {
	st, s := m.st, m.slicer
	order := s.sl.Order
	unused := s.sl.Mode != formats.ModeOrder
	label := "Order  "
	if unused {
		label = "Order  " // kept, but the mode picks by itself
	}
	room := max((width-len(label)-6)/4, 1)
	from := clamp(s.lane-room/2, 0, max(len(order)-room, 0))
	to := min(from+room, len(order))
	_, headSlice := s.playhead()
	var b strings.Builder
	if from > 0 {
		b.WriteString(st.faint.Render("‹ "))
	}
	for i := from; i < to; i++ {
		chip := fmt.Sprintf(" %-2d ", order[i]+1)
		style := lipgloss.NewStyle().Foreground(st.sliceFG(order[i], s.sl.Slices[order[i]].Chance == 0))
		switch {
		case s.focusLane && i == s.lane:
			style = lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true)
		case !unused && playing && order[i] == headSlice:
			style = style.Background(st.th.tile).Bold(true)
		case unused:
			style = lipgloss.NewStyle().Foreground(st.th.faint)
		}
		b.WriteString(style.Render(chip))
	}
	if to < len(order) {
		b.WriteString(st.faint.Render(" ›"))
	}
	hint := ""
	switch {
	case unused:
		hint = st.faint.Render("   not used in " + s.sl.Mode.String() + " mode")
	}
	return st.dim.Render(label) + b.String() + hint
}

// roundLine shows what the sequence is playing in the random modes: the steps of the round
// that sounds, so the picks can be followed.
func (m Model) roundLine(width int) string {
	st, s := m.st, m.slicer
	if s.sl.Mode == formats.ModeOrder || s.cur == nil || !s.playing {
		return ""
	}
	var parts []string
	for _, p := range s.cur.placed {
		if len(strings.Join(parts, " "))+3 > width-12 {
			parts = append(parts, "…")
			break
		}
		n := fmt.Sprint(p.Slice + 1)
		if p.Rest {
			n = "·"
		}
		parts = append(parts, n)
	}
	return st.dim.Render(fmt.Sprintf("Round %-3d", s.round+1)) + st.text.Render(strings.Join(parts, " "))
}
