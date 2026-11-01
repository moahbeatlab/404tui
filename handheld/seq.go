package handheld

import (
	"encoding/json"
	"fmt"
	"image/color"
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

func (a *App) rowPad() int { return a.bank*16 + a.row }

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
	switch k {
	case Up:
		a.row = (a.row + 15) % 16
	case Down:
		a.row = (a.row + 1) % 16
	case Left:
		a.step = max(a.step-1, 0)
	case Right:
		a.step = min(a.step+1, a.steps()-1)
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
	case X:
		a.bank = (a.bank + 1) % 10
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

func (a *App) seqMenu() {
	pad := a.rowPad()
	m := &menu{title: fmt.Sprintf("pattern %s  row %s", a.label(a.pat-1), a.label(pad))}
	names := make([]string, len(rowPresets))
	for i, p := range rowPresets {
		names[i] = p.name
	}
	m.items = append(m.items, menuItem{label: "Fill the row with", vals: names, run: func(v int) { a.setRow(pad, rowPresets[v].mask) }})
	hits := make([]string, 16)
	for i := range hits {
		hits[i] = fmt.Sprintf("%d per bar", i+1)
	}
	m.items = append(m.items, menuItem{label: "Euclidean row", vals: hits, val: 4, run: func(v int) { a.setRow(pad, euclid(v+1)) }})
	m.items = append(m.items, menuItem{label: "Clear the row", run: func(int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad})
		a.lab.PatternOp(a.pat, "clearPad", args)
		a.refresh()
		a.replay()
	}})
	sw := []string{"54%", "58%", "62%", "66%", "70%"}
	m.items = append(m.items, menuItem{label: "Swing", vals: sw, val: 2, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: -1, Grid: stepTicks, Amount: float64([]int{54, 58, 62, 66, 70}[v])})
		a.opSay("swing", args)
	}})
	m.items = append(m.items, menuItem{label: "Humanize", vals: []string{"light", "medium", "loose"}, val: 1, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: -1, Timing: uint32([]int{6, 12, 24}[v]), Velocity: []int{6, 12, 24}[v], Seed: int64(a.step + 1)})
		a.opSay("humanize", args)
	}})
	m.items = append(m.items, menuItem{label: "Quantize to 1/16", run: func(int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: -1, Grid: stepTicks, Strength: 1})
		a.opSay("quantize", args)
	}})
	m.items = append(m.items, menuItem{label: "Shift the row", vals: []string{"1 step <", "1 step >"}, val: 1, run: func(v int) {
		args, _ := json.Marshal(lab.PatternArgs{Pad: pad, Grid: stepTicks, Steps: []int{-1, 1}[v]})
		a.opSay("shift", args)
	}})
	m.items = append(m.items, menuItem{label: "Copy to the next pattern", run: func(int) {
		to := a.pat%160 + 1
		if err := a.lab.CopyPattern(a.pat, to); err != nil {
			a.say(err.Error(), true)
			return
		}
		a.pat = to
		a.refresh()
		a.say("copied to "+a.label(to-1), false)
	}})
	m.items = append(m.items, menuItem{label: "Delete the pattern", run: func(int) {
		a.lab.DeletePattern(a.pat)
		a.refresh()
		a.replay()
	}})
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
	bpm := a.info.Tempo
	if bpm < 30 || bpm > 300 {
		bpm = 120
	}
	perTick := 60 / bpm / formats.PPQ * formats.SampleRate
	n := int(float64(d.Length) * perTick)
	mix := make([]int32, n*2)
	for _, nt := range d.Notes {
		id, ok := a.padSample(nt.Pad)
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
	bpm := a.info.Tempo
	if bpm < 30 || bpm > 300 {
		bpm = 120
	}
	stepDur := time.Duration(float64(time.Minute) / bpm / 4)
	return int(a.clock().Sub(a.seqStart)/stepDur) % a.steps()
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

func (a *App) drawSeq(c *canvas) {
	d, ok := a.doc()
	bpm := a.info.Tempo
	title := fmt.Sprintf("PATTERN %s", a.label(a.pat-1))
	c.text(margin, 38, title, cAmber, true)
	sub := fmt.Sprintf("%d bars   %.1f BPM", d.Bars, bpm)
	if !ok {
		sub = "empty: A puts the first note"
	}
	c.text(margin+textW(title)+16, 38, sub, cDim, false)
	page := a.step / 16
	for b := 0; b < d.Bars; b++ {
		col := cLine
		if b == page {
			col = cAmber
		}
		c.rect(W-margin-(d.Bars-b)*22, 42, 18, 8, col)
	}
	const x0, y0, cw, rh = 128, 62, 29, 20
	ph := a.playStep()
	for r := 0; r < 16; r++ {
		pad := a.bank*16 + r
		y := y0 + r*rh
		has := a.info.Pads[pad].File
		fg := cDim
		if has {
			fg = cText
		}
		if r == a.row {
			c.rect(margin-4, y-1, x0-margin-2, rh-2, cAmberD)
			fg = cWhite
		}
		c.text(margin, y+1, fmt.Sprintf("%-4s%s", a.label(pad), short(a.info.Pads[pad].Name, 7)), fg, r == a.row)
		for s := 0; s < 16; s++ {
			st := page*16 + s
			x := x0 + s*cw + (s/4)*6
			bg := cPanel
			if (s/4)%2 == 1 {
				bg = cPanel2
			}
			if ph == st {
				bg = cLine
			}
			c.rect(x, y, cw-3, rh-3, bg)
			if n, ok := noteAt(d, pad, st); ok {
				c.rect(x, y, cw-3, rh-3, lerp(cAmberD, cAmber, float64(n.Vel)/127))
				if ph == st {
					c.rect(x, y, cw-3, rh-3, cGood)
				}
			}
			if r == a.row && st == a.step {
				c.box(x-1, y-1, cw-1, rh-1, 2, cWhite)
			}
		}
	}
	c.text(margin, 388, fmt.Sprintf("step %d.%d   bank %c", a.step/16+1, a.step%16+1, 'A'+rune(a.bank)), cDim, false)
	if a.seqOn {
		c.text(W-margin-textW("PLAYING"), 388, "PLAYING", cGood, true)
	}
}
