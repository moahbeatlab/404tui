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
- **SEQ was a tracker** (rows were steps, columns pads; see the next rounds for the grid it is now): a row for each step (16 to a bar, scrolling, a line at each bar), a column for each of 8 pads,
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
- **Tempo is the bank's** (the SP-404MKII keeps a tempo for each bank A-J, and *Tempo Select* says whether the banks play
  at their own or all at the project tempo). The panel shows the tempo that plays, `BPM 124 bank A` or `BPM 90 proj`; the
  sequencer plays the pattern at it, and the slicer's grid uses its pad's bank. The SEQ menu's *Tempo of bank X* (or
  *of the project*) changes the one that plays, and *Tempo select* switches between the two. Before, the app read only
  the project tempo word, which also holds the Tempo Select flag (it came out as a nonsense value that fell back to 120
  BPM) and which the unit ignores in bank mode.

## Autosave and the splash (fifth round)

- **Autosave**: every few minutes (off, 1, 2, 3, 5, 10 or 15; Up/Down on the SAVE screen, remembered) the project is written to
  `<card>/autosave/PROJECT_nn`, never to where a save writes, and only when something changed. Only the files that differ
  from the last autosave are written (checksums), in the background, so a 100 MB project does not stop the screen after the
  first one; PADCONF.BIN goes last so a power cut leaves a project that opens. A quit writes a last one first. The SAVE
  screen shows the interval and the last lines of the log (also in logs.log); the footer says how long ago it was.
- **Splash** (about 2.5 s, any key skips it): the logo and a dashboard (project, samples folder, card, autosave, version). When an
  autosave of an earlier session is there and nothing is open, it stays until a key: **A restores it**, any other key starts
  empty. A restored project counts as not saved: SAVE writes it.

## Actions: pads and patterns (sixth round)

Every action belongs to a **pad** (its sample) or to a **pattern**; each has its own menu, with headings.

| Menu | Group | Actions |
| --- | --- | --- |
| **PADS, A** (sample tools) | SAMPLE | pick a sample, slice, **chords & arps**, **keyboard**, reverse, normalize, pitch |
| | THE PAD | pattern with this pad, move or swap, copy, clear |
| **SLICE, Y+B** (the slicer, a sample tool) | slice / order / result | pitch, gain, reverse, fades, roll, chance; play order, grid, swing, length in steps; play, to a pad, shuffle |
| **SEQ, B** (pattern tools) | THE PATTERN | choose a pattern, template, copy to the next, delete |
| | TEMPO | tempo of the bank (or the project), tempo select |
| | TIMING AND LENGTH | swing, humanize, quantize, reverse, double, halve (every pad) |
| | THE COLUMN | fill, euclid, shift, clear, pad of the column (the notes of one pad) |
| | GENERATE | **arp over the columns** |

- **Chords & arps** (a sample tool): the pad's sample pitched once for each note of a chord, mixed or laid out in time (11
  patterns, rates 1/4 to 1/32 with triplets, gate, swing, 1-3 octaves, 1-8 bars at the bank's tempo) and put on a free pad.
  This is the computer app's engine.
- **Keyboard**: 4 to 16 pitched copies of the sample on a run of free pads, chromatic or in one of the scales: the pads play like
  keys.
- **Arp over the columns** (a pattern tool, after the arpeggiator of the unit's DOOM OS): the pads of the columns that have a
  sample are the keys; it lays notes over them in time and writes them into the pattern, replacing those pads' notes.
  Types: UP, DOWN, U/D, U/D+ (the ends twice), RND, ORD (as the columns stand), CHRD, CONV (outside in), DIV (middle out), PNKY
  (the top key between), THMB (the bottom key between); rate, gate, velocity, and the length when the pattern is new. With a
  keyboard (above) as the columns it is an arp of one sample at different pitches, on the unit itself.

## Marking pads: the live arp (seventh round)

- **Y+A on PADS marks a pad** as a key of the arp (only a pad with a sample; up to 16): a numbered badge shows on its tile, in the
  order of marking. Y+A again unmarks. **L2 clears the marks.**
- **R2 starts and stops a loop of the arp over the marked pads**, with the type, rate, gate and velocity of the pattern arp. The panel
  says ARP while it plays; B stops it, and so does leaving PADS. The pad menu's *Arp the marked pads...* has the settings,
  which go into the loop as you change them (type, rate, gate, velocity, length), and:
  - **Play it / stop it**
  - **Bounce it to a free pad**: the loop as a sample
  - **Write it into the pattern**: the notes over the marked pads, in the pattern in front (their notes are replaced)
  - **Clear the marks**
- The marked pads are the **columns** of SEQ, so the same keys are in front when the pattern is opened.

## The step grid (eighth round)

- **SEQ is the unit's step grid**: the pads are the rows (up to 16, A1 at the top by default), the 16 steps of a bar run across.
  A note is a block as tall as its velocity (the number is in the panel for the note under the cursor). Left/Right move the step
  (held: faster), Up/Down the pad, **L1/R1 turn the bar** to the left and right, and the cursor turns it by itself at a bar line.
  `bar 2/4` at the top right, with a marker for the bar that plays. Y+X gives the row the next pad that has a sample; the menu's
  *Pad of this row* any pad. Marked pads (Y+A on PADS) are the rows.

## Files and samples (ninth round)

- **The browser keeps its place**: the folder and the cursor in it come back after assigning a file, the cursor of every folder is
  remembered while the app runs, and the last folder is the start folder of the next run (settings.json, beside the card folder,
  not in it; the autosaves moved there too).
- **Listening while the pattern plays**: the audio has a second voice for previews, so a preview no longer stops the loop (it
  did before, and the panel still said PLAY). With the pattern looping, **X on a file plays it *in the pattern*, in place of the
  target pad's sample** (the pad's notes play the file; X on another file tries that one); A keeps it, and the loop goes on with
  the new sample. A target pad with no notes in the pattern gets a plain preview over the loop. B on PADS stops everything.
- **Search (Y+A)**: an on-screen keyboard (arrows, A types, B deletes, X done, Y+B clears); the list is filtered live to the audio
  files in this folder *and below* whose path has all the words. `<` ends the search. A search gives up after 1.5 s or 400 hits and says so.
- **Favourites and places (Y+X)**: a menu of the places (samples, usb drive, games card, the top) and the favourite folders (go there
  or remove), with *Add this folder*. Y+B toggles the folder as a favourite at once; a favourite shows a `*` in the path.
- **Memory**: only the last 8 files looked at are kept decoded (browsing a big folder used to keep every file in memory).

## Undo and redo (tenth round)

- **Y+L2 undoes, Y+R2 redoes**, on every screen, for everything that changes the project: assigning, clearing, moving and
  copying pads, slices to pads, chords and arps, keyboards, notes and pattern tools, templates, arps, tempo and Tempo Select,
  loading a project or an autosave. The message says what was undone (`undone: hat_closed on A3 (2 more)`) and the help page
  (tap Y) the depth of both stacks.
- A snapshot is taken before each key and kept when the key changed the project; no action has to be wired in. The lab replaces
  a file with a new buffer and never edits one, so a snapshot shares the sample data and costs a copy of the pad settings
  (0.16 ms a key press on the 98 MB project). A key held down that goes on changing something (the tempo) is one step.
- 30 steps deep, and cut shorter when the undo steps would keep more than 192 MB of samples alive that the project no longer has.
- An edit after an undo ends the redo steps. Cursor moves, marks, the screen and what is playing are not part of it.
- A pattern that loops goes on with the project that undo gives back; if it is left with no notes it stops.
