// Package septet encodes the integers inside file-API payloads: 5 septets, big-endian,
// 35 bits.
//
// Plain values are signed 32-bit integers sign-extended to 35 bits, so −1 is
// 7F 7F 7F 7F 7F. Data-length fields reuse the three bits above bit 31 as flags.
package septet

import "github.com/moahbeatlab/404tui/proto"

// Len is the encoded length of one integer.
const Len = 5

const (
	// FlagLast is the first-septet flag on a data length: last read chunk, or last write
	// chunk of a batch.
	FlagLast byte = 0x40
	// FlagMulti is the first-septet flag the host sets on read requests larger than one chunk.
	FlagMulti byte = 0x20
)

// Encode encodes a signed value (sign-extended to 35 bits).
func Encode(value int32) [Len]byte {
	return encodeRaw(uint64(int64(value)) & 0x7_FFFF_FFFF)
}

// EncodeLen encodes an unsigned length with first-septet flags (FlagLast, FlagMulti).
func EncodeLen(value uint32, flags byte) [Len]byte {
	out := encodeRaw(uint64(value))
	out[0] |= flags
	return out
}

func encodeRaw(raw uint64) [Len]byte {
	var out [Len]byte
	for i := range out {
		out[i] = byte(raw>>(7*(Len-1-i))) & 0x7f
	}
	return out
}

// Septets is a decoded 35-bit integer, split into the low 32 bits and the 3 bits above them.
type Septets struct {
	Low  uint32
	High uint8
}

// Int32 is the value as a signed 32-bit integer (for results, handles, offsets).
func (s Septets) Int32() int32 {
	return int32(s.Low)
}

// Uint32 is the low 32 bits as an unsigned value (lengths), ignoring flags.
func (s Septets) Uint32() uint32 {
	return s.Low
}

// IsLast reports whether the FlagLast bit is set (only meaningful on data lengths).
func (s Septets) IsLast() bool {
	return s.High&0b100 != 0
}

// Decode decodes 5 septets from the start of b.
func Decode(b []byte) (Septets, error) {
	if len(b) < Len {
		return Septets{}, proto.TruncatedError{Need: Len, Have: len(b)}
	}
	var raw uint64
	for _, c := range b[:Len] {
		if c&0x80 != 0 {
			return Septets{}, proto.BadSeptetError(c)
		}
		raw = raw<<7 | uint64(c)
	}
	return Septets{Low: uint32(raw), High: uint8(raw >> 32)}, nil
}
