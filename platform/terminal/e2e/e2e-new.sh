#!/bin/bash
# The features of 0.2.0, end to end (use cases UC19-UC23, see docs/e2e.md): the Chord and Arp tools of a pad with the progressions,
# auto play in FILES, the pattern generator and the variations, MIDI files out and back in, and the terminal cast. It adds to the
# results of the other scripts when APPEND is set.
. "$(dirname "$0")/e2e-lib.sh"

tool() { lit t; sleep 0.3; gopage "$1"; choose "$2"; }

uc UC19 "Chord tool of a pad: the chord, the progression box, a progression bounced onto free pads"
begin 150 44
lit l l
lit t
check "the pad tools offer the Chord tool" "Chord..."
check "and the Arp tool" "Arp..."
choose "Chord..."
check "Chord opens its tool on the pad" "CHORD FROM A3"
check "it says which note the sample is in" "Sample is in"
check "the notes of the chord are shown" "C E G"
check "Hold is off: Space plays it once" "off: once"
pick "Chord"; lit l
check "right on Chord makes it minor" "< minor >"
check "the notes follow" "C Eb G"
check "the progression rows are under it" "PROGRESSION (IN THE ROOT'S KEY)"
check "a progression has a name and its chords" "Epic anthem: Cm Ab Eb Bb"
check "and what it feels like" "euphoric, uplifting"
snap 60-chord-tool
pick "Progression"; key Enter
check "Enter on Progression opens the box of progressions" "PROGRESSIONS IN C"
check "minor ones are listed apart" "MINOR (ROOT IS THE MINOR KEY)"
check "and major ones" "MAJOR"
check "with their chords in the key" "Cm Ddim G Cm"
pick "Pop anthem"
check "the foot tells the feeling of the one at the cursor" "Feels: happy, euphoric"
snap 61-progression-box
key Enter
check "Enter takes it and goes back to the chord tool" "Pop anthem: C G Am F"
check "the Feels row follows" "happy, euphoric"
choose "Bounce the progression to free pads"
check "the bounce asks where the run of pads starts" "PUT THE 4 CHORDS FROM A PAD"
key Enter
check "after the bounce the chord tool comes back as it was" "CHORD FROM A3"
check "the first pad is named after its chord" "C_03_Loop1"
check "and the next ones too" "Am_03_Loop1"
snap 62-progression-bounced
key Escape; key Escape
absent "Esc closes the tools" "CHORD FROM A3"
check "the chords are on pads" "F_03_Loop1"
lit u
checkre "u takes the bounce back as one step" "undone"
sleep 0.3
absent "the chord pads are empty again" "Am_03_Loop1"

uc UC20 "Arp tool of a pad, and auto play in FILES"
begin 150 44
lit l l
lit t
choose "Arp..."
check "Arp opens its tool on the pad" "ARP FROM A3"
check "with its pages" "NOTES   PLAY   MAKE"
check "Hold is off: one pass" "off: one pass"
snap 63-arp-tool
key Escape; key Escape
lit 4
screenis "FILES" FILES
check "the hint names auto play" "auto play: off"
key C-a
check "Ctrl-A turns auto play on" "auto play: ON"
lit j k
check "the cursor moves with auto play on" "auto play: ON"
snap 64-auto-play
key C-a
check "Ctrl-A turns it off again" "auto play: off"

uc UC21 "The pattern generator: a trial of one pattern, Keep it, a variation of it"
begin 150 44
lit 2
tool PATTERN "New pattern from a template..."
gopage GENERATOR
check "the generator page lists its styles" "House  124 BPM"
check "and the tool for the kit" "Kit: which pad plays kick"
check "and the variations" "Variation of this pattern"
choose "House  124 BPM"
check "a style makes a trial" "GENERATE: HOUSE"
check "it can be rolled again" "Another roll of the dice"
check "voices can stay at the next roll" "WHAT STAYS AT THE NEXT ROLL"
checkre "the trial is in the next free pattern" "PATTERN A2"
checkre "with tracks" "Tracks +[1-9]"
snap 65-generator-trial
choose "Another roll of the dice"
check "a new roll stays in the same slot" "PATTERN A2"
choose "Keep it"
check "Keep it keeps it" "kept in A2"
key Escape
choose "Variation of this pattern"
check "a variation of the pattern in front" "VARIATION OF A2"
checkre "it goes into the next free pattern" "PATTERN A3"
snap 66-variation
choose "Keep it"
check "Keep it keeps the variation" "kept in A3"
key Escape; key Escape; key Escape
absent "Esc closes the menus" "VARIATION OF A2"
lit u
checkre "u takes the variation back" "undone"
sleep 0.3
checkre "the variation is gone: A3 is empty again" "PATTERN A3 +empty"

uc UC22 "MIDI files: a pattern goes out as a .mid file and comes back in as a pattern"
begin 150 44
lit 2
tool PATTERN "Export this pattern as a MIDI file"
checkre "the pattern is written as a .mid file" "written: .*pattern-A1\.mid"
lit 4
sleep 0.3
# FILES opens where it was left: up until the work folder (it holds exports/) is the middle column, then onto exports/
for i in 1 2 3 4 5; do cap | grep -q "╮╭─ work ─" && break; lit h; sleep 0.3; done
pick "exports/" || bad "no exports/ folder in the work folder"
lit l
check "the export folder holds the file" "pattern-A1.mid"
lit a
check "a on a .mid file opens the import" "MIDI FILE PATTERN-A1.MID"
check "it says which part of the file" "Part of the file"
check "and offers to hear it first" "Play it with the sample"
checkre "it goes into a free pattern" "Into pattern +A2"
snap 67-midi-import
choose "Import the pattern"
checkre "the file is a pattern again" "A2: [0-9]+ notes"
lit 2
checkre "SEQ shows the new pattern" "PATTERN A2"

uc UC23 "Terminal cast: Ctrl-T records the terminal program and stops again"
begin 150 44
key C-t
check "Ctrl-T starts a cast" "casting the terminal"
lit 2; lit 1
key C-t
check "Ctrl-T again saves it" "cast saved"
snap 68-cast-saved
ls ~/Music/404tui/*.events.jsonl >/dev/null 2>&1 && ok "the cast's events are a file" || bad "no events file of the cast in ~/Music/404tui"
lit q
sleep 1
if [ -s $W/stderr.txt ]; then bad "something was written to stderr"; cat $W/stderr.txt | head; else ok "nothing was written to stderr"; fi

finish
