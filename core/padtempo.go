package core

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// The tempo a pad's sample says it is at (its BPM, what the unit's BPM sync stretches it by) is checked against the tempo heard in
// the sample, and against one its name states: the ones that differ are listed with both, and nothing changes until one is chosen.

// padTempo is what is known of the tempo of one pad.
type padTempo struct {
	pad   int
	unit  float64 // the pad's BPM setting, 0: none
	heard float64 // the tempo heard in the sample, 0: no steady beat (or too short)
	conf  float64
	named float64 // the tempo the name states, 0: none
	short bool    // too short to hear a tempo in
}

// want is the tempo the check proposes: the name's, else the heard one; 0 when there is none.
func (t padTempo) want() float64 {
	if t.named > 0 {
		return t.named
	}
	return t.heard
}

// sameTempo: within half a BPM, or the same at half or double (a 70 BPM loop heard at 140 is at 70).
func sameTempo(a, b float64) bool {
	for _, f := range []float64{1, 2, 0.5} {
		if math.Abs(a-b*f) < 0.5 {
			return true
		}
	}
	return false
}

// agrees says whether the pad's BPM is what the check would set.
func (t padTempo) agrees() bool {
	w := t.want()
	return w <= 0 || (t.unit > 0 && sameTempo(t.unit, w))
}

// minTempoFrames: a sample shorter than two bars at 120 has too few beats to hear a tempo in.
const minTempoFrames = 4 * formats.SampleRate

// padTempoOf is what the project says of a pad's tempo: its BPM, its name, whether it is too short to hear one in.
func (a *App) padTempoOf(n int) padTempo {
	t := padTempo{pad: n, named: bpmOfName(a.info.Pads[n].Name), short: a.info.Pads[n].Frames < minTempoFrames}
	if v, err := a.lab.PadParam(n, 0x6F); err == nil && v > 0 {
		t.unit = float64(v) / 100
	}
	return t
}

