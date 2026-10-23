package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// ArpConsts lists the choices of the chord and arp maker: patterns, rates, gates, swings,
// scales, chord types, pitch names and the progressions.
func ArpConsts() ([]byte, error) {
	type named struct {
		Name string  `json:"name"`
		Note string  `json:"note,omitempty"`
		Iv   []int   `json:"iv,omitempty"`
		Per  int     `json:"per,omitempty"`
		Part float64 `json:"part,omitempty"`
	}
	var dirs, rates, swings, scales, chords []named
	for i, d := range formats.ArpDirs {
		dirs = append(dirs, named{Name: d, Note: formats.ArpDirNotes[i]})
	}
	for _, r := range formats.ArpRates {
		rates = append(rates, named{Name: r.Name, Per: r.PerBeat})
	}
	for _, s := range formats.ArpSwings {
		swings = append(swings, named{Name: s.Name, Part: s.Part})
	}
	for _, s := range formats.ArpScales {
		scales = append(scales, named{Name: s.Name, Iv: s.Iv})
	}
	for _, c := range formats.ArpChords {
		chords = append(chords, named{Name: c.Name, Iv: c.Iv})
	}
	var progs []map[string]any
	for _, p := range Progressions {
		progs = append(progs, map[string]any{"name": p.Name, "scale": p.Scale, "degrees": p.Degrees, "note": p.Note})
	}
	return json.Marshal(map[string]any{
		"dirs": dirs, "rates": rates, "gates": formats.ArpGates, "swings": swings, "scales": scales,
		"chords": chords, "pitchNames": formats.PitchNames, "progressions": progs, "rhythms": RhythmNames,
	})
}

// ChordNotes gives the pitch classes of a chord (see formats.ChordNotes).
func ChordNotes(root, scale, chord, degree int) []int {
	return formats.ChordNotes(((root%12)+12)%12, scale, chord, degree)
}

// ChordName names the chord whose notes are given in the order of the chord (root first), such
// as "Cm7" or "F#dim"; "?" when it is none of the known types.
func ChordName(notes []int) string {
	if len(notes) == 0 {
		return ""
	}
	root := notes[0]
	var iv []int
	for _, n := range notes {
		iv = append(iv, ((n-root)%12+12)%12)
	}
	slices.Sort(iv)
	iv = slices.Compact(iv)
	if slices.Equal(iv, []int{0, 3, 6, 10}) {
		return formats.PitchNames[root] + "m7b5"
	}
	for _, c := range formats.ArpChords {
		var want []int
		for _, i := range c.Iv {
			want = append(want, i%12)
		}
		slices.Sort(want)
		want = slices.Compact(want)
		if slices.Equal(want, iv) {
			q := map[string]string{"MAJ": "", "MIN": "m", "7": "7", "MAJ7": "maj7", "MIN7": "m7", "SUS2": "sus2", "SUS4": "sus4",
				"DIM": "dim", "AUG": "aug", "5": "5", "6": "6", "9": "9"}[c.Name]
			return formats.PitchNames[root] + q
		}
	}
	return formats.PitchNames[root] + "?"
}

// Chord is one chord of a chord kit.
type Chord struct {
	Degree int    `json:"degree"` // 1..7 (triads) or 8..14 (sevenths)
	Roman  string `json:"roman"`
	Name   string `json:"name"`
	Notes  []int  `json:"notes"`
}

var romans = []string{"I", "II", "III", "IV", "V", "VI", "VII"}

