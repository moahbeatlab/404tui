package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

// patternBed is a pattern, mixed from its pads, that plays under the slices: every round of the
// slicer is as long as the pattern (or a whole number of times that), the slice steps lie on the
// pattern's grid, and the two are added into one sample, so they cannot drift apart.
type patternBed struct {
	slot   proto.PadIndex
	sample formats.Sample
	bpm    float64
}

// slicerBedMsg is a pattern mixed for the slicer, or the end of the pattern (none).
type slicerBedMsg struct {
	slot   proto.PadIndex
	sample formats.Sample
	bpm    float64
	remix  bool
	err    error
}

// patternChooser is the list of a bank's 16 pattern slots, laid out like the pads, that P opens
// in place of the keys under the waveform.
type patternChooser struct {
	bank, cursor int
	rows         []app.PatternRow
}

// slicerChoosePattern opens the chooser on the bank of the pattern now under the slices, or on
// the bank of the pad in front, at the first pattern there is.
func (m Model) slicerChoosePattern() (tea.Model, tea.Cmd) {
	s := m.slicer
	if m.unit == nil || m.snap == nil {
		m.message = "no unit to read patterns from"
		return m, nil
	}
	bank := m.bank
	if s.bed != nil {
		bank = int(s.bed.slot.Bank())
	}
	c := &patternChooser{bank: bank}
	if err := m.loadChooser(c); err != nil {
		m.message = "patterns: " + err.Error()
		return m, nil
	}
	if s.bed != nil {
		c.cursor = int(s.bed.slot.Pad())
	}
	s.chooser = c
	return m, nil
}

// loadChooser reads the slots of the chooser's bank, and puts the cursor on the first pattern
// when it is not on one.
func (m Model) loadChooser(c *patternChooser) error {
	m.ops.Lock()
	rows, err := app.LoadPatterns(m.unit, c.bank)
	m.ops.Unlock()
	if err != nil {
		return err
	}
	c.rows = rows
	if !c.rows[c.cursor].Exists {
		for i, r := range rows {
			if r.Exists {
				c.cursor = i
				break
			}
		}
	}
	return nil
}

// slicerChooserKey moves in the chooser: the arrows or h j k l over the slots, [ ] for another
// bank, Enter to take the pattern, 0 for none, Esc to leave it as it was.
func (m Model) slicerChooserKey(name string) (tea.Model, tea.Cmd) {
	s := m.slicer
	c := s.chooser
	switch name {
	case "esc", "q", "P":
		s.chooser = nil
	case "h", "left":
		c.cursor = clamp(c.cursor-1, 0, proto.PadsPerBank-1)
	case "l", "right":
		c.cursor = clamp(c.cursor+1, 0, proto.PadsPerBank-1)
	case "k", "up":
		c.cursor = clamp(c.cursor-4, 0, proto.PadsPerBank-1)
	case "j", "down":
		c.cursor = clamp(c.cursor+4, 0, proto.PadsPerBank-1)
	case "[", "]":
		next := c.bank + 1
		if name == "[" {
			next = c.bank - 1
		}
		if next >= 0 && next < proto.Banks {
			old := c.bank
			c.bank = next
			if err := m.loadChooser(c); err != nil {
				c.bank = old
				m.message = "patterns: " + err.Error()
			}
		}
	case "0", "backspace", "x":
		s.chooser = nil
		if s.bed != nil {
			s.bed, s.bedRange, s.dirty = nil, nil, true
			m.message = "the slices play alone again"
		}
	case "enter":
		row := c.rows[c.cursor]
		if !row.Exists {
			m.message = "that pattern slot is empty"
			return m, nil
		}
		s.chooser = nil
		if s.bedRange != nil && s.bedRange.slot != row.Slot {
			s.bedRange = nil
		}
		m.message = "mixing pattern " + row.Slot.String() + "…"
		return m, m.slicerMixBed(row.Slot, false)
	}
	return m, nil
}

// slicerMixBed mixes a pattern in the background: remix when it is the pattern already under the
// slices, mixed again after something changed.
func (m Model) slicerMixBed(slot proto.PadIndex, remix bool) tea.Cmd {
	var rng *barRange
	if r := m.slicer.bedRange; r != nil && r.slot == slot {
		rng = r
	}
	mix, bpm := m.patternMixIn(slot, rng), m.bankTempo(int(slot.Bank()))
	return func() tea.Msg {
		sample, _, err := mix()
		return slicerBedMsg{slot: slot, sample: sample, bpm: bpm, err: err, remix: remix}
	}
}

// slicerRemix mixes the pattern under the slices again, as the pads or the pattern are now. It
// is asked for whenever a looping pattern would be: the pads were read again after a bounce, a
// pad's setting changed, the pattern was edited.
func (m Model) slicerRemix() tea.Cmd {
	s := m.slicer
	if s == nil || s.bed == nil || m.unit == nil || m.snap == nil {
		return nil
	}
	return m.slicerMixBed(s.bed.slot, true)
}

