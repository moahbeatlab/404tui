//go:build windows

package record

import (
	"errors"
	"os"
)

func interrupt(p *os.Process) error { return errors.New("a program cannot be interrupted here") }
