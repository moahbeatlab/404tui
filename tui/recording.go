package tui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/record"
)

// WithRecorder gives the model what records the unit's audio input; without one Ctrl-R says there
// is no recorder.
func (m Model) WithRecorder(b record.Backend) Model {
	m.recorder = b
	return m
}

// recordDir is the folder recordings go into: the one set in Config, else Music/404tui in the home
// folder.
func (m Model) recordDir() string {
	if m.mem.RecordDir != "" {
		return expandHome(m.mem.RecordDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Music", "404tui")
}

// recordDevice is the part of an input's name that makes it the unit's.
func (m Model) recordDevice() string {
	if m.mem.RecordDevice != "" {
		return m.mem.RecordDevice
	}
	return record.DefaultDevice
}

func expandHome(p string) string {
	if p == "~" || len(p) > 1 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// toggleRecording starts a recording of the unit's audio, or ends the one that is running.
func (m Model) toggleRecording() (tea.Model, tea.Cmd) {
	if m.rec != nil {
		s := m.rec
		m.message = "stopping the recording…"
		return m, func() tea.Msg {
			err := s.Stop()
			return recEndedMsg{session: s, path: s.Path, size: s.Size(), err: err, onPurpose: true}
		}
	}
	if m.recorder == nil {
		m.message = record.ErrNoTool.Error()
		return m, nil
	}
	if m.lvl != nil && m.lvl.src != nil {
		m.lvl.src.Stop() // the recorder takes the input
		m.lvl = nil
	}
	b, dir, device := m.recorder, m.recordDir(), m.recordDevice()
	m.message = "starting the recorder…"
	return m, func() tea.Msg {
		s, err := b.Start(dir, device)
		return recStartedMsg{session: s, err: err}
	}
}

// waitRecording ends when the recording does, by itself or because it was stopped.
func waitRecording(s *record.Session) tea.Cmd {
	return func() tea.Msg {
		<-s.Done()
		return recEndedMsg{session: s, path: s.Path, size: s.Size(), err: s.Err()}
	}
}

// stopRecordingNow ends a recording before the program quits, so the file is finished.
func (m Model) stopRecordingNow() {
	if m.lvl != nil && m.lvl.src != nil {
		m.lvl.src.Stop()
	}
	if m.rec != nil {
		_ = m.rec.Stop()
	}
	if m.ring != nil {
		m.ring.Stop()
	}
	if m.lp != nil && m.lp.session != nil {
		_ = m.lp.session.Stop()
		_ = os.Remove(m.lp.session.Path)
	}
	m.closePerfLog()
}

// closePerfLog finishes the event log of a recording.
func (m Model) closePerfLog() {
	if m.perfLog != nil {
		_ = m.perfLog.Close()
	}
}

// recBadge is the mark in the header while a recording runs: red, with how long it has gone on and
// how big the file is. Its dot blinks.
func (m Model) recBadge() string {
	if m.rec == nil {
		return m.looperBadge()
	}
	dot := "●"
	if (m.frame/4)%2 == 1 {
		dot = "○"
	}
	d := time.Since(m.rec.Started).Truncate(time.Second)
	text := fmt.Sprintf(" %s REC %02d:%02d  %s ", dot, int(d.Minutes()), int(d.Seconds())%60, humanBytes(m.rec.Size()))
	if m.lvl != nil && m.lvl.ok {
		// How loud, and a warning when it clipped: that is what to look at while playing.
		text = strings.TrimRight(text, " ") + "  " + strings.TrimSpace(dbText(math.Max(m.lvl.hold[0], m.lvl.hold[1]))) + " "
		if m.lvl.clipping(time.Now()) {
			text += "CLIP! "
		}
	}
	return lipgloss.NewStyle().Background(m.st.th.hot).Foreground(m.st.th.cursorFg).Bold(true).Render(text)
}

// humanBytes is a size as KB, MB or GB.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
