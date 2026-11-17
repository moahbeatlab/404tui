package handout

import (
	"archive/zip"
	"bytes"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/moahbeatlab/404tui/formats"
	"github.com/moahbeatlab/404tui/lab"
)

func testProject() Project {
	p := Project{Name: "LIVE", Tempo: 124}
	for b := range p.Banks {
		p.Banks[b] = 124
	}
	for i := range p.Pads {
		p.Pads[i].Label = string(rune('A'+i/16)) + string(rune('0'+(i%16+1)/10)) + string(rune('0'+(i%16+1)%10))
	}
	p.Pads[0] = Pad{Label: "A1", Name: "Kick", Sample: true}
	p.Pads[4] = Pad{Label: "A5", Name: "Poly", Sample: true}
	q := formats.PPQ / 4
	p.Patterns = []Pattern{{N: 1, Label: "A1", Bars: 2, Beats: 4, Notes: []lab.Note{
		{Tick: 0, Pad: 0, Vel: 127, Len: q},
		{Tick: uint32(4 * q), Pad: 0, Vel: 80, Len: q},
		{Tick: uint32(14 * q), Pad: 4, Vel: 127, Len: 4 * q, Pitch: int(formats.PitchByte(3))}, // held into bar 2
	}}}
	p.Notes = Notes{Keys: map[string]string{"A": "Am", "A5": "A"}, Patterns: map[string]string{"A1": "Groove"}}
	return p
}

func TestTheCellsOfABar(t *testing.T) {
	p := testProject()
	pt := p.Patterns[0]
	rows := padsOf(pt)
	if len(rows) != 2 || rows[0] != 0 || rows[1] != 4 {
		t.Fatalf("the pads: %v", rows)
	}
	c := barCells(pt, 0, rows)
	if !c[0][0].on || c[0][0].vel != 127 || !c[0][4].on || c[0][4].vel != 80 || c[0][1].on || c[0][1].held {
		t.Fatalf("the kick: %+v", c[0][:5])
	}
	if !c[1][14].on || !c[1][14].pitched || c[1][14].semis != 3 || !c[1][15].held {
		t.Fatalf("the chromatic note and its hold: %+v", c[1][14:])
	}
	c2 := barCells(pt, 1, rows)
	if !c2[1][0].held || !c2[1][1].held || c2[1][2].held {
		t.Fatalf("held into the next bar for two steps: %+v", c2[1][:3])
	}
	img, err := BarPNG(p, pt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := png.Decode(bytes.NewReader(img)); err != nil || m.Bounds().Dx() != gridW {
		t.Fatalf("a picture %d wide: %v", gridW, err)
	}
}

func TestTheBook(t *testing.T) {
	b, err := Build(testProject(), time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if f := z.File[0]; f.Name != "mimetype" || f.Method != zip.Store {
		t.Fatalf("the mimetype first and stored: %s %d", f.Name, f.Method)
	}
	files := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		data, _ := io.ReadAll(r)
		files[f.Name] = string(data)
	}
	for _, name := range []string{"META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/toc.ncx", "OEBPS/glance.xhtml", "OEBPS/bank-A.xhtml", "OEBPS/img/A1-1.png", "OEBPS/img/A1-2.png"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("%s is missing", name)
		}
	}
	g := files["OEBPS/glance.xhtml"]
	if !strings.Contains(g, ">Am<") || !strings.Contains(g, "<b>Poly (A)</b>") || !strings.Contains(g, `href="bank-A.xhtml#A1"`) {
		t.Fatalf("the glance page has the keys and the patterns: %s", g)
	}
	if bank := files["OEBPS/bank-A.xhtml"]; !strings.Contains(bank, "A1 Groove") || !strings.Contains(bank, "bar 2 of 2") {
		t.Fatalf("the bank page: %s", bank)
	}
	if opf := files["OEBPS/content.opf"]; !strings.Contains(opf, `href="img/A1-2.png"`) || !strings.Contains(opf, "2026-11-17T10:00:00Z") {
		t.Fatalf("the package lists the pictures: %s", opf)
	}
}
