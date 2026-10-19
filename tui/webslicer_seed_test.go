package tui

import (
	"slices"
	"testing"

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
