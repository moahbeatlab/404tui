package tui

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/record"
)

// The Looper records what the unit plays (the loop of the MC-101 that goes through it) for a number of bars,
// beginning on the next beat (or bar) of the unit's clock, and puts it on the next free pad of a bank, ungated. The bank has a
// pattern in which all 16 pads start on beat 1, so the loop plays from the next round on. A pad that is no longer
// wanted is deleted: its note in the pattern is silent then, and the pad is free for the next take.

type looperPhase int

const (
	lpIdle      looperPhase = iota
	lpStarting              // the recorder is being started
	lpArmed                 // recording, waiting for the start
	lpRecording             // the loop is being played into the file
	lpSaving                // the take is being cut and put on its pad
)

// loopWarmup is how long the recorder runs before a take may start: the file has to hold the sound from before the
// start too. The start waits for the next bar if it is not ready by then.
const loopWarmup = 350 * time.Millisecond

// loopBarChoices are the lengths of a loop the b key goes through.
var loopBarChoices = []int{1, 2, 4, 8}

// loopSettle is how long the recorder runs on after the loop ended, so the end of it is certainly in the file.
var loopSettle = 600 * time.Millisecond

type looperState struct {
	phase   looperPhase
	session *record.Session
	// startTick is the tick of the clock the loop began on; t0 and t1 are when the loop began and ended.
	startTick int
	t0, t1    time.Time
	// target is the pad the take goes on.
	target proto.PadIndex
	// cursor is the pad in front (0 to 15); delAsk is the pad d asked to delete (-1 for none).
	cursor, delAsk int
	takes          int
	// cal is a take that measures the delay instead of making a loop.
	cal bool
	// ringTake is a take that is cut out of the memory the program listens into, not recorded by a program.
	ringTake bool
	// last is the take that was saved last, calMS what the last calibration found (calDone says there was one).
	last    loopLast
	calMS   int
	calDone bool
	// slBank and slPad are the pad the web page's Slicer will cut (slPad -1 for none); slSet says they were chosen.
	slBank, slPad int
	slSet         bool
	// slOut are slices that wait for a pad (slHave), from slFrom in slN slices; slDest is the pad chosen for them (-1 for none).
	slOut   formats.Sample
	slHave  bool
	slFrom  string
	slOrder string
	// jobs wait for the last beat of the pattern.
	jobs        []lpJob
	slN, slDest int
}

// loopLast is what the Looper knows of the loop it saved last.
type loopLast struct {
	has        bool
	name, pad  string
	peak, gain float64
}

type (
	// loopStartedMsg is the recorder of the Looper that came up (or did not).
	loopStartedMsg struct {
		session *record.Session
		err     error
	}
	// loopSavedMsg ends a take: it is on its pad, or it is not and err says why.
	loopSavedMsg struct {
		pad  proto.PadIndex
		name string
		err  error
		// peak is how loud the loop on the pad is, in dB below full scale; gain what was added to it.
		peak, gain float64
		// delay is what a calibration found, in ms.
		cal   bool
		delay int
	}
)

// looper is the state of the Looper, made when it is first wanted.
func (m *Model) looper() *looperState {
	if m.lp == nil {
		m.lp = &looperState{delAsk: -1}
	}
	return m.lp
}

func (m Model) loopBank() int { return clamp(m.mem.LooperBank, 0, proto.Banks-1) }

// loopBars is how long a loop is.
func (m Model) loopBars() int {
	if slices.Contains(loopBarChoices, m.mem.LooperBars) {
		return m.mem.LooperBars
	}
	return 2
}

// loopDelay is how late the sound is that the computer records, from Config's side of things: how far the
// recorder is behind the clock.
func (m Model) loopDelay() time.Duration { return time.Duration(m.mem.LooperDelay) * time.Millisecond }

// loopFreePad is the first pad of the bank that holds no sample.
// takeBars is how long the take that is on (or the next) is: a calibration is one bar.
func (m Model) takeBars() int {
	if m.lp != nil && m.lp.cal {
		return 1
	}
	return m.loopBars()
}

