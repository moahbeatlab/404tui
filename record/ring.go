package record

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/formats"
)

// ErrNoNative is what StartRing says on a system where the program cannot listen to the input itself.
var ErrNoNative = errors.New("no native audio capture on this system")

// Ringing is a backend that can also keep the latest seconds of the input in memory: a Ring.
type Ringing interface {
	StartRing(device string, seconds int) (*Ring, error)
}

// ringBuckets is how many half seconds the estimate of when the first frame was captured looks back over.
const ringBuckets = 20

// Ring keeps the latest seconds of a stereo input (16 bit) in memory. Whatever listens to the input calls Write with
// what it got; the rest of the program asks what is loud now, and cuts a piece out by the time it was heard.
//
// The time of a frame is worked out from when blocks of frames arrived: a block cannot arrive before its last frame
// was captured, so the earliest arrival (over the last ten seconds, which follows a device clock that drifts) is
// the closest to the truth. What is left is a fixed delay, which is the same for every take.
type Ring struct {
	rate int

	mu    sync.Mutex
	buf   []int16 // stereo, a ring
	total int64   // frames written
	// bars, beats and bpm are the grid a retroactive capture is cut to: how many bars the take
	// should be, the beats to a bar, and the tempo. Zero bars means no capture has been asked for.
	bars  int
	beats int
	bpm   float64
	// origin is when frame 0 was heard, as an estimate; the buckets hold the least of each half second.
	origin  time.Time
	buckets [ringBuckets]float64 // seconds since the epoch of the first arrival
	bucketN [ringBuckets]int64   // which half second a bucket holds
	epoch   time.Time
	wake    chan struct{}
	err     error

	stop func()
	done chan struct{}
	once sync.Once
}

// NewRing makes a ring for seconds of audio at rate frames a second, with nothing listening to the input yet.
func NewRing(rate, seconds int) *Ring {
	return &Ring{rate: rate, buf: make([]int16, 2*rate*seconds), done: make(chan struct{}), wake: make(chan struct{})}
}

// Rate is the frames a second.
func (r *Ring) Rate() int { return r.rate }

// Channels is how many channels a frame has: a ring keeps stereo, so two.
func (r *Ring) Channels() int { return 2 }

