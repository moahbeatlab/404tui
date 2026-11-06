package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/moahbeatlab/404tui/app"
)

// Tags and the index of the unit's files. The tags of every sample on the card (worked out from the file name and the
// folders it is in, plus the ones you give: "favourite", "wobbly") are kept in one file on the card, so a search by
// tag is quick and the index is the same for every program that reads the card. It needs a unit: a build or a moment
// without one does not offer it, and the computer's folders keep to the name search.

func (a *App) tagsReady() bool {
	_, ok := a.unitDevice()
	return ok
}

// tagIndex is the index of the card, read once.
func (a *App) tagIndex() *app.TagIndex {
	if a.tags == nil {
		t, err := app.LoadTagIndex(a.fileStore())
		if err != nil {
			a.say(err.Error(), true)
			return nil
		}
		a.tags = t
	}
	return a.tags
}

func (a *App) saveTags() {
	if a.tags == nil {
		return
	}
	if err := a.tags.Save(a.fileStore()); err != nil {
		a.say("the tags were not saved: "+err.Error(), true)
	}
}

// indexUnitFolder reads the folder and everything below it, tags each sample and saves the index.
func (a *App) indexUnitFolder(dir string) {
	t := a.tagIndex()
	if t == nil {
		return
	}
	st := a.fileStore()
	var n int
	var done bool
	a.startJob(0, func(progress func(done, total int, name string)) error {
		var err error
		n, done, err = app.IndexTags(st.ListDir, dir, t, app.DefaultLimits, func(p app.TagProgress) {
			progress(p.Folders, p.Folders+p.Queued, baseName(p.Current))
		}, nil)
		if err != nil {
			return err
		}
		return t.Save(st)
	}, func(err error) {
		if err != nil {
			a.say("indexing failed: "+err.Error(), true)
			return
		}
		msg := fmt.Sprintf("%d samples indexed in %s", n, short2(dir, 24))
		if !done {
			msg += " (stopped at the limit)"
		}
		a.say(msg, false)
	})
}

// tagSearchAsk asks for the tags to look for; the list is then the samples that have them.
func (a *App) tagSearchAsk() {
	t := a.tagIndex()
	if t == nil {
		return
	}
	if len(t.Files) == 0 {
		a.say("nothing is indexed yet: index a folder first", true)
		return
	}
	top := t.Facets("", a.dir, 8)
	hint := "words start a tag"
	if len(top) > 0 {
		var w []string
		for _, f := range top {
			w = append(w, f.Tag)
		}
		hint = "e.g. " + strings.Join(w, " ")
	}
	a.askText("search the tags", hint, "", func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		a.tagSearch, a.searchQ = true, strings.TrimSpace(s)
		a.list()
		a.say(fmt.Sprintf("%d samples tagged %q: %s ends the search", len(a.entries), a.searchQ, map[bool]string{true: "Esc or h", false: "<"}[Terminal]), false)
	})
}

func (a *App) tagFiles(paths []string, add bool) {
	t := a.tagIndex()
	if t == nil || len(paths) == 0 {
		return
	}
	title := map[bool]string{true: "tag with", false: "take the tag off"}[add]
	a.askText(title, "words, spaces between", "", func(s string) {
		words := strings.Fields(s)
		if len(words) == 0 {
			return
		}
		var n int
		if add {
			n = t.AddUserTags(paths, words)
		} else {
			n = t.RemoveUserTags(paths, words)
		}
		a.saveTags()
		a.say(fmt.Sprintf("%d changed", n), false)
	})
}

// tagItems are the menu entries for tags: the file or marked files, and the folder.
func (a *App) tagItems(m *menu) {
	if !a.tagsReady() {
		return
	}
	paths := a.fileTargets()
	m.head("TAGS (THE UNIT'S SAMPLES)")
	m.items = append(m.items,
		menuItem{label: fit("Index "+short2(a.dir, 30), 40), run: func(int) { a.indexUnitFolder(a.dir) }},
		menuItem{label: "Search by tags...", run: func(int) { a.tagSearchAsk() }})
	if len(paths) > 0 {
		m.items = append(m.items,
			menuItem{label: fmt.Sprintf("Tag (%d)...", len(paths)), run: func(int) { a.tagFiles(paths, true) }},
			menuItem{label: fmt.Sprintf("Take a tag off (%d)...", len(paths)), run: func(int) { a.tagFiles(paths, false) }},
			menuItem{label: fmt.Sprintf("Favourite (%d)", len(paths)), run: func(int) {
				if t := a.tagIndex(); t != nil {
					n := t.AddUserTags(paths, []string{app.FavouriteTag})
					a.saveTags()
					a.say(fmt.Sprintf("%d are favourites now", n), false)
				}
			}})
	}
}

// tagsOf are the tags of a file, for the panel.
func (a *App) tagsOf(path string) string {
	if a.tags == nil {
		return ""
	}
	tags := a.tags.Tags(path)
	sort.Strings(tags)
	return strings.Join(tags, " ")
}
