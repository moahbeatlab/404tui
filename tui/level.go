package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

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
}

// sampleLevel reads the level of the recording that runs (the end of its file), a few times a second.
func (m Model) sampleLevel() {
	if m.rec == nil || m.lvl == nil || time.Since(m.lvl.read) < 90*time.Millisecond {
		return
	}
	now := time.Now()
	m.lvl.read = now
	l, err := record.ReadLevel(m.rec.Path, meterWindow)
	if err != nil || len(l.Peak) == 0 {
		return
	}
	m.lvl.ok = true
	for c := 0; c < 2; c++ {
		src := min(c, len(l.Peak)-1)
		db := record.DB(l.Peak[src])
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
	if m.rec == nil {
		return nil
	}
	now := time.Now()
	head := st.bold.Render("REC LEVEL") + "  " + m.clipBadge(now)
	if !m.lvl.ok {
		return []string{head, st.dim.Render("reading the recording…")}
	}
	barW := max(width-12, 10)
	out := []string{head}
	for c, name := range []string{"L", "R"} {
		out = append(out, st.bold.Render(name)+" "+m.meterBar(m.lvl.peak[c], m.lvl.hold[c], barW)+st.text.Render(" "+dbText(m.lvl.hold[c])))
	}
	hint := st.dim.Render(fmt.Sprintf("loudest so far %s", strings.TrimSpace(dbText(m.lvl.top))))
	if m.lvl.clipping(now) {
		hint = lipgloss.NewStyle().Foreground(st.th.bad).Bold(true).Render("too loud: turn the unit's volume or the pad levels down")
	}
	return append(out, hint)
}
