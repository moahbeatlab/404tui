"""Clip: the Config (tab 6). Captions are "title|line under it"; a step is (key, seconds until the next, caption)."""

CMD = '{bin} -sim'
HOME = '/home/user'   # so the paths on screen do not show a real user name
SETUP = [('6', 2.5)]   # keys before the recording starts, or ("wait", regex)

STEPS = [
    (None, 3.4, 'Config|Every setting of the program on one page, kept in state.json; the MIDI monitor is at the foot'),
    ('Enter', 3.2, 'Enter  Edit a text setting|The cursor is on the words that send a pad to BUS 1: a prompt opens to edit the list'),
    ('Escape', 2.0, 'Esc  Cancel|The prompt closes and nothing changes; Enter would keep an edit'),
    ('j', 1.6, 'j k  Next / previous setting|The cursor moves down the list'),
    ('j', 1.8, ''),
    ('l', 1.8, 'h l  Change a setting|Colours, choices and on/off switches change in place: here the colour of BUS 1'),
    ('l', 1.8, ''),
    ('h', 2.0, ''),
    ('j', 1.8, 'j k  Next / previous setting|On to the rule for names that match no word'),
    ('l', 2.2, 'h l  Change a setting|BUS 1, BUS 2 or no bus'),
    ('j', 1.6, ''),
    ('Space', 2.4, 'Space  Switch on / off|Whether a file assigned to a pad also sets its bus'),
    ('Space', 2.2, ''),
    ('G', 2.8, 'g G  First / last setting|Jumps to the end of the list: the MIDI settings'),
    ('g', 2.4, 'g  Back to the first|'),
    ('c', 2.4, 'c  Clear the MIDI monitor|Resets the counts of what the unit and any controller send'),
]