// takeAlign is how many clock ticks the start of a take is a multiple of: a beat, or a bar when set (and for a
// calibration, which needs the bar so that a hit on beat 1 is at the start).
func (m Model) takeAlign() int {
	if !m.mem.LooperOnBeat || (m.lp != nil && m.lp.cal) {
		return ticksPerBar
	}
	return 24
}

func (m Model) loopFreePad() (proto.PadIndex, bool) {
	for _, row := range m.snap.Bank(m.loopBank()) {
		if !row.HasSample {
			return row.Pad, true
		}
	}
	return proto.PadIndex{}, false
}

func (m Model) clockRuns() bool {
	return m.clock != nil && m.clock.running && time.Since(m.clock.at) < 2*time.Second
}

// loopDir is where the recorder of the Looper keeps its files while they are cut.
func loopDir() string { return filepath.Join(os.TempDir(), "404tui-looper") }

// looperKey is a key of the Looper view.
func (m Model) looperKey(name string) (tea.Model, tea.Cmd) {
	lp := m.looper()
	ask := lp.delAsk
	lp.delAsk = -1
	switch name {
	case "a", "enter", " ", "space":
		return m.looperArm()
	case "c":
		return m.looperArmWith(true)
	case "t":
		return m.looperAlignToggle()
	case "s":
		return m.looperSlicesNext()
	case "x":
		return m.looperRemix(lp.cursor)
	case "[":
		return m.looperBankStep(-1)
	case "]":
		return m.looperBankStep(1)
	case "b":
		return m.looperBarsNext()
	case "g":
		return m.looperGainStep(-1)
	case "G":
		return m.looperGainStep(1)
	case "n":
		return m.looperNormalizeToggle()
	case ",":
		return m.looperDelayStep(-5)
	case ".":
		return m.looperDelayStep(5)
	case "h", "left":
		lp.cursor = clamp(lp.cursor-1, 0, 15)
	case "l", "right":
		lp.cursor = clamp(lp.cursor+1, 0, 15)
	case "k", "up":
		lp.cursor = clamp(lp.cursor-4, 0, 15)
	case "j", "down":
		lp.cursor = clamp(lp.cursor+4, 0, 15)
	case "p":
		return m.looperPattern()
	case "d":
		row := m.snap.Bank(m.loopBank())[lp.cursor]
		if !row.HasSample {
			m.message = fmt.Sprintf("%s holds no sample", row.Pad)
			return m, nil
		}
		if ask != lp.cursor {
			lp.delAsk = lp.cursor
			m.message = fmt.Sprintf("d again deletes %s %s (its note in the pattern stays, silent)", row.Pad, row.Name)
			return m, nil
		}
		return m.looperDelete(lp.cursor)
	}
	return m, nil
}

// looperBankStep moves the bank the loops go to, up or down.
func (m Model) looperBankSet(bank int) (tea.Model, tea.Cmd) {
	if bank < 0 || bank >= proto.Banks {
		return m, nil
	}
	return m.looperBankStep(bank - m.loopBank())
}

func (m Model) looperBankStep(dir int) (tea.Model, tea.Cmd) {
	if m.looper().phase != lpIdle {
		m.message = "the bank cannot change while a take is on"
		return m, nil
	}
	m.mem.LooperBank = (m.loopBank() + dir + proto.Banks) % proto.Banks
	m.message = fmt.Sprintf("loops go to bank %c", 'A'+m.loopBank())
	return m, m.wanted()
}

// looperBarsNext goes to the next length of a loop.
func (m Model) looperBarsNext() (tea.Model, tea.Cmd) {
	if m.looper().phase != lpIdle {
		m.message = "the length cannot change while a take is on"
		return m, nil
	}
	i := slices.Index(loopBarChoices, m.loopBars())
	m.mem.LooperBars = loopBarChoices[(i+1)%len(loopBarChoices)]
	m.message = fmt.Sprintf("a loop is %s long", plural(m.mem.LooperBars, "bar"))
	return m, nil
}

// looperDelayStep changes how many milliseconds the recording is behind the clock.
func (m Model) looperDelayStep(step int) (tea.Model, tea.Cmd) {
	m.mem.LooperDelay = clamp(m.mem.LooperDelay+step, -250, 500)
	m.message = fmt.Sprintf("the recording is %d ms behind the clock: loops are taken that much later from the recording", m.mem.LooperDelay)
	return m, nil
}

