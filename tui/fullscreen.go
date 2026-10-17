package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// toggleFullScreen is Ctrl-F, on every system. A program in a terminal cannot make its window
// full screen in general (that is the terminal's own key: F11 on Windows and most Linux
// terminals, Ctrl-Cmd-F on a Mac), so what it does is its own: the header and the tabs go and
// the body gets their rows. xterm and what behaves like it is asked for its window too.
func (m Model) toggleFullScreen() (tea.Model, tea.Cmd) {
	m.full = !m.full
	if m.full {
		m.message = "full screen: Ctrl-F again to bring back the header and the tabs"
	} else {
		m.message = ""
	}
	on := m.full
	return m, func() tea.Msg {
		if strings.HasPrefix(os.Getenv("TERM"), "xterm") && os.Getenv("TMUX") == "" {
			seq := "\x1b[10;0t"
			if on {
				seq = "\x1b[10;1t"
			}
			os.Stdout.WriteString(seq)
		}
		return nil
	}
}
