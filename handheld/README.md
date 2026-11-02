# 404h: the SP-404MKII on a handheld (proof of concept)

A front end for 404tui's file engine (`lab`, `formats`) that is driven with a D-pad and the four buttons on the
right, nothing else: no shoulder buttons, no Start/Select, no sticks, no keyboard. It makes projects for the
SP-404MKII away from a computer: pick samples, cut them, play patterns together, write the project to a card or
send it over USB. There is no synth: it is about samples, slicing and sequencing.

    go run ./cmd/sp404h -demo -samples /tmp/404/samples -web :8405     # a page with the screen and buttons
    make arm64 / GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o sp404h-arm64 ./cmd/sp404h

## The keys

`Y` is the shift key, as on an M8: held, the D-pad moves between screens, and A, B and X do their second thing.
`Y` tapped alone shows the key map. The footer of every screen shows the keys of that screen.

| | PADS | FILES | SLICE | SEQ | SAVE | UNIT |
| --- | --- | --- | --- | --- | --- | --- |
| arrows | move over the 4x4 pads | browse, `>` open, `<` back | move the cursor (hold: faster), up/down jump to a cut | move over steps and pads | `<` `>` project number | choose a unit project |
| A | pad menu | assign to the pad | cut at the cursor | put or take a note | write the project | send (asks twice) |
| B | stop | back to PADS | join the slice | menu (stop while playing) | back | back |
| X | play the pad | listen | play the slice | play / stop the pattern | | look again |
| Y + arrows | `<` `>` screen, `^` `v` bank | screen | screen, zoom | screen, pattern | screen | screen |
| Y + A / B / X | B: clear the pad | | A: auto-cut at the onsets, X: slices to pads | A: velocity of the note, X: next bank | | |

Pad menu: pick a sample, slice, move or swap, copy, reverse, normalize, pitch, clear. Sequencer menu: fill a row
(every beat, backbeat, off-beats, eighths, sixteenths, boom-bap, 3+3+2), Euclidean row, swing, humanize, quantize,
shift, copy to the next pattern, delete.

## Where things go

- Samples: any folder (`-samples`). WAV, AIFF, FLAC, MP3, SMP.
- SAVE writes `<out>/ROLAND/SP-404MKII/PROJECT_nn/`, the layout the simulator in this repo (from SparkyMK2's
  reverse-engineered docs) uses for a card. Put `<out>` at the root of the SP's card, or copy the folder.
- UNIT: with the SP-404MKII on the OTG port, it finds the unit, lists its 16 projects and sends the project
  over one of them (the same restore the main program does). Without a unit, `-sim` finds a simulated one.
- `-open <project folder>` starts from a project that is already on a card.

## What has been tried

Tried: the Go tests (`go test ./handheld`: all keys, assign, slice, slices to pads, patterns, save and reopen,
move and copy, send to the simulated unit); screens drawn to PNG; the web page (frames and keys over HTTP);
arm64 build (7 MB, no cgo).

Not tried: **anything on an R36S**. That means the framebuffer (`-fb`), the input device and its codes
(`Evdev`: BTN_EAST/SOUTH/NORTH/WEST and the D-pad as buttons or a hat; firmware images differ, so A/B may
need swapping), audio output through ALSA, the permissions in `packaging/arkos/404h.sh`, the OTG port as a USB host
for the SP-404MKII, and the speed of the slicer on the RK3326. Nothing written was loaded into a real SP-404MKII:
the pad, sample and pattern files come from the same engine as the web Sample Lab, whose output is unverified
on hardware too. Patterns are always 4 bars (other lengths are a guess).


## Added after the first run on the R36S

- **Select+Start quits** from any screen (the pad reports them as codes 0x2c0/0x2c1; 0x13a/0x13b are accepted too).
  A project with samples that was not written is asked about once: Select+Start again quits, any other key stays.
  On SAVE, X does the same without the chord.
- **FILES browses the whole card**: `<` goes up to `/`, `>` opens a folder, `B` leaves for the pads, `Y+X` jumps between
  places (samples folder, `/mnt/usbdrive`, `/roms`, `/`). An empty samples folder says where to put files.
- **Projects load from the file system**: a folder with a PADCONF.BIN is marked `project`; `A` on it replaces the project
  in memory (asks first when there is unsaved work). Folders named PROJECT_nn also set the slot number.


## The panel and the tracker (second round)

