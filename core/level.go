package core

import (
	"fmt"
	"image/color"
	"math"
	"strings"
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
	spec    []float64     // the spectrum of the latest reading (record.SpectrumBands values), when the input is listened to
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
	if a.scr != Perform && !(Terminal && a.link != nil) { // a computer shows the meter in its panel for as long as a unit is plugged in
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
		l.spec, _ = l.src.Spectrum()
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

// drawMeter is the level of the unit's sound in the panel of a computer: two bars (left and right, the highest peak held for a
// moment), their level, and CLIP when the sound reached full scale lately. It takes two rows from y.
func (a *App) drawMeter(g Gfx, x, y, w int) int {
	l := a.lvl
	if l == nil || !l.ok {
		msg := "level: listening to the unit..."
		if l != nil && l.err != "" {
			msg = "level: needs ffmpeg or sox and the unit's audio"
		}
		g.Text(x, y, fit(msg, w), cDim, false)
		return 3
	}
	now := time.Now()
	bw := max(w-2-10, 6)
	for c, name := range []string{"L", "R"} {
		g.Text(x, y+c, name, cDim, true)
		g.Bar(x+2, y+c, bw, levelFrac(l.hold[c]), levelColour(l.hold[c]), cPanel2)
		db := strings.TrimSpace(dbText(l.hold[c]))
		g.Text(x+w-len(db), y+c, db, cDim, false)
	}
	if l.clipping(now) { // the clip light is under the bars, over the row that keeps them apart from the tempo
		g.Fill(x+w-6, y+2, 6, 1, cBad)
		g.Text(x+w-5, y+2, "CLIP", cBg, true)
	}
	if len(l.spec) > 0 && Rows >= 36 { // the spectrum under the bars, when the window is tall enough
		const rows = 3
		n := w * g.BarsPerCell()
		h := make([]float64, n)
		for i := range h {
			h[i] = l.spec[min(i*len(l.spec)/n, len(l.spec)-1)]
		}
		g.Bars(x, y+3, w, rows, h, false, cAmber)
		return 3 + rows + 1
	}
	return 3
}
