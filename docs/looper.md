# Looper

The Looper (`8`) records what the unit plays, for example a loop of an MC-101 that goes through it, and puts the
loop on a pad. It does what recording in the unit's looper, copying the loop to a pad and ungating it does by hand.

## Before

On Linux (PulseAudio or PipeWire) the program listens to the unit's USB audio itself and keeps the last 90 seconds
of it in memory, as long as the Looper, Perform or a browser is open. Nothing is started when you arm and nothing is
written to a file: a take is cut out of the memory by the clock, and the level meter reads the same sound. On other
systems the recorder program (ffmpeg or sox) does it, and a take then needs a moment to start before bar 1.

- The unit is a USB audio interface: the app records its audio the way `Ctrl-R` does (ffmpeg or sox must be
  installed; `Recording` in [config.md](config.md) tells which input).
- It follows the unit's **MIDI clock**: start the unit's sequencer, or the Looper says so.
- The recording is what the computer hears from the unit. Whatever else is playing is in it: mute the other pads
  before a take.

## The view

The 16 pads of the bank loops go to, a line saying what the Looper does, and the keys:

| Key | Does |
| --- | --- |
| `a`, `Enter` | arms a take; again (or `Esc`) cancels it |
| `b` | how long a loop is: 1, 2, 4 or 8 bars |
| `[` `]` | the bank the loops go to |
| `p` | writes the pattern of the bank in which all 16 pads start on beat 1 (in the first free pattern slot) |
| `d` | deletes the sample of the pad in front (press it twice) |
| `g` `G` | makes loops 1 dB quieter or louder (-24 to +24) |
| `n` | normalise: every loop is made as loud as it can be (peak -1 dB), the gain is ignored |
| `t` | a take starts on beat 1 of the next **bar** (the default) or on the next **beat** |
| `c` | calibrates the delay (see below) |
| `, .` | how many milliseconds the recording is behind the clock (0 to 500, 5 at a time) |
| `h j k l` | the pad in front |

## A take

1. Arm it. (With the memory there is nothing to wait for; with the recorder program it starts at once.) The next pad that holds no sample of the
   bank is the target (it is lit).
2. The loop begins on beat 1 of the next bar of the clock (`t` makes it the next beat), counted from the unit's Start. Arm on beat 2 and it starts one bar line later; the recorder only needs a third of a second to be ready. While it records the header shows a red `LOOPER REC 2/4` in every view.
3. When the bars are over the recording ends, the loop is cut out of it by the clock (so it is as long as the bars
   are, to the sample, with a fade of a millisecond at both ends), put on the pad, named `LOOP hh:mm:ss`, and the
   pad's **gate is turned off**. The temporary recording is deleted.
4. The pattern of the bank plays it from its next round on. Arm again for the next loop: it goes on the next free
   pad.

If the clock stops during a take it is dropped. If the take could not be cut (the recording began too late, for
example because it was armed right before bar 1) the message says so; nothing is put on a pad.

## Taking a loop out

`d` on the pad (twice) deletes its sample. The note in the pattern stays, but a note on an empty pad is silent,
and the pad is free for the next take. So the pattern never has to be edited while it plays.

## Delay

USB audio arrives a little after the clock tick it belongs to (and the recorder buffers some of it), so a loop
comes out late by a fixed time, usually tens of milliseconds. **Calibrate it once**: put a short, loud hit (a kick)
on beat 1 of a pattern on the unit, mute everything else, start the sequencer and press `c`. The app records one
bar from the next bar, finds the hit in it and sets the delay to how late it is. After that loops line up with the
clock; `, .` fine-tunes (the delay can be negative). The delay stays as it is when you start the program again.

A loop starts on a bar line (or, with `t`, on a beat), so what was playing there is the loop's first beat. The pad is
triggered by the pattern on its beat 1, so the bars of the pattern and of the clock should line up.

## Loudness

A loop is saved exactly as the computer hears it: without any change in level. After a take the message says how
loud it is ("peak -8.3 dB"). How loud that is depends on what the unit sends over USB (its main volume, the input
level, the effects) and on the input volume of the computer's own sound settings. If the loops are too quiet or too
loud next to the rest, set the **gain** (`g` `G`) once, or switch **normalise** on (`n`) to have every loop come out
at the same peak. The pad itself is set to level 127.

## Remix and the Slicer

`x` cuts the sample of the pad in front into equal slices, plays every slice once in a random order, and puts the
result on the next free pad of the bank, ungated. The loop keeps its length, so it fills the pattern as before. `s` sets
how many slices: 4, 8, 16 or 32.

The web page has a basic **SLICER** page (the switch LOOP / SLICER / SETTINGS in the Looper): pick a pad of any bank
(FROM BANK), then set the slices (2 to 32), equal or random cuts, the order (shuffle, in order, random picks), the
timing (the slices' own length, or a 1/4, 1/8 or 1/16 grid at the bank's tempo, with swing) and the bank the result goes
to. SLICE makes a new version on the next free pad of that bank each time it is pressed.
