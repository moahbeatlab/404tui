"""Clip: the Looper (tab 8). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -sim'
SETUP = [('8', 2.5)]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 3.6, 'Looper|A bank of loops that play together; this view keeps the bank and its pattern in order. It does not record sound itself'),
    (']', 2.4, "[ ]  Bank|The simulator's samples are in bank A; B is empty, and the line above the pads says where the next loop would go"),
    ('[', 2.6, '[ ]  Back to bank A|Three samples on pads 1 to 3, and the next free pad is A4'),
    ('l', 1.8, 'h j k l  The pad in front|Over to pad 2, the snare'),
    ('p', 3.6, 'p  Make the pattern|One pattern in which all 16 pads start on beat 1 and last the whole length: a sample put on any pad plays from the next round'),
    ('x', 3.6, 'x  Remix|Cuts the pad in front into equal slices, shuffles them and puts the result on the next free pad, muted'),
    ('s', 2.6, 's  How many slices|Cycles 4, 8, 16 or 32 slices for the next remix'),
    ('s', 2.4, ''),
    ('l', 2.0, 'h j k l  Over to the new pad|'),
    ('l', 1.6, ''),
    ('d', 3.2, "d  Delete a pad's sample|Pressed twice. While the unit's clock runs the delete waits for beat 1, and a second d before then takes it back"),
    ('d', 3.0, 'd  Confirm|The sample is gone and its note stays in the pattern, silent'),
]
