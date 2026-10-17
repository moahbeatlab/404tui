package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moahbeatlab/404tui/app"
)

// memoryStore keeps the State in memory, so a smoke test leaves the user's own settings alone.
type memoryStore struct{ st State }

func (s *memoryStore) Load() State   { return s.st }
func (s *memoryStore) Save(st State) { s.st = st }

// Smoke runs the UI on a unit with no terminal: it connects, goes through every tab at several
// screen sizes and presses the keys of the views, drawing after each. It says what the screens
// held (the number of draws) or what went wrong, a panic included.
func Smoke(unit app.Unit) (draws int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	var model tea.Model = New(unit, nil, &memoryStore{})
	var run func(cmd tea.Cmd)
	send := func(msg tea.Msg) {
		var cmd tea.Cmd
		model, cmd = model.Update(msg)
		run(cmd)
	}
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		got := make(chan tea.Msg, 1)
		go func() { got <- cmd() }()
		select {
		case msg := <-got:
			switch msg := msg.(type) {
			case nil, tickMsg, slicerTickMsg:
			case tea.BatchMsg:
				for _, c := range msg {
					run(c)
				}
			default:
				send(msg)
			}
		case <-time.After(300 * time.Millisecond):
		}
	}
	draw := func(what string) {
		out := model.View()
		draws++
		if strings.TrimSpace(out) == "" && err == nil {
			err = fmt.Errorf("the screen is empty %s", what)
		}
	}
	run(model.Init())
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 60}} {
		send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, tab := range "1234567" {
			what := fmt.Sprintf("on tab %c at %dx%d", tab, size[0], size[1])
			send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tab}})
			draw(what)
			// Move about in the view and leave whatever it opened.
			for _, k := range []tea.KeyMsg{
				{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyUp},
				{Type: tea.KeyTab}, {Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyEsc},
			} {
				send(k)
				draw(what)
			}
			send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			draw(what + " with the help")
			send(tea.KeyMsg{Type: tea.KeyEsc})
		}
	}
	return draws, err
}
