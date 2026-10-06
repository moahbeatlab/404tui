package septet

import (
	"errors"
	"math"
	"testing"

	"github.com/moahbeatlab/404tui/proto"
)

// A device-side pointer used as a directory handle.
const pointer = 0x8043_CDE8

func TestSpecExamples(t *testing.T) {
	ptr := uint32(pointer)
	cases := []struct {
		got, want [Len]byte
	}{
		{Encode(-1), [Len]byte{0x7f, 0x7f, 0x7f, 0x7f, 0x7f}},
		{Encode(0x2b), [Len]byte{0, 0, 0, 0, 0x2b}},
		{Encode(512), [Len]byte{0, 0, 0, 0x04, 0x00}},
		{Encode(int32(ptr)), [Len]byte{0x78, 0x02, 0x0f, 0x1b, 0x68}},
		{EncodeLen(512, FlagLast), [Len]byte{0x40, 0, 0, 0x04, 0x00}},
		{EncodeLen(0x20000, FlagMulti), [Len]byte{0x20, 0, 0x08, 0, 0}},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got % x, want % x", i, c.got, c.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	ptr := uint32(pointer)
	for _, v := range []int32{0, 1, 127, 128, 20480, -1, math.MinInt32, math.MaxInt32, int32(ptr)} {
		enc := Encode(v)
		d, err := Decode(enc[:])
		if err != nil || d.Int32() != v {
			t.Errorf("round trip of %d gave %d (%v)", v, d.Int32(), err)
		}
	}
	d, err := Decode([]byte{0x40, 0, 0, 0x04, 0x00})
	if err != nil || d.Uint32() != 512 || !d.IsLast() {
		t.Errorf("flagged length decoded as %+v (%v)", d, err)
	}
	d, err = Decode([]byte{0, 0, 0x01, 0x20, 0})
	if err != nil || d.IsLast() {
		t.Errorf("plain length decoded as %+v (%v)", d, err)
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, err := Decode([]byte{0x80, 0, 0, 0, 0}); err != proto.BadSeptetError(0x80) {
		t.Errorf("err = %v, want bad septet 0x80", err)
	}
	var truncated proto.TruncatedError
	if _, err := Decode([]byte{0, 0}); !errors.As(err, &truncated) {
		t.Errorf("err = %v, want truncated", err)
	}
}
