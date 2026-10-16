//go:build !midi

package midiin

// Default is the MIDI input of this build: none.
func Default() Source { return unavailable{} }
