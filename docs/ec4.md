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

In the EC4 editor (or on the unit) make a group, name it `404tui`, and for each of the 16 encoders set its
**push** to send a **note**, on **channel 16**, with the note number of the table (60 for E1, 61 for E2 …
75 for E16), velocity 127. What the encoders do when turned is free: leave it, or use it for something
else on the unit, since 404tui does not read it for this map. Other settings, such as the LED ring, do
not matter for 404tui.

The EC4's output has to reach 404tui: either plug it into the SP-404MKII's MIDI input and let the unit
pass it on over USB (the unit sends what it receives on its MIDI input to the computer), or plug it into
the computer. 404tui listens to the input with `404` in its name (Config); see the MIDI monitor in
Config to check that the pushes arrive: a push should show `note on ch 16 64 …`.

The four messages of Config (live mode, recording, next becomes now, clear next) are the first, third,
fifteenth and sixteenth push by default; changing them there moves those four away from the map, the
rest of the table stays.

I have not tried this on an EC4: the menu names of the editor may differ from what is said above. The
notes and the channel are what 404tui reads.
