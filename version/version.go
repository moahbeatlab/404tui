// Package version is the version of 404tui, shown in the corner of the screen, by
// sp404 -version and in the first line of a debug log.
package version

import "runtime/debug"

// Version is the release. It can be set when building: go build -ldflags "-X
// github.com/moahbeatlab/404tui/version.Version=0.2".
var Version = "0.1"

// Date is the day the program was built (2006-01-02, UTC). It can be set when building: -X
// github.com/moahbeatlab/404tui/version.Date=2026-11-04. Without it the day of the commit the program was built from is used.
var Date string

// Built is the build date to show beside the version: Date, else the day of the commit, else "".
func Built() string {
	if Date != "" {
		return Date
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.time" && len(s.Value) >= 10 {
				return s.Value[:10]
			}
		}
	}
	return ""
}

// Full is the version with the build date, for the corner of the screen and sp404 -version.
func Full() string {
	if d := Built(); d != "" {
		return Version + "  " + d
	}
	return Version
}
