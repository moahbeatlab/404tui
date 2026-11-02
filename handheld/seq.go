package handheld

import (
	"encoding/json"
	"fmt"
	"image/color"
	"sort"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// stepTicks is one sixteenth note.
const stepTicks = formats.PPQ / 4

var velSteps = []int{127, 100, 70, 40}

// doc reads the pattern in front; ok is false when it is empty (it does not exist yet).
func (a *App) doc() (lab.PatternDoc, bool) {
	var d lab.PatternDoc
	b, err := a.lab.Pattern(a.pat)
	if err != nil || json.Unmarshal(b, &d) != nil {
		return lab.PatternDoc{Bars: 4, Length: 4 * 4 * formats.PPQ, Beats: 4}, false
	}
	return d, true
}

func (a *App) steps() int {
	d, _ := a.doc()
	return d.Bars * d.Beats * 4
}

// rowPad is the pad of the row under the cursor.
func (a *App) rowPad() int { return a.rows[a.row] }

// autoRows picks the 8 pads of the columns, unless the user chose some: first the pad that was in front on PADS,
// then pads that have notes in the pattern, then pads with samples, then the pads that follow.
func (a *App) autoRows() {
	if a.rowsOwn && a.rowsSet {
		return
	}
	var picks []int
	seen := map[int]bool{}
	add := func(p int) {
		if p >= 0 && p < 160 && !seen[p] && len(picks) < len(a.rows) {
			seen[p] = true
			picks = append(picks, p)
		}
	}
	d, _ := a.doc()
	withNotes := map[int]bool{}
	for _, n := range d.Notes {
		withNotes[n.Pad] = true
	}
	for _, p := range a.marks { // the marked pads are the keys: they are the columns
		add(p)
	}
	add(a.padN())
	for p := 0; p < 160; p++ {
		if withNotes[p] {
			add(p)
		}
	}
	for off := 0; off < 160; off++ {
		if p := (a.padN() + off) % 160; a.info.Pads[p].File {
			add(p)
		}
	}
	for p := a.padN() + 1; len(picks) < len(a.rows) && p < 320; p++ {
		add(p % 160)
	}
	sort.Ints(picks) // in pad order; the cursor starts on the pad that was in front
	copy(a.rows[:], picks)
	a.rowsSet = true
	for i, p := range picks {
		if p == a.padN() {
			a.row = i
		}
	}
}

// noteAt finds the note of a pad that starts in a step.
func noteAt(d lab.PatternDoc, pad, step int) (lab.Note, bool) {
	for _, n := range d.Notes {
		if n.Pad == pad && int(n.Tick) >= step*stepTicks && int(n.Tick) < (step+1)*stepTicks {
			return n, true
		}
	}
	return lab.Note{}, false
}

func (a *App) seqKey(k Key, repeat int) {
	jump := 1 + repeat/8 // a held key runs faster
	switch k {
	case Left: // the steps run across the screen
		a.step = max(a.step-jump, 0)
	case Right:
		a.step = min(a.step+jump, a.steps()-1)
	case Up: // the pads run down it
		a.row = max(a.row-1, 0)
	case Down:
		a.row = min(a.row+1, len(a.rows)-1)
	case A:
		a.toggle(a.rowPad(), a.step)
	case X:
		a.togglePlay()
	case B:
		if a.seqOn {
			a.stop()
			a.seqOn = false
			return
		}
		a.seqMenu()
	}
}

func (a *App) seqShifted(k Key) {
	switch k {
	case Up:
		a.setPattern(a.pat + 1)
	case Down:
		a.setPattern(a.pat - 1)
	case B:
		a.openPicker()
	case X:
		// the column's pad: the next pad that has a sample
		for off := 1; off < 160; off++ {
			p := (a.rowPad() + off) % 160
			if a.info.Pads[p].File {
				a.rows[a.row] = p
				a.rowsOwn = true
				a.say(fmt.Sprintf("row %d is pad %s", a.row+1, a.label(p)), false)
				return
			}
		}
		a.say("no other pad has a sample", true)
	case A:
		d, ok := a.doc()
		if !ok {
			return
		}
		if n, ok := noteAt(d, a.rowPad(), a.step); ok {
			next := velSteps[0]
			for i, v := range velSteps {
				if n.Vel >= v-5 {
					next = velSteps[(i+1)%len(velSteps)]
					break
				}
			}
			args, _ := json.Marshal(lab.PatternArgs{Notes: []lab.PadAt{{Pad: n.Pad, Tick: n.Tick}}, Vel: next})
			if err := a.lab.PatternOp(a.pat, "edit", args); err != nil {
				a.say(err.Error(), true)
				return
			}
			a.say(fmt.Sprintf("velocity %d", next), false)
			a.replay()
		}
	}
}

func (a *App) setPattern(n int) {
	a.pat = (n+159)%160 + 1
	a.step = min(a.step, a.steps()-1)
	a.autoRows()
	if a.seqOn {
		a.replay()
	}
}

func (a *App) toggle(pad, step int) {
	d, ok := a.doc()
	if !ok {
		if err := a.lab.NewPattern(a.pat, 4); err != nil {
			a.say(err.Error(), true)
			return
		}
		d, _ = a.doc()
	}
	if n, ok := noteAt(d, pad, step); ok {
		args, _ := json.Marshal(lab.PatternArgs{Notes: []lab.PadAt{{Pad: n.Pad, Tick: n.Tick}}, Delete: true})
		if err := a.lab.PatternOp(a.pat, "edit", args); err != nil {
			a.say(err.Error(), true)
		}
	} else {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad, Tick: uint32(step * stepTicks), Vel: 100, Len: stepTicks})
		if err := a.lab.PatternOp(a.pat, "insert", args); err != nil {
			a.say(err.Error(), true)
		}
	}
	a.refresh()
	a.replay()
}

