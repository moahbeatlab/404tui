//go:build js || wasip1

package formats

import (
	"context"
	"errors"
)

// ErrNeedsDecoder says that a file needs ffmpeg (or a Mac's afconvert) to be read; a browser has neither.
var ErrNeedsDecoder = errors.New("m4a can't be read here: convert it to wav or mp3")

func decodeExternal(context.Context, []byte, string) (pcm, error) { return pcm{}, ErrNeedsDecoder }
