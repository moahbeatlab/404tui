package app

import (
	"fmt"
	"math"

	"github.com/moahbeatlab/404tui/proto"
)

// Bank tempo is BPM × 100 within the same range as the project tempo; bank volume is 0..127
// as the unit shows it (it stores 127 minus that).
const (
	MinBankBPM    = 40.0
	MaxBankBPM    = 200.0
	MaxBankVolume = 127
)

// BankInfo is one bank's settings.
type BankInfo struct {
	// Tempo is in BPM.
	Tempo  float64
	Volume int
}

func bankParam(kind string, bank int) (id byte, inverted bool, err error) {
	if bank < 0 || bank >= proto.Banks {
		return 0, false, fmt.Errorf("no bank %d", bank)
	}
	id, inverted, ok := proto.BankParam(fmt.Sprintf("bank-%s-%c", kind, 'a'+bank))
	if !ok {
		return 0, false, fmt.Errorf("unknown bank setting %q", kind)
	}
	return id, inverted, nil
}

// SetBankTempo sets a bank's tempo, 40 to 200 BPM.
func SetBankTempo(p Pads, bank int, bpm float64) error {
	if bpm < MinBankBPM || bpm > MaxBankBPM {
		return fmt.Errorf("bank tempo must be between %.0f and %.0f BPM", MinBankBPM, MaxBankBPM)
	}
	id, _, err := bankParam("tempo", bank)
	if err != nil {
		return err
	}
	return p.SetParam(id, proto.TargetGlobal, int32(math.Round(bpm*100)))
}

// SetBankVolume sets a bank's volume, 0 to 127.
func SetBankVolume(p Pads, bank, volume int) error {
	if volume < 0 || volume > MaxBankVolume {
		return fmt.Errorf("bank volume must be between 0 and %d", MaxBankVolume)
	}
	id, inverted, err := bankParam("volume", bank)
	if err != nil {
		return err
	}
	stored := volume
	if inverted {
		stored = MaxBankVolume - volume
	}
	return p.SetParam(id, proto.TargetGlobal, int32(stored))
}