// looperDelete deletes the sample of a pad (0 to 15) of the bank, on the last beat of the pattern when the clock runs
// (asking again takes it back).
func (m Model) looperDelete(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i > 15 || m.snap == nil {
		return m, nil
	}
	m.looper() // made here, so that the queue stays with the model that goes back
	row := m.snap.Bank(m.loopBank())[i]
	key := fmt.Sprintf("del:%d", i)
	if m.looperUnlater(key) {
		m.message = fmt.Sprintf("%s is no longer deleted on the last beat", row.Pad)
		return m, nil
	}
	if !row.HasSample {
		m.message = fmt.Sprintf("%s holds no sample", row.Pad)
		return m, nil
	}
	if m.looperLater(lpJob{key: key, label: "delete " + row.Pad.String(), cmd: m.deleteCmd([]proto.PadIndex{row.Pad})}) {
		m.message = fmt.Sprintf("%s is deleted on the last beat of the pattern, in %.1f beats (delete it again to keep it)", row.Pad, m.beatsToLast())
		return m, nil
	}
	m.busy = "deleting " + row.Pad.String()
	return m, m.deleteCmd([]proto.PadIndex{row.Pad})
}

// looperArm starts a take: the recorder comes up now, and the loop begins on the next beat.
func (m Model) looperArm() (tea.Model, tea.Cmd) { return m.looperArmWith(false) }

// looperArmWith starts a take, or a calibration (cal) that measures the delay.
func (m Model) looperArmWith(cal bool) (tea.Model, tea.Cmd) {
	lp := m.looper()
	switch lp.phase {
	case lpStarting, lpArmed, lpRecording:
		return m.looperCancel("the take was cancelled")
	case lpSaving:
		m.message = "the last take is still being put on its pad"
		return m, nil
	}
	switch {
	case m.recorder == nil && m.ring == nil:
		m.message = record.ErrNoTool.Error()
		return m, nil
	case !m.clockRuns():
		m.message = "the Looper follows the unit's MIDI clock: start its sequencer"
		return m, nil
	case m.rec != nil && m.ring == nil:
		m.message = "a recording is running: Ctrl-R ends it first"
		return m, nil
	}
	pad, ok := m.loopFreePad()
	if !ok && !cal {
		m.message = fmt.Sprintf("bank %c has no free pad: d deletes a loop", 'A'+m.loopBank())
		return m, nil
	}
	lp.target, lp.cal, lp.ringTake = pad, cal, m.ring != nil
	if lp.ringTake {
		// The program listens to the input all the time: nothing to start, and the meter goes on.
		lp.phase = lpArmed
	} else {
		if m.lvl != nil && m.lvl.src != nil {
			m.lvl.src.Stop() // the recorder takes the input
			m.lvl = nil
		}
		m.lvl = newLevelMeter() // the level is the end of the take's file
		lp.phase = lpStarting
	}
	if cal {
		m.message = "calibrating: one bar from the next bar; a hit (a kick) on beat 1 of the pattern, everything else muted"
	} else {
		m.message = fmt.Sprintf("armed: %s from the next %s go to %s", plural(m.loopBars(), "bar"), map[bool]string{true: "bar", false: "beat"}[!m.mem.LooperOnBeat], pad)
	}
	if lp.ringTake {
		return m, nil
	}
	b, device := m.recorder, m.recordDevice()
	return m, func() tea.Msg {
		s, err := b.Start(loopDir(), device)
		return loopStartedMsg{session: s, err: err}
	}
}

// looperCancel drops a take that has not been saved: the recording is stopped and thrown away.
func (m Model) looperCancel(message string) (tea.Model, tea.Cmd) {
	lp := m.looper()
	s := lp.session
	lp.phase, lp.session, lp.cal, lp.ringTake = lpIdle, nil, false, false
	if m.ring == nil {
		m.lvl = nil
	}
	m.message = message
	if s == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		_ = s.Stop()
		_ = os.Remove(s.Path)
		return nil
	}
}

