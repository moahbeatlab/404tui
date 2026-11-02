package handheld

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

var keyNames = []string{"Up", "Down", "Left", "Right", "A", "B", "X", "Y", "L1", "R1"}

// Event is a key going down or up.
type Event struct {
	Quit   bool // Select+Start
	Key    Key
	Down   bool
	Repeat int
}

// Dispatch gives an event to the app.
func (a *App) Dispatch(e Event) {
	if e.Quit {
		a.RequestQuit()
		return
	}
	if e.Down {
		a.Press(e.Key, e.Repeat)
	} else {
		a.Release(e.Key)
	}
}

// Script plays a string of keys, for tests and screenshots, and returns the names of the snapshots it
// asked for with their frames' index. The words, separated by spaces:
//
//	U D L R A B X Y   a key pressed and let go
//	[ ]               the shoulder buttons L1 and R1
//	Y+R               a chord: Y held, R pressed
//	R*8               a key held down: pressed, then seven repeats
//	@name             take a snapshot called name
func (a *App) Script(s string, snap func(name string)) error {
	for _, w := range strings.Fields(s) {
		if strings.HasPrefix(w, "@") {
			if snap != nil {
				snap(w[1:])
			}
			continue
		}
		if w == "~" { // wait for the transfer
			for i := 0; i < 200 && a.job != nil; i++ {
				if _, _, _, _, fin := a.job.state(); fin {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			continue
		}
		if w == "." { // a pause for the unit to be found
			a.Tick()
			continue
		}
		mod, hasMod := Key(0), false
		if m, rest, ok := strings.Cut(w, "+"); ok {
			km, err := parseKey(m)
			if err != nil {
				return err
			}
			mod, hasMod, w = km, true, rest
			a.Dispatch(Event{Key: mod, Down: true})
		}
		n := 1
		if k, cnt, ok := strings.Cut(w, "*"); ok {
			fmt.Sscanf(cnt, "%d", &n)
			w = k
		}
		k, err := parseKey(w)
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			a.Dispatch(Event{Key: k, Down: true, Repeat: i})
		}
		a.Dispatch(Event{Key: k, Down: false})
		if hasMod {
			a.Dispatch(Event{Key: mod, Down: false})
		}
	}
	return nil
}

func parseKey(s string) (Key, error) {
	if len(s) == 1 {
		if i := strings.Index("UDLRABXY", strings.ToUpper(s)); i >= 0 {
			return Key(i), nil
		}
		switch s { // the shoulder buttons: [ is L1, ] is R1
		case "[":
			return L1, nil
		case "]":
			return R1, nil
		}
	}
	return 0, fmt.Errorf("unknown key %q", s)
}

// Linux input event codes of the gamepad. The R36S's own pad is a gamepad with BTN_DPAD_* or a hat; which
// code is which button differs between firmware images, so the map can be changed (Evdev.Map).
const (
	evKey = 1
	evAbs = 3
)

var defaultMap = map[uint16]Key{
	0x220: Up, 0x221: Down, 0x222: Left, 0x223: Right, // BTN_DPAD_*
	0x131: A,  // BTN_EAST: the right button
	0x130: B,  // BTN_SOUTH: the bottom one
	0x133: X,  // BTN_NORTH: the top one
	0x134: Y,  // BTN_WEST: the left one
	0x136: L1, // BTN_TL, the left shoulder button (seen as 0x136 on the R36S)
	0x137: R1, // BTN_TR
}

// quitCodes are Select and Start; any two of them held together quit. 0x13a/0x13b are BTN_SELECT and BTN_START;
// the R36S's pad (GO-Super Gamepad) reported its Select and Start as 0x2c0 and 0x2c1 (BTN_TRIGGER_HAPPY1, 2).
var quitCodes = map[uint16]bool{0x13a: true, 0x13b: true, 0x2c0: true, 0x2c1: true}

// Evdev reads keys from a Linux input device (/dev/input/eventN). Not tried on a device yet.
func Evdev(path string, m map[uint16]Key, out chan<- Event) error {
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
		held := map[Key]int{}
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
							out <- Event{Quit: true}
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
					hat(out, held, &hatX, val, Left, Right)
				}
				if code == 0x11 {
					hat(out, held, &hatY, val, Up, Down)
				}
			}
		}
	}()
	return nil
}

func hat(out chan<- Event, held map[Key]int, old *int32, v int32, neg, pos Key) {
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

func emit(out chan<- Event, held map[Key]int, k Key, val int32) {
	switch val {
	case 0:
		delete(held, k)
		out <- Event{Key: k}
	case 1:
		held[k] = 0
		out <- Event{Key: k, Down: true}
	case 2:
		held[k]++
		out <- Event{Key: k, Down: true, Repeat: held[k]}
	}
}
