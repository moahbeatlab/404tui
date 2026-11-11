# End-to-end tests of the computer's program

`platform/terminal/e2e/run.sh` builds `sp404` for Linux and runs two scripts, `e2e.sh` (UC01-UC13) and `e2e-tools.sh` (the advanced tools, UC14-UC17). It starts the program **as a user would**: in a real terminal (tmux, 150x44), in an Ubuntu
container, against a copy of the real project *Construction Kits House 01* (71 pads, 98 MB, 124 BPM, two-bar patterns) and its real
samples. It plays keys, reads what is on the screen and keeps every step as text, as colours and as a picture in
`platform/terminal/e2e/out/` (the table below names the pictures). The run is `-audio silent`: everything works as if sound were
played, nothing is. The results of each run, per use case, are in `out/usecases.tsv`.

    platform/terminal/e2e/run.sh                      # needs docker; the real project is read from ~/test-projects
    platform/terminal/e2e/run.sh /path/to/project     # another project (the checks name pads of the Construction Kits project)

Last run (2026-11-03): **205 checks, 205 passed, 0 failed**, 17 use cases (126 in `e2e.sh`, 79 in `e2e-tools.sh`).

## The use cases

| # | Use case | What the test does and expects | Pictures |
| --- | --- | --- | --- |
| UC01 | Start | the splash shows; a key opens the pads of the project at its tempo (124 BPM); pad A3 is the loop; the title says PADS | `01-splash`, `02-pads` |
| UC02 | Layout on every size | at 80x30, 100x34, 120x40, 160x48 and 200x56 the side panel starts at 70% of the width; a 70x20 window says it is too small and the screen comes back when it grows | `03-size-*`, `04-too-small` |
| UC03 | Go to a screen | `1`-`9`, `Tab`, `Shift-Tab`, `g g`, `g l` and `g h` land on the right screen (a pad with no sample bounces the slicer back) | |
| UC04 | The pad's information | on A3: the sample (size 1451.6 KB, 124 BPM, 4.0 bars), routing (BUS 1), the tile text (`4.0 bars`, `G L O R S V`); on a drum pad BUS 2 and the playback, sound and envelope groups; `]` and `[` change the bank | `05-pad-info`, `06-pad-bus2` |
| UC05 | The pad panel | `a` opens the panel (SOUND); `b` does not close it; a setting is changed; `]` turns to SHAPE; `Esc` closes; `u` takes the change back | `07-pad-panel`, `08-pad-panel-shape` |
| UC06 | The pad's tools | `t` opens the TOOLS page: Slice..., Arp...; `Esc` closes | `09-pad-tools` |
| UC07 | The sequencer | two bars side by side with the steps numbered 1 to 32; `z` one bar (and `]` shows steps 17 to 32); `f` freezes (FROZEN) and lets go; `Enter` opens the note panel, `b` does not close it, `Esc` does; `t` opens the pattern tools, two pages on is Euclid, `Enter` opens its dialog, `Esc` goes back to the tools menu and `Esc` again closes it; `x` hears the note without starting the pattern; `Space` plays (PLAY) and stops (STOP); `a` puts a note of velocity 100 on an empty step and takes it off again | `10`-`16` |
| UC08 | The slicer | on A3: `y a` cuts into 16 slices, the panel tells about slice 1 of 16; `+` zooms in and `-` out; `Enter` opens the slice settings and `Esc` closes; `t` the slice tools; `n` makes a new order and plays it in a loop, `x` stops it; `.` plays the order in a loop and again stops it; `n` again while it plays; `b` removes a slice (15) and `u` brings it back (16) | `17`-`20` |
| UC09 | Files | the ranger view has panes; the real samples are listed; `l` goes into the folder; `/` opens a one-line search, typed text filters and the line says how many files; `Enter` keeps the result and `Esc` ends the search; `t` opens the files menu | `21`-`23` |
| UC10 | The other screens | LOOP, PERF (says "no MIDI in this build"), CONFIG with its prompt (`Esc` closes it), UNIT without a unit, the help page, `Ctrl-R` without a recorder says why it cannot record | `24`-`28` |
| UC11 | A unit (the simulator) | the unit and its projects are found; the project menu; a backup is made and has the project's files; loading asks first when there is unsaved work; live editing starts (LIVE 1); the places include the card; the card is listed; the files menu has the tag entries; the card is indexed; a search by tags finds the kick | `29`-`34` |
| UC12 | Save | the project is written unchanged: PADCONF.BIN, the patterns and every sample file are byte for byte those of the project that was opened | `35-saved` |
| UC13 | Quit | `q` quits; nothing was written to stderr as a crash | |
| UC14 | Pattern tools on a track | for the track in front (one note): **Fill** (a trial of 8 notes), **Euclid** (10), **Echo** (4), **Cycle** (3) each open their dialog, show their trial in the panel and **Esc** throws it away (one note again); Ratchet, Evolve, Accents and ghosts, LFO, ENV, Chance, Randomize velocity, Shape and Nudge open their dialogs and `Esc` returns to the tools menu; **Keep it** commits the Euclid notes, `u` undoes the tool as one step, `R` redoes it; **Clear** takes every note off and `u` brings them back; Copy to a pad asks for the pad | `40`-`43` |
| UC15 | Patterns | **Double** makes the pattern four bars and **Halve** two again; the tempo of the bank goes up and down by one; **New pattern from a template** lists the templates with their tempo, House makes a new pattern in the next free slot with named tracks; Copy to the next pattern; Use the pads of another bank offers the banks; **Delete the pattern** empties the slot and `u` brings it back with its tracks | `44`-`46`, `51` |
| UC16 | Slicer tools | **Cut in equal parts** (8), at random places, on the grid; auto-cut again (16); **Shuffle the order** and back to 1, 2, 3; **Mangle** opens its dialog and `Esc` puts the slices back; **Edit the order lane** shows the lane and `y b` leaves it; **Slices to pads and a pattern under them** puts the slices on pads (their lengths show on the tiles) and a pattern with them in SEQ | `47`-`50`, `52` |
| UC17 | Slicer undo | the slice settings dialog opens and closes; `b` removes a slice and `u` brings it back, `R` removes it again; a cut in equal parts is taken back by `u`, and `u` goes on back | |

