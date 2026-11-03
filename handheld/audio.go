//go:build !windows

package handheld

import "github.com/moahbeatlab/404tui/formats"

// NullAudio takes every sound and plays nothing: it lets the app act as if there was an output (screenshots, tests).
type NullAudio struct{ Played, Loops, Previewed int }

func (n *NullAudio) Play(formats.Sample) error        { n.Played++; return nil }
func (n *NullAudio) Loop(formats.Sample) error        { n.Loops++; return nil }
func (n *NullAudio) ReplaceLoop(formats.Sample) error { n.Loops++; return nil }
func (n *NullAudio) Stop()                            {}

func (n *NullAudio) Preview(formats.Sample) error { n.Previewed++; return nil }
func (n *NullAudio) StopPreview()                 {}
