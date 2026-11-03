# The program: `sp404`

`sp404` with no command opens the same program on every system: the screens, the layout, the menus and the features are those of the
handheld version (R36S), drawn for the window of a terminal on Linux, macOS and Windows. The earlier terminal program with its tabs
(Pads, Files, Projects, Slicer, Sequencer, Config, Perform, Looper) is still there as `sp404 classic`, while the features that only it
had are being moved into this one.

    sp404                                  # starts in ~/404tui/samples, writes below ~/404tui/out
    sp404 -project "path/to/PROJECT_01"    # open a project folder (one with a PADCONF.BIN in it)
    sp404 -samples ~/Music/kits -out ~/Desktop/card
    sp404 -sim                             # the UNIT screen finds a simulated unit
    sp404 classic                          # the earlier tabs

Flags: `-samples` (where the file browser starts), `-out` (where the card folder is made; settings, autosaves, backups and exports
are kept beside it), `-project`, `-basic-glyphs` (see below), and the usual `-sim`, `-port`, `-audio`, `-v`.

![The pads in a 120x40 terminal](img/lab-pads.png)

## One layout on every size

The screen is a grid of characters, and every screen is laid out on it:

- The window is measured when the program starts and again whenever it changes.
- The right **30%** (at least 24 columns) is the panel that is always there: the screens with the one you are on lit, the tempo and
  whether the loop plays, what is under the cursor, and the keys that work right now. The rest is the screen itself.
