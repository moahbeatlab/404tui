package tui

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Retroactive capture: the ring keeps the last seconds of the unit's input, so a phrase that has
// already been played can still be caught. What makes it a loop rather than a recording is the cut:
// the take is measured back from the trigger to a whole number of bars at the unit's tempo, so what
// lands on disk drops onto a pad without any editing.
//
// The arithmetic of the cut lives in record.Ring.Capture, where the frames are; this is the part
// that decides which bars and which tempo, and what the screen says.
//
// The clock the ring and the MIDI clock share is the trigger. The trigger is stamped as a frame of
// the ring (FrameAt) and it arrives while the clock is counting, so one bar is the same length in
// either: the take is cut as whole bars back from the trigger, never against an absolute bar line
// the two might disagree about.

// retroEar is how many bars of sound before the take are kept as the run-up, so a phrase that
// started a moment before the key was pressed is not clipped.
const retroEar = 1

// retroMaxBars is the longest take a retroactive capture will cut.
const retroMaxBars = 8

// takeLastBars is the bars of a take when the bank says nothing better.
const takeLastBars = 1

// retroReason is why a retroactive capture cannot be taken; empty when it can.
func (m Model) retroReason() string {
	switch {
	case m.ring == nil:
		if m.ringOff {
			return "no retroactive capture here: this system cannot listen to the input"
		}
		return "no retroactive capture yet: open Perform or Looper, or press R to listen to the input"
	case !m.ring.Running():
		return "no retroactive capture: the input is no longer being listened to"
	}
	if m.retroTempo() <= 0 {
		return "no retroactive capture without a tempo: start the clock on the unit, or set a bank tempo"
	}
	return ""
}

// retroTempo is the tempo a take is cut to: what the clock has just been measured at, else the
// tempo of the pattern of the bank the Looper is on, which is what a loop dropped onto it would be
// cut to.
func (m Model) retroTempo() float64 {
	if m.mon != nil && m.mon.bpm > 1 {
		return m.mon.bpm
	}
	if slot, ok := m.loopPatternSlot(); ok {
		if d := m.patPreview[slot]; d != nil && d.BankTempo > 1 {
			return d.BankTempo
		}
	}
	return 0
}

// retroBarsWanted is how long the take should be: the bars of the pattern of the bank the Looper is
// on, which is the length a loop dropped onto a free pad of it wants to be. Outside the Looper a
// take is one bar.
func (m Model) retroBarsWanted() int {
	if m.section != secLooper {
		return takeLastBars
	}
	if n := m.loopBars(); n > 0 {
		return min(n, retroMaxBars)
	}
	return takeLastBars
}

// retroFrames is how many frames the ring has to keep for a capture of bars bars with the run-up:
// the take itself, the ear before it, and a bar of slack so a take caught right after a bar line is
// not lost. It is frames, not seconds, because the slack is what keeps a capture from being refused
// for a few frames of jitter at a bar line.
func retroFrames(bars int, bpm float64, rate int) int64 {
	if bpm <= 0 || rate <= 0 || bars < 1 {
		return 0
	}
	beats := float64(bars+retroEar+1) * 4
	return int64(float64(rate) * (beats * 60 / bpm))
}

