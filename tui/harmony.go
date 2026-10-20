package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// harmonyFields are the settings the dialog lists, top to bottom.
const (
	hfKey = iota
	hfRoot
	hfScale
	hfChord
	hfDir
	hfRate
	hfGate
	hfOctaves
	hfShift
	hfSwing
	hfBars
	hfBPM
	hfCount
)

// harmonyBars are the lengths an arp can have, in bars.
var harmonyBars = []int{1, 2, 4}

// harmony is the dialog that makes a chord or an arp out of a pad's sample and puts it on a pad: the sample is pitched once per
// note (see formats.RenderArp), on this computer.
type harmony struct {
	row    app.PadRow
	sample *formats.Sample
	// arp is what is set; scale is an index of formats.ArpScales, or -1 for chord types, chord the chord type and deg the
	// degree of the scale (0 the whole scale).
	arp          formats.Arp
	scale, chord int
	deg          int
	// custom is set when notes were switched on or off by hand, after the last chord was chosen.
	custom bool
	field  int
}

// harmonyMsg is a pad's sample, read for the dialog.
type harmonyMsg struct {
	pad    proto.PadIndex
	sample formats.Sample
	err    error
}

// harmonySettings is what the dialog was left at, to come back to the next time.
type harmonySettings struct {
	arp          formats.Arp
	scale, chord int
	deg, field   int
	custom       bool
}

// openHarmony opens the dialog on a pad, reading its sample from the unit unless that was done before.
func (m Model) openHarmony(row app.PadRow) (tea.Model, tea.Cmd) {
	if !row.HasSample {
		m.message = fmt.Sprintf("%s is empty; a assigns a file to it", row.Pad)
		return m, nil
	}
	m.stopListening()
	h := &harmony{row: row, scale: -1, arp: formats.DefaultArp()}
	if s := m.harmonyMem; s != nil {
		h.arp, h.scale, h.chord, h.deg, h.field, h.custom = s.arp, s.scale, s.chord, s.deg, s.field, s.custom
		h.arp.Notes = append([]int(nil), s.arp.Notes...)
	} else if bpm := m.snap.Banks[row.Pad.Bank()].Tempo; bpm >= app.MinBankBPM {
		h.arp.BPM = bpm
	}
	m.harmony = h
	key := factKey{row.Pad, row.Size, row.Name}
	if sample, ok := m.padAudio.get(key); ok {
		h.sample = &sample
		return m, nil
	}
	unit, ops, log, cache := m.unit, m.ops, m.log, m.padAudio
	return m, func() tea.Msg {
		ops.Lock()
		sample, err := app.PadAudio(unit, row)
		ops.Unlock()
		if err != nil {
			log.Warn("could not read the pad to make chords of it", "pad", row.Pad.String(), "err", err)
			return harmonyMsg{pad: row.Pad, err: err}
		}
		cache.put(key, sample)
		return harmonyMsg{pad: row.Pad, sample: sample}
	}
}

// harmonyLoaded takes in the sample the dialog was waiting for.
func (m Model) harmonyLoaded(msg harmonyMsg) (tea.Model, tea.Cmd) {
	h := m.harmony
	if h == nil || h.row.Pad != msg.pad {
		return m, nil
	}
	if msg.err != nil {
		m.harmony = nil
		m.message = fmt.Sprintf("could not read %s: %v", msg.pad, msg.err)
		return m, nil
	}
	h.sample = &msg.sample
	return m, nil
}

// derive sets the notes from the root, the scale and the chord, the way the choices say.
func (h *harmony) derive() {
	h.arp.Notes = formats.ChordNotes(h.arp.Root, h.scale, h.chord, h.deg)
	h.custom = false
}

func wrap(v, delta, n int) int { return ((v+delta)%n + n) % n }

// adjust changes the field in front by delta steps.
func (h *harmony) adjust(delta int) {
	a := &h.arp
	switch h.field {
	case hfKey:
		a.Key = wrap(a.Key, delta, 12)
	case hfRoot:
		a.Root = wrap(a.Root, delta, 12)
		h.derive()
	case hfScale:
		// -1 is "off": the chord types.
		h.scale = wrap(h.scale+1, delta, len(formats.ArpScales)+1) - 1
		h.deg = 0
		h.derive()
	case hfChord:
		if h.scale < 0 {
			h.chord = wrap(h.chord, delta, len(formats.ArpChords))
		} else {
			h.deg = wrap(h.deg, delta, 15)
		}
		h.derive()
	case hfDir:
		a.Dir = wrap(a.Dir, delta, len(formats.ArpDirs))
	case hfRate:
		a.Rate = wrap(a.Rate, delta, len(formats.ArpRates))
	case hfGate:
		a.Gate = wrap(a.Gate, delta, len(formats.ArpGates))
	case hfOctaves:
		a.Octaves = min(max(a.Octaves+delta, 1), 3)
	case hfShift:
		a.Shift = min(max(a.Shift+delta, -1), 1)
	case hfSwing:
		a.Swing = wrap(a.Swing, delta, len(formats.ArpSwings))
	case hfBars:
		at := 0
		for i, b := range harmonyBars {
			if b == a.Bars {
				at = i
			}
		}
		a.Bars = harmonyBars[min(max(at+delta, 0), len(harmonyBars)-1)]
	case hfBPM:
		a.BPM = min(max(a.BPM+float64(delta), app.MinBankBPM), app.MaxBankBPM)
	}
}

