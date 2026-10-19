package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

func TestSlicerKeepsItsOrderWhenOnlyTheDiceChange(t *testing.T) {
	src := formats.Sample{Channels: 1, Samples: make([]int16, 48000)}
	for i := range src.Samples {
		src.Samples[i] = int16(i%200 + 1)
	}
	o := sliceOpts{n: 16, order: slOrderShuffle, seed: 42}
	_, a := sliceSample(src, o)
	o.mangle = mangleOpts{rev: 8, pitchN: 4, pitch: 5, roll: 6, fade: 10}
	_, b := sliceSample(src, o)
	if !slices.Equal(a, b) {
		t.Fatalf("the order changed with the dice: %v then %v", a, b)
	}
	o.seed = 43
	if _, c := sliceSample(src, o); slices.Equal(a, c) {
		t.Fatalf("a new seed gave the same order %v", c)
	}
}

func TestPickIntTakesTheChoiceOrStepsOn(t *testing.T) {
	l := []int{2, 4, 8}
	if pickInt(l, 2, 2) != 8 || pickInt(l, 2, -1) != 4 || pickInt(l, 2, 9) != 4 {
		t.Fatal("pickInt")
	}
}

func TestSettingsCutTheWaitingSlicesAgainWithoutPreviewAndKeepTheOrder(t *testing.T) {
	r, w := webRig(t)
	r.s.press("r")
	m := r.s.m()
	from, ok := m.slFreePad(0)
	if !ok {
		t.Skip("bank A is full")
	}
	src := formats.Sample{Channels: 1}
	for i := 0; i < 96000; i++ {
		src.Samples = append(src.Samples, int16(i/6000*1500))
	}
	if _, err := app.PutSample(m.unit, from, src, "SRC"); err != nil {
		t.Fatal(err)
	}
	r.s.run(m.load(""))
	r.web(fmt.Sprintf(`{"a":"slpad","i":%d}`, from.Pad()))
	r.web(`{"a":"slorder","pick":0}`)
	r.web(`{"a":"slgo"}`)
	first := w.last(t).Slicer
	if first.Phase != "ready" {
		t.Fatalf("not ready: %+v", first)
	}
	r.web(`{"a":"slmang","i":1,"n":1}`)
	r.web(`{"a":"sldiv","pick":2}`)
	second := w.last(t).Slicer
	if second.Phase != "ready" || second.Mangle[1] != 1 || second.Div != 4 {
		t.Fatalf("settings not taken while the slices wait: %+v", second)
	}
	if first.Played == "" || len(strings.Fields(second.Played)) == 0 {
		t.Fatalf("no order shown: %q %q", first.Played, second.Played)
	}
	r.web(`{"a":"slreorder"}`)
	if third := w.last(t).Slicer; third.Phase != "ready" || third.Played == second.Played {
		t.Errorf("holding the order button left the order as it was: %q", third.Played)
	}
}
