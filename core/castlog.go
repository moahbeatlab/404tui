package core

import (
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/perflog"
)

// The terminal cast: while a recording runs, the keys that are pressed and the screen of the terminal program as it changes go into the
// event log, so that the replay can show the terminal and the keys. A "key" line has the key's name; a "screen" line has the lines of the
// screen that changed (with their colours), and a whole screen every few seconds and when the size changes. The terminal program
// (platform/terminal) calls CastKey and CastScreen.

// castClock is the time of the cast's lines (a script that makes a take sets its own).
var castClock = time.Now

const castKeyframe = 4 * time.Second

type castState struct {
	last     []string
	w        int
	keyframe time.Time
}

// CastTo makes the log the one the keys and the screen go to (nil: none). A recording sets it itself.
func (a *App) CastTo(l *perflog.Log) {
	a.cast = castState{}
	a.castLog = l
}

// CastKey writes a key press (its name as the terminal program calls it: "z", "tab", "ctrl+z", "shift+tab").
func (a *App) CastKey(name string) {
	if a.castLog == nil || name == "" {
		return
	}
	a.castLog.Add(castClock(), "key", perflog.P("key", name))
}

// CastScreen writes what changed on the screen since the last one that was written.
func (a *App) CastScreen(out string, width int) {
	if a.castLog == nil {
		return
	}
	c := &a.cast
	lines := strings.Split(out, "\n")
	now := castClock()
	if len(c.last) == len(lines) && c.w == width && now.Sub(c.keyframe) < castKeyframe {
		diff := map[int]string{}
		for i, l := range lines {
			if l != c.last[i] {
				diff[i] = l
			}
		}
		if len(diff) == 0 {
			return
		}
		a.castLog.Add(now, "screen", perflog.P("d", diff))
		c.last = lines
		return
	}
	a.castLog.Add(now, "screen", perflog.P("w", width), perflog.P("h", len(lines)), perflog.P("lines", lines))
	c.last, c.w, c.keyframe = lines, width, now
}

// SetCastClock sets the time of the cast's lines and gives back the one it had (a script that makes a take drives it by hand).
func SetCastClock(f func() time.Time) func() time.Time {
	prev := castClock
	castClock = f
	return prev
}
