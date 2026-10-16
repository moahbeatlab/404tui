package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/device"
)

// A pad code is a run of pads hit on the unit in turn, any bank: 1 3 3 7 for the live mode, 1 16 16 16
// for a recording. The unit sends nothing when a pad is hit; what the app sees is the pad the unit has
// selected, and how far the pad that sounds has played. A pad that is hit again while it is the
// selected one shows as its position starting over, so the pads of a code that repeats need a
// sample, and a hit has to last (or be as long as) a poll, a tenth of a second or so.

// Default codes.
const (
	defaultLiveCode = "1 3 3 7"
	defaultRecCode  = "1 16 16 16"
	// codeWindow is the longest a code may take, from its first hit to its last.
	codeWindow = 4 * time.Second
)

type codeHit struct {
	pad int
	at  time.Time
}

// codeState holds the hits of the last moments. It is shared by the copies of the model.
type codeState struct{ hits []codeHit }

// parseCode reads a code written as numbers 1 to 16 with spaces, commas or dashes between them;
// empty or "off" is no code.
func parseCode(s string) []int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "off" || s == "-" {
		return nil
	}
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '-' || r == '.' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > 16 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// feed notes a hit of a pad (1 to 16) and says which of the codes it completed, or -1. The hits
// of a completed code are forgotten, so that it has to be played again to count again.
func (c *codeState) feed(pad int, now time.Time, codes ...[]int) int {
	c.hits = append(c.hits, codeHit{pad: pad, at: now})
	keep := c.hits[:0]
	for _, h := range c.hits {
		if now.Sub(h.at) <= codeWindow {
			keep = append(keep, h)
		}
	}
	c.hits = keep
	for i, code := range codes {
		if len(code) == 0 || len(c.hits) < len(code) {
			continue
		}
		tail := c.hits[len(c.hits)-len(code):]
		match := true
		for k, p := range code {
			if tail[k].pad != p {
				match = false
				break
			}
		}
		if match {
			c.hits = nil
			return i
		}
	}
	return -1
}

func (m Model) liveCode() []int {
	if m.mem.LiveCode != "" {
		return parseCode(m.mem.LiveCode)
	}
	return parseCode(defaultLiveCode)
}

func (m Model) recCode() []int {
	if m.mem.RecCode != "" {
		return parseCode(m.mem.RecCode)
	}
	return parseCode(defaultRecCode)
}

// codesOn says whether any code is set, for polling the unit faster.
func (m Model) codesOn() bool { return len(m.liveCode()) > 0 || len(m.recCode()) > 0 }

// pollEvery is how many ticks pass between polls of the unit: every tick while a code is waited for.
func (m Model) pollEvery() int {
	if m.codesOn() && m.unit != nil {
		return 1
	}
	return pollTicks
}

// padHit tells whether a poll shows a pad hit: another pad selected, or the pad that was already
// selected sounding again from its start.
func (m Model) padHit(selected bool, a device.Activity) bool {
	if !a.HasSelected {
		return false
	}
	switch {
	case !selected || a.Selected != m.selected:
		return true
	case a.Playing && (!m.playing || a.Position < m.playPos):
		return true
	}
	return false
}

// codeHit feeds a hit to the codes and does what the one it completes says.
func (m Model) codeHit(pad int) (Model, tea.Cmd) {
	if m.codes == nil {
		m.codes = &codeState{}
	}
	switch m.codes.feed(pad, time.Now(), m.liveCode(), m.recCode()) {
	case 0:
		return m.toggleLive()
	case 1:
		next, cmd := m.toggleRecording()
		return next.(Model), cmd
	}
	return m, nil
}

// toggleLive goes to the Perform view, or back to where it came from.
func (m Model) toggleLive() (Model, tea.Cmd) {
	if m.liveOn {
		m.liveOn = false
		back := m.liveBack
		next, cmd := m.goTo(back)
		nm := next.(Model)
		nm.message = "live mode off"
		return nm, cmd
	}
	m.liveOn, m.liveBack = true, m.section
	if m.liveBack == secPerform {
		m.liveBack = secPads
	}
	next, cmd := m.goTo(secPerform)
	nm := next.(Model)
	nm.message = fmt.Sprintf("live mode on: %s again leaves it", codeText(m.liveCode()))
	return nm, cmd
}

func codeText(code []int) string {
	parts := make([]string, len(code))
	for i, n := range code {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, " ")
}
