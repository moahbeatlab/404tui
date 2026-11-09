package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
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

// midiItem is a file that waits for its turn: by its path (on FILES) or with its bytes (sent from the page).
type midiItem struct {
	name string
	path string
	data []byte
}

// midiBatch is a run of files that make one pattern: a song in several files, each of them on a pad of its own. The first goes into
// the pattern that was chosen, the next ones are added to it, and the pattern gets as long as the longest of them.
type midiBatch struct {
	queue []midiItem // the files still to come
	into  int        // the pattern the files go into (0 until the first one is in)
	used  map[int]bool
	last  int // the pad of the last melody
	done  int
	total int
}

// importMIDIEntry is A on a MIDI file of FILES: with MIDI files marked, all of them (in the order of their names) one after the other into
// one pattern; else this one.
func (a *App) importMIDIEntry(e entry) {
	var items []midiItem
	if len(a.fmarks) > 0 {
		var paths []string
		for p := range a.fmarks {
			if isMIDIFile(p) {
				paths = append(paths, p)
			}
		}
		sort.Slice(paths, func(i, j int) bool { return app.NaturalLess(strings.ToLower(paths[i]), strings.ToLower(paths[j])) })
		for _, p := range paths {
			items = append(items, midiItem{name: baseName(p), path: p})
		}
	}
	if len(items) == 0 {
		items = []midiItem{{name: e.name, path: joinPath(a.dir, e.name)}}
	}
	a.startMIDIBatch(items, len(items))
}

// startMIDIBatch starts a run of files and opens the form of the first.
func (a *App) startMIDIBatch(items []midiItem, total int) {
	a.midiRun = &midiBatch{queue: items, total: max(total, len(items)), used: map[int]bool{}}
	a.nextMIDI()
}

// nextMIDI opens the form of the next file of the run; with none left the run is over.
func (a *App) nextMIDI() {
	b := a.midiRun
	for b != nil && len(b.queue) > 0 {
		it := b.queue[0]
		b.queue = b.queue[1:]
		data := it.data
		if data == nil {
			var err error
			if data, err = a.fileStore().ReadFile(it.path); err != nil {
				a.say(it.name+": "+err.Error(), true)
				continue
			}
		}
		if a.openMIDIImport(it.name, data) {
			return
		}
	}
	if b != nil && b.done > 0 {
		a.say(fmt.Sprintf("%d of %d files are in %s", b.done, b.total, a.label(b.into-1)), false)
	}
	a.midiRun, a.menu = nil, nil
}

