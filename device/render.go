package device

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/moahbeatlab/404tui/proto"
	"github.com/moahbeatlab/404tui/proto/fileapi"
)

// Pattern rendering: Bounce and MULTIPAD export (docs/re/04-patterns.md).
//
// The device renders in real time and then writes a complete WAV file into a host file
// handle, sending write and seek requests on channel 6 that the host must serve.

const (
	// Longest silence tolerated while the device renders (it reports progress meanwhile).
	renderIdle = 60 * time.Second
	// Status poll interval that keeps the device rendering.
	renderPoll = 500 * time.Millisecond
	// Handle number the host gives the device; only one render runs at a time.
	renderHandle uint16 = 0
)

// PatternPads are the pads a pattern plays.
func (d *Device) PatternPads(pattern proto.PadIndex) ([]proto.PadIndex, error) {
	payload, err := d.controlCall(short(proto.RenderPadsUsedRequest(pattern)), "pattern pads",
		startsWith(proto.RenderPadsUsedReply, 1))
	if err != nil {
		return nil, err
	}
	pads, ok := proto.ParseRenderPadsUsed(payload)
	if !ok {
		return nil, UnexpectedError{"pattern pads"}
	}
	return pads, nil
}

// BouncePattern bounces a pattern to a WAV file written into out.
func (d *Device) BouncePattern(pattern proto.PadIndex, out io.WriteSeeker) error {
	return d.renderSession(pattern, proto.RenderBounce, out)
}

// RenderPatternPad renders one pad of a pattern (a MULTIPAD export part) into out.
func (d *Device) RenderPatternPad(pattern, pad proto.PadIndex, out io.WriteSeeker) error {
	return d.renderSession(pattern, pad.Index(), out)
}

func (d *Device) renderSession(pattern proto.PadIndex, op uint16, out io.WriteSeeker) error {
	if err := d.fire(short(proto.EditBegin())); err != nil {
		return err
	}
	result := d.renderControl(pattern, proto.RenderBegin)
	if result == nil {
		result = d.render(pattern, op, out)
	}
	ended := d.renderControl(pattern, proto.RenderEnd)
	if err := d.fire(short(proto.EditEnd())); err != nil {
		return err
	}
	if result != nil {
		return result
	}
	return ended
}

func (d *Device) renderControl(pattern proto.PadIndex, op uint16) error {
	_, err := d.controlCall(short(proto.RenderControl(pattern, op, renderHandle)), fmt.Sprintf("render control %d", op),
		func(p []byte) bool {
			return len(p) >= 5 && p[0] == proto.RenderControlReply && binary.LittleEndian.Uint16(p[3:5]) == op
		})
	return err
}

// render starts a render and serves the device's file requests until it reports completion.
func (d *Device) render(pattern proto.PadIndex, op uint16, out io.WriteSeeker) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drain()
	if err := d.send(short(proto.RenderControl(pattern, op, renderHandle))); err != nil {
		return err
	}
	lastPoll, lastHeard := time.Now(), time.Now()
	for {
		// The device pauses rendering when the host goes quiet for a few seconds; keep
		// polling status as the official app does.
		if time.Since(lastPoll) >= renderPoll {
			if err := d.send(short(proto.StatusRequest())); err != nil {
				return err
			}
			lastPoll = time.Now()
		}
		m, err := d.inbox.recv(renderPoll)
		if err == errNoAnswer {
			if time.Since(lastHeard) < renderIdle {
				continue
			}
			return TimeoutError{"render"}
		}
		if err != nil {
			return err
		}
		lastHeard = time.Now()
		p := m.Payload()
		if isControl(&m) && len(p) > 0 && p[0] == proto.ReplyStatus {
			continue
		}
		if isFileReply(&m) {
			if err := d.serveHostRequest(p, out); err != nil {
				return err
			}
			continue
		}
		if isControl(&m) {
			if done, ok := proto.ParseRenderDone(p); ok && done.Op == op {
				if done.Result != 0 {
					return RefusedError{fmt.Sprintf("render (result %d)", done.Result)}
				}
				return nil
			}
		}
		// The 38 acknowledgement and progress (global parameter 86).
		d.pushEvent(m)
	}
}

// serveHostRequest applies one write or seek the device asked for and answers it.
func (d *Device) serveHostRequest(payload []byte, out io.WriteSeeker) error {
	req, err := fileapi.ParseHostRequest(payload)
	if err != nil {
		return fmt.Errorf("protocol: %w", err)
	}
	var result int32
	switch r := req.(type) {
	case fileapi.HostWrite:
		_, err = out.Write(r.Data)
		result = int32(len(r.Data))
	case fileapi.HostSeek:
		var pos int64
		pos, err = out.Seek(int64(r.Offset), io.SeekStart)
		result = int32(pos)
	}
	if err != nil {
		result = -1
	}
	if sendErr := d.send(proto.Short(proto.ChannelFile, fileapi.HostAck())); sendErr != nil {
		return sendErr
	}
	if sendErr := d.send(proto.Long(proto.ChannelFile, fileapi.HostStatus(result))); sendErr != nil {
		return sendErr
	}
	if err != nil {
		return fmt.Errorf("I/O: %w", err)
	}
	return nil
}
