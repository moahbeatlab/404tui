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

Open that on the phone. The key is made once and kept (so the link stays the same from one run to the
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
| a pad twice | makes it the only one on its bus (the others on that bus go dry); twice again brings them back |
| a pad while **FOCUS** is on | picks the pad: the picked ones keep their bus; FOCUS again puts everything back |

The tile shows the number, the name, the bus (a coloured box), the volume (the bar under the name),
the length in bars, and what the pattern does: **green** the pad was heard in the pattern that plays,
**tinted** it is in the pattern and not heard yet, an orange frame is the pad the unit has selected,
a red flash is a hit, **×** is muted.

**MIX** turns the pads into faders: drag up or down on a pad to set its volume (it is sent to the unit as
you drag), tap to mute or bring back.

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
