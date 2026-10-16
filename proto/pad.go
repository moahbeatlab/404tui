package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Pad addressing and the pad parameter block.

const (
	Banks       = 10
	PadsPerBank = 16
	PadCount    = Banks * PadsPerBank
)

// PadIndex is a pad as the device indexes it: bank × 16 + pad, with A1 = 0 and J16 = 159.
type PadIndex struct {
	i uint16
}

func NewPadIndex(index uint16) (PadIndex, bool) {
	if index >= PadCount {
		return PadIndex{}, false
	}
	return PadIndex{index}, true
}

// PadFromBankPad takes bank 0..=9 (A..J) and pad 0..=15.
func PadFromBankPad(bank, pad uint16) (PadIndex, bool) {
	if bank >= Banks || pad >= PadsPerBank {
		return PadIndex{}, false
	}
	return PadIndex{bank*PadsPerBank + pad}, true
}

// ParsePad parses a pad name, A1..J16.
func ParsePad(s string) (PadIndex, error) {
	t := strings.TrimSpace(s)
	if len(t) < 2 {
		return PadIndex{}, BadPadError(s)
	}
	bank := t[0] &^ 0x20
	if bank < 'A' || bank > 'J' {
		return PadIndex{}, BadPadError(s)
	}
	pad, err := strconv.ParseUint(t[1:], 10, 16)
	if err != nil || pad < 1 || pad > PadsPerBank {
		return PadIndex{}, BadPadError(s)
	}
	return PadIndex{uint16(bank-'A')*PadsPerBank + uint16(pad) - 1}, nil
}

func (p PadIndex) Index() uint16 {
	return p.i
}

func (p PadIndex) Bank() uint16 {
	return p.i / PadsPerBank
}

func (p PadIndex) Pad() uint16 {
	return p.i % PadsPerBank
}

// AllPads lists every pad, A1 to J16.
func AllPads() []PadIndex {
	out := make([]PadIndex, PadCount)
	for i := range out {
		out[i] = PadIndex{uint16(i)}
	}
	return out
}

// SamplePath is the card-relative path of this pad's sample in a project (1-based project
// number).
func (p PadIndex) SamplePath(project uint8) string {
	return fmt.Sprintf("ROLAND/SP-404MKII/PROJECT_%02d/SMPL/BANK%d-%02d.SMP", project, p.Bank()+1, p.Pad()+1)
}

func (p PadIndex) String() string {
	return fmt.Sprintf("%c%d", 'A'+rune(p.Bank()), p.Pad()+1)
}

const (
	padBlockPrefix     = 3
	padBlockNameOffset = 175
	padBlockNameLen    = 24
	padBlockChopOffset = 199

	PadBlockChopPoints = 16
	// PadBlockWriteLen is the number of bytes the official app sends when writing a block
	// (everything up to the name).
	PadBlockWriteLen = padBlockChopOffset

	padFieldAbsEnd = 0
	padFieldStart  = 1
	padFieldEnd    = 2
	// Loop start as a byte offset (the device moves it along when start passes it).
	padFieldLoopStart = 11
)

// PadBlock is the pad parameter block (1E pad … on channel 5).
//
// After the 3-byte prefix the block is little-endian u32 fields; see
// docs/re/03-parameters.md for the layout.
type PadBlock struct {
	Pad PadIndex
	raw []byte
}

// ParsePadBlock parses a 1E payload (including its command byte).
func ParsePadBlock(payload []byte) (*PadBlock, error) {
	need := padBlockChopOffset + PadBlockChopPoints*4
	if len(payload) < need {
		return nil, TruncatedError{Need: need, Have: len(payload)}
	}
	if payload[0] != ReplyPadBlock {
		return nil, UnexpectedCommandError(payload[0])
	}
	index := binary.LittleEndian.Uint16(payload[1:3])
	pad, ok := NewPadIndex(index)
	if !ok {
		return nil, BadPadError(strconv.Itoa(int(index)))
	}
	return &PadBlock{Pad: pad, raw: bytes.Clone(payload)}, nil
}

// Field is raw u32 field n (0-based, counted from the end of the prefix).
func (b *PadBlock) Field(n int) uint32 {
	return binary.LittleEndian.Uint32(b.raw[padBlockPrefix+n*4:])
}

func (b *PadBlock) SetField(n int, value uint32) {
	binary.LittleEndian.PutUint32(b.raw[padBlockPrefix+n*4:], value)
}

// HasSample reports whether the pad holds a sample.
func (b *PadBlock) HasSample() bool {
	return b.Field(padFieldAbsEnd) != 0
}

// FileSize is the size of the SMP file in bytes.
func (b *PadBlock) FileSize() uint32 {
	return b.Field(padFieldAbsEnd)
}

// StartEndBytes are start and end as byte offsets into the SMP file.
func (b *PadBlock) StartEndBytes() (start, end uint32) {
	return b.Field(padFieldStart), b.Field(padFieldEnd)
}

// Param is a parameter value by set-parameter id, for ids whose position is verified.
func (b *PadBlock) Param(id byte) (int32, bool) {
	n, ok := blockField(id)
	if !ok {
		return 0, false
	}
	return int32(b.Field(n)), true
}

func (b *PadBlock) Name() string {
	name := b.raw[padBlockNameOffset : padBlockNameOffset+padBlockNameLen]
	if end := bytes.IndexByte(name, 0); end >= 0 {
		name = name[:end]
	}
	return strings.TrimRightFunc(strings.ToValidUTF8(string(name), "�"), unicode.IsSpace)
}

func (b *PadBlock) SetName(name string) {
	field := b.raw[padBlockNameOffset : padBlockNameOffset+padBlockNameLen]
	clear(field)
	copy(field[:padBlockNameLen-1], printable(name))
}

// ChopPoints are the chop points in sample frames; negative for unused slots.
func (b *PadBlock) ChopPoints() []int32 {
	out := make([]int32, PadBlockChopPoints)
	for i := range out {
		out[i] = int32(binary.LittleEndian.Uint32(b.raw[padBlockChopOffset+i*4:]))
	}
	return out
}

// HoldDefault is the Hold of a pad whose sample was just assigned: the whole sample plays.
const HoldDefault = 100

// AssignSample points the block at a freshly written SMP file of fileSize bytes, as the
// official app does on import.
func (b *PadBlock) AssignSample(fileSize uint32, name string) {
	b.SetField(padFieldAbsEnd, fileSize)
	b.SetField(padFieldStart, SMPHeaderLen)
	b.SetField(padFieldEnd, fileSize)
	b.SetField(padFieldLoopStart, SMPHeaderLen)
	// A new sample starts with Hold at 100, all of it: a pad made offline has none set, and at 1
	// the pad plays a sliver of its sample when it is pressed.
	if n, ok := blockField(0x7E); ok {
		b.SetField(n, HoldDefault)
	}
	b.SetName(name)
}

// Raw is the whole reply payload as received.
func (b *PadBlock) Raw() []byte {
	return b.raw
}

// LoopStartBytes is the loop start as a byte offset into the SMP file.
func (b *PadBlock) LoopStartBytes() uint32 {
	return b.Field(padFieldLoopStart)
}

// WritePayload is the payload to send when writing the block back to the device.
func (b *PadBlock) WritePayload() []byte {
	return bytes.Clone(b.raw[:PadBlockWriteLen])
}

// printable keeps the printable ASCII bytes of s, the only ones the device accepts in names.
func printable(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x20 && s[i] < 0x7f {
			out = append(out, s[i])
		}
	}
	return out
}
