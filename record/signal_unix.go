//go:build !windows

package record

import (
	"os"
	"syscall"
)

// interrupt asks a program to finish, as Ctrl-C does.
func interrupt(p *os.Process) error { return p.Signal(syscall.SIGINT) }