// armRetro prepares a retroactive capture and reads the trigger: the frame the key was pressed at.
// The ring is widened first so it reaches back over the bars that were asked for; the take is
// measured from the trigger, so the same call both arms the capture and stamps it when one is
// already armed.
func (m Model) armRetro() (tea.Model, tea.Cmd) {
	if why := m.retroReason(); why != "" {
		m.message = why
		return m, nil
	}
	rate := m.ring.Rate()
	bpm := m.retroTempo()
	bars := m.retroBarsWanted()
	frames := retroFrames(bars, bpm, rate)
	if frames <= 0 {
		m.message = "no retroactive capture without a tempo"
		return m, nil
	}
	// The frame the key was pressed at, and the grid the take is cut against.
	at := m.ring.FrameAt(time.Now())
	m.ring.SetGrid(bars, 4, bpm)
	// Widen wants seconds, and rounds up, so the ring reaches back over the whole take.
	secs := int((frames + int64(rate) - 1) / int64(rate))
	if grew := m.ring.Widen(secs); grew {
		m.log.Info("the ring grew for a retroactive capture", "seconds", secs)
	}
	if !m.retroArmed {
		m.retroArmed = true
		m.retroAt = at
		m.message = fmt.Sprintf("holding the last %s: play, then press again to keep %s before that",
			secondsText(float64(m.ring.Span())/float64(rate)), plural(bars, "bar"))
		return m, nil
	}
	// A second press: the phrase between the two is what is kept.
	return m.takeRetro()
}

// takeRetro cuts the take out of the ring and writes it beside the recordings.
func (m Model) takeRetro() (tea.Model, tea.Cmd) {
	ring, dir := m.ring, m.recordDir()
	trigger := m.retroAt
	m.retroArmed = false
	bars, beats, bpm := ring.Grid()
	if bars < 1 {
		bars, beats, bpm = m.retroBarsWanted(), 4, m.retroTempo()
		ring.SetGrid(bars, beats, bpm)
	}
	name := "retro " + time.Now().Format("2006-01-02_15-04-05") + ".wav"
	m.message = "cutting the take…"
	m.retroBusy = true
	return m, func() tea.Msg {
		start, end, err := ring.Capture(trigger, retroEar)
		if err != nil {
			return retroDoneMsg{err: err}
		}
		path, err := ring.Save(dir, name, start, end-start)
		if err != nil {
			return retroDoneMsg{err: err}
		}
		return retroDoneMsg{path: path, frames: end - start, bars: bars, bpm: bpm, beats: beats}
	}
}

// retroDone is a finished capture: the file that was written, or why it was not.
type retroDoneMsg struct {
	path   string
	frames int64
	bars   int
	bpm    float64
	beats  int
	err    error
}

// retroFinished says what became of the capture.
func (m Model) retroFinished(msg retroDoneMsg) (tea.Model, tea.Cmd) {
	m.retroBusy = false
	if msg.err != nil {
		m.message = "the take was not kept: " + msg.err.Error()
		return m, nil
	}
	secs := 0.0
	if rate := m.ringRate(); rate > 0 {
		secs = float64(msg.frames) / float64(rate)
	}
	m.message = fmt.Sprintf("kept %s as %s", plural(msg.bars, "bar"), filepath.Base(msg.path))
	if secs > 0 {
		m.message += fmt.Sprintf(" (%.2fs at %.1f BPM)", secs, msg.bpm)
	}
	m.retroSaved = msg.path
	m.log.Info("retroactive capture", "path", msg.path, "frames", msg.frames, "bars", msg.bars, "bpm", msg.bpm)
	return m, nil
}

// ringRate is the rate of the ring, 0 when there is none.
func (m Model) ringRate() int {
	if m.ring == nil {
		return 0
	}
	return m.ring.Rate()
}

// secondsText is a length in seconds as "8.0s".
func secondsText(secs float64) string {
	if secs < 10 {
		return fmt.Sprintf("%.1fs", secs)
	}
	return fmt.Sprintf("%.0fs", secs)
}

// retroBadge is the mark in the header while a capture is armed.
func (m Model) retroBadge() string {
	switch {
	case m.retroBusy:
		return "cutting"
	case m.retroArmed:
		return "RETRO"
	}
	return ""
}

// retroStatus is the line the Looper shows while a capture is armed.
func (m Model) retroStatus() string {
	if !m.retroArmed {
		return ""
	}
	return "retro: press again to keep the bars before this point"
}

// retroSavedPath is the last take, or "" when there has not been one. It is what the Looper shows
// so the file can be found without leaving the view.
func (m Model) retroSavedPath() string {
	return m.retroSaved
}
