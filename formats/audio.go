package formats

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"
)

// AudioExtensions are the file extensions that can be imported.
var AudioExtensions = []string{"wav", "wave", "bwf", "aif", "aiff", "flac", "mp3"}

// IsAudioFile reports whether a file name has an importable extension.
func IsAudioFile(name string) bool {
	// macOS leaves "._name.wav" beside every file it copies to a card: not audio.
	if strings.HasPrefix(name, "._") {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	for _, e := range AudioExtensions {
		if e == ext {
			return true
		}
	}
	return false
}

// Imported is a decoded audio file ready for the unit, plus what the source looked like.
type Imported struct {
	Sample         Sample
	SourceRate     int
	SourceChannels int
}

// Resampled reports whether the audio went through sample-rate conversion.
func (i Imported) Resampled() bool {
	return i.SourceRate != SampleRate
}

// ReadAudio decodes an audio file (WAV, AIFF, FLAC, MP3) into 48 kHz 16-bit audio. Mono
// sources stay mono, sources with more than two channels keep the first two, and other
// sample rates are converted with a band-limited resampler.
func ReadAudio(path string) (Imported, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Imported{}, err
	}
	return DecodeAudio(b, filepath.Ext(path))
}

// DecodeAudio decodes audio already in memory, as it arrives from the unit's file API. ext
// picks the format and may be empty, in which case the content is sniffed.
func DecodeAudio(b []byte, ext string) (Imported, error) {
	decoded, err := decode(b, strings.TrimPrefix(strings.ToLower(ext), "."))
	if err != nil {
		return Imported{}, err
	}
	if len(decoded.samples) == 0 || decoded.channels == 0 {
		return Imported{}, ErrEmpty
	}
	imported := Imported{SourceRate: decoded.rate, SourceChannels: decoded.channels}
	keep := min(decoded.channels, 2)
	// Keep the first two channels of anything wider.
	samples := decoded.samples
	if keep != decoded.channels {
		samples = make([]float32, 0, len(decoded.samples)/decoded.channels*keep)
		for i := 0; i+decoded.channels <= len(decoded.samples); i += decoded.channels {
			samples = append(samples, decoded.samples[i:i+keep]...)
		}
	}
	samples = resample(samples, keep, decoded.rate)
	if len(samples) == 0 {
		return Imported{}, ErrEmpty
	}
	out := make([]int16, len(samples))
	for i, v := range samples {
		out[i] = quantize(v)
	}
	imported.Sample = Sample{Channels: keep, Samples: out}
	return imported, nil
}

// quantize converts full-scale float (−1..1) to 16-bit. Exact for audio that was 16-bit
// to begin with.
func quantize(v float32) int16 {
	return int16(math.Max(-32768, math.Min(32767, math.Round(float64(v)*32768))))
}

func decode(b []byte, ext string) (pcm, error) {
	kind := ext
	switch {
	case bytes.HasPrefix(b, []byte("RIFF")), bytes.HasPrefix(b, []byte("RF64")):
		kind = "wav"
	case bytes.HasPrefix(b, []byte("FORM")):
		kind = "aiff"
	case bytes.HasPrefix(b, []byte("fLaC")):
		kind = "flac"
	case bytes.HasPrefix(b, []byte("ID3")) || (len(b) > 1 && b[0] == 0xFF && b[1]&0xE0 == 0xE0):
		kind = "mp3"
	}
	switch kind {
	case "wav", "wave", "bwf":
		return decodeWAV(b)
	case "aif", "aiff", "aifc":
		return decodeAIFF(b)
	case "flac":
		return decodeFLAC(b)
	case "mp3":
		return decodeMP3(b)
	}
	return pcm{}, ErrNoAudioTrack
}

func decodeFLAC(b []byte) (pcm, error) {
	stream, err := flac.New(bytes.NewReader(b))
	if err != nil {
		return pcm{}, fmt.Errorf("decoding audio: %w", err)
	}
	info := stream.Info
	channels, depth := int(info.NChannels), int(info.BitsPerSample)
	scale := float32(math.Ldexp(1, depth-1))
	out := &pcm{rate: int(info.SampleRate), channels: channels}
	for {
		frame, err := stream.ParseNext()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A corrupt frame is skipped, as players do.
			if len(out.samples) > 0 {
				break
			}
			return pcm{}, fmt.Errorf("decoding audio: %w", err)
		}
		for i := range frame.Subframes[0].Samples {
			for c := 0; c < channels; c++ {
				out.samples = append(out.samples, float32(frame.Subframes[c].Samples[i])/scale)
			}
		}
	}
	return *out, nil
}

// decodeMP3 always yields stereo: the decoder duplicates a mono stream into both channels.
func decodeMP3(b []byte) (pcm, error) {
	dec, err := mp3.NewDecoder(bytes.NewReader(b))
	if err != nil {
		return pcm{}, fmt.Errorf("decoding audio: %w", err)
	}
	raw, err := io.ReadAll(dec)
	if err != nil && len(raw) == 0 {
		return pcm{}, fmt.Errorf("decoding audio: %w", err)
	}
	out := make([]float32, len(raw)/2)
	for i := range out {
		out[i] = float32(int16(uint16(raw[2*i])|uint16(raw[2*i+1])<<8)) / 32768
	}
	return pcm{rate: dec.SampleRate(), channels: 2, samples: out}, nil
}