// ChordKit is the chords on each degree of a scale (the triads, or the seventh chords), one
// for each of the pads of a chord kit.
func ChordKit(root, scale int, sevenths bool) ([]Chord, error) {
	if scale < 0 || scale >= len(formats.ArpScales) {
		return nil, errors.New("pick a scale")
	}
	root = ((root % 12) + 12) % 12
	if len(formats.ArpScales[scale].Iv) < 7 {
		return nil, fmt.Errorf("the %s scale has fewer than 7 notes: use a 7-note scale for a chord kit", formats.ArpScales[scale].Name)
	}
	var out []Chord
	for d := 1; d <= 7; d++ {
		deg := d
		if sevenths {
			deg = d + 7
		}
		notes := formats.ChordNotes(root, scale, 0, deg)
		name := ChordName(notes)
		r := romans[d-1]
		if strings.HasSuffix(name, "m") || strings.HasSuffix(name, "m7") || strings.HasSuffix(name, "dim") {
			r = strings.ToLower(r)
		}
		if strings.Contains(name, "dim") {
			r += "°"
		}
		out = append(out, Chord{Degree: deg, Roman: r, Name: name, Notes: notes})
	}
	return out, nil
}

// Progression is a chord sequence by scale degree.
type Progression struct {
	Name    string `json:"name"`
	Scale   string `json:"scale"` // the kind of scale it is written for: MAJ or MIN
	Degrees []int  `json:"degrees"`
	Note    string `json:"note"`
}

// Progressions are the progressions the lab can write as a pattern over a chord kit.
var Progressions = []Progression{
	{"I V vi IV", "MAJ", []int{1, 5, 6, 4}, "the pop progression"},
	{"vi IV I V", "MAJ", []int{6, 4, 1, 5}, "the same four, starting on the minor"},
	{"I vi IV V", "MAJ", []int{1, 6, 4, 5}, "50s doo-wop"},
	{"ii V I I", "MAJ", []int{2, 5, 1, 1}, "jazz turnaround"},
	{"I IV I V", "MAJ", []int{1, 4, 1, 5}, "blues-like, rock"},
	{"i VI III VII", "MIN", []int{1, 6, 3, 7}, "epic minor"},
	{"i VII VI VII", "MIN", []int{1, 7, 6, 7}, "trance / anthem"},
	{"i iv v i", "MIN", []int{1, 4, 5, 1}, "plain minor"},
	{"i VI iv V", "MIN", []int{1, 6, 4, 5}, "dramatic"},
	{"i III VII IV", "MIN", []int{1, 3, 7, 4}, "dorian lift"},
	{"I iii IV V", "MAJ", []int{1, 3, 4, 5}, "rising"},
	{"vi V IV III", "MIN", []int{6, 5, 4, 3}, "andalusian fall"},
}

// Variant is an arp setting with a name.
type Variant struct {
	Name string      `json:"name"`
	Arp  formats.Arp `json:"arp"`
}

func (v Variant) with(f func(a *formats.Arp)) Variant {
	f(&v.Arp)
	return v
}

