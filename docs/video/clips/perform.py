"""Clip: the Perform (tab 7). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -sim'
SETUP = [('7', 2.5)]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 3.6, 'Perform|A dashboard for the stage, meant to be read from across the room while you play the unit'),
    (None, 3.4, 'Pads|The pads of the bank the unit is on, with whole names; hit pads light up by how hard they were hit, and a stripe on the tile shows its bus'),
    (None, 3.6, "Clock, level and buses|Left: bar and beat from the unit's MIDI clock, the output level with a clip light, and the two bus effects with their knobs C1 to C3"),
    ('a', 2.6, 'a  Choose the pattern that plays now|The pattern chooser opens over the dashboard'),
    (']', 3.0, "[ ]  Bank|Bank B holds the simulator's pattern, slot 2 with three pads; Enter would open it"),
    ('Escape', 2.2, 'Esc  Back|The chooser closes'),
    ('n', 3.0, 'n  Choose the next pattern|Queues what plays after this one; Space makes it the one playing, x clears it'),
    ('Escape', 2.2, ''),
    ('f', 3.0, 'f F  Note what a bus is for|The unit does not say which effect a bus runs, so you write it down here'),
    ('p', 0.25, 'Typing the note|'),
    ('l', 0.25, ''),
    ('a', 0.25, ''),
    ('t', 0.25, ''),
    ('e', 1.2, ''),
    ('Enter', 3.2, 'Enter  Keep it|The note shows under the bus name'),
]
