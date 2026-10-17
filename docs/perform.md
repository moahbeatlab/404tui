# Perform (tab 7)

A dashboard for the stage. It is meant to be looked at from afar while you play the unit: the pads
and the beat on the screen, nothing you have to operate. It listens to the unit's MIDI (a Mac, Windows
or Linux release build; "no MIDI in this build" otherwise) and, for the level, to its USB audio.

**Open it:** tab `7`, or the live-mode pad code (default `1 3 3 7`) or control-channel note 60, which
also takes you back to where you were.

## Layout

Wide screens (110 columns and up): the pads of the bank the unit is on in the right 60 %, with their whole
names in plain text, hit pads lighting up by how hard they were hit (a stripe on the tile's edge is
its bus, the selected pad is tinted, a × marks a track inferred as muted); the left 40 % in titled
panels:

- **CLOCK**: bar and beat from the unit's MIDI clock, four big beat blocks, the tempo.
- **REC LEVEL / level**: a stereo meter from −60 to 0 dB (green, yellow, red near the top), the peak held
  for a moment and a **CLIP** light (full scale, above −0.3 dB; stays on for five seconds) meaning the sound
  is distorted: turn the unit's volume or pad levels down. With `ffmpeg` / `sox` it listens to the unit's
  audio input while Perform is open; during a `Ctrl-R` recording it reads the end of the WAV, and the header
  badge carries the peak. macOS may ask to let the terminal use the microphone.
- **BUS 1 / BUS 2**: ON or off, the effect's name, your note, and its knobs C1–C3 as tall faders in the bus
  colour (lit while you turn them). They come from the unit's MIDI: buses send on channel 1 and 2, **CC 83**
  is the effect, **CC 19** switches it (64 and up on), **CC 16–18** are the knobs.
- **CONTROLS**: every other controller seen as a fader with its value and `channel:number`.
- **KEYS**.

Narrower screens put the same panels one under the other.

## Keys

| Key | What |
| --- | --- |
| `a`, `Enter` | choose the pattern that plays now |
| `n` | choose the next pattern |
| `Space` | the next pattern becomes the one that plays |
| `x` | clear the next pattern |
| `i`, `Esc` | the ISOLATE switch on / everything back (the mouse does the same) |
| `f` `F` | note what BUS 1 / BUS 2 is used for (the unit does not tell the effect by itself) |
| `Ctrl-R` | start / stop a recording |
| `Ctrl-F` | full screen |
| `?` | all keys |

The control channel does the same from a controller: notes 60–63 (live mode, recording, next becomes now,
clear next), see [config.md](config.md).

## Mouse and touch

Perform is made to be used without the keyboard: everything below is a big button (as tall as the screen allows, 3 to 7 rows) or a pad to tap (a touch
screen sends taps as clicks). In the wide layout the left side has, from the top:

- **VIEW**: *● record* / *■ stop recording* and *full screen*.
- **ISOLATE**: a switch. See below.
- **SCENES**: the effects of each bus, saved and faded. See below.

## Isolating a pad

The unit has one effect on each bus; to hear what a bus does to one sound, the other pads on it have to
go dry.

- **Tap a pad** once: it is muted (a × on the tile). It mutes on this computer only; the unit has no mute
  to set from here.
- **Double tap a pad**: it alone keeps its own bus, every other pad on the same bus is set dry (bus off).
  The ISOLATE switch turns on and says which pad is alone. Double tap it again, or switch ISOLATE off,
  and every pad is back on the bus it had.
- **The ISOLATE switch**: switch it on, then tap pads to add them or take them out; each keeps its bus and
  the others on those buses go dry. Switch it off and everything is put back.

These are real changes to the pads' bus setting on the unit, saved in the project, until they are put
back (quitting puts them back first; if the program is killed or the unit unplugged, they stay as they
were set). A pad that goes to no bus has nothing to isolate.

## Scenes: fading each bus's effect from one state to another

Every bus has its own two scenes, **A** and **B**, and its own fade, so one bus can glide while the other
stays. Set the effect on the unit, tap *save A* on that bus; change it, tap *save B*. A scene holds
whether the effect is on, which effect it is and the three knobs C1–C3, all as the unit last said them over
MIDI, so touch each knob once first (a knob it never sent is left alone). The fade is sent back to the
unit as MIDI controllers (CC 16–18, 19 and 83 on channel 1 for BUS 1 and channel 2 for BUS 2), so the
unit's own knobs follow.

Per bus, in the SCENES panel:

| Button | What |
| --- | --- |
| *save A* / *save B* | save the effect as that scene |
| the bar (tap anywhere on it) | fade to that point, from A at the left to B at the right |
| *A* / *B* | jump to that scene |
| *glide →B* / *glide →A* | glide to the other scene over the number of bars set at the top, beginning on the next bar of the unit's clock (so the unit's sequencer must run); tap again to stop where it is |

*1 2 4 8* at the top is how long a glide takes. The knobs move smoothly; the effect and the on/off switch
change at the middle of the fade (an effect cannot be half another). A bus with only one scene saved is left
alone. It needs a MIDI output to the unit, found by the same name as the input (Config); where it cannot be
opened the panel says why.

## Motion: recording the knobs and looping them

The MOTION panel has a button for each bus. Tap it: it waits for the next bar of the unit's clock (so its
sequencer must run), then records everything you do to C1–C3 of that bus on the unit for the number of bars
set at the top of SCENES (*1 2 4 8*, the same setting as a glide), and then plays it back in a loop by
sending the same controllers to the unit. The loop begins where the knobs stood when the recording began.
Tap the button again to stop it; while it waits or records, a tap cancels. A bus with a motion is not moved
by its scenes, and its glide button asks you to stop the motion first. Each bus has its own motion, so one
can loop while you play with the other.

The motion is not saved: it is gone when the program ends. Turning a knob while a loop plays moves the
knob until the loop comes round to it again.

## Pad colours

A pad keeps a **green** tint while it is heard in the pattern that plays (within a round of the pattern); a
pad the pattern plays that has not been heard lately is **tinted** in the accent colour; a pad the pattern
does not play has no tint. What was heard more than a couple of bars ago does not count, so after a switch
to another pattern the tints follow it. A hit still flashes on top of that.

## What it works out by itself

- **Which pattern plays.** While the unit's sequencer runs it sends every note its pattern plays over MIDI
  (channel = the pad's bank). 404tui does not take those for pads hit by hand (so a pattern cannot trigger a
  pad code) and finds the pattern whose pads fit them, marked "heard". Choosing one by hand (`a`) stays
  while it fits.
- **Mutes on the unit.** The unit does not report them. A track with notes in the playing pattern that has
  not been heard for a whole round while others have is shown as muted.
- **Tempo and position** from the clock (24 ticks to a quarter note) and Start / Stop / Continue.

## Event log

While `Ctrl-R` records, everything above is also written beside the WAV as `name.events.jsonl`: a first
line `{"k":"start","version":1,…}`, then one JSON object per line with `t`, seconds from the start of the
recording. Kinds: `on` / `off` (channel, note, velocity, bank, pad), `cc`, `program`, `sysex`, `beat`
(with the tempo), `start` / `stop` / `continue`, `select` / `sound` (polled pad), `pattern`, `fx`. It is
the base for replaying a performance.

## Troubleshooting

Config's MIDI monitor shows what really arrives. Run `sp404 -v` (log in the user cache folder, e.g.
`~/Library/Caches/404tui/debug.log`; `-trace` adds raw bytes) and check the unit's MIDI sync setting if
no clock arrives.
