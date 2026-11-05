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
| a pattern is **changed** | **nothing**: in two runs, switching between patterns of one bank and between banks (A1, B1, C1, A1, B1) sent no MIDI and changed nothing in the status |
| the sequencer **plays its notes** | **no pad notes**: only the loop marker above. A pad note in the log is a pad someone hit |
| a knob is turned | a control change (the bus effect knobs, with the effect's switch), as `cc` in the log |

What the unit does **take** over MIDI: a Program Change switches the pattern (the channel is the bank, the program the pattern, 0 to 15), checked with `sp404 probe pc`.

## Over the serial link (the file and control protocol)

The status request (`FE 66 00`) is answered with a status of 19 bytes. What is known of it:

| Byte | Meaning |
| --- | --- |
| 1 | the project |
| 3 | the bank: in **pad mode** it follows the bank buttons. In pattern mode it does **not** follow the pattern that plays (one change of it was seen once, 14 s after a bank was chosen, and did not come back in the second run) |
| 5 | the selected pad (0 to 15): a pad press selects it, also in mute mode; a change of bank in pad mode sets it back to 0 |
| 8 to 11 | the frames played of the pad that sounds, all `FF` when none does |
| 12 | 0 when the sequencer is stopped, 4 when it plays |
| the others | did not change in the takes tried; they may hold more (the pattern?), which is what `probe watch` is for |

A pad hit **selects** the pad, so the recorder polls the status four times a second and writes `select`, `sound` and `status` to the event log.

What can be **sent**: a pad pressed and released (`PreviewStart`, `PreviewStop`; whether it layers over a playing pattern and goes through the bus effects is what `sp404 probe pad` is for), a pad's parameters (`SetParam`:
setting its level to 0 mutes it, which is how the program mutes), the project data (pads, patterns, project settings), and files.

**Mute.** Muting a pad on the unit (mute mode, pressing the pad) changed only byte 5, the selected pad. No MIDI was sent, and the pad's own data (the 327 bytes of its block, read once a second for the muted pad) did not change. So a mute on the unit is not visible from outside; the program only knows the mutes it makes itself (the pad's level set to 0).

## So: which pattern plays?

Nothing the unit sends names the pattern that plays. The program therefore asks to be told (the web page's patterns, the Perform tab's pattern, `pattern from here...` in the replay), and knows it for
sure only when it sent the change itself (a Program Change, or a pattern started from the web page). Nothing in the status, the pad data or the MIDI follows it, so the pattern has to be told. What is left is a guess from the sound: the loop length and the tempo, or the rhythm of the notes against the audio. That is not built. See [replay.md](replay.md#which-pattern-plays).

## `sp404 probe`

    sp404 probe pad A1 [seconds]          press a pad over USB (play a pattern first, to hear whether it layers and goes through the bus effects)
    sp404 probe pc A 2 [now|loop]         send a Program Change (bank A, pattern 2) now, or at the next loop start, and print what the unit sends for 12 seconds
    sp404 probe watch [seconds] [pad...]  poll the unit and print everything that changes

`probe watch` polls the status ten times a second (byte by byte, with what changed), the project settings and the pads you name once a second, and prints the MIDI the unit sends. Type a line
and press Enter to put a mark into the list ("pattern A2 pressed"), and it keeps a copy in `probe-watch-<time>.txt`. It is the tool to find out whether the unit says something when you do
something on it: start it, do the thing, and look at what moved. Useful runs: a pattern switch while it plays, a bank switch in pattern mode, muting and unmuting a pad on the unit.
