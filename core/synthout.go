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

// synthNote is a note of the loop: when in the round, how long, the MIDI note and velocity.
type synthNote struct {
	at, dur time.Duration
	note    int
	vel     int
}

type synthOut struct {
	mu    sync.Mutex
	out   midiin.Out
	ch    int // 1..16
	notes []synthNote
	start time.Time
	loop  time.Duration // 0: paused
	stop  chan struct{}
	done  chan struct{}
}

// synthPlay sends the notes of the track on channel ch (1..16) while the pattern loops; tonic is the MIDI note of the key's tonic and
// k the semitones of the tonic from the pad's own pitch (a note's pitch p is the MIDI note tonic+p-k). The pad is quiet on the
// computer meanwhile. It says whether a MIDI output is there.
func (a *App) synthPlay(pad int, notes []lab.Note, ch, tonic, k int) bool {
	out := a.midiOutput()
	if out == nil {
		return false
	}
	d, ok := a.doc()
	if !ok {
		return false
	}
	perTick := 60 / a.tempo() / formats.PPQ * float64(time.Second)
	sn := make([]synthNote, 0, len(notes))
	for _, n := range notes {
		note := tonic + formats.PitchSemitones(uint8(n.Pitch)) - k
		if note < 0 || note > 127 {
			continue
		}
		sn = append(sn, synthNote{at: time.Duration(float64(n.Tick) * perTick), dur: time.Duration(float64(n.Len) * perTick), note: note, vel: n.Vel})
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
	s.ch, s.notes = ch, sn
	s.loop = 0
	if a.seqOn {
		s.start, s.loop = a.seqStart, time.Duration(float64(d.Length)*perTick)
	}
	s.mu.Unlock()
	a.synthPad = pad + 1
	a.replay() // the pad goes quiet on the computer
	return true
}

// synthSync follows the loop on the computer: its start, and a stop (the synth goes quiet with it).
func (a *App) synthSync() {
	s := a.synth
	if s == nil {
		return
	}
	d, ok := a.doc()
	s.mu.Lock()
	switch {
	case !a.seqOn || !ok:
		if s.loop != 0 {
			s.loop = 0
			s.allOff()
		}
	default:
		s.start, s.loop = a.seqStart, time.Duration(float64(d.Length)*60/a.tempo()/formats.PPQ*float64(time.Second))
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

// run sends the notes as the rounds go by, and the note offs when they are due.
func (s *synthOut) run() {
	defer close(s.done)
	type pending struct {
		ch, note int
		off      time.Time
	}
	var on []pending
	round, pos := -1, time.Duration(0)
	var start time.Time
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
				round = -1
				s.mu.Unlock()
				continue
			}
			if !s.start.Equal(start) { // a new start: from where the loop is, nothing behind it
				start, round = s.start, -1
			}
			el := now.Sub(s.start)
			r, p := int(el/s.loop), el%s.loop
			if round >= 0 {
				for _, n := range s.notes {
					due := r == round && n.at > pos && n.at <= p || r > round && (n.at > pos || n.at <= p)
					if !due {
						continue
					}
					s.out.Send([]byte{0x90 | byte(s.ch-1), byte(n.note), byte(min(max(n.vel, 1), 127))})
					on = append(on, pending{s.ch, n.note, now.Add(max(n.dur, 10*time.Millisecond))})
				}
			}
			round, pos = r, p
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

// synthOutName is the dialog's choice of where a melody plays: the pad, or a MIDI channel of the synth behind the unit.
var synthOutNames = []string{"the pad (its sample)", "MIDI channel 11", "MIDI channel 12", "MIDI channel 13", "MIDI channel 14"}

// synthTonic is the MIDI note the key's tonic is played on for a role: the bass low, the rest an octave up.
func synthTonic(role, key int) int {
	if role == 0 {
		return 36 + key // C2..B2
	}
	return 48 + key // C3..B3
}

func synthSaid(ch int, name string) string {
	return fmt.Sprintf("on MIDI channel %d (to %s, through the unit)", ch, name)
}