// loopStarted is the recorder coming up.
func (m Model) loopStarted(msg loopStartedMsg) (tea.Model, tea.Cmd) {
	lp := m.looper()
	if msg.err != nil {
		lp.phase = lpIdle
		m.lvl = nil
		m.message = "the recorder did not start: " + msg.err.Error()
		return m, nil
	}
	if lp.phase != lpStarting {
		// Cancelled while it started.
		s := msg.session
		return m, func() tea.Msg { _ = s.Stop(); _ = os.Remove(s.Path); return nil }
	}
	lp.phase, lp.session = lpArmed, msg.session
	return m, nil
}

// looperTick moves a take on with the clock: the start of a beat starts it, the end of the loop finishes it. at is when the
// tick came.
func (m Model) looperTick(at time.Time) tea.Cmd {
	return tea.Batch(m.looperJobsTick(), m.looperTickTake(at))
}

func (m Model) looperTickTake(at time.Time) tea.Cmd {
	lp := m.lp
	if lp == nil || m.clock == nil {
		return nil
	}
	length := m.takeBars() * ticksPerBar
	switch lp.phase {
	case lpArmed:
		// The recorder needs a moment before it hears anything: the start comes after that.
		if m.clock.ticks%m.takeAlign() != 0 || (!lp.ringTake && time.Since(lp.session.Started) < loopWarmup+max(-m.loopDelay(), 0)) {
			return nil
		}
		lp.phase, lp.startTick, lp.t0 = lpRecording, m.clock.ticks, at
		m.log.Info("looper: start", "tick", m.clock.ticks, "align", m.takeAlign(), "bars", m.takeBars(), "bpm", m.mon.bpm,
			"ring", lp.ringTake, "event_age_ms", time.Since(at).Milliseconds())
	case lpRecording:
		if m.clock.ticks-lp.startTick < length {
			return nil
		}
		lp.phase, lp.t1 = lpSaving, at
		m.log.Info("looper: end", "tick", m.clock.ticks, "ticks", m.clock.ticks-lp.startTick, "event_age_ms", time.Since(at).Milliseconds())
		return m.looperSave()
	}
	return nil
}

// looperSave stops the recorder a moment after the loop ended, cuts the loop out of the file and puts it on the
// pad.
func (m Model) looperSave() tea.Cmd {
	lp := m.lp
	if lp.ringTake {
		return m.looperSaveRing()
	}
	s, t0, t1, pad, delay := lp.session, lp.t0, lp.t1, lp.target, m.loopDelay()
	gain, normalize, cal := float64(m.mem.LooperGain), m.mem.LooperNormalize, lp.cal
	lp.takes++
	name := fmt.Sprintf("LOOP %s", t1.Format("15:04:05"))
	unit := m.unit
	edit := m.edit
	log := m.log
	return func() tea.Msg {
		time.Sleep(loopSettle)
		stopped := time.Now()
		err := s.Stop()
		if err != nil && s.Size() == 0 {
			return loopSavedMsg{pad: pad, err: fmt.Errorf("the recorder: %w", err)}
		}
		imported, err := formats.ReadAudio(s.Path)
		if err != nil {
			return loopSavedMsg{pad: pad, err: fmt.Errorf("reading the take (%s): %w", s.Path, err)}
		}
		log.Info("looper: take", "loop_ms", t1.Sub(t0).Milliseconds(), "stop_after_end_ms", stopped.Sub(t1).Milliseconds(),
			"file_frames", imported.Sample.Frames(), "file_ms", imported.Sample.Frames()*1000/formats.SampleRate,
			"source_rate", imported.SourceRate, "delay_ms", delay.Milliseconds(), "gain_db", gain, "normalize", normalize,
			"calibration", cal, "tool", s.Tool, "file", s.Path)
		if cal {
			// With no delay the hit on beat 1 is at the start of the loop, and as late as the recording is.
			sample, err := app.CutLoop(imported.Sample, stopped, t0, t1, 0)
			if err != nil {
				return loopSavedMsg{err: err, cal: true}
			}
			ms, err := app.HitDelay(sample)
			log.Info("looper: calibration", "hit_ms", ms, "peak_db", app.PeakDB(sample), "err", err)
			_ = os.Remove(s.Path)
			return loopSavedMsg{err: err, cal: true, delay: ms}
		}
		sample, err := app.CutLoop(imported.Sample, stopped, t0, t1, delay)
		if err != nil {
			return loopSavedMsg{pad: pad, err: err}
		}
		sample, applied := app.ApplyGain(sample, gain, normalize)
		peak := app.PeakDB(sample)
		log.Info("looper: loop", "frames", sample.Frames(), "peak_db", peak, "gain_applied_db", applied)
		msg := edit(func() tea.Msg {
			if _, err := app.PutSampleLive(unit, pad, sample, name); err != nil {
				return loopSavedMsg{pad: pad, err: err}
			}
			// Ungated: the loop plays to its end after the pattern hits the pad.
			if info, ok := proto.ParamByName("gate"); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(pad), 0)
			}
			return loopSavedMsg{pad: pad, name: name, peak: peak, gain: applied}
		})()
		if saved, ok := msg.(loopSavedMsg); ok && saved.err == nil {
			_ = os.Remove(s.Path)
		}
		return msg
	}
}

