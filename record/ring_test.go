package record

import (
	"testing"
	"time"
)

// feed plays the input into the ring: a ramp whose value is the frame number, in blocks of 960 frames (20 ms) that
// arrive late by a jitter of up to 15 ms.
func feed(r *Ring, start time.Time, seconds int) {
	const block = 960
	frame := 0
	for t := 0; t < seconds*50; t++ {
		data := make([]int16, block*2)
		for i := 0; i < block; i++ {
			v := int16((frame + i) % 30000)
			data[2*i], data[2*i+1] = v, -v
		}
		frame += block
		// block t ends at start + (t+1)*20 ms: it arrives that late, plus a jitter
		jitter := time.Duration((t*7)%16) * time.Millisecond
		r.Write(data, start.Add(time.Duration(t+1)*20*time.Millisecond+jitter))
	}
}

func TestTheRingKeepsTheLatestSoundAndFindsFramesByTime(t *testing.T) {
	r := NewRing(48000, 10)
	start := time.Unix(1000, 0)
	feed(r, start, 15) // 15 s into a ring of 10
	if got := r.Total(); got != 15*48000 {
		t.Fatalf("total %d", got)
	}
	// The time of frame n is start + n/48000, give or take the jitter that is left (under 2 ms).
	at := start.Add(12 * time.Second)
	f := r.FrameAt(at)
	if d := f - 12*48000; d < -100 || d > 100 {
		t.Errorf("frame at 12 s is %d, want %d (off by %d frames)", f, 12*48000, d)
	}
	data, err := r.Copy(f, 4800)
	if err != nil {
		t.Fatal(err)
	}
	if want := int16(f % 30000); data[0] != want || data[1] != -want {
		t.Errorf("first frame %d %d, want %d", data[0], data[1], want)
	}
	// What is older than the ring, and what is not there yet, is refused.
	if _, err := r.Copy(0, 100); err == nil {
		t.Error("sound older than the ring was given")
	}
	if _, err := r.Copy(r.Total()-10, 100); err == nil {
		t.Error("sound that has not been heard was given")
	}
	if p, ok := r.Level(); !ok || p[0] <= 0 || p[0] > 1 {
		t.Errorf("level %v %v", p, ok)
	}
}

func TestTheRingWaitsForAFrame(t *testing.T) {
	r := NewRing(48000, 2)
	if r.WaitFor(100, 20*time.Millisecond) {
		t.Error("a frame that never came was waited for")
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		r.Write(make([]int16, 400), time.Now())
	}()
	if !r.WaitFor(150, time.Second) {
		t.Error("the frame came and was not seen")
	}
}
