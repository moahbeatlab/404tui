// Package midiin listens to a MIDI input: the pads the unit sends over USB MIDI, its clock, and
// whatever else is sent to control the app. The listening itself is the driver's, which is built
// in only with the build tag midi (it needs a C compiler, and on Linux the ALSA headers).
package midiin

import (
	"errors"
	"fmt"
)

// Kind is what a message is.
type Kind int

const (
	NoteOn Kind = iota
	NoteOff
	CC
	// Clock is a tick of the MIDI clock, 24 to a quarter note; Start, Stop and Continue are the
	// transport.
	Clock
	Start
	Stop
	Continue
)

func (k Kind) String() string {
	return [...]string{"note on", "note off", "cc", "clock", "start", "stop", "continue"}[k]
}

// Event is a message. Channel is 1 to 16 for a channel message and 0 for the others; Data1 is the
// note or controller number, Data2 the velocity or value.
type Event struct {
	Kind         Kind
	Channel      int
	Data1, Data2 int
}

func (e Event) String() string {
	if e.Channel == 0 {
		return e.Kind.String()
	}
	return fmt.Sprintf("%s ch %d %d %d", e.Kind, e.Channel, e.Data1, e.Data2)
}

// Parse reads the bytes of one MIDI message. A note on with velocity 0 is a note off. It says
// false for the messages nothing here uses.
func Parse(b []byte) (Event, bool) {
	if len(b) == 0 {
		return Event{}, false
	}
	switch b[0] {
	case 0xF8:
		return Event{Kind: Clock}, true
	case 0xFA:
		return Event{Kind: Start}, true
	case 0xFB:
		return Event{Kind: Continue}, true
	case 0xFC:
		return Event{Kind: Stop}, true
	}
	if b[0] < 0x80 || b[0] >= 0xF0 || len(b) < 3 {
		return Event{}, false
	}
	ch := int(b[0]&0x0F) + 1
	d1, d2 := int(b[1]&0x7F), int(b[2]&0x7F)
	switch b[0] & 0xF0 {
	case 0x90:
		if d2 == 0 {
			return Event{Kind: NoteOff, Channel: ch, Data1: d1}, true
		}
		return Event{Kind: NoteOn, Channel: ch, Data1: d1, Data2: d2}, true
	case 0x80:
		return Event{Kind: NoteOff, Channel: ch, Data1: d1, Data2: d2}, true
	case 0xB0:
		return Event{Kind: CC, Channel: ch, Data1: d1, Data2: d2}, true
	}
	return Event{}, false
}

// Source is somewhere MIDI comes from.
type Source interface {
	// Ports names the inputs there are.
	Ports() ([]string, error)
	// Open listens to the first input whose name holds part (any case), and calls on for each
	// message until stop is called. It says which input it took.
	Open(part string, on func(Event)) (stop func(), name string, err error)
}

// ErrUnavailable is what a build without the midi tag says.
var ErrUnavailable = errors.New("this build has no MIDI: build with -tags midi (needs a C compiler)")

type unavailable struct{}

func (unavailable) Ports() ([]string, error) { return nil, ErrUnavailable }
func (unavailable) Open(string, func(Event)) (func(), string, error) {
	return nil, "", ErrUnavailable
}
