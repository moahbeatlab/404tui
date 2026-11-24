package core

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

// On a computer the third column of FILES shows what a track says about itself: its tags (ID3, Vorbis, RIFF INFO) and
// how it is stored. They are read once per file (again when it changed), from its head, never by decoding it.

type fileTagsCache struct {
	mod  time.Time
	size int64
	t    formats.FileTags
	err  error
}

func (a *App) fileTags(path string) (formats.FileTags, int64, error) {
	if !app.IsLocal(path) {
		return formats.FileTags{}, 0, fmt.Errorf("not on this computer")
	}
	st, err := os.Stat(path)
	if err != nil {
		return formats.FileTags{}, 0, err
	}
	if c, ok := a.fileTagMemo[path]; ok && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.t, st.Size(), c.err
	}
	t, err := formats.ReadFileTags(path)
	if a.fileTagMemo == nil || len(a.fileTagMemo) > 2000 {
		a.fileTagMemo = map[string]fileTagsCache{}
	}
	a.fileTagMemo[path] = fileTagsCache{st.ModTime(), st.Size(), t, err}
	return t, st.Size(), err
}

// drawFileTags fills the third column (x, w wide, from row 2) with the tags of the file at path.
func (a *App) drawFileTags(g Gfx, path string, x, w int) {
	t, size, err := a.fileTags(path)
	if err != nil {
		g.Text(x, 2, fitRunes(err.Error(), w), cDim, false)
		return
	}
	y, last := 2, Rows-5
	line := func(s string, c color.RGBA, bold bool) {
		if y <= last && s != "" {
			g.Text(x, y, fitRunes(s, w), c, bold)
			y++
		}
	}
	if t.Title != "" {
		for _, l := range wrapWords(t.Title, w, 2) {
			line(l, cWhite, true)
		}
	}
	if t.Artist != "" {
		for _, l := range wrapWords(t.Artist, w, 2) {
			line(l, cAmber, true)
		}
	}
	if t.Title != "" || t.Artist != "" {
		y++
	}
	lw := 8
	row := func(k, v string, c color.RGBA) {
		if v == "" || y > last {
			return
		}
		ls := wrapWords(v, max(w-lw, 8), 3)
		for i, l := range ls {
			if y > last {
				return
			}
			if i == 0 {
				g.Text(x, y, k, cDim, false)
			}
			g.Text(x+lw, y, l, c, false)
			y++
		}
	}
	row("album", t.Album, cText)
	row("remix", t.Remixer, cText)
	row("label", t.Label, cText)
	row("year", t.Year, cText)
	row("genre", t.Genre, cText)
	row("bpm", t.BPM, cAmber)
	row("key", t.Key, cAmber)
	row("comment", t.Comment, cDim)
	if t.Empty() {
		line("(no tags in this file)", cDim, false)
	}
	y++
	var st []string
	if t.Seconds > 0 {
		s := int(t.Seconds + .5)
		st = append(st, fmt.Sprintf("%d:%02d", s/60, s%60))
	}
	if t.Kbps > 0 {
		k := fmt.Sprintf("%d kbps", t.Kbps)
		if t.VBR {
			k += " vbr"
		}
		st = append(st, k)
	}
	if t.Rate > 0 {
		st = append(st, strings.TrimSuffix(fmt.Sprintf("%.1f", float64(t.Rate)/1000), ".0")+" kHz")
	}
	switch t.Channels {
	case 1:
		st = append(st, "mono")
	case 2:
		st = append(st, "stereo")
	}
	st = append(st, fmt.Sprintf("%.1f MB", float64(size)/(1<<20)))
	for _, l := range wrapWords(strings.Join(st, " · "), w, 2) {
		line(l, cDim, false)
	}
	if tg := a.tagsOf(path); tg != "" {
		y++
		row("yours", tg, cGood)
	}
}

// wrapWords breaks s into lines of at most w characters, at most n lines (the last cut short with ~).
func wrapWords(s string, w, n int) []string {
	var out []string
	for _, l := range wrap(s, w) {
		r := []rune(l)
		for len(r) > w { // a word longer than the line
			out, r = append(out, string(r[:w])), r[w:]
		}
		out = append(out, string(r))
	}
	if len(out) > n {
		out = out[:n]
		r := []rune(out[n-1])
		out[n-1] = string(r[:min(len(r), w-1)]) + "~"
	}
	return out
}

// fitRunes cuts s to n characters (not bytes), ending in ~ when it was longer.
func fitRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:max(n, 0)])
	}
	return string(r[:n-1]) + "~"
}
