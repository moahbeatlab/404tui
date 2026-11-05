# 404tui documentation

The program itself (`sp404`) is described in [ui.md](ui.md), its screens with every menu and setting in [tools.md](tools.md) and its tests in [e2e.md](e2e.md).

| Page | What is in it |
| --- | --- |
| [ui.md](ui.md) | the program: layout, keys, the screens (with pictures), online and offline, recording a set |
| [tools.md](tools.md) | every screen with its keys, menus and settings (written by the program's own test) |
| [replay.md](replay.md) | the replay of a recording and the video editor: captions, chapters, cuts, a camera, a look, templates, `replay render` |
| [unit-messages.md](unit-messages.md) | what the unit tells the program and what it does not; `sp404 probe` |
| [handheld.md](handheld.md) | the R36S version: buttons, installing, what has been tried |
| [install.md](install.md) | checking an installation, building, audio, what to do before you run it on a unit |
| [architecture.md](architecture.md) | how it is put together: core, platforms, packages, the replay |
| [e2e.md](e2e.md) | the end-to-end tests of the computer's program: the use cases |

## The views

One page for each screen of the program, with its picture, what it is for, what you do there and its keys. `?` in the program lists the keys of the view in front.

| Screen | Page |
| --- | --- |
| `1` PADS | [pads.md](pads.md) |
| `2` SEQ | [sequencer.md](sequencer.md) |
| `3` PROJECTS (UNIT on the handheld) | [projects.md](projects.md) |
| `4` FILES | [files.md](files.md) |
| `5` SLICE | [slicer.md](slicer.md) |
| `6` CONFIG | [config.md](config.md) |
| `sp404 replay`: the recording as a picture, the video editor | [replay.md](replay.md) |
| `sp404 serve`: Perform on a phone, and the pattern tools in the page | [web.md](web.md) |

## Earlier pages

Perform ([perform.md](perform.md)), the Faderfox EC4 map ([ec4.md](ec4.md)) and the Looper ([looper.md](looper.md)) were tabs of the earlier terminal program, which is gone; what is left of them is reached through `sp404 serve`.

Everywhere: `1`-`6` go to a screen, `g h` and `g l` the one before and after, `Ctrl-R` records the unit's audio, `?` lists the keys, `q` quits. `sp404 check` checks the installation.
