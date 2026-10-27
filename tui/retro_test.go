package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/record"
)

// Retroactive capture is tested the way the rest of the views are: a rig with a ring of its own,
// the key pressed, and the screen and the file that come out of it read. The ring is fed by hand,
// so nothing here needs a unit, a clock, or a sound card.

// retroRig is a rig whose input is a ring of its own, already full of a ramp: frame n carries n, so
// a take says where it was cut from by what is in it.
func retroRig(t *testing.T) *perfRig {
	t.Helper()
	r := newPerfRig(t, 200, 44)
	r.s.press("8") // the Looper, which is where a loop is kept
	ring := record.NewRing(1000, 30)
	base := time.Now()
	// 20 seconds of ramp heard from the moment listening started, so the ring's own estimate of
	// when frame 0 was heard agrees with its frame numbers: a trigger stamped from time.Now() then
	// lands inside the buffer, which is where a retroactive capture has to look.
	for i := int64(0); i < 20000; i++ {
		ring.Write([]int16{int16(i), int16(-i)}, base.Add(time.Duration(i)*time.Millisecond))
	}
	m := r.s.m()
	m.ring, m.ringOff = ring, false
	r.s.model = m
	return r
}

// play hears the next frames of the same ramp, stamped now: a phrase is played, and then the key
// is pressed. The stamps have to run up to the present or the trigger lands before the take can
// end. time.Sleep is what a real input does; laying the stamps out in one go is the same
// arithmetic without the wait, but the last one has to be now.
func play(r *perfRig, frames int) {
	ring := r.s.m().ring
	start := ring.Total()
	now := time.Now()
	for i := int64(0); i < int64(frames); i++ {
		at := now.Add(-time.Duration(int64(frames)-i) * time.Millisecond)
		ring.Write([]int16{int16(start + i), int16(-start - i)}, at)
	}
}

// TestTheRetroKeyKeepsTheBarsJustPlayed is the whole feature in one pass: the key arms the capture,
// the phrase is played, the key again cuts it, and what lands is whole bars of the sound that had
// already gone by when the key was pressed.
func TestTheRetroKeyKeepsTheBarsJustPlayed(t *testing.T) {
	r := retroRig(t)
	// A tempo, as the unit would be sending one.
	r.feed(midiin.Event{Kind: midiin.Start})
	m := r.s.m()
	m.mon.bpm = 120
	m.retroSaved, m.retroFor = "", t.TempDir()
	m.mem.RecordDir = t.TempDir()
	r.s.model = m

	r.s.press("ctrl+g")
	if !r.s.m().retroArmed {
		t.Fatalf("ctrl+g did not arm a capture: %q", r.s.m().message)
	}
	if msg := r.s.m().message; !strings.Contains(msg, "press again") {
		t.Errorf("the screen does not say what to do next: %q", msg)
	}
	if badge := r.s.m().retroBadge(); badge != "RETRO" {
		t.Errorf("the header badge is %q, want RETRO", badge)
	}

	// The phrase is played and the ring carries on hearing it: five seconds, which is more than
	// the two bars the take wants, so the take is complete and the ring still holds before it.
	play(r, 5000)
	r.s.pressRunning("ctrl+g")
	if r.s.m().retroArmed {
		t.Error("the capture is still armed after the second press")
	}

	m = r.s.m()
	if !strings.Contains(m.message, "kept") {
		t.Fatalf("the take was not kept: %q", m.message)
	}
	if m.retroSaved == "" {
		t.Fatal("no file was named")
	}
	if _, err := os.Stat(m.retroSaved); err != nil {
		t.Fatalf("the take is not there: %v", err)
	}
	// At 120 BPM a bar is two seconds, which is 2000 frames of a 1000 Hz ring. The take is one bar
	// plus the bar of run-up kept before it, so two bars: the phrase is longer than the take.
	info, err := os.Stat(m.retroSaved)
	if err != nil {
		t.Fatal(err)
	}
	seconds := float64(info.Size()-44) / 2 / 2 / 1000
	if seconds < 3.9 || seconds > 4.1 {
		t.Errorf("the take is %.2fs, want about 4s: two bars at 120 BPM", seconds)
	}
}

