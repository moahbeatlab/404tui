package handheld

import (
	"encoding/binary"
	"log"
	"os"
	"time"

	"github.com/moahbeatlab/404tui/core"
)

// Linux input event codes of the gamepad. The R36S's own pad is a gamepad with BTN_DPAD_* or a hat; which
// code is which button differs between firmware images, so the map can be changed (Evdev.Map).
const (
	evKey = 1
	evAbs = 3
)

var defaultMap = map[uint16]core.Key{
	0x220: core.Up, 0x221: core.Down, 0x222: core.Left, 0x223: core.Right, // BTN_DPAD_*
	0x131: core.A,   // BTN_EAST: the right button
	0x130: core.B,   // BTN_SOUTH: the bottom one
	0x133: core.X,   // BTN_NORTH: the top one
	0x134: core.Y,   // BTN_WEST: the left one
	0x136: core.L1,  // BTN_TL, the left shoulder button (seen as 0x136 on the R36S)
	0x137: core.R1,  // BTN_TR
	0x138: core.L2,  // BTN_TL2, the left trigger (seen as 0x138 on the R36S)
	0x139: core.R2,  // BTN_TR2
	0x13a: core.Sel, // BTN_SELECT; the R36S's pad reports Select as 0x2c0 and Fn may be BTN_MODE: the log names a code that is not mapped
	0x2c0: core.Sel,
	0x13c: core.Sel,
}

// quitCodes are Select and Start; any two of them held together quit. 0x13a/0x13b are BTN_SELECT and BTN_START;
// the R36S's pad (GO-Super Gamepad) reported its Select and Start as 0x2c0 and 0x2c1 (BTN_TRIGGER_HAPPY1, 2).
var quitCodes = map[uint16]bool{0x13a: true, 0x13b: true, 0x2c0: true, 0x2c1: true}

// Evdev reads keys from a Linux input device (/dev/input/eventN). Not tried on a device yet.
func Evdev(path string, m map[uint16]core.Key, out chan<- core.Event) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if m == nil {
		m = defaultMap
	}
	go func() {
		defer f.Close()
		buf := make([]byte, 24) // struct input_event on 64 bit: timeval (16), type, code (2 each), value (4)
		held := newChain()
		var hatX, hatY int32
		unknown := map[uint16]bool{}
		quitHeld := map[uint16]bool{}
		for {
			if _, err := f.Read(buf); err != nil {
				return
			}
			typ, code, val := binary.LittleEndian.Uint16(buf[16:]), binary.LittleEndian.Uint16(buf[18:]), int32(binary.LittleEndian.Uint32(buf[20:]))
			switch typ {
			case evKey:
				if quitCodes[code] { // Select and Start together
					if val == 0 {
						delete(quitHeld, code)
					} else if val == 1 {
						quitHeld[code] = true
						if len(quitHeld) >= 2 {
							out <- core.Event{Quit: true}
						}
					}
				}
				if k, ok := m[code]; ok {
					emit(out, held, k, val)
				} else if val == 1 && !unknown[code] { // a button that is not mapped: tell which code it is
					unknown[code] = true
					log.Printf("input: unmapped button, code 0x%x", code)
				}
			case evAbs: // a hat for the D-pad: ABS_HAT0X, ABS_HAT0Y
				if code == 0x10 {
					hat(out, held, &hatX, val, core.Left, core.Right)
				}
				if code == 0x11 {
					hat(out, held, &hatY, val, core.Up, core.Down)
				}
			}
		}
	}()
	return nil
}

func hat(out chan<- core.Event, held *chain, old *int32, v int32, neg, pos core.Key) {
	if *old < 0 {
		emit(out, held, neg, 0)
	}
	if *old > 0 {
		emit(out, held, pos, 0)
	}
	if v < 0 {
		emit(out, held, neg, 1)
	}
	if v > 0 {
		emit(out, held, pos, 1)
	}
	*old = v
}

// chain counts how long a direction key has been held, in units of 160 ms, whatever the pad sends while it is held: key
// repeats (value 2), or the same press again and again (value 1), as the R36S's pad seems to do. A press within 350 ms of the
// last event of that key goes on with the hold.
type chain struct {
	start, last map[core.Key]time.Time
}

const (
	chainGap  = 350 * time.Millisecond
	chainUnit = 160 * time.Millisecond
)

func newChain() *chain {
	return &chain{start: map[core.Key]time.Time{}, last: map[core.Key]time.Time{}}
}

// down is the Repeat of a press: 0 for the first of a hold, then the time held in units, at least 1.
func (c *chain) down(k core.Key, now time.Time) int {
	if t, ok := c.last[k]; ok && now.Sub(t) < chainGap {
		c.last[k] = now
		return max(int(now.Sub(c.start[k])/chainUnit), 1)
	}
	c.start[k], c.last[k] = now, now
	return 0
}

func (c *chain) up(k core.Key, now time.Time) { c.last[k] = now }

func emit(out chan<- core.Event, held *chain, k core.Key, val int32) {
	now := time.Now()
	switch val {
	case 0:
		held.up(k, now)
		out <- core.Event{Key: k}
	case 1, 2:
		rep := 0
		if val == 2 {
			rep = 1
		}
		if core.Holds(k) {
			rep = held.down(k, now)
			if val == 2 {
				rep = max(rep, 1)
			}
		}
		out <- core.Event{Key: k, Down: true, Repeat: rep}
	}
}
