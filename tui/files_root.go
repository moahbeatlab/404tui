package tui

import "os"

// localRoot is where browsing this computer's files starts: the home folder.
func localRoot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}
