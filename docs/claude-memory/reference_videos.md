---
name: reference-videos
description: "How to record captioned clips of every 404tui tab and make the presentation film (HyperFrames); when to redo it"
metadata:
  node_type: memory
  type: reference
---

Clips and the presentation film are made by scripts in `docs/video/` (see its README, which is the full recipe): `record.py <clip>` drives
the program against `-sim` in its own tmux server (`-L clips`) with a throwaway `XDG_CONFIG_HOME`, `clips/*.py` hold the keys and captions,
`review.py` makes a page to watch them, and `presentation/` is a HyperFrames project (`build.py`, `music.py`).

**When to use:** after a feature is added or a view changes, re-record that tab's clip (`record.py <name> --frames`), look at frames (not the exit code),
update the film's scene in `presentation/build.py`, `npx hyperframes check`, snapshot, render. Also re-take `docs/img/<tab>.png`.

**Pitfalls (each cost a take):** a font without Braille draws the Slicer waveform as boxes; `Esc` on a tab's main screen asks to quit; banks and
patterns load asynchronously (wait 3 s); the Slicer restores earlier slices from `slices.json`, so use a throwaway config; the sim's *This computer*
source lists the machine's disk; render the film with `--workers 1 --low-memory-mode` on a 4 GB machine; run `npx hyperframes telemetry disable`.
Related: [[feedback-tmux-private-socket]].
