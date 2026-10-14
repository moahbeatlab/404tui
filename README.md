# 404tui

A command-line tool and terminal UI for the Roland SP-404MKII, talking to
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
    go run ./cmd/sp404 -version        # print the version (0.1-alpha)

The header says `404tui by moahbeatlab` with the version at the top right. A rail down the
left lists the three sections, **Pads** (`1`), **Files** (`2`) and **Projects** (`3`), and
under them the banks or file sources of the section you are in. Dialogs open over the
screen. A History panel on the right keeps what happened, with times.

### Connecting

The UI starts even with no unit, shows a waiting screen, connects by itself when you plug
it in (the header shows `● CONNECTED` and the port), and goes back to waiting and
reconnects if the cable is pulled. Long operations (restore, export, erase, copies) show a
progress bar with a line per step.

### Pads

- A 4×4 grid in the unit's own arrangement (pad 1 at the top left). Tiles grow taller when
  there is room. The detail panel beside the grid shows the selected pad: length, trim and
  chop points, its settings, which patterns play it, where its sample came from, and a
  progress bar while it sounds on the unit. The cursor follows pads you hit on the unit.
- Each tile shows flag letters, lit when a setting is on: `G` gate, `L` loop, `O` one shot,
  `R` reverse, `S` BPM sync, `V` vinyl, plus `~` for ping-pong and `M1`… for a mute group.
  The detail panel explains them. Pads going to
  BUS 1 are marked in blue, BUS 2 in red, and the bank bar counts them.
- `S` edits a pad's settings (or the marked pads') in a dialog, `,` `.` step its level, and
  `t` sets gate, loop, BPM sync, vinyl, one shot, fixed velocity or bus FX on the marked pads
  (or the whole bank) at once.
- **Bank tempo and volume** sit in a bar above the grid: `< >` step the tempo by 1 BPM,
  `- +` the volume, and `b` opens a dialog to type both.
- **Copying pads:** `c` copies the selected pad (or the marked ones, which keep their layout)
  to any pad in any bank. The unit has no copy command, so the sample is read and written
  onto the other pad together with its settings and chop points. Pads that already hold a
  sample are replaced only after a second Enter. `x` picks a pad up, `p` moves it, `s` swaps.
- **Assigning:** `a` opens a file dialog for the selected pad, with the file tree, file
  details and Space to listen. It reopens where you left it, also after restarting (that,
  the Files position, bank and theme are kept in `state.json` in your config folder). Marked
  files fill free pads in order; `#` instead puts each file on the pad its name starts with
  (`09_…` goes on pad 9), and a tempo in the name (`…_124_…`) sets the pad's BPM. `EMPTY`
  placeholder files are skipped.
- **Where samples came from:** a pad's name holds only 16 characters, so the file it was
  assigned from is noted in `SD:404tui-origins.json` and shown as **From** in the detail
  panel. `F` finds it for pads with no note, by matching names and lengths against the tag
  index, and comparing the audio when several files fit.
- **On this computer:** `o` switches where Enter plays a pad: on the unit (the default) or
  through this computer's speakers (the sample is read from the unit and cut to the part the
  pad plays). `P` plays once on the computer.
- **Chromatic:** `C` turns the keyboard into a piano that plays the pad's sample on this
  computer: `a s d f g h j k l ; '` are the white keys and `w e t y u o p` the black ones,
  `a` being the sample as it is. `z`/`x` shift an octave. The pitch changes with the speed,
  as on the unit; one note sounds at a time.
