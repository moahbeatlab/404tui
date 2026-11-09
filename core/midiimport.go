package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// MIDI files: a part of a .mid file (a track of an instrument) becomes a pattern, and a pattern can be written as a .mid file. In
// FILES, A on a .mid file opens the form below; on the phone page the file is sent to the same form. See formats/midiimport.go for how
// the notes land on the pads.

// isMIDIFile is whether a name is a MIDI file.
func isMIDIFile(name string) bool {
	e := strings.ToLower(pathExt(name))
	return e == ".mid" || e == ".midi"
}

// importMIDIEntry is A on a MIDI file of FILES.
func (a *App) importMIDIEntry(e entry) {
	b, err := a.fileStore().ReadFile(joinPath(a.dir, e.name))
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	a.openMIDIImport(e.name, b)
}

// midiForm is what the form of an import holds.
type midiForm struct {
	name  string
	f     formats.MIDIFile
	part  int
	drums bool
	pad   int // 0..159
	root  int
	into  int // the pattern, 1..160
	bars  int // 0: as the file
	tempo bool
}

// partLabel is a line of the list of parts: "1  Bass  ch 2  124 notes  C1-G2".
func partLabel(i int, p formats.MIDIPart) string {
	lo, hi := p.Range()
	what := fmt.Sprintf("%s-%s", formats.NoteName(int(lo)), formats.NoteName(int(hi)))
	if p.Drums() {
		what = "drums"
	}
	name := p.Name
	if name == "" {
		name = "track " + fmt.Sprint(p.Track+1)
	}
	return fmt.Sprintf("%d  %s  ch %d  %d notes  %s", i+1, fit(name, 16), p.Channel+1, len(p.Notes), what)
}

// openMIDIImport reads the file and opens the form: which part, as what, onto which pad, into which pattern.
func (a *App) openMIDIImport(name string, data []byte) {
	f, err := formats.ParseMIDI(data)
	if err != nil {
		a.say(name+": "+err.Error(), true)
		return
	}
	if a.scr != Seq && a.scr != Files {
		a.go_(Seq)
	}
	st := &midiForm{name: name, f: f, pad: a.rowPad(), into: max(a.freePattern(), 1)}
	// the part with the most notes that is not the drums is the one to start with (the drums when that is all there is)
	best := -1
	for i, p := range f.Parts {
		if best < 0 || (f.Parts[best].Drums() && !p.Drums()) || (f.Parts[best].Drums() == p.Drums() && len(p.Notes) > len(f.Parts[best].Notes)) {
			best = i
		}
	}
	st.part = best
	m := &menu{title: "MIDI file " + fit(name, 28)}
	pads := make([]string, 160)
	for i := range pads {
		pads[i] = a.label(i)
	}
	parts := make([]string, len(f.Parts))
	for i, p := range f.Parts {
		parts[i] = partLabel(i, p)
	}
	notes := make([]string, 128)
	for i := range notes {
		notes[i] = fmt.Sprintf("%s (%d)", formats.NoteName(i), i)
	}
	const (
		iPart = iota
		iAs
		iPad
		iRoot
		iInto
		iBars
		iTempo
	)
	barVals := func() []string {
		v := []string{fmt.Sprintf("as the file: %d", f.MIDIBars(st.part))}
		for _, b := range []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64} {
			v = append(v, fmt.Sprint(b))
		}
		return v
	}
	bars := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 48, 64}
	choose := func() { // the part changed: what suits it
		p := f.Parts[st.part]
		st.drums, st.root, st.bars = p.Drums(), formats.SuggestRoot(p), 0
		m.items[iAs].val = b2i(st.drums)
		m.items[iRoot].val = st.root
		m.items[iBars].vals, m.items[iBars].val = barVals(), 0
	}
	m.items = []menuItem{
		{label: "Part of the file", vals: parts, val: st.part, keepOpen: true, onChange: func(v int) { st.part = v; choose() }},
		{label: "Make it", vals: []string{"a melody: keys on one pad", "drums: a pad for each drum"}, keepOpen: true, onChange: func(v int) { st.drums = v == 1 }},
		{label: "Melody onto pad", vals: pads, val: st.pad, keepOpen: true, onChange: func(v int) { st.pad = v }},
		{label: "Pad's own pitch is the note", vals: notes, keepOpen: true, onChange: func(v int) { st.root = v }},
		{label: "Into pattern", vals: pads, val: st.into - 1, keepOpen: true, onChange: func(v int) { st.into = v + 1 }},
		{label: "Length in bars", keepOpen: true, vals: []string{"-"}, onChange: func(v int) { st.bars = bars[v] }},
	}
	if f.Tempo > 0 {
		m.items = append(m.items, menuItem{label: "Tempo", vals: []string{"keep " + bpmText(a.tempo()) + " BPM", "set " + bpmText(f.Tempo) + " BPM"}, keepOpen: true,
			onChange: func(v int) { st.tempo = v == 1 }})
	}
	m.items = append(m.items, menuItem{label: "Import the pattern", run: func(int) { a.runMIDIImport(st) }})
	choose()
	a.menu = m
	a.say(fmt.Sprintf("%s: %d parts. Left and Right choose, A imports", name, len(f.Parts)), false)
}

