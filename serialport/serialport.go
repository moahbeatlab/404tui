// Package serialport finds and opens the SP-404MKII's USB serial port.
package serialport

import (
	"errors"
	"fmt"
	"log/slog"
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

var log = slog.New(slog.DiscardHandler)

// SetLogger sends discovery and opening diagnostics to l.
func SetLogger(l *slog.Logger) { log = l }

// PortDetail is a serial port the system lists, with why it is or is not taken for the unit.
type PortDetail struct {
	Name, VID, PID, SerialNumber, Product string
	USB                                   bool
	// Matches is set for the ports that belong to an SP-404MKII; Reason says why not for
	// the others.
	Matches bool
	Reason  string
}

// All lists every serial port the system reports, whether or not it is the unit. It is what
// to look at when the unit is plugged in but not found.
func All() ([]PortDetail, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Warn("listing serial ports failed", "err", err)
		return nil, fmt.Errorf("serial port: %w", err)
	}
	out := make([]PortDetail, 0, len(ports))
	for _, p := range ports {
		d := PortDetail{Name: p.Name, VID: p.VID, PID: p.PID, SerialNumber: p.SerialNumber, Product: p.Product, USB: p.IsUSB}
		switch {
		// macOS lists each device twice, as a callout cu.* and a dial-in tty.*. The dial-in
		// one waits for a carrier the device never raises; cu.* is the one to use.
		case runtime.GOOS == "darwin" && strings.HasPrefix(p.Name, "/dev/tty."):
			d.Reason = "dial-in port; the cu.* one is used"
		case !p.IsUSB:
			d.Reason = "not a USB port"
		case !strings.EqualFold(p.VID, fmt.Sprintf("%04x", USBVID)) || !strings.EqualFold(p.PID, fmt.Sprintf("%04x", USBPID)):
			d.Reason = fmt.Sprintf("USB id %s:%s is not %04x:%04x", p.VID, p.PID, USBVID, USBPID)
		default:
			d.Matches = true
		}
		log.Debug("serial port", "name", d.Name, "usb", d.USB, "vid", d.VID, "pid", d.PID, "serial", d.SerialNumber,
			"product", d.Product, "matches", d.Matches, "reason", d.Reason)
		out = append(out, d)
	}
	return out, nil
}

// Find lists the serial ports that belong to an SP-404MKII.
func Find() ([]PortInfo, error) {
	all, err := All()
	if err != nil {
		return nil, err
	}
	var found []PortInfo
	for _, p := range all {
		if p.Matches {
			found = append(found, PortInfo{Name: p.Name, SerialNumber: p.SerialNumber, Product: p.Product})
		}
	}
	log.Debug("looked for the unit", "ports listed", len(all), "found", len(found))
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
	log.Debug("opening the port", "name", name, "baud", Baud, "read timeout", readTimeout())
	port, err := serial.Open(name, &serial.Mode{
		BaudRate:          Baud,
		InitialStatusBits: &serial.ModemOutputBits{RTS: true, DTR: true},
	})
	if err != nil {
		log.Warn("opening the port failed", "name", name, "err", err)
		return nil, fmt.Errorf("serial port: %w", err)
	}
	if err := errors.Join(port.SetReadTimeout(readTimeout()), port.SetRTS(true), port.SetDTR(true)); err != nil {
		log.Warn("setting up the port failed", "name", name, "err", err)
		port.Close()
		return nil, fmt.Errorf("serial port: %w", err)
	}
	log.Debug("port open; RTS and DTR asserted, waiting for the unit to settle", "settle", settle)
	time.Sleep(settle)
	return port, nil
}
