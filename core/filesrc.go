package core

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/moahbeatlab/404tui/app"
)

// The sources of the file browser: this computer, and, when a unit is plugged in, its SD card ("SD:") and its own storage. A path
// says where it is: a computer's path is absolute, the unit's are written with slashes, and the card's start with "SD:".

// fileStore is the storage the browser reads.
func (a *App) fileStore() app.Storage {
	if dev, ok := a.unitDevice(); ok {
		return app.NewRouter(dev)
	}
	return app.Local{}
}

// joinPath puts a name in a folder, on the computer or on the unit.
func joinPath(dir, name string) string {
	if app.IsLocal(dir) {
		return filepath.Join(dir, name)
	}
	if strings.HasSuffix(dir, ":") || strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// parentPath is the folder above one; false at the top.
func parentPath(dir string) (string, bool) {
	if app.IsLocal(dir) {
		if dir == "/" || dir == filepath.Dir(dir) {
			return "", false
		}
		return filepath.Dir(dir), true
	}
	if dir == app.CardRoot || dir == app.StorageRoot || !strings.Contains(strings.TrimPrefix(dir, app.CardRoot), "/") {
		if strings.HasPrefix(dir, app.CardRoot) && dir != app.CardRoot {
			return app.CardRoot, true
		}
		return "", false
	}
	return path.Dir(dir), true
}

func baseName(p string) string {
	if app.IsLocal(p) {
		return filepath.Base(p)
	}
	return path.Base(strings.TrimPrefix(p, app.CardRoot))
}

func pathExt(name string) string { return path.Ext(filepath.ToSlash(name)) }

// sourceMenu is the places of the unit when there is one: its card and its storage.
func (a *App) unitPlaces(m *menu, go_ func(dir string) func(int)) {
	if _, ok := a.unitDevice(); !ok {
		return
	}
	m.head("THE UNIT")
	m.items = append(m.items,
		menuItem{label: "SD card", run: go_(app.CardRoot)},
		menuItem{label: "Unit storage", run: go_(app.StorageRoot)})
}
