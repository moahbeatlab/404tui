// Package handout writes a project as an EPUB for an e-reader: every bank at a glance (its key, tempo, pads and patterns) and every
// pattern as step grids, a bar to a grid, for a look at what a pattern holds while the unit plays it.
package handout

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/moahbeatlab/404tui/lab"
)

// Pad is a pad of the project.
type Pad struct {
	Label   string // A1 .. J16
	Name    string
	Sample  bool    // it has a sample
	Seconds float64 // how long the sample is
}

// Pattern is a pattern that has notes.
type Pattern struct {
	N     int // 1..160
	Label string
	Bars  int
	Beats int // beats to the bar
	Notes []lab.Note
}

// Project is what the handout shows.
type Project struct {
	Name     string
	Tempo    float64
	Banks    [10]float64 // the tempo each bank plays at
	Pads     [160]Pad
	Patterns []Pattern
	Notes    Notes
}

// Notes are what the user writes for the handout: keys of banks and pads, names of patterns, a line for a bank. Keyed by the bank's
// letter ("A"), a pad ("A9") or a pattern ("A1"); pads and patterns share their labels, so they have a map each.
type Notes struct {
	Title    string            `json:"title,omitempty"`
	Keys     map[string]string `json:"keys,omitempty"`     // "A": "Am", "A9": "A"
	Banks    map[string]string `json:"banks,omitempty"`    // "A": "Drums, bongos, bass"
	Patterns map[string]string `json:"patterns,omitempty"` // "A1": "Groove"
}

// ReadNotes reads a notes file (JSON); a missing file is no notes.
func ReadNotes(path string) (Notes, error) {
	var n Notes
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return n, nil
	}
	if err != nil {
		return n, err
	}
	if err := json.Unmarshal(b, &n); err != nil {
		return n, fmt.Errorf("%s: %w", path, err)
	}
	return n, nil
}

// FromLab reads the project that is open in l.
func FromLab(l *lab.Lab) (Project, error) {
	var p Project
	raw, err := l.Info()
	if err != nil {
		return p, err
	}
	var in lab.Info
	if err := json.Unmarshal(raw, &in); err != nil {
		return p, err
	}
	if !in.Open {
		return p, fmt.Errorf("no project is open")
	}
	p.Name, p.Tempo = in.Name, in.Tempo
	for b := range p.Banks {
		p.Banks[b] = in.Tempo
		if !in.UsesProjectTempo && b < len(in.BankTempos) && in.BankTempos[b] > 0 {
			p.Banks[b] = in.BankTempos[b]
		}
	}
	for _, pi := range in.Pads {
		if pi.N < 0 || pi.N >= 160 {
			continue
		}
		p.Pads[pi.N] = Pad{Label: pi.Label, Name: pi.Name, Sample: pi.File, Seconds: float64(pi.Frames) / 48000}
	}
	for _, pt := range in.Patterns {
		if pt.Bad != "" || pt.Notes == 0 {
			continue
		}
		raw, err := l.Pattern(pt.N)
		if err != nil {
			return p, err
		}
		var d lab.PatternDoc
		if err := json.Unmarshal(raw, &d); err != nil {
			return p, err
		}
		p.Patterns = append(p.Patterns, Pattern{N: pt.N, Label: pt.Label, Bars: max(d.Bars, 1), Beats: max(d.Beats, 1), Notes: d.Notes})
	}
	return p, nil
}

// bankUsed is whether a bank has a sample on a pad or a pattern.
func (p Project) bankUsed(b int) bool {
	for i := b * 16; i < b*16+16; i++ {
		if p.Pads[i].Sample {
			return true
		}
	}
	for _, pt := range p.Patterns {
		if (pt.N-1)/16 == b {
			return true
		}
	}
	return false
}

func (p Project) patternsOf(b int) []Pattern {
	var out []Pattern
	for _, pt := range p.Patterns {
		if (pt.N-1)/16 == b {
			out = append(out, pt)
		}
	}
	return out
}

// padName is a pad's name, or its label when it has none.
func (p Project) padName(i int) string {
	if n := p.Pads[i].Name; n != "" {
		return n
	}
	return p.Pads[i].Label
}

func bankLetter(b int) string { return string(rune('A' + b)) }