Together 205 checks. A failed check prints the screen it saw.

## How it reads the screen

The test sends keys with `tmux send-keys` and reads the screen with `tmux capture-pane`. A menu entry is found on the screen: `hl.py` reads the colours of the screen and says which entry is lit (dark letters on the amber ground), and the script moves the cursor until it is the entry it wants, so the tests do not depend on how far down an entry is. A check waits up to ten seconds for a text to
appear (`check`), or for a pattern (`checkre`), or says a text must not be there (`absent`). The colours of each step are kept
(`.ansi`) and turned into a picture by `ansipng`, so a change of look can be looked at after the run.

## What it does not cover

Real sound, a real unit, a real recorder or MIDI port; macOS and Windows (the program is built for them, but only Linux is run here);
the handheld (its screens are guarded by `go test ./core -run Handheld`, a picture of every screen that must not change).

## The pictures in the docs

The pictures in [ui.md](ui.md), [replay.md](replay.md), [handheld.md](handheld.md) and the README are made from the real program, not drawn:

- **The computer's program** (`docs/img/pc-*.png`): the same helpers as above (`e2e-lib.sh`: `start`, `key`, `snap`) start `sp404` in a tmux window of 150x44 (200x56 and 80x30 for the sizes), against a copy of the Construction Kits House 01
  project, and play the keys of the use case; `snap` keeps the screen with its colours and `platform/terminal/e2e/ansipng` turns that into a PNG. A private tmux socket (`TMUX_SOCKET`) keeps it apart from any other session, and `-samples` and `-out`
  are given neutral folders so that no path of the maker's computer is on a picture. The unit pictures use `-sim`.
- **The handheld** (`docs/img/handheld-*.png`): `sp404h -script "@pads Y+R @seq"` writes a PNG of the framebuffer for each `@name`.
- **The replay** (`docs/img/replay-*.png`, `.jpg`): the editor pages are the replay page in a headless Chrome driven over the DevTools protocol (the buttons of the sidebar are clicked, the audio is put at a time, the screen is
  saved); the frames are `sp404 replay render take.wav -from 22 -to 22.06 -fps 20`, the first picture of the video. The take is a real recording with its captions, chapters and a cut set in `take.events.cues.json`.

When the look of a screen changes, take its picture again the same way; a picture that shows an old screen is a bug of the docs.
