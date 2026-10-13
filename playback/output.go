package playback

import (
	"os/exec"
	"runtime"
	"strings"
)

// OutputName is the name of the output device sound goes to: the system's default output,
// since that is the only one this package can play on. It is empty where it cannot be found.
// On macOS it asks system_profiler, which takes a moment: call it in the background.
func OutputName() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("system_profiler", "SPAudioDataType").Output()
	if err != nil {
		return ""
	}
	return defaultOutput(string(out))
}

// defaultOutput finds the device marked "Default Output Device: Yes" in system_profiler's
// listing, where each device is a line of 8 spaces, a name and a colon.
func defaultOutput(listing string) string {
	device := ""
	for _, line := range strings.Split(listing, "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case indent == 8 && strings.HasSuffix(trimmed, ":"):
			device = strings.TrimSuffix(trimmed, ":")
		case trimmed == "Default Output Device: Yes":
			return device
		}
	}
	return ""
}
