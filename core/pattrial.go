package core

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/generate"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/proto"
)

// A new pattern that is tried before it is kept: the one pattern of the generator, and a variation of the pattern in front. It is written
// into the next free pattern and the loop plays it; "Another roll of the dice" writes another one into the same pattern; "Keep it" makes it
// one undo step, and the next roll goes into the next free pattern, so rolling and keeping gives a row of them; B throws away what was
// not kept and goes back to the pattern that was in front (or the last one kept).

type patTrial struct {
	a           *App
	base        *lab.Snap // the project without the trial
	home        int       // the pattern to go back to: the one in front when the dialog opened, then the last one kept
	slot        int       // the pattern the trial is in
	pending     bool      // there is a trial that is not kept
	startedLoop bool      // nothing played when the trial began: the dialog started the loop, and stops it
	played      int       // the pattern the loop played before the trial (0: none)
	label       string    // what the trial is, for undo
	// the rolls since the dialog opened or the last keep, for undo and redo inside the dialog: at is the one in the pattern
	hist      []trialRoll
	at        int
	onRestore func(state any) // puts the dialog's own state (the groove, the kind) back to a roll's
}

// trialRoll is one roll of the dice: the pattern, what it is and the dialog's state that made it.
type trialRoll struct {
	b     []byte
	label string
	state any
}

func (a *App) newPatTrial() *patTrial {
	t := &patTrial{a: a, base: a.lab.Snapshot(), home: max(a.pat, 1), at: -1}
	if a.seqOn {
		t.played = a.playPat
	}
	a.toolBase = t.base // what the keys do from here on is a trial: the dialog commits it or throws it away
	a.trialUndo = t.step
	return t
}

