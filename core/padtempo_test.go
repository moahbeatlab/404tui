package core

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// houseLoop is bars of a kick on every beat and a hat on every offbeat.
func houseLoop(bpm float64, bars int) formats.Sample {
	beat := 60 / bpm * formats.SampleRate
	n := int(float64(bars*4) * beat)
	s := formats.Sample{Channels: 1, Samples: make([]int16, n)}
	r := rand.New(rand.NewSource(1))
	for i := range s.Samples {
		s.Samples[i] = int16(r.NormFloat64() * 300)
	}
	for b := 0; b < bars*4; b++ {
		at := int(math.Round(float64(b) * beat))
		for k := 0; k < 9000 && at+k < n; k++ {
			f := 50 + 100*math.Exp(-float64(k)/1500)
			s.Samples[at+k] += int16(20000 * math.Sin(2*math.Pi*f*float64(k)/formats.SampleRate) * math.Exp(-float64(k)/5000))
		}
		off := int(math.Round((float64(b) + 0.5) * beat))
		for k := 0; k < 2000 && off+k < n; k++ {
			s.Samples[off+k] += int16(r.NormFloat64() * 4000 * math.Exp(-float64(k)/500))
		}
	}
	return s
}

func putOnPad(t *testing.T, a *App, n int, s formats.Sample, name string, bpm float64) {
	t.Helper()
	b, err := a.lab.Decode(s.WAV(), "wav")
	if err != nil {
		t.Fatal(err)
	}
	var m lab.Meta
	json.Unmarshal(b, &m)
	if err := a.assignPad(n, m.ID, name); err != nil {
		t.Fatal(err)
	}
	a.lab.Free(m.ID)
	if err := a.lab.SetPadParam(n, 0x6F, int32(bpm*100)); err != nil {
		t.Fatal(err)
	}
	a.refresh()
}

func padBPM(a *App, n int) float64 {
	v, _ := a.lab.PadParam(n, 0x6F)
	return float64(v) / 100
}

func TestTheBPMOfThePadsIsCheckedBeforeItIsSet(t *testing.T) {
	a, _ := newApp(t)
	keys(t, a, kit)                                         // A1..A3: one shots
	putOnPad(t, a, 3, houseLoop(124, 8), "Groove", 120)     // A4: the unit got it wrong
	putOnPad(t, a, 4, houseLoop(126, 8), "Roll", 126)       // A5: right
	putOnPad(t, a, 5, houseLoop(128, 8), "Top 125bpm", 120) // A6: the name says otherwise
	putOnPad(t, a, 6, houseLoop(140, 8), "Half", 70)        // A7: half time is the same tempo
	keys(t, a, "A [")
	chooseMenu(t, a, "Check the BPM")
	m := menuOf(a)
	if m == nil || !strings.Contains(m.title, "2 of 7 differ") {
		t.Fatalf("A4 and A6 differ: %+v", m)
	}
	var rows []string
	for _, it := range m.items {
		rows = append(rows, it.label)
	}
	all := strings.Join(rows, "\n")
	for _, want := range []string{"A4 Groove       120.00 -> 124.00 heard", "A6 Top 125bpm   120.00 -> 125.00 its name", "too short", "A7 Half          70.00  heard 140."} {
		if !strings.Contains(all, want) {
			t.Fatalf("no %q in\n%s", want, all)
		}
	}
	if padBPM(a, 3) != 120 {
		t.Fatal("nothing is set before it is chosen")
	}
	seekMenu(t, a, func(l string) bool { return strings.HasPrefix(l, "A4 ") })
	keys(t, a, "A")
	if v := padBPM(a, 3); math.Abs(v-124) > 0.3 || padBPM(a, 5) != 120 {
		t.Fatalf("A4 alone set to the heard 124: A4 %.2f A6 %.2f", v, padBPM(a, 5))
	}
	keys(t, a, "A [")
	chooseMenu(t, a, "Check the BPM")
	if m := menuOf(a); !strings.Contains(m.title, "1 of 7 differ") {
		t.Fatalf("A6 is left: %q", m.title)
	}
	chooseMenu(t, a, "Set the 1 that differ")
	if padBPM(a, 5) != 125 || padBPM(a, 6) != 70 || padBPM(a, 4) != 126 {
		t.Fatalf("A6 to its name's 125, A5 and A7 kept: %.2f %.2f %.2f", padBPM(a, 5), padBPM(a, 4), padBPM(a, 6))
	}
}