// tempoText is a tempo to two decimals, the way the unit shows a pad's BPM; "-" for none.
func tempoText(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

// tempoRow is one pad of the check: its name and its BPM, then what was heard and what its name says.
func (a *App) tempoRow(t padTempo) string {
	l := fmt.Sprintf("%s %-12s %6s ", a.label(t.pad), fitRunes(a.info.Pads[t.pad].Name, 12), tempoText(t.unit))
	switch {
	case t.short:
		l += " too short"
	case t.heard > 0:
		l += fmt.Sprintf(" heard %s %.0f%%", tempoText(t.heard), t.conf*100)
	default:
		l += " no steady beat"
	}
	if t.named > 0 {
		l += " name " + bpmText(t.named)
	}
	return l
}

// changeRow is a pad whose BPM differs: its BPM, the one it gets, and why.
func (a *App) changeRow(t padTempo) string {
	why := fmt.Sprintf("heard %.0f%%", t.conf*100)
	if t.named > 0 {
		why = "its name"
	}
	return fmt.Sprintf("%s %-12s %6s -> %-6s %s", a.label(t.pad), fitRunes(a.info.Pads[t.pad].Name, 12), tempoText(t.unit), tempoText(t.want()), why)
}

// The states of a pad in the check.
const (
	tWait  = iota // its sample is not taken yet
	tFetch        // its sample is coming from the unit (a project edited live)
	tQueue        // its sample is with the worker, waiting
	tHear         // the worker is hearing its tempo
	tDone
)

// tempoJob hears the tempos in the background: the main loop hands the worker each pad's sample file (asking the unit for the ones
// still on it), the worker reads it and hears it. The dialog shows how far it is and, at the end, what it found.
type tempoJob struct {
	scope   string
	m       *menu // the progress dialog: while it is the one in front the job goes on, closed it stops
	in      chan tempoWork
	sent    int
	mu      sync.Mutex
	rows    []padTempo
	state   []int
	secs    []float64 // seconds of audio of each pad
	heard   float64   // seconds of audio heard
	all     float64   // seconds of audio to hear
	started time.Time
	stop    atomic.Bool
}

type tempoWork struct {
	i    int
	data []byte
}

// checkTempos hears the tempo of every pad of the bank with a sample (the marked pads, when some are marked), with its progress
// on screen, then shows the ones whose BPM differs, to be set one by one or all together.
func (a *App) checkTempos() {
	var pads []int
	scope := fmt.Sprintf("bank %c", 'A'+a.bank)
	if len(a.marks) > 0 {
		pads = a.targetsFor(a.padN())
		scope = fmt.Sprintf("the %d marked pads", len(pads))
	} else {
		for n := a.bank * 16; n < a.bank*16+16; n++ {
			if a.info.Pads[n].File {
				pads = append(pads, n)
			}
		}
	}
	if len(pads) == 0 {
		a.say("no pad with a sample in "+scope, false)
		return
	}
	j := &tempoJob{scope: scope, started: time.Now(), in: make(chan tempoWork, len(pads))}
	for _, n := range pads {
		t := a.padTempoOf(n)
		st, secs := tWait, float64(a.info.Pads[n].Frames)/formats.SampleRate
		if t.short {
			st, secs = tDone, 0
		}
		j.rows = append(j.rows, t)
		j.state = append(j.state, st)
		j.secs = append(j.secs, secs)
		j.all += secs
	}
	j.m = &menu{title: "checking the BPM of " + scope}
	j.m.onClose = func() { j.stop.Store(true) }
	j.m.footH = 4
	j.m.foot = func(g Gfx, x, y, wc int) { a.drawTempoProgress(j, g, x, y, wc) }
	a.tempoJob = j
	a.menu = j.m
	go j.work()
	a.tempoFeed(j)
	a.tempoProgressRows(j)
}

// work reads and hears the samples handed to it, one at a time.
func (j *tempoJob) work() {
	for w := range j.in {
		if j.stop.Load() {
			continue
		}
		j.set(w.i, tHear)
		s, err := formats.ParseSMP(w.data)
		var tp formats.Tempo
		if err == nil {
			if s.Frames() < minTempoFrames {
				j.mu.Lock()
				j.rows[w.i].short = true
				j.mu.Unlock()
			} else {
				tp = formats.DetectTempo(s)
			}
		}
		j.mu.Lock()
		j.rows[w.i].heard, j.rows[w.i].conf = tp.BPM, tp.Confidence
		j.state[w.i] = tDone
		j.heard += j.secs[w.i]
		j.mu.Unlock()
	}
}

func (j *tempoJob) set(i, st int) {
	j.mu.Lock()
	j.state[i] = st
	j.mu.Unlock()
}

// tempoFeed hands the worker the samples that are here, and asks the unit for the ones still on it (first among what it fetches).
func (a *App) tempoFeed(j *tempoJob) {
	for i := range j.rows {
		j.mu.Lock()
		st, n := j.state[i], j.rows[i].pad
		j.mu.Unlock()
		if st != tWait && st != tFetch {
			continue
		}
		if a.live != nil && a.lab.IsRemote(n) {
			a.wantPad(n)
			j.set(i, tFetch)
			continue
		}
		data, ok := a.lab.PadData(n)
		if !ok {
			j.mu.Lock()
			j.state[i] = tDone
			j.heard += j.secs[i]
			j.mu.Unlock()
			continue
		}
		j.set(i, tQueue)
		j.in <- tempoWork{i, data}
		j.sent++
	}
}

// tempoTick follows the job: samples that arrived to the worker, the rows into its dialog and, at the end, the review.
func (a *App) tempoTick() {
	j := a.tempoJob
	if j == nil {
		return
	}
	if a.menu != j.m { // closed (B): the job stops after the pad it is on
		j.stop.Store(true)
		close(j.in)
		a.tempoJob = nil
		return
	}
	a.tempoFeed(j)
	a.tempoProgressRows(j)
	j.mu.Lock()
	done := true
	for _, st := range j.state {
		done = done && st == tDone
	}
	rows := append([]padTempo(nil), j.rows...)
	j.mu.Unlock()
	if done {
		close(j.in)
		a.tempoJob = nil
		a.tempoReview(j.scope, rows)
	}
}

// tempoProgressRows lists the pads in the dialog: the ones heard with what was heard, then what is happening to the others.
func (a *App) tempoProgressRows(j *tempoJob) {
	j.mu.Lock()
	defer j.mu.Unlock()
	items := []menuItem{{label: "Stop (B)", run: func(int) { j.stop.Store(true) }}}
	for i, t := range j.rows {
		l := a.tempoRow(t)
		if j.state[i] != tDone {
			l = fmt.Sprintf("%s %-12s %6s  %s", a.label(t.pad), fitRunes(a.info.Pads[t.pad].Name, 12), tempoText(t.unit), map[int]string{
				tWait:  "...",
				tFetch: fmt.Sprintf("from the unit (%.1f MB)...", j.secs[i]*formats.SampleRate*4/1e6),
				tQueue: "waiting",
				tHear:  fmt.Sprintf("hearing %.0f s...", j.secs[i]),
			}[j.state[i]])
		}
		items = append(items, menuItem{label: l, run: func(int) {}})
	}
	cur := j.m.cur
	j.m.items = items
	j.m.cur = min(cur, len(items)-1)
}

// drawTempoProgress is the bar under the dialog: how much of the audio is heard, what is going on, and how long it has taken.
func (a *App) drawTempoProgress(j *tempoJob, g Gfx, x, y, wc int) {
	j.mu.Lock()
	heard, all, n := j.heard, j.all, len(j.rows)
	done, now := 0, ""
	for i, st := range j.state {
		switch st {
		case tDone:
			done++
		case tHear:
			now = "hearing " + a.label(j.rows[i].pad) + " " + a.info.Pads[j.rows[i].pad].Name
		case tFetch:
			if now == "" {
				now = "from the unit: " + a.label(j.rows[i].pad) + " " + a.info.Pads[j.rows[i].pad].Name
			}
		}
	}
	j.mu.Unlock()
	g.HRule(x+1, y, wc-2, cLine)
	g.Bar(x+2, y+1, wc-4, heard/math.Max(all, 0.001), cAmber, cPanel)
	took := time.Since(j.started).Seconds()
	left := ""
	if heard > 0 && heard < all {
		left = fmt.Sprintf(", about %.0f s left", took/heard*(all-heard))
	}
	g.Text(x+2, y+2, fitRunes(fmt.Sprintf("%d of %d pads  %s", done, n, now), wc-4), cText, false)
	g.Text(x+2, y+3, fitRunes(fmt.Sprintf("%.0f of %.0f s of audio in %.0f s%s", heard, all, took, left), wc-4), cDim, false)
}

// tempoReview shows what the check found: the pads whose BPM differs first, each to be set, or all of them.
func (a *App) tempoReview(scope string, rows []padTempo) {
	var differ, agree []padTempo
	for _, t := range rows {
		if t.agrees() {
			agree = append(agree, t)
		} else {
			differ = append(differ, t)
		}
	}
	m := &menu{title: fmt.Sprintf("tempos of %s: %d of %d differ", scope, len(differ), len(rows))}
	add := func(l string, run func(int)) { m.items = append(m.items, menuItem{label: l, run: run}) }
	if len(differ) > 0 {
		add(fmt.Sprintf("Set the %d that differ (the name's tempo, else the one heard)", len(differ)), func(int) { a.setTempos(differ) })
		add("No, leave them", func(int) {})
		m.head("DIFFER: A SETS THAT ONE")
		for _, t := range differ {
			t := t
			add(a.changeRow(t), func(int) { a.setTempos([]padTempo{t}) })
		}
	} else {
		add("Every BPM is right: close", func(int) {})
	}
	if len(agree) > 0 {
		m.head("RIGHT, OR NOTHING TO GO BY")
		for _, t := range agree {
			m.items = append(m.items, menuItem{label: a.tempoRow(t), run: func(int) {}})
		}
	}
	a.menu = m
}

// setTempos gives pads the tempo the check found for them (undone with the rest).
func (a *App) setTempos(ts []padTempo) {
	pre := a.lab.Snapshot()
	n := 0
	for _, t := range ts {
		if w := t.want(); w > 0 && a.lab.SetPadParam(t.pad, 0x6F, int32(math.Round(w*100))) == nil {
			n++
		}
	}
	a.commitUndo(pre, A, 0)
	a.refresh()
	if len(ts) == 1 && n == 1 {
		a.say(fmt.Sprintf("%s: BPM %s (was %s)", a.label(ts[0].pad), tempoText(ts[0].want()), tempoText(ts[0].unit)), false)
		return
	}
	a.say(fmt.Sprintf("the BPM of %d pads set", n), false)
}
