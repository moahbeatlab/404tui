package playback

import "testing"

func TestDefaultOutputIsFoundInTheListing(t *testing.T) {
	listing := `Audio:

    Devices:

        WH-1000XM3:

          Default Input Device: Yes
          Manufacturer: Apple Inc.
          Transport: Bluetooth

        WH-1000XM3:

          Default Output Device: Yes
          Default System Output Device: Yes
          Transport: Bluetooth

        SP-404MKII-OUT:

          Transport: USB
`
	if got := defaultOutput(listing); got != "WH-1000XM3" {
		t.Errorf("default output = %q", got)
	}
	if got := defaultOutput("nothing here"); got != "" {
		t.Errorf("found %q in nothing", got)
	}
}
