package tui

import (
	"os"
	"path/filepath"
)

// localRoot is where browsing this computer's files starts: the volume the home folder is on,
// so other mounts are reachable, not only $HOME. On macOS and Linux that is "/", so /Volumes
// (external drives, network shares) is a step up; on Windows it is the home drive, e.g. "C:\".
func localRoot() string {
	home := localHome()
	if vol := filepath.VolumeName(home); vol != "" {
		return vol + string(filepath.Separator) // e.g. "C:\" on Windows
	}
	return "/"
}

// localHome is the folder a fresh browse of this computer lands on and opens down to: the home
// folder, where files usually are, while leaving the rest of the volume one step up.
func localHome() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}
