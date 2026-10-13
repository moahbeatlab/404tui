// Package device is a session with a Roland SP-404MKII over its serial link.
//
// Device owns the port and a reader goroutine. Transactions are serialised: each call
// sends a request and waits for the matching reply; anything else that arrives meanwhile
// (status notifications, duplicate echoes) is kept as an event.
package device

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moahbeatlab/404tui/proto"
)

const maxEvents = 256

// LevelTrace is the log level that includes every message on the wire, as hex.
const LevelTrace = slog.LevelDebug - 4

const levelTrace = LevelTrace

var (
	ErrClosed   = errors.New("connection closed")
	errNoAnswer = errors.New("no message in time")
)

// TimeoutError reports a reply that did not arrive in time.
type TimeoutError struct{ What string }

func (e TimeoutError) Error() string { return "timed out waiting for " + e.What }

// RefusedError reports something the device declined to do.
type RefusedError struct{ What string }

func (e RefusedError) Error() string { return "device refused " + e.What }

// UnexpectedError reports a reply of the wrong shape.
type UnexpectedError struct{ What string }

func (e UnexpectedError) Error() string { return "unexpected reply to " + e.What }

// UnsupportedError reports something the device would accept but not keep, explained for
// whoever asked.
type UnsupportedError struct{ Reason string }

func (e UnsupportedError) Error() string { return e.Reason }

// Port is the byte stream to the device. Read may return 0 bytes without an error when
// nothing arrived within the port's own timeout, and Close must unblock a pending Read.
type Port interface {
	io.ReadWriteCloser
}

// Device is an open session. Its methods are safe for concurrent use.
type Device struct {
	portName string
	port     Port
	log      *slog.Logger

	// writeMu guards writes to the port.
	writeMu sync.Mutex
	// mu serialises transactions and guards events.
	mu     sync.Mutex
	events []proto.Message

	inbox *queue
	stop  atomic.Bool
	done  chan struct{}
}

// Option configures a Device.
type Option func(*Device)

// WithLogger sends the session's diagnostics to log instead of discarding them.
func WithLogger(log *slog.Logger) Option {
	return func(d *Device) { d.log = log }
}

// New starts a session on an open port. The port must already be set up for the device
// (line state, settle time); Close closes it.
func New(port Port, portName string, opts ...Option) *Device {
	d := &Device{
		portName: portName,
		port:     port,
		log:      slog.New(slog.DiscardHandler),
		inbox:    newQueue(),
		done:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(d)
	}
	go d.readLoop()
	return d
}

func (d *Device) PortName() string {
	return d.portName
}

// Close ends the session and closes the port.
func (d *Device) Close() error {
	d.stop.Store(true)
	err := d.port.Close()
	<-d.done
	return err
}

// TakeEvents returns the messages that arrived without matching a request (notifications,
// extra echoes).
func (d *Device) TakeEvents() []proto.Message {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drain()
	events := d.events
	d.events = nil
	return events
}

// drain moves everything already received into the events. The caller holds mu.
func (d *Device) drain() {
	for {
		m, ok := d.inbox.tryRecv()
		if !ok {
			return
		}
		d.pushEvent(m)
	}
}

func (d *Device) pushEvent(m proto.Message) {
	d.log.Debug("event", "channel", fmt.Sprintf("%02x", byte(m.Channel)), "payload", hexPreview(m.Payload()))
	if len(d.events) == maxEvents {
		d.events = d.events[1:]
	}
	d.events = append(d.events, m)
}

func (d *Device) send(msg proto.Message) error {
	b := msg.Encode()
	d.log.Log(context.Background(), levelTrace, ">", "bytes", hexPreview(b))
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	for len(b) > 0 {
		n, err := d.port.Write(b)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return fmt.Errorf("I/O: %w", err)
			}
			// Anything else (a broken pipe once the cable is out, on macOS and Linux alike)
			// means the port is gone, and saying so lets the app drop it and reconnect.
			d.log.Warn("serial write failed", "err", err)
			return ErrClosed
		}
		b = b[n:]
	}
	return nil
}

// transact sends msg, then returns the first incoming message accepted by match.
func (d *Device) transact(msg proto.Message, what string, timeout time.Duration, match func(*proto.Message) bool) (proto.Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drain()
	start := time.Now()
	if err := d.send(msg); err != nil {
		d.log.Warn("tx failed to send", "what", what, "err", err)
		return proto.Message{}, err
	}
	m, err := d.wait(what, timeout, match)
	d.logTx(what, start, err)
	return m, err
}

// logTx records a finished transaction and how long it took; the ones that fail are warnings.
func (d *Device) logTx(what string, start time.Time, err error) {
	took := time.Since(start).Round(100 * time.Microsecond)
	if err != nil {
		d.log.Warn("tx", "what", what, "took", took, "err", err)
		return
	}
	d.log.Debug("tx", "what", what, "took", took)
}

