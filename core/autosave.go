package core

import (
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Autosave: every few minutes, when something changed, the project is written to <out>/autosave/PROJECT_nn, never over
// the folder a save wrote. Only the files that differ from the last autosave are written (a project can be 100 MB, and
// the card is slow), in the background, from a snapshot of the file list: the lab replaces files, it does not edit them
// in place, so the snapshot stays as it was. PADCONF.BIN goes last, so a power cut leaves a project that opens.

var autosaveChoices = []int{0, 1, 2, 3, 5, 10, 15} // minutes, 0 is off

type autosave struct {
	mu       sync.Mutex
	every    time.Duration
	lastRun  time.Time
	busy     bool
	dir      string            // the folder the hashes are of
	hashes   map[string]uint32 // what is in it, by file
	sizes    map[string]int
	history  []string // the log, oldest first
	lastDone time.Time
	pending  string // a message for the screen, when a run has finished
}

func (a *App) autosaveRoot() string { return filepath.Join(a.dataDir(), "autosave") }

// loadAutosaveSetting reads the interval the user chose last time (3 minutes at first).
func (a *App) loadAutosaveSetting() {
	a.as.lastRun = time.Now() // the first run is an interval after the start
	a.as.every = 3 * time.Minute
	if m := a.set.AutosaveMin; m != nil && *m >= 0 && *m <= 60 {
		a.as.every = time.Duration(*m) * time.Minute
	}
}

// setAutosave moves the interval one place up or down the choices and remembers it.
func (a *App) setAutosave(dir int) {
	at := 0
	for i, m := range autosaveChoices {
		if time.Duration(m)*time.Minute <= a.as.every {
			at = i
		}
	}
	at = (at + dir + len(autosaveChoices)) % len(autosaveChoices)
	m := autosaveChoices[at]
	a.as.mu.Lock()
	a.as.every = time.Duration(m) * time.Minute
	a.as.mu.Unlock()
	a.set.AutosaveMin = &m
	a.saveSettings()
	a.autosaveLog(fmt.Sprintf("autosave %s", a.autosaveSetting()))
	a.changed = true // the next run is due at the new interval
}

func (a *App) autosaveSetting() string {
	if a.as.every <= 0 {
		return "off"
	}
	return fmt.Sprintf("every %d min", int(a.as.every/time.Minute))
}

// autosaveLog puts a line in the log shown on the SAVE screen (and in logs.log).
func (a *App) autosaveLog(s string) {
	a.as.mu.Lock()
	a.appendLog(s)
	a.as.mu.Unlock()
	log.Println("autosave:", s)
}

// appendLog adds a line to the history; the caller holds the lock.
func (a *App) appendLog(s string) {
	a.as.history = append(a.as.history, fmt.Sprintf("%s  %s", a.clock().Format("15:04:05"), s))
	if len(a.as.history) > 30 {
		a.as.history = a.as.history[len(a.as.history)-30:]
	}
}

// autosaveTick starts a run when one is due and says when one has finished. Run from Tick, on the main goroutine.
func (a *App) autosaveTick() {
	as := &a.as
	as.mu.Lock()
	msg := as.pending
	as.pending = ""
	due := as.every > 0 && !as.busy && a.changed && a.clock().Sub(as.lastRun) >= as.every
	as.mu.Unlock()
	if msg != "" {
		a.say(msg, strings.Contains(msg, "failed"))
	}
	if due {
		a.startAutosave()
	}
}

type snapItem struct {
	name string
	data []byte
}

func (a *App) startAutosave() {
	as := &a.as
	as.mu.Lock()
	if as.busy {
		as.mu.Unlock()
		return
	}
	as.busy, as.lastRun = true, a.clock()
	as.mu.Unlock()
	a.changed = false
	if a.live != nil && a.lab.RemoteCount() > 0 { // the unit has the project: an autosave would hold only what was fetched
		as.mu.Lock()
		as.busy = false
		as.mu.Unlock()
		return
	}
	if !a.hasWork() && a.info.Files <= 1 {
		as.mu.Lock()
		as.busy = false
		as.mu.Unlock()
		return
	}
	var items []snapItem
	err := a.lab.ExportEach(func(name string, data []byte) error {
		if name == "PADCONF.BIN" { // built fresh from a buffer the lab reuses: take a copy
			data = append([]byte(nil), data...)
		}
		items = append(items, snapItem{name, data})
		return nil
	})
	if err != nil {
		as.mu.Lock()
		as.busy = false
		as.mu.Unlock()
		a.autosaveLog("failed: " + err.Error())
		return
	}
	dir := filepath.Join(a.autosaveRoot(), a.ProjectName())
	started := a.clock()
	go func() {
		wrote, total, err := a.writeAutosave(dir, items)
		took := time.Since(started)
		if a.Now != nil {
			took = a.Now().Sub(started)
		}
		// the log line goes in before the run counts as finished, so whoever waits for it finds the line
		as.mu.Lock()
		var line string
		if err != nil {
			as.pending = "autosave failed: " + err.Error()
			line = "failed: " + err.Error()
		} else {
			as.lastDone = a.clock()
			as.pending = fmt.Sprintf("autosaved: %d of %d files", wrote, total)
			line = fmt.Sprintf("saved %d of %d files to %s in %.1f s", wrote, total, filepath.Base(dir), took.Seconds())
		}
		a.appendLog(line)
		as.busy = false
		as.mu.Unlock()
		log.Println("autosave:", line)
	}()
}

// writeAutosave writes what differs from the last run into dir, PADCONF.BIN last, and takes away files the project no
// longer has. It returns how many files it wrote and how many there are.
func (a *App) writeAutosave(dir string, items []snapItem) (wrote, total int, err error) {
	as := &a.as
	if as.dir != dir || as.hashes == nil { // another slot, or the first run: everything is written
		as.dir, as.hashes, as.sizes = dir, map[string]uint32{}, map[string]int{}
	}
	for _, d := range []string{"PTN", "SMPL", "PICTURE"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return 0, 0, err
		}
	}
	put := func(it snapItem) error {
		h := crc32.ChecksumIEEE(it.data)
		if oh, ok := as.hashes[it.name]; ok && oh == h && as.sizes[it.name] == len(it.data) {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(it.name))); err == nil {
				return nil
			}
		}
		dst := filepath.Join(dir, filepath.FromSlash(it.name))
		if !strings.HasPrefix(dst, dir+string(filepath.Separator)) {
			return fmt.Errorf("bad path %q", it.name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, it.data, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
		as.hashes[it.name], as.sizes[it.name] = h, len(it.data)
		wrote++
		return nil
	}
	have := map[string]bool{}
	var conf *snapItem
	for i := range items {
		have[items[i].name] = true
		if items[i].name == "PADCONF.BIN" {
			conf = &items[i]
			continue
		}
		if err := put(items[i]); err != nil {
			return wrote, len(items), err
		}
	}
	if conf != nil {
		if err := put(*conf); err != nil {
			return wrote, len(items), err
		}
	}
	for name := range as.hashes { // files of the last run that are gone
		if !have[name] {
			os.Remove(filepath.Join(dir, filepath.FromSlash(name)))
			delete(as.hashes, name)
			delete(as.sizes, name)
			wrote++
		}
	}
	return wrote, len(items), nil
}

