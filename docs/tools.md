# Tools and settings of every screen

*Written by the program's own test (`go test ./core -run ToolsReference -update-docs`): the keys and the menus below are what the program has, with the values a menu offers. Do not edit by hand.*

## PADS

The 16 pads of a bank. `a` opens the pad panel: SOUND and SHAPE pages with the pad's settings, and the TOOLS page.

**Keys on a computer:** `a` menu · `Space` play · `x` stop · `hjkl` move · `[ ]` bank · `.` arp marked · `t` tools · `^E` export bank · `g hl` screen (1-6) · `y a` mark pad · `y jk` bank · `y b` clear pad · `s x` record

**Keys on the handheld:** `A` menu · `X` play · `B` stop · `^v<>` move · `L1 R1` bank · `R2` arp marked · `Y+<>` screen · `Y+A` mark pad · `Y+^v` bank · `Y+B` clear pad · `Sel+X` record


### Pad panel (`a`)

The settings of the pad under the cursor, written into the project at once. With pads marked (`y a`) a change goes to all of them.

*Page SOUND* (`[` `]` turn the pages)

- `Level` — 0 to 127 (the default is 127)
- `Pan` — L100 to R100 (the default is C)
- `Pitch` — -12 st to +12 st (the default is +0 st)
- `Fine pitch` — -100 ct to +100 ct (the default is +0 ct)
- `Time stretch` — 50% to 150% (the default is 100%)
- `Sample BPM` — 40 to 200 (the default is 120)

*Page SHAPE* (`[` `]` turn the pages)

- `Attack` — 0 to 127 (the default is 0)
- `Hold` — 1% to 100% (the default is 100%)
- `Release` — 0 to 127 (the default is 0)
- `Play mode` — forward / reverse / ping-pong / rev ping-pong
- `Gate` — off / on
- `Loop` — off / on
- `One shot` — off / on
- `Fixed velocity` — off / on
- `Vinyl` — off / on
- `BPM sync` — off / on
- `Mute group` — off / 1 / 2 / 3 / 4 / 5 / 6 / 7 / 8 / 9 / 10
- `Bus` — off / BUS 1 / BUS 2

*Page TOOLS* — the pad's tools, listed below.

### Pad tools (the TOOLS page of the pad panel, `t`)


*Sample*

- `Pick a sample...`
- `Slice...`
- `Chords & arps...`
  - **chords & arps  A3** (a dialog):

    *The Notes*

    - `Root` — C, Db, ... B (12 values); now C
    - `Chord` — MAJ, MIN, ... 9 (12 values); now MAJ
    - `Sample is in` — C, Db, ... B (12 values); now C
    - `Octaves` — 1 / 2 / 3; now 2

    *How It Plays*

    - `Pattern` — CHORD, UP, ... STABS (11 values); now UP
    - `Rate` — 1/4 / 1/8 / 1/8T / 1/16 / 1/16T / 1/32; now 1/16
    - `Gate` — 25% / 50% / 75% / 100% / ring; now 50%
    - `Swing` — OFF / LIGHT / TRIPLET; now OFF
    - `Length` — 1 bar / 2 bar / 4 bar / 8 bar; now 1 bar

    *Make It*

    - `Play it`
    - `Put it on a free pad`
- `Arp from this one sample (scales, chords)...`
  - **arp from A3** (a dialog):

    *Page Notes*

    - `Key` — C, Db, ... B (12 values); now C
    - `Scale` — CHROMATIC (chord types) / MAJ / MIN / MIN-P / MAJ-P / DOR / MIXO / BLUES; now CHROMATIC (chord types)
    - `On degree (scales)` — 1 / 2 / 3 / 4 / 5 / 6 / 7; now 1
    - `Chord` — MAJ, MIN, ... 9 (12 values); now MAJ
    - `Octaves` — -3 / -2 / -1 / +0 / +1 / +2 / +3; now +1

    *Page Play*

    - `Type` — UP, DOWN, ... THMB (11 values); now UP
    - `Rate` — 1/4 / 1/8 / 1/8T / 1/16 / 1/16T / 1/32; now 1/16
    - `Gate` — 25% / 50% / 75% / 100%; now 50%
    - `Velocity` — 127 / 100 / 80 / 60; now 100
    - `Length` — 1 bar / 2 bar / 4 bar / 8 bar; now 2 bar

    *Page Make*

    - `Hear it / stop it`
    - `Bounce the arp to a new pad`
    - `Bounce the chord to a new pad`
    - `Print the arp to the pattern`
