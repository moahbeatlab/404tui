# Faderfox EC4: one group for 404tui

Perform can be played from a controller as well as the mouse. This is a map for the Faderfox EC4: **one
group of 16 encoders, called `404tui`**, in which the push of each encoder does one thing. Print it with
`sp404 midimap`.

All pushes send a **Note On on channel 16** (the control channel of Config: change it there if you use
another). Notes 60 to 75, in the order of the encoders:

| | col 1 | col 2 | col 3 | col 4 |
| --- | --- | --- | --- | --- |
| **row 1** | E1 · 60 · record on / off | E2 · 61 · focus on / off | E3 · 62 · live mode | E4 · 63 · length 1, 2, 3, 4, 8 bars (each push the next) |
| **row 2** | E5 · 64 · BUS 1 slot 1 | E6 · 65 · BUS 1 slot 2 | E7 · 66 · BUS 1 slot 3 | E8 · 67 · BUS 1 slot 4 |
| **row 3** | E9 · 68 · BUS 2 slot 1 | E10 · 69 · BUS 2 slot 2 | E11 · 70 · BUS 2 slot 3 | E12 · 71 · BUS 2 slot 4 |
| **row 4** | E13 · 72 · clear BUS 1 motion | E14 · 73 · clear BUS 2 motion | E15 · 74 · next pattern becomes now | E16 · 75 · clear next pattern |

What they do is what the buttons do (see [perform.md](perform.md)): the slot pushes record into an empty
slot from the next bar, play a slot that holds a motion, and stop the one that plays; *clear* empties the
slot of that bus tapped last (by a push or by the mouse). *Focus* is the switch of VIEW.

## Setting up the EC4

The EC4 sends a push as a **note with the same channel and number as the encoder's own command** (that is
how its *Push* setting **Note** works; the manual, "Push functions"). So each encoder is set once: what it
sends turned is the CC of its number, pushed the note of that number:

| encoder | turn sends | push sends |
| --- | --- | --- |
| E1 | CC 60 | Note 60 |
| E2 | CC 61 | Note 61 |
| … | … | … |
| E16 | CC 75 | Note 75 |

There are **two groups**, the same except for the channel: group 1 **`404t`**, channel **16**, for the
pushes of the table above (its turns are not used), and group 2 **`VOL`**, channel **15**, for the pad
volumes (below). On the EC4 you pick the group with the GROUP keys, which is the switch between the two.

In the EC4 (the numbers of the manual's *Edit mode* and *Setup mode*):

1. **Setup mode** (hold FUNC, press encoder 4): turn **encoder 16** (*Push functions*) to **Note**. Turn
   **encoder 2** (*Routing*) to what your cable needs: with the EC4 on USB to the computer, any that keeps USB
   out (e.g. `MMMU`, the factory one).
2. **Edit mode** (hold FUNC, press encoder 8). Choose group 1 (and name it `404t` in Name mode, 4 letters).
3. For the encoders of the group: **command type** (encoder 10) **CCr2** (relative 63/65); **channel**
   (encoder 4) **16**, with *Copy channel number to all encoders in the group* from its options menu (press
   encoder 4, hold encoder 4 while the bars run); **mode** (encoder 14) **Acc0** (no acceleration; Acc1 if
   you like to turn fast); **number** (encoder 7 or 8) of E1 **60**, then *Fill command numbers in the same
   group with ascending numbers* (press encoder 7 quickly, hold encoder 8 while the bars run): E1 is 60,
   E2 61 … E16 75.
4. Choose group 2 and do the same with channel **15**, named `VOL`. (Or copy the group: setup mode, Copy
   group, Paste group, and change only the channel.)
5. Leave edit mode the same way as you got in (the setup is saved by itself).

In Perform the notes arrive as `note on ch 16 64 …`, a turn as `cc ch 15 60 …`: the MIDI monitor in Config
shows both. The EC4's output has to reach 404tui: plug it into the computer by USB (404tui listens to the
input with `404` in its name; in Config change the part of the name to match the EC4, e.g. `EC4` or
`faderfox`), or plug it into the SP-404MKII's MIDI input and let the unit pass it on over USB.

The messages of Config (live mode, recording, next becomes now, clear next) are the first, third, fifteenth
and sixteenth push by default; changing them there moves those away from the map, the rest of the table
stays.

## Pad volumes: the second group

On **channel 15** (Config: *Pad volumes channel*, 1 to 16 or off; the number 17 does not exist in MIDI, and
15 is used by nothing else) the 16 encoders are the volumes of the 16 pads of the bank the unit is on. It
works at any time, in any view; Perform shows each pad's level as the bar under its name.

| | on the EC4 |
| --- | --- |
| **turn** encoder 1 to 16 | the level of pad 1 to 16, **relative** (CC 60 to 75: 64 is none, above up, below down, by the distance) |
| **push** E1 to E16 | mute that pad (level 0, shown with a × on the pad); a second push brings back the level it had; turning a muted pad starts from 0 |

Levels are written to the unit's pad volume itself (the same setting as Level in the pad settings), so they
stay when 404tui is closed.
