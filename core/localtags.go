package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/app"
)

// Your own tags on the samples of this computer: kick, bass, a key, "favourite". A tag on a folder is on everything below it. They are
// kept in tags.json next to the settings, and the search by categories (#) finds them with the tags the names and folders give.

type localTags struct {
	Files   map[string][]string `json:"files,omitempty"`
	Folders map[string][]string `json:"folders,omitempty"`
}

func (a *App) localTagsPath() string { return filepath.Join(a.dataDir(), "tags.json") }

func (a *App) localTagStore() *localTags {
	if a.ltags == nil {
		a.ltags = &localTags{}
		if b, err := os.ReadFile(a.localTagsPath()); err == nil {
			json.Unmarshal(b, a.ltags)
		}
	}
	if a.ltags.Files == nil {
		a.ltags.Files = map[string][]string{}
	}
	if a.ltags.Folders == nil {
		a.ltags.Folders = map[string][]string{}
	}
	return a.ltags
}

func (a *App) saveLocalTags() {
	b, err := json.MarshalIndent(a.localTagStore(), "", " ")
	if err != nil {
		return
	}
	os.MkdirAll(a.dataDir(), 0o755)
	tmp := a.localTagsPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, a.localTagsPath())
	}
}

// userTags are the tags given to the file, and to every folder above it.
func (t *localTags) userTags(path string) []string {
	path = filepath.Clean(path)
	out := slices.Clone(t.Files[path])
	for d := filepath.Dir(path); ; d = filepath.Dir(d) {
		out = append(out, t.Folders[d]...)
		if d == filepath.Dir(d) {
			break
		}
	}
	return out
}

// change adds the tags to the paths (a folder gets them for everything below), or takes them off; it returns how many paths changed.
func (t *localTags) change(paths, tags []string, add bool) int {
	n := 0
	for _, p := range paths {
		p = filepath.Clean(p)
		set := t.Files
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			set = t.Folders
		}
		cur := set[p]
		changed := false
		for _, tg := range tags {
			tg = app.CleanTag(tg)
			if tg == "" {
				continue
			}
			has := slices.Contains(cur, tg)
			switch {
			case add && !has:
				cur, changed = append(cur, tg), true
			case !add && has:
				cur, changed = slices.DeleteFunc(cur, func(x string) bool { return x == tg }), true
			}
		}
		if changed {
			n++
			sort.Strings(cur)
			if len(cur) == 0 {
				delete(set, p)
			} else {
				set[p] = cur
			}
		}
	}
	return n
}

// tagQuick are the categories that a sample is put in with one key press.
var tagQuick = []string{"kick", "snare", "clap", "hat", "perc", "tom", "bass", "sub", "synth", "pad", "lead", "keys", "chord", "vocal", "fx", "loop", "oneshot", "favourite"}

// localTagItems are the tag entries of the files menu on this computer.
func (a *App) localTagItems(m *menu) {
	paths := a.fileTargets()
	m.head("YOUR TAGS (THIS COMPUTER)")
	m.items = append(m.items, menuItem{label: "Search by categories (#)", run: func(int) { a.OpenTagSearch() }})
	if len(paths) == 0 {
		return
	}
	what := fmt.Sprintf("%d", len(paths))
	if e, ok := a.current(); ok && e.dir && len(a.marked()) == 0 {
		what = "this folder: everything in it"
	}
	m.items = append(m.items, menuItem{label: "Tag as (" + what + ")", vals: tagQuick, val: 0, run: func(v int) {
		n := a.localTagStore().change(paths, []string{tagQuick[v]}, true)
		a.saveLocalTags()
		a.say(fmt.Sprintf("%d tagged %s", n, tagQuick[v]), false)
	}})
	for _, add := range []bool{true, false} {
		add := add
		label := map[bool]string{true: "Tag with words...", false: "Take a tag off..."}[add]
		m.items = append(m.items, menuItem{label: label, run: func(int) {
			a.askText(strings.TrimSuffix(label, "..."), "words, spaces between: kick 909 c#m", "", func(s string) {
				words := strings.Fields(s)
				if len(words) == 0 {
					return
				}
				n := a.localTagStore().change(paths, words, add)
				a.saveLocalTags()
				a.say(fmt.Sprintf("%d changed", n), false)
			})
		}})
	}
}