- `Keyboard from it...`
  - **keyboard from A3** (a dialog):
    - `Keys` — 4 pads / 5 pads / 7 pads / 8 pads / 12 pads / 16 pads; now 12 pads
    - `Scale` — chromatic / MAJ / MIN / MIN-P / MAJ-P / DOR / MIXO / BLUES; now chromatic
    - `Put them on free pads`
- `Rename...`
- `Reverse`
- `Normalize`
- `Pitch` — -12 / -5 / -2 / -1 / +1 / +2 / +5 / +12; now +1

*The Pad*

- `Pattern with this pad...`
- `Move or swap to...`
- `Copy to...`
- `Clear the pad`

*Bank A*

- `Find a pad by name...`
- `Set up the pads that were never set up`
- `Switches for every pad of the bank...`
  - **switches of every pad of bank A** (a dialog):
    - `Gate` — on / off; now on
    - `Loop` — on / off; now on
    - `One shot` — on / off; now on
    - `Fixed velocity` — on / off; now on
    - `Vinyl` — on / off; now on
    - `BPM sync` — on / off; now on
    - `Bus of every pad` — off / BUS 1 / BUS 2; now off
- `Set the buses of the bank by the names...`
  - **set the bus of 3 pads by their names?** (a dialog):

    *Bank A*

    - `Yes: 0 to BUS 1, 3 to BUS 2, 0 to none`
    - `This whole project instead...`
      - **set the bus of 3 pads of the project?** (a dialog):
        - `Yes`
        - `No`
    - `No`

    *Examples*

    - `A1 kick_808 -> BUS 2`
    - `A2 snare_tight -> BUS 2`
    - `A3 hat_closed -> BUS 2`
- `Export the bank to a folder`
- `Restore pads from an export...`
  - **restore pads from an export** (a dialog):
    - `A folder somewhere else...`

*Marked Pads (0)*

- `Mark every pad of this bank with a sample`
- `Arp the marked pads...`
  - **arp over 0 marked pads** (a dialog):

    *How It Plays*

    - `Type` — UP, DOWN, ... THMB (11 values); now UP
    - `Rate` — 1/4 / 1/8 / 1/8T / 1/16 / 1/16T / 1/32; now 1/16
    - `Gate` — 25% / 50% / 75% / 100%; now 50%
    - `Velocity` — 127 / 100 / 80 / 60; now 100
    - `Length` — 1 bar / 2 bar / 4 bar / 8 bar; now 2 bar

    *Make It*

    - `Play it / stop it`
    - `Bounce it to a free pad`
    - `Write it into the pattern`
    - `Clear the marks`
- `Clear the marks`

## SEQ

The step grid of the pattern in front. `Enter` (or `b`) opens the note panel, `t` the pattern's tools and settings.

**Keys on a computer:** `a` note on/off · `e hjkl` edit (hold) · `Space` play / stop · `b` note settings · `x` hear note · `.` mute/solo · `hjkl` pad, step · `[ ]` bank · `{ }` bar · `t` tools · `m` mute track · `s` solo track · `f` freeze view · `z` 1 bar / more · `y x` add track · `y b` pick pattern · `y jk` pattern +-

**Keys on the handheld:** `A` note on/off · `A+^<>` edit (hold) · `X` play · `B` note settings · `L2` hear note · `R2` mute/solo · `^v<>` pad, step · `L1 R1` bar · `Y+X` add track · `Y+B` pick pattern · `Sel+B` tools · `Y+^v` pattern +- · `Sel+X` record


### Note panel (`Enter` or `b`)