// midiForm is what the form of an import holds.
type midiForm struct {
	name    string
	f       formats.MIDIFile
	part    int
	drums   bool
	pad     int // 0..159
	root    int
	into    int // the pattern, 1..160
	mode    int // 0 replaces the pattern, 1 adds to it (a shorter part goes round again), 2 adds to it (a shorter part plays once)
	bars    int // 0: as the file
	tempo   bool
	playing bool // the form is playing the pattern it would make
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

// nextMIDIPad is the pad for the next melody of a run: the first one after the last that has a sample and is not used yet.
func (a *App) nextMIDIPad(b *midiBatch) int {
	for k := 1; k < 160; k++ {
		i := (b.last + k) % 160
		if !b.used[i] && i < len(a.info.Pads) && a.info.Pads[i].File {
			return i
		}
	}
	return (b.last + 1) % 160
}

// openMIDIImport reads the file and opens the form: which part, as what, onto which pad, into which pattern. It says whether it did.
func (a *App) openMIDIImport(name string, data []byte) bool {
	f, err := formats.ParseMIDI(data)
	if err != nil {
		a.say(name+": "+err.Error(), true)
		return false
	}
	if a.scr != Seq && a.scr != Files {
		a.go_(Seq)
	}
	a.refresh()           // the names of the samples, as they are now
	if a.midiRun == nil { // a file on its own
		a.midiRun = &midiBatch{total: 1, used: map[int]bool{}}
	}
	b := a.midiRun
	st := &midiForm{name: name, f: f, pad: a.rowPad(), into: max(a.freePattern(), 1)}
	if a.midiPad > 0 { // the sample of the last melody
		st.pad = a.midiPad - 1
	}
	if b.into > 0 { // the next file of a run: into the pattern of the first, added to it, on the next pad
		st.into, st.mode, st.pad = b.into, 1, a.nextMIDIPad(b)
	}
	if b.into == 0 && a.midiInto > 0 { // the pattern of the last import: the song that is being made. It is added to (nothing is lost) when it has notes
		st.into = a.midiInto
		if pb, err := a.lab.PatternBytes(st.into); err == nil && len(pb) > 0 {
			st.mode = 1
		}
	}
	// the part with the most notes that is not the drums is the one to start with (the drums when that is all there is)
	best := -1
	for i, p := range f.Parts {
		if best < 0 || (f.Parts[best].Drums() && !p.Drums()) || (f.Parts[best].Drums() == p.Drums() && len(p.Notes) > len(f.Parts[best].Notes)) {
			best = i
		}
	}
	st.part = best
	title := "MIDI file " + fit(name, 28)
	if b.total > 1 {
		title = fmt.Sprintf("MIDI %d of %d: %s", b.done+1, b.total, fit(name, 24))
	}
	m := &menu{title: title}
	pads := make([]string, 160)    // the pads, for the pattern
	samples := make([]string, 160) // the pads with the name of their sample, for the melody
	for i := range pads {
		pads[i] = a.label(i)
		samples[i] = pads[i] + "  (no sample)"
		if i < len(a.info.Pads) && a.info.Pads[i].File {
			samples[i] = pads[i] + "  " + fit(a.info.Pads[i].Name, 22)
		}
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
		iMode
		iBars
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
		{label: "Use sample", vals: samples, val: st.pad, keepOpen: true, onChange: func(v int) { st.pad = v },
			run: func(int) { // A opens the pad dialog, the same one as everywhere a pad is chosen
				a.pickPad("use which sample?", st.pad, true, func(pad int) {
					st.pad = pad
					m.items[iPad].val = pad
					if st.playing {
						a.midiListen(st)
					}
				})
			}},
		{label: "Pad's own pitch is the note", vals: notes, keepOpen: true, onChange: func(v int) { st.root = v }},
		{label: "Into pattern", vals: pads, val: st.into - 1, keepOpen: true, onChange: func(v int) { st.into = v + 1 }},
		{label: "In the pattern", vals: []string{"replace what is there", "add to it; a shorter part goes round again", "add to it; a shorter part plays once"}, val: st.mode, keepOpen: true,
			onChange: func(v int) { st.mode = v }},
		{label: "Length in bars", keepOpen: true, vals: []string{"-"}, onChange: func(v int) { st.bars = bars[v] }},
	}
	if f.Tempo > 0 {
		m.items = append(m.items, menuItem{label: "Tempo", vals: []string{"keep " + bpmText(a.tempo()) + " BPM", "set " + bpmText(f.Tempo) + " BPM"}, keepOpen: true,
			onChange: func(v int) { st.tempo = v == 1 }})
	}
	m.items = append(m.items,
		menuItem{label: "Play it with the sample", keepOpen: true, run: func(int) {
			if st.playing {
				a.stop()
				st.playing = false
				return
			}
			a.midiListen(st)
		}},
		menuItem{label: "Import the pattern", run: func(int) { a.runMIDIImport(st) }})
	if b.total > 1 {
		m.items = append(m.items, menuItem{label: "Leave this file out", run: func(int) {
			a.stop()
			st.playing = false
			b.total--
			a.nextMIDI()
		}})
	}
	choose()
	for i := range m.items { // a change of the form is heard at once while it plays
		if ch := m.items[i].onChange; ch != nil {
			m.items[i].onChange = func(v int) {
				ch(v)
				if st.playing {
					a.midiListen(st)
				}
			}
		}
	}
	m.onClose = func() { a.stop(); st.playing = false; a.midiRun = nil } // B ends the run
	a.menu = m
	hint := "Left and Right choose, A on Use sample picks a pad, A on Import writes"
	if b.total > 1 {
		hint += ", B ends the run"
	}
	a.say(fmt.Sprintf("%s: %d parts. %s", name, len(f.Parts), hint), false)
	return true
}

// midiBuild makes the pattern the form describes (added to the one that is there, when the form says so), and says in a clause what pads it uses.
func (a *App) midiBuild(st *midiForm) (res formats.MIDIResult, said string, err error) {
	o := formats.MIDIImport{Part: st.part, Bars: st.bars, Pad: padIndex(st.pad), Root: st.root}
	if st.drums {
		pads, text := a.drumPads(st.f.Parts[st.part])
		if len(pads) == 0 {
			return res, "", errors.New("no pad has a sample: put some on the pads first")
		}
		o.Drums, said = pads, text
	} else if st.pad < len(a.info.Pads) && !a.info.Pads[st.pad].File {
		said = ", but " + a.label(st.pad) + " has no sample"
	}
	res, err = formats.BuildFromMIDI(st.f, o)
	if err != nil || st.mode == 0 {
		return res, said, err
	}
	base, perr := a.lab.PatternBytes(st.into)
	if perr != nil || len(base) == 0 { // nothing there yet: it is the first
		return res, said, nil
	}
	mg, err := formats.MergePatterns(base, res.Pattern, st.mode == 1)
	if err != nil {
		return res, said, err
	}
	res.Pattern, res.Merge = mg.Pattern, &mg
	return res, said, nil
}

// midiListen plays the pattern the form would make, in a loop, with the samples of the pads (the pattern is not written); again while
// it plays it takes the changes of the form into the loop.
func (a *App) midiListen(st *midiForm) {
	res, _, err := a.midiBuild(st)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	p, err := formats.ParsePattern(res.Pattern)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	d := lab.PatternDoc{Length: p.Length, Bars: res.Bars, Beats: st.f.Beats, PPQ: formats.PPQ}
	for _, e := range p.Events {
		if !e.Control {
			d.Notes = append(d.Notes, lab.Note{Tick: e.Tick, Pad: int(e.Pad.Index()), Vel: int(e.Velocity), Len: int(e.Length), Pitch: int(e.Pitch)})
		}
	}
	bpm := a.tempo()
	if st.tempo && st.f.Tempo > 0 {
		bpm = st.f.Tempo
	}
	s := a.mixDoc(d, bpm, 0, int(p.Length), func(int) bool { return true })
	silent := true
	for _, v := range s.Samples {
		if v != 0 {
			silent = false
			break
		}
	}
	if silent {
		a.say("nothing to hear: the pad has no sample", true)
		return
	}
	if r, ok := a.Audio.(replacer); ok && st.playing {
		r.ReplaceLoop(s)
		return
	}
	lp, ok := a.Audio.(looper)
	if !ok {
		a.say("no audio output", true)
		return
	}
	if err := lp.Loop(s); err != nil {
		a.say(err.Error(), true)
		return
	}
	st.playing = true
}

// runMIDIImport writes the pattern the form describes, and goes on to the next file of the run, if there is one.
func (a *App) runMIDIImport(st *midiForm) {
	a.stop()
	st.playing = false
	res, said, err := a.midiBuild(st)
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
	a.midiInto = st.into
	if !st.drums {
		a.midiPad = st.pad + 1
	}
	a.openPattern(st.into)
	msg := fmt.Sprintf("%s: %d notes, %d bars", a.label(st.into-1), res.Notes, res.Bars)
	if mg := res.Merge; mg != nil {
		msg = fmt.Sprintf("%s: now %d notes in %d bars", a.label(st.into-1), mg.Notes, mg.Bars)
		if mg.Bars != mg.WasBars {
			msg += fmt.Sprintf(" (was %d)", mg.WasBars)
		}
		if mg.Repeated {
			msg += ", shorter parts go round again"
		}
	}
	if st.drums {
		msg += ": " + said
	} else {
		msg += " with " + a.label(st.pad) + said
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
	if b := a.midiRun; b != nil {
		b.into, b.last = st.into, st.pad
		b.used[st.pad] = true
		b.done++
		if len(b.queue) > 0 {
			saved := a.msg
			a.nextMIDI()
			if a.midiRun != nil {
				a.say(saved, false)
			}
			return
		}
	}
	a.midiRun = nil
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