- `e` exports the bank (one WAV per pad plus a `samples.json` with each pad's settings), and
  `R` restores an export onto a bank you choose. `p` there also makes a pattern: every
  restored pad starts together on beat 1 and is held for 2 bars.

### Slicer

`Z` on a pad (or on an audio file in Files) opens the slicer: the sample as a waveform cut into
slices, played back on this computer in an order you set or at random, and rendered onto a pad
when you like it. Everything happens in the software; the unit's own chop points are not used.
The slices are kept per sample in `slices.json` in your config folder, so the sample opens as
you left it.

- **Slices:** the sample opens in 8 equal slices. `e` cuts it in 16, 32, 4, 8… equal ones, `t`
  slices at the onsets it finds (again for a different sensitivity), `c` cuts the slice in
  front in two and `d` removes it. `h l` choose a slice and `[ ]` move its start (one dot of
  the waveform at a time, `{ }` eight), the end of the slice before follows. `i` zooms to the
  slice, so a dot is a millisecond or less; `I` zooms out. Starts settle on a zero crossing, so
  the cut does not click (`z` turns that off). `u` undoes.
- **Per slice:** `r` reverse, `m` mute, `( )` chance, `< >` pitch in semitones, `- +` gain,
  `X` back to plain. Enter plays the slice once.
- **Order:** `Tab` moves to the order lane, the sequence the slices play in. `H L` carry a step,
  `j k` make it play another slice, `a` repeats it, `d` drops it, `R` shuffles, `o` resets.
- **Mode** (`M`): *order* plays the lane over and over (a slice's chance is the chance it plays,
  else its time is left silent), *random* picks by chance as weight, *no repeat* never plays the
  same slice twice in a row, *shuffle* plays every slice once per round in a new order. `S` rolls
  the seed.
- **Grid** (`T`): off, 1/4, 1/8 or 1/16. On a grid every step lasts that long at the bank's tempo
  (`, .` change it), whatever the slice's length: a longer slice is cut off, a shorter one leaves
  silence, so a bounce can be dropped into a pattern at the tempo. Off, the slices play one after
  the other at their own length.
- **Play** with Space. The next round is rendered while the one before sounds and takes over
  without a gap; edits are heard from the next round on, and the playhead follows on the waveform.
- **Bounce:** `b` asks for a pad and renders what the slicer plays, for as many rounds as `n`
  sets (1, 2, 4 or 8), onto it. The same seed gives the same bounce.

### Patterns

The bank's 16 pattern slots are a panel under the pads; `Tab` moves between the two, and
the highlighted pattern is previewed on the right.

- `N` makes a pattern in the first free slot: every sample of the bank on beat 1, 2 bars.
  `c` or `x` pick a pattern up and `p` puts it in a slot of any bank (a second `p` replaces
  one), `v` marks and `d` deletes.
- `P` or Space plays the pattern **on this computer**, in a loop, with a playhead over the
  steps. Each pad is played with its level, pan, pitch, reverse, gate and loop; the unit's
  envelope, effects, vinyl and time stretch are not reproduced. A change to a pad or a note
  is heard at once.
- Enter opens the step editor. Notes are blocks as tall as their velocity and as wide as
  they last. `h j k l` move, Enter puts a note in or takes it out, `,` `.` change the
  velocity by 1 and `<` `>` by 10, `*` sets it to 127, `+` `-` make a note a step longer or
  shorter, and `u` undoes. `b` moves the pattern's pads to another bank.
- **Mute and solo:** `m` mutes a pad and `s` solos it (in Pads and in the editor). They only
  change what this computer plays: muted rows show a red `MUTE`, soloed ones a green `SOLO`,
  and pads you do not hear are greyed. `S` in the editor saves the pattern **as you hear it**
  into a slot you choose, without the muted pads (or with only the soloed ones), so the unit
  plays that version too.

### Files

- Browses the SD card, the unit's storage or this computer (`t` switches). The rail shows
  the **target bank**; `[` `]` change it. Enter on an audio file opens a pad picker on that
  bank's next free pad, so Enter, Enter assigns it; type a bank letter and a pad number
  (`C`, `7`) to go elsewhere. Space listens, and `Ctrl-A` turns on autoplay.
- `v` marks a file or folder, `V` every file in the folder, and `J` the junk macOS and
  Windows leave on a card. On the marked files: `d` deletes (SD card only, after a question),
  `c` copies and `x` cuts, `p` pastes (also into another source), `n` renames and `N` makes
  a folder. `Ctrl-D`/`Ctrl-U` page, `}`/`{` jump between folders, `Shift-Tab` folds all.
- **Tags:** `i` indexes the folder in front in the background: words from file and folder
  names become tags, saved on the card in `SD:404tui-index.json.gz`, so the index is built
  once and kept up to date, not rebuilt at every start. `*` favourites the marked files or
  folders, and `+` adds your own tags (a word with `-` in front takes one off). The same keys
  work in the `a` dialog.
- **Finding:** `/` searches every audio file of the source by keyword, and `s` searches tags
  only, with the tags listed in a sidebar (`Tab` switches between the two). Enter assigns,
  `Ctrl-O` shows the file in the tree. `f` finds a folder by name; favourite folders come
  first.

### Projects

Switch (Enter) and rename (`n`) projects. `e` exports a project (all its files) to the SD
card or this computer, `R` restores a backup folder (the one holding `PADCONF.BIN`) after
showing what will be erased, and `X` erases a project, leaving it empty. Each asks first.

### Offline

Away from the unit, keep working on your projects on this computer:

1. At home, export the projects you want (Projects, `e`, "This computer") into one folder,
   for example `~/404-projects`. Each export is a project folder as the unit keeps it
   (`PADCONF.BIN`, `SMPL/`, `PTN/`).
2. Without the unit, the waiting screen offers `o`: choose that folder (or one project
   folder in it), or start with `sp404 -offline ~/404-projects`. A yellow **◌ OFFLINE**
   badge stays in the header for as long as you work this way.
3. Everything works as usual on those projects: assign, pad settings, patterns, playback on
   this computer. Each project folder is a project slot, in the order of the folder names,
   and an empty slot becomes a new folder (`PROJECT_02`, …) once you put something in it.
   Every change is saved to the folders as you make it. Samples to assign can be anywhere
   on this computer; the `SD/` folder next to the projects stands in for the SD card.
4. Back at the unit, plug it in: the header says so, and `U` leaves offline mode and
   connects. Then restore a project from its folder with Projects, `R`.

A new project copies the settings of a project exported from the unit when the folder has
one; otherwise it gets the documented defaults, some of which are guesses, so export at
least one project from the unit into the folder first.

### Everywhere

Keys are vim-style (`h j k l`, `g`/`G`, `[`/`]` for banks). `?` lists every key, `:` opens a
command palette, `r` reads everything from the unit again, `T` steps through the colour
themes (Hardware, Ocean, Paper; the choice is kept), and the mouse works for tabs, banks,
pads, rows and the wheel. `q` asks before quitting; `Ctrl-C` quits at once.

Other commands: `ports`, `status`, `pads`, `ls [path]`, `doctor`, `beep`, `play <file>`.
Add `-v` to log the session, `-port` to pick a serial port and `-offline <folder>` to work
without the unit.

## Layout

| Package | Role |
|---|---|
| `proto`, `proto/septet`, `proto/fileapi` | Wire format, with no I/O |
| `device` | Session with the unit over a `Port`: transactions, events, files, import, render |
| `serialport` | Finds and opens the unit's USB serial port |
| `sim` | In-memory SP-404MKII, for tests, and over folders of projects for offline work |
| `formats` | SMP, WAV, AIFF, FLAC, MP3 decoding with resampling to 48 kHz, patterns, `PADCONF.BIN`, the slicer's slices and sequencer |
| `app` | What the tool does with a unit, apart from how it is shown: assign, export/restore, patterns, file tree |
| `playback` | Plays samples on this computer's speakers |
| `tui` | The Bubble Tea UI |
| `cmd/sp404` | The command |

Tests: `go test -race ./...`, or `make test` (vet, gofmt check and the tests).

## Build

    make                    # ./sp404 for this computer
    make dist               # every platform, into dist/
    make linux              # linux amd64 and arm64
    make windows            # windows amd64 and arm64 (.exe)
    make macos              # darwin amd64, arm64 and a universal binary
    make dist VERSION=0.2   # stamp a version into the binaries

Linux and Windows build without cgo from any computer. The macOS binaries need cgo (the serial
port library asks IOKit for the USB ports), so `make macos` runs on a Mac.

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
