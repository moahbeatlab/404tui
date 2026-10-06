package proto

import (
	"encoding/binary"
	"testing"
)

// Captured with Tempo Select = Project, bank A tempo 60.00, bank J tempo 120.00 and
// protected, bank A volume 80, bank J volume 80 (values from separate experiments,
// combined here).
func sampleSettings() []byte {
	var words [SettingsWords]uint32
	words[0] = 0x1_2738
	for i := 3; i < 6; i++ {
		words[i] = 64
	}
	for i := 7; i < 12; i++ {
		words[i] = 32
	}
	for i := 12; i < 22; i++ {
		words[i] = 18_000
	}
	words[12] = 12_000
	words[21] = 24_001
	words[23] = 47
	words[27] = 47 << 16
	p := []byte{0x7D, 0x05, 0x00}
	for _, w := range words {
		p = binary.LittleEndian.AppendUint32(p, w)
	}
	name := []byte("PROJECT_06                     \x00")
	return append(p, name...)
}

func TestDecodesPackedFields(t *testing.T) {
	s, err := ParseProjectSettings(sampleSettings())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Raw()) != 147 {
		t.Errorf("raw is %d bytes", len(s.Raw()))
	}
	if s.ProjectTempo() != 10_040 || !s.UsesProjectTempo() {
		t.Errorf("tempo = %d, project tempo = %v", s.ProjectTempo(), s.UsesProjectTempo())
	}
	for bank, want := range map[int]BankSettings{
		0: {Tempo: 6000, Volume: 80},
		1: {Tempo: 9000, Volume: 127},
		9: {Tempo: 12_000, Protected: true, Volume: 80},
	} {
		if got := s.Bank(bank); got != want {
			t.Errorf("bank %d = %+v, want %+v", bank, got, want)
		}
	}
	if s.Name() != "PROJECT_06" {
		t.Errorf("name = %q", s.Name())
	}
}
