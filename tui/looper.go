package tui

import (
	"fmt"
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
// beginning on bar 1 of the unit's clock, and puts it on the next free pad of a bank, ungated. The bank has a
// pattern in which all 16 pads start on beat 1, so the loop plays from the next round on. A pad that is no longer
// wanted is deleted: its note in the pattern is silent then, and the pad is free for the next take.

type looperPhase int

const (
	lpIdle      looperPhase = iota
	lpStarting              // the recorder is being started
	lpArmed                 // recording, waiting for bar 1
	lpRecording             // the loop is being played into the file
	lpSaving                // the take is being cut and put on its pad
)

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
	case "[":
		return m.looperBankStep(-1)
	case "]":
		return m.looperBankStep(1)
	case "b":
		return m.looperBarsNext()
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
	m.mem.LooperDelay = clamp(m.mem.LooperDelay+step, 0, 500)
	m.message = fmt.Sprintf("the recording is %d ms behind the clock: loops are taken that much later from the recording", m.mem.LooperDelay)
	return m, nil
}

// looperDelete deletes the sample of a pad (0 to 15) of the bank.
func (m Model) looperDelete(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i > 15 {
		return m, nil
	}
	row := m.snap.Bank(m.loopBank())[i]
	if !row.HasSample {
		m.message = fmt.Sprintf("%s holds no sample", row.Pad)
		return m, nil
	}
	unit, target := m.unit, row.Pad
	m.busy = "deleting " + target.String()
	return m, m.edit(func() tea.Msg {
		if failed := app.DeletePads(unit, []proto.PadIndex{target}); len(failed) > 0 {
			return doneMsg{message: "not deleted: " + strings.Join(failed, ", "), reload: true}
		}
		return doneMsg{message: fmt.Sprintf("%s deleted", target), reload: true}
	})
}

// looperArm starts a take: the recorder comes up now, and the loop begins on the next bar 1.
func (m Model) looperArm() (tea.Model, tea.Cmd) {
	lp := m.looper()
	switch lp.phase {
	case lpStarting, lpArmed, lpRecording:
		return m.looperCancel("the take was cancelled")
	case lpSaving:
		m.message = "the last take is still being put on its pad"
		return m, nil
	}
	switch {
	case m.recorder == nil:
		m.message = record.ErrNoTool.Error()
		return m, nil
	case !m.clockRuns():
		m.message = "the Looper follows the unit's MIDI clock: start its sequencer"
		return m, nil
	case m.rec != nil:
		m.message = "a recording is running: Ctrl-R ends it first"
		return m, nil
	}
	pad, ok := m.loopFreePad()
	if !ok {
		m.message = fmt.Sprintf("bank %c has no free pad: d deletes a loop", 'A'+m.loopBank())
		return m, nil
	}
	if m.lvl != nil && m.lvl.src != nil {
		m.lvl.src.Stop() // the recorder takes the input
		m.lvl = nil
	}
	lp.phase, lp.target = lpStarting, pad
	m.message = fmt.Sprintf("armed: %s from bar 1 go to %s", plural(m.loopBars(), "bar"), pad)
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
	lp.phase, lp.session = lpIdle, nil
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

// looperTick moves a take on with the clock: bar 1 starts it, the end of the loop finishes it. at is when the
// tick came.
func (m Model) looperTick(at time.Time) tea.Cmd {
	lp := m.lp
	if lp == nil || m.clock == nil {
		return nil
	}
	length := m.loopBars() * ticksPerBar
	switch lp.phase {
	case lpArmed:
		// The recorder needs a moment before it hears anything: bar 1 comes after that.
		if m.clock.ticks%length != 0 || time.Since(lp.session.Started) < 1500*time.Millisecond {
			return nil
		}
		lp.phase, lp.startTick, lp.t0 = lpRecording, m.clock.ticks, at
	case lpRecording:
		if m.clock.ticks-lp.startTick < length {
			return nil
		}
		lp.phase, lp.t1 = lpSaving, at
		return m.looperSave()
	}
	return nil
}

// looperSave stops the recorder a moment after the loop ended, cuts the loop out of the file and puts it on the
// pad.
func (m Model) looperSave() tea.Cmd {
	lp := m.lp
	s, t0, t1, pad, delay := lp.session, lp.t0, lp.t1, lp.target, m.loopDelay()
	lp.takes++
	name := fmt.Sprintf("LOOP %s", t1.Format("15:04:05"))
	unit := m.unit
	edit := m.edit
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
		sample, err := app.CutLoop(imported.Sample, stopped, t0, t1, delay)
		if err != nil {
			return loopSavedMsg{pad: pad, err: err}
		}
		msg := edit(func() tea.Msg {
			if _, err := app.PutSample(unit, pad, sample, name); err != nil {
				return loopSavedMsg{pad: pad, err: err}
			}
			// Ungated: the loop plays to its end after the pattern hits the pad.
			if info, ok := proto.ParamByName("gate"); ok {
				_ = unit.SetParam(info.ID, proto.TargetPad(pad), 0)
			}
			return loopSavedMsg{pad: pad, name: name}
		})()
		if saved, ok := msg.(loopSavedMsg); ok && saved.err == nil {
			_ = os.Remove(s.Path)
		}
		return msg
	}
}

