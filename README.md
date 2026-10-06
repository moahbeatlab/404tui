# 404tui

A command-line tool and (in progress) terminal UI for the Roland SP-404MKII, talking to
the unit over its USB serial link: projects, pads, samples and the files on its storage
and SD card.

This is a Go port of [SparkyMK2](https://github.com/TuscanPL/SparkyMK2), whose authors
reverse-engineered the protocol. The wire format follows the specification in that
project's `docs/re/`; comments here that mention `docs/re/` refer to it.

## Status

The protocol, device session and the terminal UI are in place and tested against a
simulated device in this repo. None of it has been **verified against real hardware**
yet. The simulator encodes the protocol as read from SparkyMK2's code and docs, so it
checks sequencing and data handling, not the unit's actual behaviour.

## Usage

    go run ./cmd/sp404                 # the terminal UI (same as `menu`)
    go run ./cmd/sp404 -sim            # the terminal UI against the simulated device

The UI is laid out after the unit itself:

- Navigation is a rail down the left: the four sections, and under them the banks or the
  file sources of the section you are in.
- **Pads** is a 4×4 grid in the unit's own arrangement (pad 1 at the top left), with the
  selected pad's details beside or below it: length, trim and chop points, its settings,
  which patterns play it, and a progress bar while it sounds on the unit. `S` edits its
  settings (or the marked pads') in a dialog, and `,` `.` step its level. The cursor
  follows pads you hit on the unit, and a sounding pad lights up.
- **Connecting:** the UI starts even with no unit, shows a waiting screen, connects by itself
  when you plug it in (the header shows `● CONNECTED` and the port), and goes back to waiting
  and reconnects if the cable is pulled.
- **Bank tempo and volume** sit in a bar above the pad grid (and on Patterns): `< >` step the
  tempo by 1 BPM, `- +` the volume by 5, and `b` opens a dialog to type both.
- **Copying pads:** `c` copies the selected pad (or the marked ones, which keep their layout)
  to any pad in any bank, through the same bank-letter and pad-number picker. The unit has
  no copy command, so the sample is read and written onto the other pad together with its
  settings and chop points. Pads that already hold a sample are replaced only after a second
  Enter.
- **Patterns** shows the 16 pattern slots the same way; Enter opens a step grid, and `b`
  moves a pattern's pads to another bank.
- **Files** browses the SD card, the unit's storage or this computer. The rail shows the
  **target bank**; `[` `]` change it. Enter on an audio file opens a pad picker on that
  bank's next free pad, so Enter, Enter assigns it. Type a bank letter and a pad number
  (`C`, `7`) to go elsewhere. `v` marks several files to fill free pads in order, and Space
  listens first. `Ctrl-A` turns on autoplay (a file plays as the cursor rests on it). `/` searches every audio file in the source by keyword (words narrow it,
  folders count, the first search indexes the source and later ones are instant): Enter
  assigns, Tab shows the file in the tree, Ctrl-L listens.
- **Assigning from Pads:** `a` opens a file dialog for the selected pad, with the file tree,
  file details and Space to listen. It reopens exactly where you left it (source, open
  folders, highlighted file), also after restarting; that and the Files tree position,
  bank and theme are kept in `state.json` in your config folder.
- **Projects** switches and renames projects. `e` exports the project (all its files) to the
  SD card or this computer, and `R` restores a backup folder (the one holding `PADCONF.BIN`)
  over the project: it reads and checks the backup, shows what will be erased, and asks
  before it writes anything.

Keys are vim-style (`h j k l`, `g`/`G`, `[`/`]` for banks, `1`-`4` for sections); `:`
opens a command palette, `?` lists every key, `/` finds by name, `T` changes the colour
theme, and the mouse works for tabs, banks, pads, rows and the wheel. Bank export writes
one WAV per pad plus a `samples.json` with each pad's settings. `e` and `R` open the same
folder tree as Files (SD card, unit storage or this computer) to choose where to export to
or which export to restore; `/` types a path instead.

Other commands: `ports`, `status`, `pads`, `ls [path]`. Add `-v` to log the session and
`-port` to pick a serial port.

## Layout

| Package | Role |
|---|---|
| `proto`, `proto/septet`, `proto/fileapi` | Wire format, with no I/O |
| `device` | Session with the unit over a `Port`: transactions, events, files, import, render |
| `serialport` | Finds and opens the unit's USB serial port |
| `sim` | In-memory SP-404MKII, for tests and for working without the hardware |
| `formats` | SMP, WAV, AIFF, FLAC, MP3 decoding with resampling to 48 kHz, patterns |
| `app` | What the tool does with a unit, apart from how it is shown: assign, export/restore, patterns, file tree |
| `playback` | Plays samples on this computer's speakers |
| `tui` | The Bubble Tea UI |
| `cmd/sp404` | The command |

Tests: `go test ./...`

## Licence

GPL-3.0-or-later, as SparkyMK2. See `LICENSE`.

Not affiliated with or endorsed by Roland.

## Audio

Files are auditioned on the computer's default output. `sp404 beep` plays a test tone and
`sp404 play <file>` plays a file, to tell silence in the UI from a problem with the output
(for example the system sending sound to the SP-404 itself when it is connected as the
USB audio device). If there is no output, the UI says why when you try to listen.
