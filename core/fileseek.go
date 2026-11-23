package core

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// Skipping through a file on FILES: L2 and R2 (, and . on a keyboard) go back and forth while it plays, to hear a whole song
// before it is put on a pad.

// fileSkip is how far one press of L2 or R2 goes.
const fileSkip = 10 * time.Second

// filePlay is the file of FILES that plays: which, from where, since when.
type filePlay struct {
	id     int       // the decoded file
	frames int       // its length
	from   int       // the frame it was started at
	at     time.Time // when
}

// filePos is where the file that plays is (in frames), false when none plays (or it has ended).
func (a *App) filePos() (int, bool) {
	f := a.fplay
	if f == nil || a.Audio == nil {
		return 0, false
	}
	p := f.from + int(a.clock().Sub(f.at).Seconds()*48000)
	if p >= f.frames {
		return 0, false
	}
	return p, true
}

// playFileFrom plays the file from a frame on, and keeps where.
func (a *App) playFileFrom(id int, s formats.Sample, from int) {
	ch := max(s.Channels, 1)
	from = max(0, min(from, s.Frames()-1))
	a.fplay = &filePlay{id: id, frames: s.Frames(), from: from, at: a.clock()}
	a.play(formats.Sample{Channels: s.Channels, Samples: s.Samples[from*ch:]})
}

// fileSkipKey is L2 (back) and R2 (forward) on FILES: the file that plays goes on from 10 s earlier or later.
func (a *App) fileSkipKey(fwd bool) {
	at, ok := a.filePos()
	if !ok {
		play := "X"
		if Terminal {
			play = "Space"
		}
		a.say("nothing plays: "+play+" plays the file, then "+fileSkipKeys()+" skip 10 s", false)
		return
	}
	s, err := a.lab.Sample(a.fplay.id)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	d := int(fileSkip.Seconds() * 48000)
	if !fwd {
		d = -d
	}
	to := at + d
	if to >= s.Frames() {
		a.say("the end of the file", false)
		return
	}
	a.playFileFrom(a.fplay.id, s, max(to, 0))
	a.say(fmt.Sprintf("%s / %s", clockTime(a.fplay.from), clockTime(s.Frames())), false)
}

// fileSkipKeys names the keys that skip.
func fileSkipKeys() string {
	if Terminal {
		return ", ."
	}
	return "L2 R2"
}

// clockTime is a number of frames as minutes and seconds: 3:07.
func clockTime(frames int) string {
	s := frames / 48000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
