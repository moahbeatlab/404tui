---
name: project_404tui_slicer_roadmap
description: "404tui slicer/pattern direction: decisions made, what shipped, what is next, \"tools\" idea parked"
metadata:
  node_type: memory
  type: project
  originSessionId: a53933a9-af1f-4feb-9d93-1ade2db85047
  modified: 2026-10-15T16:27:34.143Z
---

404tui (~/projects/404tui, branch `slicer`): Go TUI for SP-404MKII, works offline with `-sim` / `-offline`.

Decisions (2026-10-15): slice envelope (attack/release) is baked into the audio; playback is
from the computer's audio, no dependency on the unit; user wants to prepare samples offline;
in the slicer they want to pick a pattern that plays in sync (computer-side pattern player,
one clock/one audio stream).

Shipped: grid ticks, order-lane highlight, number thinning, `X` fix, per-slice envelope (keys 1-4),
mark slices `s`/`y` + `B` to pads, real-sample e2e + fuzz tests (SLICER_SAMPLE / SLICER_SAMPLES
env, skipped without them).

Next: pattern player on the computer (renders pattern pads at a tempo), then "pick a pattern" in
the slicer, bar-synced order changes. Open: one-by-one target pad choice for `B`.

Also shipped later (same day): trig conditions per note (`c`/`C`, resample the track to a pad, long
samples need the pattern doubled), select mode `V` for batch edits (keys must be shown whenever a
mode is active), double/half pattern tools, clear-track `D`, Tab collapses a track in the editor,
slicer fills the screen. Parked: "negative" sample (plays only the conditioned hits).
UI layout review (2026-10-15): screenshots taken, three directions offered (A tighten, B workspace
with right-rail inspector, C tabbed tools); user is thinking, no change wanted yet.
SparkyMK2 gap list (not built, user said "nah"): Truncate/Normalize keys (PadOp exists), unit-side
pattern bounce/MULTIPAD, Screens tab (ReadScreen/WriteScreen exist), chop-point/start/end editing
on pads, SMF pattern export, bank protect, working-mode banner. Upstream last commit 2026-10-04.

**Why:** the user's flow is offline sample prep, then hearing it against a pattern.
**How to apply:** don't build the "tools" restructure (shared transport + tool frame) until the
user comes back to it; they said "park it" and want to think. Build the transport/pattern player
first, tool frame later. Commit messages: no Claude-Session URL ([[feedback-commit-messages]]).
Push only when asked (earlier "push when ready" covered the first two commits).

Branch `ranger` (2026-10-16, pushed; spun off `slicer`): Files as three ranger-style columns (`w` toggles
the old tree; default is ranger), sections as tabs (1 Pads, 2 Files, 3 Projects, 4 Slicer, 5 Sequencer =
the patterns). Tabs sit in the header when it has room (~150+ cols), else on the row under it. Slicer tab
= landing page; `a` asks pad (`p`) or new sample from Files (`f`), Z slices; Slicer tab is lit whenever a
slicer is open. Sequencer `a`: sample (file dialog onto a pad) or pattern (chooser, opens the editor).
Slice edges in the slicer waveform are hairlines `▏▕`. Key menu under the ranger preview has a divider and
indent. Rename/text prompts have cursor editing (Ctrl-A/E etc.). Folder-use markers (`◆`, `W` scan).
Open: mouse clicks in ranger Files not adapted; layout redesign (A/B/C) parked; tag-index storage format
(todo.md); header tabs may jump between header/row during the CONNECTED banner on borderline widths.
