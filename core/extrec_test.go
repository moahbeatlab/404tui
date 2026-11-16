package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/midiin"
)

func TestParseChans(t *testing.T) {
	for in, want := range map[string]string{"11-14": "[11 12 13 14]", "11 13,12": "[11 12 13]", "16": "[16]"} {
		if got, err := parseChans(in); err != nil || fmt.Sprint(got) != want {
			t.Fatalf("%q: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "0", "17", "14-11", "x"} {
		if _, err := parseChans(bad); err == nil {
			t.Fatalf("%q should not be read", bad)
		}
	}
}

// at is the moment of a tick of a take at 120 BPM (a tick is 1/960 s).
func at(start time.Time, tick int) time.Time {
	return start.Add(time.Duration(tick) * time.Second / 960)
}

func TestATakeTimesTheNotesFromTheFirstClock(t *testing.T) {
	start := time.Unix(1000, 0)
	tk := newTake(120, 1, 0, []int{11, 12, 13, 14}, start)
	feed := func(kind midiin.Kind, ch, note, tick int) {
		tk.feed(midiin.Event{Kind: kind, Channel: ch, Data1: note, Data2: 100, At: at(start, tick)})
	}
	feed(midiin.NoteOn, 11, 36, -5) // a little early: the first tick
	feed(midiin.NoteOff, 11, 36, 100)
	feed(midiin.NoteOn, 14, 60, 480)
	feed(midiin.NoteOff, 14, 60, 960)
	feed(midiin.NoteOn, 14, 64, 1440) // held past the end: it ends there
	feed(midiin.NoteOn, 1, 50, 10)    // not a channel of the take
	feed(midiin.NoteOn, 12, 40, 1920) // after the end
	if tk.count != 3 {
		t.Fatalf("three notes: %d", tk.count)
	}
	f, err := formats.ParseMIDI(tk.finish(11))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Parts) != 2 || !f.Parts[0].Drums() || f.Parts[1].Channel != 13 {
		t.Fatalf("a drums part (channel 11 as 10) and channel 14: %+v", f.Parts)
	}
	if n := f.Parts[0].Notes[0]; n.Tick != 0 || n.Len != 100 || n.Note != 36 {
		t.Fatalf("the kick: %+v", n)
	}
	mel := f.Parts[1].Notes
	if len(mel) != 2 || mel[0].Tick != 480 || mel[0].Len != 480 || mel[1].Tick != 1440 || mel[1].Len != 480 {
		t.Fatalf("the melody: %+v", mel)
	}
	if f.Tempo < 119.9 || f.Tempo > 120.1 || f.MIDIBars(1) != 1 {
		t.Fatalf("120 BPM, one bar: %v %d", f.Tempo, f.MIDIBars(1))
	}
}

func TestATakeSnapsTheStarts(t *testing.T) {
	start := time.Unix(1000, 0)
	tk := newTake(120, 1, 16, []int{14}, start)
	tk.feed(midiin.Event{Kind: midiin.NoteOn, Channel: 14, Data1: 60, Data2: 90, At: at(start, 125)})
	tk.feed(midiin.Event{Kind: midiin.NoteOn, Channel: 14, Data1: 62, Data2: 90, At: at(start, 1915)}) // snaps onto the end: left out
	if len(tk.notes[14]) != 1 || tk.notes[14][0].Tick != 120 {
		t.Fatalf("on the 1/16 grid: %+v", tk.notes[14])
	}
}

type fakeOut struct {
	mu   sync.Mutex
	sent [][]byte
}

func (o *fakeOut) Send(b []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, append([]byte(nil), b...))
	return nil
}
func (o *fakeOut) Close() {}
func (o *fakeOut) has(b ...byte) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, s := range o.sent {
		if bytes.Equal(s, b) {
			return true
		}
	}
	return false
}

// fakeMC is an MC-101 on USB: the program listens to the unit on its own input, the device has an input and an output.
type fakeMC struct {
	ch  chan midiin.Event
	out *fakeOut
}

func (f fakeMC) Ports() ([]string, error) { return []string{"SP-404MKII", "MC-101"}, nil }
func (f fakeMC) Open(part string, on func(midiin.Event)) (func(), string, error) {
	if !strings.Contains("mc-101", part) {
		return func() {}, "SP-404MKII", nil
	}
	go func() {
		for e := range f.ch {
			on(e)
		}
	}()
	return func() {}, "MC-101", nil
}
func (f fakeMC) OpenOut(part string) (midiin.Out, string, error) { return f.out, "MC-101", nil }

func TestRecordingFromTheMC101SendsStartAndOpensTheImport(t *testing.T) {
	a, _ := newApp(t)
	f := fakeMC{ch: make(chan midiin.Event, 16), out: &fakeOut{}}
	a.SetPlatform(Platform{Name: "test", MIDI: f})
	a.recordExtMenu()
	m := a.menu
	if m == nil || !strings.Contains(m.title, "MC-101") {
		t.Fatalf("the form: %+v", m)
	}
	a.menu.item().run(0) // Start
	for i := 0; i < 100 && (a.ext == nil || a.ext.take.start.IsZero()); i++ {
		a.Tick()
		time.Sleep(5 * time.Millisecond)
	}
	if a.ext == nil || a.ext.take.start.IsZero() {
		t.Fatal("the take started")
	}
	if !f.out.has(0xFA) || !f.out.has(0xF8) {
		t.Fatal("Start and the clock were sent")
	}
	f.ch <- midiin.Event{Kind: midiin.NoteOn, Channel: 14, Data1: 60, Data2: 100}
	f.ch <- midiin.Event{Kind: midiin.NoteOn, Channel: 11, Data1: 36, Data2: 100}
	for i := 0; i < 100 && a.ext.take.count < 2; i++ {
		a.Tick()
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.HasPrefix(m.title, "REC MC-101") {
		t.Fatalf("the title shows the take: %q", m.title)
	}
	a.endExt()
	if a.ext != nil || !f.out.has(0xFC) {
		t.Fatal("Stop was sent")
	}
	if a.menu == nil || !strings.HasPrefix(a.menu.title, "MIDI file take-") || a.midiRun == nil {
		t.Fatalf("the import form opened: %+v", a.menu)
	}
	files, _ := filepath.Glob(filepath.Join(a.exportDir(), "take-*.mid"))
	if len(files) != 1 {
		t.Fatalf("the take is saved: %v", files)
	}
	b, _ := os.ReadFile(files[0])
	if fl, err := formats.ParseMIDI(b); err != nil || len(fl.Parts) != 2 {
		t.Fatalf("two parts: %v %+v", err, fl)
	}
	if !strings.HasPrefix(m.title, "record notes") {
		t.Fatalf("the form is ready for the next take: %q", m.title)
	}
}

func TestATakeWithNoNotesSaysWhy(t *testing.T) {
	a, _ := newApp(t)
	f := fakeMC{ch: make(chan midiin.Event, 1), out: &fakeOut{}}
	a.SetPlatform(Platform{Name: "test", MIDI: f})
	a.recordExtMenu()
	a.menu.item().run(0)
	a.endExt()
	if !strings.Contains(a.msg, "Tx USB") {
		t.Fatalf("it says what to check: %q", a.msg)
	}
}