// toggle switches a note on or off, by hand.
func (h *harmony) toggle(pc int) {
	for i, n := range h.arp.Notes {
		if n == pc {
			h.arp.Notes = append(h.arp.Notes[:i:i], h.arp.Notes[i+1:]...)
			h.custom = true
			return
		}
	}
	h.arp.Notes = append(h.arp.Notes, pc)
	h.custom = true
}

func (m *Model) rememberHarmony() {
	h := m.harmony
	m.harmonyMem = &harmonySettings{arp: h.arp, scale: h.scale, chord: h.chord, deg: h.deg, field: h.field, custom: h.custom}
}

func (m Model) harmonyKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	h := m.harmony
	// Caps Lock must not silence the keyboard.
	name := strings.ToLower(key.String())
	switch name {
	case "esc", "q":
		m.rememberHarmony()
		m.harmony = nil
		m.stopListening()
		return m, nil
	case "up", "shift+tab":
		h.field = wrap(h.field, -1, hfCount)
	case "down", "tab":
		h.field = wrap(h.field, 1, hfCount)
	case "left":
		h.adjust(-1)
	case "right":
		h.adjust(1)
	case " ":
		return m.harmonyPreview()
	case "enter":
		return m.harmonyPut()
	}
	// The home row and the one above it are piano keys, as in the chromatic dialog: the first twelve switch the twelve notes.
	if at := strings.Index(chromaticKeys, name); len(name) == 1 && at >= 0 && at < 12 {
		h.toggle(at)
	}
	return m, nil
}

// harmonyPreview plays the chord or the arp on this computer, or stops what plays.
func (m Model) harmonyPreview() (tea.Model, tea.Cmd) {
	h := m.harmony
	if h.sample == nil {
		return m, nil
	}
	if m.player == nil {
		m.message = "no audio output to play on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	if len(h.arp.Notes) == 0 {
		m.message = "no notes: the keys a w s e d f t g y h u j switch them on"
		return m, nil
	}
	if m.listening == "chords" {
		m.stopListening()
		return m, nil
	}
	id := m.listenGen.Add(1)
	m.listening = "chords"
	sample, a, player, gen, log := *h.sample, h.arp, m.player, m.listenGen, m.log
	return m, func() tea.Msg {
		out := formats.RenderArp(sample, a)
		if gen.Load() != id {
			return listenMsg{path: "chords", stale: true}
		}
		err := player.Play(out)
		log.Debug("playing chords made from a pad", "frames", out.Frames(), "notes", len(a.Notes), "err", err)
		return listenMsg{path: "chords", err: err}
	}
}

// harmonyName is the name the new pad gets: the pad's own name and what was made of it.
func (h *harmony) name() string {
	what := "custom"
	switch {
	case h.custom || len(h.arp.Notes) == 0:
	case h.scale < 0:
		what = formats.PitchNames[h.arp.Root] + formats.ArpChords[h.chord].Name
	default:
		what = formats.PitchNames[h.arp.Root] + formats.ArpScales[h.scale].Name
	}
	suffix := " " + what + " " + formats.ArpDirs[h.arp.Dir]
	base := []rune(strings.TrimSpace(h.row.Name))
	return strings.TrimSpace(string(base[:min(len(base), max(app.MaxPadName-len([]rune(suffix)), 1))])) + suffix
}

// harmonyPut renders the chord or the arp and asks which pad it goes on.
func (m Model) harmonyPut() (tea.Model, tea.Cmd) {
	h := m.harmony
	if h.sample == nil {
		return m, nil
	}
	if len(h.arp.Notes) == 0 {
		m.message = "no notes: the keys a w s e d f t g y h u j switch them on"
		return m, nil
	}
	out := formats.RenderArp(*h.sample, h.arp)
	if out.Frames() == 0 {
		m.message = "nothing to put on a pad: the sample is too short"
		return m, nil
	}
	name := h.name()
	note := fmt.Sprintf("%s, %d notes, %.1f s", formats.ArpDirs[h.arp.Dir], len(h.arp.Offsets()), float64(out.Frames())/formats.SampleRate)
	m.rememberHarmony()
	m.harmony = nil
	m.stopListening()
	m.picker = &picker{bank: m.bank, pad: m.nextFree(m.bank), bounces: []formats.Sample{out}, bounceNames: []string{name}, bounceWhat: "harmony", note: note}
	return m, nil
}

