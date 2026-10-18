# Perform on a phone: the web app

404tui can serve Perform to a browser on the same network, so a phone or tablet is a second screen with
big buttons: the pads, the clock, the cue, the motion slots, focus and the pad volumes. It is a
responsive page (portrait and landscape, from a small phone to a tablet) that opens from a link; nothing
is installed on the phone.

## Start it

```
sp404 serve
```

runs 404tui **without a screen** (a small computer in the stage case, or a laptop with the lid down), talks
to the unit and its MIDI, and serves the page. It prints where to go:

```
web page for Perform: open one of these on a phone on this network:
  http://192.168.1.20:8404/?key=k7p2mdq4
```

Under the link it draws a QR code of the first one: scan it with the phone camera instead of typing
(`sp404 serve` only; with `-web` the TUI covers the terminal right away). The key is made once and kept (so the link stays the same from one run to the
next); the phone keeps it too. Ctrl-C stops the server and puts back what the program changed on the
unit (the buses of pads that focus made dry).

The terminal program can serve the page as well, while you use it: `sp404 -web :8404`. Its Perform tab
shows the address (`web: …`).

| Flag | |
| --- | --- |
| `-web :8404` | the address to serve on (`serve` uses `:8404` by itself; `127.0.0.1:8404` serves this computer only) |
| `-web-key KEY` | the key a browser has to give; `none` lets everyone on the network in |

The browser can also be pointed at the computer by hand: open the page from anywhere (it asks for an
address and a key if it was not opened from the link), or give it in the link: `?host=192.168.1.20:8404&key=…`.

## On the phone

*Landscape* (the way to play): the pads in the middle, STAGE on the left (bank, tempo, bar and beat,
what plays now and next, record, focus, mix, next → now, clear next), MOTION on the right. On a small
phone the side panel is one of two tabs, STAGE or MOTION.

*Portrait*: three tabs under the screen, PADS, MOTION and STAGE.

Everything reacts when the finger goes **down**, not when it comes up, and the page shows the round trip
to the program at the top right (a few milliseconds on a good network).

### Pads

| Tap | Does |
| --- | --- |
| a pad | mutes it (its volume goes to 0 on the unit; another tap brings back the level it had) |
| a pad, held (0.4 s) | it keeps its bus and the others on that bus go dry (the phone vibrates); hold more pads to keep them too; hold a kept pad again to let it go, the last one brings everything back |
| a pad while **FOCUS** is on | picks the pad: the picked ones keep their bus; FOCUS again puts everything back |

The tile shows the number, the name, the bus (a coloured box), the volume (the bar under the name),
the length in bars, and what the pattern does: **green** the pad was heard in the pattern that plays,
**tinted** it is in the pattern and not heard yet, an orange frame is the pad the unit has selected,
a red flash is a hit, **×** is muted.

**Swipe** up or down on a pad to change its volume from where it is (it is sent to the unit as you swipe). The level bar of a pad is at its top, and while you swipe a bubble above the pad shows the level (0 to 127), since the finger covers the pad.
A tap counts when the finger comes up, since until then it could still be a swipe. Zooming by double tap or pinch is switched off.

### The cue

Tap **NOW** or **NEXT** to choose the pattern of the bank that plays now or comes next (the unit does
not say which pattern runs, so it is told, as with `a` and `n` in the terminal). **NEXT ▸ NOW** makes the
next one the one that plays, **CLEAR NEXT** drops it.

### Motion

The same as MOTION in [perform.md](perform.md): the length in bars (1, 2, 3, 4, 8), four slots per bus
(tap an empty one to record from the next bar, a full one to play it, the playing one to stop it), and
**speed**, **reverse** and **clear** for the slot tapped last. **OVERDUB** adds the knobs you turn while a
slot plays. The slots draw the knob that moves most as a small curve, and a bar under it shows where the
loop is. Under BUS 1 and BUS 2 are the effect the unit reported and its three knobs.

Motion needs the unit's MIDI clock: start its sequencer.

## What it needs

* The computer runs 404tui (`sp404 serve` or the terminal program), with the unit plugged in by USB.
  MIDI (the clock, the pads you hit, the knobs) needs a build with MIDI: the release builds have it, and
  `make build` makes one; without it, the page shows the pads and takes the pad and volume commands.
* The phone and the computer are on the **same network**. A guest network that keeps devices apart
  (client isolation) will not do, and a firewall has to let port 8404 through (macOS and Windows ask the
  first time).
* The page keeps the screen of the phone on while it is open, when the browser allows it. "Add to home
  screen" opens it without the browser's bars.

## Safety

The connection is plain HTTP and WebSocket on your own network, protected by the key in the link: whoever
has the link can play the pads. It is meant for a stage or a studio network, not the internet; do not
forward the port. `-web 127.0.0.1:8404` keeps it on the computer, `-web-key none` turns the key off.

