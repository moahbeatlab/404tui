package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// chromaticKeys are the keys that play notes, each a semitone above the one before: the home
// row is the white keys of a piano and the row above it the black ones, as on the typing
// keyboard of a DAW. The first plays the sample as it is.
const chromaticKeys = "awsedftgyhujkolp;'"

// chromaticOctaves is how many octaves z and x can shift the keys down and up.
const chromaticOctaves = 3

// keyRepeat tells a key held down from one pressed again: a terminal reports no key going up,
// only the same key over and over, and those come faster than fingers do.
var keyRepeat = 60 * time.Millisecond

// chromatic is the dialog that plays a pad's sample as notes from the keyboard, on this
// computer.
type chromatic struct {
	row app.PadRow
	// sample is the part of the sample the pad plays; nil while it is being read from the unit.
	sample *formats.Sample
	octave int
	// note is the last one played, in semitones from the sample's own pitch.
	note   int
	played bool
	// key and at are the last note key and when it came, to tell a held key.
	key string
	at  time.Time
}

// chromaticMsg is a pad's sample, read for the chromatic dialog.
type chromaticMsg struct {
	pad    proto.PadIndex
	sample formats.Sample
	err    error
}

// openChromatic opens the chromatic dialog on a pad, reading its sample from the unit unless
// it was read before.
func (m Model) openChromatic(row app.PadRow) (tea.Model, tea.Cmd) {
	if !row.HasSample {
		m.message = fmt.Sprintf("%s is empty; a assigns a file to it", row.Pad)
		return m, nil
	}
	if m.player == nil {
		m.message = "no audio output to play on"
		if m.audioErr != "" {
			m.message += ": " + m.audioErr
		}
		return m, nil
	}
	m.stopListening()
	m.chromatic = &chromatic{row: row}
	key := factKey{row.Pad, row.Size, row.Name}
	if sample, ok := m.padAudio.get(key); ok {
		m.chromatic.sample = &sample
		return m, nil
	}
	unit, ops, log, cache := m.unit, m.ops, m.log, m.padAudio
	return m, func() tea.Msg {
		ops.Lock()
		sample, err := app.PadAudio(unit, row)
		ops.Unlock()
		if err != nil {
			log.Warn("could not read the pad to play it chromatically", "pad", row.Pad.String(), "err", err)
			return chromaticMsg{pad: row.Pad, err: err}
		}
		cache.put(key, sample)
		return chromaticMsg{pad: row.Pad, sample: sample}
	}
}

// chromaticLoaded takes in the sample the dialog was waiting for.
func (m Model) chromaticLoaded(msg chromaticMsg) (tea.Model, tea.Cmd) {
	c := m.chromatic
	if c == nil || c.row.Pad != msg.pad {
		return m, nil
	}
	if msg.err != nil {
		m.chromatic = nil
		m.message = fmt.Sprintf("could not read %s: %v", msg.pad, msg.err)
		return m, nil
	}
	c.sample = &msg.sample
	return m, nil
}

func (m Model) chromaticKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.chromatic
	// Caps Lock must not silence the keyboard.
	name := strings.ToLower(key.String())
	switch name {
	case "esc", "q":
		m.chromatic = nil
		m.stopListening()
		return m, nil
	case "z":
		c.octave = max(c.octave-1, -chromaticOctaves)
	case "x":
		c.octave = min(c.octave+1, chromaticOctaves)
	case " ":
		m.stopListening()
	case "enter":
		if c.played {
			return m.playNote(c.note, "")
		}
	}
	if at := strings.Index(chromaticKeys, name); len(name) == 1 && at >= 0 {
		return m.playNote(at+12*c.octave, name)
	}
	return m, nil
}

// playNote plays the dialog's sample a number of semitones from its own pitch. One note
// sounds at a time: it plays to its end, or until the next one. key is the key that asked, so
// that holding it down does not start the note over and over.
func (m Model) playNote(semitones int, key string) (tea.Model, tea.Cmd) {
	c := m.chromatic
	if c.sample == nil {
		return m, nil
	}
	now := time.Now()
	held := key != "" && key == c.key && now.Sub(c.at) < keyRepeat
	c.key, c.at = key, now
	if held {
		return m, nil
	}
	c.note, c.played = semitones, true
	// A note still being worked out must not start after a later one.
	id := m.listenGen.Add(1)
	label := "pad " + c.row.Pad.String()
	m.listening = label
	sample, pad, player, gen, log := *c.sample, c.row.Pad.String(), m.player, m.listenGen, m.log
	return m, func() tea.Msg {
		start := time.Now()
		note := sample.Pitched(semitones)
		if gen.Load() != id {
			return listenMsg{path: label, stale: true}
		}
		err := player.Play(note)
		log.Debug("playing a note on this computer", "pad", pad, "semitones", semitones,
			"frames", note.Frames(), "took", time.Since(start).Round(time.Millisecond), "err", err)
		return listenMsg{path: label, err: err}
	}
}

// semitoneText names a note by its distance from the sample's own pitch.
func semitoneText(n int) string {
	switch n {
	case 0:
		return "the sample's own pitch"
	case 1, -1:
		return fmt.Sprintf("%+d semitone", n)
	}
	return fmt.Sprintf("%+d semitones", n)
}

// chromaticBox draws the keys as a piano: the black keys above the gaps between the white
// ones, and the note that was played last lit.
func (m Model) chromaticBox() string {
	st, c := m.st, m.chromatic
	white := lipgloss.NewStyle().Background(st.th.text).Foreground(st.th.tileEmpty)
	black := lipgloss.NewStyle().Background(st.th.tile).Foreground(st.th.text)
	lit := lipgloss.NewStyle().Background(st.th.hot).Foreground(st.th.cursorFg).Bold(true)

	// A white key is three cells wide with one between them; a black key sits over that gap.
	var top, bottom strings.Builder
	topW, whites := 0, 0
	for i, r := range chromaticKeys {
		style, isBlack := white, false
		switch i % 12 {
		case 1, 3, 6, 8, 10:
			style, isBlack = black, true
		}
		if c.played && c.note == i+12*c.octave {
			style = lit
		}
		label := style.Render(" " + string(r) + " ")
		if isBlack {
			col := 4*whites - 2
			top.WriteString(strings.Repeat(" ", col-topW) + label)
			topW = col + 3
			continue
		}
		if whites > 0 {
			bottom.WriteString(" ")
		}
		bottom.WriteString(label)
		whites++
	}

	state := st.dim.Render("reading the sample from the unit…")
	if c.sample != nil {
		state = st.dim.Render("octave ") + st.accent.Render(fmt.Sprintf("%+d", c.octave)) + st.faint.Render("   ·   ")
		if c.played {
			state += st.dim.Render("last note ") + st.hot.Render(semitoneText(c.note))
		} else {
			state += st.dim.Render("a plays the sample as it is, each key after it a semitone higher")
		}
	}
	lines := []string{
		st.bold.Render(fmt.Sprintf("Chromatic  ·  %s  %s", c.row.Pad, c.row.Name)), "",
		" " + top.String(), " " + bottom.String(), "",
		state, "",
		st.dim.Render("a…' play  ·  z / x octave down / up  ·  Enter again  ·  Space stop  ·  Esc close"),
		st.faint.Render("plays on this computer, one note at a time, by changing the speed: higher is shorter"),
	}
	return st.modal.Render(strings.Join(lines, "\n"))
}