// harmonyBox draws the dialog.
func (m Model) harmonyBox() string {
	st, h := m.st, m.harmony
	a := h.arp
	title := st.bold.Render(fmt.Sprintf("Chords & arps  ·  %s  %s", h.row.Pad, h.row.Name))
	if h.sample == nil {
		return st.modal.Render(title + "\n\n" + st.dim.Render("reading the sample from the unit…"))
	}
	scaleName, chordName, chordLabel := "OFF", formats.ArpChords[h.chord].Name, "CHORD"
	if h.scale >= 0 {
		scaleName, chordLabel = formats.ArpScales[h.scale].Name, "ON SCALE"
		chordName = "SCALE"
		if h.deg > 0 {
			chordName = []string{"I", "II", "III", "IV", "V", "VI", "VII"}[(h.deg-1)%7]
			if h.deg > 7 {
				chordName += "7"
			}
		}
	}
	gate := "OPEN"
	if g := formats.ArpGates[a.Gate]; g > 0 {
		gate = fmt.Sprintf("%d%%", g)
	}
	rows := [hfCount][2]string{
		hfKey:     {"SAMPLE KEY", formats.PitchNames[a.Key]},
		hfRoot:    {"ROOT", formats.PitchNames[a.Root]},
		hfScale:   {"SCALE", scaleName},
		hfChord:   {chordLabel, chordName},
		hfDir:     {"PATTERN", formats.ArpDirs[a.Dir] + "  " + st.faint.Render(formats.ArpDirNotes[a.Dir])},
		hfRate:    {"RATE", formats.ArpRates[a.Rate].Name},
		hfGate:    {"GATE", gate},
		hfOctaves: {"OCTAVES", fmt.Sprint(a.Octaves)},
		hfShift:   {"SHIFT", fmt.Sprintf("%+d oct", a.Shift)},
		hfSwing:   {"SWING", formats.ArpSwings[a.Swing].Name},
		hfBars:    {"BARS", fmt.Sprint(a.Bars)},
		hfBPM:     {"BPM", fmt.Sprintf("%g", a.BPM)},
	}
	lines := []string{title, ""}
	for i, r := range rows {
		cursor, label := "  ", st.dim.Render(fmt.Sprintf("%-11s", r[0]))
		value := st.text.Render(r[1])
		if i == h.field {
			cursor, value = st.accent.Render("› "), st.accent.Render(r[1])
			if i == hfDir {
				value = st.accent.Render(formats.ArpDirs[a.Dir]) + "  " + st.faint.Render(formats.ArpDirNotes[a.Dir])
			}
		}
		lines = append(lines, cursor+label+" "+value)
	}
	// The twelve notes, lit when they are in the chord, with the keys that switch them.
	var names, keys strings.Builder
	for pc := 0; pc < 12; pc++ {
		on := false
		for _, n := range a.Notes {
			on = on || n == pc
		}
		cell := fmt.Sprintf("%-3s", formats.PitchNames[pc])
		if on {
			names.WriteString(st.hot.Render(cell))
		} else {
			names.WriteString(st.faint.Render(cell))
		}
		keys.WriteString(st.faint.Render(fmt.Sprintf("%-3s", string(chromaticKeys[pc]))))
	}
	what := "no notes"
	if len(a.Notes) > 0 {
		what = fmt.Sprintf("%d notes", len(a.Offsets()))
		if h.custom {
			what += ", by hand"
		}
	}
	length := float64(a.Steps()) * 60 / a.BPM / float64(formats.ArpRates[a.Rate].PerBeat)
	if a.Dir == formats.ArpChord {
		length = float64(h.sample.Frames()) / formats.SampleRate
	}
	lines = append(lines, "", "  "+names.String(), "  "+keys.String(), "",
		st.dim.Render(fmt.Sprintf("%s  ·  about %.1f s", what, length)), "",
		st.dim.Render("↑ ↓ field  ·  ← → change  ·  Space play  ·  Enter put on a pad  ·  Esc close"),
		st.dim.Render("a w s e d f t g y h u j  switch the notes C … B on and off, like piano keys"),
		st.faint.Render("pitched once per note, as the unit's chromatic mode does it: higher is shorter"))
	return st.modal.Render(strings.Join(lines, "\n"))
}
