package handout

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"fmt"
	"html"
	"strings"
	"time"
)

// An EPUB 3 (with the table of contents of EPUB 2 too, for older readers): a page with every bank at a glance, then a page for each bank
// with patterns, each pattern a grid for each of its bars.

const css = `body { font-family: serif; line-height: 1.35; margin: 0 4%; }
h1 { font-size: 1.6em; margin: 0.4em 0 0.2em; }
h2 { font-size: 1.25em; margin: 1.2em 0 0.1em; page-break-after: avoid; }
.kick { font-family: sans-serif; font-size: 0.75em; letter-spacing: 0.1em; text-transform: uppercase; color: #444; margin: 0; }
.meta { font-family: monospace; font-size: 0.85em; color: #333; margin: 0 0 0.4em; }
.small { font-size: 0.85em; color: #333; }
table.glance { width: 100%; border-collapse: collapse; margin-top: 0.6em; }
.glance td { vertical-align: top; padding: 0.45em 0.5em 0.45em 0; border-bottom: 1px solid #999; }
.glance td.bk { font-size: 1.7em; font-weight: bold; width: 1.4em; line-height: 1; }
.glance td.kk { width: 5.5em; }
.glance .key { font-weight: bold; font-size: 1.1em; }
.glance .pads { font-family: sans-serif; font-size: 0.8em; }
.bar { font-family: sans-serif; font-size: 0.75em; color: #444; margin: 0.5em 0 0.1em; page-break-after: avoid; }
div.grid { page-break-inside: avoid; }
img { width: 100%; }
ul.toc { list-style: none; padding: 0; }
ul.toc li { margin: 0.3em 0; }
hr { border: 0; border-top: 2px solid #000; }
`

type file struct {
	name, mime, id string
	data           []byte
	page           bool
}

type tocEntry struct {
	title, href string
	kids        []tocEntry
}

// Build writes the handout of p as an EPUB.
func Build(p Project, at time.Time) ([]byte, error) {
	title := p.Notes.Title
	if title == "" {
		title = p.Name
	}
	if title == "" {
		title = "Project"
	}
	var files []file
	var toc []tocEntry
	add := func(f file) { files = append(files, f) }

	// at a glance
	var used []int
	for b := 0; b < 10; b++ {
		if p.bankUsed(b) {
			used = append(used, b)
		}
	}
	var g strings.Builder
	fmt.Fprintf(&g, `<p class="kick">404tui handout · %s</p><h1>%s</h1>`, esc(p.Name), esc(title))
	fmt.Fprintf(&g, `<p class="meta">%s BPM · %d banks · %d pads · %d patterns · %s</p><hr/>`, bpm(p.Tempo), len(used), p.samples(), len(p.Patterns), at.Format("2 Jan 2006"))
	g.WriteString(`<table class="glance">`)
	for _, b := range used {
		l := bankLetter(b)
		key := p.Notes.Keys[l]
		if key == "" {
			key = "–"
		}
		var pads []string
		for i := b * 16; i < b*16+16; i++ {
			if !p.Pads[i].Sample {
				continue
			}
			s := esc(p.padName(i))
			if k := p.Notes.Keys[p.Pads[i].Label]; k != "" {
				s = "<b>" + s + " (" + esc(k) + ")</b>"
			}
			pads = append(pads, s)
		}
		pats := p.patternsOf(b)
		line := ""
		if role := p.Notes.Banks[l]; role != "" {
			line = "<b>" + esc(role) + "</b><br/>"
		}
		if len(pads) == 0 {
			line += "no samples"
		} else {
			line += strings.Join(pads, " · ")
		}
		if len(pats) > 0 {
			var ls []string
			for _, pt := range pats {
				ls = append(ls, fmt.Sprintf(`<a href="bank-%s.xhtml#%s">%s</a>`, l, pt.Label, pt.Label))
			}
			line += "<br/>patterns " + strings.Join(ls, " ")
		}
		fmt.Fprintf(&g, `<tr><td class="bk">%s</td><td class="kk"><span class="key">%s</span><br/><span class="small">%s BPM</span></td><td class="pads">%s</td></tr>`,
			l, esc(key), bpm(p.Banks[b]), line)
	}
	g.WriteString(`</table>`)
	add(file{name: "glance.xhtml", id: "glance", page: true, data: page("At a glance", g.String())})
	toc = append(toc, tocEntry{title: "At a glance", href: "glance.xhtml"})

	// a page for each bank with patterns
	for _, b := range used {
		pats := p.patternsOf(b)
		if len(pats) == 0 {
			continue
		}
		l := bankLetter(b)
		var s strings.Builder
		fmt.Fprintf(&s, `<p class="kick">Bank %s · patterns · %s BPM</p><h1>Bank %s</h1>`, l, bpm(p.Banks[b]), l)
		s.WriteString(`<p class="small">Black: an accent (velocity 100 or more), grey: a softer note, a bar after it: the note is held; a number: semitones from the pad's own pitch.</p>`)
		entry := tocEntry{title: "Bank " + l + " patterns", href: "bank-" + l + ".xhtml"}
		for _, pt := range pats {
			name := p.Notes.Patterns[pt.Label]
			head := pt.Label
			if name != "" {
				head += " " + name
			}
			fmt.Fprintf(&s, `<h2 id="%s">%s</h2>`, pt.Label, esc(head))
			fmt.Fprintf(&s, `<p class="meta">%d bar%s · %d/4 · %d note%s · %d pad%s</p>`, pt.Bars, plural(pt.Bars), pt.Beats, len(pt.Notes), plural(len(pt.Notes)), len(padsOf(pt)), plural(len(padsOf(pt))))
			for bar := 0; bar < pt.Bars; bar++ {
				img, err := BarPNG(p, pt, bar)
				if err != nil {
					return nil, err
				}
				name := fmt.Sprintf("img/%s-%d.png", pt.Label, bar+1)
				add(file{name: name, mime: "image/png", id: fmt.Sprintf("i%s-%d", pt.Label, bar+1), data: img})
				s.WriteString(`<div class="grid">`)
				if pt.Bars > 1 {
					fmt.Fprintf(&s, `<p class="bar">bar %d of %d</p>`, bar+1, pt.Bars)
				}
				fmt.Fprintf(&s, `<img src="%s" alt="%s bar %d"/></div>`, name, pt.Label, bar+1)
			}
			entry.kids = append(entry.kids, tocEntry{title: head, href: "bank-" + l + ".xhtml#" + pt.Label})
		}
		add(file{name: "bank-" + l + ".xhtml", id: "bank" + l, page: true, data: page("Bank "+l, s.String())})
		toc = append(toc, entry)
	}
	add(file{name: "style.css", mime: "text/css", id: "css", data: []byte(css)})
	return pack(title, p.Name, at, files, toc)
}

