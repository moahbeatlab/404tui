---
name: feedback-404tui-commit-push
description: "404tui: user said to always commit and push after each change (standing, as of 2026-10-16)"
metadata:
  node_type: memory
  type: feedback
  originSessionId: a488cafa-9925-476e-9f4a-80f277b2b422
  modified: 2026-10-16T05:31:55.577Z
---

For 404tui, commit and push after each finished change without asking again.

**Why:** On 2026-10-16 the user said "commit and push.. always commit and push for now.." after being asked each round. Supersedes the earlier "push only when asked" in [[project_404tui_slicer_roadmap]].
**How to apply:** Once tests pass, commit (Co-Authored-By trailer, no Claude-Session URL, see [[feedback-commit-messages]]) and push the current branch (`ranger` now) in the same turn. "For now" — drop it if the user says otherwise.
