package tui

import (
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/perflog"
)

// The terminal cast: while a recording runs, the keys that are pressed and the screen as it changes go into the event log, so that the
// replay can show the terminal and the keys (a "key" line has the key's name; a "screen" line has the lines of the screen that changed,
// with their colours, and a whole screen every few seconds and when the size changes).

const castKeyframe = 4 * time.Second

// castState is what the last logged screen looked like. It lives in the view cache (a pointer), because View has a copy of the model.
type castState struct {
	last     []string
	w        int
	keyframe time.Time
}

// logKey writes a key press (its name as the terminal program calls it: "z", "tab", "ctrl+z", "shift+tab").
func (m Model) logKey(name string) {
	if m.perfLog == nil || name == "" {
		return
	}
	m.perfLog.Add(time.Now(), "key", perflog.P("key", name))
}

// logScreen writes what changed on the screen since the last one that was logged.
func (m Model) logScreen(out string) {
	if m.perfLog == nil || m.vc == nil {
		return
	}
	c := &m.vc.cast
	lines := strings.Split(out, "\n")
	now := time.Now()
	if len(c.last) == len(lines) && c.w == m.width && now.Sub(c.keyframe) < castKeyframe {
		diff := map[int]string{}
		for i, l := range lines {
			if l != c.last[i] {
				diff[i] = l
			}
		}
		if len(diff) == 0 {
			return
		}
		m.perfLog.Add(now, "screen", perflog.P("d", diff))
		c.last = lines
		return
	}
	m.perfLog.Add(now, "screen", perflog.P("w", m.width), perflog.P("h", len(lines)), perflog.P("lines", lines))
	c.last, c.w, c.keyframe = lines, m.width, now
}
