package formats

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// WAV encodes the sample as a 16-bit PCM WAV file.
func (s Sample) WAV() []byte {
	dataLen := len(s.Samples) * 2
	out := make([]byte, 0, 44+dataLen)
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(36+dataLen))
	out = append(out, "WAVEfmt "...)
	out = binary.LittleEndian.AppendUint32(out, 16)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, uint16(s.Channels))
	out = binary.LittleEndian.AppendUint32(out, SampleRate)
	out = binary.LittleEndian.AppendUint32(out, uint32(SampleRate*s.Channels*2))
	out = binary.LittleEndian.AppendUint16(out, uint16(s.Channels*2))
	out = binary.LittleEndian.AppendUint16(out, bits)
	out = append(out, "data"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(dataLen))
	for _, v := range s.Samples {
		out = binary.LittleEndian.AppendUint16(out, uint16(v))
	}
	return out
}

// pcm is decoded audio before it is fitted to what the unit stores: interleaved samples
// as floats in −1..1, at the file's own rate.
type pcm struct {
	rate     int
	channels int
	samples  []float32
}

// riffChunks walks the chunks of a RIFF/AIFF body.
func riffChunks(body []byte, bigEndian bool, visit func(id string, data []byte) error) error {
	order := binary.ByteOrder(binary.LittleEndian)
	if bigEndian {
		order = binary.BigEndian
	}
	for len(body) >= 8 {
		id, size := string(body[:4]), int(order.Uint32(body[4:]))
		body = body[8:]
		// A streaming writer may leave the data size as 0 or all ones: take what is there.
		if size > len(body) || (id == "data" && (size == 0 || size == math.MaxUint32)) {
			size = len(body)
		}
		if err := visit(id, body[:size]); err != nil {
			return err
		}
		// Chunks are padded to an even length.
		body = body[min(size+size%2, len(body)):]
	}
	return nil
}

// decodeWAV reads PCM (8, 16, 24 and 32 bit) and IEEE float (32 and 64 bit) WAV files.
func decodeWAV(b []byte) (pcm, error) {
	if len(b) < 12 || !(bytes.HasPrefix(b, []byte("RIFF")) || bytes.HasPrefix(b, []byte("RF64"))) || string(b[8:12]) != "WAVE" {
		return pcm{}, fmt.Errorf("not a WAV file")
	}
	var (
		format, channels, depth, rate int
		data                          []byte
		haveFmt                       bool
	)
	err := riffChunks(b[12:], false, func(id string, c []byte) error {
		switch id {
		case "fmt ":
			if len(c) < 16 {
				return fmt.Errorf("WAV fmt chunk is too short")
			}
			format = int(binary.LittleEndian.Uint16(c))
			channels = int(binary.LittleEndian.Uint16(c[2:]))
			rate = int(binary.LittleEndian.Uint32(c[4:]))
			depth = int(binary.LittleEndian.Uint16(c[14:]))
			// WAVE_FORMAT_EXTENSIBLE carries the real format in its sub-format GUID.
			if format == 0xFFFE && len(c) >= 26 {
				format = int(binary.LittleEndian.Uint16(c[24:]))
			}
			haveFmt = true
		case "data":
			data = c
		}
		return nil
	})
	if err != nil {
		return pcm{}, err
	}
	if !haveFmt || data == nil {
		return pcm{}, fmt.Errorf("WAV file lacks a fmt or data chunk")
	}
	if channels < 1 || rate < 1 {
		return pcm{}, fmt.Errorf("WAV file declares %d channels at %d Hz", channels, rate)
	}
	width := depth / 8
	frame := width * channels
	if frame == 0 {
		return pcm{}, fmt.Errorf("WAV file declares %d bits", depth)
	}
	data = data[:len(data)/frame*frame]
	out := make([]float32, len(data)/width)
	switch {
	case format == 1 && depth == 8:
		for i, v := range data {
			out[i] = (float32(v) - 128) / 128
		}
	case format == 1 && depth == 16:
		for i := range out {
			out[i] = float32(int16(binary.LittleEndian.Uint16(data[2*i:]))) / 32768
		}
	case format == 1 && depth == 24:
		for i := range out {
			v := int32(data[3*i]) | int32(data[3*i+1])<<8 | int32(int8(data[3*i+2]))<<16
			out[i] = float32(v) / 8388608
		}
	case format == 1 && depth == 32:
		for i := range out {
			out[i] = float32(int32(binary.LittleEndian.Uint32(data[4*i:]))) / 2147483648
		}
	case format == 3 && depth == 32:
		for i := range out {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4*i:]))
		}
	case format == 3 && depth == 64:
		for i := range out {
			out[i] = float32(math.Float64frombits(binary.LittleEndian.Uint64(data[8*i:])))
		}
	default:
		return pcm{}, fmt.Errorf("unsupported WAV format %d with %d bits", format, depth)
	}
	return pcm{rate: rate, channels: channels, samples: out}, nil
}

// decodeAIFF reads uncompressed big-endian PCM AIFF files (8, 16, 24 and 32 bit).
func decodeAIFF(b []byte) (pcm, error) {
	if len(b) < 12 || string(b[:4]) != "FORM" || (string(b[8:12]) != "AIFF" && string(b[8:12]) != "AIFC") {
		return pcm{}, fmt.Errorf("not an AIFF file")
	}
	var (
		channels, depth, rate int
		data                  []byte
		haveComm              bool
	)
	err := riffChunks(b[12:], true, func(id string, c []byte) error {
		switch id {
		case "COMM":
			if len(c) < 18 {
				return fmt.Errorf("AIFF COMM chunk is too short")
			}
			channels = int(binary.BigEndian.Uint16(c))
			depth = int(binary.BigEndian.Uint16(c[6:]))
			rate = int(math.Round(extended(c[8:18])))
			if len(c) >= 22 && string(c[18:22]) != "NONE" && string(c[18:22]) != "twos" {
				return fmt.Errorf("compressed AIFF (%q) is not supported", c[18:22])
			}
			haveComm = true
		case "SSND":
			if len(c) < 8 {
				return fmt.Errorf("AIFF SSND chunk is too short")
			}
			offset := int(binary.BigEndian.Uint32(c))
			data = c[min(8+offset, len(c)):]
		}
		return nil
	})
	if err != nil {
		return pcm{}, err
	}
	if !haveComm || data == nil || channels < 1 || rate < 1 {
		return pcm{}, fmt.Errorf("AIFF file lacks a COMM or SSND chunk")
	}
	width := (depth + 7) / 8
	frame := width * channels
	if width < 1 || width > 4 {
		return pcm{}, fmt.Errorf("unsupported AIFF depth %d", depth)
	}
	data = data[:len(data)/frame*frame]
	out := make([]float32, len(data)/width)
	for i := range out {
		var v int32
		for j := 0; j < width; j++ {
			v = v<<8 | int32(data[width*i+j])
		}
		shift := uint(32 - 8*width)
		out[i] = float32(v<<shift) / 2147483648
		if width == 1 {
			out[i] = float32(int8(data[i])) / 128
		}
	}
	return pcm{rate: rate, channels: channels, samples: out}, nil
}

// extended reads an 80-bit IEEE extended float, as AIFF stores the sample rate.
func extended(b []byte) float64 {
	exp := int(binary.BigEndian.Uint16(b)&0x7FFF) - 16383
	mant := binary.BigEndian.Uint64(b[2:])
	return math.Ldexp(float64(mant), exp-63)
}