// runMIDIImport writes the pattern the form describes.
func (a *App) runMIDIImport(st *midiForm) {
	o := formats.MIDIImport{Part: st.part, Bars: st.bars, Pad: padIndex(st.pad), Root: st.root}
	var said string
	if st.drums {
		pads, text := a.drumPads(st.f.Parts[st.part])
		if len(pads) == 0 {
			a.say("no pad has a sample: put some on the pads first", true)
			return
		}
		o.Drums, said = pads, text
	} else if st.pad < len(a.info.Pads) && !a.info.Pads[st.pad].File {
		said = ", but " + a.label(st.pad) + " has no sample"
	}
	res, err := formats.BuildFromMIDI(st.f, o)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	if err := a.lab.SetPatternBytes(st.into, res.Pattern); err != nil {
		a.say(err.Error(), true)
		return
	}
	if st.tempo && st.f.Tempo > 0 {
		bank, bpm := (st.into-1)/16, float64(int(st.f.Tempo+0.5))
		var terr error
		if a.info.UsesProjectTempo {
			terr = a.lab.SetTempo(bpm)
		} else {
			terr = a.lab.SetBankTempo(bank, bpm)
		}
		if terr != nil {
			said += " (tempo: " + terr.Error() + ")"
		}
	}
	a.rowExtra = nil
	a.openPattern(st.into)
	msg := fmt.Sprintf("%s: %d notes, %d bars", a.label(st.into-1), res.Notes, res.Bars)
	if st.drums {
		msg += ": " + said
	} else {
		msg += " on " + a.label(st.pad) + said
	}
	if res.Folded > 0 {
		msg += fmt.Sprintf("; %d notes moved by octaves to fit", res.Folded)
	}
	if res.Left > 0 {
		msg += fmt.Sprintf("; %d left out", res.Left)
	}
	if res.Chords > 0 {
		msg += fmt.Sprintf("; %d notes start together on one pad", res.Chords)
	}
	a.say(msg, false)
}

func padIndex(i int) proto.PadIndex {
	p, _ := proto.NewPadIndex(uint16(i))
	return p
}

// drumPads says which pad plays each drum note of a part: by the voice of the note (a kick, a snare...) and the names of the samples,
// the way the templates find them, and a pad that has a sample for a voice with no name match.
func (a *App) drumPads(p formats.MIDIPart) (map[uint8]proto.PadIndex, string) {
	rows := make([]app.PadRow, 0, 160)
	for i, pd := range a.info.Pads {
		if idx, ok := proto.NewPadIndex(uint16(i)); ok {
			rows = append(rows, app.PadRow{Pad: idx, HasSample: pd.File, Name: pd.Name})
		}
	}
	byLane := app.GuessRoles(rows)
	used := map[proto.PadIndex]bool{}
	for _, pad := range byLane {
		used[pad] = true
	}
	var spare []proto.PadIndex
	cands := append([]int{}, a.rows[:]...)
	for i := range a.info.Pads {
		cands = append(cands, i)
	}
	for _, i := range cands {
		if idx, ok := proto.NewPadIndex(uint16(i)); ok && a.info.Pads[i].File && !used[idx] {
			spare = append(spare, idx)
			used[idx] = true
		}
	}
	out := map[uint8]proto.PadIndex{}
	var said []string
	seen := map[formats.Lane]bool{}
	for _, note := range formats.DrumNotes(p) {
		ln := formats.DrumLane(note)
		pad, ok := byLane[ln]
		if !ok && len(spare) > 0 {
			pad, spare = spare[0], spare[1:]
			byLane[ln] = pad
			ok = true
		}
		if !ok {
			continue
		}
		out[note] = pad
		if !seen[ln] {
			seen[ln] = true
			said = append(said, fmt.Sprintf("%s %s", ln.LaneName(), a.label(int(pad.Index()))))
		}
	}
	return out, strings.Join(said, ", ")
}

// exportMIDI writes the pattern in front as a MIDI file in the folder of the exports, and says where.
func (a *App) exportMIDI() {
	name, b, err := a.patternMIDI(a.pat)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	dir := a.exportDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.say(err.Error(), true)
		return
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		a.say(err.Error(), true)
		return
	}
	a.say("written: "+p, false)
}

// patternMIDI is pattern n as a MIDI file, and the name for it.
func (a *App) patternMIDI(n int) (string, []byte, error) {
	pb, err := a.lab.PatternBytes(n)
	if err != nil {
		return "", nil, err
	}
	b, err := formats.PatternToMIDI(pb, a.tempoOf((n-1)/16))
	if err != nil {
		return "", nil, err
	}
	return "pattern-" + a.label(n-1) + ".mid", b, nil
}
