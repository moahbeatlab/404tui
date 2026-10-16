package midiin

import (
	"reflect"
	"testing"
)

func TestParseReadsTheMessagesTheAppUses(t *testing.T) {
	for _, c := range []struct {
		in   []byte
		want Event
		ok   bool
	}{
		{[]byte{0x90, 48, 100}, Event{Kind: NoteOn, Channel: 1, Data1: 48, Data2: 100}, true},
		{[]byte{0x95, 36, 1}, Event{Kind: NoteOn, Channel: 6, Data1: 36, Data2: 1}, true},
		{[]byte{0x90, 48, 0}, Event{Kind: NoteOff, Channel: 1, Data1: 48}, true},
		{[]byte{0x8F, 60, 64}, Event{Kind: NoteOff, Channel: 16, Data1: 60, Data2: 64}, true},
		{[]byte{0xBF, 20, 127}, Event{Kind: CC, Channel: 16, Data1: 20, Data2: 127}, true},
		{[]byte{0xF8}, Event{Kind: Clock}, true},
		{[]byte{0xFA}, Event{Kind: Start}, true},
		{[]byte{0xFB}, Event{Kind: Continue}, true},
		{[]byte{0xFC}, Event{Kind: Stop}, true},
		{[]byte{0xE0, 0, 64}, Event{Kind: PitchBend, Channel: 1, Data1: 8192}, true},
		{[]byte{0xC2, 5}, Event{Kind: ProgramChange, Channel: 3, Data1: 5}, true},
		{[]byte{0xD0, 90}, Event{Kind: Pressure, Channel: 1, Data2: 90}, true},
		{[]byte{0xA0, 48, 70}, Event{Kind: Pressure, Channel: 1, Data1: 48, Data2: 70}, true},
		{[]byte{0xF0, 0x41, 0xF7}, Event{Kind: Sysex, Raw: []byte{0xF0, 0x41, 0xF7}}, true},
		{[]byte{0xFE}, Event{}, false}, // active sensing
		{[]byte{0x90, 48}, Event{}, false},
		{nil, Event{}, false},
	} {
		got, ok := Parse(c.in)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(% x) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestABuildWithoutMIDISaysSo(t *testing.T) {
	if _, _, err := (unavailable{}).Open("404", func(Event) {}); err != ErrUnavailable {
		t.Errorf("error %v", err)
	}
}