// looperSaveRing cuts the loop out of the memory the program listens into, once the end of it has arrived, and puts
// it on the pad.
func (m Model) looperSaveRing() tea.Cmd {
	lp, ring := m.lp, m.ring
	t0, t1, pad, delay := lp.t0, lp.t1, lp.target, m.loopDelay()
	gain, normalize, cal := float64(m.mem.LooperGain), m.mem.LooperNormalize, lp.cal
	lp.takes++
	name := fmt.Sprintf("LOOP %s", t1.Format("15:04:05"))
	unit, edit, log := m.unit, m.edit, m.log
	return func() tea.Msg {
		cut := func(delay time.Duration) (formats.Sample, error) {
			start := ring.FrameAt(t0.Add(delay))
			frames := int64(app.LoopFrames(t1.Sub(t0)))
			if !ring.WaitFor(start+frames, 2*time.Second) {
				return formats.Sample{}, errors.New("the end of the loop never came from the input: is it still there?")
			}
			data, err := ring.Copy(start, frames)
			if err != nil {
				return formats.Sample{}, err
			}
			log.Info("looper: take", "ring", true, "loop_ms", t1.Sub(t0).Milliseconds(), "start_frame", start, "frames", frames, "ring_total", ring.Total(),
				"delay_ms", delay.Milliseconds(), "calibration", cal)
			return app.FadeLoop(formats.Sample{Channels: 2, Samples: data}), nil
		}
		if cal {
			sample, err := cut(0)
			if err != nil {
				return loopSavedMsg{err: err, cal: true}
			}
			ms, err := app.HitDelay(sample)
			log.Info("looper: calibration", "hit_ms", ms, "peak_db", app.PeakDB(sample), "err", err)
			return loopSavedMsg{err: err, cal: true, delay: ms}
		}
		sample, err := cut(delay)
		if err != nil {
			return loopSavedMsg{pad: pad, err: err}
		}
		sample, applied := app.ApplyGain(sample, gain, normalize)
		peak := app.PeakDB(sample)
		log.Info("looper: loop", "frames", sample.Frames(), "peak_db", peak, "gain_applied_db", applied)
		return edit(func() tea.Msg {
			if _, err := app.PutSampleLive(unit, pad, sample, name); err != nil {
				return loopSavedMsg{pad: pad, err: err}
			}
			if info, ok := proto.ParamByName("gate"); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(pad), 0)
			}
			return loopSavedMsg{pad: pad, name: name, peak: peak, gain: applied}
		})()
	}
}

// loopSaved is a take that ended.
func (m Model) loopSaved(msg loopSavedMsg) (tea.Model, tea.Cmd) {
	lp := m.looper()
	lp.phase, lp.session, lp.cal, lp.ringTake = lpIdle, nil, false, false
	if m.ring == nil {
		m.lvl = nil
	}
	if msg.cal {
		if msg.err != nil {
			m.message = "calibration failed: " + msg.err.Error()
			return m, nil
		}
		lp.calMS, lp.calDone = msg.delay, true
		m.mem.LooperDelay = clamp(msg.delay, -250, 500)
		m.message = fmt.Sprintf("calibrated: the recording is %d ms behind the clock, the delay is set to it", msg.delay)
		return m, nil
	}
	if msg.err != nil {
		m.message = "the take was lost: " + msg.err.Error()
		return m, nil
	}
	lp.cursor = int(msg.pad.Pad())
	lp.last = loopLast{has: true, name: msg.name, pad: msg.pad.String(), peak: msg.peak, gain: msg.gain}
	m.message = fmt.Sprintf("%s is on %s (%s): it plays from the next round of the pattern", msg.name, msg.pad, loudness(msg.peak, msg.gain))
	m.busy = "reading the unit"
	return m, m.load(m.message)
}

