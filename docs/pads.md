# Pads (tab 1)

The sixteen pads of a bank, the ten banks (A to J) of the project in front, and under the grid the
patterns of that bank. Everything that changes a pad is done here.

Press `?` in the view for the searchable list of its keys (the same list as `:`). Keys marked
*computer* play on this computer's audio output, not on the unit.

## Moving

| Key | What |
| --- | --- |
| `h j k l`, arrows | move on the grid |
| `[` `]` | previous / next bank (the rail on the left lists the banks) |
| `g` `G` | first / last |
| `Tab` | move between the pads and the patterns panel under them |
| `/` | find a pad by name |
| `1`–`7`, `Shift-Tab` | other tabs |
| mouse | click a pad, a bank or a tab; the wheel scrolls |

## Marking

Most keys work on the pad in front, or on every **marked** pad when some are marked.

| Key | What |
| --- | --- |
| `v` | mark / unmark the pad |
| `V` | mark every pad with a sample in the bank (again, or `Esc`, unmarks) |

## Hearing

| Key | What |
| --- | --- |
| `Enter` | preview the pad, on the unit or on the computer (see `o`) |
| `o` (`O`, `0`) | switch where pads preview: unit / this computer |
| `P` | play the pad once on this computer |
| `C` | **Chromatic** (computer): play the pad as notes. `a w s e d f …` are piano keys, `z` / `x` move the octave, `Space` / `Enter` play the note, `Esc` leaves |
| `Z` | the **Slicer** on this pad's sample (see [slicer.md](slicer.md)) |

## Samples

| Key | What |
| --- | --- |
| `a` | assign a file to the pad (opens the file dialog where you left it) |
| `n` | rename (with marked pads: add a prefix / suffix) |
| `d` | delete the sample (of every marked pad) |
| `c` | copy the pad (or the marked ones) to another pad, in any bank |
| `x`, then `p` / `s` | pick a pad up, then `p` moves it onto the pad in front (overwrites) or `s` swaps |
| `e` | export the bank: choose a folder, get WAV files and `samples.json` |
| `R` | restore pads from an export (`p` there also makes a pattern) |
| `K` | check the pad's sample on the unit against the file it came from |
| `F` | find where the samples came from (names and lengths matched against the file index) |
| `W` | find which folder each bank was filled from |
| `H` | set up pads that were never set up (pan centre, hold 100, time stretch 100 %) |

## Settings

| Key | What |
| --- | --- |
| `S` | the pad's settings dialog (level, pan, pitch, fine pitch, time stretch, BPM, play mode, gate, loop, BPM sync, vinyl, mute group, bus FX, attack, hold, release) |
| `,` `.` | level down / up (of the marked pads) |
| `t` | menu of switches for the pad: gate, loop, BPM sync, vinyl, one shot, fixed velocity, bus FX |
| `T` | the same switches on every pad of the bank |
| `b` | the bank's tempo and volume in a dialog |
| `<` `>` | bank tempo down / up by 1 BPM |
| `-` `+` | bank volume down / up |
| `B` | set the buses of the bank's pads (the marked ones; `p`: the whole project) from their names, as **Config** says |

**Settings dialog (`S`):** `j k` / `Tab` choose a setting, `h l` / `- +` / `Space` change it, `0` puts it
back to its default (a step stops on the default, e.g. the centre of the pan), `g` `G` first / last,
`Esc` / `Enter` closes. With marked pads it changes all of them.

**Switch menu (`t`):** `j k` choose, `Enter` / `Space` switch, a letter picks a switch directly,
`0` `1` `2` set the bus (off, BUS 1, BUS 2), `Esc` / `t` closes.

## Patterns of the bank

`Tab` goes to the patterns panel. Keys there: `Enter` opens a pattern (then see
[sequencer.md](sequencer.md)), `d` deletes, `v` marks, `c` / `x` pick up to copy / move and `p` puts
it in a slot (twice replaces), `P` plays it on the computer, `b` retargets it to another bank,
`N` makes a new pattern from a template. `m` mutes and `M` / `s` solos a pad in the pattern playing
on the computer.

## Everywhere

`Ctrl-R` records the unit's audio, `Ctrl-F` full screen, `Ctrl-T` / `T` next colour theme, `r` refresh
from the unit, `U` leave offline mode, `?` the keys, `q` quit.
