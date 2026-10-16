package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/record"
)

// The level meter of a recording: how loud the sound being recorded is, per channel, with the highest
// peak held for a moment and a clip light that stays on for a few seconds after the sound reached
// full scale (past which it is distorted and cannot be mended).

const (
	meterWindow = 100 * time.Millisecond // how far back a reading looks
	meterHold   = 1500 * time.Millisecond
	clipHold    = 5 * time.Second
	// clipAt is the peak, in dB below full scale, from which a sound counts as clipping; warnAt is
	// where the meter goes red, a little under: not much room left.
	clipAt = -0.3
	warnAt = -3.0
	// meterFloor is the quietest the meter shows.
	meterFloor = -60.0
)

// levelMeter is what the recording's level has been lately.
type levelMeter struct {
	read    time.Time
	peak    [2]float64 // dB now
	hold    [2]float64
	holdAt  [2]time.Time
	clipped time.Time // when the sound last clipped
	top     float64   // the highest dB of the whole recording
	ok      bool
	// src is the program that listens to the input when nothing is being recorded; nil while the
	// level is read from the file of a recording.
	src *record.Meter
}

func newLevelMeter() *levelMeter {
	return &levelMeter{hold: [2]float64{math.Inf(-1), math.Inf(-1)}, peak: [2]float64{math.Inf(-1), math.Inf(-1)}, top: math.Inf(-1)}
}

// sampleLevel reads the level of what is heard: the end of the recording's file while one runs, else
// what the listening program says, a few times a second.
func (m Model) sampleLevel() {
	if m.lvl == nil || time.Since(m.lvl.read) < 90*time.Millisecond {
		return
	}
	now := time.Now()
	m.lvl.read = now
	var peaks []float64
	switch {
	case m.lvl.src != nil:
		p, ok := m.lvl.src.Level()
		if !ok {
			return
		}
		peaks = p[:]
	case m.rec != nil:
		l, err := record.ReadLevel(m.rec.Path, meterWindow)
		if err != nil || len(l.Peak) == 0 {
			return
		}
		peaks = l.Peak
	default:
		return
	}
	m.lvl.ok = true
	for c := 0; c < 2; c++ {
		db := record.DB(peaks[min(c, len(peaks)-1)])
		m.lvl.peak[c] = db
		if db >= m.lvl.hold[c] || now.Sub(m.lvl.holdAt[c]) > meterHold {
			m.lvl.hold[c], m.lvl.holdAt[c] = db, now
		}
		if db >= clipAt {
			m.lvl.clipped = now
		}
		if db > m.lvl.top {
			m.lvl.top = db
		}
	}
}

// meterEnded is the listening program ending: wait for it, so the meter can be started again.
type meterStartedMsg struct {
	meter *record.Meter
	err   error
}
type meterEndedMsg struct{ meter *record.Meter }

func waitMeter(mt *record.Meter) tea.Cmd {
	return func() tea.Msg {
		<-mt.Done()
		return meterEndedMsg{meter: mt}
	}
}

// meterRetry is how long to wait before listening again after it failed (the unit may be plugged in).
const meterRetry = 5 * time.Second

// syncMeter starts the listening program when the Perform view is open and nothing records, and ends
// it when either stops being so: the level is there to set the volume by, whether or not it records.
func (m Model) syncMeter() (Model, tea.Cmd) {
	if m.section != secPerform || m.rec != nil {
		if m.lvl != nil && m.lvl.src != nil {
			m.lvl.src.Stop()
			m.lvl = nil
		}
		return m, nil
	}
	mt, ok := m.recorder.(record.Metering)
	if !ok || m.lvl != nil || m.meterBusy || time.Now().Before(m.meterAgain) {
		return m, nil
	}
	m.meterBusy = true
	device := m.recordDevice()
	return m, func() tea.Msg {
		meter, err := mt.StartMeter(device)
		return meterStartedMsg{meter: meter, err: err}
	}
}

// meterStarted takes in the listening program that started, or why it did not.
func (m Model) meterStarted(msg meterStartedMsg) (Model, tea.Cmd) {
	m.meterBusy = false
	if msg.err != nil {
		m.meterErr, m.meterAgain = msg.err.Error(), time.Now().Add(meterRetry)
		return m, nil
	}
	if m.section != secPerform || m.rec != nil || m.lvl != nil {
		msg.meter.Stop()
		return m, nil
	}
	m.meterErr = ""
	m.lvl = newLevelMeter()
	m.lvl.src = msg.meter
	return m, waitMeter(msg.meter)
}

