---
name: project_404tui_perform
description: "404tui `perform` branch (live performance + recording): what is built, how it works, what to test on the Mac, open ideas"
metadata:
  type: project
---

404tui (~/projects/404tui), **experimental branch `perform`** (from main, pushed, NOT merged; merge only
when the user asks). Latest at time of writing: `a5cda92` (recording level meter) plus the docs/memory
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

## To test on the Mac
`git pull; git checkout perform; make macos && ./dist/sp404-darwin-arm64 -v` (RtMidi VLA warning harmless).
Log: `~/Library/Caches/404tui/debug.log`. Check: FX learning (switch an effect on the unit, watch Config monitor),
knob learning, pattern detection, mute inference, level meter latency with real ffmpeg/sox, layout on their terminal.
CC 16/17/18 sweeps on ch 1/2 in their log: source unknown (EC4 vs unit) so not merged across channels.

## Open / ideas (not built)
- Player view (`p`) and patterns view (`v`) still use the old full-width layout (not split).
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