// wait returns the first incoming message accepted by match. The caller holds mu.
func (d *Device) wait(what string, timeout time.Duration, match func(*proto.Message) bool) (proto.Message, error) {
	deadline := time.Now().Add(timeout)
	for {
		m, err := d.inbox.recv(time.Until(deadline))
		switch {
		case err == errNoAnswer:
			d.log.Warn("timed out waiting for a reply", "what", what, "timeout", timeout)
			return proto.Message{}, TimeoutError{what}
		case err != nil:
			return proto.Message{}, err
		case match(&m):
			return m, nil
		}
		d.pushEvent(m)
	}
}

// AwaitEvent waits up to timeout for an unsolicited message accepted by match and returns it,
// removing only that message from the events: the others stay for whoever reads them.
func (d *Device) AwaitEvent(timeout time.Duration, match func(*proto.Message) bool) (proto.Message, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	deadline := time.Now().Add(timeout)
	for {
		d.drain()
		for i := range d.events {
			if match(&d.events[i]) {
				m := d.events[i]
				d.events = append(d.events[:i], d.events[i+1:]...)
				return m, true
			}
		}
		m, err := d.inbox.recv(time.Until(deadline))
		if err != nil {
			return proto.Message{}, false
		}
		d.pushEvent(m)
	}
}

// RawExchange is a debugging aid: send a raw message and collect everything that arrives
// for wait.
func (d *Device) RawExchange(msg proto.Message, wait time.Duration) ([]proto.Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.send(msg); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	var out []proto.Message
	for {
		m, err := d.inbox.recv(time.Until(deadline))
		switch {
		case err == errNoAnswer:
			return out, nil
		case err != nil:
			return nil, err
		}
		out = append(out, m)
	}
}

// fire sends without waiting for a reply.
func (d *Device) fire(msg proto.Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.log.Debug("fire (no reply expected)", "channel", fmt.Sprintf("%02x", byte(msg.Channel)), "payload", hexPreview(msg.Payload()))
	return d.send(msg)
}

func (d *Device) readLoop() {
	defer close(d.done)
	defer d.inbox.close()
	defer d.log.Debug("reader stopped", "port", d.portName)
	d.log.Debug("reader started", "port", d.portName)
	var dec proto.Decoder
	buf := make([]byte, 64*1024)
	for !d.stop.Load() {
		n, err := d.port.Read(buf)
		if n > 0 {
			dec.Push(buf[:n])
			for {
				m, ok := dec.Next()
				if !ok {
					break
				}
				d.log.Log(context.Background(), levelTrace, "<", "channel", fmt.Sprintf("%02x", byte(m.Channel)), "payload", hexPreview(m.Payload()))
				d.inbox.push(m)
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			if !d.stop.Load() {
				d.log.Warn("serial read failed", "err", err)
			}
			return
		}
	}
}

func hexPreview(b []byte) string {
	const limit = 48
	shown := make([]string, 0, limit)
	for _, c := range b[:min(len(b), limit)] {
		shown = append(shown, fmt.Sprintf("%02x", c))
	}
	s := strings.Join(shown, " ")
	if len(b) > limit {
		return fmt.Sprintf("%s … (+%d bytes)", s, len(b)-limit)
	}
	return s
}

// queue is the unbounded mailbox between the reader goroutine and the caller in a
// transaction.
type queue struct {
	mu     sync.Mutex
	items  []proto.Message
	closed bool
	wake   chan struct{}
}

func newQueue() *queue {
	return &queue{wake: make(chan struct{}, 1)}
}

func (q *queue) push(m proto.Message) {
	q.mu.Lock()
	q.items = append(q.items, m)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// close marks the sender gone: what is queued can still be received, then ErrClosed.
func (q *queue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	close(q.wake)
}

func (q *queue) tryRecv() (proto.Message, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return proto.Message{}, false
	}
	m := q.items[0]
	q.items = q.items[1:]
	return m, true
}

// recv waits up to timeout for a message; errNoAnswer when none came.
func (q *queue) recv(timeout time.Duration) (proto.Message, error) {
	timer := time.NewTimer(max(timeout, 0))
	defer timer.Stop()
	for {
		if m, ok := q.tryRecv(); ok {
			return m, nil
		}
		q.mu.Lock()
		closed := q.closed
		q.mu.Unlock()
		if closed {
			// A message pushed just before the close must still be delivered.
			if m, ok := q.tryRecv(); ok {
				return m, nil
			}
			return proto.Message{}, ErrClosed
		}
		select {
		case <-q.wake:
		case <-timer.C:
			if m, ok := q.tryRecv(); ok {
				return m, nil
			}
			return proto.Message{}, errNoAnswer
		}
	}
}
