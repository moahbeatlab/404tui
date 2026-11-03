#!/usr/bin/env python3
"""Reads the screen as tmux captures it with colours (tmux capture-pane -e -p) and prints, one per row, the text that is lit: dark
letters on the amber ground. The end-to-end scripts use it to know which entry of a menu the cursor is on."""
import re, sys

AMBER = (255, 175, 0)
tok = re.compile(r'\x1b\[([0-9;]*)m|([^\x1b])')
for line in sys.stdin:
    bg = fg = None
    out = []
    for m in tok.finditer(line.rstrip('\n')):
        if m.group(1) is not None:
            p = [int(x) if x else 0 for x in m.group(1).split(';')]
            i = 0
            while i < len(p):
                if p[i] == 0:
                    bg = fg = None
                elif p[i] in (38, 48) and i + 4 < len(p) and p[i + 1] == 2:
                    c = tuple(p[i + 2:i + 5])
                    if p[i] == 38: fg = c
                    else: bg = c
                    i += 4
                elif p[i] in (38, 48) and i + 2 < len(p) and p[i + 1] == 5:
                    i += 2
                i += 1
        else:
            if bg == AMBER and fg is not None and sum(fg) < 100:
                out.append(m.group(2))
    s = ''.join(out).strip()
    if s:
        print(s)