// loopSaved is a take that ended.
func (m Model) loopSaved(msg loopSavedMsg) (tea.Model, tea.Cmd) {
	lp := m.looper()
	lp.phase, lp.session = lpIdle, nil
	if msg.err != nil {
		m.message = "the take was lost: " + msg.err.Error()
		return m, nil
	}
	lp.cursor = int(msg.pad.Pad())
	m.message = fmt.Sprintf("%s is on %s: it plays from the next round of the pattern", msg.name, msg.pad)
	m.busy = "reading the unit"
	return m, m.load(m.message)
}

// looperWatch ends a take when the clock has stopped: it would wait for ever.
func (m Model) looperWatch() Model {
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
		return doneMsg{message: fmt.Sprintf("pattern %s made: all 16 pads on beat 1, %s", slot, plural(bars, "bar")), reload: true, patterns: true}
	})
}

// looperBadge is the mark in the header while a take is on, so it shows in every view.
func (m Model) looperBadge() string {
	lp := m.lp
	if lp == nil || lp.phase == lpIdle {
		return ""
	}
	bg := m.st.th.bus1
	text := " LOOPER armed: bar 1 next "
	switch lp.phase {
	case lpStarting:
		text = " LOOPER starting… "
	case lpRecording:
		dot := "●"
		if (m.frame/4)%2 == 1 {
			dot = "○"
		}
		bg = m.st.th.hot
		text = fmt.Sprintf(" %s LOOPER REC %d/%d ", dot, min((m.clock.ticks-lp.startTick)/ticksPerBar+1, m.loopBars()), m.loopBars())
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
		st.dim.Render(fmt.Sprintf("   delay %d ms", m.mem.LooperDelay))
	if clock := m.clockText(m.loopBars()); clock != "" {
		head += "   " + lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render("♩ "+clock)
	}
	var status string
	switch lp.phase {
	case lpIdle:
		if pad, ok := m.loopFreePad(); ok {
			status = st.text.Render(fmt.Sprintf("READY  a arms: the next %s from bar 1 go to %s", plural(m.loopBars(), "bar"), pad))
		} else {
			status = lipgloss.NewStyle().Foreground(st.th.hot).Render(fmt.Sprintf("bank %c is full: d deletes a loop", 'A'+bank))
		}
	case lpStarting:
		status = lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render("STARTING THE RECORDER…")
	case lpArmed:
		length := m.loopBars() * ticksPerBar
		left := (length - m.clock.ticks%length) % length
		status = lipgloss.NewStyle().Foreground(st.th.bus1).Bold(true).Render(fmt.Sprintf("ARMED  bar 1 in %.1f beats  ·  goes to %s  ·  a or Esc cancels", float64(left)/24, lp.target))
	case lpRecording:
		status = lipgloss.NewStyle().Background(st.th.hot).Foreground(st.th.cursorFg).Bold(true).Render(fmt.Sprintf(" ● REC  bar %d of %d  →  %s ", min((m.clock.ticks-lp.startTick)/ticksPerBar+1, m.loopBars()), m.loopBars(), lp.target))
	case lpSaving:
		status = lipgloss.NewStyle().Foreground(st.th.good).Bold(true).Render(fmt.Sprintf("SAVING to %s…", lp.target))
	}
	pattern := st.dim.Render("pattern: reading…")
	if rows := m.patterns[bank]; rows != nil {
		pattern = lipgloss.NewStyle().Foreground(st.th.hot).Render("no pattern with all 16 pads: p makes one")
		for i, r := range rows {
			if r.Exists && len(r.Pads) == proto.PadsPerBank {
				pattern = st.dim.Render(fmt.Sprintf("pattern %d plays all 16 pads", i+1))
				break
			}
		}
	}
	keys := st.dim.Render("a arm   b length   [ ] bank   p pattern   d delete   , . delay   h j k l pad")
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
