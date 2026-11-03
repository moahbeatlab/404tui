package core

import (
	"fmt"
	"time"

	"github.com/moahbeatlab/404tui/lab"
)

// Undo and redo (Y+L2, Y+R2), for everything that changes the project: a snapshot is taken before each key, and if the key
// changed the project it goes on the undo stack. Nothing has to be told which actions are edits. A key held down that goes on
// changing something (a tempo) is one step. The stack is 30 deep, and is cut shorter when the old states would hold more
// than 192 MB of samples that are not in the project any more.

const (
	maxUndo      = 30
	maxUndoBytes = 192 << 20
	coalesceWith = 800 * time.Millisecond
)

type undoEntry struct {
	snap  *lab.Snap
	label string
}

// Press handles one key going down: a key with Y held and L2 or R2 is undo or redo; any other key is run, and the project
// before it is kept when it changed.
func (a *App) Press(k Key, repeat int) {
	if a.toolBase != nil && a.menu == nil { // the tool's dialog is gone: its trial stands as it is
		a.toolBase = nil
	}
	if a.yHeld && a.splash == nil && (k == L2 || k == R2) {
		if a.toolBase != nil {
			if repeat == 0 {
				a.yUsed = true
				a.say("a tool is open: Keep it commits, B throws the trial away", true)
			}
			return
		}
		if repeat == 0 {
			a.yUsed = true
			if k == L2 {
				a.undoStep()
			} else {
				a.redoStep()
			}
		}
		return
	}
	if a.toolBase != nil { // inside a tool's dialog the changes are trials: the dialog commits them as one step, or throws them away
		a.press(k, repeat)
		return
	}
	pre := a.lab.Snapshot()
	a.press(k, repeat)
	if a.toolBase != nil { // this key opened a tool: what it did so far is a trial
		return
	}
	a.commitUndo(pre, k, repeat)
}

// commitUndo keeps the project as it was before (pre) on the undo stack when it has changed since. A held key that goes on
// changing something, or the edits made while A is held, are one step.
func (a *App) commitUndo(pre *lab.Snap, k Key, repeat int) {
	if !a.lab.Changed(pre) {
		return
	}
	defer a.liveSync() // a unit that is edited live gets what changed
	now := a.clock()
	merge := len(a.undo) > 0 && ((repeat > 0 && k == a.undoKey && now.Sub(a.undoAt) < coalesceWith) ||
		(a.aHeld && a.undoSes == a.aSession))
	if merge {
		a.undoAt = now
		return
	}
	label := a.msg
	if label == "" {
		label = "a change on " + screenNames[a.scr]
	}
	a.undo = append(a.undo, undoEntry{pre, fit(label, 40)})
	a.redo = nil
	a.undoKey, a.undoAt = k, now
	a.undoSes = 0
	if a.aHeld {
		a.undoSes = a.aSession
	}
	a.trimUndo()
}

// trimUndo keeps the stack within its depth and its memory.
func (a *App) trimUndo() {
	for len(a.undo) > maxUndo {
		a.undo = a.undo[1:]
	}
	for len(a.undo) > 1 && a.undoHeld() > maxUndoBytes {
		a.undo = a.undo[1:]
	}
}

// undoHeld is the file data the undo stack keeps alive that the project does not have any more.
func (a *App) undoHeld() int {
	n := 0
	for i, e := range a.undo {
		if i+1 < len(a.undo) {
			n += e.snap.HeldBytes(a.undo[i+1].snap)
		} else {
			n += a.lab.HeldBytesNow(e.snap)
		}
	}
	return n
}

func (a *App) undoStep() {
	if len(a.undo) == 0 {
		a.say("nothing to undo", false)
		return
	}
	e := a.undo[len(a.undo)-1]
	a.undo = a.undo[:len(a.undo)-1]
	a.redo = append(a.redo, undoEntry{a.lab.Snapshot(), e.label})
	a.restore(e.snap)
	a.liveSync()
	a.say(fmt.Sprintf("undone: %s  (%d more)", e.label, len(a.undo)), false)
}

func (a *App) redoStep() {
	if len(a.redo) == 0 {
		a.say("nothing to redo", false)
		return
	}
	e := a.redo[len(a.redo)-1]
	a.redo = a.redo[:len(a.redo)-1]
	a.undo = append(a.undo, undoEntry{a.lab.Snapshot(), e.label})
	a.restore(e.snap)
	a.liveSync()
	a.say(fmt.Sprintf("redone: %s  (%d more)", e.label, len(a.redo)), false)
}

// restore puts the project back to a snapshot and brings what depends on it (decoded pads, the slicer, loops) along.
func (a *App) restore(s *lab.Snap) {
	a.restoreProject(s)
	if a.menu != nil || a.picker != nil || a.kb != nil {
		a.menu, a.picker, a.kb = nil, nil, nil
	}
}

// restoreProject puts the project back to a snapshot and brings what depends on it along (decoded pads, the slicer, loops).
func (a *App) restoreProject(s *lab.Snap) {
	a.lab.Restore(s)
	for _, id := range a.pcm {
		a.lab.Free(id)
	}
	a.pcm = map[int]int{}
	a.aud = nil
	a.sl = nil
	a.refresh()
	a.step = min(a.step, a.steps()-1)
	a.autoRows()
	a.replay()
	a.liveArpUpdate()
}
