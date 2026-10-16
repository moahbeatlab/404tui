package record

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"time"
)

// Level is how loud the end of a recording is, as the highest sample of each channel, 0 to 1 (1 is
// full scale: the loudest a WAV file can hold, past which the sound clips).
type Level struct {
	Peak []float64
}

// DB is a peak in dB below full scale (0 is full scale); a silent channel is -Inf.
func DB(peak float64) float64 {
	if peak <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(peak)
}

type wavFormat struct {
	channels, bits, dataAt int
	rate                   int
	float                  bool
}

// readHeader finds how the samples of a WAV file are laid out and where they start.
func readHeader(f *os.File) (wavFormat, error) {
	var buf [4096]byte
	n, err := f.ReadAt(buf[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return wavFormat{}, err
	}
	b := buf[:n]
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return wavFormat{}, errors.New("not a WAV file (yet)")
	}
	var wf wavFormat
	have := false
	for at := 12; at+8 <= len(b); {
		id, size := string(b[at:at+4]), int(binary.LittleEndian.Uint32(b[at+4:at+8]))
		body := at + 8
		switch id {
		case "fmt ":
			if body+16 > len(b) {
				return wavFormat{}, errors.New("short header")
			}
			tag := binary.LittleEndian.Uint16(b[body:])
			wf.channels = int(binary.LittleEndian.Uint16(b[body+2:]))
			wf.rate = int(binary.LittleEndian.Uint32(b[body+4:]))
			wf.bits = int(binary.LittleEndian.Uint16(b[body+14:]))
			wf.float = tag == 3
			have = true
		case "data":
			if !have || wf.channels < 1 || wf.bits%8 != 0 || wf.bits == 0 {
				return wavFormat{}, errors.New("unreadable WAV header")
			}
			wf.dataAt = body
			return wf, nil
		}
		at = body + size + size%2
	}
	return wavFormat{}, errors.New("no sample data yet")
}

// ReadLevel reads the last window of a WAV file that is still being written and says how loud it is.
// It works for 16, 24 and 32 bit integer files and 32 bit float ones.
func ReadLevel(path string, window time.Duration) (Level, error) {
	f, err := os.Open(path)
	if err != nil {
		return Level{}, err
	}
	defer f.Close()
	wf, err := readHeader(f)
	if err != nil {
		return Level{}, err
	}
	st, err := f.Stat()
	if err != nil {
		return Level{}, err
	}
	frame := wf.channels * wf.bits / 8
	have := (int(st.Size()) - wf.dataAt) / frame
	if have <= 0 {
		return Level{Peak: make([]float64, wf.channels)}, nil
	}
	want := min(max(int(float64(wf.rate)*window.Seconds()), 1), have)
	buf := make([]byte, want*frame)
	if _, err := f.ReadAt(buf, int64(wf.dataAt+(have-want)*frame)); err != nil && !errors.Is(err, io.EOF) {
		return Level{}, err
	}
	peak := make([]float64, wf.channels)
	bytes := wf.bits / 8
	for i := 0; i+frame <= len(buf); i += frame {
		for c := 0; c < wf.channels; c++ {
			s := buf[i+c*bytes : i+(c+1)*bytes]
			var v float64
			switch {
			case wf.float && wf.bits == 32:
				v = math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(s))))
			case wf.bits == 16:
				v = math.Abs(float64(int16(binary.LittleEndian.Uint16(s)))) / 32768
			case wf.bits == 24:
				x := int32(s[0]) | int32(s[1])<<8 | int32(int8(s[2]))<<16
				v = math.Abs(float64(x)) / 8388608
			case wf.bits == 32:
				v = math.Abs(float64(int32(binary.LittleEndian.Uint32(s)))) / 2147483648
			}
			if v > peak[c] {
				peak[c] = v
			}
		}
	}
	return Level{Peak: peak}, nil
}
