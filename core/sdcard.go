package core

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// An SP-404MKII card in the computer is found by itself: a mounted drive with an EXPORT/PROJECT folder (at its top, or one or
// two folders down, as ROLAND/SP-404MKII/EXPORT/PROJECT), which is where the unit reads projects from the card. PROJECTS offers
// it as the offline folder, and says so when the offline folder is a card that is not in.

// sdCards is what was found last, looked for again every few seconds (the screen asks every frame).
var sdCards struct {
	at    time.Time
	found []string
}

// mountRoots are the folders drives are mounted in (macOS, then Linux); a test puts its own.
var mountRoots = func() []string {
	roots := []string{"/Volumes", "/media", "/mnt"}
	if u, err := user.Current(); err == nil {
		roots = append(roots, filepath.Join("/media", u.Username), filepath.Join("/run/media", u.Username))
	}
	return roots
}

// spCards are the EXPORT/PROJECT folders of the SP cards in the computer.
func spCards() []string {
	if time.Since(sdCards.at) < 3*time.Second {
		return sdCards.found
	}
	var found []string
	for _, r := range mountRoots() {
		vols, _ := filepath.Glob(filepath.Join(r, "*"))
		for _, v := range vols {
			if st, err := os.Stat(v); err != nil || !st.IsDir() || v == "/Volumes/Macintosh HD" {
				continue
			}
			for _, pat := range []string{"EXPORT/PROJECT", "*/EXPORT/PROJECT", "*/*/EXPORT/PROJECT"} {
				ms, _ := filepath.Glob(filepath.Join(v, pat))
				for _, m := range ms {
					if st, err := os.Stat(m); err == nil && st.IsDir() && !slices.Contains(found, m) {
						found = append(found, m)
					}
				}
			}
		}
	}
	sdCards.at, sdCards.found = time.Now(), found
	return found
}

// cardLabel names a card's project folder by its drive: "SP404 (EXPORT/PROJECT)".
func cardLabel(p string) string {
	for _, r := range mountRoots() {
		if rel, err := filepath.Rel(r, p); err == nil && !strings.HasPrefix(rel, "..") {
			parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
			if len(parts) == 2 {
				return fmt.Sprintf("%s (%s)", parts[0], parts[1])
			}
		}
	}
	return p
}

// useCard makes a card's EXPORT/PROJECT the offline folder.
func (a *App) useCard(p string) {
	a.set.OfflineDir = p
	a.offlineChosen = true
	a.saveSettings()
	a.say("offline: the projects of the SD card "+cardLabel(p), false)
}

// cardOffer is a card that is in and is not the offline folder ("" when there is none).
func (a *App) cardOffer() string {
	for _, c := range spCards() {
		if filepath.Clean(c) != filepath.Clean(a.offlineDir()) {
			return c
		}
	}
	return ""
}

// offlineMissing says whether the offline folder that was chosen is not there (a card that was taken out). The program's own
// card folder is made when it is first written to.
func (a *App) offlineMissing() bool {
	if a.set.OfflineDir == "" {
		return false
	}
	st, err := os.Stat(a.offlineDir())
	return err != nil || !st.IsDir()
}

// UseCardKey is c on PROJECTS offline: the SP card that is in becomes the offline folder.
func (a *App) UseCardKey() {
	if a.scr != Unit || a.onlineNow() || a.Modal() || a.prompt != nil || a.job != nil {
		return
	}
	if c := a.cardOffer(); c != "" {
		a.useCard(c)
		return
	}
	if len(spCards()) > 0 {
		a.say("the SD card is the offline folder already", false)
		return
	}
	a.say("no SP-404 card found (a drive with an EXPORT/PROJECT folder)", true)
}

// newOfflineHere starts a new, empty project as PROJECT_nn of the offline folder: it asks for its name, writes it there
// and opens it in the editor. Work in the editor that is not saved is asked about first.
func (a *App) newOfflineHere(n int) {
	if a.offlineMissing() {
		a.say("the offline folder is not there: "+short2(a.offlineDir(), 40), true)
		return
	}
	ask := func() {
		a.askText(fmt.Sprintf("name of the new PROJECT_%02d", n+1), "up to 31 letters; empty: no name", "", func(s string) { a.newOfflineNow(n, s) })
	}
	if a.hasWork() && !a.saved {
		m := &menu{title: "the editor has work that is not saved"}
		m.items = append(m.items,
			menuItem{label: fmt.Sprintf("Leave it and start PROJECT_%02d new", n+1), run: func(int) { ask() }},
			menuItem{label: "No", run: func(int) {}})
		a.menu = m
		return
	}
	ask()
}

func (a *App) newOfflineNow(n int, name string) {
	if _, ok := a.offlineProject(n); ok {
		a.say(fmt.Sprintf("PROJECT_%02d is there already", n+1), true)
		return
	}
	a.stop()
	a.lab.NewProject(fmt.Sprintf("PROJECT_%02d", n+1))
	if name = cleanName(name, 31); name != "" {
		a.lab.SetName(name)
	}
	a.slot = n + 1
	a.sl, a.pat, a.step = nil, 1, 0
	a.bank, a.cur = 0, 0
	a.pcm = map[int]int{}
	a.undo, a.redo = nil, nil
	a.refresh()
	names, err := a.exportTo(a.offlineDir())
	if err != nil {
		a.say("cannot write it: "+err.Error(), true)
		return
	}
	a.wrote, a.saved, a.changed = names, true, false
	a.set.LastOffline = n + 1
	a.saveSettings()
	dir := filepath.Join(a.offlineDir(), a.ProjectName())
	a.RememberProject(dir)
	what := a.ProjectName()
	if name != "" {
		what = fmt.Sprintf("%q (%s)", name, a.ProjectName())
	}
	a.say(fmt.Sprintf("new project %s in %s: put samples on its pads, then write it here again", what, short2(a.offlineDir(), 30)), false)
	a.go_(Pads)
}
