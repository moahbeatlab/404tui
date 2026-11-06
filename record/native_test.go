package record

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeInput is native capture for the tests: the test pushes frames in as the audio system would.
type fakeInput struct {
	mu      sync.Mutex
	opened  int
	closed  int
	frames  func([]float32)
	end     func(error)
	missing bool
}

func (f *fakeInput) install(t *testing.T) {
	t.Helper()
	old := nativeOpener
	nativeOpener = func(device string, onFrames func([]float32), onEnd func(error)) (io.Closer, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.missing {
			return nil, ErrNoDevice
		}
		f.opened++
		f.frames, f.end = onFrames, onEnd
		return closerFunc(func() error { f.mu.Lock(); f.closed++; f.mu.Unlock(); return nil }), nil
	}
	t.Cleanup(func() { nativeOpener = old })
}

func (f *fakeInput) push(frames []float32) {
	f.mu.Lock()
	fn := f.frames
	f.mu.Unlock()
	fn(frames)
}

func tone(amp, freq float64, frames int) []float32 {
	out := make([]float32, 0, frames*2)
	for i := 0; i < frames; i++ {
		v := float32(amp * math.Sin(2*math.Pi*freq*float64(i)/48000))
		out = append(out, v, v)
	}
	return out
}

func TestNativeMeterAndRecordingShareOneCapture(t *testing.T) {
	var in fakeInput
	in.install(t)
	dir := t.TempDir()
	m, err := startNativeMeter("404")
	if err != nil {
		t.Fatal(err)
	}
	s, err := startNativeRecording(dir, "404")
	if err != nil {
		t.Fatal(err)
	}
	if in.opened != 1 {
		t.Fatalf("the meter and the recording share one capture of the input: opened %d times", in.opened)
	}
	// 100 ms of a 1 kHz tone at half scale, in the small pieces the audio system hands over
	for i := 0; i < 5; i++ {
		in.push(tone(0.5, 1000, 960))
	}
	p, ok := m.Level()
	if !ok || p[0] < 0.49 || p[0] > 0.51 || p[1] < 0.49 {
		t.Fatalf("the meter has the level of what was just played: %v %v", p, ok)
	}
	if spec, ok := m.Spectrum(); !ok || len(spec) != SpectrumBands {
		t.Fatal("and its spectrum")
	}
	time.Sleep(10 * time.Millisecond)
	// the recording stops, the capture stays for the meter
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if in.closed != 0 {
		t.Fatal("the capture goes on while the meter listens")
	}
	m.Stop()
	if in.closed != 1 {
		t.Fatalf("it is closed after the last one left: %d", in.closed)
	}
	// the file: a WAV, 24 bit, 48 kHz, stereo, the length of what was played, with the half scale tone in it
	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || binary.LittleEndian.Uint16(b[20:]) != 1 || binary.LittleEndian.Uint16(b[22:]) != 2 ||
		binary.LittleEndian.Uint32(b[24:]) != 48000 || binary.LittleEndian.Uint16(b[34:]) != 24 {
		t.Fatalf("the header of a 24 bit stereo 48 kHz WAV: %v", b[:44])
	}
	data := int(binary.LittleEndian.Uint32(b[40:]))
	if data != 5*960*2*3 || len(b) != 44+data || int(binary.LittleEndian.Uint32(b[4:])) != 36+data {
		t.Fatalf("the sizes were written when it stopped: data %d, file %d", data, len(b))
	}
	peak := 0
	for i := 44; i+2 < len(b); i += 3 {
		v := int(int32(uint32(b[i])|uint32(b[i+1])<<8|uint32(b[i+2])<<16) << 8 >> 8)
		if v > peak {
			peak = v
		}
	}
	if peak < 4100000 || peak > 4300000 { // half of 8388607
		t.Fatalf("the half scale tone is in the file: peak %d", peak)
	}
	if filepath.Dir(s.Path) != dir || s.Tool != "the program itself" {
		t.Fatalf("the file is in the folder: %s %s", s.Path, s.Tool)
	}
}

func TestNativeCaptureEndsWhenTheInputGoes(t *testing.T) {
	var in fakeInput
	in.install(t)
	m, err := startNativeMeter("404")
	if err != nil {
		t.Fatal(err)
	}
	s, err := startNativeRecording(t.TempDir(), "404")
	if err != nil {
		t.Fatal(err)
	}
	in.push(tone(0.3, 440, 960))
	in.end(errors.New("unplugged"))
	select {
	case <-m.Done():
	case <-time.After(time.Second):
		t.Fatal("the meter ends")
	}
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("the recording ends and the file is finished")
	}
	if s.Err() == nil {
		t.Fatal("it says why")
	}
	if b, _ := os.ReadFile(s.Path); len(b) < 44 || binary.LittleEndian.Uint32(b[40:]) != 960*2*3 {
		t.Fatal("what was heard before is in the file")
	}
}

func TestNativeFirstFallsBackAndKeepsAMissingInputAnError(t *testing.T) {
	var in fakeInput
	in.install(t)
	b := nativeFirst{exec: execBackend{}}
	in.missing = true
	if _, err := b.Start(t.TempDir(), "404"); !errors.Is(err, ErrNoDevice) {
		t.Fatalf("a missing input is said, the program does not try another way: %v", err)
	}
	in.missing = false
	UseNative = false
	defer func() { UseNative = true }()
	if _, err := b.Start(t.TempDir(), "404"); !errors.Is(err, ErrNoTool) {
		t.Fatalf("with native capture off the recorder program is used, and there is none here: %v", err)
	}
	if Tool(nativeFirst{}) != "" {
		t.Fatal("no program, no native: no tool")
	}
	UseNative = true
	if Tool(nativeFirst{}) != "the program itself" {
		t.Fatal("native capture is named")
	}
}
