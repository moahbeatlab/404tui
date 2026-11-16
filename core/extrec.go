package core

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
)

// Recording notes from another device (an MC-101 over USB): the program sends that device a MIDI clock at the pattern's tempo and a
// Start, the device plays its clips and sends their notes back on its track channels (11 to 14 by default), and after the bars that
// were asked for the program sends Stop. The take is saved as a .mid file and opens in the MIDI import form, where a track of it
// becomes a pattern on a pad (the drums track by the drum map, the others as melodies).

const (
	defaultExtPort  = "mc-101"
	defaultExtChans = "11-14"
	defaultExtDrums = 11
	// extTail is how long after the last bar the take still takes notes in (a note-off of the last note that comes a bit late).
	extTail = 60 * time.Millisecond
)

var (
	extBarVals  = []int{1, 2, 4, 8, 16}
	extSnapVals = []int{0, 16, 32} // off, 1/16, 1/32
)

func (a *App) extPort() string {
	if a.set.ExtPort != "" {
		return a.set.ExtPort
	}
	return defaultExtPort
}

func (a *App) extChansText() string {
	if a.set.ExtChans != "" {
		return a.set.ExtChans
	}
	return defaultExtChans
}

// extChans are the channels the take listens to, 1..16, in order.
func (a *App) extChans() []int {
	cs, err := parseChans(a.extChansText())
	if err != nil {
		cs, _ = parseChans(defaultExtChans)
	}
	return cs
}

// extDrums is the channel whose notes are drums (they go into the file on channel 10, so the import maps them onto pads), 0 for none.
func (a *App) extDrums() int { return channelOf(a.set.ExtDrums, defaultExtDrums) }

func (a *App) extBars() int {
	if a.set.ExtBars > 0 {
		return a.set.ExtBars
	}
	return 4
}

// parseChans reads "11-14", "11 12 14" or "11,13": channels 1 to 16.
func parseChans(s string) ([]int, error) {
	seen := map[int]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		lo, hi, isRange := strings.Cut(f, "-")
		from, err1 := strconv.Atoi(lo)
		to := from
		var err2 error
		if isRange {
			to, err2 = strconv.Atoi(hi)
		}
		if err1 != nil || err2 != nil || from < 1 || to > 16 || from > to {
			return nil, fmt.Errorf("channels are 1 to 16, as 11-14 or 11 12 13")
		}
		for c := from; c <= to; c++ {
			seen[c] = true
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("no channel given")
	}
	var out []int
	for c := range seen {
		out = append(out, c)
	}
	sort.Ints(out)
	return out, nil
}

// take is the notes of one recording, timed from the first clock the program sent (the first beat of the first bar).
type take struct {
	bpm   float64
	bars  int
	beats int
	snap  int // notes start on a grid of 1/snap notes, 0 for as they came
	chans map[int]bool
	start time.Time
	open  map[[2]int]int             // channel and note: the index of the note in notes[channel] that has not ended yet
	notes map[int][]formats.MIDINote // by channel
	count int
}

func newTake(bpm float64, bars, snap int, chans []int, start time.Time) *take {
	t := &take{bpm: bpm, bars: bars, beats: 4, snap: snap, chans: map[int]bool{}, start: start, open: map[[2]int]int{}, notes: map[int][]formats.MIDINote{}}
	for _, c := range chans {
		t.chans[c] = true
	}
	return t
}

// length is the ticks of the take (PPQ 480).
func (t *take) length() uint32 { return uint32(t.bars * t.beats * formats.PPQ) }

// tickAt is the tick of a moment of the take.
func (t *take) tickAt(at time.Time) int64 {
	return int64(math.Round(at.Sub(t.start).Seconds() * t.bpm / 60 * formats.PPQ))
}

// duration is how long the take runs.
func (t *take) duration() time.Duration {
	return time.Duration(float64(t.length()) / formats.PPQ * 60 / t.bpm * float64(time.Second))
}

// feed takes in a message: a note on starts a note, a note off ends it. Notes before the start count as on the first tick (the device
// may be a little early), notes after the end are left out.
func (t *take) feed(e midiin.Event) {
	if !t.chans[e.Channel] || (e.Kind != midiin.NoteOn && e.Kind != midiin.NoteOff) {
		return
	}
	tick := max(t.tickAt(e.At), 0)
	key := [2]int{e.Channel, e.Data1}
	if i, ok := t.open[key]; ok { // the note ends (or starts again: the one before ends here)
		n := &t.notes[e.Channel][i]
		n.Len = uint32(max(tick-int64(n.Tick), 1))
		delete(t.open, key)
	}
	if e.Kind != midiin.NoteOn {
		return
	}
	start := tick
	if t.snap > 0 {
		grid := int64(formats.PPQ * 4 / t.snap)
		start = (tick + grid/2) / grid * grid
	}
	if start >= int64(t.length()) {
		return
	}
	t.notes[e.Channel] = append(t.notes[e.Channel], formats.MIDINote{Tick: uint32(start), Note: uint8(e.Data1), Vel: uint8(max(e.Data2, 1))})
	t.open[key] = len(t.notes[e.Channel]) - 1
	t.count++
}

