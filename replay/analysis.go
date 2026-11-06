package replay

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/moahbeatlab/404tui/record"
)

// The analysis of the audio: for every frame of the picture (30 a second) how loud each channel is and the spectrum of the sound, worked
// out here from the WAV, so that the replay shows the same meter and spectrum whether it is watched in a browser or rendered into a video.

// FPS is the frames a second of the analysis.
const FPS = 30

// Analysis is what the page is given.
type Analysis struct {
	FPS      int     `json:"fps"`
	Rate     int     `json:"rate"`
	Duration float64 `json:"duration"`
	Frames   int     `json:"frames"`
	Bands    int     `json:"bands"`
	// Peak is, a frame after another, the highest value of the left and of the right channel in that frame, 0 to 65535 (two bytes each, little
	// endian), as base64. Spec is the spectrum of each frame, one byte a band, 0 to 255, as base64.
	Peak []byte `json:"peak"`
	Spec []byte `json:"spec"`
}

type wavInfo struct {
	rate, channels, bits int
	float                bool
	dataStart            int64
	dataLen              int64
}

// readWAVHeader finds the format and the data of a WAV file (PCM 16, 24 or 32 bit, or 32 bit float).
func readWAVHeader(f *os.File) (wavInfo, error) {
	var w wavInfo
	head := make([]byte, 12)
	if _, err := io.ReadFull(f, head); err != nil || string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return w, errors.New("not a WAV file")
	}
	pos := int64(12)
	for {
		ch := make([]byte, 8)
		if _, err := io.ReadFull(f, ch); err != nil {
			return w, errors.New("no audio data in the WAV file")
		}
		size := int64(binary.LittleEndian.Uint32(ch[4:]))
		pos += 8
		switch string(ch[0:4]) {
		case "fmt ":
			b := make([]byte, size)
			if _, err := io.ReadFull(f, b); err != nil || len(b) < 16 {
				return w, errors.New("a broken WAV format")
			}
			tag := binary.LittleEndian.Uint16(b[0:])
			if tag == 0xFFFE && len(b) >= 26 { // extensible: the real tag is in the sub format
				tag = binary.LittleEndian.Uint16(b[24:])
			}
			w.channels = int(binary.LittleEndian.Uint16(b[2:]))
			w.rate = int(binary.LittleEndian.Uint32(b[4:]))
			w.bits = int(binary.LittleEndian.Uint16(b[14:]))
			w.float = tag == 3
			if tag != 1 && tag != 3 || w.channels < 1 || w.channels > 8 || w.bits != 16 && w.bits != 24 && w.bits != 32 {
				return w, fmt.Errorf("a WAV of %d channels, %d bits, format %d is not supported (16, 24 or 32 bit PCM, or float)", w.channels, w.bits, tag)
			}
			pos += size
		case "data":
			w.dataStart = pos
			w.dataLen = size
			return w, nil
		default:
			if _, err := f.Seek(size+size%2, io.SeekCurrent); err != nil {
				return w, err
			}
			pos += size + size%2
		}
		if size%2 == 1 && string(ch[0:4]) == "fmt " {
			f.Seek(1, io.SeekCurrent)
			pos++
		}
	}
}

// frameValue reads one sample of a frame as -1..1.
func (w wavInfo) value(b []byte) float64 {
	switch {
	case w.float && w.bits == 32:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case w.bits == 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case w.bits == 24:
		return float64(int32(uint32(b[0])|uint32(b[1])<<8|uint32(b[2])<<16)<<8>>8) / 8388608
	}
	return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
}

// Analyze reads the WAV once and gives the meter and the spectrum of every frame of the picture.
func Analyze(path string) (*Analysis, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	w, err := readWAVHeader(f)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(w.dataStart, io.SeekStart); err != nil {
		return nil, err
	}
	bytesPer := w.bits / 8
	frameBytes := bytesPer * w.channels
	perFrame := w.rate / FPS // audio frames in one frame of the picture
	r := bufio.NewReaderSize(f, 1<<20)
	buf := make([]byte, perFrame*frameBytes)
	var peak, spec []byte
	hist := make([]float64, 0, 4096)
	total := int64(0)
	for {
		n, err := io.ReadFull(r, buf)
		got := n / frameBytes
		if got == 0 {
			break
		}
		var pl, pr float64
		for i := 0; i < got; i++ {
			l := w.value(buf[i*frameBytes:])
			rr := l
			if w.channels > 1 {
				rr = w.value(buf[i*frameBytes+bytesPer:])
			}
			pl, pr = max(pl, math.Abs(l)), max(pr, math.Abs(rr))
			hist = append(hist, (l+rr)/2)
		}
		if len(hist) > 4096 {
			hist = hist[len(hist)-4096:]
		}
		peak = binary.LittleEndian.AppendUint16(peak, uint16(min(pl, 1)*65535))
		peak = binary.LittleEndian.AppendUint16(peak, uint16(min(pr, 1)*65535))
		for _, v := range record.Bands(hist, w.rate) {
			spec = append(spec, byte(math.Round(min(max(v, 0), 1)*255)))
		}
		total += int64(got)
		if err != nil {
			break
		}
	}
	frames := len(peak) / 4
	return &Analysis{FPS: FPS, Rate: w.rate, Duration: float64(total) / float64(w.rate), Frames: frames, Bands: record.SpectrumBands, Peak: peak, Spec: spec}, nil
}
