"""Clip: the Files (tab 2). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -sim'
SETUP = [('2', 2.5)]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 3.0, 'Files|The SD card in three columns, like a file manager: folders on the left, the folder in front in the middle, details on the right'),
    ('Tab', 2.0, 'Tab  Go into a folder|IMPORT opens: its folders and files appear in the next column'),
    ('Tab', 2.4, 'Tab  Go into a folder|Down into pack/, where two samples live'),
    ('j', 2.6, 'j k  Move|The details panel shows the file: length, rate, channels and its waveform'),
    ('Space', 3.0, 'Space  Listen|Plays the file on this computer, so you can hear it before it goes anywhere'),
    (']', 2.6, '[ ]  Target bank|Chooses the bank a file will be assigned to; the next free pad (B1 here) is shown on the right'),
    ('[', 2.2, '[ ]  Target bank|And back to bank A, whose next free pad is A4'),
    ('v', 2.8, 'v  Mark a file|Marked files go to the free pads in order when you press Enter; V marks the whole folder'),
    ('k', 2.2, 'j k  Mark more|Move up and mark bass.wav too'),
    ('v', 2.4, ''),
    ('u', 2.4, 'u  Unmark everything|Both marks are gone'),
    ('w', 3.2, 'w  Tree view|The same files as a tree of folders that open and close in place'),
    ('t', 3.2, "t  Change the source|Unit storage: the SP-404MKII's own card, read over USB (empty in the simulator)"),
    ('t', 0.12, ''),
    ('t', 2.4, 't  Back to the SD card|The source cycles SD card, unit, this computer'),
    ('/', 1.8, '/  Search|Finds audio files in every folder of the source by keyword'),
    ('b', 0.3, 'Typing narrows the list|'),
    ('a', 0.3, ''),
    ('s', 0.3, ''),
    ('s', 2.6, ''),
    ('Escape', 2.2, 'Esc  Close the search|'),
]
