package core

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Melody (the NOTES tools of SEQ): notes at pitches for the pad of the track, a sample or the pad that plays a synth, in the key of
// the bank: a bass line, a lead, an arp, stabs or held pad chords, in a style. It is written as a motif, not as notes at random: one
// bar is made (its rhythm from the style, its notes from the scale, the chord tones on the strong steps) and the bars go A A' A B,
// each over its chord of the progression, the last one coming home. Ghost notes and accents are settings of their own, and the
// range is the octaves the notes stay in (a pattern note goes 12 semitones up or down from the pad's own pitch, not more).

var melRoles = []string{"bass", "lead", "arp", "stabs", "pad (held chords)"}

// melRoleFor names which of the three tracks of a groovebox a role fits (MC-101: bass, synth/lead, pads/everything else).
var melRoleFor = []string{"bass track", "synth / lead track", "synth / lead track", "pads track", "pads track"}

var melStyles = []string{"house", "deep house", "tech house", "techno", "trance", "progressive"}

var melModes = []struct {
	name  string
	steps []int
	minor bool
}{
	{"minor", []int{0, 2, 3, 5, 7, 8, 10}, true},
	{"major", []int{0, 2, 4, 5, 7, 9, 11}, false},
	{"dorian", []int{0, 2, 3, 5, 7, 9, 10}, true},
	{"phrygian", []int{0, 1, 3, 5, 7, 8, 10}, true},
	{"harmonic minor", []int{0, 2, 3, 5, 7, 8, 11}, true},
	{"minor pentatonic", []int{0, 3, 5, 7, 10}, true},
}

// melRanges are the windows of pitch the notes stay in, in semitones from the pad's own pitch; the first follows the role.
var melRanges = []struct {
	name   string
	lo, hi int
}{
	{"for the role", 0, 0},
	{"low octave (-12 to 0)", -12, 0},
	{"around the pad (-6 to +6)", -6, 6},
	{"high octave (0 to +12)", 0, 12},
	{"two octaves (-12 to +12)", -12, 12},
}

var melDensities = []string{"sparse", "medium", "busy"}
var melGhosts = []string{"none", "a few", "many"}
var melAccents = []string{"none", "on the offbeats", "on the beats", "here and there"}

// melodySet is the dialog's settings, kept between visits; key is -1 until the bank's key is read.
type melodySet struct {
	key, mode, role, style, prog, rng, density, ghost, accent int
	out                                                       [5]int // per role: where it plays, an index of synthOutNames
	init                                                      bool
}

