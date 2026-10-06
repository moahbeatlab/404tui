# 404tui

A command-line tool and (in progress) terminal UI for the Roland SP-404MKII, talking to
the unit over its USB serial link: projects, pads, samples and the files on its storage
and SD card.

This is a Go port of [SparkyMK2](https://github.com/TuscanPL/SparkyMK2), whose authors
reverse-engineered the protocol. The wire format follows the specification in that
project's `docs/re/`; comments here that mention `docs/re/` refer to it.

## Status

Early. The protocol and device session are ported and tested against a simulated device
in this repo. They have **not yet been verified against real hardware**. The terminal UI,
sample-format parsing and audio import are not ported yet.

## Usage

    go run ./cmd/sp404 status          # current project, selected pad, project names
    go run ./cmd/sp404 pads            # pads that hold a sample
    go run ./cmd/sp404 ls SD:IMPORT    # list a folder on the SD card
    go run ./cmd/sp404 -sim status     # the same, against the simulated device

Add `-v` to log the session, and `-port` to pick a serial port.

## Layout

| Package | Role |
|---|---|
| `proto`, `proto/septet`, `proto/fileapi` | Wire format, with no I/O |
| `device` | Session with the unit over a `Port`: transactions, events, files, import, render |
| `serialport` | Finds and opens the unit's USB serial port |
| `sim` | In-memory SP-404MKII, for tests and for working without the hardware |
| `cmd/sp404` | The command |

Tests: `go test ./...`

## Licence

GPL-3.0-or-later, as SparkyMK2. See `LICENSE`.

Not affiliated with or endorsed by Roland.
