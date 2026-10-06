// Package formats reads and writes the SP-404MKII's card files (SMP samples, patterns)
// and decodes audio files into samples the unit can hold. Layouts are specified in
// docs/re/02-files.md of SparkyMK2.
package formats

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// HeaderLen is the length of an SMP file's RFWV header.
	HeaderLen = 512
	// SampleRate is the only rate the unit stores.
	SampleRate = 48_000
	bits       = 16

	// The size field is the PCM bytes plus this (the header minus magic and size field).
	sizeBias = HeaderLen - 8
)

var magic = []byte("RFWV")

// Errors a file can fail with.
var (
	ErrBadMagic       = errors.New("wrong file type (magic bytes do not match)")
	ErrDigestMismatch = errors.New("SMP header digest does not match its fields")
	ErrEmpty          = errors.New("file has no audio")
	ErrNoAudioTrack   = errors.New("no audio track found")
)

// Sample is interleaved 16-bit audio at 48 kHz, mono or stereo.
type Sample struct {
	Channels int
	Samples  []int16
}

// Frames is the number of sample frames.
func (s Sample) Frames() int {
	return len(s.Samples) / max(s.Channels, 1)
}

// ParseSMP decodes an SMP file, verifying magic, format, size and digest.
func ParseSMP(b []byte) (Sample, error) {
	info, err := ParseHeader(b)
	if err != nil {
		return Sample{}, err
	}
	pcm := b[HeaderLen:]
	if int(info.PCMLen) != len(pcm) {
		return Sample{}, fmt.Errorf("SMP size field says %d bytes of audio, file has %d", info.PCMLen, len(pcm))
	}
	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.BigEndian.Uint16(pcm[2*i:]))
	}
	return Sample{Channels: info.Channels, Samples: samples}, nil
}

// SMP encodes the sample as an SMP file.
func (s Sample) SMP() []byte {
	out := make([]byte, 0, HeaderLen+len(s.Samples)*2)
	h := header(uint32(len(s.Samples)*2), uint32(s.Channels))
	out = append(out, h[:]...)
	for _, v := range s.Samples {
		out = binary.BigEndian.AppendUint16(out, uint16(v))
	}
	return out
}

// HeaderInfo holds the format fields of an SMP header.
type HeaderInfo struct {
	Channels int
	// PCMLen is the PCM bytes after the header, per the size field.
	PCMLen uint32
}

// ParseHeader parses and verifies the first 512 bytes of an SMP file.
func ParseHeader(h []byte) (HeaderInfo, error) {
	if len(h) < HeaderLen {
		return HeaderInfo{}, fmt.Errorf("file too short (%d bytes)", len(h))
	}
	if !bytes.Equal(h[:4], magic) {
		return HeaderInfo{}, ErrBadMagic
	}
	be := func(at int) uint32 { return binary.BigEndian.Uint32(h[at:]) }
	size, rate, channels, depth := be(4), be(8), be(12), be(16)
	if rate != SampleRate || depth != bits || channels < 1 || channels > 2 {
		return HeaderInfo{}, fmt.Errorf("unsupported SMP format: %d Hz, %d channels, %d bits", rate, channels, depth)
	}
	if !digestMatches(h) {
		return HeaderInfo{}, ErrDigestMismatch
	}
	return HeaderInfo{Channels: int(channels), PCMLen: size - min(size, sizeBias)}, nil
}

// Frames is the number of sample frames the header declares.
func (i HeaderInfo) Frames() uint32 {
	return i.PCMLen / (2 * uint32(i.Channels))
}

// FrameAt is the frame number of a byte offset into the file (as stored in pad blocks).
func (i HeaderInfo) FrameAt(byteOffset uint32) uint32 {
	return (byteOffset - min(byteOffset, HeaderLen)) / (2 * uint32(i.Channels))
}

// ByteAt is the byte offset into the file of a frame number.
func (i HeaderInfo) ByteAt(frame uint32) uint32 {
	return HeaderLen + frame*2*uint32(i.Channels)
}

func header(pcmLen, channels uint32) [HeaderLen]byte {
	var h [HeaderLen]byte
	copy(h[:4], magic)
	binary.BigEndian.PutUint32(h[4:], pcmLen+sizeBias)
	binary.BigEndian.PutUint32(h[8:], SampleRate)
	binary.BigEndian.PutUint32(h[12:], channels)
	binary.BigEndian.PutUint32(h[16:], bits)
	digest := md5.Sum(h[4:20])
	copy(h[0x20:], digest[:])
	fill(h[0x30:], digest[:])
	return h
}

// digestMatches applies the digest rule: MD5 over the four big-endian fields after the magic.
func digestMatches(h []byte) bool {
	if len(h) < 0x30 {
		return false
	}
	digest := md5.Sum(h[4:20])
	return bytes.Equal(digest[:], h[0x20:0x30])
}

// fill writes the header's filler, which has no meaning: deterministic pseudo-random
// bytes, so output is reproducible.
func fill(out, seed []byte) {
	x := binary.LittleEndian.Uint64(seed[:8]) | 1
	for i := range out {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		out[i] = byte(x >> 32)
	}
}