The note under the cursor: `Velocity` 1 to 127, `Length` from 15 ticks to the end of the pattern, `Timing` (how far off the grid, early or late, in ticks), `Pitch` (-12 to +12 semitones above the pad's own; plain when it is not changed). `a` sets a value back to its default, `x` hears the note, `Esc` closes. On a step with no note `a` puts one.

### Pattern tools and settings (`t`)


*Page Pattern*

- `Choose a pattern (bank, then pattern)...`
- `New pattern from a template...`
  - **new pattern from a template** (a dialog):
    - `Blank pattern` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 4 bars
    - `House  122-127` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Deep house  120-125` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Tech house  122-130` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 4 bars
    - `Techno  128-135` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Hypnotic techno  135-140` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Hip-hop  85-95` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 1 bar
    - `Drum and bass  170-175` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Jungle  160-170` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Dubstep  138-142` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Trance  136-142` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 1 bar
    - `Psytrance  140-148` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Hard techno  145-160` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `UK garage  130-135` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Breakbeat  120-135` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Electro  120-135` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Trap  130-150` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Amapiano  108-115` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Reggaeton  90-100` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
    - `Disco  110-125` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 1 bar
    - `Footwork  155-165` — 1 bar / 2 bars / 3 bars / 4 bars / 5 bars / 6 bars / 7 bars / 8 bars; now 2 bars
- `Loop only the bar in focus (Y+A)` — off / on; now off
- `New pattern from the audible tracks`
- `Bounce the tracks as you hear them to a pad`
- `Bounce only this track to a pad`
- `Unmute and unsolo every track`
- `Copy to the next pattern`
- `Copy to a slot...`
- `Move to a slot...`
- `Use the pads of another bank...`
  - **pattern A1: use the pads of bank** (a dialog):
    - `Bank A`
    - `Bank B`
    - `Bank C`
    - `Bank D`
    - `Bank E`
    - `Bank F`
    - `Bank G`
    - `Bank H`
    - `Bank I`
    - `Bank J`
- `Delete the pattern`

*Tempo*

- `Tempo of bank A` — 60 BPM, 61 BPM, ... 240 BPM (181 values); now 120 BPM
- `Tempo select` — each bank's own / the project's; now each bank's own

*Page Timing*

- `Applies to` — every pad / only A3; now every pad
- `Swing` — 54% / 58% / 62% / 66% / 70%; now 62%
- `Humanize` — light / medium / loose; now medium
- `Quantize to 1/16`
- `Shift` — 1 step < / 1 step >; now 1 step >
- `Reverse`

*Length*

- `Double`
- `Halve`

*Page Notes*


*Write*

- `Fill...`
  - **fill A3** (a dialog):

    *Settings*

    - `Pattern` — every beat, beats 1 and 3, ... pickup before the next bar (19 values); now every beat
    - `Velocity` — 127 / 100 / 80 / 60; now 100
    - `Gate` — 25% / 50% / 75% / 100% / 150% / 200%; now 75%
    - `Decay per hit` — off / 5% / 10% / 15% / 20% / 30% / 40%; now off
    - `Pitch` — -12 st, -11 st, ... +12 st (25 values); now plain

    *Trial*

    - `Keep it (B throws it away)`
- `Euclid...`
  - **euclid on A3** (a dialog):

    *Settings*

    - `Rate` — 1/4 / 1/8 / 1/8T / 1/16 / 1/16T / 1/32; now 1/16
    - `Steps` — 1, 2, ... 32 (32 values); now 16
    - `Hits` — 0, 1, ... 32 (33 values); now 5
    - `Rotate` — 0, 1, ... 31 (32 values); now 0
    - `Gate` — 25% / 50% / 75% / 100%; now 50%
    - `Velocity` — 127 / 100 / 80 / 60; now 100
    - `Decay per hit` — off / 5% / 10% / 15% / 20% / 30% / 40%; now off
    - `Pitch` — -12 st, -11 st, ... +12 st (25 values); now plain

    *Trial*

    - `Keep it (B throws it away)`
- `Evolve over the bars...`
  - **evolve A3** (a dialog):

    *Settings*

    - `Change` — 10% a bar / 20% a bar / 30% a bar / 40% a bar / 60% a bar; now 20% a bar
    - `What` — moves, drops and adds / moves only / adds only / drops only / velocities only / pitches only; now moves, drops and adds
    - `Back to the first bar` — never / every 2 bars / every 3 bars / every 4 bars; now never
    - `Tracks` — this one / every one with notes; now this one
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`

*Change The Notes*

- `Ratchet (sub-steps)...`
  - **ratchet A3** (a dialog):

    *Settings*

    - `Notes` — every note / every 2nd note / every 3rd note / every 4th note / the last note of each bar / 25% of the notes / 50% of the notes / 75% of the notes; now every note
    - `Split into` — 2 parts / 3 parts / 4 parts; now 2 parts
    - `Parts that play` — all parts / skip the first / skip the last / every other part; now all parts
    - `Velocity` — flat / louder / quieter / accent the first; now flat
    - `Pitch of each part` — no change, +1 st, ... -12 st (15 values); now no change
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`
- `Accents and ghosts...`
  - **accents and ghosts on A3** (a dialog):

    *Settings*

    - `Accent` — none / the beats (1 2 3 4) / the off-beats (the ands) / every 2nd note / every 3rd note / every 4th note / the first of each bar; now none
    - `Accent velocity` — 127 / 115 / 105; now 115
    - `The other notes` — as they are / 100 / 90 / 80 / 70 / 60; now 80
    - `Ghost notes` — none / before each hit / after each hit / empty off-beat 16ths / every empty step; now before each hit
    - `Ghost velocity` — 20 / 30 / 40 / 50; now 30
    - `Ghost chance` — 25% / 50% / 75% / 100%; now 50%
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`
- `LFO on the notes (velocity, length, timing)...`
  - **LFO on A3** (a dialog):

    *Settings*

    - `Moves the notes'` — velocity / length / timing / pitch; now velocity
    - `Wave` — SIN / TRI / SQR / SAW / RND / PUMP / SAW DN / S&H / DRIFT; now SIN
    - `Rate (one round)` — 8/1 / 4/1 / 2/1 / 1/1 / 1/2 / 1/4 / 1/8 / 1/16; now 1/1
    - `Velocity goes` — around the notes / between min and max; now around the notes
    - `  min velocity` — 1 / 16 / 32 / 48 / 64 / 80 / 96 / 112 / 127; now 32
    - `  max velocity` — 1 / 16 / 32 / 48 / 64 / 80 / 96 / 112 / 127; now 127
    - `Direction` — as it is / inverted; now as it is
    - `Center (shifts the middle)` — -32 / -16 / +0 / +16 / +32; now +0
    - `Depth` — 10% / 20% / 30% / 50% / 75% / 100%; now 50%
    - `Shift (starts later by)` — 0 / 1/4 / 1/2 / 3/4; now 0
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`
- `ENV: each note shapes the next (velocity, length, timing)...`
  - **ENV on A3** (a dialog):

    *Settings*

    - `Moves the notes'` — velocity / length / timing / pitch; now velocity
    - `Attack` — 0 steps / 0.5 steps / 1 steps / 2 steps / 4 steps / 8 steps; now 0 steps
    - `Decay` — 0 steps / 0.5 steps / 1 steps / 2 steps / 4 steps / 8 steps; now 1 steps
    - `Sustain` — 0% / 25% / 50% / 75% / 100%; now 25%
    - `Release` — 0 steps / 0.5 steps / 1 steps / 2 steps / 4 steps / 8 steps; now 1 steps
    - `Direction` — as it is (up) / inverted (ducks); now inverted (ducks)
    - `Center (shifts the middle)` — -32 / -16 / +0 / +16 / +32; now +0
    - `Depth` — 10% / 20% / 30% / 50% / 75% / 100%; now 50%

    *Trial*

    - `Keep it (B throws it away)`
- `Echo...`
  - **echo on A3** (a dialog):

    *Settings*

    - `Rate` — 1/4 / 1/8 / 1/8T / 1/16 / 1/16T / 1/32; now 1/8
    - `Repeats` — 1, 2, ... 16 (16 values); now 3
    - `Decay` — x85% / x70% / x55% / x40% / x25%; now x70%
    - `Gate` — 25% / 50% / 75% / 100%; now 50%
    - `Pitch of each repeat` — no change, +1 st, ... -12 st (15 values); now no change

    *Trial*

    - `Keep it (B throws it away)`
- `Chance...`
  - **chance on A3** (a dialog):

    *Settings*

    - `Chance` — 0%, 10%, ... 100% (11 values); now 70%
    - `Choices` — fresh every note, repeats every 1, ... repeats every 16 (17 values); now fresh every note
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`
- `Randomize velocity...`
  - **randomize velocity of A3** (a dialog):

    *Settings*

    - `Amount` — +-5 / +-10 / +-15 / +-20 / +-30 / +-40 / +-60; now +-5
    - `Shape` — even / bell; now even
    - `Bias` — -30 / -20 / -10 / +0 / +10 / +20 / +30; now +0
    - `Pitch spread` — off / +-1 st / +-2 st / +-3 st / +-5 st / +-7 st / +-12 st; now off
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`
- `Shape (gate, decay)...`
  - **shape the notes of A3** (a dialog):

    *Settings*

    - `Gate` — to the next note / 25% of a step / 50% of a step / 75% of a step / 100% of a step / 150% of a step / 200% of a step; now 50% of a step
    - `Decay per hit` — off / 5% / 10% / 15% / 20% / 30% / 40%; now off

    *Trial*

    - `Keep it (B throws it away)`
- `Cycle...`
  - **cycle A3** (a dialog):

    *Settings*

    - `Repeat the first` — 1 steps, 2 steps, ... 32 steps (32 values); now 12 steps

    *Trial*

    - `Keep it (B throws it away)`
- `Nudge (push, pull)...`
  - **nudge A3** (a dialog):

    *Settings*

    - `Push (-) or pull (+)` — -40 ticks, -35 ticks, ... +40 ticks (17 values); now +0 ticks

    *Trial*

    - `Keep it (B throws it away)`
- `Pitch (transpose, random, runs)...`
  - **pitch of A3** (a dialog):

    *Settings*

    - `What to do` — transpose by / set every note to / random within / rising run, a step each note / falling run, a step each note / back to plain; now transpose by
    - `Semitones` — -12, -11, ... +12 (25 values); now +1

    *Trial*

    - `Keep it (B throws it away)`
- `Invert`

*Page Tracks*

- `Dispatch over the marked pads...`
  - **dispatch A3 over the marked pads** (a dialog):

    *Settings*

    - `Mode` — in turn / at random; now in turn
    - `Another roll of the dice`

    *Trial*

    - `Keep it (B throws it away)`
- `Copy to a pad...`
  - **copy A3 to a pad** (a dialog):

    *Settings*

    - `To` — A1, A2, ... J16 (160 values); now A1

    *Trial*

    - `Keep it (B throws it away)`
- `Clear`
- `Add a track (choose a pad)...`

## PROJECTS

On a computer: the 16 projects of the unit when one is plugged in (online), else those of a folder on this computer (offline). The program looks for the unit all the time, and the start screen offers `u` online and `o` offline (the project open last). `a` opens a project's menu, `t` chooses online or offline and the offline folder. On the handheld this screen is called UNIT.

**Keys on a computer:** `a` project menu · `hjkl` choose · `Esc` back · `t` online / offline · `g hl` screen (1-6) · `s x` record

**Keys on the handheld:** `A` project menu · `X` look again · `^v` choose · `L1 R1` page · `B` back · `Y+<>` screen · `Sel+X` record


### Project menu of the unit (`a`, online)

- `Send: replace this project with the editor's (editor -> unit)...`
- `Edit live: open it here, every change goes to the unit at once`
- `Open a copy in the editor (the unit is not changed)`
- `Back up: write a copy to this computer`
- `Restore: write a backup over this project...`
  - **restore over unit project 1** (a dialog):
    - `A folder somewhere else...`
- `Rename it on the unit...`
- `Make it the project the unit has open`
- `Erase it on the unit...`
  - **erase unit project 1: samples, patterns and settings?** (a dialog):
    - `Yes, erase it`
    - `No`

### Online or offline (`t`)


*Work*

- `Online: no unit is plugged in (looking for it)`
- `Offline: the projects in ~ui-docs/out/ROLAND/SP-404MKII`

*Offline Folder*

- `Choose the folder of the offline projects...`

### Project menu of the offline folder (`a`, offline)

- `Write the project of the editor here`
- `Export the project of the editor into a folder...`

## FILES

The file browser: this computer, the unit's card and its storage. `t` chooses where the files are loaded from (the computer, the samples folder, the unit's SD card, a USB drive, favourites) and leads to the file tools.