## For developers

The program sends its state as JSON over the WebSocket at `/ws` each time it changes (at most what the
network carries: a slow phone skips states), and takes commands as JSON: `{"a":"pad","i":3}`, `mute`,
`solo`, `level` (`i`, `v` 0–127), `focus`, `rec`, `motion` (`bus`, `slot`), `mclear`, `mspeed`, `mrev`
(`bus`), `bars` (`n`), `dub`, `cue` (`i`, `n` 1 now, 2 next), `nextnow`, `clearnext`. The code is in
`web/` (server and page) and `tui/web.go` (state and commands).

## Looper and Slicer

The **LOOPER** tab in portrait (the button at the top right in landscape) is the bank of loops that play together
([looper.md](looper.md)), for the phone: the 16 pads of a bank, and a pattern in which all 16 pads start on beat 1 and last
the whole pattern. The samples come from the unit's own looper (copy one to a pad), the Slicer or anywhere else; this page keeps
the bank and the pattern in order (the bank is chosen with the chip at the top left, which says LOOP there). The length of the pattern is the unit's: it is read from the pattern of the bank in which all 16 pads play, or else from how long the loops on the pads are (2 bars when neither is known). It has two pages, switched at the top: **LOOP** and **SLICER**.

| Button | Does |
| --- | --- |
| **MAKE PATTERN** | writes the pattern of the bank in which all 16 pads start on beat 1 (it says whether there is one) |
| a pad, then **DELETE** | deletes the sample of the pad (tap DELETE twice; asking again before it happens takes it back) |

While the unit's clock runs a delete (and a slice that is placed) waits for beat 1 of the pattern, so that the work on the
unit is not heard as a gap; a pad that waits is striped red, and the page says in how many beats it happens.

## Level meter

A stereo level meter (left and right, with the held peak as a white mark and a **CLIP** light that stays on for
five seconds after the sound reached full scale) sits on the stage panel. It is
there to see whether the sound is loud enough: it shows what the unit sends over USB all the time a browser is
open (on Linux the program listens itself, elsewhere ffmpeg or sox does), and the sound being recorded (marked **REC**) while Ctrl-R runs. The number is the loudest recent peak in dB below full scale: aim for the yellow, not the red.
If the computer cannot listen, the meter says why.

The bank chip at the top left of the page opens a list of the ten banks with the number of pads that hold a sample; choosing one moves the unit to that bank, as the bank keys do in the Pads view, and the Pads view shows it.

In the Looper and the Slicer the bank boxes (BANK, FROM, TO) can be tapped for a list of all ten banks, so a bank is chosen in one tap.


In the Slicer, holding a pad (0.5 s) mutes it, in the bank the grid shows (again brings it back); muted pads are striped red. A tap
chooses the pad. The Looper's and Slicer's settings (bank, bars, slices …) are no longer held up while the unit is busy reading; a command that
needs the unit says "busy (…)" and why, instead of doing nothing.

The bank chip at the top follows the page: on the Pads and Stage pages it is the unit's bank (BANK), in the Looper it is the bank the loops go to (LOOP), and on the Slicer page it is the bank a pad is taken from (FROM). A pad the Slicer puts on a pad of a bank starts muted (level 0); hold it in the Slicer, or mute it on the Pads page, to bring it in at the level it had.

The Slicer page has a **MANGLE** button: a list of dice that are rolled for every slice each time SLICE is pressed: DROP-OUTS (slices that stay silent), REVERSE (played backwards), PITCH (each slice up or down by up to 2, 5 or 12 semitones, at random), ROLLS (slices that stutter 2 to 8 times), HUMANIZE (loudness, and timing on a grid) and FADE (at the ends of every slice). The result always has the length of the original, so it still fits the pattern; a pitched slice keeps its place in the rhythm (a shorter one ends in silence, a longer one is cut).


## Patterns

The **PATTERNS** tab in portrait (the button at the top right in landscape goes Perform, Looper, Patterns) shows the 16 patterns of a bank, each with the number of pads it plays; empty slots are dark. The pattern that plays is **lit**. The program finds it from the pads the unit plays (the unit sends every note of a pattern over MIDI, and the patterns whose pads fit what is heard are looked for in all ten banks), and the page follows it to its bank when it changes; the chip at the top left (PTN there) chooses another bank, which stays until the pattern that plays changes.

Tapping a pattern makes the unit play it **next**: the program sends a MIDI Program Change on the channel of the bank (1 for A to 10 for J) with the number of the pattern, as 404jam does. The unit needs **PC Rx** switched on (and the MIDI mode B of its MIDI settings). The pattern sent **blinks** until it takes over: when the pattern that plays ends (its length counted from the unit's start or from when the last one took over; the length is read from the pattern file), or earlier when the notes heard say it plays already. The unit does not confirm anything, so this is the program's estimate.
