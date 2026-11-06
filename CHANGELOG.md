# Changes

## 0.1 (not released yet)

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

- **Tools in the phone page:** `sp404 serve` has a TOOLS page with the pattern tools (every page, setting and value of the terminal, a trial that is kept or thrown away, play on the computer, the unit's projects). → [docs/web.md](docs/web.md)

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
