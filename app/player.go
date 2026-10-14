package app

import "github.com/moahbeatlab/404tui/formats"

// Player plays a sample through this computer's speakers, to audition a file before it
// goes onto a pad (the unit can only preview what is already on a pad).
type Player interface {
	// Play stops whatever is playing and starts the sample from the beginning.
	Play(formats.Sample) error
	Stop()
	// Playing reports whether a sound is still sounding.
	Playing() bool
}

// Looper is a Player that can also repeat a sample, without a gap that the ear would hear,
// until it is stopped.
type Looper interface {
	Loop(formats.Sample) error
}

// LoopSwapper is a Looper that can change what it loops without a break: the new sample takes
// over when the one playing comes round to its end. Starting a loop when none plays.
type LoopSwapper interface {
	NextLoop(formats.Sample) error
}

// LoopReplacer is a Looper that can change what it loops at once, at the place it has
// reached, instead of at the end of the round.
type LoopReplacer interface {
	ReplaceLoop(formats.Sample) error
}