// loudness says how loud a loop is: its peak, and what was added to it.
func loudness(peak, gain float64) string {
	text := "peak " + dbText(math.Pow(10, peak/20))
	text = strings.TrimSpace(text)
	if gain != 0 {
		text += fmt.Sprintf(", %+.1f dB added", gain)
	}
	return text
}

// looperGainStep changes how much louder a loop is made.
func (m Model) looperGainStep(step int) (tea.Model, tea.Cmd) {
	m.mem.LooperGain = clamp(m.mem.LooperGain+step, -24, 24)
	m.message = fmt.Sprintf("loops are made %+d dB louder (n makes them as loud as they can be instead)", m.mem.LooperGain)
	return m, nil
}

// looperNormalizeToggle switches making every loop as loud as it can be (peak -1 dB).
func (m Model) looperNormalizeToggle() (tea.Model, tea.Cmd) {
	m.mem.LooperNormalize = !m.mem.LooperNormalize
	if m.mem.LooperNormalize {
		m.message = "normalise on: every loop gets a peak of -1 dB, the gain is ignored"
	} else {
		m.message = fmt.Sprintf("normalise off: loops are made %+d dB louder", m.mem.LooperGain)
	}
	return m, nil
}

// looperAlignToggle switches between starting on the next beat and on the next bar.
func (m Model) looperAlignToggle() (tea.Model, tea.Cmd) {
	m.mem.LooperOnBeat = !m.mem.LooperOnBeat
	m.message = map[bool]string{false: "a take starts on beat 1 of the next bar", true: "a take starts on the next beat"}[m.mem.LooperOnBeat]
	return m, nil
}

// looperWatch ends a take when the clock has stopped: it would wait for ever.
func (m Model) looperWatch() Model {
	m = m.looperJobsWatch()
	lp := m.lp
	if lp == nil || (lp.phase != lpArmed && lp.phase != lpRecording) || m.clockRuns() {
		return m
	}
	nm, _ := m.looperCancel("the clock stopped: the take was dropped")
	return nm.(Model)
}

// looperPattern writes the pattern of the bank in which all 16 pads start on beat 1.
func (m Model) looperPattern() (tea.Model, tea.Cmd) {
	unit, bank, bars := m.unit, m.loopBank(), m.loopBars()
	m.busy = "making a pattern"
	return m, m.edit(func() tea.Msg {
		slot, err := firstFreePatternSlot(unit, bank)
		if err != nil {
			return doneMsg{message: err.Error()}
		}
		pads := proto.AllPads()[bank*proto.PadsPerBank : (bank+1)*proto.PadsPerBank]
		if err := app.WriteStartPattern(unit, slot, pads, bars); err != nil {
			return doneMsg{message: "the pattern was not made: " + err.Error()}
		}
		return doneMsg{message: fmt.Sprintf("pattern %s made (bank %c, pad %d): all 16 pads on beat 1, %s", slot, 'A'+slot.Bank(), slot.Pad()+1, plural(bars, "bar")), reload: true, patterns: true}
	})
}

// looperBadge is the mark in the header while a take is on, so it shows in every view.
func (m Model) looperBadge() string {
	lp := m.lp
	if lp == nil || lp.phase == lpIdle {
		return ""
	}
	bg := m.st.th.bus1
	text := " LOOPER armed "
	switch lp.phase {
	case lpStarting:
		text = " LOOPER starting… "
	case lpRecording:
		dot := "●"
		if (m.frame/4)%2 == 1 {
			dot = "○"
		}
		bg = m.st.th.hot
		text = fmt.Sprintf(" %s LOOPER REC %d/%d ", dot, min((m.clock.ticks-lp.startTick)/ticksPerBar+1, m.takeBars()), m.takeBars())
	case lpSaving:
		text = " LOOPER saving… "
	}
	return lipgloss.NewStyle().Background(bg).Foreground(m.st.th.cursorFg).Bold(true).Render(text)
}

