package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The folder chooser: a dialog with the folders of the computer (or a drive) to walk through, for where a backup or an export goes.
// The first row is the folder you are in, the rows below are the folders in it; l opens one, h goes up, Enter uses the one in front,
// x makes a new folder, [ ] jump between the places (home, the samples, the card folder, drives, the top).

type dirPicker struct {
	title  string
	dir    string
	names  []string // the folders in dir
	cur    int      // 0 is the folder itself, i the folder names[i-1]
	places []string
	onDone func(dir string)
}

// pickFolder opens the chooser at start (or the nearest folder above it that is there); onDone gets the folder that was chosen.
func (a *App) pickFolder(title, start string, onDone func(dir string)) {
	dir := start
	for dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			dir = ""
			break
		}
		dir = next
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	dp := &dirPicker{title: title, dir: dir, onDone: onDone}
	seen := map[string]bool{}
	add := func(p string) {
		if st, err := os.Stat(p); err == nil && st.IsDir() && !seen[p] {
			seen[p] = true
			dp.places = append(dp.places, p)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home)
	}
	add(a.Root)
	add(filepath.Dir(filepath.Clean(a.OutDir)))
	for _, p := range []string{"/Volumes", "/mnt", "/media", "/"} {
		add(p)
	}
	a.dp = dp
	a.dpList("")
}

// dpList reads the folders of the chooser's folder, and puts the cursor on `on` when it is one of them.
func (a *App) dpList(on string) {
	dp := a.dp
	dp.names, dp.cur = nil, 0
	if up := filepath.Dir(dp.dir); up != dp.dir {
		dp.names = append(dp.names, "..") // the folder above, a row of its own: always a way to a different place
	}
	des, err := os.ReadDir(dp.dir)
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	for _, de := range des {
		isDir := de.IsDir()
		if !isDir && de.Type()&os.ModeSymlink != 0 { // a link to a folder is one too
			if st, err := os.Stat(filepath.Join(dp.dir, de.Name())); err == nil && st.IsDir() {
				isDir = true
			}
		}
		if isDir && !strings.HasPrefix(de.Name(), ".") {
			dp.names = append(dp.names, de.Name())
		}
	}
	start := 0
	if len(dp.names) > 0 && dp.names[0] == ".." {
		start = 1
	}
	rest := dp.names[start:]
	sort.Slice(rest, func(i, j int) bool { return strings.ToLower(rest[i]) < strings.ToLower(rest[j]) })
	for i, n := range dp.names {
		if n == on {
			dp.cur = i + 1
		}
	}
}

func (a *App) dpKey(k Key) {
	dp := a.dp
	rows := len(dp.names) + 1
	switch k {
	case Up:
		dp.cur = (dp.cur + rows - 1) % rows
	case Down:
		dp.cur = (dp.cur + 1) % rows
	case L1, R1: // the places: home, the samples, the card folder, drives, the top
		if n := len(dp.places); n > 0 {
			at := -1
			for i, p := range dp.places {
				if p == dp.dir {
					at = i
				}
			}
			if k == R1 {
				at++
			} else if at <= 0 {
				at = n - 1
			} else {
				at--
			}
			dp.dir = dp.places[at%n]
			a.dpList("")
		}
	case Right:
		if dp.cur > 0 {
			if dp.names[dp.cur-1] == ".." {
				a.dpKey(Left)
				return
			}
			dp.dir = filepath.Join(dp.dir, dp.names[dp.cur-1])
			a.dpList("")
		}
	case Left:
		if up := filepath.Dir(dp.dir); up != dp.dir {
			from := filepath.Base(dp.dir)
			dp.dir = up
			a.dpList(from)
		}
	case A:
		dir := dp.dir
		if dp.cur > 0 {
			dir = filepath.Join(dp.dir, dp.names[dp.cur-1]) // ".." is the folder above
		}
		a.dp = nil
		if dp.onDone != nil {
			dp.onDone(dir)
		}
	case B:
		a.dp = nil
	case X: // a new folder in this one
		a.askText("new folder in "+short2(dp.dir, 30), "", "", func(name string) {
			if name == "" || strings.ContainsAny(name, `/\\`) {
				return
			}
			if err := os.MkdirAll(filepath.Join(dp.dir, name), 0o755); err != nil {
				a.say(err.Error(), true)
				return
			}
			if a.dp == dp {
				a.dpList(name)
			}
		})
	}
}