// meterEnded takes in the listening program ending by itself (the unit was switched off).
func (m Model) meterEnded(msg meterEndedMsg) Model {
	if m.lvl != nil && m.lvl.src == msg.meter {
		m.lvl = nil
		m.meterErr = "the input is gone"
		if err := msg.meter.Err(); err != nil {
			m.meterErr = err.Error()
		}
		m.meterAgain = time.Now().Add(meterRetry)
	}
	return m
}

// clipping says whether the sound clipped lately.
func (l *levelMeter) clipping(now time.Time) bool {
	return l != nil && !l.clipped.IsZero() && now.Sub(l.clipped) < clipHold
}

// levelColour is the colour of a level: green, then yellow, then red near full scale.
func (m Model) levelColour(db float64) lipgloss.Color {
	th := m.st.th
	switch {
	case db >= warnAt:
		return th.bad
	case db >= -12:
		return th.warn
	}
	return th.good
}

// meterBar draws one channel of the meter width cells wide, from meterFloor to full scale, in the
// colours of the level, with the held peak as a mark.
func (m Model) meterBar(db, hold float64, width int) string {
	st := m.st
	pos := func(v float64) int {
		if math.IsInf(v, -1) || v <= meterFloor {
			return 0
		}
		return clamp(int(math.Round((v-meterFloor)/(-meterFloor)*float64(width))), 0, width)
	}
	fill, mark := pos(db), pos(hold)
	var b strings.Builder
	for i := 0; i < width; i++ {
		// The colour of this cell is the level it stands for.
		cellDB := meterFloor + float64(i+1)/float64(width)*(-meterFloor)
		switch {
		case i < fill:
			b.WriteString(lipgloss.NewStyle().Foreground(m.levelColour(cellDB)).Render("█"))
		case i == mark-1 && mark > 0:
			b.WriteString(lipgloss.NewStyle().Foreground(m.levelColour(cellDB)).Bold(true).Render("▌"))
		default:
			b.WriteString(st.faint.Render("·"))
		}
	}
	return b.String()
}

func dbText(db float64) string {
	if math.IsInf(db, -1) || db < -99 {
		return " -∞ dB"
	}
	return fmt.Sprintf("%+5.1f dB", db)
}

// clipBadge is the clip light: red and loud when the sound clipped lately, a quiet OK otherwise.
func (m Model) clipBadge(now time.Time) string {
	if m.lvl.clipping(now) {
		return lipgloss.NewStyle().Background(m.st.th.bad).Foreground(m.st.th.cursorFg).Bold(true).Render(" CLIP ")
	}
	return m.st.dim.Render("  ok  ")
}

// levelLines is the big meter for the Perform view: a bar for each channel with its level in dB, and
// the clip light, with what to do about it.
func (m Model) levelLines(width int) []string {
	st := m.st
	if m.lvl == nil || !m.lvl.ok {
		switch {
		case m.rec != nil:
			return []string{st.dim.Render("reading the recording…")}
		case m.meterErr != "":
			return []string{st.dim.Render("no level: " + m.meterErr), st.dim.Render("the unit has to be on and plugged in")}
		}
		if _, ok := m.recorder.(record.Metering); !ok {
			return []string{st.dim.Render("no level: install ffmpeg (or sox) to listen")}
		}
		return []string{st.dim.Render("listening…")}
	}
	now := time.Now()
	barW := max(width-12, 10)
	var out []string
	for c, name := range []string{"L", "R"} {
		bar := m.meterBar(m.lvl.peak[c], m.lvl.hold[c], barW)
		// Two rows a channel: the level is what you set the volume by, so it is big.
		out = append(out, st.bold.Render(name)+" "+bar+st.text.Render(" "+dbText(m.lvl.hold[c])), "  "+bar)
	}
	hint := m.clipBadge(now) + st.dim.Render(fmt.Sprintf(" loudest so far %s", strings.TrimSpace(dbText(m.lvl.top))))
	if m.lvl.clipping(now) {
		hint = m.clipBadge(now) + lipgloss.NewStyle().Foreground(st.th.bad).Bold(true).Render(" too loud: turn the unit's volume down")
	}
	return append(out, hint)
}
