package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Message framing on the serial link.
//
// Short messages are 12 bytes: up to 7 payload bytes inline plus a length byte (the
// device ignores a short message whose length byte is 0). Long messages have a 16-byte
// header followed by a length-prefixed payload. Each direction is a byte stream: messages
// may be split across or packed into USB transfers.
const (
	KindShort  byte = 0x12
	KindLong   byte = 0x13
	AddrHost   byte = 0x60
	AddrDevice byte = 0xE0
	ShortLen        = 12
	// ShortMax is the largest payload of a short message.
	ShortMax      = 7
	LongHeaderLen = 16

	// Largest long payload accepted by the decoder before it treats the header as garbage.
	maxPayload = 1 << 24
)

// Channel is the channel byte of a message; values other than the named ones pass through.
type Channel byte

const (
	// ChannelControl carries control and parameters.
	ChannelControl Channel = 0x05
	// ChannelFile carries the remote file API.
	ChannelFile Channel = 0x06
)

// Message is one framed message, short or long.
type Message struct {
	Source      byte
	Destination byte
	Channel     Channel

	short bool
	// The 8 raw bytes of a short message: payload followed by padding, length in the last byte.
	raw  [8]byte
	long []byte
}

// Short builds a host-to-device short message. It panics if payload exceeds ShortMax bytes.
func Short(channel Channel, payload []byte) Message {
	if len(payload) > ShortMax {
		panic(fmt.Sprintf("short payload of %d bytes", len(payload)))
	}
	m := Message{Source: AddrHost, Destination: AddrDevice, Channel: channel, short: true}
	copy(m.raw[:], payload)
	m.raw[7] = byte(len(payload))
	return m
}

// Long builds a host-to-device long message.
func Long(channel Channel, payload []byte) Message {
	return Message{Source: AddrHost, Destination: AddrDevice, Channel: channel, long: payload}
}

func (m *Message) Payload() []byte {
	if m.short {
		return m.raw[:min(int(m.raw[7]), ShortMax)]
	}
	return m.long
}

func (m *Message) IsShort() bool {
	return m.short
}

func (m *Message) Equal(o *Message) bool {
	return m.Source == o.Source && m.Destination == o.Destination && m.Channel == o.Channel &&
		m.short == o.short && m.raw == o.raw && bytes.Equal(m.long, o.long)
}

func (m *Message) Encode() []byte {
	head := []byte{m.Source, m.Destination, byte(m.Channel)}
	if m.short {
		out := make([]byte, 0, ShortLen)
		out = append(out, KindShort)
		out = append(out, head...)
		return append(out, m.raw[:]...)
	}
	out := make([]byte, 0, LongHeaderLen+len(m.long))
	out = append(out, KindLong)
	out = append(out, head...)
	// The value and tag fields are unused by the device; send zeros.
	out = append(out, make([]byte, 8)...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(m.long)))
	return append(out, m.long...)
}

// Decoder is an incremental decoder for one direction of the byte stream.
type Decoder struct {
	buf     []byte
	skipped int
}

func (d *Decoder) Push(b []byte) {
	d.buf = append(d.buf, b...)
}

// Skipped is the number of bytes discarded while resynchronising.
func (d *Decoder) Skipped() int {
	return d.skipped
}

// Next returns the next complete message, if the buffer holds one.
func (d *Decoder) Next() (Message, bool) {
	for {
		if len(d.buf) == 0 {
			return Message{}, false
		}
		kind := d.buf[0]
		var total int
		switch kind {
		case KindShort:
			total = ShortLen
		case KindLong:
			if len(d.buf) < LongHeaderLen {
				return Message{}, false
			}
			n := binary.LittleEndian.Uint32(d.buf[12:16])
			if n > maxPayload {
				d.drop(1)
				d.skipped++
				continue
			}
			total = LongHeaderLen + int(n)
		default:
			d.drop(1)
			d.skipped++
			continue
		}
		if len(d.buf) < total {
			return Message{}, false
		}
		m := Message{Source: d.buf[1], Destination: d.buf[2], Channel: Channel(d.buf[3])}
		if kind == KindShort {
			m.short = true
			copy(m.raw[:], d.buf[4:12])
		} else {
			m.long = bytes.Clone(d.buf[LongHeaderLen:total])
			if m.long == nil {
				m.long = []byte{}
			}
		}
		d.drop(total)
		return m, true
	}
}

func (d *Decoder) drop(n int) {
	d.buf = d.buf[:copy(d.buf, d.buf[n:])]
}
