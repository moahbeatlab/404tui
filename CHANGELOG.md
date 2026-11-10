# Changes

## Unreleased

- Terminal cast: Ctrl-T (or F10) records the keys and the screen of the terminal program (and the unit's audio, unless Config says no) as a recording of its own, with no MIDI or pattern events; Ctrl-R stays the music recording. `sp404 replay` shows such a take as the terminal with the keys as caps. The title bar shows CAST while it runs.
- The replay page stretches the terminal to the full width of the page.

## 0.1.3 (2026-11-09)

The replay page gets a new look.

- The replay page has a new look: the control bar is two grouped rows (transport, zoom, speed and title; pattern and cuts, then Add and View buttons), the sidebar is a tidy form (label left, control right, nothing runs off the edge), with one set of buttons, fields, sliders and popups.
- The tool buttons (templates, camera, captions, chapters, view change, bus change, view modes, style, full screen) moved from the bottom bar to a rail at the right edge; the bottom bar is one row.
- Captions: every row the full width, the text in a box of three lines.
- The bus controls are vertical bars instead of knobs; the tempo graph under the level is gone (the tempo is in the header).

## 0.1.2 (2026-11-09)

The web page.

- On a wide window the sections are a bar at the bottom (Perform, Patterns, Tools), as on a phone.
- A popup with a progress bar while the tools load in the demo (download in MB, preparing, starting), with an error after a minute.
- The tools page grows to 1320 px on a wide window (it was held to 860).
- The arp and MIDI views show only what says something under the title (a direction, more than one octave, the rate, a swing).

## 0.1.1 (2026-11-08)

Replay and render.

- **Split view** for the replay (the default; the normal view is one click or `v` away), wide and vertical: the camera, a mode view and an area for the captions, which is there only while a caption is on. Layouts: video + mode + captions, side by side, over, and **video + sidebar** (buses, level, events). Modes: pads, chromatic keys, **arp** (rate, gate, octaves, swing; step grid and ring looks), **MIDI** (the notes the unit sends out) and **buses + level**. View change marks on the timeline, with a fade between views.
- **Camera:** choose the video in the computer's file dialog; sync in three steps (a moment of the take, the frame that shows it, set) with jump buttons and a live time line; the anchor can be the playhead, a pattern start or a pad hit.
- The REC mark on the video is off by default (an option in the style tab); no empty caption panel between captions, smaller caption text.
- **Render:** camera pictures are sent as bitmaps and a frame that gets no answer from the browser is tried again; an error names the frame, its time and whether it had a camera picture. `-q "title=..."` changes the title for one render.
- Docs: [docs/replay.md](docs/replay.md).

## 0.1 (2026-11-06)

Since `v0.1-alpha`.

### One program on every system

- `sp404` is now **one set of screens** on Linux, macOS and Windows and on the R36S handheld (PADS, SEQ, PROJECTS, FILES, SLICE, CONFIG, and on the handheld SAVE, PERF, LOOP), drawn for a terminal window, with vim keys and a panel that
  always lists the keys that work now. The earlier tabs (`sp404 classic`) have been removed as a command; the phone page (`sp404 serve`) stays. → [docs/ui.md](docs/ui.md), [docs/tools.md](docs/tools.md)
- The program is split into `core` (screens, menus, features) and platforms (`platform/terminal`, `platform/handheld`). → [docs/architecture.md](docs/architecture.md)
- **Handheld version** for the R36S (`sp404h`), with its own launcher, an emulator for trying it without the device, and screenshots from a script. → [docs/handheld.md](docs/handheld.md)

### Pads, files and the unit

- FILES: a ranger view on wide windows, search, favourites, places (the computer, the unit's card and storage), copy, cut, paste, rename, delete, marks, and tags with an index of the unit's files. A kit's numbered files go on the pad of that number.
- Online: the unit's 16 projects as tiles; load a project into the editor and **edit it live** (what changes on the unit comes into the editor); back up, restore, send, rename, make current. A level meter and a spectrum while a unit is there.
- Offline: a folder of projects on the computer, opened and written like the unit's.
- Pads: rename, find, set up, switches and buses of a bank, export a bank, restore pads, bus by pad name.

### Sequencer and slicer

- The pattern as step boxes with a bar as tall as the velocity, two bars side by side, notes with velocity, length, timing and **pitch** (the pitch byte of a note is decoded).
- **Pattern tools:** Fill, Euclid, Echo, Cycle, Ratchet, Evolve, Accents and ghosts, LFO, ENV, Chance, Randomize velocity, Shape, Nudge, Pitch, Dispatch, Copy, **Swap with a pad**, Invert; each shows its trial in the pattern and one undo step keeps it.
  Pattern templates from House to Footwork, per-bank tempo, copy and move patterns, undo and redo everywhere.
- **Slicer:** cut by hand, at the onsets, equal parts, random places, on the grid; reorder, mangle, roll; slices to pads with a pattern under them. Switching bank or pattern while a pattern plays leaves the loop alone.

### The phone page: Tools

- **`sp404 serve` has a TOOLS tab** (PADS, PATTERNS, MOTION, TOOLS, STAGE) with the tools of the program, every page, setting and value of the terminal, on a phone and on a laptop. It works on **the project that is loaded on the unit** (it follows the unit when the project is changed there). → [docs/web.md](docs/web.md)
- **Pattern:** the tracks as tiles to choose from, the 16 patterns of the bank, the tools as a sheet that slides up (every tool a tile, a trial that is kept, as one step and back one menu, or discarded), play on the computer, undo and redo.
- **Live or copy:** edited live by default (a kept tool is written to the unit, which then reads the project again; it waits while the unit plays; RELOAD UNIT), or a copy with SEND TO UNIT.
- **Slicer:** a real waveform, touch and slide for the cursor, cut, join, play the slice or the whole order in a loop (the slice that sounds is lit; changes are heard at once), shuffle, marks for reversed, rolled, pitched and chance slices, all settings and tools of the terminal, the slicing kept when you go to another mode.
- **Pad tools** and the dialogs a menu opens (a pad or pattern to choose, a text to type) in the page.
- **One bank** for the whole page (Pads, Patterns, Tools). The Looper tab, with its Slicer, is gone from the page.

### Recording and replay

- `Ctrl-R` records the unit's audio with an **event log** (pads, knobs, clock, status, mutes, isolation, the project as it was).
- **`sp404 replay`**: a page that plays a recording as a picture of the unit, and a video editor: patterns set by hand, captions (with spotlight on a pad, a bus, the pattern...; as a card, words popping in, or typed; `*accent*` words),
  chapters with the list for YouTube and optional chapter cards, **cuts**, bus changes, undo and redo, a timeline that zooms, a sidebar.
- **Camera:** a video of you playing, matched by eye, with scenes, transitions (glide, cross-fade, pop, slide, cut) and layout presets (left, right, corner, top, bottom, full).
- **Look:** palettes, accent, a background picture (fill modes, position, zoom, blur, a slow random drift and breathing), panel opacity, logo, handle, subtitle, hit reactions. **Templates** keep a setup for the next take.
- **`sp404 replay render`**: the video (MP4), wide or vertical (with safe margins for Instagram and YouTube), a part of the take, a quick draft without the camera. → [docs/replay.md](docs/replay.md)
- **`sp404 probe`**: what the unit does when it is told something (a pad press, a Program Change) and, with `probe watch`, what it says when you do something. → [docs/unit-messages.md](docs/unit-messages.md)

### Documentation

- A new README, and pages for the replay, the unit's messages, the handheld and installing; pictures of the screens of the program, the handheld and the replay editor.
