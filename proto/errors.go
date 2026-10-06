// Package proto is a sans-IO implementation of the SP-404MKII serial link protocol.
//
// Everything here is derived from the specification in docs/re/: framing and the file
// API in 01-transport.md, parameters and pad commands in 03-parameters.md.
package proto

import (
	"errors"
	"fmt"
)

// ErrNotSysex reports a payload that is not shaped like a file API message.
var ErrNotSysex = errors.New("not a file API payload (missing F0 41 7A … F7)")

// TruncatedError reports a payload shorter than its layout requires.
type TruncatedError struct {
	Need, Have int
}

func (e TruncatedError) Error() string {
	return fmt.Sprintf("payload too short: need %d bytes, have %d", e.Need, e.Have)
}

// BadSeptetError reports a byte with bit 7 set where a septet was expected.
type BadSeptetError byte

func (e BadSeptetError) Error() string {
	return fmt.Sprintf("byte 0x%02x is not a valid septet", byte(e))
}

// UnexpectedCommandError reports a reply whose command byte is not the expected one.
type UnexpectedCommandError byte

func (e UnexpectedCommandError) Error() string {
	return fmt.Sprintf("unexpected reply command 0x%02x", byte(e))
}

// BadPadError reports a pad name or index outside A1..J16.
type BadPadError string

func (e BadPadError) Error() string {
	return fmt.Sprintf("invalid pad name %q (expected A1..J16)", string(e))
}
