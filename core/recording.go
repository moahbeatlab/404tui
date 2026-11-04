package core

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/record"
)

// Recording a set: the unit's audio (its USB audio interface) is recorded to a WAV file in the folder CONFIG names, from any screen,
// with the recorder the platform has.

func (a *App) recording() bool { return a.rec != nil }

// ToggleRecording starts a recording, or ends the one that runs.
func (a *App) ToggleRecording() {
	if a.rec != nil {
		s := a.rec
		a.rec = nil
		a.stopEventLog()
		a.say("stopping the recording...", false)
		go func() {
			err := s.Stop()
			a.recDone <- recResult{s, err}
		}()
		return
	}
	if a.plat.Recorder == nil {
		a.say("this build cannot record: no recorder here", true)
		return
	}
	s, err := a.plat.Recorder.Start(a.recordDir(), a.recordDevice())
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.rec = s
	a.startEventLog(s)
	a.say("recording to "+short2(s.Path, 40)+" ("+s.Tool+")", false)
}

// CuePattern asks which pattern plays now, while a recording runs: the unit does not tell when its pattern changes, and the event log
// keeps the answer with the time, for the replay.
func (a *App) CuePattern() {
	if a.rec == nil {
		a.say("no recording runs: the pattern is noted only in a recording", true)
		return
	}
	a.pickPattern("the pattern that plays now", max(a.perfNow, 1), func(n int) {
		a.perfNow = n
		a.say("now: pattern "+a.label(n-1)+" (noted in the recording)", false)
	})
}

type recResult struct {
	s   *record.Session
	err error
}

// recordingTick says what became of a recording that ended, or one that stopped by itself.
func (a *App) recordingTick() {
	select {
	case r := <-a.recDone:
		if r.err != nil {
			a.say("the recording ended with an error: "+r.err.Error(), true)
			return
		}
		a.say(fmt.Sprintf("recorded %s (%.1f MB)", short2(r.s.Path, 40), float64(r.s.Size())/1e6), false)
	default:
	}
	if s := a.rec; s != nil {
		select {
		case <-s.Done():
			a.rec = nil
			a.stopEventLog()
			err := s.Err()
			if err == nil {
				err = fmt.Errorf("the recorder stopped")
			}
			a.say("the recording stopped: "+err.Error(), true)
		default:
		}
	}
}

// recText is the time of the running recording, for the title bar.
func (a *App) recText() string {
	if a.rec == nil {
		return ""
	}
	d := time.Since(a.rec.Started)
	return fmt.Sprintf("REC %02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
