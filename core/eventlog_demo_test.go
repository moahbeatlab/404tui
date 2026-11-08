package core

import (
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/record"
)

// TestWriteReplayDemo writes a made-up performance of a project (REPLAY_DEMO=folder REPLAY_PROJECT=project folder): a WAV and the event log
// beside it, for looking at `sp404 replay` without a unit. It does nothing unless REPLAY_DEMO is set.
func TestWriteReplayDemo(t *testing.T) {
	out := os.Getenv("REPLAY_DEMO")
	proj := os.Getenv("REPLAY_PROJECT")
	if out == "" || proj == "" {
		t.Skip("REPLAY_DEMO and REPLAY_PROJECT are not set")
	}
	termOn(t)
	a, _ := newApp(t)
	if err := a.OpenProject(proj); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(out, 0o755)
	const rate = 48000
	secs := 40.0
	bpm := 124.0
	started := time.Now()
	s := &record.Session{Path: filepath.Join(out, "demo.wav"), Started: started, Tool: "the program itself"}
	a.startEventLog(s)
	at := func(sec float64) time.Time { return started.Add(time.Duration(sec * float64(time.Second))) }
	a.evlog.log.Add(at(0.02), "audio_start")
	t0 := 1.0
	beat := 60 / bpm
	a.midiEvent(midiin.Event{Kind: midiin.Start, At: at(t0)})
	// a pattern: the first one that has notes
	d, ok := a.lab.Pattern(1)
	if ok != nil {
		t.Skip("no pattern 1 in the project")
	}
	_ = d
	a.pat = 1
	doc, _ := a.doc()
	a.perfNow = 1
	a.eventLogTick()
	type hit struct {
		t   float64
		pad int
		v   int
		d   float64
	}
	var hits []hit
	for bar := 0; float64(bar)*4*beat+t0 < secs-1; bar++ {
		for _, n := range doc.Notes {
			tt := t0 + float64(bar)*float64(doc.Length)/480*beat + float64(n.Tick)/480*beat
			if tt < secs-1 {
				hits = append(hits, hit{tt, n.Pad, n.Vel, float64(n.Len) / 480 * beat})
			}
		}
	}
	for q := 0; float64(q)*beat+t0 < secs-0.5; q++ { // 24 clock ticks to a quarter note
		for k := 0; k < 24; k++ {
			a.midiEvent(midiin.Event{Kind: midiin.Clock, At: at(t0 + float64(q)*beat + float64(k)*beat/24)})
		}
	}
	for _, h := range hits {
		bank, pad := h.pad/16, h.pad%16
		note := []int{48, 49, 50, 51, 44, 45, 46, 47, 40, 41, 42, 43, 36, 37, 38, 39}[pad]
		a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: bank + 1, Data1: note, Data2: h.v, At: at(h.t)})
		a.midiEvent(midiin.Event{Kind: midiin.NoteOff, Channel: bank + 1, Data1: note, At: at(h.t + math.Min(h.d, 0.4))})
	}
	// effects: bus 1 a tape echo with a knob that is turned, bus 2 a reverb
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 83, Data2: 14, At: at(2)})
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 83, Data2: 22, At: at(2)})
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 19, Data2: 127, At: at(8)})
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 19, Data2: 0, At: at(28)})
	a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 19, Data2: 127, At: at(14)})
	for tt := 8.0; tt < 28; tt += 0.08 {
		a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 16, Data2: int(64 + 60*math.Sin(tt*0.7)), At: at(tt)})
		a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 1, Data1: 17, Data2: int(40 + 40*math.Sin(tt*0.25+1)), At: at(tt)})
	}
	for tt := 14.0; tt < 34; tt += 0.1 {
		a.midiEvent(midiin.Event{Kind: midiin.CC, Channel: 2, Data1: 18, Data2: int(clampInt(int(tt*3)-30, 0, 127)), At: at(tt)})
	}
	a.midiEvent(midiin.Event{Kind: midiin.Stop, At: at(secs - 0.5)})
	a.stopEventLog()
	// the sound: a click and a tone for every hit, with a little noise
	n := int(secs * rate)
	pcm := make([]float32, n*2)
	rng := rand.New(rand.NewSource(1))
	for _, h := range hits {
		start := int(h.t * rate)
		freq := 50 + float64(h.pad%16)*40
		for i := 0; i < rate/3 && start+i < n; i++ {
			env := math.Exp(-float64(i) / (rate * 0.06))
			v := float32(float64(h.v) / 127 * 0.5 * env * (math.Sin(2*math.Pi*freq*float64(i)/rate) + 0.2*(rng.Float64()-0.5)))
			pcm[(start+i)*2] += v
			pcm[(start+i)*2+1] += v * 0.9
		}
	}
	f, _ := os.Create(s.Path)
	defer f.Close()
	f.Write(demoHeader(n * 2 * 3))
	for _, v := range pcm {
		x := int32(math.Max(-1, math.Min(1, float64(v))) * 8388607)
		f.Write([]byte{byte(x), byte(x >> 8), byte(x >> 16)})
	}
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }

func demoHeader(data int) []byte {
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+data))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], 48000)
	binary.LittleEndian.PutUint32(h[28:], 48000*6)
	binary.LittleEndian.PutUint16(h[32:], 6)
	binary.LittleEndian.PutUint16(h[34:], 24)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(data))
	return h
}

// TestWriteReplayArpDemo writes a made-up take of an arpeggio (REPLAY_ARP_DEMO=folder REPLAY_PROJECT=project folder): one sample (pad A3) played at different
// pitches, in loops that come round again and again, with the sound, the captions and the chapters. It is the take to open to see the ARP view of the replay
// (`sp404 replay arp.wav`). It does nothing unless REPLAY_ARP_DEMO is set.
func TestWriteReplayArpDemo(t *testing.T) {
	out := os.Getenv("REPLAY_ARP_DEMO")
	proj := os.Getenv("REPLAY_PROJECT")
	if out == "" || proj == "" {
		t.Skip("REPLAY_ARP_DEMO and REPLAY_PROJECT are not set")
	}
	termOn(t)
	a, _ := newApp(t)
	if err := a.OpenProject(proj); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(out, 0o755)
	const rate = 48000
	secs := 44.0
	bpm := 124.0
	started := time.Now()
	s := &record.Session{Path: filepath.Join(out, "arp.wav"), Started: started, Tool: "the program itself"}
	a.startEventLog(s)
	at := func(sec float64) time.Time { return started.Add(time.Duration(sec * float64(time.Second))) }
	a.evlog.log.Add(at(0.02), "audio_start")
	t0 := 1.0
	beat := 60 / bpm
	a.midiEvent(midiin.Event{Kind: midiin.Start, At: at(t0)})
	a.eventLogTick()
	for q := 0; float64(q)*beat+t0 < secs-0.5; q++ {
		for k := 0; k < 24; k++ {
			a.midiEvent(midiin.Event{Kind: midiin.Clock, At: at(t0 + float64(q)*beat + float64(k)*beat/24)})
		}
	}
	// the loops: a pad of the bank is a semitone above the sample's own pitch (the chromatic mode), a step comes every `step` quarter notes
	type loop struct {
		from, to, step float64
		gate           float64 // how long a note sounds, as a part of the step
		semis          []int
	}
	loops := []loop{
		{3, 12, 0.25, 0.5, []int{0, 4, 7, 12, 7, 4}},          // a major triad up and down, in 16ths
		{13, 22, 0.5, 0.9, []int{0, 3, 7, 10, 12, 10, 7, 3}},  // a minor seventh up and down, in 8ths
		{23, 32, 0.25, 0.25, []int{0, 2, 4, 5, 7, 9, 11, 12}}, // a scale up, in 16ths
		{33, 43, 0.25, 1.0, []int{0, 7, 12, 7, 3, 7, 12, 15}}, // wider, ending on the minor third
	}
	type hit struct {
		t, d float64
		semi int
		v    int
	}
	var hits []hit
	for _, l := range loops {
		k := 0
		for tt := l.from; tt < l.to; tt += l.step * beat {
			v := 96
			if k%len(l.semis) == 0 {
				v = 122 // the first step of a round is accented
			}
			hits = append(hits, hit{tt, l.gate * l.step * beat, l.semis[k%len(l.semis)], v})
			k++
		}
	}
	for _, h := range hits {
		note := []int{48, 49, 50, 51, 44, 45, 46, 47, 40, 41, 42, 43, 36, 37, 38, 39}[h.semi]
		a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 1, Data1: note, Data2: h.v, At: at(h.t)})
		a.midiEvent(midiin.Event{Kind: midiin.NoteOff, Channel: 1, Data1: note, At: at(h.t + h.d)})
	}
	a.midiEvent(midiin.Event{Kind: midiin.Stop, At: at(secs - 0.5)})
	a.stopEventLog()
	// the sound: one pluck, played at the pitch of each step (2^(n/12) above 196 Hz)
	n := int(secs * rate)
	pcm := make([]float32, n*2)
	for _, h := range hits {
		start, f := int(h.t*rate), 196*math.Pow(2, float64(h.semi)/12)
		for i := 0; i < rate/3 && start+i < n; i++ {
			x := float64(i) / rate
			env := math.Exp(-x*14) * math.Min(1, math.Max(0, (h.d+0.012-x)/0.012)) // the gate cuts the note short
			v := float32(float64(h.v) / 127 * 0.35 * env * (math.Sin(2*math.Pi*f*x) + 0.4*math.Sin(4*math.Pi*f*x) + 0.2*math.Sin(6*math.Pi*f*x)))
			pcm[(start+i)*2] += v
			pcm[(start+i)*2+1] += v
		}
	}
	f, _ := os.Create(s.Path)
	defer f.Close()
	f.Write(demoHeader(n * 2 * 3))
	for _, v := range pcm {
		x := int32(math.Max(-1, math.Min(1, float64(v))) * 8388607)
		f.Write([]byte{byte(x), byte(x >> 8), byte(x >> 16)})
	}
	// the captions and chapters, and the view: the split view with the arp
	meta := `{"title":"One sample, many pitches","patterns_adopted":true,"cues":[],
 "captions":[{"from":3.5,"to":9.5,"text":"One *sample* on pad A3, played at different pitches"},
  {"from":13.5,"to":19.5,"text":"The same *loop* comes round again and again, a long *gate*"},
  {"from":23.5,"to":29.5,"text":"A *scale* up, step by step, short *gate*"},
  {"from":33.5,"to":40,"text":"Wider *steps*, tied: the *gate* is open all the way"}],
 "chapters":[{"t":1,"title":"Triad"},{"t":13,"title":"Minor seventh"},{"t":23,"title":"Scale"},{"t":33,"title":"Wide"}],
 "style":{"view":"split","rmode":"arp","chroma":"A3"}}`
	if err := os.WriteFile(filepath.Join(out, "arp.events.cues.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWriteReplayMidiArpDemo writes a made-up take of the arpeggiator playing another instrument (REPLAY_MIDI_DEMO=folder REPLAY_PROJECT=project folder): MIDI
// notes on channel 3 in four loops with their own mode, octaves, rate, gate and swing. Open it with `sp404 replay midiarp.wav` and the key m (the MIDI view).
func TestWriteReplayMidiArpDemo(t *testing.T) {
	out := os.Getenv("REPLAY_MIDI_DEMO")
	proj := os.Getenv("REPLAY_PROJECT")
	if out == "" || proj == "" {
		t.Skip("REPLAY_MIDI_DEMO and REPLAY_PROJECT are not set")
	}
	termOn(t)
	a, _ := newApp(t)
	if err := a.OpenProject(proj); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(out, 0o755)
	const rate = 48000
	secs := 52.0
	bpm := 124.0
	started := time.Now()
	s := &record.Session{Path: filepath.Join(out, "midiarp.wav"), Started: started, Tool: "the program itself"}
	a.startEventLog(s)
	at := func(sec float64) time.Time { return started.Add(time.Duration(sec * float64(time.Second))) }
	a.evlog.log.Add(at(0.02), "audio_start")
	t0, beat := 1.0, 60/bpm
	a.midiEvent(midiin.Event{Kind: midiin.Start, At: at(t0)})
	a.eventLogTick()
	for q := 0; float64(q)*beat+t0 < secs-0.5; q++ {
		for k := 0; k < 24; k++ {
			a.midiEvent(midiin.Event{Kind: midiin.Clock, At: at(t0 + float64(q)*beat + float64(k)*beat/24)})
		}
	}
	// a C minor seventh chord (C Eb G Bb) from C4, spread over some octaves, played in a mode
	chord := func(oct int) []int {
		var out []int
		for o := 0; o < oct; o++ {
			for _, n := range []int{0, 3, 7, 10} {
				out = append(out, 60+12*o+n)
			}
		}
		return append(out, 60+12*oct)
	}
	reverse := func(x []int) []int {
		r := make([]int, len(x))
		for i, v := range x {
			r[len(x)-1-i] = v
		}
		return r
	}
	upDown := func(x []int) []int { return append(append([]int{}, x...), reverse(x)[1:len(x)-1]...) }
	type loop struct {
		from, to, step, gate, swing float64
		notes                       []int
	}
	loops := []loop{
		{3, 15, 0.25, 0.6, 0.5, chord(1)},                                // up, 1 octave, 1/16, straight
		{16, 28, 0.25, 0.9, 0.5, upDown(chord(2))},                       // up-down, 2 octaves, long gate
		{29, 41, 0.5, 0.3, 0.6, reverse(chord(3))},                       // down, 3 octaves, 1/8, short gate, swing 60 %
		{42, 51, 0.25, 0.5, 0.58, []int{60, 67, 63, 72, 70, 63, 67, 75}}, // a pattern, swing 58 %
	}
	type hit struct {
		t, d float64
		n, v int
	}
	var hits []hit
	for _, l := range loops {
		k := 0
		for tt := l.from; tt < l.to; tt += l.step * beat {
			tk := tt
			if k%2 == 1 {
				tk += (l.swing - 0.5) * 2 * l.step * beat
			}
			v := 92
			if k%len(l.notes) == 0 {
				v = 120
			}
			hits = append(hits, hit{tk, l.gate * l.step * beat, l.notes[k%len(l.notes)], v})
			k++
		}
	}
	for _, h := range hits {
		a.midiEvent(midiin.Event{Kind: midiin.NoteOn, Channel: 3, Data1: h.n, Data2: h.v, At: at(h.t)})
		a.midiEvent(midiin.Event{Kind: midiin.NoteOff, Channel: 3, Data1: h.n, At: at(h.t + h.d)})
	}
	a.midiEvent(midiin.Event{Kind: midiin.Stop, At: at(secs - 0.5)})
	a.stopEventLog()
	n := int(secs * rate)
	pcm := make([]float32, n*2)
	for _, h := range hits {
		start, f := int(h.t*rate), 440*math.Pow(2, float64(h.n-69)/12)
		for i := 0; i < rate/2 && start+i < n; i++ {
			x := float64(i) / rate
			env := math.Exp(-x*9) * math.Min(1, math.Max(0, (h.d+0.012-x)/0.012))
			ph := math.Mod(f*x, 1)
			v := float32(float64(h.v) / 127 * 0.22 * env * (2*ph - 1))
			pcm[(start+i)*2] += v
			pcm[(start+i)*2+1] += v
		}
	}
	f, _ := os.Create(s.Path)
	defer f.Close()
	f.Write(demoHeader(n * 2 * 3))
	for _, v := range pcm {
		x := int32(math.Max(-1, math.Min(1, float64(v))) * 8388607)
		f.Write([]byte{byte(x), byte(x >> 8), byte(x >> 16)})
	}
	meta := `{"title":"The arp sends MIDI","patterns_adopted":true,"cues":[],
 "captions":[{"from":3.5,"to":9.5,"text":"*Up*, one octave, 16ths"},
  {"from":16.5,"to":23,"text":"*Up-down* over two octaves, a long *gate*"},
  {"from":29.5,"to":36,"text":"*Down* over three octaves, 8ths, short *gate* and *swing*"},
  {"from":42.5,"to":49,"text":"A *pattern* of its own, with swing"}],
 "chapters":[{"t":1,"title":"Up"},{"t":16,"title":"Up-down"},{"t":29,"title":"Down"},{"t":42,"title":"Pattern"}],
 "style":{"view":"split","rmode":"midi"}}`
	if err := os.WriteFile(filepath.Join(out, "midiarp.events.cues.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}