func (a *App) melodyMenu() {
	pad := a.rowPad()
	st := &a.mel
	bank := (a.pat - 1) / 16
	key, minor, why := a.bankKey(bank)
	if !st.init {
		*st = melodySet{density: 1, ghost: 1, accent: 1, out: [5]int{1, 2, 2, 3, 3}, init: true} // MC-101: bass 11, lead 12, pads 13
		if !minor {
			st.mode = 1
		}
		st.role = melRoleGuess(a.info.Pads[pad].Name)
	}
	st.key = key
	if why != "" && !minor && st.mode == 0 {
		st.mode = 1
	}
	keys := make([]string, 12)
	for i, n := range sharpNames {
		keys[i] = n
		if i == key && why != "" {
			keys[i] = n + "  (the bank's)"
		}
	}
	modes := make([]string, len(melModes))
	for i, m := range melModes {
		modes[i] = m.name
	}
	roles := make([]string, len(melRoles))
	for i, r := range melRoles {
		roles[i] = fmt.Sprintf("%s  (%s)", r, melRoleFor[i])
	}
	progs := []string{"one chord (the key's)"}
	for _, p := range progressions {
		progs = append(progs, fmt.Sprintf("%s  %s", p.name, p.roman))
	}
	ranges := make([]string, len(melRanges))
	for i, r := range melRanges {
		ranges[i] = r.name
	}
	padNote, _ := a.sampleNote(pad)
	settings := []menuItem{
		val("Role", roles, st.role, func(v int) { st.role = v }),
		val("Style", melStyles, st.style, func(v int) { st.style = v }),
		val("Key", keys, st.key, func(v int) { st.key = v }),
		val("Scale", modes, st.mode, func(v int) { st.mode = v }),
		val("Chords", progs, st.prog, func(v int) { st.prog = v }),
		val("Range", ranges, st.rng, func(v int) { st.rng = v }),
		val("Density", melDensities, st.density, func(v int) { st.density = v }),
		val("Ghost notes", melGhosts, st.ghost, func(v int) { st.ghost = v }),
		val("Accents", melAccents, st.accent, func(v int) { st.accent = v }),
	}
	online := a.live != nil
	if online { // the synth behind the unit: only while the unit is edited live
		settings = append(settings, val("Play on", synthOutNames, st.out[st.role], func(v int) { st.out[st.role] = v }))
	}
	a.toolNoPlay = true
	defer func() { a.toolNoPlay = false }()
	a.toolMenu(fmt.Sprintf("melody for %s (sample in %s)", a.label(pad), sharpNames[padNote]), settings, "Write the melody", func() {
		d, err := a.ensurePattern()
		if err != nil {
			a.say(err.Error(), true)
			return
		}
		bars := max(int(d.Length)/(16*stepTicks), 1)
		pn, _ := a.sampleNote(pad)
		notes := melodyNotes(pad, bars, *st, pn, a.tt.seed)
		what := fmt.Sprintf("%s %s in %s %s", melStyles[st.style], melRoles[st.role], sharpNames[st.key], melModes[st.mode].name)
		a.finishTrack(pad, notes, what)
		if o := st.out[st.role]; o > 0 && online {
			k := ((st.key-pn)%12 + 12) % 12
			if k > 6 {
				k -= 12
			}
			if a.synthPlay(pad, a.trackNotes(pad), 10+o, synthTonic(st.role, st.key), k) {
				a.say(what+" "+synthSaid(10+o, a.midiOutName), false)
			}
		} else if a.synth != nil {
			a.synthStop()
		}
	})
	if m := a.menu; m != nil { // the synth stops with the dialog; Space plays the pattern on the computer, or stops it
		m.onX = func() {
			a.togglePlay()
			a.synthSync()
		}
		m.xHintOf = func() string {
			if a.seqOn {
				return "stop"
			}
			return "play here"
		}
		orig := m.onClose
		m.onClose = func() {
			a.synthStop()
			if orig != nil {
				orig()
			}
		}
	}
	said := "no key found in the bank's names or sounds: C minor, change it in Key"
	if why != "" {
		said = fmt.Sprintf("the bank is in %s %s (%s)", sharpNames[key], map[bool]string{true: "minor", false: "major"}[minor], why)
	}
	if a.synth != nil {
		said += "; playing " + synthSaid(a.synth.ch, a.midiOutName)
	}
	a.say(said, false)
}

// melRoleGuess is the role a pad's name points to (a bass, a lead, a pad...), else the lead.
func melRoleGuess(name string) int {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "bass") || strings.Contains(n, "sub") || strings.Contains(n, "808"):
		return 0
	case strings.Contains(n, "arp"):
		return 2
	case strings.Contains(n, "stab") || strings.Contains(n, "chord") || strings.Contains(n, "key") || strings.Contains(n, "organ") || strings.Contains(n, "piano"):
		return 3
	case strings.Contains(n, "pad") || strings.Contains(n, "string") || strings.Contains(n, "atmo"):
		return 4
	}
	return 1
}

