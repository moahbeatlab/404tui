// Package serialport finds and opens the SP-404MKII's USB serial port.
package serialport

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// USB ids of the SP-404MKII's serial function.
const (
	USBVID = 0x0582
	USBPID = 0x02E7
	Baud   = 921_600

	settle = 350 * time.Millisecond
)

// ErrNotFound reports that no SP-404MKII is connected.
var ErrNotFound = fmt.Errorf("no SP-404MKII found (USB %04x:%04x)", USBVID, USBPID)

type PortInfo struct {
	Name         string
	SerialNumber string
	Product      string
}

// readTimeout of the reader. On Windows, synchronous reads and writes on the port handle
// are serialised, so a write waits for a pending read to time out: keep it short there.
func readTimeout() time.Duration {
	if runtime.GOOS == "windows" {
		return 2 * time.Millisecond
	}
	return 50 * time.Millisecond
}

// Find lists the serial ports that belong to an SP-404MKII.
func Find() ([]PortInfo, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("serial port: %w", err)
	}
	var found []PortInfo
	for _, p := range ports {
		// macOS lists each device twice, as a callout cu.* and a dial-in tty.*. The dial-in
		// one waits for a carrier the device never raises; cu.* is the one to use.
		if runtime.GOOS == "darwin" && strings.HasPrefix(p.Name, "/dev/tty.") {
			continue
		}
		if !p.IsUSB || !strings.EqualFold(p.VID, fmt.Sprintf("%04x", USBVID)) || !strings.EqualFold(p.PID, fmt.Sprintf("%04x", USBPID)) {
			continue
		}
		found = append(found, PortInfo{Name: p.Name, SerialNumber: p.SerialNumber, Product: p.Product})
	}
	return found, nil
}

// OpenFirst opens the first SP-404MKII found and returns it with its port name.
func OpenFirst() (serial.Port, string, error) {
	ports, err := Find()
	if err != nil {
		return nil, "", err
	}
	if len(ports) == 0 {
		return nil, "", ErrNotFound
	}
	port, err := Open(ports[0].Name)
	return port, ports[0].Name, err
}

// Open opens a port and readies it for the device.
func Open(name string) (serial.Port, error) {
	// The official app asserts both lines (control line state 0x0003) and pauses before its
	// first message; the control channel does not answer otherwise.
	port, err := serial.Open(name, &serial.Mode{
		BaudRate:          Baud,
		InitialStatusBits: &serial.ModemOutputBits{RTS: true, DTR: true},
	})
	if err != nil {
		return nil, fmt.Errorf("serial port: %w", err)
	}
	if err := errors.Join(port.SetReadTimeout(readTimeout()), port.SetRTS(true), port.SetDTR(true)); err != nil {
		port.Close()
		return nil, fmt.Errorf("serial port: %w", err)
	}
	time.Sleep(settle)
	return port, nil
}
