package core

import (
	"math"
	"regexp"
	"strings"

	"github.com/moahbeatlab/404tui/formats"
)

// The note a sample is in, for the tools that play it at pitches (the chord and the arp from one sample): a sample of a G bass played
// "in C" must go down 7 semitones, not stay where it is. The guess comes from the pad's name (bass_G1, Pad Am, stab-F#), else from the
// sound (the pitch of a tone that holds), else C; the tool shows it and it can be changed.

// noteToken is a note in a name: a letter, a sharp or flat, then perhaps minor, major or an octave (G, F#, Bb, Am, C3, Ebmin).
var noteToken = regexp.MustCompile(`^([A-Ga-g])([#b]?)(m|min|maj|M)?(-?[0-9])?$`)

// noteFromName is the pitch class (0 C .. 11 B) named in a sample's name, false when it names none.
func noteFromName(name string) (int, bool) {
	toks := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '_' || r == '-' || r == '.' || r == '(' || r == ')' })
	for i := len(toks) - 1; i >= 0; i-- { // the note is mostly at the end (bass_G1, Pad 3 Am)
		t := toks[i]
		m := noteToken.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		if len(t) == 1 && m[1] == strings.ToLower(m[1]) { // a lone small letter is a word ("a"), not a note
			continue
		}
		if m[1] == "b" && m[2] == "" && m[3] == "" && m[4] == "" {
			continue
		}
		pc := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}[strings.ToUpper(m[1])[0]]
		switch m[2] {
		case "#":
			pc++
		case "b":
			pc--
		}
		return (pc + 12) % 12, true
	}
	return 0, false
}

// noteFromSound is the pitch class of a sample that holds a tone, found by autocorrelation over a part of it after the attack; false
// for a drum, a noise or a chord (no period stands out).
func noteFromSound(s formats.Sample) (int, bool) {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	const win = 4096
	start := min(frames/10, formats.SampleRate/10) // past the attack
	if frames < start+win+formats.SampleRate/40 {
		return 0, false
	}
	x := make([]float64, win+formats.SampleRate/40)
	for i := range x {
		x[i] = float64(s.Samples[(start+i)*ch])
	}
	minLag, maxLag := formats.SampleRate/1000, formats.SampleRate/40 // 40 Hz to 1 kHz
	var e0 float64
	for i := 0; i < win; i++ {
		e0 += x[i] * x[i]
	}
	if e0 == 0 {
		return 0, false
	}
	best, bestLag := 0.0, 0
	corr := make([]float64, maxLag+1)
	for lag := minLag; lag <= maxLag; lag++ {
		var c, e1 float64
		for i := 0; i < win; i++ {
			c += x[i] * x[i+lag]
			e1 += x[i+lag] * x[i+lag]
		}
		if e1 > 0 {
			corr[lag] = c / math.Sqrt(e0*e1)
		}
		if corr[lag] > best {
			best, bestLag = corr[lag], lag
		}
	}
	if best < 0.85 {
		return 0, false
	}
	// the first peak nearly as high as the best is the period (the best may be two or three periods)
	for lag := minLag + 1; lag < bestLag; lag++ {
		if corr[lag] >= best*0.95 && corr[lag] >= corr[lag-1] && corr[lag] >= corr[lag+1] {
			bestLag = lag
			break
		}
	}
	f := float64(formats.SampleRate) / float64(bestLag)
	midi := int(math.Round(69 + 12*math.Log2(f/440)))
	return (midi%12 + 12) % 12, true
}

// sampleNote is the note pad n's sample is in: the one chosen for it before, else the guess, with where the guess came from.
func (a *App) sampleNote(n int) (int, string) {
	if v, ok := a.onepad.in[n]; ok {
		return v, ""
	}
	if pc, ok := noteFromName(a.info.Pads[n].Name); ok {
		return pc, "from its name"
	}
	if id, ok := a.padSample(n); ok {
		if s, err := a.lab.Sample(id); err == nil {
			if pc, ok := noteFromSound(s); ok {
				return pc, "from its sound"
			}
		}
	}
	return 0, "a guess: change it if it is not"
}
