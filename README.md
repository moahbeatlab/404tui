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
  listens first. `Ctrl-A` turns on autoplay in the file list, the search and the `a` dialog: a file plays as the cursor rests on it, and moving on stops the old sound at once. `/` searches every audio file in the source by keyword (words narrow it,
  folders count, the first search indexes the source and later ones are instant): Enter
  assigns, Tab shows the file in the tree, Ctrl-L listens.
- **Assigning from Pads:** `a` opens a file dialog for the selected pad, with the file tree,
  file details and Space to listen. It reopens exactly where you left it (source, open
  folders, highlighted file), also after restarting; that and the Files tree position,
  bank and theme are kept in `state.json` in your config folder.
- **Working with files:** `v` marks a file or folder, `V` every file in the folder, and `J` all
  the junk macOS and Windows leave on a card (`._` files, `.DS_Store`, `Thumbs.db`). On the
  marked files (or the one in front): `d` deletes (SD card only, after a question that counts
  what is in them), `c` copies and `x` cuts to the clipboard, and `p` pastes into the folder in
  front. The clipboard survives `t`, so you can copy from the card to this computer and back.
  Nothing is overwritten. `N` makes a folder. `Ctrl-D`/`Ctrl-U` move half a page and `}`/`{`
  (or `Alt-n`/`Alt-p`) jump to the next or previous folder.
- **Pads on this computer:** `o` switches where Enter and a click play a pad: on the unit (the
  default) or through this computer's speakers like a file preview (the sample is read from the
  unit, cut to the part the pad plays; no pitch, envelope or effects). `P` plays once on the computer.
- **Chromatic:** `C` on a pad turns the keyboard into a piano that plays its sample on this
  computer: `a s d f g h j k l ; '` are the white keys and `w e t y u o p` the black ones, `a`
  being the sample as it is and each key after it a semitone higher. `z`/`x` shift an octave
  down or up, Enter plays the last note again, Space stops it. The pitch changes with the speed,
  as on the unit, so a higher note is shorter; one note sounds at a time and plays to its end
  (a terminal does not report a key being let go).
- **Projects** switches and renames projects. `e` exports the project (all its files) to the
  SD card or this computer, and `R` restores a backup folder (the one holding `PADCONF.BIN`)
  over the project: it reads and checks the backup, shows what will be erased, and asks
  before it writes anything.

Keys are vim-style (`h j k l`, `g`/`G`, `[`/`]` for banks, `1`-`4` for sections); `:`
opens a command palette, `?` lists every key, `/` finds by name, `T` changes the colour
theme, and the mouse works for tabs, banks, pads, rows and the wheel. Bank export writes
one WAV per pad plus a `samples.json` with each pad's settings. `e` and `R` open the same
folder tree as Files (SD card, unit storage or this computer) to choose where to export to
or which export to restore; `/` types a path instead. Restore then asks which bank to put
the pads on (the one they came from is chosen; pads that would be replaced are counted and
need a second Enter). `p` there also makes a pattern: after the restore, every restored pad
starts together on beat 1 and is held for 2 bars; you choose the pattern slot next (a slot
that holds a pattern needs a second Enter).

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

Files are auditioned on the computer's default output. `sp404 beep` plays a test tone with each of the two playback methods in turn (and names the system's output device), `sp404 play <file>` plays a file,
and `-audio afplay` switches the UI to macOS's own player if the default one stays silent, to tell silence in the UI from a problem with the output
(for example the system sending sound to the SP-404 itself when it is connected as the
USB audio device). If there is no output, the UI says why when you try to listen.

## Before you run it on a unit

Nothing here has been verified against hardware, so check it in this order:

1. `sp404 ports` lists every serial port and why each is or is not taken for the unit
   (USB id `0582:02e7`). If the unit is plugged in and no line says `SP-404MKII`, discovery
   is the problem; `-port /dev/cu.…` skips it.
2. `sp404 doctor` runs read-only checks and compares what this program parses with what the
   unit says: the status poll, project settings and names, all 160 pad blocks, every
   sample file's size and header against its pad block, the file API on the unit and the SD
   card, `PADCONF.BIN`, and our pattern parser against the unit's own list of pads a
   pattern plays. It changes nothing; add `-preview` as an argument to also play a pad.
   Each line is `ok`, `skip` (something the project does not have) or `FAIL` with the
   disagreement spelled out.
3. `sp404 -v` (the UI) writes a debug log: every transaction with its timing, port
   discovery, connects and disconnects, every operation on the unit with its result, and the
   keys pressed. The UI owns the screen, so the log goes to a file (`debug.log` in the user
   cache folder, or `-log <file>`), and a `● debug log` chip shows in the status bar.
   `-trace` adds every message on the wire as hex. The first lines say which build,
   machine and flags the log came from.

If something fails, `sp404 -trace -log doctor.log doctor` (or the same flags for the UI)
produces what is needed to see where the implementation and the unit part ways.