**Keys on a computer:** `a` to a pad... · `Space` listen · `x` stop · `jk` browse · `l` open · `h` up · `[ ]` page · `Esc` pads · `t` from where · `m` mark file · `g hl` screen (1-6) · `y x` places · `y a` search · `y b` favourite · `s a` mark · `s x` record

**Keys on the handheld:** `A` to pad A3 · `X` listen · `^v` browse · `>` open · `<` up · `L1 R1` page · `B` pads · `Y+<>` screen · `Y+X` places · `Y+A` search · `Y+B` favourite · `Sel+A` mark · `Sel+B` files menu · `Sel+X` record


### Load files from where (`t`)


*Places*

- `samples    ~p/404tui-docs/samples`
- `the top    /`

*Favourites (0)*


*This Folder*

- `Add ~4tui-docs/samples/drums to the fav~`

*Search Index*

- `Index /tmp/404tui-docs/samples/drums`

### File tools (in the menu of `t`)


*Marks*

- `Mark every file here`
- `Unmark everything`

*The File*

- `Copy (1)`
- `Cut (1)`
- `Paste here (0)`
- `Delete...`
- `Rename...`

*This Folder*

- `New folder...`

## SLICE

The slicer: cut a sample into slices and play them in an order. `Enter` opens the settings, `t` the tools.

