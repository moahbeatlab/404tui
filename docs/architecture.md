# How the program is put together

One program, several systems. The features are written once; each system is an implementation around them.

    cmd/sp404            the program for a computer (Linux, macOS, Windows): flags, the system's pieces, then platform/terminal
    cmd/sp404h           the program for the handheld (R36S): then platform/handheld
    platform/terminal    a terminal: bubbletea, the keyboard as the buttons, colours and glyphs of the terminal
    platform/handheld    the R36S: the framebuffer, the gamepad (evdev), the screen as a web page for trying it on a computer
    core                 the app: every screen, every dialog, every feature, and how they are drawn
    perform              the unit's MIDI as a state (clock, tempo, pads hit, controllers)
    lab                  the project in memory: pads, samples, patterns, slicing (also built as WebAssembly for the web page)
    app, device, proto   talking to the unit: files, projects, pads, importing samples
    formats, record, midiin, playback, serialport   file formats and the pieces of the system

## core

`core.App` is the whole program minus the system. It is driven by `Dispatch(Event)` (a button down or up) and `Tick()`, and it draws
through `Gfx` (gfxapi.go): the screens say what they want (a filled block of cells, text, a bar, a waveform, a frame around a
dialog, the selected row) and a backend decides how it looks. `App.Render()` is the pixel backend (the handheld, the web page,
screenshots); `App.RenderCells()` is the text backend (the terminal, in the style of the terminal program). A screen is laid out in
cells for the size set with `core.SetSize(cols, rows)`.

## What a platform hands over

`core.Platform` is what differs between systems:

| Field | What |
| --- | --- |
| `Name` | "linux", "macos", "windows", "handheld" |
| `Audio` | opens the sound output (in the background) |
| `Recorder` | records the unit's audio (`record.Backend`); nil when the system cannot. Windows may need another device name or tool than Linux: it is the platform's to say |
| `MIDI` | listens to the unit's MIDI (`midiin.Source`); nil when the build has none |
| `Connect` | finds a plugged-in unit for UNIT (and live editing) |
| `Keyboard` | there is a computer's keyboard |

A feature whose piece is missing says so (for example "this build cannot record", "no MIDI in this build") and does not fail. So a
feature may be missing from one build; none is written twice.

## Adding a feature

Write it in `core` against `Gfx` (never against pixels or characters), take the system's part from `Platform`, give it a screen or a
menu entry, and test it in `core` (the layout test draws every screen at eight sizes and checks that no text lands on other text).
A new system is a new package under `platform/` that builds a `Platform`, shows `RenderCells()` or `Render()`, and turns its input into
`Event`s.
