# TODO

## Record a tmux-based session on a real device

Goal: a replayable recording of 404tui driving a real SP-404MKII, to show the workflow
(index, assign a kit, patterns, bulk settings) and to check the app against the hardware.

Tools: `tmux` is installed. `asciinema` and `vhs` are not (`brew install asciinema vhs`).

Plan:

- [ ] **Record by hand with asciinema (the simplest start).**
  `tmux new -s rec -x 160 -y 50`, then inside it
  `asciinema rec -c "./sp404" docs/session-device.cast`. Press `q` in the app to end it.
  Replay with `asciinema play`, share with `asciinema upload`, make a GIF with `agg`
  (`brew install agg`). Keep the window at one fixed size so the replay does not wrap.
- [ ] **Script the same session with `tmux send-keys`** so it can be repeated:
  start `sp404` in a detached session, send keys with small pauses, and after each step
  save `tmux capture-pane -p -t rec` to `docs/session/NN-step.txt`. This gives text
  snapshots you can read and diff, with no video needed. Wait for the unit by polling the
  captured text for the thing you expect (for example "indexed" or "assigned"), not by
  fixed sleeps: the serial link is slow.
- [ ] **Optional: a `vhs` tape** (`docs/session.tape`: `Type`, `Enter`, `Sleep`, `Screenshot`)
  for a polished GIF or MP4 from the same steps.

Things to get right:

- Only one program can hold the serial port: close any other 404tui before recording.
- Use a spare project or a copy, and avoid destructive steps (erase project, delete,
  restore) in a recording that is meant to be shared.
- Clear names and paths that are private before publishing; `-log` writes a debug log
  that can show them too.
- The session cannot run in CI (it needs the unit). Keep a `-sim` version of the same
  script for tests: the same keys against the simulator.

## Make sure the whole application is SOLID

Goal: review the layers and keep them honest. The layering is `cmd/sp404` → `tui` → `app`
→ `formats` / `audio` / `device` → `proto`, with dependencies pointing down only.

- [ ] **Single responsibility.** `tui/keys.go` and `tui/view.go` have grown very large and
  hold many features each. Split them by feature (search, tags, patterns, pad detail,
  assign dialog) into their own files and types.
- [ ] **Open for extension.** Adding a key means editing a big `switch`. Look at a table of
  key bindings per view that the help, the bottom bar and the key guide are all built from.
- [ ] **Liskov / interfaces.** Players (`oto`, `afplay`) share `app.Player`, with looping as
  the optional `app.Looper`; check every optional interface this way and that fakes in the
  tests behave like the real thing.
- [ ] **Interface segregation.** `app.Unit` is wide. Keep each use case on the small
  interface it needs (as `Storage`, `Restorer`, `Eraser` already do), and stop `tui` from
  using the whole `Unit` where a part is enough.
- [ ] **Dependency inversion.** `tui` should not reach for `serialport`, `playback`
  or the file system directly: pass them in (the player and the store already are).
- [ ] **Shared state.** Background jobs read and write the tag index on other goroutines
  while the screen reads it. Make the rule explicit (work on a copy, swap the pointer on
  the UI goroutine) and run the tests with `-race` in CI.
- [ ] Add `go vet`, `staticcheck` and `go test -race ./...` to a CI workflow.

## Add a Makefile

- [ ] Targets for the everyday steps: `build` (the `sp404` binary), `test`
  (`go test -race ./...`), `vet`, `fmt` (gofmt check), `run` and `sim` (`./sp404 -sim`),
  `clean`.
- [ ] A `release` target that sets the version with
  `-ldflags "-X github.com/moahbeatlab/404tui/version.Version=…"`, and builds for macOS,
  Linux and Windows.

## Let the tag index grow without rewriting it

- [ ] `SD:404tui-index.json.gz` grows with the sample library and is written whole on every
  save, over a link of about 2 MB/s. Look at a format that can be updated in part (an
  append-only log of changes folded in now and then, or SQLite on the computer with only a
  copy on the card). The origins and folder notes are bounded (2560 and 160 entries) and can
  stay JSON.