**Keys on a computer:** `a` cut · `b` join · `x` play slice · `.` play order · `,` grid step · `hl` move · `jk` marker · `t` tools · `n` new order · `+ -` zoom in / out · `y a` auto cut · `y x` to pads · `y b` settings · `s x` record

**Keys on the handheld:** `A` cut · `B` join · `X` play slice · `R2` play order · `L2` grid step · `<>` move · `^v` marker · `Y+A` auto cut · `Y+X` to pads · `Y+B` settings · `Sel+B` tools · `Y+^v` zoom · `Sel+X` record


### Slice settings (`Enter`)


*Page Slice 1*

- `Pitch` — -12, -11, ... +12 (25 values); now +0
- `Gain` — 25% / 50% / 75% / 100% / 125% / 150% / 200%; now 100%
- `Reverse` — off / on; now off
- `Fade in` — 0 ms, 2 ms, ... 2000 ms (11 values); now 0 ms
- `Fade out` — 0 ms, 2 ms, ... 2000 ms (11 values); now 0 ms
- `Roll` — off / 2 hits / 3 hits / 4 hits / 6 hits / 8 hits; now off
- `Roll louder` — -100 / -75 / -50 / -25 / +0 / +25 / +50 / +75 / +100; now +0
- `Roll pitch` — -12, -11, ... +12 (25 values); now +0
- `Chance` — off / 25% / 50% / 75% / 100%; now 100%
- `Reset this slice`

