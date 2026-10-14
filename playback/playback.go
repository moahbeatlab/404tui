// Package playback plays samples through the computer's default audio output.
package playback

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/ebitengine/oto/v3"

	"github.com/moahbeatlab/404tui/formats"
)

// Player holds the audio output and the sound playing, if any.
type Player struct {
	ctx *oto.Context

	mu      sync.Mutex
	current *oto.Player
	// loop is the reader of the loop playing, if one is.
	loop *loopReader
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

// loopReader is an endless reader over a sound: when it reaches the end it starts again, so
// the audio output never runs dry between two rounds.
type loopReader struct {
	pcm []byte
	at  int
	// next is the sound to play from the next round on, when one is waiting; now is one to
	// go on with at once, from the same place.
	mu        sync.Mutex
	next, now []byte
}

func (r *loopReader) Read(b []byte) (int, error) {
	r.mu.Lock()
	if r.now != nil {
		r.pcm, r.now = r.now, nil
		// The same place in the round; a frame is four bytes.
		if len(r.pcm) > 0 {
			r.at = r.at % len(r.pcm) / 4 * 4
		}
	}
	r.mu.Unlock()
	if len(r.pcm) == 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(b) {
		c := copy(b[n:], r.pcm[r.at:])
		n += c
		r.at += c
		if r.at >= len(r.pcm) {
			// A round is over: a new sound waiting takes over here.
			r.at = 0
			r.mu.Lock()
			if r.next != nil {
				r.pcm, r.next = r.next, nil
			}
			r.mu.Unlock()
		}
	}
	return n, nil
}

// pcmOf is a sample as 16-bit stereo for the output, whole frames only.
func pcmOf(s formats.Sample) []byte {
	pcm := make([]byte, 0, len(s.Samples)*4)
	for i := 0; i+s.Channels <= len(s.Samples); i += s.Channels {
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s.Samples[i]))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s.Samples[i+s.Channels-1]))
	}
	return pcm
}

// ReplaceLoop has the loop playing go on with s at once, from where it is in the round; with no
// loop playing it starts one.
func (p *Player) ReplaceLoop(s formats.Sample) error {
	p.mu.Lock()
	loop := p.loop
	playing := p.current != nil && p.current.IsPlaying()
	p.mu.Unlock()
	if loop == nil || !playing {
		return p.Loop(s)
	}
	loop.mu.Lock()
	loop.now, loop.next = pcmOf(s), nil
	loop.mu.Unlock()
	return nil
}

// NextLoop has the loop playing go on with s from its next round, without a break; with no
// loop playing it starts one.
func (p *Player) NextLoop(s formats.Sample) error {
	p.mu.Lock()
	loop := p.loop
	playing := p.current != nil && p.current.IsPlaying()
	p.mu.Unlock()
	if loop == nil || !playing {
		return p.Loop(s)
	}
	loop.mu.Lock()
	loop.next = pcmOf(s)
	loop.mu.Unlock()
	return nil
}

// Loop plays the sample over and over from the beginning, until Stop.
func (p *Player) Loop(s formats.Sample) error {
	p.Stop()
	reader := &loopReader{pcm: pcmOf(s)}
	player := p.ctx.NewPlayer(reader)
	p.mu.Lock()
	p.current, p.loop = player, reader
	p.mu.Unlock()
	player.Play()
	return nil
}

// Playing reports whether a sound is still sounding.
func (p *Player) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current != nil && p.current.IsPlaying()
}

// Stop stops whatever is playing.
func (p *Player) Stop() {
	p.mu.Lock()
	player := p.current
	p.current, p.loop = nil, nil
	p.mu.Unlock()
	if player != nil {
		player.Pause()
	}
}
