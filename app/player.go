package app

import "github.com/moahbeatlab/404tui/formats"

// Player plays a sample through this computer's speakers, to audition a file before it
// goes onto a pad (the unit can only preview what is already on a pad).
type Player interface {
	// Play stops whatever is playing and starts the sample from the beginning.
	Play(formats.Sample) error
	Stop()
}