*Page Play*

- `Play order` — in order / random / no repeat / shuffle; now in order
- `Grid, at 120 BPM of bank A` — off / 1/2 / 1/4 / 1/8 / 1/16; now off
- `Swing` — off / 54% / 58% / 62% / 66% / 70%; now off
- `Humanize` — off / 25% / 50% / 100%; now off
- `Length` — auto, 1 steps / 4 steps / 8 steps / 12 steps / 16 steps / 24 steps / 32 steps / 48 steps / 64 steps / 128 steps; now auto, 1 steps
- `Play the whole order`
- `Play again, new dice`
- `Tools (Select+B)...`
  - **slice tools  1 slices** (a dialog):

    *Page Cut*

    - `Cut in equal parts` — 2 / 4 / 8 / 16 / 32; now 8
    - `Cut at random places` — 2 / 4 / 8 / 16 / 32; now 8
    - `Tap: cut while the sample plays`
    - `Cut on the grid at 120 BPM` — 1/4 / 1/8 / 1/16 / 1/32; now 1/8
    - `Auto-cut (Y+A): up to` — 4 slices / 8 slices / 16 slices / 24 slices / 32 slices; now 16 slices
    - `Auto-cut (Y+A): sensitivity` — low / less / normal / more / high; now normal

    *Page Order*

    - `Mangle the slices (dice)...`
      - **mangle  1 slices** (a dialog):
        - `Which slices` — at random (the dice) / spread evenly / the first ones / the last ones; now at random (the dice)
        - `Drop-outs (silent slices)` — 0 / 1; now 0
        - `Reversed slices` — 0 / 1; now 0
        - `Pitched slices` — 0 / 1; now 0
        - `  pitch by` — random in the range, -12 semitones, ... +12 semitones (13 values); now random in the range
        - `  random range (+/-)` — 2 semitones / 5 semitones / 12 semitones; now 5 semitones
        - `Slices with a roll` — 0 / 1; now 0
        - `  roll hits` — random / 2 hits / 3 hits / 4 hits / 6 hits / 8 hits; now random
        - `Humanize (loudness, timing)` — off / 25% / 50% / 100%; now off
        - `Fades at the slice ends` — 0 ms / 10 ms / 50 ms / 200 ms; now 0 ms
        - `Roll the dice again`
        - `Keep it (B puts the slices back)`
    - `Edit the order lane (step by step)`
    - `Shuffle the order`
    - `Play order back to 1, 2, 3...`

    *Page Keep*

    - `Result to a pad (choose bank and pad)...`
    - `Slices to pads (choose the first)...`
    - `Slices to pads, random order...`
    - `Slices to pads and a pattern under them` — 1 bar / 2 bars / 4 bars / 8 bars; now 2 bars
    - `Add the sample of a pad...`
    - `Add a sample from a file...`
    - `Take the last added sample out`
    - `Settings: slice, order, grid (Y+B)...`
      - **slice settings  1 slices** (a dialog):

        *Page Slice 1*

        - `Pitch` — -12, -11, ... +12 (25 values); now +0
        - `Gain` — 25% / 50% / 75% / 100% / 125% / 150% / 200%; now 100%
        - `Reverse` — off / on; now off
        - `Fade in` — 0 ms, 2 ms, ... 2000 ms (11 values); now 0 ms
        - `Fade out` — 0 ms, 2 ms, ... 2000 ms (11 values); now 0 ms
        - `Roll` — off / 2 hits / 3 hits / 4 hits / 6 hits / 8 hits; now off
        - `Roll louder` — -100 / -75 / -50 / -25 / +0 / +25 / +50 / +75 / +100; now +0
        - `Roll pitch` — -12, -11, ... +12 (25 values); now +0
        - `Chance` — off / 25% / 50% / 75% / 100%; now 100%
        - `Reset this slice`

        *Page Play*

        - `Play order` — in order / random / no repeat / shuffle; now in order
        - `Grid, at 120 BPM of bank A` — off / 1/2 / 1/4 / 1/8 / 1/16; now off
        - `Swing` — off / 54% / 58% / 62% / 66% / 70%; now off
        - `Humanize` — off / 25% / 50% / 100%; now off
        - `Length` — auto, 1 steps / 4 steps / 8 steps / 12 steps / 16 steps / 24 steps / 32 steps / 48 steps / 64 steps / 128 steps; now auto, 1 steps
        - `Play the whole order`
        - `Play again, new dice`
        - `Tools (Select+B)...`