// finish ends the notes still held at the end of the take, and writes it as a MIDI file: a track for each channel that played, the
// drums channel as channel 10.
func (t *take) finish(drums int) []byte {
	for key, i := range t.open {
		n := &t.notes[key[0]][i]
		n.Len = max(t.length()-n.Tick, 1)
	}
	t.open = map[[2]int]int{}
	var chans []int
	for c := range t.notes {
		chans = append(chans, c)
	}
	sort.Ints(chans)
	var tracks []formats.MIDIOut
	for _, c := range chans {
		ch := uint8(c - 1)
		name := fmt.Sprintf("ch %d", c)
		if c == drums {
			ch, name = 9, fmt.Sprintf("drums ch %d", c)
		}
		for i := range t.notes[c] {
			if n := &t.notes[c][i]; n.Tick+n.Len > t.length() { // a note does not run past the end of the take
				n.Len = t.length() - n.Tick
			}
		}
		tracks = append(tracks, formats.MIDIOut{Name: name, Channel: ch, Notes: t.notes[c]})
	}
	return formats.WriteMIDI(tracks, t.bpm, t.beats, t.length())
}

// extRun is a recording that runs: the device's input and output and the take.
type extRun struct {
	take    *take
	ch      chan midiin.Event
	stopIn  func()         // closes the input, nil when the program's own MIDI input is the device's
	quit    chan struct{}  // ends the clock (which then sends Stop and closes the output)
	clockOK chan time.Time // the moment of the first clock: the take starts there
	done    chan struct{}  // the clock has stopped
	name    string
	m       *menu
}

// recordExtMenu is the form of a recording: how many bars, the grid, and start.
func (a *App) recordExtMenu() {
	if a.plat.MIDI == nil {
		a.say("no MIDI in this build: recording from "+a.extPort()+" needs it", true)
		return
	}
	m := &menu{}
	a.extIdle(m)
	m.onClose = func() {
		if a.ext != nil {
			a.stopExt(true)
		}
	}
	a.menu = m
	a.say("A on Start sends the clock and Start to "+a.extPort()+"; Esc closes", false)
}

// extIdle is the form before a take (and again after one: Esc in the import form comes back here, to record again).
func (a *App) extIdle(m *menu) {
	m.title = "record notes from " + strings.ToUpper(a.extPort())
	bars := make([]string, len(extBarVals))
	cur := 2
	for i, b := range extBarVals {
		bars[i] = fmt.Sprint(b)
		if b == a.extBars() {
			cur = i
		}
	}
	snap := 0
	for i, s := range extSnapVals {
		if s == a.set.ExtSnap {
			snap = i
		}
	}
	m.items = []menuItem{
		{label: "Length in bars", vals: bars, val: cur, keepOpen: true, onChange: func(v int) { a.set.ExtBars = extBarVals[v]; a.saveSettings() }},
		{label: "Snap the notes to", vals: []string{"off: as they come", "1/16", "1/32"}, val: snap, keepOpen: true,
			onChange: func(v int) { a.set.ExtSnap = extSnapVals[v]; a.saveSettings() }},
		{label: fmt.Sprintf("Tempo: %s BPM (the pattern's), sent as the clock", bpmText(a.tempo())), header: true},
		{label: fmt.Sprintf("Channels %s, drums on %s", a.extChansText(), chanText(a.extDrums())), header: true},
		{label: "Start: the device plays, the notes are recorded", keepOpen: true, run: func(int) { a.startExt(m) }},
	}
	m.cur = len(m.items) - 1
}