- **Right panel on every screen**, as on the M8: a map of the six screens with the current one lit, the tempo and the
  loop's state, what is under the cursor (pad, file preview, slice numbers, pattern and note), and the keys that work
  right now with their letters. Hold Y and the panel lights the shifted set (`SHIFT`). The footer is only the message line.
- **SEQ is a tracker**: a row for each step (16 to a bar, scrolling, a line at each bar), a column for each of 8 pads,
  the velocity of a note as a number (100), `--` where there is none. Up/Down move the step (held: faster),
  Left/Right the column, `A` puts or takes a note, `Y+A` cycles the velocity, `Y+X` gives the column the next pad with a
  sample, `Y+Up/Down` changes the pattern. The columns start as the pad that was in front, then pads with notes, then
  pads with samples, in pad order; the menu's *Pad of this column* picks any pad. The menu's row operations (fill,
  euclid, clear, shift) now work on the column.

## Shoulder buttons and the launcher's diagnostics

- **L1 / R1 page** (codes 0x136 / 0x137 on the R36S's pad): a page of 20 in FILES, a bank in PADS, a bar in SEQ, the zoom
  in SLICE, 10 slots in SAVE, 8 projects in UNIT. They are listed in the panel. In the browser: PageUp / PageDown (or q / e),
  and in scripts `[` and `]`.
- **When it starts with a black screen**, `logs.log` (copied to `/mnt/usbdrive/r36s/`) says why: the launcher writes the
  uptime, memory, the biggest processes and any copy of the program that is still running; it samples the program's state
  and kernel wait function at 1, 2, 3, 5, 7, 10 and 15 s; the program dumps all goroutines after 4 s without a first frame.
  A copy of the program left running by an earlier start is stopped; the program runs from `/dev/shm` or `/tmp` when
  those allow programs, so its code is not read from the card while it runs.

## Slicer menu, pattern templates, pattern picker (third round)

- **Slicer menu (Y+B in SLICE)**: for the slice under the cursor: pitch, gain, reverse, fade in/out, roll (a stutter of that
  many hits, getting louder or quieter, rising or falling in pitch) and chance; for the whole slicing: the play order
  (in order, random, no repeat, shuffle), a grid (1/2 to 1/16 at the project tempo), swing, humanize, and the **length of
  the result in steps**. Actions: *Play the result*, *Play again, new dice*, *Result to a free pad*, *Shuffle the order*
  (a random order that stays), *Slices to pads, shuffled*. Changing a slice's value plays that slice. A slice's settings are
  baked into its audio when it goes to a pad. (The engine is the computer slicer's, `formats.Slicing`.)
- **Templates (SEQ menu, B)**: the genre grooves of the computer app (house, deep house, techno, hip-hop, drum and bass,
  jungle, trap, amapiano, disco, footwork...) with a number of bars, written into the next free pattern (the one in front
  when it is empty), never over a pattern with notes. Which pad plays kick, snare, hat... is guessed from the sample
  names; a voice with no name match takes a pad of the columns that has a sample, so any kit gets a beat. The message says
  which pad took which voice. *Blank pattern* makes an empty pattern of 1 to 8 bars. *Tempo* in the same menu sets the BPM.
- **Pattern picker (Y+B in SEQ, or the menu's first item)**: patterns are in banks A to J of 16, like the pads. L1/R1 change
  the bank (so does Left/Right past the edge of the grid), the arrows choose the pattern, A opens it. Patterns with notes
  show their bars and notes. Y+Up/Down still steps one pattern.
- In every menu L1/R1 page the list (or ten values, on an item with many values such as the tempo); long lists scroll.

## Fourth round

- **Velocities are decimal** (0-127) in the tracker and the panel.
- **The play order is on the SLICE screen**: the numbers under the waveform are the slices in the order they play, the slice
  under the cursor lit. *Shuffle the order* in the slicer menu changes it (and the message lists it); *Slices to pads* (Y+X)
  puts the slices on the pads in that order, each once.
- **One slice or the whole order**: X (or L2) plays the slice under the cursor, R2 plays the whole order as the slicer is
  set (order or random mode, grid, swing, length in steps). The same is in the slicer menu (*Play the whole order*).
  L2/R2 are codes 0x138/0x139 on the R36S's pad (not seen to press yet); in the browser: z and c, in scripts `{` and `}`.
- **SEQ shows one bar at a time**: the 16 steps of a bar fill the whole height, a line at each beat, `bar 2/4` at the top
  right with a marker for the bar the loop is in while it plays. L1/R1 turn to the next or previous bar (keeping the row);
  the cursor turns the page by itself when it runs past a bar line.
