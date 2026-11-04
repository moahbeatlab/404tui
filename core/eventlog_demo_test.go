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
