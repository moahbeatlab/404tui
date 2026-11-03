package handheld

import (
	"encoding/json"
	"fmt"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Sample tools (from the PADS menu): what is done with the sample of a pad. Chords and arps make a new sample from it (the
// engine of the computer app: the sample pitched once for each note, mixed into a chord or laid out in time); the keyboard
// puts pitched copies on a run of pads, so that the pads can be played, and sequenced, like keys.

var (
	arpGateNames = []string{"25%", "50%", "75%", "100%", "ring"}
	arpBars      = []int{1, 2, 4, 8}
	arpOcts      = []int{1, 2, 3}
)

func arpNames[T any](list []T, name func(T) string) []string {
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = name(v)
	}
	return out
}

// arpState is the settings of the sample arp menu, kept between visits.
type arpState struct {
	formats.Arp
	chord int // the chord type, an index of ArpChords
	init  bool
}

func (a *App) arpDefaults() {
	if a.arp.init {
		return
	}
	a.arp = arpState{Arp: formats.DefaultArp(), init: true}
	a.arp.Arp.Dir = formats.ArpUp
	a.arp.chord = 0
}

// openArpMenu is the chord and arp maker for the sample of pad n.
func (a *App) openArpMenu(n int) {
	a.arpDefaults()
	st := &a.arp
	m := &menu{title: "chords & arps  " + a.label(n)}
	remake := func() { st.Notes = formats.ChordNotes(st.Root, -1, st.chord, 0) }
	set := func(label string, vals []string, cur int, change func(v int)) {
		m.items = append(m.items, menuItem{label: label, vals: vals, val: cur, keepOpen: true, onChange: change})
	}
	m.head("THE NOTES")
	set("Root", formats.PitchNames, st.Root, func(v int) { st.Root = v; remake() })
	set("Chord", arpNames(formats.ArpChords, func(c struct {
		Name string
		Iv   []int
	}) string {
		return c.Name
	}), st.chord, func(v int) { st.chord = v; remake() })
	set("Sample is in", formats.PitchNames, st.Key, func(v int) { st.Key = v })
	set("Octaves", names(arpOcts, func(v int) string { return fmt.Sprint(v) }), near(arpOcts, st.Octaves), func(v int) { st.Octaves = arpOcts[v] })
	m.head("HOW IT PLAYS")
	set("Pattern", formats.ArpDirs, st.Dir, func(v int) { st.Dir = v })
	set("Rate", arpNames(formats.ArpRates, func(r formats.ArpRate) string { return r.Name }), st.Rate, func(v int) { st.Rate = v })
	set("Gate", arpGateNames, st.Gate, func(v int) { st.Gate = v })
	set("Swing", arpNames(formats.ArpSwings, func(s struct {
		Name string
		Part float64
	}) string {
		return s.Name
	}), st.Swing, func(v int) { st.Swing = v })
	set("Length", names(arpBars, func(v int) string { return fmt.Sprintf("%d bar", v) }), near(arpBars, st.Bars), func(v int) { st.Bars = arpBars[v] })
	m.head("MAKE IT")
	m.items = append(m.items,
		menuItem{label: "Play it", keepOpen: true, run: func(int) { a.playArp(n) }},
		menuItem{label: "Put it on a free pad", run: func(int) { a.arpToPad(n) }},
	)
	remake()
	a.menu = m
}

// renderArp makes the arp from pad n's sample as set, into a sample the caller frees.
func (a *App) renderArp(n int) (formats.Sample, int, error) {
	id, ok := a.padSample(n)
	if !ok {
		return formats.Sample{}, 0, fmt.Errorf("pad %s has no sample", a.label(n))
	}
	st := a.arp
	st.Arp.Notes = formats.ChordNotes(st.Root, -1, st.chord, 0)
	st.BPM = a.tempoOf(n / 16)
	if st.Octaves < 1 {
		st.Octaves = 1
	}
	if st.Bars < 1 {
		st.Bars = 1
	}
	b, _ := json.Marshal(st.Arp)
	mb, err := a.lab.RenderArp(id, b)
	if err != nil {
		return formats.Sample{}, 0, err
	}
	var meta lab.Meta
	json.Unmarshal(mb, &meta)
	s, err := a.lab.Sample(meta.ID)
	return s, meta.ID, err
}

func (a *App) playArp(n int) {
	s, id, err := a.renderArp(n)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.lab.Free(id)
	a.play(s)
	a.say(fmt.Sprintf("%s %s, %.1f s", formats.PitchNames[a.arp.Root], formats.ArpChords[a.arp.chord].Name, float64(s.Frames())/formats.SampleRate), false)
}

func (a *App) arpToPad(n int) {
	a.pickFreePad("Put the arp on a pad", n+1, func(to int) {
		_, id, err := a.renderArp(n)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		name := fmt.Sprintf("%s_%s%s", fit(a.info.Pads[n].Name, 8), formats.PitchNames[a.arp.Root], formats.ArpChords[a.arp.chord].Name)
		err = a.lab.Assign(to, id, fit(name, 15))
		a.lab.Free(id)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, to)
		a.refresh()
		a.say(fmt.Sprintf("the arp is on %s", a.label(to)), false)
	})
}

// ---- the keyboard

var keyboardSizes = []int{4, 5, 7, 8, 12, 16}

// openKeyboardMenu spreads pitched copies of pad n's sample over a run of free pads.
func (a *App) openKeyboardMenu(n int) {
	scales := append([]string{"chromatic"}, arpNames(formats.ArpScales, func(s struct {
		Name string
		Iv   []int
	}) string {
		return s.Name
	})...)
	size, scale := 4, 0 // 8 keys, chromatic
	m := &menu{title: "keyboard from " + a.label(n)}
	m.items = append(m.items,
		menuItem{label: "Keys", vals: names(keyboardSizes, func(v int) string { return fmt.Sprintf("%d pads", v) }), val: size, keepOpen: true, onChange: func(v int) { size = v }},
		menuItem{label: "Scale", vals: scales, val: scale, keepOpen: true, onChange: func(v int) { scale = v }},
		menuItem{label: "Put them on free pads", run: func(int) { a.keyboard(n, keyboardSizes[size], scale-1) }},
	)
	a.menu = m
}

// keySemis is the pitch of key i in semitones from the sample, for a scale (-1: chromatic).
func keySemis(i, scale int) int {
	if scale < 0 || scale >= len(formats.ArpScales) {
		return i
	}
	iv := formats.ArpScales[scale].Iv
	return iv[i%len(iv)] + 12*(i/len(iv))
}

func (a *App) keyboard(n, keys, scale int) {
	id, ok := a.padSample(n)
	if !ok {
		a.say("this pad has no sample", true)
		return
	}
	first := a.freeRun(keys, n+1)
	if first < 0 {
		a.say(fmt.Sprintf("no room: %d empty pads in a row are needed", keys), true)
		return
	}
	base := fit(a.info.Pads[n].Name, 9)
	for i := 0; i < keys; i++ {
		semi := keySemis(i, scale)
		pid, err := a.lab.Pitched(id, semi)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		err = a.lab.Assign(first+i, pid, fmt.Sprintf("%s_%+d", base, semi))
		a.lab.Free(pid)
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		delete(a.pcm, first+i)
	}
	a.refresh()
	a.bank, a.cur = first/16, first%16
	a.say(fmt.Sprintf("%d keys on %s...%s", keys, a.label(first), a.label(first+keys-1)), false)
}
