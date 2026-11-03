//go:build darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// quietStderr sends what the C libraries print on standard error to a file: on the screen of the terminal UI it would
// spoil the picture. It returns where it went.
func quietStderr(path string) string {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return ""
	}
	defer f.Close()
	if unix.Dup2(int(f.Fd()), 2) != nil {
		return ""
	}
	return path
}