// chooserLines draw the chooser: a heading, then the 16 slots as the pads lie, four to a row.
func (m Model) chooserLines(width int) []string {
	st, s := m.st, m.slicer
	c := s.chooser
	lines := []string{st.bold.Render(fmt.Sprintf("Pattern  bank %c", 'A'+c.bank)) + st.dim.Render("   arrows move  [ ] bank  Enter take it  0 none  Esc leave")}
	cell := max((width-3)/4, 8)
	for row := 0; row < 4; row++ {
		var b strings.Builder
		for col := 0; col < 4; col++ {
			i := row*4 + col
			r := c.rows[i]
			text := fmt.Sprintf("%2d ·", i+1)
			if r.Exists {
				var pads []string
				for _, p := range r.Pads {
					pads = append(pads, p.String())
				}
				text = fmt.Sprintf("%2d %s", i+1, strings.Join(pads, " "))
			}
			if s.bed != nil && s.bed.slot == r.Slot {
				text += " ◀"
			}
			text = ansi.Truncate(text, cell-1, "…")
			text += strings.Repeat(" ", max(cell-ansi.StringWidth(text), 0))
			switch {
			case i == c.cursor:
				text = lipgloss.NewStyle().Background(st.th.accent).Foreground(st.th.cursorFg).Bold(true).Render(text)
			case r.Exists:
				text = st.text.Render(text)
			default:
				text = st.faint.Render(text)
			}
			b.WriteString(text)
		}
		lines = append(lines, b.String())
	}
	return lines
}

// slicerBedMixed puts a mixed pattern under the slices: the grid takes the pattern's tempo
// (and a 1/16 division when there was none), and the next round plays both.
func (m Model) slicerBedMixed(msg slicerBedMsg) (tea.Model, tea.Cmd) {
	s := m.slicer
	if s == nil {
		return m, nil
	}
	if msg.remix {
		// Only if that pattern is still the one under the slices; the grid stays as it is.
		if s.bed == nil || s.bed.slot != msg.slot || msg.err != nil {
			return m, nil
		}
		s.bed = &patternBed{slot: msg.slot, sample: msg.sample, bpm: msg.bpm}
		s.sl.BPM = msg.bpm
		s.settle(m)
		return m, nil
	}
	if msg.err != nil {
		m.message = "pattern " + msg.slot.String() + ": " + msg.err.Error()
		return m, nil
	}
	s.bed = &patternBed{slot: msg.slot, sample: msg.sample, bpm: msg.bpm}
	s.sl.BPM = msg.bpm
	if s.sl.Division == 0 {
		s.sl.Division = 16
	}
	s.settle(m)
	m.message = fmt.Sprintf("pattern %s plays with the slices at %g BPM; the tempo follows the pattern", msg.slot, msg.bpm)
	return m, nil
}

// withBed adds the pattern, repeated passes times, to the slices' audio: the result is exactly
// the bed long, in stereo; what the slices play past the end is cut, and a gap is silence.
func withBed(slices, bed formats.Sample, passes int) formats.Sample {
	total := bed.Frames() * passes
	mix := make([]int32, total*2)
	ch := max(slices.Channels, 1)
	for f := 0; f < min(slices.Frames(), total); f++ {
		l := int32(slices.Samples[f*ch])
		r := l
		if ch > 1 {
			r = int32(slices.Samples[f*ch+1])
		}
		mix[f*2], mix[f*2+1] = l, r
	}
	bch := max(bed.Channels, 1)
	for f := 0; f < total; f++ {
		b := f % bed.Frames()
		l := int32(bed.Samples[b*bch])
		r := l
		if bch > 1 {
			r = int32(bed.Samples[b*bch+1])
		}
		mix[f*2] += l
		mix[f*2+1] += r
	}
	out := formats.Sample{Channels: 2, Samples: make([]int16, len(mix))}
	for i, v := range mix {
		out.Samples[i] = int16(max(min(v, 32767), -32768))
	}
	return out
}

// previewSlot is the pattern to draw under the slicer: the one the chooser is on, else the one
// under the slices.
func (s *slicer) previewSlot() (proto.PadIndex, bool) {
	if c := s.chooser; c != nil {
		if r := c.rows[c.cursor]; r.Exists {
			return r.Slot, true
		}
		return proto.PadIndex{}, false
	}
	if s.bed != nil {
		return s.bed.slot, true
	}
	return proto.PadIndex{}, false
}

