// Package playback plays samples through the computer's default audio output.
package playback

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/ebitengine/oto/v3"

	"github.com/moahbeatlab/404tui/formats"
)

// Player holds the audio output and the sound playing, if any.
type Player struct {
	ctx *oto.Context

	mu      sync.Mutex
	current *oto.Player
}

// Open opens the default audio output. It fails when there is none, and the tool then
// goes without auditioning files.
func Open() (*Player, error) {
	// The context is fixed to stereo; mono samples are played on both sides.
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   formats.SampleRate,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, fmt.Errorf("opening audio output: %w", err)
	}
	<-ready
	return &Player{ctx: ctx}, nil
}

// Play stops whatever is playing and starts the sample from the beginning.
func (p *Player) Play(s formats.Sample) error {
	p.Stop()
	pcm := make([]byte, 0, len(s.Samples)*4)
	for i := 0; i+s.Channels <= len(s.Samples); i += s.Channels {
		left := s.Samples[i]
		right := s.Samples[i+s.Channels-1]
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(left))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(right))
	}
	player := p.ctx.NewPlayer(bytes.NewReader(pcm))
	p.mu.Lock()
	p.current = player
	p.mu.Unlock()
	player.Play()
	return nil
}

// Stop stops whatever is playing.
func (p *Player) Stop() {
	p.mu.Lock()
	player := p.current
	p.current = nil
	p.mu.Unlock()
	if player != nil {
		player.Pause()
	}
}
