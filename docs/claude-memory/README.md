# Claude memory for 404tui

Notes copied from Claude Code's memory on the machine this was developed on, so a session on
another machine can pick up where it left off. To use them there, copy these files into that
machine's Claude memory directory for the project (or just ask Claude to read this folder).

- `project_404tui_slicer_roadmap.md` — decisions, what shipped, what is open
- `feedback_404tui_commit_push.md` — commit and push after each change
- `feedback-commit-messages.md` — no Claude-Session URL in commits (the trailer is `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`)
- `feedback_show_keys_for_modes.md` — any active mode must show its keys on screen
- `feedback_tmux_private_socket.md` — never a bare `tmux kill-server`; use `tmux -L shots` for TUI captures
- `feedback-no-stray-scripts.md` — one-off scripts via heredoc, not files in the project

Current branch: `ranger`. Build: `go build -o sp404 ./cmd/sp404`; try without hardware: `./sp404 -sim`.
