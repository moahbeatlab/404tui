package record

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/dbg"
)

// closerFunc makes a function an io.Closer (the capture of each system returns one).
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// Native capture: the program listens to the input itself (CoreAudio on a Mac, PulseAudio or PipeWire on Linux) in place of starting
// ffmpeg or sox. One capture of the input feeds everything that wants it, the level meter and the recording, so they hear the same sound at
// the same time, a few milliseconds after it was played, and nothing is started or buffered in another program.

// UseNative says whether the recorder listens itself where it can (the default); CONFIG can turn it off, for ffmpeg or sox.
var UseNative = true

// ErrNoDevice is what native capture says when no input has the device's name.
var ErrNoDevice = errors.New("no audio input with that name")

// nativeOpener starts listening to the input whose name holds device: onFrames gets interleaved stereo frames at 48 kHz (-1..1) on a
// thread of the audio system, and onEnd is called once when listening ends by itself (the device went away). It is set by the files of the
// systems that have native capture.
var nativeOpener func(device string, onFrames func([]float32), onEnd func(error)) (io.Closer, error)

// Native says whether this system has native capture.
func Native() bool { return nativeOpener != nil }

type listener struct {
	frames func([]float32)
	end    func(error)
}

// hub shares one capture of the input among the meter and the recording: it is opened for the first that joins and closed after the last
// leaves.
type hub struct {
	mu     sync.Mutex
	in     io.Closer
	device string
	next   int
	subs   map[int]listener
}

var theHub = &hub{subs: map[int]listener{}}

// join starts (or joins) the capture of the input; leave stops listening for this one.
func (h *hub) join(device string, frames func([]float32), end func(error)) (leave func(), err error) {
	if nativeOpener == nil {
		return nil, errors.New("no native audio capture on this system")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.in != nil && h.device != device {
		return nil, fmt.Errorf("the input %q is being listened to already", h.device)
	}
	if h.in == nil {
		in, err := nativeOpener(device, h.dispatch, h.fail)
		if err != nil {
			dbg.Logger.Warn("capture: the input would not open", "device", device, "err", err)
			return nil, err
		}
		dbg.Logger.Info("capture: listening to the input", "device", device)
		h.in, h.device = in, device
	}
	id := h.next
	h.next++
	h.subs[id] = listener{frames, end}
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.subs, id)
			if len(h.subs) == 0 && h.in != nil {
				h.in.Close()
				h.in = nil
				dbg.Logger.Info("capture: the input is let go")
			}
		})
	}, nil
}

func (h *hub) dispatch(frames []float32) {
	h.mu.Lock()
	fns := make([]func([]float32), 0, len(h.subs))
	for _, l := range h.subs {
		fns = append(fns, l.frames)
	}
	h.mu.Unlock()
	for _, f := range fns {
		f(frames)
	}
}

// fail is the capture ending by itself: every listener is told and the capture is let go.
func (h *hub) fail(err error) {
	dbg.Logger.Warn("capture: the input ended by itself", "err", err)
	h.mu.Lock()
	subs := h.subs
	h.subs = map[int]listener{}
	if h.in != nil {
		go h.in.Close()
		h.in = nil
	}
	h.mu.Unlock()
	for _, l := range subs {
		if l.end != nil {
			l.end(err)
		}
	}
}

// startNativeMeter is StartMeter without another program.
func startNativeMeter(device string) (*Meter, error) {
	if device == "" {
		device = DefaultDevice
	}
	m := &Meter{done: make(chan struct{})}
	var once sync.Once
	finish := func(err error) {
		once.Do(func() {
			m.mu.Lock()
			m.err = err
			m.mu.Unlock()
			close(m.done)
		})
	}
	leave, err := theHub.join(device, m.feed, finish)
	if err != nil {
		return nil, err
	}
	m.stopFn = func() { leave(); finish(nil) }
	return m, nil
}

// ---- the recording: a WAV file written here, 24 bit, 48 kHz, stereo

const (
	wavRate  = 48000
	wavBytes = 3 // a sample is 24 bits
)

