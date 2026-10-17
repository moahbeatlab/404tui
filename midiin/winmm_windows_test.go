package midiin

import "testing"

// There may be no MIDI device on the computer that runs this: listing them must still work.
func TestWindowsInputsCanBeListed(t *testing.T) {
	if _, err := Default().Ports(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Default().Open("no such input anywhere", func(Event) {}); err == nil {
		t.Error("opened an input that is not there")
	}
}
