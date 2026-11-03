# End-to-end tests of the computer's program

`platform/terminal/e2e/run.sh` builds `sp404` for Linux and starts it **as a user would**: in a real terminal (tmux, 150x44), in an Ubuntu
container, against a copy of the real project *Construction Kits House 01* (71 pads, 98 MB, 124 BPM, two-bar patterns) and its real
samples. It plays keys, reads what is on the screen and keeps every step as text, as colours and as a picture in
`platform/terminal/e2e/out/` (the table below names the pictures). The run is `-audio silent`: everything works as if sound were
played, nothing is. The results of each run, per use case, are in `out/usecases.tsv`.

    platform/terminal/e2e/run.sh                      # needs docker; the real project is read from ~/test-projects
    platform/terminal/e2e/run.sh /path/to/project     # another project (the checks name pads of the Construction Kits project)

Last run (2026-11-03): **126 checks, 126 passed, 0 failed**, 13 use cases.

## The use cases

| # | Use case | What the test does and expects | Pictures |
| --- | --- | --- | --- |
| UC01 | Start | the splash shows; a key opens the pads of the project at its tempo (124 BPM); pad A3 is the loop; the title says PADS | `01-splash`, `02-pads` |
| UC02 | Layout on every size | at 80x30, 100x34, 120x40, 160x48 and 200x56 the side panel starts at 70% of the width; a 70x20 window says it is too small and the screen comes back when it grows | `03-size-*`, `04-too-small` |
| UC03 | Go to a screen | `1`-`9`, `Tab`, `Shift-Tab`, `g g`, `g l` and `g h` land on the right screen (a pad with no sample bounces the slicer back) | |
| UC04 | The pad's information | on A3: the sample (size 1451.6 KB, 124 BPM, 4.0 bars), routing (BUS 1), the tile text (`4.0 bars`, `G L O R S V`); on a drum pad BUS 2 and the playback, sound and envelope groups; `]` and `[` change the bank | `05-pad-info`, `06-pad-bus2` |
| UC05 | The pad panel | `a` opens the panel (SOUND); `b` does not close it; a setting is changed; `]` turns to SHAPE; `Esc` closes; `u` takes the change back | `07-pad-panel`, `08-pad-panel-shape` |
| UC06 | The pad's tools | `t` opens the TOOLS page: Slice..., Arp from this one sample; `Esc` closes | `09-pad-tools` |
| UC07 | The sequencer | two bars side by side with the steps numbered 1 to 32; `z` one bar (and `]` shows steps 17 to 32); `f` freezes (FROZEN) and lets go; `Enter` opens the note panel, `b` does not close it, `Esc` does; `t` opens the pattern tools, two pages on is Euclid, `Enter` opens its dialog, `Esc` goes back to the tools menu and `Esc` again closes it; `x` hears the note without starting the pattern; `Space` plays (PLAY) and stops (STOP); `a` puts a note of velocity 100 on an empty step and takes it off again | `10`-`16` |
| UC08 | The slicer | on A3: `y a` cuts into 16 slices, the panel tells about slice 1 of 16; `+` zooms in and `-` out; `Enter` opens the slice settings and `Esc` closes; `t` the slice tools; `n` makes a new order and plays it in a loop, `x` stops it; `.` plays the order in a loop and again stops it; `n` again while it plays; `b` removes a slice (15) and `u` brings it back (16) | `17`-`20` |
| UC09 | Files | the ranger view has panes; the real samples are listed; `l` goes into the folder; `/` opens a one-line search, typed text filters and the line says how many files; `Enter` keeps the result and `Esc` ends the search; `t` opens the files menu | `21`-`23` |
| UC10 | The other screens | LOOP, PERF (says "no MIDI in this build"), CONFIG with its prompt (`Esc` closes it), UNIT without a unit, the help page, `Ctrl-R` without a recorder says why it cannot record | `24`-`28` |
| UC11 | A unit (the simulator) | the unit and its projects are found; the project menu; a backup is made and has the project's files; loading asks first when there is unsaved work; live editing starts (LIVE 1); the places include the card; the card is listed; the files menu has the tag entries; the card is indexed; a search by tags finds the kick | `29`-`34` |
| UC12 | Save | the project is written unchanged: PADCONF.BIN, the patterns and every sample file are byte for byte those of the project that was opened | `35-saved` |
| UC13 | Quit | `q` quits; nothing was written to stderr as a crash | |

Together about 120 checks. A failed check prints the screen it saw.

## How it reads the screen

The test sends keys with `tmux send-keys` and reads the screen with `tmux capture-pane`. A check waits up to ten seconds for a text to
appear (`check`), or for a pattern (`checkre`), or says a text must not be there (`absent`). The colours of each step are kept
(`.ansi`) and turned into a picture by `ansipng`, so a change of look can be looked at after the run.

## What it does not cover

Real sound, a real unit, a real recorder or MIDI port; macOS and Windows (the program is built for them, but only Linux is run here);
the handheld (its screens are guarded by `go test ./core -run Handheld`, a picture of every screen that must not change).
