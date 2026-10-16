package record

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTheUnitsInputIsFoundInEachSystemsDeviceList(t *testing.T) {
	avf := `[AVFoundation indev @ 0x1] AVFoundation video devices:
[AVFoundation indev @ 0x1] [0] FaceTime HD Camera
[AVFoundation indev @ 0x1] AVFoundation audio devices:
[AVFoundation indev @ 0x1] [0] MacBook Pro Microphone
[AVFoundation indev @ 0x1] [1] SP-404MKII
`
	if got := parseAVFoundation(avf, "404"); got != 1 {
		t.Errorf("avfoundation: %d, want 1", got)
	}
	if got := parseAVFoundation(avf, "nothing"); got != -1 {
		t.Errorf("a missing device is %d", got)
	}
	dshow := `[dshow @ 0x1] "Integrated Camera" (video)
[dshow @ 0x1] "Microphone (SP-404MKII)" (audio)
[dshow @ 0x1]   Alternative name "@device_cm_..."
`
	if got := parseDShow(dshow, "404"); got != "Microphone (SP-404MKII)" {
		t.Errorf("dshow: %q", got)
	}
	pulse := "0\talsa_output.pci.analog\tPipeWire\n1\talsa_output.usb-Roland_SP-404MKII.monitor\tx\n2\talsa_input.usb-Roland_SP-404MKII-00.analog-stereo\tx\n"
	if got := parsePulseSources(pulse, "404"); got != "alsa_input.usb-Roland_SP-404MKII-00.analog-stereo" {
		t.Errorf("pulse: %q (a monitor is not an input)", got)
	}
}

func TestFileNamesAreTheDateAndTime(t *testing.T) {
	got := FileName(time.Date(2026, 10, 17, 14, 32, 5, 0, time.UTC))
	if got != "2026-10-17_14-32-05.wav" {
		t.Errorf("name %q", got)
	}
}

func TestSoxArgsNameTheDriverAndTheFormat(t *testing.T) {
	args, err := soxArgs("darwin", "SP-404MKII", "/tmp/x.wav")
	if err != nil || strings.Join(args, " ") != "-q -t coreaudio SP-404MKII -b 24 -r 48000 -c 2 /tmp/x.wav" {
		t.Errorf("%v %v", args, err)
	}
	if _, err := soxArgs("windows", "x", "y"); err == nil {
		t.Error("sox on Windows should say it cannot")
	}
}

// A program that writes a file and stops when it is told q is a recorder as far as a session
// can tell.
func TestASessionIsStoppedByQAndTheFileStays(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "take.wav")
	s, err := run("sh", []string{"-c", "echo audio > " + path + "; read q"}, path, "sh", func(in io.Writer, p *os.Process) error {
		_, err := in.Write([]byte("q\n"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Size() == 0 {
		t.Error("the file was not started")
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("Done is not closed after Stop")
	}
	if s.Err() != nil {
		t.Errorf("a recording stopped on purpose says %v", s.Err())
	}
	if data, _ := os.ReadFile(path); len(data) == 0 {
		t.Error("the file is empty")
	}
}

func TestAProgramThatRefusesTheDeviceIsReportedAtOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	_, err := run("sh", []string{"-c", "echo 'no such device' >&2; exit 1"}, "x", "sh", func(in io.Writer, p *os.Process) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "no such device") {
		t.Errorf("error %v", err)
	}
}
