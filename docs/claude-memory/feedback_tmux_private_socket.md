---
name: feedback-tmux-private-socket
description: "Never run `tmux kill-server`; use a private socket (`tmux -L shots`) for TUI screenshots"
metadata:
  node_type: memory
  type: feedback
  originSessionId: a488cafa-9925-476e-9f4a-80f277b2b422
  modified: 2026-10-15T16:27:30.729Z
---

When capturing a TUI (404tui etc.) with tmux, use a private socket: `tmux -L shots new-session -d -s ui -x 140 -y 45 "./sp404 -sim"`, `tmux -L shots capture-pane -t ui -p`, and clean up with `tmux -L shots kill-server`.

**Why:** On 2026-10-15 a plain `tmux kill-server` killed the tmux server that hosts the Claude Code session itself (exit 137), ending the session mid-task; the user had to ask where we left off.
**How to apply:** Never call bare `tmux kill-server` or `tmux kill-session` on the default socket except for a session I created by name. See [[project_404tui_slicer_roadmap]], [[agent-browser-demo-capture]].