### Slice tools (`t`)


*Page Cut*

- `Cut in equal parts` — 2 / 4 / 8 / 16 / 32; now 8
- `Cut at random places` — 2 / 4 / 8 / 16 / 32; now 8
- `Tap: cut while the sample plays`
- `Cut on the grid at 120 BPM` — 1/4 / 1/8 / 1/16 / 1/32; now 1/8
- `Auto-cut (Y+A): up to` — 4 slices / 8 slices / 16 slices / 24 slices / 32 slices; now 16 slices
- `Auto-cut (Y+A): sensitivity` — low / less / normal / more / high; now normal

*Page Order*

- `Mangle the slices (dice)...`
  - **mangle  1 slices** (a dialog):
    - `Which slices` — at random (the dice) / spread evenly / the first ones / the last ones; now at random (the dice)
    - `Drop-outs (silent slices)` — 0 / 1; now 0
    - `Reversed slices` — 0 / 1; now 0
    - `Pitched slices` — 0 / 1; now 0
    - `  pitch by` — random in the range, -12 semitones, ... +12 semitones (13 values); now random in the range
    - `  random range (+/-)` — 2 semitones / 5 semitones / 12 semitones; now 5 semitones
    - `Slices with a roll` — 0 / 1; now 0
    - `  roll hits` — random / 2 hits / 3 hits / 4 hits / 6 hits / 8 hits; now random
    - `Humanize (loudness, timing)` — off / 25% / 50% / 100%; now off
    - `Fades at the slice ends` — 0 ms / 10 ms / 50 ms / 200 ms; now 0 ms
    - `Roll the dice again`
    - `Keep it (B puts the slices back)`
- `Edit the order lane (step by step)`
- `Shuffle the order`
- `Play order back to 1, 2, 3...`

*Page Keep*