// findAutosave is the newest autosaved project, "" when there is none.
func (a *App) findAutosave() (dir string, when time.Time) {
	ms, _ := filepath.Glob(filepath.Join(a.autosaveRoot(), "PROJECT_*", "PADCONF.BIN"))
	for _, m := range ms {
		if st, err := os.Stat(m); err == nil && st.ModTime().After(when) {
			dir, when = filepath.Dir(m), st.ModTime()
		}
	}
	return dir, when
}

// restoreAutosave opens an autosaved project, in the slot it came from. It is not a save: the project counts as not saved.
func (a *App) restoreAutosave(dir string) {
	if err := a.OpenProject(dir); err != nil {
		a.say("cannot restore: "+err.Error(), true)
		return
	}
	var n int
	if _, err := fmt.Sscanf(filepath.Base(dir), "PROJECT_%d", &n); err == nil && n >= 1 && n <= 99 {
		a.slot = n
	}
	a.sl, a.pat, a.step = nil, 1, 0
	a.bank, a.cur = 0, 0
	a.saved = false
	a.autosaveLog("restored " + filepath.Base(dir))
	a.say("restored the autosave: not saved yet, SAVE writes it", false)
}

// autosaveAgo is for the footer: how long ago the last autosave finished.
func (a *App) autosaveAgo() string {
	a.as.mu.Lock()
	defer a.as.mu.Unlock()
	switch {
	case a.as.every <= 0:
		return "autosave off"
	case a.as.busy:
		return "autosaving..."
	case a.as.lastDone.IsZero():
		return "autosave " + a.autosaveSettingLocked()
	}
	d := a.clock().Sub(a.as.lastDone)
	if d < time.Minute {
		return "autosaved just now"
	}
	return fmt.Sprintf("autosaved %d min ago", int(d/time.Minute))
}

func (a *App) autosaveSettingLocked() string {
	return fmt.Sprintf("every %d min", int(a.as.every/time.Minute))
}

// FlushAutosave runs an autosave now if something changed since the last one and waits for it (a quit calls it).
func (a *App) FlushAutosave() {
	if a.as.every <= 0 || !a.changed {
		return
	}
	a.startAutosave()
	for i := 0; i < 1500; i++ { // up to 15 s
		a.as.mu.Lock()
		busy := a.as.busy
		a.as.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
