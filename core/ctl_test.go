package core

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/perflog"
)

func TestControllerPatternGroup(t *testing.T) {
	a, _ := newApp(t)
	cc := func(ch, n int) midiin.Event { return midiin.Event{Kind: midiin.CC, Channel: ch, Data1: n, Data2: 65} }
	on := func(ch, n, v int) midiin.Event {
		return midiin.Event{Kind: midiin.NoteOn, Channel: ch, Data1: n, Data2: v}
	}

	// the event log of a recording that runs
	path := filepath.Join(t.TempDir(), "take.events.jsonl")
	l, err := perflog.Create(path, "take.wav", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a.evlog = &eventLog{log: l, startedAt: time.Now()}

	// a push is a pattern of the bank that was chosen: the default bank is A
	a.ctlEvent(on(16, 62, 100))
	if a.perfNow != 3 || a.slotLabel(a.perfNow) != "A3" {
		t.Fatalf("a push of E3 in bank A: slot %d (%s)", a.perfNow, a.slotLabel(a.perfNow))
	}
	// a turn of E3 (the controller 62) chooses bank C; the next push is a pattern of it
	a.ctlEvent(cc(16, 62))
	if a.ctlBank != 2 {
		t.Fatalf("bank %d", a.ctlBank)
	}
	a.ctlEvent(on(16, 61, 100))
	if a.slotLabel(a.perfNow) != "C2" {
		t.Fatalf("E2 in bank C: %s", a.slotLabel(a.perfNow))
	}
	// what is not for the pattern group is left alone: another channel, a release, a controller or a note out of range
	a.ctlEvent(on(15, 70, 100))
	a.ctlEvent(on(16, 70, 0))
	a.ctlEvent(cc(16, 70))
	a.ctlEvent(on(16, 76, 100))
	a.ctlEvent(on(16, 59, 100))
	if a.slotLabel(a.perfNow) != "C2" || a.ctlBank != 2 {
		t.Fatalf("something else moved the pattern: %s, bank %d", a.slotLabel(a.perfNow), a.ctlBank)
	}
	// the channel can be changed or switched off
	a.set.MidiCtlChan = "off"
	a.ctlEvent(on(16, 60, 100))
	if a.slotLabel(a.perfNow) != "C2" {
		t.Fatal("the group is off")
	}
	a.set.MidiCtlChan = "12"
	a.ctlEvent(on(12, 75, 100))
	if a.slotLabel(a.perfNow) != "C16" {
		t.Fatalf("channel 12, E16: %s", a.slotLabel(a.perfNow))
	}

	l.Close()
	entries, err := perflog.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		if e["k"] == "pattern" {
			if e["by"] != "controller" {
				t.Fatalf("the entry says who: %v", e)
			}
			got = append(got, e["now"].(string))
		}
	}
	if len(got) != 3 || got[0] != "A3" || got[1] != "C2" || got[2] != "C16" {
		t.Fatalf("the patterns in the event log: %v", got)
	}
}

func TestControllerPortSetting(t *testing.T) {
	a, _ := newApp(t)
	if a.ctlPort() != "ec4" {
		t.Fatalf("by default %q", a.ctlPort())
	}
	a.set.MidiCtlPort = "off"
	if a.ctlPort() != "" {
		t.Fatal("off")
	}
	a.set.MidiCtlPort = "Faderfox"
	if a.ctlPort() != "faderfox" {
		t.Fatalf("%q", a.ctlPort())
	}
	if a.ctlStatus() == "" {
		t.Fatal("no status")
	}
}

// fakeOutputs is a MIDI source that can also send: what is sent is kept.
type fakeOutputs struct {
	fakeMIDI
	sent *[][]byte
	part *string
}

func (f fakeOutputs) OpenOut(part string) (midiin.Out, string, error) {
	*f.part = part
	return fakeOut{f.sent}, "SP-404MKII", nil
}

type fakeOut struct{ sent *[][]byte }

func (o fakeOut) Send(m []byte) error {
	*o.sent = append(*o.sent, append([]byte(nil), m...))
	return nil
}
func (o fakeOut) Close() {}

func TestControllerSendsThePatternToTheUnit(t *testing.T) {
	a, _ := newApp(t)
	var sent [][]byte
	var part string
	a.SetPlatform(Platform{Name: "test", MIDI: fakeOutputs{fakeMIDI: fakeMIDI{ch: make(chan midiin.Event, 1)}, sent: &sent, part: &part}})
	on := func(n int) midiin.Event { return midiin.Event{Kind: midiin.NoteOn, Channel: 16, Data1: n, Data2: 100} }
	// bank C (the turn of E3), then E2: pattern 2 of bank C is the Program Change 1 on channel 3
	a.ctlEvent(midiin.Event{Kind: midiin.CC, Channel: 16, Data1: 62, Data2: 65})
	a.ctlEvent(on(61))
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{0xC2, 0x01}) {
		t.Fatalf("sent %v", sent)
	}
	if part != "404" {
		t.Fatalf("to the unit's input: %q", part)
	}
	// turned off: the controller sends it itself
	a.set.MidiCtlSend = "off"
	a.ctlEvent(on(62))
	if len(sent) != 1 || a.slotLabel(a.perfNow) != "C3" {
		t.Fatalf("not sent: %v, %s", sent, a.slotLabel(a.perfNow))
	}
	// a Program Change of the controller (on a bank's channel) is the unit's own way: the pattern is noted, nothing is sent
	a.set.MidiCtlSend = ""
	a.ctlEvent(midiin.Event{Kind: midiin.ProgramChange, Channel: 5, Data1: 7})
	if a.slotLabel(a.perfNow) != "E8" || a.ctlBank != 4 || len(sent) != 1 {
		t.Fatalf("PC ch 5 prog 7: %s, bank %d, sent %v", a.slotLabel(a.perfNow), a.ctlBank, sent)
	}
	// out of range: ignored
	a.ctlEvent(midiin.Event{Kind: midiin.ProgramChange, Channel: 12, Data1: 7})
	a.ctlEvent(midiin.Event{Kind: midiin.ProgramChange, Channel: 5, Data1: 16})
	if a.slotLabel(a.perfNow) != "E8" {
		t.Fatal("out of range moved the pattern")
	}
}
