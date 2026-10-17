---
name: project_404tui_perform
description: "404tui `perform` branch (live performance + recording): what is built, how it works, what to test on the Mac, open ideas"
metadata:
  type: project
---

404tui (~/projects/404tui). **UPDATE 2026-10-17: the user worked on another machine and committed to `main`;
`main` (0ce576e) now contains everything from `perform` plus 28 newer commits (see "Changed on main" below). Work
on `main` from now on.** Original note: experimental branch `perform` (e3c36f0 = its tip, an ancestor of main). Latest at time of writing: `a5cda92` (recording level meter) plus the docs/memory
commit after it. User: music producer @moahbeatlab, works on a Mac arm64, SP-404MKII is MIDI clock master,
Faderfox EC4 feeds the SP (control channel 16 by default, notes 60-63 / CC >= 64).

## Built on `perform` (all in README too)
- **midiin**: Event has `At`, `Raw`; kinds ProgramChange/Pressure/PitchBend/Sysex. Real driver
  `midiin/rtmidi.go` (tag `midi`) uses `ListenConfig{TimeCode:true, SysEx:true}` (RtMidi drops clock otherwise).
- **perflog**: JSONL event log beside the WAV (`name.events.jsonl`); first line `{"k":"start","version":1,...}`,
  `t` = seconds from recording start; kinds on/off, cc, program, sysex, beat, start/stop/continue, select/sound,
  pattern, fx. Opened on Ctrl-R when MIDI input is open.
- **tui/monitor.go** MIDI monitor (Config tab block, `c` clears); `-v` log covers MIDI (`-trace` raw bytes).
- **Perform tab (7)**: >=110 cols = pad grid right 70%, left 30% sidebar (`tui/perfside.go`): REC LEVEL, BANK/BPM,
  clock, 4 big beat blocks, cue NOW/NEXT/IN/OUT, PATTERNS (max 4), FX per bus with tall faders, CONTROLS, LAST,
  key hints. Narrower = stacked layout. Pad names whole, plain bold, `wrapName` (user dislikes the pixel font).
- **Ticker**, **pattern player** (`p`, playhead from clock, `s` = pattern starts now), pattern view `v`.
- **FX learn**: `l`/`L` bus 1/2 on/off message, `k`/`K` up to 3 knob CCs, Esc cancels, Backspace forgets.
- **Mutes** inferred (`unitMuted`). **Pattern detection** (`tui/detect.go`): the SP sends every note its running
  pattern plays over MIDI, so while clock runs those notes are NOT pad-code hits; they fill `mon.played` and pick NOW.
  Consequence: pad codes (1 3 3 7) by hand don't work while the sequencer runs; use control channel notes.
- **Recording level meter** (`record/level.go`, `tui/level.go`): peak of last 100 ms of the growing WAV, hold 1.5 s,
  CLIP at >= -0.3 dBFS held 5 s, header badge + Perform bars. UNTESTED against a real ffmpeg-written file.

## Changed on main after `perform` (done by the user on another machine, with Claude; summary of git log)
- Perform: sidebar as panels (clock, bus panels with C1-C3 faders in the bus colour, controls, keys); split went
  50:50 then 40:60 (panels left 40%, pads right 60%); cue / patterns / last-MIDI panels REMOVED; no glow on empty pads.
- FX now from the unit's own MIDI: CC 83 = effect name (`tui/fxnames.go`), CC 19 = on/off, knobs always CC 16-18 on
  channel 1 (bus 1) / 2 (bus 2); no knob learning any more (`k`/`K` gone; FxKnobs state removed).
- Always-on level meter listening to the MIDI input's audio (`record/meter.go`, `tui/level.go`), not only while recording.
- Sequencer: sub-steps like the unit's SUBSTEP (`i` splits 2-4, `e` substep mode, h/l parts, x switch); solo only counts
  in a pattern that has a soloed pad.
- New pattern dialog: groove templates (+14 genres in `formats/grooves.go`), list in columns, 2-bar preview,
  "All pads held"; grid spreads spare cells evenly. Expanded-track "roll" drawing tried then reverted.
