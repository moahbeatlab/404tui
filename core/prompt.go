package core

import "strings"

// A text prompt: a line of text to type, for a name, a folder, a list of words. On the handheld it is typed on an on-screen
// keyboard with the D-pad; at a computer's keyboard the letters go straight in (Typing, TypeText).

var promptPages = [3][]string{
	{"abcdefghij", "klmnopqrst", "uvwxyz0123", "456789_-. "},
	{"ABCDEFGHIJ", "KLMNOPQRST", "UVWXYZ0123", "456789_-. "},
	{"0123456789", "/\\:~,;+=()", "[]{}!@#$%&", "*?<>^'\"`|!"},
}

var promptPageNames = [3]string{"abc", "ABC", "123 /:"}

type textPrompt struct {
	title, hint string
	buf         string
	r, c, page  int
	onDone      func(string)
	onCancel    func()
}

// askText opens a prompt with text in it already; onDone gets the text when it is finished.
func (a *App) askText(title, hint, text string, onDone func(string)) {
	a.prompt = &textPrompt{title: title, hint: hint, buf: text, onDone: onDone}
}

func (p *textPrompt) rows() []string { return promptPages[p.page] }

func (a *App) promptKey(k Key, repeat int) {
	p := a.prompt
	rows := p.rows()
	switch k {
	case Up:
		p.r = (p.r + len(rows) - 1) % len(rows)
	case Down:
		p.r = (p.r + 1) % len(rows)
	case Left:
		p.c = (p.c + len(rows[0]) - 1) % len(rows[0])
	case Right:
		p.c = (p.c + 1) % len(rows[0])
	case L1:
		p.page = (p.page + 2) % 3
	case R1:
		p.page = (p.page + 1) % 3
	case A:
		p.buf += string(rows[p.r][p.c])
	case B:
		switch {
		case a.yHeld: // Y+B: all of it
			p.buf = ""
		case p.buf == "":
			a.closePrompt(false)
		default:
			p.buf = p.buf[:len(p.buf)-1]
		}
	case X:
		if a.yHeld { // Y+X: leave it
			a.closePrompt(false)
			return
		}
		a.closePrompt(true)
	}
}

func (a *App) closePrompt(ok bool) {
	p := a.prompt
	a.prompt = nil
	if ok && p.onDone != nil {
		p.onDone(strings.TrimSpace(p.buf))
	} else if !ok && p.onCancel != nil {
		p.onCancel()
	}
}

// Typing says whether a text is being typed (the search keyboard or a prompt), for a keyboard that can type into it directly.
func (a *App) Typing() bool { return a.kb != nil || a.prompt != nil }

// TypeText adds typed text to the search or the prompt in front.
func (a *App) TypeText(s string) {
	switch {
	case a.prompt != nil:
		a.prompt.buf += s
	case a.kb != nil && s != "":
		a.searchQ += s
		a.list()
	}
}

// TypeBackspace takes the last character away.
func (a *App) TypeBackspace() {
	switch {
	case a.prompt != nil:
		if p := a.prompt; p.buf != "" {
			p.buf = p.buf[:len(p.buf)-1]
		}
	case a.kb != nil:
		a.kbKey(B)
	}
}

// TypeEnter finishes the text, TypeEscape gives it up.
func (a *App) TypeEnter() {
	switch {
	case a.prompt != nil:
		a.closePrompt(true)
	case a.kb != nil:
		a.kbKey(X)
	}
}

func (a *App) TypeEscape() {
	switch {
	case a.prompt != nil:
		a.closePrompt(false)
	case a.kb != nil:
		a.kbKey(X)
	}
}

func (a *App) drawPrompt(g Gfx) {
	p := a.prompt
	wc, hr := contentCols-2, 14
	x0 := 1
	y0 := Rows - 2 - hr
	g.Fill(x0, y0, wc, hr, cPanel2)
	g.Frame(x0, y0, wc, hr, cAmber)
	g.Text(x0+2, y0+1, fit(strings.ToUpper(p.title), wc-4), cAmber, true)
	g.Fill(x0+2, y0+2, wc-4, 1, cPanel)
	shown := p.buf + "_"
	g.Text(x0+3, y0+2, shown[max(len(shown)-(wc-7), 0):], cText, true)
	const kw = 4 // a key: 4 cells wide, 2 high
	rows := p.rows()
	gx := x0 + (wc-10*kw)/2
	for r, row := range rows {
		for col := 0; col < len(row); col++ {
			x, y := gx+col*kw, y0+4+r*2
			bg, fg := cPanel, cText
			if r == p.r && col == p.c {
				bg, fg = cAmber, cBg
			}
			g.Fill(x, y, kw-1, 1, bg)
			ch := string(row[col])
			if ch == " " {
				ch = "sp"
			}
			g.Text(x+(kw-1-len(ch))/2, y, ch, fg, true)
		}
	}
	page := promptPageNames[p.page]
	hint := tk("L1 R1") + " page " + page + "   A type   B delete   X done   Y+X cancel"
	if p.hint != "" {
		hint = p.hint
	}
	g.Text(x0+2, y0+hr-2, fit(hint, wc-4), cDim, false)
}