- `Result to a pad (choose bank and pad)...`
- `Slices to pads (choose the first)...`
- `Slices to pads, random order...`
- `Slices to pads and a pattern under them` — 1 bar / 2 bars / 4 bars / 8 bars; now 2 bars
- `Add the sample of a pad...`
- `Add a sample from a file...`
- `Take the last added sample out`
- `Settings: slice, order, grid (Y+B)...`
  - **slice settings  1 slices** (a dialog):

    *Page Slice 1*

    - `Pitch` — -12, -11, ... +12 (25 values); now +0
    - `Gain` — 25% / 50% / 75% / 100% / 125% / 150% / 200%; now 100%
    - `Reverse` — off / on; now off
    - `Fade in` — 0 ms, 2 ms, ... 2000 ms (11 values); now 0 ms
    - `Fade out` — 0 ms, 2 ms, ... 2000 ms (11 values); now 0 ms
    - `Roll` — off / 2 hits / 3 hits / 4 hits / 6 hits / 8 hits; now off
    - `Roll louder` — -100 / -75 / -50 / -25 / +0 / +25 / +50 / +75 / +100; now +0
    - `Roll pitch` — -12, -11, ... +12 (25 values); now +0
    - `Chance` — off / 25% / 50% / 75% / 100%; now 100%
    - `Reset this slice`

    *Page Play*

    - `Play order` — in order / random / no repeat / shuffle; now in order
    - `Grid, at 120 BPM of bank A` — off / 1/2 / 1/4 / 1/8 / 1/16; now off
    - `Swing` — off / 54% / 58% / 62% / 66% / 70%; now off
    - `Humanize` — off / 25% / 50% / 100%; now off
    - `Length` — auto, 1 steps / 4 steps / 8 steps / 12 steps / 16 steps / 24 steps / 32 steps / 48 steps / 64 steps / 128 steps; now auto, 1 steps
    - `Play the whole order`
    - `Play again, new dice`
    - `Tools (Select+B)...`
      - **slice tools  1 slices** (a dialog):

        *Page Cut*

        - `Cut in equal parts` — 2 / 4 / 8 / 16 / 32; now 8
        - `Cut at random places` — 2 / 4 / 8 / 16 / 32; now 8
        - `Tap: cut while the sample plays`
        - `Cut on the grid at 120 BPM` — 1/4 / 1/8 / 1/16 / 1/32; now 1/8
        - `Auto-cut (Y+A): up to` — 4 slices / 8 slices / 16 slices / 24 slices / 32 slices; now 16 slices
        - `Auto-cut (Y+A): sensitivity` — low / less / normal / more / high; now normal

        *Page Order*

        - `Mangle the slices (dice)...`
        - `Edit the order lane (step by step)`
        - `Shuffle the order`
        - `Play order back to 1, 2, 3...`

        *Page Keep*

        - `Result to a pad (choose bank and pad)...`
        - `Slices to pads (choose the first)...`
        - `Slices to pads, random order...`
        - `Slices to pads and a pattern under them` — 1 bar / 2 bars / 4 bars / 8 bars; now 2 bars
        - `Add the sample of a pad...`
        - `Add a sample from a file...`
        - `Take the last added sample out`
        - `Settings: slice, order, grid (Y+B)...`

## CONFIG

The program's settings; on a computer the writing of the project (what SAVE does on the handheld) is its first section.

**Keys on a computer:** `jk` setting · `hl` change · `a` edit / switch · `[ ]` jump · `Esc` pads · `g hl` screen (1-6) · `s x` record

**Keys on the handheld:** `^v` setting · `<>` change · `A` edit / switch · `L1 R1` jump · `B` pads · `Y+<>` screen · `Sel+X` record


### Settings

*Project*

- `Project number` — left right change
- `Write it to the card folder` — A writes
- `Export it into a folder...` — A
- `Autosave` — left right change
- `Quit the program` — A

*Buses By Pad Name*

- `Words for samples on BUS 1` — A edits
- `Words for samples on BUS 2` — A edits
- `Colour of BUS 1` — left right change
- `Colour of BUS 2` — left right change
- `A name with none of the words` — left right change
- `Set the bus when a sample is put on a pad` — A or left right switch
- `Set the buses of this bank by the names...` — A
- `Back to the defaults` — A

*Recorder*

- `Record into folder` — A edits
- `Input with this in its name` — A edits
- `Recorder` — 

*Pad Codes On The Unit*

- `Start and stop a recording` — A edits

*Midi*

- `Input with this in its name` — A edits
- `Control channel` — A edits
- `Live mode` — A edits
- `Start and stop a recording` — A edits
- `Next pattern becomes the one that plays` — A edits
- `Clear the next pattern` — A edits
- `Pad volumes channel` — A edits
- `MIDI` — 

*Colours*

- `Colour theme of the terminal` — left right change

## SAVE (handheld only)

Writes the project as a folder for the SD card. On a computer this is in CONFIG, under PROJECT.

**Keys on the handheld:** `A` write · `<>` slot · `^v` autosave · `L1 R1` slot 10 · `X` quit · `B` back · `Y+<>` screen

## PERF (handheld only)

The unit's MIDI as it plays, and the pattern now and next.

**Keys on the handheld:** `A` pattern now · `X` next pattern · `B` back · `Y+<>` screen · `Y+A` next -> now · `Y+X` clear next · `Y+B` note BUS 1 · `Sel+B` note BUS 2 · `Sel+X` record

## LOOP (handheld only)

Makes patterns from the slices of the pads.

**Keys on the handheld:** `A` make the pattern · `X` delete (twice) · `^v<>` move · `L1 R1` bank · `B` pads · `Y+<>` screen · `Y+X` remix · `Y+^v` slices · `Sel+X` record