- Ctrl-F full screen (header/tabs away); GitHub Actions CI + release builds on `v*` tag; header link state top right.
- 2026-10-17 later: Perform redraw cached (`viewCache`, every 6th clock tick) fixed the slow View; `sp404 check`
  + `make check` + `tui.Smoke` added; CI runs tests + check on Linux/macOS/Windows (Windows path fixes in `app`);
  docs/ has one page per tab; double-click start (Windows holds console on error, macOS `packaging/404tui.command`);
  patterns re-read when the unit says the project changed; release workflow dry run OK on all 5 builds;
  release tag format `v0.1-alpha` (a `-` makes it a prerelease).
- (Fixed) Earlier on 2026-10-17: `go vet` clean, but `TestRawMIDIBytesDriveARunningProgram` FAILS on main (clock
  counted 169 of 216 ticks in the 3 s sleep, live mode not yet off). Cause: Perform View at 170 cols costs ~15 ms
  (lipgloss Render / ansi.StringWidth in perfSplitBody, grid, tile) and runs once per MIDI message, so the loop falls
  behind (~50 clock msgs/s at 125 BPM = most of a core). Fix options: make the test wait until the ticks arrive;
  and/or make Perform cheaper (cache tiles/panels between ticks, render only when something visible changed).

## To test on the Mac
`git pull; git checkout perform; make macos && ./dist/sp404-darwin-arm64 -v` (RtMidi VLA warning harmless).
Log: `~/Library/Caches/404tui/debug.log`. Check: FX learning (switch an effect on the unit, watch Config monitor),
knob learning, pattern detection, mute inference, level meter latency with real ffmpeg/sox, layout on their terminal.
CC 16/17/18 sweeps on ch 1/2 in their log: source unknown (EC4 vs unit) so not merged across channels.

## Open / ideas (not built)
- Player view (`p`) and patterns view (`v`) may still use the old full-width layout (unchecked on main).
- Replay a recorded performance from `.events.jsonl`, then offline overlay-video render (frames piped to ffmpeg
  with the WAV; recommended over screen-recording a terminal). Calibration offset for log time zero vs audio start.
- Computer-side player synced to MIDI clock; live OBS browser-source overlay; pick patterns from sidebar list directly.
- Parked: set list with per-bank notes file, pad colours only in 404tui, app memory indicator, pattern chain
  (`PTN/PATTERNCHAIN_00.CHN`), Sparky key param, native audio capture.
- Known: ranger Files mouse clicks not adapted; header tabs may jump during CONNECTED banner on borderline widths.

**Why:** user wants to focus on the device while performing; the app only gives cues/recording/overlay data.
**How to apply:** research first before building uncertain live features; every mode shows its keys on screen;
commit AND push after each change ([[feedback_404tui_commit_push]]); no Claude-Session URL ([[feedback-commit-messages]]);
see [[project_404tui_slicer_roadmap]] for the earlier slicer work.

## State on 2026-10-17 evening (main, v0.1-alpha released, tag replaced after MIDI on all Windows builds)
- Perform is mouse/touch first: big buttons (`tui/buttons.go`, `rowBuilder`, hit boxes in `viewCache.hit`), VIEW panel
  (record, full screen, focus = the old isolate), MOTION panel (length 1/2/3/4/8 bars; per bus four motion slots that
  record C1-C3 from a bar of the unit's clock and loop them back as MIDI CC; clear; sparkline on a slot).
  Scenes (A/B snapshots + glide, `tui/scenes.go`) are built and tested but switched off (`scenesOn=false`).
  Player view, patterns view, controls panel, keys panel are gone. One click mutes a pad (computer only), double tap focus.
- MIDI output exists on all platforms (`midiin.Outputs`); Windows MIDI is pure Go winmm (no cgo).
- Faderfox EC4 map for a group `404tui`: pushes = notes 60-75 on channel 16 (`tui/ec4.go`, `sp404 midimap`, docs/ec4.md).
- `sp404 check` self-test; docs/ has one page per tab; CI green on Linux/macOS/Windows; release via tag `v*` (workflow).
- Ideas offered, not built: LED feedback to the EC4, encoder turns as macro depth, save motions with the project,
  overdub, glide between motion slots. Unverified on hardware: whether the unit follows CC sent by the app.
- Gotchas: keep go build cache small (disk filled once: `go clean -cache`); test chains must not commit after a failing test.
