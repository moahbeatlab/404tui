package proto

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPadNames(t *testing.T) {
	for name, want := range map[string]uint16{"A1": 0, "b1": 16, "J16": 159} {
		if got := mustPad(t, name).Index(); got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	for _, name := range []string{"K1", "A17"} {
		if _, err := ParsePad(name); err == nil {
			t.Errorf("%s should not parse", name)
		}
	}
	j16, _ := NewPadIndex(159)
	if j16.String() != "J16" {
		t.Errorf("String = %s", j16)
	}
	b1, _ := NewPadIndex(16)
	if got, want := b1.SamplePath(6), "ROLAND/SP-404MKII/PROJECT_06/SMPL/BANK2-01.SMP"; got != want {
		t.Errorf("SamplePath = %s", got)
	}
}

func emptyBlock(pad uint16) []byte {
	p := make([]byte, 327)
	p[0] = 0x1E
	binary.LittleEndian.PutUint16(p[1:], pad)
	copy(p[175:198], bytes.Repeat([]byte{' '}, 23))
	copy(p[199:263], bytes.Repeat([]byte{0xff}, 64))
	return p
}

func TestBlockFieldsAndImportPatch(t *testing.T) {
	block, err := ParsePadBlock(emptyBlock(159))
	if err != nil {
		t.Fatal(err)
	}
	if block.HasSample() {
		t.Error("empty block reports a sample")
	}
	if block.Name() != "" {
		t.Errorf("name = %q", block.Name())
	}
	for i, p := range block.ChopPoints() {
		if p >= 0 {
			t.Errorf("chop point %d = %d, want unused", i, p)
		}
	}

	block.AssignSample(576_512, "spmk2_test_48k_stereo")
	if start, end := block.StartEndBytes(); start != 512 || end != 576_512 {
		t.Errorf("start/end = %d/%d", start, end)
	}
	if hold, ok := block.Param(0x7E); !ok || hold != HoldDefault {
		t.Errorf("hold after an assign is %d, want %d", hold, HoldDefault)
	}
	if block.Name() != "spmk2_test_48k_stereo" {
		t.Errorf("name = %q", block.Name())
	}
	out := block.WritePayload()
	if len(out) != 199 {
		t.Fatalf("write payload is %d bytes", len(out))
	}
	if !bytes.Equal(out[:3], []byte{0x1E, 0x9F, 0x00}) {
		t.Errorf("prefix = % x", out[:3])
	}
	want := []byte{0x00, 0xcc, 0x08, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0xcc, 0x08, 0x00}
	if !bytes.Equal(out[3:15], want) {
		t.Errorf("fields = % x, want % x", out[3:15], want)
	}
}