// bankKey is the key of a bank: the note its names say most (G#, Gs, Am, Cmin), else the note its tonal pads sound in; minor unless
// the names say major. why is where it came from, empty when nothing was found.
func (a *App) bankKey(bank int) (pc int, minor bool, why string) {
	var votes [12]int
	majors, minors, named := 0, 0, 0
	for p := bank * 16; p < bank*16+16; p++ {
		pd := a.info.Pads[p]
		if !pd.File {
			continue
		}
		if n, mode, ok := keyFromName(pd.Name); ok {
			votes[n]++
			named++
			switch mode {
			case 1:
				majors++
			case 0:
				minors++
			}
		}
	}
	best := func() int {
		b := 0
		for i := range votes {
			if votes[i] > votes[b] {
				b = i
			}
		}
		return b
	}
	if named > 0 {
		b := best()
		return b, majors <= minors, fmt.Sprintf("%s in the names of %s", sharpNames[b], count(votes[b], "pad"))
	}
	sounded := 0
	for p := bank * 16; p < bank*16+16; p++ {
		pd := a.info.Pads[p]
		if !pd.File || melRoleGuess(pd.Name) == 1 && !tonalName(pd.Name) {
			continue
		}
		if id, ok := a.padSample(p); ok {
			if s, err := a.lab.Sample(id); err == nil {
				if n, ok := noteFromSound(s); ok {
					votes[n]++
					sounded++
				}
			}
		}
	}
	if sounded > 0 {
		return best(), true, fmt.Sprintf("the sound of %s", count(sounded, "tonal pad"))
	}
	return 0, true, ""
}

