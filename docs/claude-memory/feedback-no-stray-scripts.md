---
name: feedback-no-stray-scripts
description: "don't write standalone .js/.py files to the project dir for one-off API calls — pipe a heredoc into the interpreter instead"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: c20634a6-1523-48cf-8705-529d4a6c63a3
  modified: 2026-10-07T14:31:31.731Z
---

For a one-off scripted action (e.g. a single YouTube API update via [[yt-updater-mcp]]'s googleapis client), don't create a named file in the project directory. Pipe the script into the interpreter directly (`node <<'EOF' ... EOF`) so nothing is left on disk.

**Why:** user pushed back — "why did you create that js file? i dont need that." A file like `update-doomos-looper.js` sitting in `~/projects/ai-tools/yt-updater/` looks like a permanent addition to the project, not a transient action, and has to be noticed and deleted later.

**How to apply:** default to inline execution (heredoc, `node -e`, `python -c`) for single-use scripted actions anywhere in this user's projects — not just yt-updater. Only write a real file when the script is meant to be reused (a new CLI tool, a repeatable task) or the user asks for one.