func (p Project) samples() int {
	n := 0
	for _, pd := range p.Pads {
		if pd.Sample {
			n++
		}
	}
	return n
}

func page(title, body string) []byte {
	return []byte(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en" xml:lang="en">
<head><meta charset="utf-8"/><title>` + esc(title) + `</title><link rel="stylesheet" type="text/css" href="style.css"/></head>
<body>` + body + `</body></html>`)
}

// pack zips the book: the mimetype first and stored, then the container, the package, the tables of contents and the files.
func pack(title, project string, at time.Time, files []file, toc []tocEntry) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	w.Write([]byte("application/epub+zip"))
	put := func(name string, b []byte) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	h := sha1.Sum([]byte(project + at.String()))
	uid := fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
	if err := put("META-INF/container.xml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)); err != nil {
		return nil, err
	}
	var man, spine strings.Builder
	for _, f := range files {
		mime := f.mime
		if f.page {
			mime = "application/xhtml+xml"
			fmt.Fprintf(&spine, `<itemref idref="%s"/>`, f.id)
		}
		fmt.Fprintf(&man, `<item id="%s" href="%s" media-type="%s"/>`, f.id, f.name, mime)
		if err := put("OEBPS/"+f.name, f.data); err != nil {
			return nil, err
		}
	}
	opf := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid" xml:lang="en">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="uid">%s</dc:identifier><dc:title>%s</dc:title><dc:creator>404tui</dc:creator><dc:language>en</dc:language><meta property="dcterms:modified">%s</meta></metadata>
<manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>%s</manifest>
<spine toc="ncx">%s</spine></package>`, uid, esc(title), at.UTC().Format("2006-01-02T15:04:05Z"), man.String(), spine.String())
	if err := put("OEBPS/content.opf", []byte(opf)); err != nil {
		return nil, err
	}
	var nav, ncx strings.Builder
	order := 0
	var walk func(es []tocEntry)
	walk = func(es []tocEntry) {
		nav.WriteString("<ol>")
		for _, e := range es {
			order++
			fmt.Fprintf(&nav, `<li><a href="%s">%s</a>`, e.href, esc(e.title))
			fmt.Fprintf(&ncx, `<navPoint id="n%d" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="%s"/>`, order, order, esc(e.title), e.href)
			if len(e.kids) > 0 {
				walk(e.kids)
			}
			nav.WriteString("</li>")
			ncx.WriteString("</navPoint>")
		}
		nav.WriteString("</ol>")
	}
	walk(toc)
	if err := put("OEBPS/nav.xhtml", page("Contents", `<nav epub:type="toc" id="toc"><h1>Contents</h1>`+nav.String()+`</nav>`)); err != nil {
		return nil, err
	}
	if err := put("OEBPS/toc.ncx", []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1"><head><meta name="dtb:uid" content="%s"/></head><docTitle><text>%s</text></docTitle><navMap>%s</navMap></ncx>`, uid, esc(title), ncx.String()))); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func esc(s string) string { return html.EscapeString(s) }

func bpm(v float64) string {
	if v == float64(int(v)) {
		return fmt.Sprint(int(v))
	}
	return fmt.Sprintf("%.2f", v)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
