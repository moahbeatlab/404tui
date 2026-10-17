# Perform (tab 7)

A dashboard for the stage. It is meant to be looked at from afar while you play the unit: the pads
and the beat on the screen, nothing you have to operate. It listens to the unit's MIDI (a Mac, Windows
or Linux release build; "no MIDI in this build" otherwise) and, for the level, to its USB audio.

**Open it:** tab `7`, or the live-mode pad code (default `1 3 3 7`) or control-channel note 60, which
also takes you back to where you were.

## Layout

Wide screens (110 columns and up): the pads of the bank the unit is on in the right 60 %, with their whole
names in plain text, hit pads lighting up by how hard they were hit (a stripe on the tile's edge is
its bus, the selected pad is tinted, a × marks a track inferred as muted); the left 40 % in titled
panels:

- **CLOCK**: bar and beat from the unit's MIDI clock, four big beat blocks, the tempo.
- **REC LEVEL / level**: a stereo meter from −60 to 0 dB (green, yellow, red near the top), the peak held
  for a moment and a **CLIP** light (full scale, above −0.3 dB; stays on for five seconds) meaning the sound
  is distorted: turn the unit's volume or pad levels down. With `ffmpeg` / `sox` it listens to the unit's
  audio input while Perform is open; during a `Ctrl-R` recording it reads the end of the WAV, and the header
  badge carries the peak. macOS may ask to let the terminal use the microphone.
- **BUS 1 / BUS 2**: ON or off, the effect's name, your note, and its knobs C1–C3 as tall faders in the bus
  colour (lit while you turn them). They come from the unit's MIDI: buses send on channel 1 and 2, **CC 83**
  is the effect, **CC 19** switches it (64 and up on), **CC 16–18** are the knobs.
- **CONTROLS**: every other controller seen as a fader with its value and `channel:number`.
- **KEYS**.

Narrower screens put the same panels one under the other.

## Keys

| Key | What |
| --- | --- |
| `p` | **player**: the pattern you said plays, drawn as the Sequencer does, with a ▼ playhead that follows the unit's MIDI clock |
| `s` | (player) the pattern starts now: the playhead goes back to its beginning |
| `v` | the patterns of the bank in front |
| `a`, `Enter` | choose the pattern that plays now |
| `n` | choose the next pattern |
| `Space` | the next pattern becomes the one that plays |
| `x` | clear the next pattern |
| `f` `F` | note what BUS 1 / BUS 2 is used for (the unit does not tell the effect by itself) |
| `Ctrl-R` | start / stop a recording |
| `Ctrl-F` | full screen |
| `?` | all keys |

The control channel does the same from a controller: notes 60–63 (live mode, recording, next becomes now,
clear next), see [config.md](config.md).

## What it works out by itself

- **Which pattern plays.** While the unit's sequencer runs it sends every note its pattern plays over MIDI
  (channel = the pad's bank). 404tui does not take those for pads hit by hand (so a pattern cannot trigger a
  pad code) and finds the pattern whose pads fit them, marked "heard". Choosing one by hand (`a`) stays
  while it fits.
- **Mutes on the unit.** The unit does not report them. A track with notes in the playing pattern that has
  not been heard for a whole round while others have is shown as muted.
- **Tempo and position** from the clock (24 ticks to a quarter note) and Start / Stop / Continue.

## Event log

While `Ctrl-R` records, everything above is also written beside the WAV as `name.events.jsonl`: a first
line `{"k":"start","version":1,…}`, then one JSON object per line with `t`, seconds from the start of the
recording. Kinds: `on` / `off` (channel, note, velocity, bank, pad), `cc`, `program`, `sysex`, `beat`
(with the tempo), `start` / `stop` / `continue`, `select` / `sound` (polled pad), `pattern`, `fx`. It is
the base for replaying a performance.

## Troubleshooting

Config's MIDI monitor shows what really arrives. Run `sp404 -v` (log in the user cache folder, e.g.
`~/Library/Caches/404tui/debug.log`; `-trace` adds raw bytes) and check the unit's MIDI sync setting if
no clock arrives.
