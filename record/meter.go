package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Metering is a backend that can also listen to the input without recording it, for a level meter.
type Metering interface {
	StartMeter(device string) (*Meter, error)
}

// meterWindow is how much sound one reading covers.
const meterWindow = 25 * time.Millisecond

// Meter listens to an audio input and keeps how loud it is, per channel, over the latest 100 ms. It
// writes nothing to disk: the program that reads the input hands the samples over a pipe.
type Meter struct {
	cmd  *exec.Cmd
	done chan struct{}

	mu   sync.Mutex
	peak [2]float64
	acc  [2]float64 // the highest peak since Level was asked last: a reading never misses a short peak
	hist []float64  // the latest mono samples, the spectrum's window (4096, a window of the meter at a time)
	spec []float64  // the spectrum of the latest window, falling slowly
	at   time.Time
	err  error
	tail string
}

// Spectrum is the bands of the latest window (SpectrumBands values, 0 to 1), and whether there is one yet.
func (m *Meter) Spectrum() ([]float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.spec == nil || time.Since(m.at) > time.Second {
		return nil, false
	}
	return append([]float64(nil), m.spec...), true
}

// Level is the peak of each channel, 0 to 1, over the latest window, and whether there is one yet.
func (m *Meter) Level() (peak [2]float64, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	peak = m.acc
	m.acc = [2]float64{}
	return peak, !m.at.IsZero() && time.Since(m.at) < time.Second
}

// Done is closed when the program ends.
func (m *Meter) Done() <-chan struct{} { return m.done }

// Err is why the program ended, nil while it runs or when it was stopped.
func (m *Meter) Err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

// Stop ends the program.
func (m *Meter) Stop() {
	if m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	<-m.done
}

// StartMeter listens to the input whose name holds device.
func (b execBackend) StartMeter(device string) (*Meter, error) {
	if device == "" {
		device = DefaultDevice
	}
	var bin string
	var args []string
	switch {
	case b.ffmpeg != "":
		in, err := ffmpegInput(runtime.GOOS, b.ffmpeg, device)
		if err != nil {
			return nil, err
		}
		bin = b.ffmpeg
		for i, v := range in { // a small buffer on the input, for a meter that follows the sound (avfoundation: milliseconds)
			if v == "avfoundation" && i > 0 && in[i-1] == "-f" {
				in = append(append(append([]string{}, in[:i+1]...), "-audio_buffer_size", "20"), in[i+1:]...)
				break
			}
		}
		args = append(append([]string{"-hide_banner", "-loglevel", "error", "-fflags", "nobuffer", "-flags", "low_delay", "-probesize", "32", "-analyzeduration", "0"}, in...), "-ac", "2", "-ar", "48000", "-f", "s16le", "pipe:1")
	case b.sox != "":
		driver := map[string]string{"darwin": "coreaudio", "windows": ""}[runtime.GOOS]
		if runtime.GOOS == "windows" {
			return nil, errors.New("metering with sox is not supported on Windows: install ffmpeg")
		}
		if driver == "" {
			driver = "pulseaudio"
		}
		bin = b.sox
		args = []string{"-q", "-t", driver, device, "-t", "raw", "-e", "signed-integer", "-b", "16", "-r", "48000", "-c", "2", "-"}
	default:
		return nil, ErrNoTool
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errOut, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	m := &Meter{cmd: cmd, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 512)
		for {
			n, _ := errOut.Read(buf)
			if n == 0 {
				return
			}
			m.mu.Lock()
			m.tail = (m.tail + string(buf[:n]))
			if len(m.tail) > 2000 {
				m.tail = m.tail[len(m.tail)-2000:]
			}
			m.mu.Unlock()
		}
	}()
	go func() {
		m.read(out)
		werr := cmd.Wait()
		m.mu.Lock()
		if werr != nil {
			if last := lastLine(m.tail); last != "" {
				m.err = errors.New(last)
			} else {
				m.err = werr
			}
		}
		m.mu.Unlock()
		close(m.done)
	}()
	// A program that is going to refuse the device does so at once.
	select {
	case <-m.done:
		if err := m.Err(); err != nil {
			return nil, fmt.Errorf("%s stopped: %v", bin, err)
		}
		return nil, fmt.Errorf("%s stopped", bin)
	case <-time.After(700 * time.Millisecond):
	}
	return m, nil
}

// read takes in 16 bit stereo samples, one reading for each window.
func (m *Meter) read(r io.Reader) {
	frame := int(48000 * meterWindow / time.Second)
	buf := make([]byte, frame*4)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		var peak [2]float64
		mono := make([]float64, 0, len(buf)/4)
		for i := 0; i+3 < len(buf); i += 4 {
			var sum float64
			for c := 0; c < 2; c++ {
				s := float64(int16(binary.LittleEndian.Uint16(buf[i+2*c:]))) / 32768
				sum += s
				if v := math.Abs(s); v > peak[c] {
					peak[c] = v
				}
			}
			mono = append(mono, sum/2)
		}
		m.hist = append(m.hist, mono...)
		if len(m.hist) > fftSize {
			m.hist = m.hist[len(m.hist)-fftSize:]
		}
		bands := Bands(m.hist, 48000)
		m.mu.Lock()
		m.peak, m.at = peak, time.Now()
		for c := range peak {
			m.acc[c] = max(m.acc[c], peak[c])
		}
		if m.spec == nil {
			m.spec = make([]float64, len(bands))
		}
		for i, v := range bands { // up at once, down slowly
			m.spec[i] = max(v, m.spec[i]*0.82)
		}
		m.mu.Unlock()
	}
}
