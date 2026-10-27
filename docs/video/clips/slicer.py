"""Clip: the Slicer (tab 4). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -offline {lib}'
OFFLINE = True   # needs the folder made from --break
SETUP = [('2', 2), ('Tab', 3), ('Z', 0), ('wait', 'Slicer\\s+·\\s+drum_break')]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 2.6, 'Slicer|A drum break (8.8 s) opened with Z in the file view, cut into 8 equal slices'),
    ('t', 3.4, 't  Cut at the onsets|Looks for where each sound starts and moves the cuts there: 32 slices, every one on a hit'),
    ('l', 1.1, 'h l  Select a slice|The slice in front is lit; its start, end and length are shown under the waveform'),
    ('l', 1.1, 'h l  Select a slice|Move along to slice 4'),
    ('l', 1.3, 'h l  Select a slice|Everything below applies to this slice only'),
    ('r', 2.2, 'r  Reverse|Slice 4 now plays backwards (rev in its label)'),
    ('x', 2.2, 'x  Roll|The slice repeats itself in the time it has (x2); j k change the roll shape'),
    ('m', 2.2, 'm  Mute|The slice is silent but keeps its place in the order'),
    ('(', 1.0, '( )  Chance|How likely the slice is to play, from 100 % down to 0 %: each pass can differ'),
    ('(', 2.0, '( )  Chance|Here 100 % → 50 %, then lower'),
    ('>', 1.0, '< >  Pitch|Shifts the slice up or down in semitones'),
    ('>', 2.0, '< >  Pitch|+2 semitones now'),
    ('u', 1.0, 'u  Undo|Steps back through every edit (U redoes)'),
    ('u', 1.0, 'u  Undo|Pitch back to 0'),
    ('u', 1.0, 'u  Undo|Chance back'),
    ('u', 1.6, 'u  Undo|Mute back on, roll and reverse still to go'),
    ('S', 2.6, 'S  New seed|Rolls the dice again: random picks and chances come out differently'),
    ('Tab', 3.0, 'Tab  Order lane|The order the slices play in; h l pick, Enter moves one, so a break is re-sequenced'),
]
