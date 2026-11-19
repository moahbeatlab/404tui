package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
	"github.com/moahbeatlab/404tui/midiin"
)

// The synth preview: a track's notes sent as MIDI notes on a channel, in time with the pattern that loops on the computer, to the
// unit's MIDI port. The unit passes what comes in on to its MIDI out (MIDI thru), so a synth behind it (an MC-101 with a track on
// each of channels 11 to 14) plays the line at once. While it does, the pad's own sample is not heard on the computer.

// synthNote is a note of the loop: when in the round and how long (in ticks of the pattern), the MIDI note and velocity.
type synthNote struct {
	tick, len int
	note, vel int
}

type synthOut struct {
	mu      sync.Mutex
	out     midiin.Out
	ch      int // 1..16
	notes   []synthNote
	start   time.Time     // when a round began
	loop    time.Duration // how long a round is; 0: nothing plays
	perTick float64       // nanoseconds a tick
	length  int           // the round in ticks of the pattern (the melody's own length)
	gen     int           // which melody the notes are (one more at every change)
	sentGen int           // the melody of the last note sent, and when its first note went
	sentAt  time.Time
	stop    chan struct{}
	done    chan struct{}
}

// synthPlay sends the notes of the track on channel ch (1..16) in time with the unit's MIDI clock while the unit plays, else with
// the loop on the computer; tonic is the MIDI note of the key's tonic and k the semitones of the tonic from the pad's own pitch (a
// note's pitch p, in semitones and not the pattern's pitch byte, is the MIDI note tonic+p-k). The pad is quiet on the computer meanwhile. It says whether a MIDI output is there.
func (a *App) synthPlay(pad int, notes []lab.Note, length, ch, tonic, k int) bool {
	out := a.midiOutput()
	if out == nil {
		return false
	}
	sn := make([]synthNote, 0, len(notes))
	for _, n := range notes {
		note := tonic + n.Pitch - k
		if note < 0 || note > 127 {
			continue
		}
		sn = append(sn, synthNote{tick: int(n.Tick), len: n.Len, note: note, vel: n.Vel})
	}
	if a.synth == nil {
		a.synth = &synthOut{out: out, stop: make(chan struct{}), done: make(chan struct{})}
		go a.synth.run()
	}
	s := a.synth
	s.mu.Lock()
	if s.ch != 0 && s.ch != ch {
		s.allOff()
	}
	s.ch, s.notes, s.length = ch, sn, length
	s.gen++
	s.mu.Unlock()
	a.synthPad = pad + 1
	a.replay() // the pad goes quiet on the computer
	a.synthSync()
	return true
}

// synthClock says whether the unit's MIDI clock runs (pulses came in the last half second).
func (a *App) synthClock() bool {
	return a.clk.Running && a.clk.Ticks > 0 && !a.clk.At.IsZero() && time.Since(a.clk.At) < 500*time.Millisecond
}

// synthTiming is when the round of a pattern length ticks long began and how long a round and a tick are: from the unit's MIDI
// clock while it plays (the rounds counted from its Start, the tempo its own), else from the loop on the computer; ok is false
// when neither plays.
func (a *App) synthTiming(length int) (start time.Time, loop time.Duration, perTick float64, ok bool) {
	if length <= 0 {
		return time.Time{}, 0, 0, false
	}
	if a.synthClock() {
		bpm := a.mon.BPM
		if bpm <= 0 {
			bpm = a.tempo()
		}
		pulse := 60 / bpm / 24 * float64(time.Second)
		perPulse := formats.PPQ / 24  // ticks of the pattern in a pulse of the clock
		lp := max(length/perPulse, 1) // the round in pulses
		n := a.clk.Ticks - 1          // the pulse that came last, from the Start (the first one is the downbeat)
		start = a.clk.At.Add(-time.Duration(float64(n%lp) * pulse))
		return start, time.Duration(float64(lp) * pulse), pulse / float64(perPulse), true
	}
	if a.seqOn {
		perTick = 60 / a.tempo() / formats.PPQ * float64(time.Second)
		return a.seqStart, time.Duration(float64(length) * perTick), perTick, true
	}
	return time.Time{}, 0, 0, false
}

// synthSync follows the unit's clock or the loop on the computer (and goes quiet when neither plays).
func (a *App) synthSync() {
	s := a.synth
	if s == nil {
		return
	}
	s.mu.Lock()
	length := s.length
	s.mu.Unlock()
	if d, ok := a.doc(); ok && length == 0 {
		length = int(d.Length)
	}
	start, loop, perTick, ok := a.synthTiming(length)
	s.mu.Lock()
	if !ok {
		if s.loop != 0 {
			s.loop = 0
			s.allOff()
		}
	} else {
		s.start, s.loop, s.perTick = start, loop, perTick
	}
	s.mu.Unlock()
}

