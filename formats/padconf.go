package formats

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// PADCONF.BIN is a project's pad and project settings as the unit saves them (docs/re/
// 02-files.md of SparkyMK2): the same structures it sends over the serial link, with the
// words big-endian. Version 3 is 52000 bytes; version 2, from older firmware, has no project
// name and no chop points.
const (
	PadconfLen   = 52_000
	padconfV2Len = 31_488

	// PadBlockLen is a pad block as the unit sends it: 1E, the pad, 43 words, the name, 16
	// chop points and 16 more words.
	PadBlockLen = 327

	padFields     = 43
	padNameLen    = 24
	padChopWords  = 32 // 16 chop points, then 16 reserved words
	blockFields   = 3
	blockName     = blockFields + padFields*4
	blockChops    = blockName + padNameLen
	padconfPads   = 160
	settingsWords = 28
	settingsAt    = 0x10
	projectNameAt = 0x80
	projectName   = 32
)

// padconfLayout is where each part of a version of the file starts.
type padconfLayout struct {
	length, records, names, chops int
}

var (
	padconfV3 = padconfLayout{length: PadconfLen, records: 0xA0, names: 0x6C20, chops: 0x7B20}
	padconfV2 = padconfLayout{length: padconfV2Len, records: 0x80, names: 0x6C00, chops: -1}
)

// Padconf is a PADCONF.BIN read into the form the unit sends live: pad blocks as the 1E
// reply has them (little-endian) and the 28 project settings words.
type Padconf struct {
	Settings [settingsWords]uint32
	// Name is the project's name; empty in a version 2 file.
	Name   string
	Blocks [padconfPads][]byte
	// raw is the file it was read from (as version 3), so that what is not understood is
	// written back as it was.
	raw []byte
}

// ParsePadconf reads a PADCONF.BIN of version 2 or 3.
func ParsePadconf(b []byte) (*Padconf, error) {
	if len(b) < 16 || !bytes.Equal(b[:4], []byte("RFPD")) {
		return nil, errors.New("not a PADCONF.BIN: no RFPD at the start")
	}
	layout := padconfV3
	switch v := b[8]; {
	case v == 2:
		layout = padconfV2
	case v != 3:
		return nil, fmt.Errorf("PADCONF.BIN version %d is not known", v)
	}
	if len(b) < layout.length {
		return nil, fmt.Errorf("PADCONF.BIN is %d bytes, version %d has %d", len(b), b[8], layout.length)
	}
	p := &Padconf{}
	for i := range p.Settings {
		p.Settings[i] = binary.BigEndian.Uint32(b[settingsAt+4*i:])
	}
	if layout == padconfV3 {
		p.Name = string(bytes.TrimRight(b[projectNameAt:projectNameAt+projectName], "\x00 "))
		p.raw = bytes.Clone(b[:PadconfLen])
	}
	for pad := range p.Blocks {
		block := EmptyPadBlock(uint16(pad))
		for f := 0; f < padFields; f++ {
			binary.LittleEndian.PutUint32(block[blockFields+4*f:], binary.BigEndian.Uint32(b[layout.records+(pad*padFields+f)*4:]))
		}
		copy(block[blockName:blockName+padNameLen], b[layout.names+pad*padNameLen:])
		if layout.chops >= 0 {
			for w := 0; w < padChopWords; w++ {
				binary.LittleEndian.PutUint32(block[blockChops+4*w:], binary.BigEndian.Uint32(b[layout.chops+(pad*padChopWords+w)*4:]))
			}
		}
		p.Blocks[pad] = block
	}
	return p, nil
}

// EmptyPadBlock is the block of a pad with no sample: no fields set, a blank name and no
// chop points.
func EmptyPadBlock(pad uint16) []byte {
	b := make([]byte, PadBlockLen)
	b[0] = 0x1E
	binary.LittleEndian.PutUint16(b[1:], pad)
	copy(b[blockName:blockName+padNameLen-1], bytes.Repeat([]byte{' '}, padNameLen-1))
	copy(b[blockChops:blockChops+16*4], bytes.Repeat([]byte{0xff}, 16*4))
	return b
}

// NewPadconf is the file of a new, empty project named name. With a template (a project read
// from a real unit) it keeps the template's settings and makes every pad like one of the
// template's empty pads, so the parts that are not understood are what the unit writes
// itself. Without one, they are the values documented for a new project, which are not
// all known.
func NewPadconf(template *Padconf, name string) *Padconf {
	p := &Padconf{Name: name}
	empty := []uint32(nil)
	if template != nil {
		p.Settings = template.Settings
		for _, b := range template.Blocks {
			if binary.LittleEndian.Uint32(b[blockFields:]) == 0 {
				empty = make([]uint32, padFields)
				for f := range empty {
					empty[f] = binary.LittleEndian.Uint32(b[blockFields+4*f:])
				}
				break
			}
		}
		if template.raw != nil {
			p.raw = bytes.Clone(template.raw)
		}
	} else {
		// Project tempo 120 BPM; the words the docs saw as 64 and 32; every bank at 120 BPM.
		p.Settings[0] = 12_000
		for i := 3; i <= 5; i++ {
			p.Settings[i] = 64
		}
		for i := 7; i <= 11; i++ {
			p.Settings[i] = 32
		}
		for bank := 0; bank < 10; bank++ {
			p.Settings[12+bank] = 12_000 << 1
		}
		// An empty pad in a saved file: field 9 is 9600, the rest 0.
		empty = make([]uint32, padFields)
		empty[9] = 9600
	}
	for pad := range p.Blocks {
		block := EmptyPadBlock(uint16(pad))
		for f, v := range empty {
			binary.LittleEndian.PutUint32(block[blockFields+4*f:], v)
		}
		p.Blocks[pad] = block
	}
	return p
}

// Bytes is the file, version 3.
func (p *Padconf) Bytes() []byte {
	b := p.raw
	if b == nil {
		b = make([]byte, PadconfLen)
		copy(b, "RFPD")
		binary.LittleEndian.PutUint32(b[4:], 0xA0)
		b[8] = 3
		binary.LittleEndian.PutUint32(b[12:], 0x7A80)
	} else {
		b = bytes.Clone(b)
	}
	for i, w := range p.Settings {
		binary.BigEndian.PutUint32(b[settingsAt+4*i:], w)
	}
	name := b[projectNameAt : projectNameAt+projectName]
	clear(name)
	copy(name, p.Name)
	l := padconfV3
	for pad, block := range p.Blocks {
		if len(block) < PadBlockLen {
			block = EmptyPadBlock(uint16(pad))
		}
		for f := 0; f < padFields; f++ {
			binary.BigEndian.PutUint32(b[l.records+(pad*padFields+f)*4:], binary.LittleEndian.Uint32(block[blockFields+4*f:]))
		}
		copy(b[l.names+pad*padNameLen:l.names+(pad+1)*padNameLen], block[blockName:blockName+padNameLen])
		for w := 0; w < padChopWords; w++ {
			binary.BigEndian.PutUint32(b[l.chops+(pad*padChopWords+w)*4:], binary.LittleEndian.Uint32(block[blockChops+4*w:]))
		}
	}
	return b
}
