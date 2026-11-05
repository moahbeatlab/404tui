# What the unit tells the program, and what it does not

The SP-404MKII sends a good deal over USB, and some things it never says. This page is what has been found out by listening (the recorder's event log, and `sp404 probe`).
It was observed on one unit; the firmware version was not noted. Where it says "nothing was seen" that is what happened in the takes and probes tried, not a promise.

## Over MIDI (the USB MIDI port, `404` in its name)

| What happens on the unit | What it sends |
| --- | --- |
| a pad is hit | a note on, on the channel of the bank (channel 1 is bank A, 2 is B, and so on), and a note off |
| the sequencer plays | the clock (24 pulses per quarter note), `start`, `stop` |
| a pattern loops | a note on channel 13, note 48, at every loop start (not every pattern sends it) |
| `SUB PAD` + a pattern pad | a `stop` and a `start` within 10 to 40 ms of each other |
| a pattern is **changed** | **nothing was seen** when the new pattern is in the same bank (A1 to A2); when a pattern of **another bank** starts, status byte 3 changes (see below) |
| the sequencer **plays its notes** | **no pad notes**: only the loop marker above. A pad note in the log is a pad someone hit |
| a knob is turned | a control change (the bus effect knobs, with the effect's switch), as `cc` in the log |

What the unit does **take** over MIDI: a Program Change switches the pattern (the channel is the bank, the program the pattern, 0 to 15), checked with `sp404 probe pc`.

## Over the serial link (the file and control protocol)

The status request (`FE 66 00`) is answered with a status of 19 bytes. What is known of it:

| Byte | Meaning |
| --- | --- |
| 1 | the project |
| 3 | the bank: in **pad mode** it follows the bank buttons. In pattern mode, switching the bank in view **changes nothing**, but when a pattern of another bank **starts** (at the end of the loop that was playing) the byte takes that bank. So it follows the bank of the pattern that plays; the pattern within the bank is not in it. Seen once, with A to B |
| 5 | the selected pad (0 to 15): a pad press selects it, also in mute mode; a change of bank in pad mode sets it back to 0 |
| 8 to 11 | the frames played of the pad that sounds, all `FF` when none does |
| 12 | 0 when the sequencer is stopped, 4 when it plays |
| the others | did not change in the takes tried; they may hold more (the pattern?), which is what `probe watch` is for |

A pad hit **selects** the pad, so the recorder polls the status four times a second and writes `select`, `sound` and `status` to the event log.

What can be **sent**: a pad pressed and released (`PreviewStart`, `PreviewStop`; whether it layers over a playing pattern and goes through the bus effects is what `sp404 probe pad` is for), a pad's parameters (`SetParam`:
setting its level to 0 mutes it, which is how the program mutes), the project data (pads, patterns, project settings), and files.

**Mute.** Muting a pad on the unit (mute mode, pressing the pad) changed only byte 5, the selected pad. No MIDI was sent. Whether the mute is in the pad's data was not seen: the pad that was muted was not among the pads `probe watch` polled.

## So: which pattern plays?

Nothing the unit sends names the pattern that plays. The program therefore asks to be told (the web page's patterns, the Perform tab's pattern, `pattern from here...` in the replay), and knows it for
sure only when it sent the change itself (a Program Change, or a pattern started from the web page). The status gives the **bank** of the pattern that plays at the moments when it changes (byte 3), which could suggest a pattern marker with the bank filled in; the pattern within the bank has to be told. A guess from the loop length and the tempo is possible but not built. See [replay.md](replay.md#which-pattern-plays).

## `sp404 probe`

    sp404 probe pad A1 [seconds]          press a pad over USB (play a pattern first, to hear whether it layers and goes through the bus effects)
    sp404 probe pc A 2 [now|loop]         send a Program Change (bank A, pattern 2) now, or at the next loop start, and print what the unit sends for 12 seconds
    sp404 probe watch [seconds] [pad...]  poll the unit and print everything that changes

`probe watch` polls the status ten times a second (byte by byte, with what changed), the project settings and the pads you name once a second, and prints the MIDI the unit sends. Type a line
and press Enter to put a mark into the list ("pattern A2 pressed"), and it keeps a copy in `probe-watch-<time>.txt`. It is the tool to find out whether the unit says something when you do
something on it: start it, do the thing, and look at what moved. Useful runs: a pattern switch while it plays, a bank switch in pattern mode, muting and unmuting a pad on the unit.
