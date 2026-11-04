package record

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

func init() { nativeOpener = openPulse }

// openPulse listens to the PulseAudio (or PipeWire) input whose name holds device, 20 ms of sound at a time.
func openPulse(device string, onFrames func([]float32), onEnd func(error)) (io.Closer, error) {
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
		return nil, fmt.Errorf("%w: %q (is the unit plugged in and on?)", ErrNoDevice, device)
	}
	stream, err := client.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
		onFrames(p)
		return len(p), nil
	}), pulse.RecordSource(src), pulse.RecordSampleRate(48000), pulse.RecordChannels(proto.ChannelMap{proto.ChannelLeft, proto.ChannelRight}), pulse.RecordLatency(0.02))
	if err != nil {
		client.Close()
		return nil, err
	}
	stopped := make(chan struct{})
	stream.Start()
	// the stream ends by itself when the sound server or the device goes away
	go func() {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-t.C:
				if stream.Closed() {
					onEnd(fmt.Errorf("the input is gone"))
					return
				}
			}
		}
	}()
	return closerFunc(func() error {
		close(stopped)
		stream.Close()
		client.Close()
		return nil
	}), nil
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
