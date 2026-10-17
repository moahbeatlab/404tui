//go:build midi && !windows

package midiin

import (
	"fmt"
	"strings"

	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

// Default is the MIDI input of this build: the system's, through RtMidi.
func Default() Source { return rtSource{} }

type rtSource struct{}

func (rtSource) Ports() ([]string, error) {
	drv, err := rtmididrv.New()
	if err != nil {
		return nil, err
	}
	defer drv.Close()
	ins, err := drv.Ins()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, in := range ins {
		names = append(names, in.String())
	}
	return names, nil
}

func (rtSource) Open(part string, on func(Event)) (func(), string, error) {
	drv, err := rtmididrv.New()
	if err != nil {
		return nil, "", err
	}
	ins, err := drv.Ins()
	if err != nil {
		drv.Close()
		return nil, "", err
	}
	var in drivers.In
	var names []string
	for _, c := range ins {
		names = append(names, c.String())
		if in == nil && strings.Contains(strings.ToLower(c.String()), strings.ToLower(part)) {
			in = c
		}
	}
	if in == nil {
		drv.Close()
		return nil, "", fmt.Errorf("no MIDI input with %q in its name (there are: %s)", part, strings.Join(names, ", "))
	}
	if err := in.Open(); err != nil {
		drv.Close()
		return nil, "", err
	}
	// RtMidi leaves out the clock (it counts as time code), system exclusive and active sensing
	// unless asked; the clock is wanted, and the monitor shows what the unit sends besides.
	stopListening, err := in.Listen(func(msg []byte, _ int32) {
		if e, ok := Parse(msg); ok {
			on(e)
		}
	}, drivers.ListenConfig{
		TimeCode: true,
		SysEx:    true,
	})
	if err != nil {
		in.Close()
		drv.Close()
		return nil, "", err
	}
	return func() {
		stopListening()
		in.Close()
		drv.Close()
	}, in.String(), nil
}