// setRow puts notes on the steps of a pad that are in the mask of each bar, taking the others off.
func (a *App) setRow(pad int, mask []int) {
	d, ok := a.doc()
	if !ok {
		if err := a.lab.NewPattern(a.pat, 4); err != nil {
			a.say(err.Error(), true)
			return
		}
		d, _ = a.doc()
	}
	args, _ := json.Marshal(lab.PatternArgs{Pad: pad})
	a.lab.PatternOp(a.pat, "clearPad", args)
	for bar := 0; bar < d.Bars; bar++ {
		for _, s := range mask {
			args, _ := json.Marshal(lab.PatternArgs{Pad: pad, Tick: uint32((bar*16 + s) * stepTicks), Vel: 100, Len: stepTicks})
			a.lab.PatternOp(a.pat, "insert", args)
		}
	}
	a.refresh()
	a.replay()
}

func euclid(hits int) []int {
	var m []int
	for i := 0; i < hits; i++ {
		m = append(m, i*16/hits)
	}
	return m
}

var rowPresets = []struct {
	name string
	mask []int
}{
	{"every beat", []int{0, 4, 8, 12}},
	{"backbeat 2 4", []int{4, 12}},
	{"off-beats", []int{2, 6, 10, 14}},
	{"eighths", []int{0, 2, 4, 6, 8, 10, 12, 14}},
	{"sixteenths", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}},
	{"boom-bap kick", []int{0, 7, 10}},
	{"3+3+2", []int{0, 3, 6, 8, 11, 14}},
}

