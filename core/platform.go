package core

import (
	"github.com/moahbeatlab/404tui/midiin"
	"github.com/moahbeatlab/404tui/record"
)

// Platform is what the app takes from the system it runs on. Everything the screens and features do is in this package; what differs
// between a computer's terminal (Linux, macOS, Windows) and the handheld is handed in here: a computer has a recorder for the
// unit's audio and MIDI, the handheld may not; Windows records from another kind of device than Linux. A feature whose piece is
// missing says so instead of failing.
type Platform struct {
	// Name is what the system is called ("linux", "macos", "windows", "handheld").
	Name string
	// Audio opens the sound output. It is called in the background: opening it can take long or block.
	Audio func() (Audio, error)
	// Recorder records the unit's audio to a file; nil when this system cannot.
	Recorder record.Backend
	// MIDI listens to the unit's MIDI; nil when this build has none.
	MIDI midiin.Source
	// Connect finds a plugged-in SP-404MKII for the UNIT screen.
	Connect func() (Link, error)
	// Keyboard is set when there is a computer's keyboard (typed text, more keys).
	Keyboard bool
}

// SetPlatform gives the app what the system offers.
func (a *App) SetPlatform(p Platform) {
	a.plat = p
	if p.Connect != nil {
		a.Connect = p.Connect
	}
}

// Platform is what the app was given.
func (a *App) Platform() Platform { return a.plat }
