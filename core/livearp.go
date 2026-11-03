package core

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/moahbeatlab/404tui/formats"
)

// The live arp: mark pads (Y+A on PADS), and R2 loops an arp over them: the pads are the keys, as when holding pads on the
// unit. The settings (type, rate, gate) are the ones of the pattern arp and can be changed while it plays; the same marked
// pads can be written into the pattern.

const maxMarks = 16

// markedKeys are the marked pads that still have a sample: in the order they were marked for ORD, else in pad order.
func (a *App) markedKeys() []int {
	var keys []int
	for _, p := range a.marks {
		if a.info.Pads[p].File {
			keys = append(keys, p)
		}
	}
	if a.arpp.typ != 5 {
		sort.Ints(keys)
	}
	return keys
}

// toggleMark marks or unmarks the pad under the cursor.
func (a *App) toggleMark() {
	n := a.padN()
	for i, p := range a.marks {
		if p == n {
			a.marks = append(a.marks[:i], a.marks[i+1:]...)
			a.say(fmt.Sprintf("%s unmarked, %d marked", a.label(n), len(a.marks)), false)
			a.liveArpUpdate()
			return
		}
	}
	if !a.info.Pads[n].File {
		a.say("only a pad with a sample can be a key", true)
		return
	}
	if len(a.marks) >= maxMarks {
		a.say("16 pads are marked: unmark one first", true)
		return
	}
	a.marks = append(a.marks, n)
	a.say(fmt.Sprintf("%s marked, %d marked: A edits them all, R2 arps them", a.label(n), len(a.marks)), false)
	a.liveArpUpdate()
}

func (a *App) clearMarks() {
	a.marks = nil
	if a.arpOn {
		a.stop()
	}
	a.say("no pad is marked", false)
}

// renderMarkedArp mixes the arp over keys into a stereo loop: a step of the rate each, a note cut by the gate.
func (a *App) renderMarkedArp(keys []int) (formats.Sample, error) {
	samples := make([]formats.Sample, len(keys))
	for i, p := range keys {
		id, ok := a.padSample(p)
		if !ok {
			return formats.Sample{}, fmt.Errorf("pad %s has no sample", a.label(p))
		}
		s, err := a.lab.Sample(id)
		if err != nil {
			return formats.Sample{}, err
		}
		samples[i] = s
	}
	return a.mixArp(samples, a.tempoOf(keys[0]/16))
}

// mixArp lays the samples (the keys, low to high) out in time as the arp settings say, at a tempo.
func (a *App) mixArp(samples []formats.Sample, bpm float64) (formats.Sample, error) {
	st := a.arpp
	per := formats.ArpRates[st.rate].PerBeat
	stepF := formats.SampleRate * 60 / bpm / float64(per)
	steps := printBars[near(printBars, st.bars)] * 4 * per
	total := int(math.Round(float64(steps) * stepF))
	if total < 2 {
		return formats.Sample{}, fmt.Errorf("the arp is too short")
	}
	mix := make([]int32, total*2)
	gate := printGates[st.gate]
	gain := int32(printVels[st.vel])
	for i, hits := range arpSequence(len(samples), st.typ, steps, rand.New(rand.NewSource(7))) {
		start := int(math.Round(float64(i) * stepF))
		length := max(int(stepF*float64(gate)/100), 32)
		for _, k := range hits {
			s := samples[k]
			ch := max(s.Channels, 1)
			n := min(s.Frames(), length)
			fade := min(n, int(0.008*formats.SampleRate))
			for f := 0; f < n && start+f < total; f++ {
				g := gain
				if n < s.Frames() && f >= n-fade { // a note cut by the gate fades out, so it does not click
					g = g * int32(n-f) / int32(fade)
				}
				mix[(start+f)*2] += int32(s.Samples[f*ch]) * g / 127
				mix[(start+f)*2+1] += int32(s.Samples[f*ch+ch-1]) * g / 127
			}
		}
	}
	out := make([]int16, len(mix))
	for i, v := range mix {
		out[i] = int16(max(-32767, min(32767, v)))
	}
	return formats.Sample{Channels: 2, Samples: out}, nil
}