// tonalName is a name of a sound with a note (a lead, a synth...), not a drum.
func tonalName(name string) bool {
	n := strings.ToLower(name)
	for _, w := range []string{"lead", "synth", "pluck", "bell", "vox", "vocal", "melod", "hook", "loop"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// melodyNotes writes the track: bars of the pattern, the settings, the note the pad's sample is in, and the roll of the dice.
func melodyNotes(pad, bars int, st melodySet, padNote int, seed int64) []lab.Note {
	rng := rand.New(rand.NewSource(seed*7919 + int64(pad)*131 + int64(st.role)))
	mode := melModes[st.mode]
	k := ((st.key-padNote)%12 + 12) % 12 // the tonic, in semitones from the pad's own pitch (-5..6)
	if k > 6 {
		k -= 12
	}
	inScale := map[int]bool{}
	for _, s := range mode.steps {
		inScale[s] = true
	}
	lo, hi := melWindow(st)
	var scale []int // the pitches of the scale in the range, low to high
	for p := lo; p <= hi; p++ {
		if inScale[((p-k)%12+12)%12] {
			scale = append(scale, p)
		}
	}
	chordOf := func(bar int) []int { // the pitch classes (from the tonic) of the bar's chord, root first
		step := progStep{0, map[bool]byte{true: 'm', false: 'M'}[mode.minor]}
		if st.prog > 0 {
			steps := progressions[st.prog-1].steps
			step = steps[bar%len(steps)]
		}
		third := map[byte]int{'M': 4, 'D': 4, 'm': 3, 'd': 3}[step.q]
		fifth := 7
		if step.q == 'd' {
			fifth = 6
		}
		return []int{step.semi, (step.semi + third) % 12, (step.semi + fifth) % 12}
	}
	isTone := func(p int, chord []int) bool {
		for _, c := range chord {
			if ((p-k-c)%12+12)%12 == 0 {
				return true
			}
		}
		return false
	}
	nearest := func(p int, ok func(int) bool) int { // the nearest pitch of the range that ok takes
		for d := 0; d <= 24; d++ {
			for _, q := range []int{p - d, p + d} {
				if q >= lo && q <= hi && ok(q) {
					return q
				}
			}
		}
		return min(max(p, lo), hi)
	}
	rootNear := func(chord []int, near int) int {
		return nearest(near, func(q int) bool { return ((q-k-chord[0])%12+12)%12 == 0 })
	}
	toneNear := func(chord []int, near int) int { return nearest(near, func(q int) bool { return isTone(q, chord) }) }
	scaleIdx := func(p int) int { // the place of p in the scale, or of the pitch below it
		i := sort.SearchInts(scale, p)
		if i < len(scale) && scale[i] == p {
			return i
		}
		return max(i-1, 0)
	}
	centre := (lo + hi) / 2
	type hit struct{ step, length, pitch, vel int }
	var out []hit
	add := func(bar int, h hit) { h.step += bar * 16; out = append(out, h) }

	switch st.role {
	case 0: // bass: the style's rhythm, the chord's root mostly, its octave and fifth, an approach into the next chord
		tmpl := melBassRhythm(st.style, st.density, rng)
		rootW := []float64{0.65, 0.5, 0.6, 0.7, 0.85, 0.7}[st.style]
		fn := make([]int, len(tmpl)) // 0 root, 1 octave, 2 fifth, 3 a scale step from the root
		roll := func(i int) {
			r := rng.Float64()
			switch {
			case r < rootW:
				fn[i] = 0
			case r < rootW+(1-rootW)*0.4:
				fn[i] = 1
			case r < rootW+(1-rootW)*0.75:
				fn[i] = 2
			default:
				fn[i] = 3
			}
		}
		for i := range tmpl {
			roll(i)
		}
		base := rootNear(chordOf(0), min(max(0, lo), hi)) // the bass sits on the root nearest the pad's own pitch
		for bar := 0; bar < bars; bar++ {
			f := append([]int(nil), fn...)
			if form := bar % 4; form == 1 || form == 3 { // A' and B: the end of the bar changes
				for i := max(len(f)-1-form/2, 0); i < len(f); i++ {
					old := fn[i]
					roll(i)
					f[i], fn[i] = fn[i], old
				}
			}
			ch := chordOf(bar)
			root := rootNear(ch, base)
			for i, h := range tmpl {
				p := root
				switch f[i] {
				case 1:
					if root+12 <= hi {
						p = root + 12
					} else if root-12 >= lo {
						p = root - 12
					}
				case 2:
					p = nearest(root+7, func(q int) bool { return ((q-k-ch[2])%12+12)%12 == 0 })
				case 3:
					j := scaleIdx(root) + []int{-1, 1}[rng.Intn(2)]
					p = scale[min(max(j, 0), len(scale)-1)]
				}
				if i == len(tmpl)-1 && bar+1 < bars && st.prog > 0 && rng.Float64() < 0.5 { // into the next chord, a step away
					next := rootNear(chordOf(bar+1), root)
					j := scaleIdx(next) + []int{-1, 1}[rng.Intn(2)]
					p = scale[min(max(j, 0), len(scale)-1)]
				}
				add(bar, hit{h[0], h[1], p, 100})
			}
		}
	case 1: // lead: a motif of one bar, walking the scale, the chord tones on the strong steps; A A' A B
		rhythm := melLeadRhythm(st.style, st.density, rng)
		walk := make([]int, len(rhythm)) // the motif as moves in the scale from its first note
		for i := 1; i < len(walk); i++ {
			walk[i] = walk[i-1] + []int{-2, -1, -1, 0, 1, 1, 2, -3, 3}[rng.Intn(9)]
		}
		start := toneNear(chordOf(0), centre)
		for bar := 0; bar < bars; bar++ {
			form := bar % 4
			r, w := rhythm, walk
			if form == 1 || form == 3 { // the answer: the end changes
				w = append([]int(nil), walk...)
				from := len(w) - 1 - form/2
				for i := max(from, 1); i < len(w); i++ {
					w[i] = w[i-1] + []int{-2, -1, 1, 2}[rng.Intn(4)]
				}
			}
			ch := chordOf(bar)
			first := toneNear(ch, start)
			prev := -100
			for i, h := range r {
				j := min(max(scaleIdx(first)+w[i], 0), len(scale)-1)
				p := scale[j]
				if h[0]%4 == 0 || i == 0 { // a strong step: a chord tone
					p = toneNear(ch, p)
				}
				if p == prev && rng.Intn(3) > 0 { // a line moves: the same note twice is rare
					j = min(max(scaleIdx(p)+[]int{-1, 1}[rng.Intn(2)], 0), len(scale)-1)
					p = scale[j]
				}
				prev = p
				if bar == bars-1 && i == len(r)-1 && (bars%4 == 0 || bars < 4) { // the last note comes home
					p = rootNear(ch, p)
				}
				add(bar, hit{h[0], h[1], p, 96})
			}
		}
	case 2: // arp: the chord's tones up (or up and down) over the range, at the style's rate
		rate := []int{2, 2, 1, 1, 1, 1}[st.style] // in steps: 8ths in house, 16ths else
		if st.density == 0 {
			rate *= 2
		}
		updown := rng.Intn(2) == 0
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			var tones []int
			for p := lo; p <= hi; p++ {
				if isTone(p, ch) && p >= centre-7 && p <= centre+9 {
					tones = append(tones, p)
				}
			}
			if len(tones) == 0 {
				tones = []int{rootNear(ch, centre)}
			}
			seq := tones
			if updown && len(tones) > 2 {
				for i := len(tones) - 2; i > 0; i-- {
					seq = append(seq, tones[i])
				}
			}
			for s, i := 0, 0; s < 16; s, i = s+rate, i+1 {
				add(bar, hit{s, max(rate*2/3, 1), seq[i%len(seq)], 92})
			}
		}
	case 3, 4: // stabs: the chord on the style's offbeats; pad: the chord held over the bar
		tmpl := [][2]int{{0, 16}}
		if st.role == 3 {
			tmpl = melStabRhythm(st.style, st.density, rng)
		}
		for bar := 0; bar < bars; bar++ {
			ch := chordOf(bar)
			root := rootNear(ch, centre-3)
			voice := []int{root}
			for _, c := range ch[1:] {
				voice = append(voice, nearest(root+((c-ch[0])%12+12)%12, func(q int) bool { return isTone(q, []int{c}) }))
			}
			r := tmpl
			if bar%4 == 3 && st.role == 3 && len(tmpl) > 1 && rng.Intn(2) == 0 { // B: one stab less, or a push before the bar
				r = tmpl[:len(tmpl)-1]
			}
			for _, h := range r {
				for _, p := range voice {
					add(bar, hit{h[0], h[1], p, 92})
				}
			}
		}
	}

	// ghost notes: quiet, short notes on free 16ths, at the pitch before them (bass, lead and arp)
	if st.ghost > 0 && st.role < 3 { // (a chord does not ghost)
		taken := map[int]bool{}
		for _, h := range out {
			taken[h.step] = true
		}
		prob := []float64{0, 0.18, 0.4}[st.ghost]
		sort.Slice(out, func(i, j int) bool { return out[i].step < out[j].step })
		var ghosts []hit
		last := -1
		for s := 0; s < bars*16; s++ {
			if taken[s] {
				for _, h := range out {
					if h.step == s {
						last = h.pitch
					}
				}
				continue
			}
			if last < 0 || s%4 == 0 || rng.Float64() >= prob {
				continue
			}
			ghosts = append(ghosts, hit{s, 1, last, 34 + rng.Intn(14)})
		}
		out = append(out, ghosts...)
		// a line with no room left (a rolling bass): some of its own notes off the 8ths go quiet instead
		for i, want := 0, bars*[]int{0, 1, 3}[st.ghost]-len(ghosts); i < len(out) && want > 0; i++ {
			if out[i].step%2 == 1 && out[i].vel >= 60 && rng.Float64() < prob*2 {
				out[i].vel, out[i].length = 34+rng.Intn(14), 1
				want--
			}
		}
	}
	// accents
	for i := range out {
		h := &out[i]
		if h.vel < 60 { // a ghost
			continue
		}
		s := h.step % 16
		acc := false
		switch st.accent {
		case 1:
			acc = s%4 == 2
		case 2:
			acc = s%4 == 0
		case 3:
			acc = rng.Intn(4) == 0
		}
		v := h.vel - 10 + rng.Intn(9) // a little life
		if acc {
			v = 120 + rng.Intn(8)
		} else if st.accent > 0 {
			v -= 8
		}
		h.vel = min(max(v, 1), 127)
	}
	notes := make([]lab.Note, 0, len(out))
	for _, h := range out {
		if h.step >= bars*16 {
			continue
		}
		ln := min(h.length, bars*16-h.step) * stepTicks
		notes = append(notes, lab.Note{Tick: uint32(h.step * stepTicks), Pad: pad, Vel: h.vel, Len: max(ln-stepTicks/8, 1), Pitch: int(formats.PitchByte(h.pitch))})
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Tick < notes[j].Tick })
	return notes
}