// seqMenu is the pattern tools: the pattern as a whole, its tempo, its timing, and the notes of the column in front.
func (a *App) seqMenu() {
	pad := a.rowPad()
	m := &menu{title: fmt.Sprintf("pattern %s  row %s", a.label(a.pat-1), a.label(pad))}
	run := func(label string, f func()) {
		m.items = append(m.items, menuItem{label: label, run: func(int) { f() }})
	}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: change})
	}

	m.head("THE PATTERN")
	run("Choose a pattern (bank, then pattern)...", a.openPicker)
	run("New pattern from a template...", a.templatesMenu)
	run("Copy to the next pattern", func() {
		to := a.pat%160 + 1
		if err := a.lab.CopyPattern(a.pat, to); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.pat = to
		a.refresh()
		a.autoRows()
		a.say("copied to "+a.label(to-1), false)
	})
	run("Delete the pattern", func() {
		a.lab.DeletePattern(a.pat)
		a.refresh()
		a.replay()
	})

	m.head("TEMPO")
	bpms := make([]string, 0, 181)
	for v := 60; v <= 240; v++ {
		bpms = append(bpms, fmt.Sprintf("%d BPM", v))
	}
	bank := (a.pat - 1) / 16
	what := fmt.Sprintf("Tempo of bank %c", 'A'+bank)
	if a.info.UsesProjectTempo {
		what = "Tempo of the project"
	}
	set(what, bpms, min(max(int(a.tempo()+0.5)-60, 0), len(bpms)-1), func(v int) {
		var err error
		if a.info.UsesProjectTempo {
			err = a.lab.SetTempo(float64(v + 60))
		} else {
			err = a.lab.SetBankTempo(bank, float64(v+60))
		}
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		a.refresh()
		a.replay()
	})
	sel := 0
	if a.info.UsesProjectTempo {
		sel = 1
	}
	set("Tempo select", []string{"each bank's own", "the project's"}, sel, func(v int) {
		a.lab.SetUsesProjectTempo(v == 1)
		a.refresh()
		a.replay()
		a.say(fmt.Sprintf("tempo: %s, %s BPM", map[bool]string{true: "the project's", false: "bank " + string(rune('A'+bank))}[v == 1], bpmText(a.tempo())), false)
	})

	m.head("TIMING AND LENGTH (every pad)")
	m.items = append(m.items, menuItem{label: "Swing", vals: []string{"54%", "58%", "62%", "66%", "70%"}, val: 2, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: -1, Grid: stepTicks, Amount: float64([]int{54, 58, 62, 66, 70}[v])})
		a.opSay("swing", args)
	}})
	m.items = append(m.items, menuItem{label: "Humanize", vals: []string{"light", "medium", "loose"}, val: 1, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: -1, Timing: uint32([]int{6, 12, 24}[v]), Velocity: []int{6, 12, 24}[v], Seed: int64(a.step + 1)})
		a.opSay("humanize", args)
	}})
	run("Quantize to 1/16", func() {
		args, _ := json.Marshal(lab.PatternArgs{Pad: -1, Grid: stepTicks, Strength: 1})
		a.opSay("quantize", args)
	})
	run("Reverse the pattern", func() { a.opSay("reverse", []byte(`{"pad":-1}`)) })
	run("Double its length", func() { a.opSay("double", []byte(`{"pad":-1}`)); a.step = min(a.step, a.steps()-1) })
	run("Halve its length", func() { a.opSay("halve", []byte(`{"pad":-1}`)); a.step = min(a.step, a.steps()-1) })

	m.head(fmt.Sprintf("THE COLUMN (%s)", a.label(pad)))
	names := make([]string, len(rowPresets))
	for i, p := range rowPresets {
		names[i] = p.name
	}
	m.items = append(m.items, menuItem{label: "Fill with", vals: names, run: func(v int) { a.setRow(pad, rowPresets[v].mask) }})
	hits := make([]string, 16)
	for i := range hits {
		hits[i] = fmt.Sprintf("%d per bar", i+1)
	}
	m.items = append(m.items, menuItem{label: "Euclidean", vals: hits, val: 4, run: func(v int) { a.setRow(pad, euclid(v+1)) }})
	m.items = append(m.items, menuItem{label: "Shift", vals: []string{"1 step <", "1 step >"}, val: 1, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad, Grid: stepTicks, Steps: []int{-1, 1}[v]})
		a.opSay("shift", args)
	}})
	run("Clear", func() {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad})
		a.lab.PatternOp(a.pat, "clearPad", args)
		a.refresh()
		a.replay()
	})
	padNames := make([]string, 160)
	for i := range padNames {
		padNames[i] = a.label(i)
	}
	m.items = append(m.items, menuItem{label: "Pad of this row", vals: padNames, val: pad, run: func(v int) {
		a.rows[a.row] = v
		a.rowsOwn = true
		a.say(fmt.Sprintf("row %d is pad %s", a.row+1, a.label(v)), false)
	}})

	m.head("GENERATE")
	run("Arp over the columns...", a.openArpPrintMenu)
	a.menu = m
}

func (a *App) opSay(op string, args []byte) {
	if err := a.lab.PatternOp(a.pat, op, args); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.refresh()
	a.say(op, false)
	a.replay()
}

// render mixes the pattern into one stereo sample: every note starts the sample of its pad at its tick.
func (a *App) render() (formats.Sample, bool) {
	d, ok := a.doc()
	if !ok {
		return formats.Sample{}, false
	}
	bpm := a.tempo()
	perTick := 60 / bpm / formats.PPQ * formats.SampleRate
	n := int(float64(d.Length) * perTick)
	mix := make([]int32, n*2)
	for _, nt := range d.Notes {
		id, ok := a.padSample(nt.Pad)
		if over, auditioned := a.aud[nt.Pad]; auditioned { // a file heard in place of the pad's sample
			id, ok = over, true
		}
		if !ok {
			continue
		}
		s, err := a.lab.Sample(id)
		if err != nil {
			continue
		}
		at := int(float64(nt.Tick) * perTick)
		g := int32(nt.Vel)
		for f := 0; f < s.Frames() && at+f < n; f++ {
			l := int32(s.Samples[f*s.Channels])
			r := int32(s.Samples[f*s.Channels+s.Channels-1])
			mix[(at+f)*2] += l * g / 127
			mix[(at+f)*2+1] += r * g / 127
		}
	}
	out := make([]int16, len(mix))
	for i, v := range mix {
		out[i] = int16(max(-32767, min(32767, v)))
	}
	return formats.Sample{Channels: 2, Samples: out}, true
}

type looper interface{ Loop(formats.Sample) error }
type replacer interface {
	ReplaceLoop(formats.Sample) error
}

func (a *App) togglePlay() {
	if a.seqOn {
		a.stop()
		a.seqOn = false
		return
	}
	a.startLoop()
}

