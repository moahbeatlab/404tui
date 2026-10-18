# Looper

The Looper (`8`) records what the unit plays, for example a loop of an MC-101 that goes through it, and puts the
loop on a pad. It does what recording in the unit's looper, copying the loop to a pad and ungating it does by hand.

## Before

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
| `, .` | how many milliseconds the recording is behind the clock (0 to 500, 5 at a time) |
| `h j k l` | the pad in front |

## A take

1. Arm it. The recorder starts at once, so there is time to get ready; the next pad that holds no sample of the
   bank is the target (it is lit).
2. The loop begins on the next bar 1: a multiple of the loop's length on the clock, counted from the unit's Start.
   While it records the header shows a red `LOOPER REC 2/4` in every view.
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

USB audio arrives a little after the clock tick it belongs to. If loops start a bit late (the first hit is cut
off or comes early), change `, .` until a loop played from the pad lines up with the pattern.