// melWindow is the range of pitch (semitones from the pad's own pitch) for the settings.
func melWindow(st melodySet) (int, int) {
	r := melRanges[st.rng]
	if st.rng > 0 {
		return r.lo, r.hi
	}
	return [][2]int{{-7, 7}, {-2, 12}, {-5, 12}, {-5, 10}, {-7, 9}}[st.role][0], [][2]int{{-7, 7}, {-2, 12}, {-5, 12}, {-5, 10}, {-7, 9}}[st.role][1]
}

// melBassRhythm is a bar of a bass line in a style: the steps and lengths (in steps).
func melBassRhythm(style, density int, rng *rand.Rand) [][2]int {
	var steps []int
	length := 1
	switch style {
	case 0: // house: the offbeat
		steps, length = []int{2, 6, 10, 14}, 2
		if density == 2 {
			steps = append(steps, 3, 11)
		}
	case 1: // deep house: syncopated, longer
		steps, length = []int{0, 3, 6, 10, 13}, 2
		if density == 0 {
			steps = []int{0, 6, 10}
		}
	case 2, 4: // tech house and trance: rolling 16ths around the kick
		for s := 0; s < 16; s++ {
			if s%4 != 0 {
				steps = append(steps, s)
			}
		}
	case 3, 5: // techno and progressive: offbeat pairs
		steps = []int{2, 3, 6, 7, 10, 11, 14, 15}
	}
	keep := []float64{0.6, 0.85, 1}[density]
	var out [][2]int
	for _, s := range steps {
		if s%4 == 2 || rng.Float64() < keep { // the offbeat stays
			out = append(out, [2]int{s, length})
		}
	}
	if len(out) == 0 {
		out = [][2]int{{2, length}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// melLeadRhythm is a bar of a lead in a style: weighted steps (syncopated in house, running in trance), each note held to the next.
func melLeadRhythm(style, density int, rng *rand.Rand) [][2]int {
	w := func(s int) float64 {
		switch {
		case s%4 == 0:
			return []float64{0.4, 0.35, 0.35, 0.45, 0.75, 0.6}[style]
		case s%2 == 0:
			return []float64{0.65, 0.5, 0.6, 0.5, 0.8, 0.7}[style]
		default:
			return []float64{0.35, 0.25, 0.4, 0.35, 0.45, 0.35}[style]
		}
	}
	scale := []float64{0.7, 1, 1.35}[density]
	var steps []int
	for s := 0; s < 16; s++ {
		if rng.Float64() < w(s)*scale {
			steps = append(steps, s)
		}
	}
	for len(steps) < 3 { // a motif has three notes at least
		s := rng.Intn(16)
		if !containsInt(steps, s) {
			steps = append(steps, s)
		}
	}
	sort.Ints(steps)
	maxLen := []int{2, 4, 2, 2, 2, 3}[style]
	out := make([][2]int, len(steps))
	for i, s := range steps {
		next := 16
		if i+1 < len(steps) {
			next = steps[i+1]
		}
		out[i] = [2]int{s, max(min(next-s, maxLen), 1)}
	}
	return out
}

// melStabRhythm is a bar of chord stabs in a style.
func melStabRhythm(style, density int, rng *rand.Rand) [][2]int {
	var steps []int
	switch style {
	case 1: // deep house: the classic push
		steps = []int{3, 6, 10, 13}
	case 2, 3: // tech house, techno: few and dry
		steps = []int{2, 10}
		if density == 2 {
			steps = []int{2, 7, 10}
		}
	default: // house, trance, progressive: the offbeats
		steps = []int{2, 6, 10, 14}
	}
	if density == 0 && len(steps) > 2 {
		steps = []int{steps[0], steps[2]}
	}
	if density == 2 && rng.Intn(2) == 0 && !containsInt(steps, 15) {
		steps = append(steps, 15)
	}
	out := make([][2]int, len(steps))
	for i, s := range steps {
		out[i] = [2]int{s, 1}
	}
	return out
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
