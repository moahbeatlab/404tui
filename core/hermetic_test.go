package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// goldenHome is the home folder the pictures of the handheld's screens and docs/tools.md were made with: the CONFIG screen shows it.
const goldenHome = "/home/hermes"

// fixedBase is a folder with a fixed name for a test whose screens show the paths: the same on Linux and macOS (the temp folder of a Mac is
// a long, different one), and the home folder is the one of the pictures.
func fixedBase(t *testing.T, name string) string {
	t.Helper()
	t.Setenv("HOME", goldenHome)
	t.Setenv("USERPROFILE", goldenHome)
	dir := os.TempDir()
	if runtime.GOOS != "windows" {
		dir = "/tmp"
	}
	return filepath.Join(dir, name)
}
