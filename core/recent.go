package core

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// The projects opened last, for the launcher at the start: the folder, and what it held when it was opened (pads with samples, the
// tempo of bank A), so the launcher says it without reading every project again.
type recentProject struct {
	Dir  string    `json:"dir"`
	Pads int       `json:"pads"`
	BPM  float64   `json:"bpm"`
	When time.Time `json:"when"`
}

const recentMax = 6

// RememberProject puts a project folder that was just opened at the top of the recent ones. An autosave and the copy of a unit's
// project (the unit cache) are not a place to go back to, and are left out.
func (a *App) RememberProject(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	for _, skip := range []string{a.autosaveRoot(), filepath.Dir(a.unitCache(0))} {
		if r, err := filepath.Rel(skip, abs); err == nil && !strings.HasPrefix(r, "..") {
			return
		}
	}
	used := 0
	for _, p := range a.info.Pads {
		if p.File {
			used++
		}
	}
	list := []recentProject{{Dir: abs, Pads: used, BPM: a.tempoOf(0), When: time.Now()}}
	for _, r := range a.set.Recent {
		if r.Dir != abs && len(list) < recentMax {
			list = append(list, r)
		}
	}
	a.set.Recent = list
	a.saveSettings()
}

// recent is the recent projects whose folders are still there.
func (a *App) recent() []recentProject {
	var out []recentProject
	for _, r := range a.set.Recent {
		if isProjectFolder(r.Dir) {
			out = append(out, r)
		}
	}
	return out
}

// openRecent opens recent project i (0..), as opening its folder would.
func (a *App) openRecent(i int) bool {
	rs := a.recent()
	if i < 0 || i >= len(rs) {
		return false
	}
	a.openFolderProject(rs[i].Dir)
	return true
}

// recentLabel is how a recent project is named: a folder PROJECT_nn says the folder it is in as well (PROJECT_02  Live set).
func recentLabel(dir string) string {
	base := filepath.Base(dir)
	var n int
	if _, err := fmt.Sscanf(strings.ToUpper(base), "PROJECT_%d", &n); err == nil {
		parent := filepath.Base(filepath.Dir(dir))
		if strings.EqualFold(parent, "SP-404MKII") { // a card: ROLAND/SP-404MKII/PROJECT_nn
			parent = filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(dir))))
		}
		return base + "  " + parent
	}
	return base
}

// ago is when, said the short way: today 19:40, yesterday, Nov 9.
func ago(when, now time.Time) string {
	y1, m1, d1 := when.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "today " + when.Format("15:04")
	case now.Sub(when) < 48*time.Hour && now.AddDate(0, 0, -1).Day() == d1:
		return "yesterday"
	case y1 == y2:
		return when.Format("Jan 2")
	}
	return when.Format("Jan 2 2006")
}