- 80 columns by 30 rows is the smallest window (it is the handheld's screen). In a smaller one the program says so and waits.

![80x30](img/lab-80x30.png)

![200x56](img/lab-200x56.png)

The look follows the earlier terminal program: flat tiles, one amber accent, rounded dialogs, waveforms in braille dots, bars in
eighths of a character. 24-bit colour when the terminal has it (`COLORTERM=truecolor`, Windows Terminal, iTerm2, most current
terminals), else the 256-colour palette. A terminal font with few glyphs (the old Windows console) gets only blocks: `-basic-glyphs`
or `SP404_GLYPHS=basic`. The colour theme (Hardware, Ocean, Paper) is set on CONFIG.

On the handheld the same screens are drawn as pixels (640x480, 80x30 cells of 8x16 pixels): the layout is the same, only the drawing
differs.

## The keys

The screens are made for a D-pad, A B X Y, L1 R1 L2 R2 and a second shift (Select). On a keyboard:

| Key | Button |
| --- | --- |
| `h j k l` or the arrows | D-pad left, down, up, right (as in vim) |
| `a` or `Enter` | A: do it, open, put a note, cut |
| `b`, `Esc` or `Backspace` | B: back, stop, join, the note settings in SEQ |
| `x` or `Space` | X: listen to what is under the cursor, play |
| `y` | Y is held for the **next** key: `y` then `l` is Y+Right (the next screen). `y` again lets go |
| `s` or `Tab` | Select is held for the next key: `s` then `b` opens the tools of the slicer or the sequencer |
| `e` | holds A, for editing a note with the arrows in SEQ (velocity, length, `[` `]` to move it); `e` again lets go |
| `[` `]`, `PgUp` `PgDn`, `Ctrl-U` `Ctrl-D` | L1, R1: pages, banks, tabs, bars |
| `,` `.` | L2, R2 |
| `u`, `R`, `Ctrl-Y` | undo, redo (every change to the project, by snapshot) |
| `Ctrl-R` | record the unit's audio (also `s` then `x`) |
| `/` | search, in the file browser; typed letters go into the search, `Enter` ends it |
| `?` | the help page for the keyboard |
| `q`, `Ctrl-C` | quit (twice when the project is not saved) |

Text asked for (a name, a folder, a list of words) is typed on the keyboard; on the handheld on an on-screen keyboard with the D-pad.
The side panel always lists the keys that work now, and in the terminal it names the keys of the keyboard (`hjkl`, `[ ]`, `y h`,
`s b`), not the buttons.

## The screens

Nine screens side by side; `y` `h` / `y` `l` (Y+Left / Y+Right) move between them.

| Screen | What it is for |
| --- | --- |
| PADS | the 4x4 pads of the bank with their waveforms. `a` opens the pad panel: faders for the settings (SOUND, SHAPE) and the TOOLS: slice, chords and arps, **arp from one sample** (key, scale, chord stacks, octaves, a keyboard), reverse, normalize, pitch, rename, move, copy, clear; for the bank: find a pad, set up pads, switches and buses of every pad, export the bank to a folder and restore pads from one |
| SEQ | the pattern as the unit's step grid. `a` puts a note, `b` the note panel (velocity, length, timing), `s` `b` the pattern tools (euclid, chance, randomize, echo, ratchet, LFO, envelope, fill, timing, copy and move to any slot, use the pads of another bank...), `y` `x` adds a track |
| FILES | this computer and, with a unit plugged in, its SD card and its storage (places, `y` `x`). `/` searches, favourites, preview, `a` puts a sample on a pad; `s` `a` marks and `s` `b` is the menu: copy, cut, paste (between the computer and the card), new folder, rename, delete |
| SLICE | cut by hand, at the onsets, equal parts, random places, on the grid at the bank's BPM; settings (`y` `b`) and tools (`s` `b`): mangle, result to a pad, slices to pads |
| SAVE | writes the project as a folder for the card, with an autosave |
| UNIT | the unit's 16 projects. `a` is the menu of a project: send the project of the editor, **edit it live**, load it into the editor, back it up, restore a backup, rename, make it the current one, erase |
| CONFIG | the buses by pad name (words, colours, set the buses of a bank), the recorder, the pad codes of the unit, MIDI, the colour theme |
| PERF | the stage view from the unit's MIDI: the clock and tempo, the pads that are hit, the knobs of both buses and the other controllers, the level of the sound, the pattern that plays and the next |
| LOOP | a bank of loops that play together: the pattern of the bank, delete a loop, remix a loop |

![The slicer](img/lab-slicer.png)

![The note panel in the sequencer](img/lab-note-panel.png)

Every question "which pad?" is the same dialog: the banks across the top, 16 tiles with the names, `[` `]` for the bank, `a` to choose.

## Recording a set

`Ctrl-R` (or Select then X) records the unit's audio, from any screen, to a WAV in the folder CONFIG names, with `ffmpeg` (or `sox`):
24-bit, 48 kHz, stereo, from the input with `404` in its name (the unit's USB audio). The red `REC` in the title bar shows the time. PERF
shows the level of what is recorded and a clip light. A system that cannot record says so instead of failing; the handheld has no
recorder yet.

## The unit: whole projects, and live

With a unit plugged in (UNIT finds it by itself) there are two ways to work with it, and the same screens do both:

- **Whole projects.** Load a project of the unit into the editor, change it, send it back; or back one up to the computer and restore it.
- **Live.** `a` on a project, *Edit it live*: the project is loaded, the unit is switched to it, and from then on what is changed in
  the editor goes to the unit as it is changed: names, settings, samples (imported again when a pad's sample changes), clears, patterns.
  Undo goes to the unit too. `LIVE n` in the title bar shows it, with how many changes are still on their way.

The unit is tried against its simulator (`sp404 -sim`); it has not been tried against a real SP-404MKII yet.

## What is not in this program yet

The earlier tabs (`sp404 classic`) still have: the motion, punch, LFO and morph of PERF, the Faderfox EC4 map, the web page for a phone, the tags
and the index of the unit's files, the columns view of the file browser, the conditions of the notes in the sequencer, the substep mode
and the note selection of its editor, a few slicer extras (tap, the order lane, several samples, the pattern under the slices),
checking and finding the origin of a pad's sample.

## Tests

`platform/terminal/e2e/run.sh` builds the program for Linux, starts it in a terminal (tmux) in an Ubuntu container against a copy of a
real project, and plays the keys: the splash, the pads, the panel's width at five sizes, a window that is too small, the vim keys, the pad
panel, the arp, the slicer and its menus, the sequencer, the note panel, the pad dialog, a typed search, the loop, perform, config and
unit screens, a recording without a recorder, saving (the files written are compared byte for byte with the project that was opened)
and quitting. Every step is kept as text and as a PNG of what the terminal showed (`platform/terminal/e2e/out/`).
`go test ./core ./platform/...` has the same keys, the layout at eight sizes, the unit (against the simulator: load, back up, rename, erase,
live edits, the card of the unit and file operations), MIDI, recording and the looper without a container.
