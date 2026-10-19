---
name: project_404tui_web_live
description: "404tui web UI + live features (Punch/Morph/LFO/Slicer preview/Patterns/Looper): state at 85c9064, what is UNVERIFIED on the unit, hardware test checklist, working rules"
metadata:
  type: project
---

404tui (~/projects/404tui, `main`, public repo moahbeatlab/404tui, release v0.1-alpha re-cut 2026-10-19 from 8f49b47; later commits not in a release). Go bubbletea TUI + phone web UI (`sp404 serve`, `web/static`). User: live performer, iPhone landscape, wants minimal delay, big touch targets. See [[project_404tui_slicer_roadmap]], [[project_404tui_perform]], [[feedback_404tui_commit_push]], [[feedback_show_keys_for_modes]], [[feedback_tmux_private_socket]], [[feedback-commit-messages]].

## Built since the release (all pushed, tests pass; NONE tried on the unit by me)
- **Punch** (side panel page MOTION | PUNCH | LFO): 4 snapshots of all 4 buses (effect CC83, knobs CC16-18 + 80-82, switch CC19, channel = bus 1-4, per 404jam). Modes HOLD / EDIT (keeps knob changes made on the unit while held) / MORPH (slider or EC4 ch14 encoders 1-4 move 0-127 from the start state to the snapshot; effect/on changes at 64). 6 presets (RISER TAPE ECHO THROW STUTTER TAPE STOP TELEPHONE; values chosen on paper from 404jam param ranges, not by ear). Values panel in EDIT (param names/units from `tui/fxparams.go`, generated from 404jam).
- **Key fact (from the user's debug log cca5e397):** the unit sends NO CC for effect/on state unless something changes on the unit, so the app does not know a bus's effect at start. Chips at the top of Punch show known state (red = unknown); `fxdeclare` tells the app; unit-reported/declared state is kept in State.BusFx across runs. Unknown bus: punch still loads the preset; on release effect switched on goes OFF and the bus stays unknown (a bus is never taken as known from what we sent). An earlier "leave unknown buses alone" rule broke presets and was reverted.
- **LFO** page: 4 LFOs (bus, CTRL 1-6 or effect switch, shape sine/tri/saw/square/random/pump, rate 1/16..8 bars, shift, depth), clock-synced (cycle starts with the bar), swing around the knob's value at start, restore on stop; pauses while a held punch / morph owns its bus; option lists open as a picker sheet (choose on tap, scrollable).
- **Slicer PREVIEW** (web Looper > Slicer): slices stay in memory, loop on the computer's audio output (like the TUI slicer; user routes it to the unit via USB + EXT SOURCE), starts on beat 1, new cuts via NextLoop; PLACE as before. (An earlier version wrote a pad + MIDI note; user did not want that; removed.)
- Earlier (before this note): Patterns page (Program Change, blink/takeover heuristics), queued delete/place on beat 1, live import without Stop, web Slicer + mangle dice, level meter (Linux pure-Go ring), Looper bank/pattern/delete.

## UNVERIFIED on hardware (hardware test checklist; also in repo todo.md)
1. Program Change pattern switching (unit needs PC Rx on, MIDI mode B) and the Patterns-page blink/takeover heuristic.
2. Live import without Stop: does the pattern still gap at beat 1? queued PLACE/DELETE on beat 1.
3. Punch: buses 3/4 really on channels 3/4 with CC 80-82/83/19; preset sounds; restore after release with declared chips; EDIT keeps changes (needs unit to echo knob CCs); MORPH; EC4 group ch14 (CCr2 CC60-63, notes 60-63).
4. LFO: 7-bit step audibility per knob; pump on Isolator LOW (bus 3/4) / a LEVEL knob; clock lock; unit's reaction to dense CC.
5. Slicer preview: routing (computer output -> unit EXT SOURCE), output latency, drift vs MIDI clock, NextLoop swap smoothness.
6. Unit reaction to a pattern file rewritten while it plays (earlier: write succeeded, no audible effect).

## How to test with the unit attached (when the user gives access on that machine)
- `go build ./cmd/sp404 && ./sp404 -v -log debug.log serve` (web URL with ?key=… printed); only one program may hold the serial port. Linux needs the MIDI build (cgo ALSA) for MIDI in/out; log shows "midi: listening/sending to".
- Use a spare project; back up before pad/pattern writes; no destructive steps without asking. I cannot hear audio: use the level meter / record ring, or ask the user. Browser checks: agent-browser (pointerdown dispatch, not click, for most buttons; option lists use click).
- Working rules: commit AND push each finished change; commit messages end with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` and NO `Claude-Session:` URL; modes must show their keys on screen; no stray scripts in the project (heredoc, scratchpad for temp); tmux only via `-L shots`; honest reporting (say what is unverified).
