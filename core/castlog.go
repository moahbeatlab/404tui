package core

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/perflog"
	"github.com/moahbeatlab/404tui/record"
)

// The terminal cast: while it runs (F10), the keys that are pressed and the screen of the terminal program as it changes go into the
// cast's own log (name.events.jsonl, beside the WAV it plays with), so that the replay can show the terminal and the keys. A "key" line has the key's name; a "screen" line has the lines of the
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
	if a.castLog == nil || name == "" || name == "f10" {
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

// castRun is a terminal cast that runs: F10 starts it and ends it. It is a recording of its own, not the music recording of Ctrl-R:
// only the keys and the screen are logged (no MIDI, no patterns), with the unit's audio beside them unless Config says no. Without the
// audio a silent WAV of the same length is written, so that the replay has something to play along with.
type castRun struct {
	log       *perflog.Log
	sess      *record.Session // the audio; nil when the cast has none
	wav       string
	started   time.Time
	audioDone bool
}

// ToggleCast starts a terminal cast, or ends the one that runs.
func (a *App) ToggleCast() {
	if a.castRun != nil {
		a.stopCast()
		return
	}
	if a.rec != nil {
		a.say("a recording runs: stop it first (Ctrl-R)", true)
		return
	}
	r := &castRun{started: time.Now()}
	if !a.set.CastNoAudio && a.plat.Recorder != nil {
		s, err := a.plat.Recorder.Start(a.recordDir(), a.recordDevice())
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		r.sess, r.wav, r.started = s, s.Path, s.Started
		a.rec = s
	} else {
		dir := a.recordDir()
		if dir == "" || os.MkdirAll(dir, 0o755) != nil {
			a.say("no folder to record into: set one in Config", true)
			return
		}
		r.wav = filepath.Join(dir, record.FileName(r.started))
	}
	l, err := perflog.Create(perflog.PathFor(r.wav), r.wav, r.started)
	if err != nil {
		if r.sess != nil {
			a.rec = nil
			go r.sess.Stop()
		}
		a.say("no cast log: "+err.Error(), true)
		return
	}
	r.log = l
	a.castRun = r
	a.CastTo(l)
	a.say("casting the terminal to "+short2(r.wav, 40), false)
}

// stopCast ends the terminal cast that runs (and its audio).
func (a *App) stopCast() {
	r := a.castRun
	if r == nil {
		return
	}
	a.castRun = nil
	a.CastTo(nil)
	r.log.Add(time.Now(), "end")
	r.log.Close()
	if r.sess != nil {
		s := r.sess
		a.rec = nil
		a.say("stopping the cast...", false)
		go func() {
			err := s.Stop()
			a.recDone <- recResult{s, err}
		}()
		return
	}
	if err := record.WriteSilence(r.wav, time.Since(r.started)); err != nil {
		a.say("the cast is saved, its silent WAV is not: "+err.Error(), true)
		return
	}
	a.say("cast saved: "+short2(r.wav, 40), false)
}

// castTick writes when the audio of the cast began.
func (a *App) castTick() {
	r := a.castRun
	if r == nil || r.sess == nil || r.audioDone {
		return
	}
	if at, ok := r.sess.FirstFrame(); ok {
		r.audioDone = true
		r.log.Add(at, "audio_start", perflog.P("rate", 48000))
	} else if r.sess.Tool != "the program itself" {
		r.audioDone = true
	}
}
