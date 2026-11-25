//go:build !js && !wasip1

package formats

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// Formats the program has no decoder of its own for (AAC in an .m4a) are decoded by a program of the computer: ffmpeg when
// there is one, else afconvert, which every Mac has.

// ErrNeedsDecoder says that a file needs ffmpeg (or a Mac's afconvert) to be read.
var ErrNeedsDecoder = errors.New("m4a needs ffmpeg (or a Mac): brew install ffmpeg")

func decodeExternal(ctx context.Context, b []byte, ext string) (pcm, error) {
	dir, err := os.MkdirTemp("", "404tui-decode-")
	if err != nil {
		return pcm{}, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in."+ext)
	if err := os.WriteFile(in, b, 0o600); err != nil {
		return pcm{}, err
	}
	if ff, err := exec.LookPath("ffmpeg"); err == nil {
		out, err := exec.CommandContext(ctx, ff, "-v", "error", "-nostdin", "-i", in, "-f", "s16le", "-acodec", "pcm_s16le", "-ac", "2", "-ar", "48000", "-").Output()
		if ctx.Err() != nil {
			return pcm{}, ctx.Err()
		}
		if err == nil && len(out) >= 4 {
			p := pcm{rate: 48000, channels: 2, samples: make([]float32, len(out)/2)}
			for i := range p.samples {
				p.samples[i] = float32(int16(binary.LittleEndian.Uint16(out[2*i:]))) / 32768
			}
			return p, nil
		}
	}
	if af, err := exec.LookPath("afconvert"); err == nil {
		out := filepath.Join(dir, "out.wav")
		if err := exec.CommandContext(ctx, af, "-f", "WAVE", "-d", "LEI16@48000", in, out).Run(); err == nil {
			if w, err := os.ReadFile(out); err == nil {
				return decodeWAV(w)
			}
		}
		if ctx.Err() != nil {
			return pcm{}, ctx.Err()
		}
	}
	return pcm{}, ErrNeedsDecoder
}