// newSeed is a seed for the generator: another one each time.
func (a *App) newSeed() int64 {
	if a.genRand == nil {
		a.genRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return a.genRand.Int63n(1 << 30)
}

// free is the first pattern from home on that holds nothing, -1 when every pattern is used.
func (t *patTrial) free() int {
	for i := 0; i < 160; i++ {
		n := (t.home-1+i)%160 + 1
		if _, ok := t.a.patternDoc(n); !ok {
			return n
		}
	}
	return -1
}

// put writes a roll into the trial's pattern, shows it and has the loop play it. what says what it is.
func (t *patTrial) put(b []byte, what string) bool {
	a := t.a
	if !t.pending {
		n := t.free()
		if n < 0 {
			a.say("all 160 patterns are used", true)
			return false
		}
		t.slot = n
	}
	if !t.show(b, what) {
		return false
	}
	t.hist = append(t.hist[:t.at+1], trialRoll{b: b, label: what})
	t.at = len(t.hist) - 1
	if _, ok := a.Audio.(looper); ok && !a.looping() {
		a.startLoop()
		t.startedLoop = a.seqOn
	}
	return true
}

// show writes a roll into the trial's pattern and puts it in front; the loop goes on with it.
func (t *patTrial) show(b []byte, what string) bool {
	a := t.a
	if err := a.lab.SetPatternBytes(t.slot, b); err != nil {
		a.say(err.Error(), true)
		return false
	}
	t.pending, t.label = true, what
	if a.seqOn {
		a.playPat = t.slot
	}
	a.openPattern(t.slot)
	return true
}

// remember keeps the dialog's state with the roll that was just put, so that undo brings both back.
func (t *patTrial) remember(state any) {
	if t.at >= 0 {
		t.hist[t.at].state = state
	}
}

// step is undo (or redo) inside the dialog: the roll before (or after) the one in the pattern comes back, with the state that made it.
func (t *patTrial) step(redo bool) {
	a := t.a
	n := t.at - 1
	if redo {
		n = t.at + 1
	}
	if n < 0 || n >= len(t.hist) {
		if redo {
			a.say("nothing to redo: this is the last roll", false)
		} else {
			a.say("nothing to undo: this is the first roll (B throws the trial away)", false)
		}
		return
	}
	r := t.hist[n]
	if !t.show(r.b, r.label) {
		return
	}
	t.at = n
	if t.onRestore != nil {
		t.onRestore(r.state)
	}
	word := "undone"
	if redo {
		word = "redone"
	}
	a.say(fmt.Sprintf("roll %s: %s, roll %d of %d", word, r.label, n+1, len(t.hist)), false)
}

// keep makes the trial one undo step; the dialog goes on from there.
func (t *patTrial) keep() {
	a := t.a
	if !t.pending {
		a.say("nothing to keep: roll the dice first", false)
		return
	}
	a.undo = append(a.undo, undoEntry{t.base, fit(t.label, 40)})
	a.redo = nil
	a.trimUndo()
	t.base = a.lab.Snapshot()
	a.toolBase = t.base
	t.home, t.pending = t.slot, false
	t.hist, t.at = nil, -1 // what was kept is one undo step of the project; the rolls before it are gone
	if !t.startedLoop {
		t.played = t.slot
	}
	a.say(fmt.Sprintf("kept in %s: %s", a.label(t.slot-1), t.label), false)
	a.liveSync() // a unit that is edited live gets what was kept (a trial is not sent)
}

// drop throws a trial that was not kept away: the project is as it was, the pattern to go back to is in front, and the loop plays what it
// played before (a loop the dialog started stops). It says whether there was one.
func (t *patTrial) drop() bool {
	a := t.a
	if !t.pending {
		return false
	}
	t.pending = false
	t.hist, t.at = nil, -1
	if t.startedLoop && a.seqOn {
		a.stop()
	}
	t.startedLoop = false
	a.pat, a.step = t.home, 0
	if a.seqOn {
		a.playPat = t.home
	}
	a.restoreProject(t.base)
	if a.seqOn && t.played != 0 && t.played != t.home { // another pattern played while this one was in view: it plays again
		a.pat, a.playPat = t.played, t.played
		a.replay()
		a.pat = t.home
	}
	return true
}

// close is what B does: the trial is thrown away.
func (t *patTrial) close() {
	a := t.a
	if t.drop() {
		a.say("the trial is thrown away", false)
	} else if t.startedLoop && a.seqOn {
		a.stop()
	}
	a.toolBase = nil
	a.trialUndo = nil
}

// ---- the generator's one pattern

var lockVals = []string{"new each roll", "stays"}

// generateOne is the generator for one pattern (a groove, a groove with a fill or with a roll), as a trial: the dice are rolled until the
// pattern is liked. The voices set to "stays" (the kick, the snare and the clap, the hats, the rest) play at the next roll what they play
// now, so a pattern is built in steps: a kick that is right, then hats for it, then the rest.
func (a *App) generateOne(st generate.Style, mode string, bars int) {
	bank, pads, alts := a.generatorKit()
	if bank < 0 || len(pads) == 0 {
		a.say("no bank has a pad named like a kick: name the pads of the kit (kick, snare, hat...) or choose them in Generator kit", true)
		return
	}
	has := func(l formats.Lane) bool { _, ok := pads[l]; return ok }
	t := a.newPatTrial()
	var (
		prev  *generate.Pattern // the groove of the roll before, without its fill
		locks = make([]bool, len(generate.Groups))
		seed  int64
	)
	// roll makes a pattern: a new groove (but for the voices that stay), or only another fill on the groove that is there
	roll := func(groove bool) bool {
		keep := func(l formats.Lane) bool { return locks[generate.GroupOf(l)] }
		if groove || prev == nil {
			seed = a.newSeed()
		} else {
			keep = func(formats.Lane) bool { return true }
		}
		r, g := generate.One(mode, st, bars, seed, a.newSeed(), has, prev, keep)
		b, err := r.Pattern.BytesAlt(pads, alts)
		if err != nil {
			a.say(err.Error(), true)
			return false
		}
		if !t.put(b, st.Name+": "+r.Section.Name) {
			return false
		}
		prev = &g
		t.remember(oneState{prev, seed})
		a.say(fmt.Sprintf("%s in %s, seed %d: a trial until it is kept (u undoes a roll)", t.label, a.label(t.slot-1), seed), false)
		return true
	}
	t.onRestore = func(s any) {
		if st, ok := s.(oneState); ok {
			prev, seed = st.prev, st.seed
		}
	}
	if !roll(true) {
		t.close()
		return
	}
	m := &menu{title: "generate: " + st.Name}
	m.head("TRIAL: " + generate.ModeLabel(mode))
	m.items = append(m.items, menuItem{label: "Another roll of the dice", keepOpen: true, run: func(int) { roll(true) }})
	switch mode {
	case "fill":
		m.items = append(m.items, menuItem{label: "Only another fill, the groove stays", keepOpen: true, run: func(int) { roll(false) }})
	case "roll":
		m.items = append(m.items, menuItem{label: "Only another drum roll, the groove stays", keepOpen: true, run: func(int) { roll(false) }})
	}
	m.items = append(m.items, menuItem{label: "Keep it (B throws it away)", keepOpen: true, run: func(int) { t.keep() }})
	m.head("WHAT STAYS AT THE NEXT ROLL")
	for i, name := range generate.Groups {
		i := i
		m.items = append(m.items, menuItem{label: name, vals: lockVals, keepOpen: true, onChange: func(v int) { locks[i] = v == 1 }})
	}
	m.onClose = t.close
	a.menu = m
}

// oneState is what the generator's dialog needs of a roll to go on from it: the groove (for "only another fill" and the voices that stay)
// and its seed.
type oneState struct {
	prev *generate.Pattern
	seed int64
}

// ---- a variation of the pattern in front

const variationLabel = "Variation of this pattern (fill, roll, thinner, busier, break)..."

// varySource is a pattern read for a variation: its notes as rows of steps, which voice each of its pads plays, and the pad a voice's new
// notes go to.
type varySource struct {
	n      int
	doc    lab.PatternDoc
	before generate.Pattern
	laneOf map[int]formats.Lane
	pads   map[formats.Lane]proto.PadIndex
}

// drumVoices says which voice each pad of a pattern plays, and which pad plays the notes that are added to a voice. The pads of the
// generator's kit are known; the other pads of the pattern are taken by their names (kick, snare, hat...), and a pad that is no drum (a
// bass, chords) has no voice: its notes are never touched. A voice the pattern does not play takes the pad of the kit.
func (a *App) drumVoices(d lab.PatternDoc) (map[int]formats.Lane, map[formats.Lane]proto.PadIndex) {
	_, kit, alts := a.generatorKit()
	laneOf := map[int]formats.Lane{}
	for l, p := range kit {
		laneOf[int(p.Index())] = l
	}
	for l, p := range alts {
		if _, ok := laneOf[int(p.Index())]; !ok {
			laneOf[int(p.Index())] = l
		}
	}
	played := map[int]bool{}
	for _, n := range d.Notes {
		played[n.Pad] = true
	}
	var used []int
	for pad := range played {
		used = append(used, pad)
	}
	sort.Ints(used)
	var rest []app.PadRow
	for _, pad := range used {
		if _, ok := laneOf[pad]; ok || pad < 0 || pad >= len(a.info.Pads) {
			continue
		}
		if idx, ok := proto.NewPadIndex(uint16(pad)); ok {
			rest = append(rest, app.PadRow{Pad: idx, HasSample: a.info.Pads[pad].File, Name: a.info.Pads[pad].Name})
		}
	}
	for len(rest) > 0 { // a round names one pad for each voice: two hats of another bank are both hats
		found := app.GuessRoles(rest)
		if len(found) == 0 {
			break
		}
		taken := map[proto.PadIndex]bool{}
		for l, p := range found {
			laneOf[int(p.Index())] = l
			taken[p] = true
		}
		var left []app.PadRow
		for _, r := range rest {
			if !taken[r.Pad] {
				left = append(left, r)
			}
		}
		rest = left
	}
	pads := map[formats.Lane]proto.PadIndex{}
	for _, pad := range used { // the pad of the pattern that plays the voice: the kit's when the pattern plays it, else the first
		l, ok := laneOf[pad]
		if !ok {
			continue
		}
		idx, _ := proto.NewPadIndex(uint16(pad))
		if _, have := pads[l]; !have || kit[l] == idx {
			pads[l] = idx
		}
	}
	for l, p := range kit {
		if _, ok := pads[l]; !ok {
			pads[l] = p
		}
	}
	return laneOf, pads
}

// readForVariation reads pattern n for a variation; the error says why there can be none.
func (a *App) readForVariation(n int) (*varySource, error) {
	d, ok := a.patternDoc(n)
	if !ok || len(d.Notes) == 0 {
		return nil, fmt.Errorf("pattern %s is empty: there is nothing to make a variation of", a.label(n-1))
	}
	if d.Beats != 4 || d.Bars < 1 || d.Bars > formats.MaxStartBars || int(d.Length) != d.Bars*4*formats.PPQ {
		return nil, fmt.Errorf("a variation needs a pattern in 4/4 of 1 to %d bars", formats.MaxStartBars)
	}
	s := &varySource{n: n, doc: d}
	s.laneOf, s.pads = a.drumVoices(d)
	var hits []generate.Hit
	for _, note := range d.Notes {
		if l, ok := s.laneOf[note.Pad]; ok {
			hits = append(hits, generate.Hit{Lane: l, Tick: note.Tick, Vel: note.Vel, Len: note.Len})
		}
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("no pad of %s is a drum of the kit or named like one (kick, snare, hat...): choose the pads in Generator kit", a.label(n-1))
	}
	s.before = generate.Read(d.Bars, hits)
	return s, nil
}

// vary makes a variation of the source: the pattern file, and what was done. Only the steps the variation changed are written anew, on the
// pad of their voice and with the swing, the nudge and the length the voice has in the pattern; every other note stays as it is, with its
// own timing, length and pitch, and so do the pads that are no drums.
func (a *App) vary(s *varySource, kind string, seed int64) ([]byte, string, error) {
	has := func(l formats.Lane) bool { _, ok := s.pads[l]; return ok }
	after, what, err := generate.Vary(s.before, kind, seed, has)
	if err != nil {
		return nil, "", err
	}
	changed, add := generate.Diff(s.before, after)
	steps := s.doc.Bars * 16
	drop := map[formats.PadTick]bool{}
	for _, note := range s.doc.Notes {
		l, ok := s.laneOf[note.Pad]
		if !ok || changed[l] == nil || !changed[l][generate.StepOf(note.Tick, steps)] {
			continue
		}
		if idx, ok := proto.NewPadIndex(uint16(note.Pad)); ok {
			drop[formats.PadTick{Pad: idx, Tick: note.Tick}] = true
		}
	}
	var more []byte
	if len(add) > 0 {
		if more, err = formats.BuildSteps(add, s.pads, s.doc.Bars); err != nil {
			return nil, "", err
		}
	}
	if len(drop) == 0 && more == nil {
		return nil, "", fmt.Errorf("%s changes nothing here", generate.VaryLabel(kind))
	}
	src, err := a.lab.PatternBytes(s.n)
	if err != nil {
		return nil, "", err
	}
	out, err := formats.ReplaceNotes(src, drop, more)
	if err != nil {
		return nil, "", err
	}
	return out, what, nil
}

// variationMenu is the dialog of a variation of the pattern in front: what kind (a change rolls it), another roll, keep it. The variation is a
// trial in the next free pattern; the pattern it is made of is never changed, and every roll starts from it again.
func (a *App) variationMenu() {
	s, err := a.readForVariation(a.pat)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	t := a.newPatTrial()
	kind := 0
	roll := func() {
		k := generate.Variations[kind]
		b, what, err := a.vary(s, k, a.newSeed())
		if err != nil { // this kind does nothing for this pattern: the trial of the kind before does not stay under its name
			t.drop()
			a.say(err.Error(), true)
			return
		}
		if t.put(b, fmt.Sprintf("%s %s: %s", a.label(s.n-1), generate.VaryLabel(k), what)) {
			t.remember(kind)
			a.say(fmt.Sprintf("%s, in %s: a trial until it is kept (u undoes a roll)", t.label, a.label(t.slot-1)), false)
		}
	}
	labels := make([]string, len(generate.Variations))
	for i, k := range generate.Variations {
		labels[i] = generate.VaryLabel(k)
	}
	m := &menu{title: "variation of " + a.label(s.n-1)}
	m.head("TRIAL")
	m.items = append(m.items, menuItem{label: "What", vals: labels, keepOpen: true, onChange: func(v int) { kind = v; roll() }, run: func(int) { roll() }})
	m.items = append(m.items, menuItem{label: "Another roll of the dice", keepOpen: true, run: func(int) { roll() }})
	m.items = append(m.items, menuItem{label: "Keep it (B throws it away)", keepOpen: true, run: func(int) { t.keep() }})
	m.footH = 3
	m.foot = func(g Gfx, x, y, wc int) {
		g.HRule(x+1, y, wc-2, cLine)
		for i, l := range wrap(generate.VaryInfo(generate.Variations[kind]), wc-4) {
			if i < 2 {
				g.Text(x+2, y+1+i, l, cText, false)
			}
		}
	}
	t.onRestore = func(s any) { // the kind of the roll that came back is the one shown
		if k, ok := s.(int); ok {
			kind = k
			m.items[1].val = k
		}
	}
	m.onClose = t.close
	a.menu = m
	roll()
}
