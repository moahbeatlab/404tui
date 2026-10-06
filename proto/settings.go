package proto

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// Project settings (7D proj 00 … reply on channel 5, and the header of PADCONF.BIN).
//
// After the 3-byte prefix come 28 little-endian u32 words and a 32-byte project name.
// Several words pack more than one value; see docs/re/03-parameters.md.
const (
	SettingsPrefix     = 3
	SettingsWords      = 28
	SettingsNameOffset = SettingsPrefix + SettingsWords*4
	SettingsNameLen    = 32
	SettingsLen        = SettingsNameOffset + SettingsNameLen

	settingsTempoWord   = 0
	settingsBankWords   = 12
	settingsVolumeWords = 23
)

// BankSettings are one bank's settings.
type BankSettings struct {
	// BPM × 100.
	Tempo     uint16
	Protected bool
	// 0..=127 as shown by the app (the device stores 127 − volume).
	Volume uint8
}

// ProjectSettings are decoded project settings.
type ProjectSettings struct {
	raw []byte
}

// ParseProjectSettings parses a 7D payload (including its command byte).
func ParseProjectSettings(payload []byte) (*ProjectSettings, error) {
	if len(payload) < SettingsLen {
		return nil, TruncatedError{Need: SettingsLen, Have: len(payload)}
	}
	if payload[0] != ReplyProjectSettings {
		return nil, UnexpectedCommandError(payload[0])
	}
	return &ProjectSettings{raw: bytes.Clone(payload[:SettingsLen])}, nil
}

func (s *ProjectSettings) Raw() []byte {
	return s.raw
}

// Project is the project these settings belong to (0-based).
func (s *ProjectSettings) Project() uint8 {
	return s.raw[1]
}

// Word is raw word n (0-based, after the prefix).
func (s *ProjectSettings) Word(n int) uint32 {
	return binary.LittleEndian.Uint32(s.raw[SettingsPrefix+n*4:])
}

// ProjectTempo is the project tempo, BPM × 100.
func (s *ProjectSettings) ProjectTempo() uint16 {
	return uint16(s.Word(settingsTempoWord))
}

// UsesProjectTempo is Tempo Select: true = Project tempo, false = Bank tempo.
func (s *ProjectSettings) UsesProjectTempo() bool {
	return s.Word(settingsTempoWord)&(1<<16) != 0
}

// Bank takes bank 0..=9 (A..J) and panics outside that range.
func (s *ProjectSettings) Bank(bank int) BankSettings {
	if bank < 0 || bank >= Banks {
		panic("bank out of range")
	}
	word := s.Word(settingsBankWords + bank)
	volumes := s.Word(settingsVolumeWords + bank/2)
	stored := uint16(volumes >> (16 * (bank % 2)))
	return BankSettings{
		Tempo:     uint16(word >> 1),
		Protected: word&1 != 0,
		Volume:    127 - uint8(min(stored, 127)),
	}
}

func (s *ProjectSettings) Name() string {
	name := s.raw[SettingsNameOffset : SettingsNameOffset+SettingsNameLen]
	return strings.Trim(strings.ToValidUTF8(string(name), "�"), "\x00 ")
}