// startExt opens the device's output (and its input, unless it is the one the program listens to), and starts the clock.
func (a *App) startExt(m *menu) {
	if a.ext != nil {
		return
	}
	outs, ok := a.plat.MIDI.(midiin.Outputs)
	if !ok {
		a.say("this build cannot send MIDI", true)
		return
	}
	out, outName, err := outs.OpenOut(a.extPort())
	if err != nil {
		a.say("no MIDI output with "+a.extPort()+" in its name: "+err.Error(), true)
		return
	}
	r := &extRun{ch: make(chan midiin.Event, 1024), quit: make(chan struct{}), clockOK: make(chan time.Time, 1), done: make(chan struct{}), name: outName, m: m}
	if a.midi == nil || !strings.Contains(strings.ToLower(a.midi.name), strings.ToLower(a.extPort())) {
		stop, _, err := a.plat.MIDI.Open(a.extPort(), func(e midiin.Event) {
			e.At = time.Now()
			select {
			case r.ch <- e:
			default:
			}
		})
		if err != nil {
			out.Close()
			a.say("no MIDI input with "+a.extPort()+" in its name: "+err.Error(), true)
			return
		}
		r.stopIn = stop
	}
	bpm := a.tempo()
	if bpm <= 0 {
		bpm = 120
	}
	r.take = newTake(bpm, a.extBars(), a.set.ExtSnap, a.extChans(), time.Time{})
	a.ext = r
	go runClock(out, bpm, r.quit, r.clockOK, r.done)
	m.items = []menuItem{
		{label: "Stop now and keep the notes", run: func(int) { a.endExt() }},
		{label: "Throw the take away", run: func(int) { a.stopExt(true) }},
	}
	m.cur = 0
	a.say("sent Start to "+outName, false)
}

// runClock sends Start and then the clock, 24 to a quarter note, each on its own time from the first (so it does not drift), until
// quit is closed; then Stop, and the output is closed.
func runClock(out midiin.Out, bpm float64, quit chan struct{}, first chan<- time.Time, done chan struct{}) {
	defer close(done)
	defer out.Close()
	tick := time.Duration(float64(time.Minute) / bpm / 24)
	_ = out.Send([]byte{0xF2, 0, 0}) // the song position: the start
	_ = out.Send([]byte{0xFA})
	start := time.Now()
	_ = out.Send([]byte{0xF8})
	first <- start
	for i := 1; ; i++ {
		next := start.Add(time.Duration(i) * tick)
		select {
		case <-quit:
			_ = out.Send([]byte{0xFC})
			return
		case <-time.After(time.Until(next)):
			_ = out.Send([]byte{0xF8})
		}
	}
}

// extTick takes in the notes that came, shows how far the take is, and ends it after its bars.
func (a *App) extTick() {
	r := a.ext
	if r == nil {
		return
	}
	if r.take.start.IsZero() {
		select {
		case s := <-r.clockOK:
			r.take.start = s
		default:
			return
		}
	}
	for i := 0; i < 1024; i++ {
		select {
		case e := <-r.ch:
			r.take.feed(e)
			continue
		default:
		}
		break
	}
	el := time.Since(r.take.start)
	bar := min(int(el/(r.take.duration()/time.Duration(r.take.bars)))+1, r.take.bars)
	r.m.title = fmt.Sprintf("REC %s  bar %d of %d  %d notes", strings.ToUpper(a.extPort()), bar, r.take.bars, r.take.count)
	if el >= r.take.duration()+extTail {
		a.endExt()
	}
}

// extFeed is a message of the program's own input while a take runs on it (the program listens to the device anyway).
func (a *App) extFeed(e midiin.Event) {
	if a.ext != nil && a.ext.stopIn == nil {
		select {
		case a.ext.ch <- e:
		default:
		}
	}
}

// stopExt stops the clock (the device gets Stop) and closes the input; with drop the take is thrown away.
func (a *App) stopExt(drop bool) {
	r := a.ext
	if r == nil {
		return
	}
	a.ext = nil
	close(r.quit)
	<-r.done
	if r.stopIn != nil {
		r.stopIn()
	}
	a.extIdle(r.m)
	if drop {
		a.menu = nil
		a.say("the take is thrown away", false)
	}
}

// endExt ends the take, saves it as a .mid file in the exports and opens it in the MIDI import form.
func (a *App) endExt() {
	r := a.ext
	if r == nil {
		return
	}
	a.extTick() // what has come in so far
	a.stopExt(false)
	a.menu = nil
	if r.take.count == 0 {
		a.say(fmt.Sprintf("no notes came on channels %s: is Tx USB on for the device's tracks?", a.extChansText()), true)
		return
	}
	b := r.take.finish(a.extDrums())
	name := "take-" + time.Now().Format("20060102-150405") + ".mid"
	dir := a.exportDir()
	if err := os.MkdirAll(dir, 0o755); err == nil {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			a.say(err.Error(), true)
		}
	}
	a.startMIDIBatch([]midiItem{{name: name, data: b}}, 1)
}