// synthStop ends the preview: every note off, the pad heard again on the computer.
func (a *App) synthStop() {
	if a.synth != nil {
		close(a.synth.stop)
		<-a.synth.done
		a.synth = nil
	}
	if a.synthPad != 0 {
		a.synthPad = 0
		a.replay()
	}
}

// midiOutput is the unit's MIDI output (the port of CONFIG), opened the first time; nil when there is none.
func (a *App) midiOutput() midiin.Out {
	if a.midiOut != nil {
		return a.midiOut
	}
	outs, ok := a.plat.MIDI.(midiin.Outputs)
	if !ok {
		a.say("this build cannot send MIDI", true)
		return nil
	}
	o, name, err := outs.OpenOut(a.midiPort())
	if err != nil {
		a.say("no MIDI output: "+err.Error(), true)
		return nil
	}
	a.midiOut, a.midiOutName = o, name
	return o
}

// run sends each note when its time comes (the round's start may move a little as the clock is read: a note is sent once a round),
// and the note offs when they are due.
func (s *synthOut) run() {
	defer close(s.done)
	type pending struct {
		ch, note int
		off      time.Time
	}
	var on []pending
	var last time.Time          // up to when the notes were sent
	sent := map[int]time.Time{} // when each note was sent last
	seen := 0                   // the melody sent is known for: a new one starts with nothing sent
	tk := time.NewTicker(2 * time.Millisecond)
	defer tk.Stop()
	for {
		select {
		case <-s.stop:
			s.mu.Lock()
			for _, p := range on {
				s.out.Send([]byte{0x80 | byte(p.ch-1), byte(p.note), 0})
			}
			s.allOff()
			s.mu.Unlock()
			return
		case now := <-tk.C:
			s.mu.Lock()
			keep := on[:0]
			for _, p := range on {
				if now.Before(p.off) {
					keep = append(keep, p)
					continue
				}
				s.out.Send([]byte{0x80 | byte(p.ch-1), byte(p.note), 0})
			}
			on = keep
			if s.loop <= 0 || now.Before(s.start) {
				last = time.Time{}
				s.mu.Unlock()
				continue
			}
			if last.IsZero() || now.Sub(last) > s.loop/2 { // (a start, or a long pause: nothing behind now)
				last = now
				s.mu.Unlock()
				continue
			}
			if seen != s.gen { // a changed melody: its notes are new, from now on
				seen, sent = s.gen, map[int]time.Time{}
			}
			r := int(now.Sub(s.start) / s.loop)
			for i, n := range s.notes {
				for _, rr := range []int{r - 1, r} {
					at := s.start.Add(time.Duration(rr)*s.loop + time.Duration(float64(n.tick)*s.perTick))
					if !at.After(last) || at.After(now) {
						continue
					}
					if t, ok := sent[i]; ok && at.Sub(t) < s.loop/2 && t.Sub(at) < s.loop/2 {
						continue
					}
					sent[i] = at
					if s.sentGen != s.gen {
						s.sentGen, s.sentAt = s.gen, at
					}
					s.out.Send([]byte{0x90 | byte(s.ch-1), byte(n.note), byte(min(max(n.vel, 1), 127))})
					on = append(on, pending{s.ch, n.note, now.Add(max(time.Duration(float64(n.len)*s.perTick), 10*time.Millisecond))})
				}
			}
			last = now
			s.mu.Unlock()
		}
	}
}

// allOff is All Notes Off on the channel (with s.mu held).
func (s *synthOut) allOff() {
	if s.ch > 0 {
		s.out.Send([]byte{0xB0 | byte(s.ch-1), 123, 0})
	}
}

// synthOutNames is the dialog's choice of where a melody plays: the pad (no MIDI), or a MIDI channel of the synth behind the unit
// (the tracks of an MC-101 set up for live).
var synthOutNames = []string{"off (the pad's sample plays it)", "channel 11 (bass)", "channel 12 (synth / lead)", "channel 13 (pads)", "channel 14"}

// synthTonic is the MIDI note the key's tonic is played on for a role: the bass low, the rest an octave up.
func synthTonic(role, key int) int {
	if melRoles[role].kind == kBass {
		return 36 + key // C2..B2
	}
	return 48 + key // C3..B3
}

func synthSaid(ch int, name string) string {
	return fmt.Sprintf("on MIDI channel %d (to %s, through the unit)", ch, name)
}
