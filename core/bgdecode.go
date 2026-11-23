package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

// Long files (a whole song) are decoded in the background on FILES: the cursor never waits for them. The panel says it is
// decoding; Space (or A) on the file goes on by itself when it is ready. One file is decoded at a time; going to another one lets
// go of the one before. With ffmpeg on the computer an MP3 takes well under a second, without it the decoder written in Go takes
// about a second for every half a minute of the song.

// bgBytes is the size from which a file on the computer is decoded in the background.
const bgBytes = 3 << 20

// bgDecode is the file decoded in the background.
type bgDecode struct {
	path   string
	at     time.Time
	cancel context.CancelFunc
	done   chan bgResult
	then   func() // what goes on when it is ready, on the same file (a key pressed on it); nil: nothing
}

type bgResult struct {
	imp formats.Imported
	err error
}

// bgDecodes says whether a file is decoded in the background: a long file on the computer.
func bgDecodes(path string, size int64) bool {
	return Terminal && app.IsLocal(path) && size >= bgBytes
}

// decodeOrQueue is the decoded file under the cursor when it is ready: a short file is decoded at once, a long one is started in the
// background (ready false until it is done).
func (a *App) decodeOrQueue(e entry) (id int, ready bool, err error) {
	p := joinPath(a.dir, e.name)
	if !bgDecodes(p, e.size) {
		id, err := a.decode(e)
		return id, err == nil, err
	}
	if id, ok := a.prev[p]; ok {
		return id, true, nil
	}
	if err, ok := a.bgFailed[p]; ok {
		return 0, false, err
	}
	a.startBgDecode(p, pathExt(e.name))
	return 0, false, nil
}

// startBgDecode decodes a file in the background (letting go of another one).
func (a *App) startBgDecode(p, ext string) {
	if a.bg != nil {
		if a.bg.path == p {
			return
		}
		a.bg.cancel()
		a.bg = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &bgDecode{path: p, at: time.Now(), cancel: cancel, done: make(chan bgResult, 1)}
	a.bg = b
	go func() {
		imp, err := decodeFast(ctx, p, ext)
		b.done <- bgResult{imp, err}
	}()
}

// bgDecodeTick takes a file that is ready, and presses again what was pressed on it.
func (a *App) bgDecodeTick() {
	b := a.bg
	if b == nil {
		return
	}
	var r bgResult
	select {
	case r = <-b.done:
	default:
		return
	}
	a.bg = nil
	if r.err != nil {
		if r.err != context.Canceled {
			if a.bgFailed == nil {
				a.bgFailed = map[string]error{}
			}
			a.bgFailed[b.path] = r.err
			a.say(r.err.Error(), true)
		}
		return
	}
	mb, err := a.lab.KeepImported(r.imp)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	var m lab.Meta
	json.Unmarshal(mb, &m)
	a.prev[b.path] = m.ID
	a.prevOrder = append(a.prevOrder, b.path)
	a.evictPreviews()
	a.changed = true
	e, ok := a.current()
	here := ok && a.scr == Files && joinPath(a.dir, e.name) == b.path && !a.Modal()
	switch {
	case here && b.then != nil:
		b.then()
	case here && a.set.AutoListen:
		a.listenHere()
	}
}

// bgWaiting says that a key was pressed on a file that is still decoding: it is pressed again when the file is ready.
func (a *App) bgWaiting(k Key) {
	if a.bg == nil {
		return
	}
	what := "plays"
	if k == A {
		what = "goes on"
	}
	a.bgThen(func() { a.filesKey(k) }, what)
}

// bgThen does something with the file that decodes when it is ready (if the cursor is still on it), and says so.
func (a *App) bgThen(f func(), what string) {
	if a.bg == nil {
		return
	}
	a.bg.then = f
	a.say(fmt.Sprintf("decoding the song (%s): it %s when it is ready", a.bgElapsed(), what), false)
}

// bgElapsed is how long the file has been decoding.
func (a *App) bgElapsed() string {
	if a.bg == nil {
		return ""
	}
	return fmt.Sprintf("%.0f s", time.Since(a.bg.at).Seconds())
}

// bgText is what the panel says of a file that is decoding.
func (a *App) bgText(path string) string {
	if a.bg != nil && a.bg.path == path {
		s := "decoding... " + a.bgElapsed()
		if ffmpegPath() == "" && strings.EqualFold(pathExt(path), ".mp3") {
			s += " (ffmpeg would be faster)"
		}
		return s
	}
	return ""
}

var ffmpegFound *string

// ffmpegPath is where ffmpeg is, "" when the computer has none.
func ffmpegPath() string {
	if ffmpegFound == nil {
		p, _ := exec.LookPath("ffmpeg")
		ffmpegFound = &p
	}
	return *ffmpegFound
}

// decodeFast decodes a file on the computer: with ffmpeg when there is one (to 48 kHz stereo), else with the decoders of the program.
func decodeFast(ctx context.Context, p, ext string) (formats.Imported, error) {
	if ff := ffmpegPath(); ff != "" {
		out, err := exec.CommandContext(ctx, ff, "-v", "error", "-nostdin", "-i", p, "-f", "s16le", "-acodec", "pcm_s16le", "-ac", "2", "-ar", "48000", "-").Output()
		if ctx.Err() != nil {
			return formats.Imported{}, ctx.Err()
		}
		if err == nil && len(out) >= 4 {
			s := formats.Sample{Channels: 2, Samples: make([]int16, len(out)/2)}
			for i := range s.Samples {
				s.Samples[i] = int16(binary.LittleEndian.Uint16(out[2*i:]))
			}
			return formats.Imported{Sample: s, SourceRate: formats.SampleRate, SourceChannels: 2}, nil
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return formats.Imported{}, err
	}
	return formats.DecodeAudioContext(ctx, b, ext)
}
