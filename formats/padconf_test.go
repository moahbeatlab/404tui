package formats_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/moahbeatlab/404tui/formats"
)

// padconfFile builds a version 3 file the way the docs lay it out, with a few values set.
func padconfFile() []byte {
	b := make([]byte, formats.PadconfLen)
	copy(b, "RFPD")
	b[4], b[8] = 0xA0, 3
	binary.BigEndian.PutUint32(b[0x10:], 9_500)         // project tempo
	binary.BigEndian.PutUint32(b[0x10+4*14:], 9_000<<1) // bank C tempo
	copy(b[0x80:], "MY BEAT")
	// Pad A2 (index 1): sample size 4096, attack 7 (field 22).
	binary.BigEndian.PutUint32(b[0xA0+(1*43+0)*4:], 4096)
	binary.BigEndian.PutUint32(b[0xA0+(1*43+22)*4:], 7)
	copy(b[0x6C20+1*24:], "snare                  ")
	// Its first chop point at frame 1000; the rest unused.
	for w := 0; w < 16; w++ {
		v := uint32(0xffffffff)
		if w == 0 {
			v = 1000
		}
		binary.BigEndian.PutUint32(b[0x7B20+(1*32+w)*4:], v)
	}
	// A reserved word after pad A6's chop points, not understood, which has to survive.
	b[0x7B20+(5*32+20)*4+3] = 0x5A
	return b
}

func TestPadconfReadsAndWritesTheFile(t *testing.T) {
	file := padconfFile()
	p, err := formats.ParsePadconf(file)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "MY BEAT" || p.Settings[0] != 9_500 || p.Settings[14] != 9_000<<1 {
		t.Errorf("settings: %q %v", p.Name, p.Settings[:15])
	}
	block := p.Blocks[1]
	if len(block) != formats.PadBlockLen || block[0] != 0x1E || binary.LittleEndian.Uint16(block[1:]) != 1 {
		t.Fatalf("block header: % x", block[:3])
	}
	if size := binary.LittleEndian.Uint32(block[3:]); size != 4096 {
		t.Errorf("size %d", size)
	}
	if attack := binary.LittleEndian.Uint32(block[3+22*4:]); attack != 7 {
		t.Errorf("attack %d", attack)
	}
	if name := string(bytes.TrimRight(block[175:199], " \x00")); name != "snare" {
		t.Errorf("name %q", name)
	}
	if chop := binary.LittleEndian.Uint32(block[199:]); chop != 1000 {
		t.Errorf("chop %d", chop)
	}
	// Unchanged, it is written back byte for byte.
	if !bytes.Equal(p.Bytes(), file) {
		t.Error("the file did not round-trip")
	}
	// A change lands where the file keeps it.
	binary.LittleEndian.PutUint32(p.Blocks[1][3+22*4:], 9)
	p.Name = "RENAMED"
	out := p.Bytes()
	if binary.BigEndian.Uint32(out[0xA0+(1*43+22)*4:]) != 9 || !bytes.HasPrefix(out[0x80:], []byte("RENAMED\x00")) {
		t.Error("the changes were not written")
	}
	if out[0x7B20+(5*32+20)*4+3] != 0x5A {
		t.Error("a reserved word was lost")
	}
}

func TestPadconfVersion2(t *testing.T) {
	b := make([]byte, 31_488)
	copy(b, "RFPD")
	b[4], b[8] = 0xA0, 2
	binary.BigEndian.PutUint32(b[0x80+(2*43)*4:], 512)
	copy(b[0x6C00+2*24:], "hat")
	p, err := formats.ParsePadconf(b)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(p.Blocks[2][3:]) != 512 || !bytes.HasPrefix(p.Blocks[2][175:], []byte("hat")) {
		t.Error("version 2 pads were not read")
	}
	if len(p.Bytes()) != formats.PadconfLen || p.Bytes()[8] != 3 {
		t.Error("a version 2 file is not written as version 3")
	}
	if _, err := formats.ParsePadconf([]byte("nope")); err == nil {
		t.Error("a file that is not a PADCONF.BIN was read")
	}
}

func TestNewPadconfFollowsItsTemplate(t *testing.T) {
	template, err := formats.ParsePadconf(padconfFile())
	if err != nil {
		t.Fatal(err)
	}
	// Pad A1 is empty in the template; its field 9 says what an empty pad holds.
	binary.LittleEndian.PutUint32(template.Blocks[0][3+9*4:], 9600)
	p := formats.NewPadconf(template, "FRESH")
	if p.Name != "FRESH" || p.Settings != template.Settings {
		t.Errorf("settings or name not taken: %q", p.Name)
	}
	if binary.LittleEndian.Uint32(p.Blocks[1][3:]) != 0 || binary.LittleEndian.Uint32(p.Blocks[1][3+9*4:]) != 9600 {
		t.Error("pads are not the template's empty pad")
	}
	// Without a template: a valid file with the documented defaults.
	bare := formats.NewPadconf(nil, "BARE")
	back, err := formats.ParsePadconf(bare.Bytes())
	if err != nil || back.Name != "BARE" || back.Settings[0] != 12_000 {
		t.Errorf("bare file: %v %+v", err, back)
	}
}
