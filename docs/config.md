# Config (tab 6)

Settings of the program, kept in `state.json` in the user's config folder under `404tui`
(`~/Library/Application Support/404tui` on a Mac, `~/.config/404tui` on Linux, `%AppData%\404tui` on
Windows).

## Keys

| Key | What |
| --- | --- |
| `j k`, arrows | next / previous setting; `g` `G` first / last |
| `h l`, arrows | change the setting (colours, choices, on/off) |
| `Enter`, `Space` | edit a text setting in a prompt, switch an on/off, run "back to the defaults" |
| `c` | clear the MIDI monitor |
| `Ctrl-T` / `T` | next colour theme |

In a prompt: type, `Enter` keeps it, `Esc` cancels.

## Buses by pad name

Which pads go to **BUS 1** and **BUS 2** is decided from the pad's name: a word of the name
(`Kick_125`, `OHat`, `DrumLoop`, `14_Perc1`; a longer word matches by its start) found in a list of words.
Where a name has words of both buses, BUS 1 wins.

- *Words for samples on BUS 1 / BUS 2* — `Enter` edits the comma-separated list.
- *Colour of BUS 1 / BUS 2* — `h l` change; used on pads, in Perform and in the sequencer.
- *A name with none of the words* — no bus, BUS 1 or BUS 2.
- *Set the bus when a file is assigned* — on / off.
- *Back to the defaults* — `Enter`.

`B` in Pads applies the rule to the bank, the marked pads or the project. The lower part of the view
shows what the rule makes of the bank in front.

## Recorder

*Record into folder* (default `~/Music/404tui`) and *Input with this in its name* (default `404`: the
unit's USB audio). `Ctrl-R` anywhere records to a WAV (24-bit, 48 kHz, stereo) with `ffmpeg` (or `sox`
where there is no ffmpeg; Windows needs ffmpeg). The line under the settings says which tool was found.

## Pad codes on the unit

Pads hit in turn on the unit switch things on (read from the pad the unit has selected, or from MIDI
when it is open): *Live mode* (default `1 3 3 7`) and *Start and stop a recording* (default `1 16 16 16`).
Pad numbers 1–16 with spaces; `off` turns a code off. A code has to be played within four seconds,
in any bank. While the unit's sequencer runs and MIDI is open, the notes it plays are not read as pad
hits.

## MIDI

*Input with this in its name* (default `404`), *Control channel* (1–16 or `off`, default 16) and four
messages on that channel, `note 60` … or `cc 20` (a controller turned up past half), or `off`:

| Default | Does |
| --- | --- |
| `note 60` | live mode (the Perform tab and back) |
| `note 61` | start / stop a recording |
| `note 62` | the next pattern becomes the one that plays |
| `note 63` | clear the next pattern |

MIDI input is built in on every release build (macOS: CoreMIDI, Linux: ALSA, Windows: winmm, on Intel and ARM alike). A build from source needs `go build -tags midi` on macOS and Linux (cgo; on Linux the ALSA headers `libasound2-dev`); on Windows there is nothing to add. The line under the settings says whether it is listening.

### MIDI monitor

Counts per kind of message, the tempo from the clock, the controllers seen and the last messages, to
see what the unit and anything plugged into it really send (`c` clears). Run with `-v` for a debug log
that also records every non-clock message.

## Checking the installation

`sp404 check` (see the README) checks the computer, the program on a simulated unit and a plugged-in
unit.
