package core

import (
	"encoding/json"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

func longSample(sec int) formats.Sample {
	n := sec * 48000
	s := formats.Sample{Channels: 2, Samples: make([]int16, n*2)}
	for i := range s.Samples {
		s.Samples[i] = int16((i * 7919) % 20000)
	}
	return s
}

func BenchmarkPeaksDirect60s(b *testing.B) {
	s := longSample(60)
	for i := 0; i < b.N; i++ {
		s.Peaks(0, s.Frames(), 440)
	}
}

func TestBlockPeaksMatchDirect(t *testing.T) {
	a, _ := newApp(t)
	mb, err := a.lab.Decode(longSample(30).WAV(), "wav")
	if err != nil {
		t.Fatal(err)
	}
	var meta struct{ ID int }
	json.Unmarshal(mb, &meta)
	id := meta.ID
	for _, r := range [][2]int{{0, 30 * 48000}, {48000, 20 * 48000}, {5, 400000}} {
		want, _ := a.lab.Peaks(id, r[0], r[1], 440)
		got, err := a.peaks(id, r[0], r[1], 440)
		if err != nil || len(got) != len(want) {
			t.Fatal(err)
		}
		for i := range got { // the blocks are coarser at the edges: never quieter than the truth by more than a block's worth
			if got[i] < want[i]-want[i]/50 || got[i] > 32767 {
				t.Fatalf("col %d: %d vs %d", i, got[i], want[i])
			}
		}
	}
}

func BenchmarkPeaksCached60s(b *testing.B) {
	a := &App{}
	s := longSample(60)
	key := peakKey{&s.Samples[0], len(s.Samples)}
	a.peakCache = map[peakKey]*peakBlocks{key: buildBlocks(s)}
	pb := a.peakCache[key]
	for i := 0; i < b.N; i++ {
		for c := 0; c < 440; c++ {
			lo := c * len(pb.maxs) / 440
			hi := (c + 1) * len(pb.maxs) / 440
			peak := int16(0)
			for _, v := range pb.maxs[lo:hi] {
				peak = max(peak, v)
			}
		}
	}
}
