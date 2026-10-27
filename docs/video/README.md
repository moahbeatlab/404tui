# Videos of 404tui

Two kinds, both made from the real program and both repeatable from the files here:

1. **Clips**: one captioned 1080p clip per tab (Pads, Files, Slicer, Sequencer, Config, Perform, Looper). A script sends
   keys to the program running against the simulator, and the screen is turned into frames and joined into an MP4.
   For the docs and for checking that a view still looks and behaves as described.
2. **The presentation film**: about two minutes, 1920×1080, with music, that says what 404tui is, which problem it
   solves, and shows each part with zoom-ins and captions. Written in HTML and rendered with
   [HyperFrames](https://github.com/heygen-com/hyperframes); it plays the clips from 1.

Neither needs a unit. The clips show the simulator's screens and the program's editing, not sound (the program's
playback is the computer's own audio, which a recording of the screen does not carry) and not a real SP-404MKII.

## What you need

`go`, `python3` with Pillow (`pip install pillow`), `tmux`, `ffmpeg`, and for the film Node 22 or newer. The fonts are
looked up by `docs/img/render.py`: DejaVu Sans Mono for the text and FreeMono or DejaVu Sans for the **Braille**
characters. The Slicer draws its waveform in Braille; a font without it shows hollow boxes, and `record.py` stops
with a message instead of recording that.

## 1. Clips

    go build -o /tmp/sp404 ./cmd/sp404
    python3 docs/video/record.py --list
    python3 docs/video/record.py --all --break drum_break.wav       # about a minute of work per clip
    python3 docs/video/record.py slicer pads                        # or just some
    python3 docs/video/timeline.py slicer                           # when each step happens (for the film's beats)
    python3 docs/video/review.py /tmp/404clips                      # a page to watch them: /tmp/404clips/review/index.html

`--break` is a WAV of a drum break (the shots use Wikimedia Commons' *Drum beat.ogg*, converted:
`ffmpeg -i drum_beat.ogg -ar 44100 -ac 2 -c:a pcm_s16le drum_break.wav`). It is only needed the first time; it is put in
a folder `/tmp/404lib` (`--lib` to change) by `docs/video/mklib` and used by the clips that run `-offline`.

What `record.py` does, so that it can be fixed when it breaks:

- starts the program in **its own tmux server** (`tmux -L clips`, never the default one: a plain `tmux kill-server`
  has ended a working session before) with a **throwaway config folder** (`XDG_CONFIG_HOME`). The Slicer keeps the slices of
  every sample it has opened in `slices.json` and restores them, so a second take would not start from the 8 default
  slices, and the recording must not write into your real `~/.config/404tui`.
- sends the clip's keys on its timeline and, meanwhile, stores the colour screen (`tmux capture-pane -e`) with its time. It
  records first and renders afterwards: rendering during the recording slows it, and then the clip plays faster than
  it was typed.
- renders a frame every 1/15 s: the terminal at 1920×982 and a 98 px caption strip under it: the title typed out, a line of
  explanation, and the key that was pressed as a badge.

### A clip is a file in `clips/`

    CMD = '{bin} -sim'                 # {bin} and {lib} are filled in; -sim is the simulated unit, -offline {lib} a folder
    HOME = '/home/user'                # optional: so paths on screen do not show your user name
    OFFLINE = True                     # optional: this clip needs the --break folder
    SETUP = [('2', 2.5)]               # keys before the recording starts (key, seconds); ('wait', regex) waits for the screen
    STEPS = [                          # (key or None, seconds until the next step, caption "Title|line under it")
        (None, 3.0, 'Files|The SD card in three columns...'),
        ('Tab', 2.0, 'Tab  Go into a folder|IMPORT opens...'),
    ]

Keys are single characters or `Space Enter Tab Escape Left Right Up Down`. An empty caption keeps the one before.

### When a feature is added or changed

1. Change or add the clip file for that tab: the keys, and **captions that say only what the screen then shows**.
2. Record just that clip: `python3 docs/video/record.py <name> --frames`, which also keeps the frames next to the MP4.
3. **Look at frames, not at the exit code.** Take a handful from across the clip (`ffmpeg -ss T -i x.mp4 -frames:v 1 t.png`) and
   check each caption against its frame. This is what has caught every mistake so far: a caption naming the wrong
   pad, a key that does nothing at its limit (`>` at velocity 127), a bank switch that had not happened yet, an Isolate
   switch that the simulator does not show.
4. Run `review.py` and read the clip through once.
5. If the tab's look changed, re-take its picture in `docs/img/` (see `docs/img/README.md`) too.

Things learned the hard way:

- `Esc` on a tab's main screen asks **"Quit 404tui?"**. Close a chooser or prompt with it, never the screen itself.
- Banks, pattern lists and the Slicer load **asynchronously**: give each such key 3 s before the next, and wait for the
  screen (`('wait', regex)`) when a setup needs it.
- The simulator's **This computer** source lists the machine's own disk. Do not show it.
- The simulator has no MIDI input, level or clock. Perform therefore shows the dashboard and its choosers, not hits.

## 2. The presentation film

The project is in `presentation/`. `build.py` writes `index.html` and `compositions/*.html` (one scene each); edit
`build.py`, not the generated files. The feature scenes are one template filled from `FEATURES`: which clip, which stretch of
it, its speed, and the **beats**: at a time of the clip, a key, a caption (about 6 words), and the part of the screen to
punch in on, as cells (`column, row, width, height` of the 140×45 terminal). The camera moves there, everything else is blurred and dimmed round
the part (a hairline edge marks it), the caption changes. Because beats are times of the clip, the captions stay in step with the footage whatever
the speed.

    mkdir -p /tmp/404film && cp -r docs/video/presentation/. /tmp/404film && cd /tmp/404film
    mkdir -p assets/clips && cp /tmp/404clips/*.mp4 assets/clips/        # the clips from part 1
    python3 music.py assets/music.wav 124                                 # a generated backing track, seeded, no samples
    python3 build.py                                                      # writes index.html and compositions/
    npx hyperframes check                                                 # lint, layout, contrast: fix what it says
    npx hyperframes snapshot --at 5,30,60                                 # look at frames before rendering
    npx hyperframes render -o 404tui.mp4                                  # 1080p30, a few minutes

`npx hyperframes telemetry disable` stops it sending anonymous usage data (it is on by default).

What `check` taught, and `build.py` now does:

- A caption needs **0.5 s plus 0.3 s a word** on screen. `build.py` prints `too quick` for a beat that gets less; slow
  the clip (`rate`), shorten the caption, or move the next beat.
- Text after any zoom has to stay at **28 px or more**: keep the part you punch in on at most about 100 columns wide.
- Pick the part so its edge falls between rows of text (rects are whole cells): a cut through a line of text looks like a mistake. Check the beats with `snapshot --at` before rendering.
- The first frame must show the title (it is the thumbnail), the first caption must not wait for an animation.
- A clip's speed has to leave room: a feature scene runs `(to - from) / rate` seconds.

### When a feature is added

Re-record its clip (part 1), add a scene to `FEATURES` (copy a neighbour: choose the stretch of the clip from its `STEPS`
times, then the beats), and if it changes the story, the `problem` and `what` scenes in `build.py`. Then `check`,
`snapshot` a frame per beat, render. Say in the closing scene's status line what is and is not verified (it says
*v0.1-alpha, tested against a simulated unit, not yet verified on real hardware*; change it when that stops being true).

## Files

| File | What |
| --- | --- |
| `record.py` | records clips |
| `clips/*.py` | one clip each: keys and captions |
| `mklib/main.go` | makes the folder the offline clips run on |
| `review.py` | a page to watch the clips |
| `timeline.py` | when each step of a clip happens |
| `presentation/` | the film: `build.py`, `music.py`, the HyperFrames project files |
| `../img/render.py` | colour screen to PNG with Braille; used by `record.py` for fonts |

The generated MP4s, the music and the clips copied into the film are not committed (they are large and can be made
again); publish them as a release or on the Pages site.
