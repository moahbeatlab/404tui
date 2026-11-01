package handheld

import "github.com/moahbeatlab/404tui/formats"

// NullAudio takes every sound and plays nothing: it lets the app act as if there was an output (screenshots, tests).
type NullAudio struct{ Played, Loops int }

func (n *NullAudio) Play(formats.Sample) error        { n.Played++; return nil }
func (n *NullAudio) Loop(formats.Sample) error        { n.Loops++; return nil }
func (n *NullAudio) ReplaceLoop(formats.Sample) error { n.Loops++; return nil }
func (n *NullAudio) Stop()                            {}
