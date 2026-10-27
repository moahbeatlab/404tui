"""Clip: the Pads (tab 1). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -sim'
SETUP = [('1', 2)]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 2.8, "Pads|The sixteen pads of bank A, the ten banks on the left, the pad's details and the history on the right"),
    ('l', 1.6, 'h j k l  Move on the grid|Over to A2, the snare. The panel on the right follows: sample, playback, sound, routing, envelope'),
    ('l', 1.6, 'h j k l  Move on the grid|On to A3, the hat'),
    ('h', 1.6, 'h j k l  Move on the grid|And back to A2, the snare: the settings below are for this pad'),
    ('Enter', 2.0, 'Enter  Preview|Plays the pad on the unit, or on this computer (o switches); the history logs it'),
    ('S', 2.6, 'S  Settings|Level, pan, pitch, fine pitch, time stretch, BPM, play mode, gate, loop, vinyl, mute group, bus FX, attack, hold, release'),
    ('j', 1.4, 'j k  Choose a setting|Down to Pan'),
    ('l', 0.8, 'h l  Change it|Pan moves from hard left (L64) towards the centre'),
    ('l', 0.8, ''),
    ('l', 0.8, ''),
    ('l', 1.6, ''),
    ('0', 1.8, '0  Back to the default|Pan goes back to its default'),
    ('Escape', 1.6, 'Esc  Close|Every change is already on the pad'),
    ('t', 2.8, 't  Switches|Gate, loop, BPM sync, vinyl, one shot, fixed velocity, bus FX: a letter picks one'),
    ('Escape', 1.4, ''),
    ('v', 1.6, 'v  Mark pads|Most keys then act on every marked pad'),
    ('l', 1.0, ''),
    ('v', 1.6, ''),
    ('V', 2.0, 'V  Mark the whole bank|Every pad with a sample; Esc unmarks'),
    ('Escape', 1.4, ''),
    (']', 3.2, '] [  Banks|Bank B has one sample and the pattern in the panel below'),
    ('[', 2.6, 'Back to bank A|'),
]