// Running says whether frames are still being heard.
func (r *Ring) Running() bool {
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

// Span is how much sound the ring keeps, in frames.
func (r *Ring) Span() int64 { return int64(len(r.buf) / 2) }

// Widen makes the ring keep at least seconds of sound, and says whether it had to grow. A ring
// cannot be narrowed: takes already in it are still wanted. It is what a retroactive capture calls
// before waiting for the phrase, so the ring reaches back over the bars it was asked for.
//
// Frame n lives at n modulo the number of frames the buffer holds, so growing the buffer moves every
// frame. The frames that were heard keep their numbers, which means the take that was cut out before
// the widening is the same take afterwards.
func (r *Ring) Widen(seconds int) bool {
	if seconds <= 0 {
		return false
	}
	want := seconds * r.rate
	r.mu.Lock()
	defer r.mu.Unlock()
	if want <= len(r.buf)/2 {
		return false
	}
	old, oldFrames := r.buf, int64(len(r.buf)/2)
	keep := min(oldFrames, r.total)
	grown := make([]int16, 2*want)
	// The oldest frame that is still held goes to its own place in the new buffer, and so does
	// each frame after it: the numbers do not change, only where they sit.
	from := r.total - keep
	for f := from; f < r.total; f++ {
		src := int((f % oldFrames) * 2)
		dst := int((f % int64(want)) * 2)
		grown[dst], grown[dst+1] = old[src], old[src+1]
	}
	r.buf = grown
	return true
}

// SetGrid says where the bars of the take are: how many bars and beats to a bar, and the tempo.
// The frame a capture starts on is worked out from these at the frame of the trigger.
func (r *Ring) SetGrid(bars, beatsPerBar int, bpm float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bars, r.beats, r.bpm = bars, beatsPerBar, bpm
}

// Grid is the bars, beats to a bar and tempo a capture is cut to.
func (r *Ring) Grid() (bars, beatsPerBar int, bpm float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bars, r.beats, r.bpm
}

// Capture cuts the phrase before the trigger out of the ring as a whole number of bars at the
// tempo of the grid, and says which frames it kept. frame is where the trigger fell, counted from
// the first frame ever heard. The take is measured back from the trigger, so a phrase played just
// after a bar line still comes out as whole bars, and preBars of the sound before it are kept as
// the run-up so nothing that was played before the trigger is clipped.
//
// It returns io.ErrShortBuffer when the ring has already been overwritten past where the take
// starts: widening it first, and reading promptly, is what avoids that.
func (r *Ring) Capture(frame int64, preBars int) (start, end int64, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bpm <= 0 || r.beats <= 0 || r.bars < 1 {
		return 0, 0, errors.New("no grid: set the tempo and the bars a take is cut to")
	}
	rate := r.rate
	samplesPerBar := int64(float64(rate) * 60 * float64(r.beats) / r.bpm)
	if samplesPerBar < 1 {
		return 0, 0, errors.New("the tempo is too fast to cut a bar")
	}
	if frame < 0 {
		frame = 0
	}
	// The take is the bars before the trigger, ending on the bar line the trigger fell after. It is
	// cut backwards from the moment the key was pressed, which is the whole point: the phrase that
	// has already gone by is what gets kept, and everything in it is in the buffer already.
	anchor := frame / samplesPerBar * samplesPerBar
	end = anchor + int64(r.bars)*samplesPerBar
	// The kick can land a hair before the bar line; take the bar before it when it does.
	for end < frame {
		end += samplesPerBar
	}
	start = end - int64(r.bars+preBars)*samplesPerBar
	if start < 0 {
		start = 0
	}
	if end > r.total {
		return 0, 0, errors.New("the take is not over yet: let it run to the end of the bar")
	}
	if end-start < samplesPerBar {
		return 0, 0, errors.New("the take is shorter than a bar: let it run longer")
	}
	if start < r.total-r.Span() {
		return 0, 0, errors.New("the take is older than the sound the ring keeps: let it run longer")
	}
	return start, end, nil
}

// Save writes frames frames from start on to a 16 bit stereo WAV in dir, and says where it went. It
// is a plain file, so a capture is still there after the program has ended.
func (r *Ring) Save(dir, name string, start, frames int64) (string, error) {
	got, err := r.Copy(start, frames)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sample := formats.Sample{Channels: 2, Samples: got}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, sample.WAV(), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Done is closed when listening ends; Err says why (nil when it was stopped on purpose).
func (r *Ring) Done() <-chan struct{} { return r.done }

func (r *Ring) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Stop ends the listening.
func (r *Ring) Stop() {
	r.once.Do(func() {
		if r.stop != nil {
			r.stop()
		}
		close(r.done)
	})
}

// fail ends the ring because listening failed.
func (r *Ring) fail(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
	r.Stop()
}

// Write adds frames (interleaved left and right) that arrived at the time at.
func (r *Ring) Write(frames []int16, at time.Time) {
	if len(frames) < 2 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(frames) / 2
	size := len(r.buf)
	pos := int(r.total*2) % size
	for i := 0; i < n*2; {
		c := copy(r.buf[pos:], frames[i:n*2])
		i += c
		pos = (pos + c) % size
	}
	r.total += int64(n)
	if r.epoch.IsZero() {
		r.epoch = at
	}
	// The latest arrival says: frame 0 was heard at most this long ago.
	cand := at.Sub(r.epoch).Seconds() - float64(r.total)/float64(r.rate)
	b := int64(at.Sub(r.epoch).Seconds() * 2)
	// A frame can arrive stamped before the one that started the clock — a driver that hands over a
	// backlog, or a test that lays its stamps out in one go — and Go's % keeps the sign, so a
	// negative bucket would index off the front of the array.
	slot := int(((b % ringBuckets) + ringBuckets) % ringBuckets)
	if r.bucketN[slot] != b {
		r.bucketN[slot], r.buckets[slot] = b, cand
	} else if cand < r.buckets[slot] {
		r.buckets[slot] = cand
	}
	best := math.Inf(1)
	for i, v := range r.buckets {
		if b-r.bucketN[i] < ringBuckets && r.bucketN[i] <= b && v < best {
			best = v
		}
	}
	r.origin = r.epoch.Add(time.Duration(best * float64(time.Second)))
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Total is how many frames have been heard.
func (r *Ring) Total() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}

// Level is the highest sample of each channel, 0 to 1, over the last 100 ms, and whether anything came lately.
func (r *Ring) Level() (peak [2]float64, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.total == 0 || r.epoch.IsZero() {
		return peak, false
	}
	n := min(int64(r.rate/10), r.total)
	size := len(r.buf)
	var hi [2]int
	for f := r.total - n; f < r.total; f++ {
		p := int(f*2) % size
		for c := 0; c < 2; c++ {
			v := int(r.buf[p+c])
			if v < 0 {
				v = -v
			}
			hi[c] = max(hi[c], v)
		}
	}
	return [2]float64{float64(hi[0]) / 32768, float64(hi[1]) / 32768}, true
}

// FrameAt is the number of the frame that was heard at the time t (it may be past the frames there are yet).
func (r *Ring) FrameAt(t time.Time) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(math.Round(t.Sub(r.origin).Seconds() * float64(r.rate)))
}

// WaitFor waits until the frame is there, or the timeout passes; it says whether it is.
func (r *Ring) WaitFor(frame int64, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		if r.Total() >= frame {
			return true
		}
		select {
		case <-r.wake:
		case <-deadline:
			return r.Total() >= frame
		case <-r.done:
			return r.Total() >= frame
		}
	}
}

// Copy is frames frames from the frame start on, as interleaved left and right.
func (r *Ring) Copy(start, frames int64) ([]int16, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	size := int64(len(r.buf) / 2)
	switch {
	case frames <= 0:
		return nil, errors.New("nothing to copy")
	case start < 0 || start < r.total-size:
		return nil, errors.New("that part of the sound is no longer kept: the loop is longer than the memory")
	case start+frames > r.total:
		return nil, errors.New("that part of the sound has not been heard yet")
	}
	out := make([]int16, frames*2)
	for i := range frames {
		p := int((start+i)*2) % len(r.buf)
		out[i*2], out[i*2+1] = r.buf[p], r.buf[p+1]
	}
	return out, nil
}