// bedFraction is how far through its loop the pattern under the slices is while they play,
// from 0 to 1: a round is whole loops of the pattern, so it is the time in the round, in loops.
func (s *slicer) bedFraction(d *app.PatternDetail) (float64, bool) {
	if s.bed == nil || !s.playing || s.cur == nil || s.cur.start.IsZero() || s.bed.slot != d.Slot {
		return 0, false
	}
	loop := d.TicksToMillis(s.bedTicks(d)) / 1000
	if loop <= 0 {
		return 0, false
	}
	return math.Mod(time.Since(s.cur.start).Seconds()/loop, 1), true
}

// bedTicks is how long the pattern under the slices loops: the bars of its range, or all of it.
func (s *slicer) bedTicks(d *app.PatternDetail) int {
	if r := s.bedRange; r != nil && r.slot == d.Slot && r.last < d.Bars() {
		return min(r.bars()*d.Beats*int(formats.PPQ), int(d.Length))
	}
	return int(d.Length)
}

// slicerBarsKey is V: the pattern under the slices is limited to one bar, two bars, then the
// whole pattern again, as v does in the pattern editor. The slices follow the shorter loop.
func (m Model) slicerBarsKey() (tea.Model, tea.Cmd) {
	s := m.slicer
	if s.bed == nil {
		m.message = "V limits the pattern to some of its bars: P puts one under the slices first"
		return m, nil
	}
	d := m.patPreview[s.bed.slot]
	if d == nil {
		m.message = "the pattern is being read…"
		return m, nil
	}
	items := []*barRange{nil}
	for b := range d.Bars() {
		items = append(items, &barRange{slot: d.Slot, first: b, last: b})
	}
	for b := 0; b+1 < d.Bars(); b++ {
		items = append(items, &barRange{slot: d.Slot, first: b, last: b + 1})
	}
	at := 0
	for i, it := range items {
		if (it == nil && s.bedRange == nil) || (it != nil && s.bedRange != nil && *it == *s.bedRange) {
			at = i
		}
	}
	s.bedRange = items[(at+1)%len(items)]
	m.message = "the whole pattern is under the slices"
	if s.bedRange != nil {
		m.message = s.bedRange.label() + " of the pattern only, looped under the slices  ·  V for the next"
	}
	return m, m.slicerMixBed(s.bed.slot, true)
}

// slicerPatternLines draws the pattern as the pattern editor does, its steps and a row for each
// pad, with a playhead going round while the slices play: at most room lines, none when that is
// too few or the pattern is not read yet.
func (m Model) slicerPatternLines(width, room int) []string {
	s := m.slicer
	slot, ok := s.previewSlot()
	if !ok || room < 5 {
		return nil
	}
	d := m.patPreview[slot]
	if d == nil {
		if m.patUnread[slot] {
			return []string{m.st.faint.Render("pattern " + slot.String() + " cannot be shown")}
		}
		return []string{m.st.faint.Render("reading pattern " + slot.String() + "…")}
	}
	st := m.st
	frac, playing := s.bedFraction(d)
	// Bars of the range: shown whole, the playhead goes round them.
	first, nBars := 0, d.Bars()
	if r := s.bedRange; r != nil && r.slot == slot && r.last < d.Bars() {
		first, nBars = r.first, r.bars()
	}
	spp := d.StepsPerPage()
	now := first*spp + min(int(frac*float64(nBars*spp)), nBars*spp-1)
	page := first
	if playing && nBars == d.Bars() {
		page = now / spp
	}
	var sp stepSpan
	if nBars < d.Bars() {
		sp = spanBars(d, first, nBars, width, 1, 4, gridGapW)
	} else {
		sp = spanFor(d, page, width, 1, 4, gridGapW)
	}
	tracks := len(d.Grid.Rows)
	shown := min(tracks, max(room-4, 1))
	head := st.bold.Render("Pattern "+slot.String()) +
		st.dim.Render(fmt.Sprintf("  %.2f BPM  ·  %d/4  ·  %s  ·  %s", d.BankTempo, d.Beats, plural(d.Bars(), "bar"), plural(len(d.PadsUsed), "pad")))
	lines := []string{head + "  " + m.spanHeader(d, sp), m.stepRuler(d, sp)}
	if playing {
		lines = append(lines, m.headLine(d, sp, now, fmt.Sprintf("▶ %.1f s to go", (1-frac)*d.TicksToMillis(s.bedTicks(d))/1000)))
	} else {
		lines = append(lines, "")
	}
	if tracks == 0 {
		lines = append(lines, st.dim.Render("  (no notes)"))
	}
	for _, row := range d.Grid.Rows[:shown] {
		name := m.snap.Pads[row.Pad.Index()].Name
		label := fmt.Sprintf("%-4s %-14s", row.Pad, ansi.Truncate(name, 14, "…"))
		lines = append(lines, m.rowLabel(row.Pad, label, st.bold)+" "+m.noteRow(d, row.Pad, false, sp))
	}
	if shown < tracks {
		lines = append(lines, st.faint.Render(fmt.Sprintf("  … and %s more", plural(tracks-shown, "track"))))
	}
	return lines
}