// toggleLiveArp starts the arp over the marked pads, or stops it.
func (a *App) toggleLiveArp() {
	if a.arpOn {
		a.stop()
		a.say("arp stopped", false)
		return
	}
	keys := a.markedKeys()
	if len(keys) < 2 {
		a.say("mark 2 or more pads first (Y+A on a pad)", true)
		return
	}
	lp, ok := a.Audio.(looper)
	if a.Audio == nil || !ok {
		a.say("no audio output", true)
		return
	}
	a.arpPrintDefaults()
	s, err := a.renderMarkedArp(keys)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.stop()
	if err := lp.Loop(s); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.arpOn = true
	a.say(fmt.Sprintf("arp %s over %d pads: R2 stops", arpTypes[a.arpp.typ], len(keys)), false)
}

// liveArpUpdate puts a changed setting or key into the loop that plays, without a gap.
func (a *App) liveArpUpdate() {
	if !a.arpOn {
		return
	}
	keys := a.markedKeys()
	r, ok := a.Audio.(replacer)
	if len(keys) < 2 || !ok {
		return
	}
	if s, err := a.renderMarkedArp(keys); err == nil {
		r.ReplaceLoop(s)
	}
}

// openMarkedArpMenu is the settings of the arp over the marked pads.
func (a *App) openMarkedArpMenu() {
	a.arpPrintDefaults()
	st := &a.arpp
	m := &menu{title: fmt.Sprintf("arp over %d marked pads", len(a.markedKeys()))}
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: func(v int) { change(v); a.liveArpUpdate() }})
	}
	m.head("HOW IT PLAYS")
	set("Type", arpTypes, st.typ, func(v int) { st.typ = v; a.say(arpTypeNotes[v], false) })
	set("Rate", arpNames(formats.ArpRates, func(r formats.ArpRate) string { return r.Name }), st.rate, func(v int) { st.rate = v })
	set("Gate", names(printGates, func(v int) string { return fmt.Sprintf("%d%%", v) }), st.gate, func(v int) { st.gate = v })
	set("Velocity", names(printVels, func(v int) string { return fmt.Sprint(v) }), st.vel, func(v int) { st.vel = v })
	set("Length", names(printBars, func(v int) string { return fmt.Sprintf("%d bar", v) }), near(printBars, st.bars), func(v int) { st.bars = printBars[v] })
	m.head("MAKE IT")
	m.items = append(m.items,
		menuItem{label: "Play it / stop it", keepOpen: true, run: func(int) { a.toggleLiveArp() }},
		menuItem{label: "Bounce it to a free pad", run: func(int) { a.bounceMarkedArp() }},
		menuItem{label: "Write it into the pattern", run: func(int) {
			keys := a.markedKeys()
			a.go_(Seq)
			a.printArpKeys(keys)
		}},
		menuItem{label: "Clear the marks", run: func(int) { a.clearMarks() }},
	)
	a.menu = m
}

// bounceMarkedArp puts the arp over the marked pads on the next free pad, as a sample (the loop as it plays).
func (a *App) bounceMarkedArp() {
	keys := a.markedKeys()
	if len(keys) < 2 {
		a.say("mark 2 or more pads first (Y+A on a pad)", true)
		return
	}
	a.pickFreePad("Put the arp on a pad", keys[len(keys)-1]+1, func(to int) {
		s, err := a.renderMarkedArp(keys)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		b := s.WAV()
		mb, err := a.lab.Decode(b, "wav")
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		var meta struct{ ID int }
		if json.Unmarshal(mb, &meta) != nil {
			a.say("cannot keep the arp", true)
			return
		}
		err = a.lab.Assign(to, meta.ID, fmt.Sprintf("arp_%s_%d", arpTypes[a.arpp.typ], len(keys)))
		a.lab.Free(meta.ID)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, to)
		a.refresh()
		a.say(fmt.Sprintf("the arp is on %s", a.label(to)), false)
	})
}

// markBank marks every pad with a sample in the bank in front (the marks are replaced), for bulk editing and the arp.
func (a *App) markBank() {
	a.marks = nil
	for i := 0; i < 16; i++ {
		if p := a.bank*16 + i; a.info.Pads[p].File {
			a.marks = append(a.marks, p)
		}
	}
	a.say(fmt.Sprintf("%d pads of bank %c marked: A edits them all", len(a.marks), 'A'+rune(a.bank)), false)
}
