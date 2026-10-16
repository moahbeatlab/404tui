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

The header says `404tui by moahbeatlab` and the project, then the state of the link and the version. The tabs are in the top right corner (on the row under the header when it is too narrow): **Pads** (`1`), **Files** (`2`), **Projects** (`3`), **Slicer** (`4`) and **Sequencer** (`5`); click one or press its number. A rail down the left lists the banks or file sources of the section you are in. Dialogs open over the
screen. A History panel on the right keeps what happened, with times.

`?` anywhere lists, searchable, what can be done with the view or dialog in front (the same list as `:` for a view; for the slicer, the pattern editor and the file dialogs, their own keys; Enter sends a single key); `1`–`5` always change the view; the slicer and the Sequencer's pattern stay as they were when you come back (Esc closes them). In the Slicer tab, `a` chooses what to slice: `p` a pad (a list of the pads, over the tab) or `f` a new sample from the file view (`Z` slices it). The slicer opens over the tab, with the tab lit. In the Sequencer tab, `a` lists the slots of a bank with a preview of the one in front (`[ ]` for another bank; Enter opens it in the editor, Esc closes the editor); `a` in the editor chooses another pattern, and `N` makes a new one.

### Buses by pad name

A pad goes to a bus by its name. **Config** (`6`) has two lines of words, one for the samples that go
to BUS 1 and one for BUS 2 (the second starts with the words of a drum kit: kick, snare, hat, perc, …;
Enter edits a line), the colour each bus is drawn in (`h` `l` choose one; it is the colour of the
pads and everywhere else), the bus of a name with none of the words (BUS 1 to start with), and whether
the bus is set when a file is assigned (it is). A word matches a word of the pad's name (`Kick_125`,
`OHat`, `DrumLoop`, `14_Perc1`), and a longer word also its start; a name with words of both lines goes
to BUS 1. The view shows what the rule makes of the bank in front. For pads that are on the unit
already, `B` in Pads looks at the bank (or the marked pads) and asks before it sets the bus of those
that are not on the one their name says; `p` there widens it to the whole project. The rule is kept
with the rest of what 404tui remembers (`state.json` in the user's config folder, under `404tui`).

### Checking and setting up pads

`K` in Pads reads the sample of the pad (or of each marked pad) from the unit and compares it with
the file it came from, converted the way an assign converts it (48 kHz, 16 bit): identical, or how
many samples differ and by how much, or a different length or channel count. The file is the one
noted when the pad was assigned, or found by `F` or `W`. A pad made in an offline project used to
start with hold 1 and the pan at its far left end, which plays a sliver of the sample to one side;
a new sample and a new pad now start with the pan in the centre, hold 100 and time stretch 100%, and
`H` in Pads sets up the pads of the bank (or the marked ones) that still look that way, after asking.
In the pad settings (`S`), a step stops on the default (the centre of the pan) and `0` puts the
setting back to it.

### Recording

`Ctrl-R` anywhere (in a dialog too) records the unit's audio, which it sends over USB as an audio
interface, to a WAV file (24-bit, 48 kHz, stereo) named by the date and time, and `Ctrl-R` again
stops it. While it runs the header shows a red, blinking `● REC 02:13  23.1 MB` on every tab, and
quitting stops it first. It runs `ffmpeg` (or `sox` where there is no ffmpeg), which must be
installed; it finds the input by a part of its name (`404` by default). **Config** (`6`) sets the
folder (`~/Music/404tui` by default) and that part of the name. It works on macOS, Linux (through
PulseAudio or PipeWire) and Windows (DirectShow, ffmpeg only).

### Perform (experimental)

Tab **Perform** (`7`) is a dashboard for the stage. The unit does not say which pattern runs, so it is
told: `a` chooses the pattern that plays and `n` the next one; `Space` makes the next the one that plays.
The samples of the pattern that plays are drawn as big as the screen allows, with the pad
number: that is what there is to mute and bring back. Under it, the next pattern: which samples to
**bring in** and which to **take out**, and in the pattern that plays each sample that leaves is
marked `▼ OUT NEXT`. Below that, what goes to each bus (BUS 1 and BUS 2, in their colours) and a
note of the effect on it (`f`, `F`), because the unit does not tell the effect. A recording started
with `Ctrl-R` is shown at the top.

### Pad codes (experimental)

Pads hit in turn on the unit can switch things on: **1 3 3 7** switches the live mode (the Perform tab,
and back to where you were), **1 16 16 16** starts a recording and the same code stops it. Any bank
counts; the pad numbers are what matter, and a code has to be played within four seconds. Config (`6`)
sets both codes (pad numbers 1 to 16 with spaces between; `off` turns one off). The unit sends nothing
when a pad is hit, so this is read from the pad it has selected and from how far the pad that sounds
has played, and the unit is asked about ten times a second while a code is set. That means: a pad
that is hit again while it is the selected one shows only through its sample starting over, so the
pads that repeat in a code need a sample, and the first pad of a code is not seen if it was the
selected pad already and makes no sound.

### MIDI (experimental)

The unit sends MIDI over USB: a note for each pad hit, and its clock and start/stop. On a Mac, `make build`
builds MIDI input in (`go build -tags midi`; it needs cgo); other builds leave it out. 404tui listens to
the input with `404` in its name (set in Config, `6`). What it does:

- **Pads:** a note on channels 1 to 10 is a pad hit (pad 1 is note 48, pad 13 note 36), so the pad codes
  see every hit exactly, repeated or on an empty pad. It takes the place of the polling for ten seconds
  after a note.
- **Clock:** the Perform tab counts `♩ BAR 5 · BEAT 2 · LOOP 3` from the clock and start/stop the unit
  sends (24 ticks to a quarter note; the loop is of the pattern you said plays). The unit has to send
  MIDI clock: check its MIDI sync setting.
- **Control channel:** a note (or a controller turned up past half) on the control channel, 16 to start
  with, does an action: notes 60 to 63 switch the live mode, start and stop a recording, make the next
  pattern the one that plays, and clear it. The channel and each message are set in Config.
- **Movement in Perform:** pads hit on the unit light up in the Perform tab by how hard they were hit and
  fade out (a stripe on the tile's edge is its bus, the selected pad is tinted), every controller seen
  gets a fader and a dial with its value and `channel:number` in a CONTROLS strip (the ones moved lately
  are bright), and the last messages scroll under them. Config has a MIDI monitor (counts per kind, the tempo of the clock, the last messages; `c`
  clears) to see what the unit and anything plugged into it really send.
- **Event log:** while Ctrl-R records the unit's audio, everything above is also written beside the WAV
  as `name.events.jsonl`, one JSON object per line with its time in seconds from the start of the
  audio (`on`/`off` with bank and pad, `cc`, `program`, `sysex`, `beat` with the tempo, `start`/`stop`,
  `select`/`sound` from the polled pad, `pattern` for the cue). It is the base for replaying a
  performance.

Run with `-v` to write a debug log (to the user cache folder, e.g. `~/Library/Caches/404tui/debug.log` on a
Mac, or where `-log FILE` says): the MIDI input found or missed, every message the unit sends except the
clock (which is summed up every 16 beats), pad codes, and the start and end of recordings.

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
you left it. The keys are listed at the bottom of the dialog, in groups.

**Cutting**

- The sample opens in 8 equal slices. `e` cuts it in 16, 32, 4, 8… **equal** slices (the message
  says which one comes next; it divides the sample by length and does not look at the tempo), `E` cuts the
  same number of slices at **random places**, so they differ in length (again for another roll),
  `t` slices at the **onsets** it finds (again for a different sensitivity), `c` cuts the slice in
  front in two and `d` removes it. `h l` choose a slice, `u` undoes.
- **On the tempo** (`C`): cuts one slice per step of the grid, at the tempo you set: `T` picks
  the step (1/4, 1/8, 1/16; with the grid off, `C` turns on 1/16 at the bank's tempo) and `, .`
  the BPM. The message says how many bars of 4/4 the sample is at that tempo: 8.00 means the
  tempo fits, 7.4 means try another with `, .` and `C` again. With the cut cursor on (`D`), one
  grid line is put at the cursor, to say where the downbeat is when the sample starts early;
  what is before it becomes a slice of its own. Samples put together (`A`) are each cut from
  their own start. `u` undoes.
- **By hand** (`D`): takes every edge away (`u` brings them back) and shows a cursor. `h l` move
  it a dot (eight with `f`), `H L` jump, by a grid step when a grid is on (`T`), else by 1/32 of
  the sample. `p` puts an edge at the cursor; while the sample plays (Space) it cuts at the
  playhead instead, so you can listen and mark as it goes by. `d` removes the slice the cursor is
  in; `D` or Esc puts the cursor away.
- **Tap** (`p` when the cursor is not on): the sample plays from the start and every press of Space cuts a slice there, the
  way you chop on the SP. The slicing starts again from one slice, so the slices you had are
  replaced (`u` brings them back). A key press comes a little after the hit, so a mark moves to
  the onset just before it, up to 100 ms back. `u` takes the last mark back, `p` or Esc stops.
- **Several samples** (`A`): puts another sample after the one you are slicing.
  The dialog steps aside and the file browser comes up: move to an audio file, press `Z`, and it joins
  (`1` goes to the pads instead, to take the sample of a pad; Esc gives up and goes back to where you were),
  cut in 8 slices of its own; Esc gives up. Slices of all the samples are played in one
  sequence, so the random modes pick across them, for example between two vocals. A thin line on
  the waveform shows where a sample ends, and a line under it lists them. `e`, `E` and `t` cut
  every sample on its own. The slicing you make of those samples is kept: put the same samples
  together again, in the same order, and it is back. `u` takes the last added sample out.
  Up to 8 samples.

**Start and end**

- Every slice has a start and an end of its own, and moving one moves nothing else: slices can
  leave gaps between them or overlap. `[ ]` move the start of the slice in front and `{ }` its
  end, one dot of the waveform at a time (`f` makes it eight). `i` zooms to the slice, so a dot
  is a millisecond or less; `I` zooms out. Edges settle on a zero crossing, so the cut does not
  click (`z` turns that off).

**Per slice**

- **Several slices at once:** `s` marks the slice in front (`y` all of them); while some are marked, the keys below and the envelope and roll keys change every marked slice, not only the one in front, in one undoable step.
- **More edges inside a slice:** `/` cuts the slice in front in 2 equal parts, then 3, 4, 6, 8 on the next presses; `c` cuts it in two at the middle, `D` places edges by hand.
- `r` reverse, `m` mute, `( )` chance, `< >` pitch in semitones, `- +` gain, `X` back to plain.
  Enter plays the slice once.
- **Envelope** (`6 7` attack shorter/longer, `8 9` release shorter/longer; `1`–`5` always change the view): a fade in and out of
  the slice, from 0 up to 2 s, baked into its audio, so it sounds the same on every pad and in the
  computer's playback. 0 leaves only the short fade that stops a cut from clicking.
- **Slices to pads** (`s` marks the slice in front, `y` marks all or none, `B`): each marked slice
  is rendered as it sounds (reverse, pitch, gain, envelope) and put on its own pad, on the free
  pads from the one you choose onward. Marks go when slices are cut or deleted.
- **Roll** (`x`): the slice plays its start 2, 3, 4, 6 or 8 times in the time it has, like a drum
  roll. On a grid the time is the step, so a roll of 4 on a 1/8 step is four 1/32 hits. Press
  `x` again for the next number; after 8 the slice plays once.

**Order, mode and the seed**

- **Order:** `Tab` moves to the order lane, the sequence the slices play in. `H L` carry a step,
  `j k` make it play another slice, `a` repeats it, `d` drops it, `R` shuffles, `o` resets.
- **Mode** (`M`): *order* plays the lane over and over (a slice's chance is the chance it plays,
  else its time is left silent), *random* picks by chance as weight, *no repeat* never plays the
  same slice twice in a row, *shuffle* plays every slice once per round in a new order.
- **Seed** (`S`): the slicer has no randomness of its own, it rolls dice from a number, the seed.
  The same seed always gives the same picks, rests and humanizing, so what you heard can be
  heard again and a bounce is repeatable. `S` picks a new seed and so a different sequence.
  Every round of a playing loop uses the next number after the seed, which is why the loop
  differs from round to round; Space starts again at the first round of the seed.

**Playing along with a pattern**

- `P` opens the patterns as the pads lie (arrows or `h j k l` move, `[ ]` change the bank,
  Enter puts that pattern under the slices, `0` takes it away, Esc leaves things as they were).
  The pattern is mixed from its pads on this computer, at its bank's tempo, and the slicer takes
  that tempo (a 1/16 grid if there was none; `, .` are locked meanwhile). Every round is exactly
  as long as the pattern, with the slice steps on its grid, and the two are added into one
  sample, so they stay together, and a new order (`M`, `S`, `R`) is heard from the next round on,
  at the loop point. No unit is needed beyond the files it holds.
- A bounce (`b`, `B`) is only ever the slices: the pattern is for playing along. The pattern is
  mixed again by itself when what it plays changes (a bounce onto one of its pads, a pad's
  setting, the pattern edited), and heard from the next round.

**Feel**

- **Grid** (`T`): off, 1/4, 1/8 or 1/16. On a grid every step lasts that long at the bank's tempo
  (`, .` change it), whatever the slice's length: a longer slice is cut off, a shorter one leaves
  silence, so a bounce can be dropped into a pattern at the tempo. Off, the slices play one after
  the other at their own length.
- **Swing** (`w`, needs the grid): every second step comes late. 54% is a little, 66% a
  triplet feel, 70% a lot. With an odd number of steps in a round the last one stays straight.
- **Humanize** (`W`): 25, 50 or 100%. Each step's loudness varies a little and, on a grid, each
  step comes a little late (by up to an eighth of a step at 100%). It follows the seed.

**Playing and bouncing**

- **Play** with Space. The next round is rendered while the one before sounds and takes over
  without a gap; edits are heard from the next round on, and the playhead follows on the waveform.
- **Bounce:** `b` asks for a pad and renders what the slicer plays onto it, for as many rounds as
  `n` sets (1, 2, 4 or 8). The same seed gives the same bounce. A bounce is the slices alone,
  not the pattern under them; with a pattern and a grid the slicer says how many rounds make a
  whole number of pattern loops, and `N` sets exactly that many, so the bounce loops cleanly
  against the pattern. `V` limits the pattern under the slices to one bar, two bars, or all of
  it (like `v` in the pattern editor); the view and the loop follow.
- **Roll shapes:** `x` makes a slice roll; `j k` make its hits quieter or louder from the first to the
  last (a small slider shows it), and `;` `'` make the last hit lower or higher in pitch, up to 12
  semitones: a fade-out stutter, a swell, or a riser. **Stretch:** `K` puts the whole sample, stretched without a change of
  pitch to a whole number of bars at the grid's (or the pattern's, or the bank's) tempo, on a
  pad; slice that pad at the tempo. `F` fits the grid tempo to the sample instead.
- **Takes** (`v`): 1, 2, 4 or 8. With more than one, `b` renders that many takes, each from its
  own rounds (take 1 is what a loop plays in its first rounds, take 2 the rounds after them…), and
  puts them on the **free** pads of the bank from the pad you choose onward, named `name slc1`,
  `name slc2`… Pads that hold a sample are skipped, never replaced. Listen to them on the unit,
  keep the best and delete the rest. Takes only differ when something is random: use a random
  mode, a chance below 100 or humanize; otherwise `b` tells you so.

### Patterns

The bank's 16 pattern slots are a panel under the pads; `Tab` moves between the two, and
the highlighted pattern is previewed on the right. In an open pattern `Tab` expands the track on
the cursor row into a big step lane (`Enter` does too) and folds it again; Esc leaves the
pattern, from the grid or from an expanded track, back to the patterns under the pads.

- `N` makes a pattern in the first free slot: every sample of the bank on beat 1, 2 bars.
  `c` or `x` pick a pattern up and `p` puts it in a slot of any bank (a second `p` replaces
  one), `v` marks and `d` deletes.
- `B` in an open pattern **bounces** it to one sample on a pad you choose: the mix that plays on
  this computer, as long as the pattern (or the bars of the range), so what you muted with `m`
  is left out and, with some tracks soloed (`s`), only those are in it. To print one track,
  solo it. You stay in the pattern, to mute others and bounce again. The unit's envelope,
  effects and time stretch are not in it, as when it plays here.
- **Select notes** (`V` in an open pattern): chooses notes of one track to change together.
  `h l` move over the track, `x` (or `Enter`) chooses the note under the cursor or lets it go again, and
  `a` takes every note of the track. Then `, .` and `< >` change their velocity, `+ -` their
  length, `H L ( )` nudge them, `0` puts them back on the grid, `c C` gives them a condition and
  `d` takes them out; the selection follows the notes. `Esc` leaves it. To make a variant of
  some notes (another pitch, say), select them, bounce the track to a new pad and point the
  notes at it.
- **Conditions** (`c` forward, `C` back, on the note under the cursor): when a note plays over the
  loops of the pattern, as the trig conditions of other sequencers do: every loop, `1:2` (the
  first of every 2 loops), `2:2`, `1:3` … `3:3`, `1:4` … `4:4`, `1:8` … `8:8`, then chances (90,
  75, 50, 25, 10 %). The unit's pattern files have no place for them, so they are kept on this
  computer (`conditions.json` next to the state file), by pad and sixteenth, and a note with one
  is drawn `◐`. They apply to what plays here and to a bounce, not to the pattern on the unit.
  The pattern then runs as a **cycle**: the least common multiple of the conditions (`1:2` with
  `2:3` makes 6 loops), and with a chance in it at least 4 loops, rolled from a seed (`y` rolls
  again). Every time you change a condition the line under the grid says how long the bounce is:
  `bounced: 6 loops = 12 bars = 24.0 s`; the message says it too. The loop that plays here is the
  same cycle, with the playhead going round each loop of it, and `B` bounces all of it.
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
- **Three columns:** the Files view shows the folder above, the folder in front and a preview
  (the inside of a folder, the details of a file), as the ranger file manager does. `j k` move
  along the folder in front, `h` goes up a folder, `l` or Enter goes into one. A folder is read
  as soon as the cursor is on it, to be shown in the preview, and closed again when the cursor
  moves on. `w` switches to the folder tree and back.
- **Tags:** `i` indexes the folder in front in the background: words from file and folder
  names become tags, saved on the card in `SD:404tui-index.json.gz`, so the index is built
  once and kept up to date, not rebuilt at every start. `*` favourites the marked files or
  folders, and `+` adds your own tags (a word with `-` in front takes one off). The same keys
  work in the `a` dialog.
- **Used folders:** a folder restored onto a bank with `R` gets a `◆` in the tree (and so do the
  folders above it), and its details say "Used already: restored onto project 1, bank C", then list the pads of each project, bank by bank, that hold a sample from the folder. The notes are in
  `SD:404tui-folders.json`, one entry per project and bank, so it never holds more than 160.
  `W` in Pads goes the other way for the project in front: it finds where the samples came from
  (as `F` does), then notes, for every bank whose samples are mostly (and at least two) from
  one folder, that the folder is on that bank. `W` (and `F`) first asks which folder to look in: the folder dialog opens on the one used last
  time, on the card or on this computer, and Enter looks in the indexed files under the folder in
  front. It needs those folders indexed with `i`. Pads assigned from now on
  note their file by themselves; `W` is for the ones assigned before. The `a` dialog shows both
  for the folder in front: where it was restored, and which pads of which projects came from it.
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
   this computer. Each project folder is a project slot, and is named for it: the slot number and
   the project's name (`02 Flavors`), renamed for you when the project is renamed or when the
   folder is opened (a folder that starts with its number keeps that slot; the others fill the free
   slots in the order of their names). An empty slot becomes a new folder (`PROJECT_02`, …) once you
   put something in it, and takes the project's name along once it has one.
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
