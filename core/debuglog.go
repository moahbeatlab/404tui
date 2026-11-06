package core

import (
	"os"

	"github.com/moahbeatlab/404tui/dbg"
	"github.com/moahbeatlab/404tui/record"
)

// The debug log (package dbg): turned on with -v, or in CONFIG; what the program does and how long it takes goes to a file.

// assignPad puts a sample on a pad (lab.Assign), and tells the debug log.
func (a *App) assignPad(n, id int, name string) error {
	err := a.lab.Assign(n, id, name)
	if dbg.Enabled() {
		l := dbg.Logger.Info
		if err != nil {
			l = dbg.Logger.Warn
		}
		l("pad assigned", "pad", a.label(n), "name", name, "live", a.live != nil, "err", err)
	}
	return err
}

// applyDebug turns the log on at the start when the settings say so.
func (a *App) applyDebug() {
	record.UseNative = !a.set.NoNative
	if a.set.Debug != dbg.Off && !dbg.Enabled() {
		dbg.Set(a.set.Debug, "")
	}
}

// debugModes are what CONFIG offers for the log.
var debugModes = []string{"off", "on", "on, with the wire bytes"}

// setDebug turns the log to a mode (0 off, 1 on, 2 with the wire bytes), keeps it, and says where the file is.
func (a *App) setDebug(mode int) {
	mode = (mode%3 + 3) % 3
	path, err := dbg.Set(mode, "")
	if err != nil {
		a.say("debug log: "+err.Error(), true)
		return
	}
	a.set.Debug = mode
	a.saveSettings()
	if mode == dbg.Off {
		a.say("debug log off", false)
		return
	}
	a.say("debug log "+debugModes[mode]+": "+path, false)
}

// debugValue is the CONFIG row for the log: its mode, and the file.
func (a *App) debugValue() string {
	if !dbg.Enabled() {
		return "off"
	}
	return debugModes[dbg.Mode()]
}

var _ = os.Stat