// startNativeRecording records the input into a new file in dir.
func startNativeRecording(dir, device string) (*Session, error) {
	if device == "" {
		device = DefaultDevice
	}
	if dir == "" {
		return nil, errors.New("no folder to record into: set one in Config")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, FileName(time.Now()))
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(f, 64<<10)
	if _, err := w.Write(wavHeader(0)); err != nil {
		f.Close()
		return nil, err
	}
	s := &Session{Path: path, Tool: "the program itself", Started: time.Now(), done: make(chan struct{})}
	var (
		mu       sync.Mutex
		data     int64
		lastSync = time.Now()
		closed   bool
		buf      []byte
		werr     error
	)
	finish := func(err error) {
		mu.Lock()
		if closed {
			mu.Unlock()
			return
		}
		closed = true
		ferr := w.Flush()
		if ferr == nil {
			// the sizes are known now: the header is written again with them
			if _, e := f.WriteAt(wavHeader(data), 0); e != nil {
				ferr = e
			}
		}
		if cerr := f.Close(); ferr == nil {
			ferr = cerr
		}
		mu.Unlock()
		if err == nil {
			err = werr
		}
		if err == nil {
			err = ferr
		}
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	}
	leave, err := theHub.join(device, func(frames []float32) {
		mu.Lock()
		defer mu.Unlock()
		if closed || werr != nil {
			return
		}
		if data == 0 { // the first frame was heard one buffer ago
			s.mu.Lock()
			s.first = time.Now().Add(-time.Duration(len(frames)/2) * time.Second / wavRate)
			s.mu.Unlock()
		}
		buf = buf[:0]
		for _, v := range frames {
			n := int32(math.Max(-1, math.Min(1, float64(v))) * 8388607)
			buf = append(buf, byte(n), byte(n>>8), byte(n>>16))
		}
		if _, err := w.Write(buf); err != nil {
			werr = err
			return
		}
		data += int64(len(buf))
		if time.Since(lastSync) > 200*time.Millisecond { // the file is read while it grows (the size shows, the level of its end)
			lastSync = time.Now()
			if err := w.Flush(); err != nil {
				werr = err
			}
		}
	}, func(err error) { finish(fmt.Errorf("the input ended: %v", err)) })
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	s.stop = func() error {
		leave()
		finish(nil)
		return nil
	}
	return s, nil
}

// wavHeader is the 44 bytes before the samples; data is how many bytes of samples follow (0 while it is not known).
func wavHeader(data int64) []byte {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+data))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 2) // stereo
	binary.LittleEndian.PutUint32(h[24:], wavRate)
	binary.LittleEndian.PutUint32(h[28:], wavRate*2*wavBytes)
	binary.LittleEndian.PutUint16(h[32:], 2*wavBytes)
	binary.LittleEndian.PutUint16(h[34:], 8*wavBytes)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(data))
	return h
}

// ---- the backend that listens itself where it can, and starts a program where it cannot

// nativeFirst is a Backend that records and meters with native capture when the system has it (and it is not turned off), and falls
// back to the recorder program when the input cannot be opened that way.
type nativeFirst struct{ exec execBackend }

func (b nativeFirst) useNative() bool { return UseNative && nativeOpener != nil }

func (b nativeFirst) Start(dir, device string) (*Session, error) {
	if b.useNative() {
		s, err := startNativeRecording(dir, device)
		if err == nil || errors.Is(err, ErrNoDevice) || b.exec.ffmpeg == "" && b.exec.sox == "" {
			return s, err
		}
		dbg.Logger.Warn("record: native capture failed, the recorder program is tried", "err", err)
	}
	return b.exec.Start(dir, device)
}

func (b nativeFirst) StartMeter(device string) (*Meter, error) {
	if b.useNative() {
		m, err := startNativeMeter(device)
		if err == nil || errors.Is(err, ErrNoDevice) || b.exec.ffmpeg == "" && b.exec.sox == "" {
			return m, err
		}
		dbg.Logger.Warn("meter: native capture failed, the recorder program is tried", "err", err)
	}
	return b.exec.StartMeter(device)
}

func (b nativeFirst) StartRing(device string, seconds int) (*Ring, error) {
	return b.exec.StartRing(device, seconds)
}
