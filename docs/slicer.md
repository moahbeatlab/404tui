# SLICE (screen 5)

Cut a sample into slices and play them in an order. It opens on the sample of the pad under the cursor on PADS (a pad with no sample sends you back); `5` or the screen map goes there.

![SLICE](img/pc-slicer.png)

## What is on the screen

- **The waveform** of the sample with a marker at each cut, the slices numbered, the cursor on one slice, and under it the **order lane**: the order in which the slices play (1, 2, 3... or any other order).
- **The panel** tells about the slice under the cursor: its number, start, end, length, beats, pitch, gain, chance, reverse, envelope, roll, and where it plays in the order. The tempo and the zoom (`zoom 2x`) are in the panel too.

## What you do here

- **Cut.** `a` cuts at the cursor, `b` joins the slice with the next (removes the marker), `h l` move, `j k` move a marker. *Auto-cut* (`y a`) cuts at the onsets of the sound, up to a number of slices you choose (4 to 32) and with a sensitivity (low to high):
  on a drum break it finds the hits. The tools also cut in equal parts (2 to 32), at random places, on the grid of the bank's tempo (1/4 to 1/32), and *tap* (cut while the sample plays).
- **A loop out of a whole track.** `S` on the track in FILES opens it here **on no pad** (the top line says FILE; the pad in front gives the bank, where kept slices go). Only the loops you keep take pads. A track on a pad opens here as before.
  - **Listen from the cursor** (`s a`, Select+A on the handheld; or the tools, *Cut*): the track plays from the cursor and the cursor goes with it. `,` `.` skip 10 s back or forward, `a` cuts where it plays, `b` or `x` stops.
  - **Bars.** The tempo is read from the track's name (`track_125`, `124 BPM`). A sample of 8 s or more with no tempo in its name is **listened to**: its tempo (to 0.1 BPM, a whole number when it is one) and its beats are found, and bar 1 goes on its first beat; the message and the tools say how sure it is. The tools also *detect the tempo and bar 1* again, or set the tempo by hand (*Bars: the tempo of the track*). The detection is made for a steady beat (house, techno, a whole mix): on the 70 loops of a house kit at 124 BPM the loops and whole takes come out at 124, a clap or snare alone on 2 and 4 often at half (62). `s k` (Select+Up) puts bar 1 at the cursor; while the track plays it takes the onset just before the key (up to 200 ms). Then the cursor line says where it is (`bar 17.1`), dim lines mark the bars when zoomed in, and every cut goes onto the nearest bar line (*Bars: the cuts snap to them* turns that off). *Cut every ... bars from bar 1* cuts the whole track in 1 to 16 bars.
  - **A slice of 1, 2, 4, 8 or 16 bars:** `w` makes the slice under the cursor longer, `W` shorter (or the tools, *This slice: ... bars from its start*). Its end goes to the bar line that many bars after its start; the slices it then covers go, the one it reaches starts later. So: cut at the start of a loop, then `w` until it is the length you want.
  - **The length in bars:** the slice line says `8 bars ✓` when a slice is whole bars to a millisecond; `x` then plays it **in a loop**, so the seam is heard.
  - **Keep it:** *This slice to a pad...* (tools, *Keep*) puts the slice under the cursor on a pad you choose, named by its bar (`test_track_b17`), with loop on and the pad's BPM set to the track's.
- **Zoom.** `+` and `-` zoom in and out on the waveform.
- **Play.** `x` plays the slice under the cursor, `.` plays the whole order in a loop (and again stops it), `n` makes a **new order** at once and plays it, and another `n` another one, while it plays; `x` stops. `,` steps the grid.
- **Settings of a slice** (`Enter`, or `y b`): pitch, gain, reverse, fade in and out, roll (2 to 8 hits, louder or quieter, with its own pitch) and chance. A second page, *Play*, sets the order mode (in order, random, no repeat, shuffle), the grid, swing, humanize and the length.

![A slice's settings](img/pc-slice-settings.png)

- **The tools** (`t`) have pages: *Cut* (above), *Order* (mangle the slices with dice, edit the order lane step by step, shuffle the order, back to 1, 2, 3...) and *Keep* (the result to a pad, the slices to pads, in order or at random, the slices to pads **and a pattern under them** with 1 to 8 bars, add the sample of another pad
  or a sample from a file, take the last added sample out).

![The slice tools](img/pc-slice-tools.png)

- **Keep what you made.** `y x` sends the slices to pads: they land on consecutive pads from the one you choose, and the pad tiles show their lengths. With a pattern under them, SEQ then has the pattern that plays the slices.
- **Undo.** `u` takes back the last change (a removed slice, a cut in equal parts, a mangle...), `R` redoes it, and it goes on back.

## Keys

**On a computer:** `a` cut · `b` join · `x` play slice (whole bars: in a loop) · `s a` listen from the cursor (`, .` skip 10 s) · `s k` bar 1 here · `w W` the slice longer or shorter in bars · `.` play order · `,` grid step · `h l` move · `j k` marker · `t` tools · `n` new order · `+ -` zoom · `Enter` settings · `y a` auto cut · `y x` to pads · `y b` settings · `u` undo, `R` redo.

**On the handheld:** `A` cut · `B` join · `X` play slice · `Sel+A` listen from the cursor (`L2 R2` skip 10 s) · `Sel+Up` bar 1 here · `R2` play order · `L2` grid step · D-pad move and marker · `Y+A` auto cut · `Y+X` to pads · `Y+B` settings · `Sel+B` tools · `Y+^v` zoom.

Every setting and tool with its values: [tools.md](tools.md#slice). From a pad, `t` then *Slice...* opens the slicer on its sample ([pads.md](pads.md)).
