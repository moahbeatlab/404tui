"""Clip: the Sequencer (tab 5). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -sim'
SETUP = [('1', 2)]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 2.4, 'Patterns|Each bank has 16 pattern slots, shown under the pads. Bank A has none yet'),
    (']', 2.4, "]  Next bank|Bank B holds a pattern in slot 2 (the sim's kick, snare and hat)"),
    ('Tab', 2.2, 'Tab  Pads ⇄ patterns|Moves the cursor from the pad grid down to the pattern panel'),
    ('Right', 2.2, '← →  Choose the slot|Slot 2 is the pattern with 3 pads'),
    ('Enter', 5.0, 'Enter  Open in the Sequencer|The pattern opens on tab 5: one lane per pad, one block per note'),
    ('l', 1.3, 'h j k l  Move the cursor|Step by step along the kick lane'),
    ('l', 1.3, 'h j k l  Move the cursor|Every step is a sixteenth'),
    ('l', 1.3, ''),
    ('x', 2.4, 'x  Note on / off|A new kick lands on this step: blocks are as tall as their velocity and as wide as they last'),
    ('+', 2.2, '+ -  Length|The note lasts one step longer'),
    ('<', 1.2, '< >  Velocity|Each press makes the note 10 softer (the block gets shorter); , . change it by one'),
    ('<', 2.2, ''),
    ('j', 1.8, "j k  Another pad's lane|Down to the snare"),
    ('x', 2.4, 'x  Note on / off|A snare hit added'),
    ('t', 2.8, 't  Tools for this pad|Swing, quantize, shift, reverse, humanize, double, extend, half. T applies them to the whole pattern'),
    ('Escape', 1.2, ''),
    ('u', 1.4, 'u  Undo|Steps back through every edit (R redoes)'),
    ('u', 1.4, ''),
    ('u', 1.6, ''),
    ('Space', 4.0, 'Space  Play the pattern|Plays it in a loop on this computer, with a playhead running along the lanes'),
    ('Space', 1.8, 'Space  Stop|Press it again to stop'),
]
