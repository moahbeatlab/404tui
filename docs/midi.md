# MIDI files: in and out

A track of a MIDI file (`.mid`) becomes a **pattern** of the project, and a pattern can be written as a MIDI file. It works in the terminal
(FILES and SEQ), on the phone page (TOOLS, PATTERN) and in the demo on GitHub Pages.

## A file as a pattern

**In the terminal:** open the folder with the file on FILES (`.mid` files are listed with the samples) and press `a` on it. **On the page:**
TOOLS, PATTERN, **MIDI FILE IN…**, and choose the file (from the computer, or from the phone). Both open the same form:

| | |
| --- | --- |
| **Part of the file** | A file holds tracks, and a track can hold more than one channel; each is a part: `1  Bassline  ch 1  8 notes  C2-C3`. The form starts on the part with the most notes that is not the drums. |
| **Make it** | *a melody: keys on one pad* or *drums: a pad for each drum*. A part on channel 10 is drums by itself. |
| **Melody onto pad** | The pad that plays the melody. It starts on the pad of the track in front on SEQ. |
| **Pad's own pitch is the note** | The note of the file that plays the pad's sample as it is. The notes around it play it higher or lower. It starts in the middle of the notes of the part, so that they fit. |
| **Into pattern** | The pattern that is written: the first free one by default. Choose one that has notes and they are replaced. |
| **Length in bars** | As the file (its length, rounded up to whole bars, in the time signature of the file), or 1 to 64. |
| **Tempo** | Keep the tempo of the project (or of the bank), or set the tempo of the file. Only there when the file has one. |

**Import the pattern** writes it, opens it, and says what it did: `A7: 8 notes, 2 bars on A1`, and if there is something to know, how many
notes were moved by octaves, left out, or start together on one pad.

### A melody

Every note goes to the one pad as a **chromatic note**: the pad's sample at the pitch of the note. A pattern note can be 12 semitones up or
12 down from the pad's own pitch and not more, so a line over more than two octaves is **moved by whole octaves** into that range (the notes
keep their name, and "moved by octaves" tells how many). To keep a line as it is, put the root where the line is: a bass sample recorded at C2
wants the note C2 as its own pitch.

A pad plays one voice: notes that start together on the pad (a chord) are all written, and the pad plays one of them. A chord is better as the
notes of the chord on pads of their own (import the part again, onto another pad, and delete the notes that are not wanted).

### Drums

Each note number goes to a pad as a plain note. The notes are read as the General MIDI drum map (36 kick, 38 snare, 39 clap, 42 and 44 closed hat, 46
open hat, toms, rides and crashes, congas, bongos, shaker; anything else is a perc), and the pad of each voice is found the way the pattern
templates find it: by the names of the samples (a pad called `kick`, `snare`...), then any pad with a sample that is not taken. Drums that
have no pad are left out, and the line says which pad plays what: `Kick A1, Snare A2, Closed hat A3`.

### What is kept

Notes: their time (the file's ticks are converted to the 480 of a pattern), length, and velocity. Not kept: controllers, pitch bend, program
changes, tempo changes after the first (the first tempo is offered), and a file with time code (SMPTE) instead of beats is refused. Time
signatures with 1 to 7 beats in the bar are written into the pattern (the same code that the program reads); others are written as the
bars of the file's length.

## A pattern as a file

SEQ, the pattern menu (`t`): **Export this pattern as a MIDI file** writes `pattern-A7.mid` to the folder of the exports (it says where).
On the page, **MIDI FILE OUT** saves the pattern that is in front. The file has a track for each pad (`A1`, `A14 keys`) and the tempo of the bank.
A plain note is the note of the unit's MIDI chart for its pad (36 for the first pad of a bank) on channel 10; a chromatic note is 60 plus its
semitones on channel 1. Read back with this page, the first is drums and the second a melody with the root 60.

## Unverified on the unit

The import writes patterns the same way the pattern templates do (the same records, the same trailer), and the notes with a pitch are written as the
unit writes the notes that are played on its chromatic keys. Not yet tried on a unit: a pattern of more than 8 bars, an odd time signature, and a
melody over a pad that is not in the bank of the pattern. Please tell what the unit makes of them.
