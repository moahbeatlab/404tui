package core

import (
	"fmt"
	"strings"
	"time"
)

var keyNames = []string{"Up", "Down", "Left", "Right", "A", "B", "X", "Y", "L1", "R1", "L2", "R2", "Sel"}

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
//	[ ]  { }          the shoulder buttons L1, R1 and the triggers L2, R2
//	S                 Select (or Fn), a chord as S+B
//	Y+R               a chord: Y held, R pressed
//	R*8               a key held down: pressed, then seven repeats
//	@name             take a snapshot called name
//	!splash !autosave the splash in front; an autosave run now
//	/text             the search text of the file browser, typed at once
func (a *App) Script(s string, snap func(name string)) error {
	for _, w := range strings.Fields(s) {
		if strings.HasPrefix(w, "@") {
			if snap != nil {
				snap(w[1:])
			}
			continue
		}
		if strings.HasPrefix(w, "/") && len(w) > 1 { // /kick: the text of a search, typed at once
			if a.kb == nil {
				a.openSearch()
			}
			a.searchQ = w[1:]
			a.list()
			continue
		}
		if w == "!splash" { // the splash, for a screenshot
			a.ShowSplash(time.Hour)
			continue
		}
		if w == "!autosave" { // an autosave now, waited for
			a.changed = true
			a.as.lastRun = time.Time{}
			a.autosaveTick()
			for i := 0; i < 300; i++ {
				a.as.mu.Lock()
				busy := a.as.busy
				a.as.mu.Unlock()
				if !busy {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			a.autosaveTick() // the message of the run that finished
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
			km, err := ParseKey(m)
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
		k, err := ParseKey(w)
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

// ParseKey reads the name of a key of a script or a web page: U D L R A B X Y [ ] { } S.
func ParseKey(s string) (Key, error) {
	if len(s) == 1 {
		if i := strings.Index("UDLRABXY", strings.ToUpper(s)); i >= 0 && s != "S" && s != "s" {
			return Key(i), nil
		}
		switch s { // the shoulder buttons: [ is L1, ] is R1
		case "[":
			return L1, nil
		case "]":
			return R1, nil
		case "{":
			return L2, nil
		case "}":
			return R2, nil
		case "S", "s": // Select (or Fn)
			return Sel, nil
		}
	}
	return 0, fmt.Errorf("unknown key %q", s)
}

// holds are the keys whose hold makes a step bigger.
// Holds says whether holding the key down means something (a direction, a shoulder): a step gets bigger.
func Holds(k Key) bool { return k <= Right || k == L1 || k == R1 }
