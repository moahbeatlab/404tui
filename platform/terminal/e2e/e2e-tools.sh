#!/bin/bash
# The advanced tools of the pattern and the slicer, end to end (use cases UC14-UC17, see docs/e2e.md). It adds to the results of
# e2e.sh when APPEND is set. Every tool is opened from its menu the way a user does: t, the page, the entry (found on the screen),
# Enter; the trial is read from the panel (the notes of the track, the slices), Keep it commits, Esc throws it away, u undoes.
. "$(dirname "$0")/e2e-lib.sh"

# tool PAGE-PRESSES "entry": open the tools menu of the screen, go to the page and run the entry
tool() { lit t; sleep 0.3; gopage "$1"; choose "$2"; }
# back to the tools menu with Esc and check that the trial is gone
back() { key Escape; sleep 0.3; }

uc UC14 "Pattern tools on a track: the trial of each tool, Esc throws it away, Keep commits, u undoes"
begin 150 44
lit 2
wait_for "TRACK" >/dev/null
sleep 0.3
PAD=$(cap | grep -oE "Pad +A[0-9]+" | head -1 | grep -oE "A[0-9]+$")   # the track in front (the first with notes)
check "a track of the pattern is in front" "Pad"
checknotes "the track $PAD starts with one note" 1
tool NOTES "Fill..."
check "Fill opens its dialog" "FILL $PAD"
checknotes "the trial is in the pattern: every beat is 8 notes" 8
snap 40-fill-trial
back
check "Esc goes back to the tools menu" "TIMING"
checknotes "Esc threw the trial away" 1
choose "Euclid..."
check "Euclid opens its dialog" "EUCLID ON $PAD"
checknotes "Euclid, 5 hits of 16 over two bars: 10 notes" 10
back
checknotes "Esc threw it away" 1
choose "Echo..."
check "Echo opens its dialog" "ECHO ON $PAD"
checknotes "Echo: the note and 3 repeats" 4
back
checknotes "Esc threw it away" 1
choose "Cycle..."
check "Cycle opens its dialog" "CYCLE $PAD"
checknotes "Cycle: the first 12 steps repeated over 32 steps" 3
back
checknotes "Esc threw it away" 1
choose "Ratchet"
check "Ratchet opens its dialog" "RATCHET $PAD"
snap 41-ratchet
back
for pair in "Evolve|EVOLVE $PAD" "Accents|ACCENTS AND GHOSTS ON $PAD" "LFO on|LFO ON $PAD" "ENV:|ENV ON $PAD" "Chance...|CHANCE ON $PAD" "Randomize velocity|RANDOMIZE VELOCITY OF $PAD" "Shape (gate|SHAPE THE NOTES OF $PAD" "Nudge|NUDGE $PAD"; do
  choose "${pair%%|*}"
  check "${pair%%|*} opens its dialog" "${pair##*|}"
  back
  check "Esc goes back to the tools menu (${pair%%|*})" "TIMING"
done
snap 42-tools-notes-page
choose "Euclid..."
check "Euclid again" "EUCLID ON $PAD"
choose "Keep it"
checknotes "Keep it commits the 10 notes" 10
key Escape; key Escape
absent "Esc twice closes the menus" "TIMING"
checknotes "the notes stay" 10
lit u
check "u undoes the tool as one step" "undone: euclid"
checknotes "one note again" 1
lit R
checknotes "R redoes it" 10
lit u
tool TRACKS "Clear"
checknotes "Clear takes every note of the track off" 0
lit u
checknotes "u brings them back" 1
tool TRACKS "Copy to a pad..."
check "Copy to a pad asks for the pad" "COPY $PAD"
snap 43-copy-to-pad
key Escape; key Escape

uc UC15 "Patterns: length, templates, copies, the pad dialog, tempo"
begin 150 44
lit 2
check "pattern A1 is two bars" "2 bars"
tool TIMING "Double"
checkre "Double makes it four bars" "Length +4 bars"
tool TIMING "Halve"
checkre "Halve makes it two again" "Length +2 bars"
tool PATTERN "Tempo of bank A"
lit l
checkre "the tempo of the bank goes up by one" "BPM 125"
lit h
checkre "and back down" "BPM 124"
key Escape
tool PATTERN "New pattern from a template..."
check "the templates are listed" "NEW PATTERN FROM A TEMPLATE"
check "with their tempo" "House"
snap 44-templates
choose "House"
checkre "the template is a new pattern in the next free slot" "Slot +A2"
check "its tracks are named" "House: Kick"
snap 45-template-pattern
tool PATTERN "Copy to the next pattern"
check "the pattern is copied" "A3"
tool PATTERN "Use the pads of another bank..."
check "the banks are offered" "Bank B"
snap 46-use-pads-of-bank
key Escape; key Escape
tool PATTERN "Delete the pattern"
check "the pattern is deleted" "empty: A puts the first note"
snap 51-pattern-deleted
lit u
checkre "u brings the pattern back with its tracks" "Tracks +[1-9]"

uc UC16 "Slicer tools: cut, order, mangle, lane, results"
begin 150 44
lit l l
lit 5
screenis "the slicer opens on A3" SLICE
lit t
check "the cut page" "Cut in equal parts"
gopage CUT
choose "Cut in equal parts"
checkslices "cut into 8 equal parts" 8
snap 47-equal-parts
lit t
gopage CUT
choose "Cut at random places"
checkre "cut at random places" "[0-9]+ slices"
lit t
gopage CUT
choose "Cut on the grid"
checkre "cut on the grid" "[0-9]+ slices"
lit y a
check "auto-cut again: 16 slices at the onsets" "16 slices"
tool ORDER "Shuffle the order"
check "shuffle the order" "order:"
tool ORDER "Play order back to 1, 2, 3"
check "the order is back to 1, 2, 3" "order"
tool ORDER "Mangle"
check "mangle opens its dialog" "MANGLE"
snap 48-mangle
key Escape
check "Esc puts the slices back and goes to the tools" "Mangle the slices"
key Escape
checkslices "16 slices again" 16
tool ORDER "Edit the order lane"
check "the order lane opens" "carry"
snap 49-order-lane
lit y; lit b
absent "y b leaves the lane" "carry"
tool KEEP "Slices to pads and a pattern under them"
sleep 0.5
snap 50-slices-to-pads
lit 2
check "a pattern with the slices is under the pads" "PATTERN"
lit 1
checkre "the pads hold the slices" "[0-9]\.[0-9]+ s"
snap 52-pads-with-slices

uc UC17 "Slicer settings of one slice, undo of every change"
begin 150 44
lit l l
lit 5
lit y a
checkslices "16 slices" 16
key Enter
check "Enter opens the slice settings" "SLICE SETTINGS"
choose "Pitch"
sleep 0.2
key Escape
check "Esc closes the settings" "16 slices"
lit b
checkslices "b removes the slice" 15
lit u
checkslices "u brings it back" 16
lit R
checkslices "R removes it again" 15
lit u
tool CUT "Cut in equal parts"
checkslices "equal parts" 8
lit u
checkslices "u takes the cut back to the 16 slices" 16
lit u
checkre "u again goes on back through the changes" "[0-9]+ slices"

finish