func (a *App) drawDirPicker(g Gfx) {
	dp := a.dp
	wc := min(max(contentCols-6, 40), 80)
	vis := min(len(dp.names)+1, max(Rows-3-7, 4))
	hr := vis + 6
	x := (contentCols - wc) / 2
	y := 1 + max((Rows-3-hr)/2, 0)
	g.Fill(x, y, wc, hr, cPanel2)
	g.Frame(x, y, wc, hr, cAmber)
	g.Text(x+2, y+1, fit(strings.ToUpper(dp.title), wc-6), cAmber, true)
	g.Text(x+2, y+2, fit(short2(dp.dir, wc-6), wc-6), cText, false)
	g.HRule(x+1, y+3, wc-2, cLine)
	top := max(0, min(dp.cur-vis/2, len(dp.names)+1-vis))
	for r := 0; r < vis; r++ {
		i := top + r
		yy := y + 4 + r
		label := "use this folder"
		if i > 0 {
			label = dp.names[i-1] + "/"
		}
		fg := cText
		if i == 0 {
			fg = cAmber
		}
		if i == dp.cur {
			g.Highlight(x+1, yy, wc-2, 1)
			fg = cWhite
		}
		g.Text(x+2, yy, fit(label, wc-6), fg, i == dp.cur || i == 0)
	}
	if len(dp.names) == 0 {
		g.Text(x+2, y+5, "no folders in it", cDim, false)
	}
	g.Text(x+2, y+hr-2, fit("l open  h up  Enter use  x new folder  / type a path  [ ] places  Esc cancel", wc-4), cDim, false)
}

// freeTarget goes on with root/name when nothing is there; when it is, it asks: overwrite it, use a different name (typed, and asked
// about again when that one is taken too), or give up. A name that cannot be changed (rename false) offers overwrite or giving up.
func (a *App) freeTarget(root, name string, rename bool, onDone func(dir string)) {
	dir := filepath.Join(root, name)
	if _, err := os.Stat(dir); err != nil {
		onDone(dir)
		return
	}
	m := &menu{title: short2(dir, 40) + " is there already"}
	m.items = append(m.items, menuItem{label: "Overwrite it", run: func(int) {
		if err := os.RemoveAll(dir); err != nil {
			a.say(err.Error(), true)
			return
		}
		onDone(dir)
	}})
	if rename {
		m.items = append(m.items, menuItem{label: "Use a different name...", run: func(int) {
			a.askText("a different name in "+short2(root, 30), "Enter goes on", name+"_2", func(s string) {
				s = strings.TrimSpace(s)
				if s == "" || strings.ContainsAny(s, `/\`) {
					return
				}
				a.freeTarget(root, s, true, onDone)
			})
		}})
	}
	m.items = append(m.items, menuItem{label: "Cancel", run: func(int) {}})
	a.menu = m
}

// dpType is / of a keyboard in the folder chooser: a path is typed, and the chooser goes there (~ is the home folder).
func (a *App) dpType() {
	dp := a.dp
	if dp == nil {
		return
	}
	a.askText("go to the folder", "any path, from / ; ~ is your home folder", dp.dir, func(s string) {
		s = expandHome(strings.TrimSpace(s))
		if st, err := os.Stat(s); err != nil || !st.IsDir() {
			a.say(s+" is not a folder", true)
			return
		}
		if a.dp == dp {
			dp.dir = filepath.Clean(s)
			a.dpList("")
		}
	})
}

// DirPickerType is / of a keyboard while the folder chooser is open; it says whether it was.
func (a *App) DirPickerType() bool {
	if a.dp == nil || a.prompt != nil {
		return false
	}
	a.dpType()
	return true
}
