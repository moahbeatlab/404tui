package core

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/moahbeatlab/404tui/record"
)

// The level meter: how loud the unit's sound is (its USB audio, listened to with the platform's recorder, or read from the end of
// the file of a recording that runs), per channel, with the highest peak held for a moment and a clip light that stays on for a few
// seconds after the sound reached full scale.

const (
	meterHold = 1500 * time.Millisecond
	clipHold  = 5 * time.Second
	// clipAt is the peak, in dB below full scale, from which a sound counts as clipping; warnAt is where the meter goes red.
	clipAt = -0.3
	warnAt = -3.0
	// meterFloor is the quietest the meter shows.
	meterFloor = -60.0
)

type levelMeter struct {
	read    time.Time
	peak    [2]float64
	hold    [2]float64
	holdAt  [2]time.Time
	clipped time.Time
	top     float64
	ok      bool
	src     *record.Meter // the program that listens to the input when nothing is recorded
	err     string
}

func newLevelMeter() *levelMeter {
	return &levelMeter{hold: [2]float64{math.Inf(-1), math.Inf(-1)}, peak: [2]float64{math.Inf(-1), math.Inf(-1)}, top: math.Inf(-1)}
}

func (l *levelMeter) clipping(now time.Time) bool {
	return l != nil && !l.clipped.IsZero() && now.Sub(l.clipped) < clipHold
}

func levelFrac(db float64) float64 {
	if math.IsInf(db, -1) || db <= meterFloor {
		return 0
	}
	return min((db-meterFloor)/(-meterFloor), 1)
}

func levelColour(db float64) color.RGBA {
	switch {
	case db >= warnAt:
		return cBad
	case db >= -12:
		return xterm(221)
	}
	return cGood
}

func dbText(db float64) string {
	if math.IsInf(db, -1) || db < -99 {
		return " -inf dB"
	}
	return fmt.Sprintf("%+5.1f dB", db)
}

// levelTick keeps the meter going while the PERF screen is in front: it listens to the input (when the platform's recorder can),
// or reads the end of the recording's file, a few times a second.
func (a *App) levelTick() {
	if a.scr != Perform {
		if a.lvl != nil && a.lvl.src != nil {
			a.lvl.src.Stop()
			a.lvl = nil
		}
		return
	}
	if a.lvl == nil {
		a.lvl = newLevelMeter()
	}
	l := a.lvl
	if a.rec == nil && l.src == nil && l.err == "" {
		if m, ok := a.plat.Recorder.(record.Metering); ok {
			src, err := m.StartMeter(a.recordDevice())
			if err != nil {
				l.err = err.Error()
			} else {
				l.src = src
			}
		}
	}
	if l.src != nil {
		select {
		case <-l.src.Done():
			l.src = nil
			l.err = "the listening program ended"
		default:
		}
	}
	now := time.Now()
	if now.Sub(l.read) < 90*time.Millisecond {
		return
	}
	l.read = now
	var peaks []float64
	switch {
	case a.rec != nil:
		lv, err := record.ReadLevel(a.rec.Path, 100*time.Millisecond)
		if err != nil || len(lv.Peak) == 0 {
			return
		}
		peaks = lv.Peak
	case l.src != nil:
		p, ok := l.src.Level()
		if !ok {
			return
		}
		peaks = p[:]
	default:
		return
	}
	l.ok = true
	for c := 0; c < 2; c++ {
		db := record.DB(peaks[min(c, len(peaks)-1)])
		l.peak[c] = db
		if db >= l.hold[c] || now.Sub(l.holdAt[c]) > meterHold {
			l.hold[c], l.holdAt[c] = db, now
		}
		if db >= clipAt {
			l.clipped = now
		}
		if db > l.top {
			l.top = db
		}
	}
}
