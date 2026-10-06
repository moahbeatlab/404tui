package proto

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestShortAndLongRoundTripAcrossSplitInput(t *testing.T) {
	a := Short(ChannelControl, []byte{0xfe, 0x66, 0x00})
	if got, want := a.Encode(), []byte{0x12, 0x60, 0xe0, 0x05, 0xfe, 0x66, 0, 0, 0, 0, 0, 0x03}; !bytes.Equal(got, want) {
		t.Fatalf("short encode = % x, want % x", got, want)
	}
	if got := a.Payload(); !bytes.Equal(got, []byte{0xfe, 0x66, 0x00}) {
		t.Fatalf("payload = % x", got)
	}
	b := Long(ChannelFile, []byte{0xf0, 0x41, 0x7a, 0x03, 0xf7})
	stream := append(a.Encode(), b.Encode()...)
	if len(stream) != 12+16+5 {
		t.Fatalf("stream is %d bytes", len(stream))
	}

	var dec Decoder
	for len(stream) > 0 {
		n := min(7, len(stream))
		dec.Push(stream[:n])
		stream = stream[n:]
	}
	for _, want := range []Message{a, b} {
		got, ok := dec.Next()
		if !ok || !got.Equal(&want) {
			t.Fatalf("decoded %+v (ok=%v), want %+v", got, ok, want)
		}
	}
	if _, ok := dec.Next(); ok {
		t.Fatal("decoder produced a third message")
	}
}

func TestLongHeaderLayout(t *testing.T) {
	m := Long(ChannelControl, []byte{1, 2, 3})
	b := m.Encode()
	if !bytes.Equal(b[:4], []byte{0x13, 0x60, 0xe0, 0x05}) {
		t.Fatalf("header = % x", b[:4])
	}
	if got := binary.LittleEndian.Uint32(b[12:16]); got != 3 {
		t.Fatalf("length field = %d", got)
	}
}

func TestResynchronisesAfterGarbage(t *testing.T) {
	msg := Short(ChannelControl, bytes.Repeat([]byte{1}, 7))
	var dec Decoder
	dec.Push([]byte{0x00, 0xff})
	dec.Push(msg.Encode())
	got, ok := dec.Next()
	if !ok || !got.Equal(&msg) {
		t.Fatalf("decoded %+v (ok=%v)", got, ok)
	}
	if dec.Skipped() != 2 {
		t.Fatalf("skipped = %d, want 2", dec.Skipped())
	}
}
