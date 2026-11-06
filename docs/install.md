# Installing, checking and building

## Checking an installation

`sp404 check` checks the program on the computer it runs on, with or without a unit, on Linux, macOS and Windows alike (it is in every release archive, so nothing else has to be installed):

    sp404 check          # also checks a plugged-in unit (read-only)
    sp404 -sim check     # leave the unit out

It looks at the terminal, the settings and cache folders (that they can be written), the serial ports, the audio output, `ffmpeg`/`sox` for recording and the MIDI inputs; runs the doctor and the whole UI (every tab at three screen sizes, keys pressed, help opened) on a simulated unit; and, when an SP-404MKII is plugged in, the doctor on it. `ok` is fine, `WARN` is a part that cannot work here (no sound, no MIDI, no recorder) and `FAIL` is a problem; the exit code is 1 on a `FAIL`. From a clone: `make check` runs the tests, builds and then runs `sp404 check`.


## Build

    make                    # ./sp404 for this computer
    make dist               # every platform, into dist/
    make linux              # linux amd64 and arm64
    make windows            # windows amd64 and arm64 (.exe)
    make macos              # darwin amd64, arm64 and a universal binary
    make dist VERSION=0.2   # stamp a version into the binaries

Linux and Windows build without cgo from any computer. The macOS binaries need cgo (the serial
port library asks IOKit for the USB ports), so `make macos` runs on a Mac.


## Audio

Files are auditioned on the computer's default output. `sp404 beep` plays a test tone with each of the two playback methods in turn (and names the system's output device), `sp404 play <file>` plays a file,
and `-audio afplay` switches the UI to macOS's own player if the default one stays silent, to tell silence in the UI from a problem with the output
(for example the system sending sound to the SP-404 itself when it is connected as the
USB audio device). If there is no output, the UI says why when you try to listen.


## Before you run it on a unit

Check it in this order:

1. `sp404 ports` lists every serial port and why each is or is not taken for the unit
   (USB id `0582:02e7`). If the unit is plugged in and no line says `SP-404MKII`, discovery
   is the problem; `-port /dev/cu.…` skips it.
2. `sp404 doctor` runs read-only checks and compares what this program parses with what the
   unit says: the status poll, project settings and names, all 160 pad blocks, every
   sample file's size and header against its pad block, the file API on the unit and the SD
   card, `PADCONF.BIN`, and our pattern parser against the unit's own list of pads a
   pattern plays. It changes nothing; add `-preview` as an argument to also play a pad.
   Each line is `ok`, `skip` (something the project does not have) or `FAIL` with the
   disagreement spelled out.
3. `sp404 -v` (the UI) writes a debug log: every transaction with its timing, port
   discovery, connects and disconnects, every operation on the unit with its result, and the
   keys pressed. The UI owns the screen, so the log goes to a file (`debug.log` in the user
   cache folder, or `-log <file>`), and a `● debug log` chip shows in the status bar.
   `-trace` adds every message on the wire as hex. The first lines say which build,
   machine and flags the log came from.

If something fails, `sp404 -trace -log doctor.log doctor` (or the same flags for the UI)
produces what is needed to see where the implementation and the unit part ways.