// looperBody is the Looper view.
func (m Model) looperBody(g geom) string {
	st, lp := m.st, m.looper()
	bank := m.loopBank()
	width := g.bodyW + g.bodyX - 3
	head := lipgloss.NewStyle().Foreground(st.th.accent).Bold(true).Render(fmt.Sprintf("BANK %c", 'A'+bank)) +
		st.dim.Render("   loop ") + st.bold.Render(plural(m.loopBars(), "bar")) +
		st.dim.Render(fmt.Sprintf("   delay %d ms   ", m.mem.LooperDelay))
	if m.mem.LooperNormalize {
		head += st.bold.Render("normalise")
	} else {
		head += st.dim.Render(fmt.Sprintf("gain %+d dB", m.mem.LooperGain))
	}
	if clock := m.clockText(m.loopBars()); clock != "" {
		head += "   " + lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+clock)
	}
	var status string
	switch lp.phase {
	case lpIdle:
		if pad, ok := m.loopFreePad(); ok {
			status = st.text.Render(fmt.Sprintf("READY  a arms: the next %s from the next %s go to %s", plural(m.loopBars(), "bar"), map[bool]string{true: "bar", false: "beat"}[!m.mem.LooperOnBeat], pad))
		} else {
			status = lipgloss.NewStyle().Foreground(st.th.hot).Render(fmt.Sprintf("bank %c is full: d deletes a loop", 'A'+bank))
		}
	case lpStarting:
		status = lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render("STARTING THE RECORDER…")
	case lpArmed:
		align := m.takeAlign()
		left := (align - m.clock.ticks%align) % align
		status = lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render(fmt.Sprintf("ARMED  starts in %.1f beats  ·  goes to %s  ·  a or Esc cancels", float64(left)/24, lp.target))
	case lpRecording:
		status = lipgloss.NewStyle().Background(st.th.hot).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf(" ● REC  bar %d of %d  →  %s ", min((m.clock.ticks-lp.startTick)/ticksPerBar+1, m.takeBars()), m.takeBars(), lp.target))
	case lpSaving:
		status = lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render(fmt.Sprintf("SAVING to %s…", lp.target))
	}
	if t := m.laterText(); t != "" {
		status += "\n" + lipgloss.NewStyle().Foreground(st.th.hot).Bold(true).Render(t)
	}
	pattern := st.dim.Render("pattern: reading…")
	if rows := m.patterns[bank]; rows != nil {
		pattern = lipgloss.NewStyle().Foreground(st.th.hot).Render("no pattern with all 16 pads: p makes one")
		for i, r := range rows {
			if r.Exists && len(r.Pads) == proto.PadsPerBank {
				pattern = st.dim.Render(fmt.Sprintf("pattern %s plays all 16 pads", rows[i].Slot))
				break
			}
		}
	}
	keys := st.dim.Render("a arm   b length   [ ] bank   p pattern   d delete   g G gain   n normalise   t beat/bar   s slices   x remix   c calibrate   , . delay   h j k l pad")
	top := []string{head, "", status, pattern, ""}
	bottom := []string{"", keys}
	avail := g.bodyH - 2 - len(top) - len(bottom) - 3
	tileH := max(avail/4, 3)
	tileW := max((width-3)/4, 10)
	rows := m.snap.Bank(bank)
	target, hasTarget := m.loopFreePad()
	lines := append([]string{}, top...)
	lines = append(lines, m.grid(g, func(p int) string {
		row := rows[p]
		ts := tileState{
			empty:    !row.HasSample,
			bus:      row.Params["bus-fx"],
			flags:    padFlags(row),
			cursor:   lp.cursor == p,
			selected: hasTarget && target == row.Pad && lp.phase != lpIdle,
			bars:     m.padBars(row, tileW),
			noLevel:  false,
		}
		return m.tile(tileW, tileH, fmt.Sprint(row.Pad.Pad()+1), row.Name, row.Level, ts)
	}))
	lines = append(lines, bottom...)
	return m.box("Looper", strings.Join(lines, "\n"), g.bodyW+g.bodyX-1, g.bodyH, true)
}
