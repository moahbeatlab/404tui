package handheld

import "github.com/moahbeatlab/404tui/formats"

// Waveforms are drawn every frame, so the samples are not scanned every frame: the first time a sample is drawn its peaks
// are kept per block of peakBlock frames (a few KB for a minute of audio), and a view that has many frames to a column
// reads the blocks. Short views (zoomed in) scan the few frames they show. The cache is keyed by the buffer itself, so an
// edited sample (a new buffer) never shows old peaks.
const peakBlock = 64

type peakKey struct {
	p *int16
	n int
}

type peakBlocks struct {
	ch   int
	maxs []int16
}

func buildBlocks(s formats.Sample) *peakBlocks {
	ch := max(s.Channels, 1)
	frames := s.Frames()
	pb := &peakBlocks{ch: ch, maxs: make([]int16, (frames+peakBlock-1)/peakBlock)}
	for b := range pb.maxs {
		lo, hi := b*peakBlock*ch, min((b+1)*peakBlock*ch, len(s.Samples))
		peak := 0
		for _, v := range s.Samples[lo:hi] {
			if v < 0 {
				v = -v
			}
			peak = max(peak, int(v))
		}
		pb.maxs[b] = int16(min(peak, 32767))
	}
	return pb
}

// peaks is formats.Sample.Peaks through the block cache.
func (a *App) peaks(id, from, to, cols int) ([]int, error) {
	s, err := a.lab.Sample(id)
	if err != nil {
		return nil, err
	}
	frames := s.Frames()
	from, to = max(from, 0), min(to, frames)
	if cols <= 0 || to <= from || len(s.Samples) == 0 || (to-from)/cols < 2*peakBlock {
		return s.Peaks(from, to, cols), nil
	}
	key := peakKey{&s.Samples[0], len(s.Samples)}
	pb := a.peakCache[key]
	if pb == nil {
		if a.peakCache == nil || len(a.peakCache) > 48 {
			a.peakCache = map[peakKey]*peakBlocks{}
		}
		pb = buildBlocks(s)
		a.peakCache[key] = pb
	}
	out := make([]int, cols)
	for c := range out {
		lo := (from + int(int64(c)*int64(to-from)/int64(cols))) / peakBlock
		hi := (from + int(int64(c+1)*int64(to-from)/int64(cols))) / peakBlock
		hi = min(max(hi, lo+1), len(pb.maxs))
		peak := 0
		for _, v := range pb.maxs[lo:hi] {
			peak = max(peak, int(v))
		}
		out[c] = peak
	}
	return out, nil
}
