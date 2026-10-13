package app

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/device"
	"github.com/moahbeatlab/404tui/proto"
)

// Previewer is what playing a pad on the unit needs.
type Previewer interface {
	SelectPad(pad proto.PadIndex) error
	PreviewStart(pad proto.PadIndex) error
	PreviewStop(pad proto.PadIndex) error
	Activity() (device.Activity, error)
	AwaitEvent(timeout time.Duration, match func(*proto.Message) bool) (proto.Message, bool)
}

const (
	// settleAfterSelect is how long the unit gets to take in a pad selection before the
	// preview starts.
	settleAfterSelect = 120 * time.Millisecond
	// playedWindow is how long to wait for the unit to say which pad it played.
	playedWindow = 700 * time.Millisecond
)

// playedPad is the pad a "pad sounded" notification (0E pad 00 velocity) names. The reply to
// a preview request has a status of 0 in the same place; the notification has the velocity.
func playedPad(m *proto.Message) bool {
	p := m.Payload()
	return m.Channel == proto.ChannelControl && len(p) >= 4 && p[0] == proto.ReplyPreviewStart && p[3] != 0
}

// StartPreview starts a pad playing on the unit, making sure it is that pad that sounds.
//
// The unit has a quirk: the first preview after the selected pad changed plays the pad that
// was selected before, and only then selects the new one. So the pad is selected first, as
// the official app does on a click. As a safeguard the unit's own "this pad sounded"
// notification is checked too, and if it names another pad, that one is stopped and the
// right one started again.
func StartPreview(u Previewer, pad proto.PadIndex, logf func(msg string, args ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if a, err := u.Activity(); err == nil && (!a.HasSelected || a.Selected != pad) {
		logf("selecting the pad before previewing it", "pad", pad.String(), "was", a.Selected.String(), "had selection", a.HasSelected)
		if err := u.SelectPad(pad); err != nil {
			return err
		}
		time.Sleep(settleAfterSelect)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := u.PreviewStart(pad); err != nil {
			return err
		}
		ev, ok := u.AwaitEvent(playedWindow, playedPad)
		if !ok {
			logf("the unit did not say which pad sounded", "pad", pad.String(), "attempt", attempt)
			return nil
		}
		p := ev.Payload()
		got, valid := proto.NewPadIndex(uint16(p[1]) | uint16(p[2])<<8)
		if valid && got == pad {
			return nil
		}
		logf("the unit played another pad than asked for; stopping it and trying again",
			"asked", pad.String(), "played", got.String(), "attempt", attempt)
		if valid {
			_ = u.PreviewStop(got)
		}
	}
	return fmt.Errorf("the unit kept playing a different pad than %s", pad)
}

// StopPreview stops a pad previewed on the unit.
func StopPreview(u Previewer, pad proto.PadIndex) error {
	return u.PreviewStop(pad)
}
