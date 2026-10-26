# Slicer (tab 4)

Cuts a sample into slices on a waveform, plays them back on this computer in an order you set or at
random, and renders what you like onto pads. Everything happens in the software; the unit's own chop
points are not used. The slices are kept per sample in `slices.json` in your config folder, so a
sample opens as you left it. The keys are listed at the bottom of the dialog, in groups, and `?` lists
them searchable.

**Open it:** `Z` on a pad (Pads) or on an audio file (Files), or tab `4` then `a`: `p` chooses a pad,
`f` a new sample from the file view. `Esc` closes it; it stays as it was when you come back.

![The Slicer on an empty tab: the view opens over the tab and says what `a` is for](img/slicer.png)

The screenshot is of the simulated device (`sp404 -sim`) with no sample loaded, so the waveform is
empty and the tab only names the way in (`a`). Open a sample and the slices, the waveform and the key
groups fill it.

## Keys by group

| Group | Keys |
| --- | --- |
| slices | `h l` select · `c` cut in two · `/` cut in parts (2, 3, 4, 6, 8) · `d` delete · `e` equal · `E` random · `t` onsets · `p` tap · `u` `U` undo / redo |
| edges | `[ ]` start · `{ }` end · `f` coarse (8 dots) · `z` snap to zero crossing on/off · `i` `I` zoom in / out · `D` clear, by hand |
| sound | `r` reverse · `x` roll · `j k` `;` `'` roll shape · `m` mute · `( )` chance · `< >` pitch · `- +` gain · `6 7` attack · `8 9` release · `X` plain |
| feel | `T` grid · `, .` tempo · `F` fit tempo to the sample · `C` cut on the grid · `w` swing · `W` humanize |
| play | `Space` play · `Enter` hear the slice · `P` pattern under the slices · `V` pattern bars · `M` mode · `S` new seed · `Tab` order lane |
| bounce | `b` to a pad · `K` stretch to bars · `s` `y` mark · `B` marked to pads · `n` `N` rounds · `v` takes · `A` add sample · `Esc` close |

`1`–`5` always change the view, so the envelope keys are `6 7 8 9`.

## Modes

The slicer has several modes; the group list at the bottom changes to show the keys of the active one.

- **Cut cursor** (`D`): takes every edge away (`u` brings them back) and shows a cursor. `h l` move a
  dot (eight with `f`), `H L` jump (a grid step when a grid is on, else 1/32 of the sample), `p` places
  an edge (at the playhead while the sample plays), `d` removes the slice the cursor is in, `Space`
  plays, `D` / `Esc` done.
- **Tap** (`p` with the cursor off): the sample plays and every `Space` cuts a slice there. A mark moves
  to the onset just before it (up to 100 ms back). `u` takes the last cut back, `p` stops.
- **Order lane** (`Tab`): the sequence the slices play in. `h l` move, `H L` carry a step, `j k` another
  slice, `a` repeat, `d` drop, `R` shuffle, `o` reset.
- **Mode** (`M`): *order* plays the lane over and over, *random* picks by chance as weight, *no repeat*
  never the same slice twice in a row, *shuffle* every slice once per round in a new order.
- **Pattern under the slices** (`P`): arrows / `h j k l` move, `[ ]` change the bank, `Enter` puts the
  pattern under the slices, `0` takes it away, `Esc` leaves things as they were. `V` limits it to one
  bar, two, or all.
- **Several samples** (`A`): the file browser opens; move to an audio file and press `Z` to join it
  (`1` takes a pad's sample, `Esc` gives up). Up to 8 samples; `u` takes the last added out.
- **Selection** (`s` marks the slice, `y` all or none): the sound and envelope keys then change every
  marked slice in one undoable step.

## Cutting

- Opens in 8 equal slices. `e` cuts 16, 32, 4, 8 … equal slices (the message says which is next), `E` the
  same number at random places, `t` at the onsets it finds (again for another sensitivity).
- **On the tempo** (`C`): one slice per step of the grid at the tempo you set (`T` the step, `, .` the
  BPM). The message says how many bars of 4/4 the sample is at that tempo: 8.00 fits, 7.4 means try
  another.
- Each slice has its own start and end; moving one moves nothing else (gaps and overlaps are allowed).

## Sound per slice

Reverse, mute, chance, pitch in semitones, gain; **envelope** attack and release up to 2 s, baked into
the audio; **roll** (`x`: 2, 3, 4, 6, 8 hits, then off) with a shape (`j k` quieter / louder, `; '` last
hit lower / higher up to 12 semitones).

## Feel and seed

Grid `T`: off, 1/4, 1/8, 1/16. Swing `w` (needs the grid): 54 % a little, 66 % triplet, 70 % a lot.
Humanize `W`: 25, 50, 100 %. The slicer rolls dice from a **seed** (`S` for a new one): the same seed
gives the same picks, so a bounce is repeatable.

## Playing and bouncing

`Space` plays; the next round is rendered while the last sounds, and edits are heard from the next
round. `b` renders what plays onto a pad for `n` rounds (1, 2, 4, 8); `N` sets the rounds that fit a
whole number of pattern loops. `K` puts the whole sample, stretched to whole bars, on a pad. `v` sets
takes (1, 2, 4, 8): `b` then renders that many onto free pads from the pad you choose (never replacing
a sample). `B` renders the marked slices, each as it sounds, onto free pads.
