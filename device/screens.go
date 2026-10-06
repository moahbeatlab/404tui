package device

import (
	"fmt"
	"slices"
)

// A project's display images, in its PICTURE folder on the card.
//
// The device reads these when it loads the project, so a write shows up the next time
// that project is selected (or at the next power-on, for the project that is current).

var (
	// StartupSlots are the frames of the animation shown while a project loads.
	StartupSlots = []string{"startup_1", "startup_2"}
	// ScreenSaverSlots are the frames of the screen saver.
	ScreenSaverSlots = []string{"screen_saver_1", "screen_saver_2", "screen_saver_3", "screen_saver_4"}
)

// ScreenSlots is every slot a project has, startup frames first.
func ScreenSlots() []string {
	return slices.Concat(StartupSlots, ScreenSaverSlots)
}

// ScreenPath is the path of a slot's BMP on the card. project is 1-based.
func ScreenPath(project uint8, slot string) (string, error) {
	if !slices.Contains(ScreenSlots(), slot) {
		return "", RefusedError{fmt.Sprintf("unknown display image %q", slot)}
	}
	return fmt.Sprintf("%s/PICTURE/%s.bmp", projectFolder(project), slot), nil
}

// ReadScreen reads a display image as it is stored; exists is false when the project has
// no such file.
func (d *Device) ReadScreen(project uint8, slot string) (bmp []byte, exists bool, err error) {
	path, err := ScreenPath(project, slot)
	if err != nil {
		return nil, false, err
	}
	if _, exists, err := d.Stat(path); err != nil || !exists {
		return nil, false, err
	}
	bmp, err = d.ReadFile(path)
	return bmp, err == nil, err
}

// WriteScreen replaces a display image. bmp must already be in the card's format.
func (d *Device) WriteScreen(project uint8, slot string, bmp []byte) error {
	path, err := ScreenPath(project, slot)
	if err != nil {
		return err
	}
	return d.WriteFile(path, bmp)
}
