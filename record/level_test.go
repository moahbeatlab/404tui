package record

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// wav24 writes a 24-bit stereo WAV the way a recorder does while it runs: a header that claims no
// length, then the frames.
func wav24(t *testing.T, frames [][2]int32) string {
	t.Helper()
	var b []byte
	le32 := func(v uint32) { b = binary.LittleEndian.AppendUint32(b, v) }
	le16 := func(v uint16) { b = binary.LittleEndian.AppendUint16(b, v) }
	b = append(b, "RIFF"...)
	le32(0xFFFFFFFF)
	b = append(b, "WAVEfmt "...)
	le32(16)
	le16(1)
	le16(2)
	le32(48000)
	le32(48000 * 6)
	le16(6)
	le16(24)
	b = append(b, "data"...)
	le32(0xFFFFFFFF)
	for _, f := range frames {
		for _, v := range f {
			b = append(b, byte(v), byte(v>>8), byte(v>>16))
		}
	}
	p := filepath.Join(t.TempDir(), "take.wav")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadLevelFindsThePeakOfEachChannelAtTheEnd(t *testing.T) {
	// A loud start (left at full scale), then 5000 frames at half scale on the left and a quarter on
	// the right; the window is the last 100 ms, 4800 frames, so the loud start is out of it.
	frames := [][2]int32{{8388607, 8388607}}
	for i := 0; i < 5000; i++ {
		frames = append(frames, [2]int32{4194304, -2097152})
	}
	l, err := ReadLevel(wav24(t, frames), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Peak) != 2 || math.Abs(l.Peak[0]-0.5) > 0.001 || math.Abs(l.Peak[1]-0.25) > 0.001 {
		t.Errorf("peaks %v, want 0.5 and 0.25", l.Peak)
	}
	if db := DB(l.Peak[0]); math.Abs(db+6.02) > 0.05 {
		t.Errorf("0.5 is %.2f dB, want -6.02", db)
	}
	// A short file: all of it counts.
	l, err = ReadLevel(wav24(t, [][2]int32{{8388607, 0}, {0, 0}}), time.Second)
	if err != nil || l.Peak[0] < 0.9999 || l.Peak[1] != 0 {
		t.Errorf("short file: %v %v", l, err)
	}
	// No frames yet, and not a WAV at all: no level, no crash.
	l, err = ReadLevel(wav24(t, nil), time.Second)
	if err != nil || l.Peak[0] != 0 {
		t.Errorf("empty: %v %v", l, err)
	}
	junk := filepath.Join(t.TempDir(), "x.wav")
	_ = os.WriteFile(junk, []byte("not a wav"), 0o644)
	if _, err := ReadLevel(junk, time.Second); err == nil {
		t.Error("junk read as a WAV")
	}
	if !math.IsInf(DB(0), -1) {
		t.Error("silence is not -Inf dB")
	}
}