// ArpVariants makes n different arps from one chord: a curated first set (each a classic way
// to play a chord), then seeded random mixes of pattern, rate, gate, swing and octaves. The
// same base, n and seed always give the same list. The chord, key, root, tempo and bars stay
// those of base.
func ArpVariants(base formats.Arp, n int, seed int64) []Variant {
	if n < 1 {
		n = 1
	}
	n = min(n, 64)
	mk := func(name string, dir, rate, gate, swing, oct int) Variant {
		a := base
		a.Dir, a.Rate, a.Gate, a.Swing, a.Octaves, a.Shift = dir, rate, gate, swing, oct, 0
		return Variant{name, a}
	}
	// Rates: 0 1/4, 1 1/8, 2 1/8T, 3 1/16, 4 1/16T, 5 1/32. Gates: 0 25%, 1 50%, 2 75%, 3 100%, 4 ring.
	// Swings: 0 off, 1 light, 2 triplet.
	curated := []Variant{
		mk("Up 16ths", formats.ArpUp, 3, 1, 0, 2),
		mk("Down 8ths", formats.ArpDown, 1, 2, 0, 2),
		mk("Up-down swung", formats.ArpUpDown, 3, 1, 1, 2),
		mk("Thirds run", formats.ArpThirds, 3, 1, 0, 2),
		mk("Pedal bass 8ths", formats.ArpPedal, 1, 2, 0, 2),
		mk("Offbeat stabs", formats.ArpStabs, 1, 0, 0, 1),
		mk("Triplet climb", formats.ArpUp, 4, 2, 0, 2),
		mk("Gliss 32nds", formats.ArpGliss, 5, 1, 0, 1),
		mk("Down-up ring", formats.ArpDownUp, 1, 4, 0, 2),
		mk("Wide 3 octaves", formats.ArpUp, 2, 3, 0, 3),
		mk("Gated 16ths", formats.ArpUpDown, 3, 0, 0, 1),
		mk("Shuffle stabs", formats.ArpStabs, 3, 0, 2, 1),
	}
	rng := rand.New(rand.NewSource(seed))
	dirs := []int{formats.ArpUp, formats.ArpDown, formats.ArpUpDown, formats.ArpDownUp, formats.ArpRandom, formats.ArpGliss,
		formats.ArpThirds, formats.ArpPedal, formats.ArpStabs}
	rates := []int{1, 2, 3, 3, 3, 4, 5}
	out := make([]Variant, 0, n)
	for i := 0; i < n; i++ {
		if i < len(curated) {
			v := curated[i]
			v.Arp.Seed = seed + int64(i)
			out = append(out, v)
			continue
		}
		dir := dirs[rng.Intn(len(dirs))]
		rate := rates[rng.Intn(len(rates))]
		gate := rng.Intn(5)
		swing := 0
		if rng.Intn(3) == 0 {
			swing = 1 + rng.Intn(2)
		}
		oct := 1 + rng.Intn(3)
		v := mk(fmt.Sprintf("%s %s", formats.ArpDirs[dir], formats.ArpRates[rate].Name), dir, rate, gate, swing, oct)
		v.Arp.Seed = rng.Int63n(1 << 20)
		if swing > 0 {
			v.Name += " swing"
		}
		out = append(out, v)
	}
	return out
}

// ArpVariantsJSON is ArpVariants over JSON.
func ArpVariantsJSON(base []byte, n int, seed int64) ([]byte, error) {
	var a formats.Arp
	if err := json.Unmarshal(base, &a); err != nil {
		return nil, err
	}
	if len(a.Notes) == 0 {
		return nil, errors.New("the chord has no notes")
	}
	return json.Marshal(ArpVariants(a, n, seed))
}

// MaxArpFrames limits the length of what RenderArp may make (about 40 s at 48 kHz).
const MaxArpFrames = 48000 * 40

// RenderArp makes the chord or arp from a sample and keeps it as a new sample.
func (l *Lab) RenderArp(id int, arp []byte) ([]byte, error) {
	src, err := l.Sample(id)
	if err != nil {
		return nil, err
	}
	var a formats.Arp
	if err := json.Unmarshal(arp, &a); err != nil {
		return nil, err
	}
	if len(a.Notes) == 0 {
		return nil, errors.New("the chord has no notes")
	}
	a.Key, a.Root = ((a.Key%12)+12)%12, ((a.Root%12)+12)%12
	a.Octaves = min(max(a.Octaves, 1), 3)
	a.Shift = min(max(a.Shift, -2), 2)
	a.Bars = min(max(a.Bars, 1), 8)
	if a.BPM < 20 || a.BPM > 300 {
		a.BPM = 120
	}
	a.Dir = min(max(a.Dir, 0), len(formats.ArpDirs)-1)
	a.Rate = min(max(a.Rate, 0), len(formats.ArpRates)-1)
	a.Gate = min(max(a.Gate, 0), len(formats.ArpGates)-1)
	a.Swing = min(max(a.Swing, 0), len(formats.ArpSwings)-1)
	if src.Frames() > MaxArpFrames {
		return nil, errors.New("that sample is too long for a chord: cut it shorter first (under 40 seconds)")
	}
	out := formats.RenderArp(src, a)
	if out.Frames() == 0 {
		return nil, errors.New("nothing to render")
	}
	if out.Frames() > MaxArpFrames {
		return nil, errors.New("that comes out too long: use fewer bars or a shorter sample")
	}
	return json.Marshal(l.meta(l.keep(out)))
}