func (a *App) startLoop() {
	s, ok := a.render()
	if !ok {
		a.say("the pattern is empty: A puts a note", true)
		return
	}
	lp, isLoop := a.Audio.(looper)
	if a.Audio == nil || !isLoop {
		a.say("no audio output", true)
		return
	}
	if err := lp.Loop(s); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.seqOn, a.seqStart = true, a.clock()
}

// replay puts an edit into the loop that plays, without a gap.
func (a *App) replay() {
	if !a.seqOn {
		return
	}
	s, ok := a.render()
	if !ok {
		return
	}
	if r, ok := a.Audio.(replacer); ok {
		r.ReplaceLoop(s)
	}
}

func (a *App) clock() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// playStep is the step the loop is at, -1 when it is not playing.
func (a *App) playStep() int {
	if !a.seqOn {
		return -1
	}
	bpm := a.tempo()
	stepDur := time.Duration(float64(time.Minute) / bpm / 4)
	return int(a.clock().Sub(a.seqStart)/stepDur) % a.steps()
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// drawSeq is the step grid, as on the unit: a row for each pad (up to 16), the 16 steps of one bar across, a lit block
// where a pad plays, as tall as its velocity. L1/R1 (or the cursor) turn the bar.
func (a *App) drawSeq(c *canvas) {
	d, ok := a.doc()
	title := fmt.Sprintf("PATTERN %s", a.label(a.pat-1))
	c.text(margin, 36, title, cAmber, true)
	sub := fmt.Sprintf("%d bars", d.Bars)
	if !ok {
		sub = "empty: A puts the first note"
	}
	c.text(margin+textW(title)+16, 36, sub, cDim, false)

	const lx, x0, cw, gap = margin, 58, 24, 4 // the pad labels, the first step, a step's width, the gap between beats
	steps := a.steps()
	page := a.step / 16
	bars := (steps + 15) / 16
	ph := a.playStep()
	const ry = 74
	rh := (panelBot - 6 - ry) / len(a.rows)
	stepX := func(s int) int { return x0 + s*cw + (s/4)*gap }

	// the step numbers
	for s := 0; s < 16 && page*16+s < steps; s++ {
		st := page*16 + s
		col := cDim
		if s%4 == 0 {
			col = cText
		}
		if ph == st {
			col = cGood
		}
		if st == a.step {
			col = cAmber
		}
		c.text(stepX(s)+(cw-textW(fmt.Sprint(s+1)))/2, 54, fmt.Sprint(s+1), col, s%4 == 0)
	}
	// a band for the step the cursor is on and for the one that plays, over the rows
	for s := 0; s < 16 && page*16+s < steps; s++ {
		st := page*16 + s
		switch {
		case ph == st:
			c.rect(stepX(s)-1, ry-2, cw, rh*len(a.rows)+2, cLine)
		case st == a.step:
			c.rect(stepX(s)-1, ry-2, cw, rh*len(a.rows)+2, cPanel2)
		}
	}
	for i, pad := range a.rows {
		y := ry + i*rh
		has := a.info.Pads[pad].File
		fg := cDim
		if has {
			fg = cText
		}
		if i == a.row {
			c.rect(lx-4, y-1, x0-lx+0, rh-1, cAmberD)
			fg = cWhite
		}
		c.text(lx, y+(rh-CellH)/2, a.label(pad), fg, i == a.row)
		for s := 0; s < 16 && page*16+s < steps; s++ {
			st := page*16 + s
			x := stepX(s)
			bg := cPanel
			if (s/4)%2 == 1 {
				bg = cPanel2
			}
			if ph == st {
				bg = cLine
			}
			c.rect(x, y, cw-2, rh-2, bg)
			if n, ok := noteAt(d, pad, st); ok {
				h := max((rh-2)*n.Vel/127, 4) // as tall as it is loud
				col := cAmber
				if ph == st {
					col = cGood
				}
				c.rect(x, y+(rh-2)-h, cw-2, h, col)
			}
			if i == a.row && st == a.step {
				c.box(x-1, y-1, cw, rh, 2, cWhite)
			}
		}
	}
	// which bar this is, and where the loop is when it plays
	bx := contentW - 4 - bars*22
	for b := 0; b < bars; b++ {
		col := cLine
		if b == page {
			col = cAmber
		}
		if ph >= 0 && ph/16 == b {
			c.rect(bx+b*22, 44, 18, 4, cGood)
		}
		c.rect(bx+b*22, 36, 18, 6, col)
	}
	c.text(bx-8-textW(fmt.Sprintf("bar %d/%d", page+1, bars)), 36, fmt.Sprintf("bar %d/%d", page+1, bars), cText, true)
}