// TestTheRetroKeyWithoutSoundToTakeSaysSo checks the refusals are useful rather than silent: a take
// that cannot be cut has to say why, because the alternative is a key that does nothing.
func TestTheRetroKeyWithoutSoundToTakeSaysSo(t *testing.T) {
	s := open(t)
	s.send(tea.WindowSizeMsg{Width: 200, Height: 44})
	s.press("ctrl+g")
	if msg := s.m().message; !strings.Contains(msg, "no retroactive capture") {
		t.Errorf("with no ring the key says %q, want a reason", msg)
	}
	if s.m().retroArmed {
		t.Error("a capture was armed with no ring to take one from")
	}
}

// TestTheRetroKeyWithoutATempoSaysSo is the other refusal: a take has to be a number of bars, and
// bars need a tempo.
func TestTheRetroKeyWithoutATempoSaysSo(t *testing.T) {
	r := retroRig(t)
	r.feed(midiin.Event{Kind: midiin.Start})
	m := r.s.m()
	m.mon.bpm = 0
	m.patterns = nil
	m.patPreview = nil
	r.s.model = m
	r.s.press("ctrl+g")
	if msg := r.s.m().message; !strings.Contains(msg, "tempo") {
		t.Errorf("with no tempo the key says %q, want a reason about tempo", msg)
	}
}

// TestTheRetroTakeIsWholeBarsAtTheTempoOfTheUnit is the point of the feature, tested through the
// screen: the same phrase at two tempos gives two different take lengths, and each is a whole
// number of bars at that tempo.
func TestTheRetroTakeIsWholeBarsAtTheTempoOfTheUnit(t *testing.T) {
	length := func(t *testing.T, bpm float64) float64 {
		t.Helper()
		r := retroRig(t)
		r.feed(midiin.Event{Kind: midiin.Start})
		m := r.s.m()
		m.mon.bpm = bpm
		m.retroSaved = ""
		r.s.model = m
		r.s.press("ctrl+g")
		play(r, int(20*bpm))
		r.s.pressRunning("ctrl+g")
		info, err := os.Stat(r.s.m().retroSaved)
		if err != nil {
			t.Fatalf("at %.0f BPM: %v (%q)", bpm, err, r.s.m().message)
		}
		return float64(info.Size()-44) / 2 / 2 / 1000
	}
	slow, fast := length(t, 90), length(t, 180)
	// 90 BPM: a bar is 2.667s and the take is two bars, so about 5.3s. 180 BPM: about 2.7s.
	if slow < 5.2 || slow > 5.5 {
		t.Errorf("at 90 BPM the take is %.2fs, want about 5.33s (two bars)", slow)
	}
	if fast < 2.6 || fast > 2.8 {
		t.Errorf("at 180 BPM the take is %.2fs, want about 2.67s (two bars)", fast)
	}
	if slow <= fast {
		t.Errorf("the slow take (%.2fs) is not longer than the fast one (%.2fs)", slow, fast)
	}
}

// TestTheRetroTakeIsNamedAndKeptBesideTheRecordings checks the take lands somewhere findable: the
// file is in the folder recordings go to, and its name says it is a take rather than a recording.
func TestTheRetroTakeIsNamedAndKeptBesideTheRecordings(t *testing.T) {
	r := retroRig(t)
	r.feed(midiin.Event{Kind: midiin.Start})
	dir := t.TempDir()
	m := r.s.m()
	m.mon.bpm = 120
	m.retroSaved = ""
	m.mem.RecordDir = dir
	r.s.model = m
	r.s.press("ctrl+g")
	play(r, 5000)
	r.s.pressRunning("ctrl+g")
	got := r.s.m().retroSaved
	if !strings.HasPrefix(filepath.Base(got), "retro ") {
		t.Errorf("the take is named %q, want it to say it is a take", filepath.Base(got))
	}
	if filepath.Dir(got) != dir {
		t.Errorf("the take is in %q, want it beside the recordings in %q", filepath.Dir(got), dir)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("the take is not there: %v", err)
	}
}
