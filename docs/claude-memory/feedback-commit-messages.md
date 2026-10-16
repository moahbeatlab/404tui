---
name: feedback-commit-messages
description: How to format git commit messages — what to include and exclude
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 69b6d820-84c1-4c90-8839-133cb70ffe53
---

Do NOT include the Claude Code session URL (e.g. `Claude-Session: https://claude.ai/code/session_...`) in commit messages.

**Why:** User finds it noisy and unnecessary.

**How to apply:** The `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer is fine to keep, but drop the `Claude-Session:` line entirely from every commit.
