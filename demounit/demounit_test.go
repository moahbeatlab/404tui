package demounit

import (
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/proto"
)

func TestTheFirstPatternHasItsNotes(t *testing.T) {
	dev := New()
	for _, slot := range []int{0, 1, 2, 16, 32} {
		p, _ := proto.NewPadIndex(uint16(slot))
		d, err := app.OpenPattern(dev, p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		per := map[proto.PadIndex]int{}
		for _, n := range d.Notes {
			per[n.Pad]++
		}
		t.Logf("%s: %d notes, bars %d, pads %v", p, len(d.Notes), d.Bars(), per)
	}
}

func TestParsePatternSeesAllTheNotes(t *testing.T) {
	dev := New()
	b, _ := dev.ReadFile("ROLAND/SP-404MKII/PROJECT_01/PTN/PTN00001.BIN")
	p, err := formats.ParsePattern(b)
	if err != nil {
		t.Fatal(err)
	}
	per := map[int]int{}
	ctl := 0
	for _, e := range p.Events {
		if e.Control {
			ctl++
			continue
		}
		per[int(e.Pad.Index())]++
	}
	t.Logf("events %d, controls %d, per pad %v, length %d", len(p.Events), ctl, per, p.Length)
	recs, _ := formats.NoteRecords(b)
	t.Logf("records %d", len(recs))
}

func TestBankBHoldsTheSon(t *testing.T) {
	dev := New()
	p, _ := proto.NewPadIndex(16 + 4)
	d, err := app.OpenPattern(dev, p)
	if err != nil {
		t.Fatal(err)
	}
	pads := map[proto.PadIndex]bool{}
	for _, n := range d.Notes {
		pads[n.Pad] = true
	}
	if len(pads) < 6 {
		t.Errorf("B5 plays %d pads, want the bongos and the conga tones", len(pads))
	}
}
