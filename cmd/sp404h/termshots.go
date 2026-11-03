//go:build termshots

package main

import (
	"github.com/moahbeatlab/404tui/core"
	"github.com/moahbeatlab/404tui/platform/terminal"
)

// termShot saves the screen as a computer's terminal draws it. It is only in builds with -tags termshots, so that the
// handheld's program carries none of the terminal's code (nor its libraries).
func termShot(app *core.App, path string) error {
	return core.WritePNG(path, terminal.Image(terminal.Cells(app.RenderCells(), terminal.FullGlyphs)))
}
