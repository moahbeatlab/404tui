package record

import (
	"fmt"
	"strings"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

// ringRate is the rate the ring listens at: what the unit's USB audio runs at.
const ringRate = 48000

// StartRing listens to the PulseAudio (or PipeWire) input whose name holds device, keeping the last seconds.
func (b execBackend) StartRing(device string, seconds int) (*Ring, error) {
	if device == "" {
		device = DefaultDevice
	}
	client, err := pulse.NewClient(pulse.ClientApplicationName("404tui"))
	if err != nil {
		return nil, fmt.Errorf("sound server: %w", err)
	}
	sources, err := client.ListSources()
	if err != nil {
		client.Close()
		return nil, err
	}
	var src *pulse.Source
	for _, s := range sources {
		if !strings.HasSuffix(s.ID(), ".monitor") && strings.Contains(strings.ToLower(s.ID()+" "+s.Name()), strings.ToLower(device)) {
			src = s
			break
		}
	}
	if src == nil {
		client.Close()
		return nil, fmt.Errorf("no audio input with %q in its name: is the unit plugged in and on?", device)
	}
	r := NewRing(ringRate, seconds)
	stream, err := client.NewRecord(pulse.Int16Writer(func(p []int16) (int, error) {
		r.Write(p, time.Now())
		return len(p), nil
	}), pulse.RecordSource(src), pulse.RecordSampleRate(ringRate), pulse.RecordChannels(proto.ChannelMap{proto.ChannelLeft, proto.ChannelRight}), pulse.RecordLatency(0.02))
	if err != nil {
		client.Close()
		return nil, err
	}
	r.stop = func() { stream.Close(); client.Close() }
	stream.Start()
	// The stream ends by itself when the sound server or the device goes away.
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-r.done:
				return
			case <-t.C:
				if stream.Closed() {
					r.fail(fmt.Errorf("the input is gone"))
					return
				}
			}
		}
	}()
	return r, nil
}
