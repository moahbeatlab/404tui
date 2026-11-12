#!/bin/bash
# The end-to-end test of the PC program, the use cases UC01-UC13 (see docs/e2e.md); e2e-tools.sh has the advanced tools.
. "$(dirname "$0")/e2e-lib.sh"

uc UC01 "Start: the splash, then the pads of the real project"
start 150 44 -project $W/proj
check "the splash shows" "SAMPLE LAB FOR THE SP-404MKII"
snap 01-splash
key Space
check "the pads screen opens with the project's tempo" "BPM 124"
check "pad A3 is loaded" "03_Loop1_124_HDK"
screenis "the title says PADS" PADS
snap 02-pads

uc UC02 "The panel is 30% of the width, on every size; a window that is too small says so"
for size in "80 30" "100 34" "120 40" "160 48" "200 56"; do
  set -- $size
  tm resize-window -t $S -x $1 -y $2; sleep 0.6
  line=$(cap | grep -m1 "BPM 124"); prefix=${line%%BPM*}; col=$(( ${#prefix} - 1 ))
  want=$(( $1 * 70 / 100 ))
  if [ "$col" -ge $((want-3)) ] && [ "$col" -le $((want+4)) ]; then ok "the panel starts at 70% of $1 columns (column $col)"; else bad "panel column $col, wanted about $want of $1"; fi
  snap 03-size-$1x$2
done
tm resize-window -t $S -x 70 -y 20; sleep 0.6
check "a window of 70x20 is too small" "needs a window of at least 80x30"
snap 04-too-small
tm resize-window -t $S -x 150 -y 44; sleep 0.6
check "back to the screen" "BPM 124"

uc UC03 "Go to a screen: 1-6, Tab, g h / g l, g g (PADS SEQ PROJECTS FILES SLICE CONFIG)"
key 5
screenis "5 is SLICE (a pad with no sample bounces back)" PADS
key 2
screenis "2 is SEQ" SEQ
key 3
screenis "3 is PROJECTS" PROJECTS
key 4
screenis "4 is FILES" FILES
key 6
screenis "6 is CONFIG" CONFIG
absent "there is no LOOP, PERF or SAVE in the screen map" "LOOP"
key Tab
screenis "Tab goes on, around to PADS" PADS
key BTab
screenis "Shift-Tab goes back to CONFIG" CONFIG
lit g; lit g
screenis "g g is the first screen" PADS
lit g; lit l
screenis "g l is the next screen" SEQ
lit g; lit h
screenis "g h is the one before" PADS

uc UC04 "Pads: the pad's information in the panel and on the tiles, the bus colours"
lit l l
check "the panel names the sample" "03_Loop1_124_HDK"
check "the sample group" "SAMPLE"
check "the size of the sample" "1451.6 KB"
check "the tempo of the sample" "124 BPM"
check "its bars" "4.0"
check "the routing group" "ROUTING"
check "the pad is on BUS 1" "BUS 1"
check "the tiles say how long the loops are" "4.0 bars"
check "the tiles show the switches" "G L O R S V"
snap 05-pad-info
lit j j
check "a drum pad is on BUS 2" "BUS 2"
check "the playback group" "PLAYBACK"
check "the sound group" "SOUND"
check "the envelope group" "ENVELOPE"
snap 06-pad-bus2
lit ']'
check "] goes to the next bank" "bank B"
lit '['
check "[ goes back to bank A" "bank A"

uc UC05 "Pad panel: settings, Esc closes (b does not), undo takes a change back"
lit a
check "a opens the pad panel on SOUND" "Level"
snap 07-pad-panel
lit b
check "b does not close the panel" "Level"
lit j l l
checkre "a change shows" "Pan.*R"
lit ']'
check "] turns to the SHAPE page" "Fixed velocity"
snap 08-pad-panel-shape
key Escape
absent "Esc closes the panel" "Fixed velocity"
lit u
check "u takes the change back" "undone"

uc UC06 "Pad tools: t opens the TOOLS page of the pad"
lit t
check "the tools page lists Slice" "Slice..."
check "and the arp tool" "Arp..."
snap 09-pad-tools
key Escape
absent "Esc closes the tools" "Arp..."

uc UC07 "Sequencer: two bars side by side, numbered steps, z, f, Enter, t, Esc, Space, x"
lit 2
check "the pattern A1 is open" "PATTERN A1"
check "a wide window shows both bars" "bars 1-2/2"
check "the steps are numbered through, to 32" "32"
check "the tracks of the pattern" "A15"
check "the panel tells about the track" "TRACK"
snap 10-seq-two-bars
lit z
check "z: one bar in view" "bar 1/2"
snap 11-seq-one-bar
lit '}'
check "} turns to bar 2, numbered 17 to 32" "17"
lit '{'
lit z
check "z: as many bars as fit" "bars 1-2/2"
lit f
check "f freezes the view" "FROZEN"
lit f
absent "f again lets it go" "FROZEN"
key Enter
check "Enter opens the note panel" "Esc close"
snap 12-note-panel
lit b
check "b does not close the note panel" "Esc close"
key Escape
absent "Esc closes it" "Esc close"
lit t
check "t opens the pattern tools" "TRACKS"
snap 13-pattern-tools
lit ']' ']'
check "the notes page has Euclid" "Euclid"
snap 14-pattern-tools-notes
lit j
key Enter
check "a tool opens its dialog" "EUCLID ON"
snap 15-euclid
key Escape
check "Esc goes back to the tools menu" "TIMING"
key Escape
absent "Esc again closes it" "TIMING"
lit m
check "m mutes the track" "is muted"
lit m
check "m again plays it" "plays"
lit s
check "s makes the track solo" "is solo"
lit s
check "s again plays it" "plays"
lit x
checkre "x hears the note without starting the pattern" "STOP"
key Space
check "Space plays the pattern" "PLAY"
key Space
check "Space again stops it" "STOP"
lit l a
check "a puts a note on an empty step" "Velocity"
check "of velocity 100" "100"
snap 16-seq-note
lit a
check "a again takes it off" "no note"

uc UC08 "Slicer: cut, settings, tools, zoom, new order, loop, remove, undo"
begin 150 44
lit l l
lit 5
screenis "the slicer opens on a pad with a sample" SLICE
lit y a
check "y a cuts at the onsets" "16 slices"
check "the panel tells about the slice" "1 of 16"
check "its start and end" "Start"
snap 17-slicer
lit +
checkre "+ zooms in" "zoom 2x"
lit -
checkre "- zooms out" "zoom 1x"
key Enter
check "Enter opens the slice settings" "SLICE SETTINGS"
snap 18-slice-settings
key Escape
absent "Esc closes it" "SLICE SETTINGS"
lit t
check "t opens the slice tools" "SLICE TOOLS"
snap 19-slice-tools
key Escape
lit n
check "n makes a new order at once and plays it in a loop" "in a loop"
lit x
check "x stops what sounds" "stopped"
lit .
check "the order plays in a loop" "in a loop"
lit .
check "playing it again stops it" "the order stopped"
lit n
check "a new order is heard at once" "in a loop"
lit n
check "and another, while it plays" "in a loop"
lit x
check "x stops it" "stopped"
lit b
check "b removes the slice" "15 slices"
lit u
check "u takes the removal back" "16 slices"
snap 20-slicer-undo

uc UC09 "Files: the ranger view, the one-line search, the files menu"
lit 4
check "the files screen has panes" "╭"
check "the real samples are listed" "construction-kit"
snap 21-files-ranger
lit l
check "l goes into the folder" "BANK1-03.SMP"
lit /
check "/ opens the one-line search" "SEARCH"
for ch in b a n k 4; do lit $ch; done
check "typed text filters" "BANK4"
check "the search line says how many files" "files"
snap 22-file-search
key Enter
check "Enter keeps the result" "search"
key Escape
absent "Esc ends the search" "search \""
lit t
check "t asks from where the files are loaded" "LOAD FILES FROM WHERE"
check "this computer is one of the places" "computer"
check "so is the samples folder" "samples"
snap 23-files-from-where
pick "Copy, cut, rename, delete, tags..."; key Enter
check "the file tools are one step on" "Paste here"
snap 23b-files-menu
key Escape; key Escape

uc UC10 "Other screens: config with the project and the prompt, offline projects, help, recording, the rename prompt"
lit 6
check "config starts with the project" "PROJECT"
check "it has the writing of the project" "Write it to the card folder"
check "and the autosave" "Autosave"
snap 24-config-project
lit j j j j j
lit a
check "config edits with a prompt" "WORDS FOR SAMPLES ON BUS 1"
check "the prompt is one line to type in" "Enter done"
absent "no keyboard of letters on the screen" "abcdefghij"
snap 26-config-prompt
for ch in x y; do lit $ch; done
check "typed text shows in the prompt" "_"
key Escape
absent "Esc closes the prompt" "WORDS FOR SAMPLES ON BUS 1"
lit 3
check "projects offline without a unit" "OFFLINE"
check "the folder's projects are tiles" "empty"
snap 27-unit
lit t
check "t offers online and offline" "Online or offline, and the offline folder..."
choose "Online or offline"
check "online and offline are offered" "PROJECTS: ONLINE OR OFFLINE"
check "and the offline folder" "Choose the folder of the offline projects"
key Escape; key Escape
lit '?'
check "the help page names the keys" "go to a screen"
snap 28-help
lit '?'
key C-r
checkre "Ctrl-R says why it cannot record" "no recorder|no audio input|cannot record|sound server"

uc UC18 "Typing a name: one line, no keyboard of letters; export of the bank; Space plays, x stops"
begin 150 44
lit l l
check "pad A3 is under the cursor" "03_Loop1_124_HDK"
lit t
choose "Rename..."
check "rename asks for the name" "RENAME A3"
absent "no keyboard of letters" "abcdefghij"
for ch in z z; do lit $ch; done
check "the typed text is in the line" "HDKzz_"
snap 36-rename
key Enter
check "Enter renames the pad" "03_Loop1_124_HDKzz"
lit u
check "u takes the name back" "undone"
key C-e
check "Ctrl-E asks where to export the bank" "EXPORT BANK A INTO THE FOLDER"
snap 37-export-bank
key Enter
check "the bank is exported" "pads exported to"
ls "$W"/PROJECT_01_bank_A/samples.json >/dev/null 2>&1 && ok "samples.json and the WAV files are in the folder" || bad "no export folder"
key Space
sleep 0.3
lit x
check "Space plays the pad and x stops it" "stopped"

uc UC11 "A unit (the simulator): found at the start, online or offline, level meter, its projects, a backup, live editing, its card, tags"
start 150 44 -sim -project $W/proj
check "the splash shows" "SAMPLE LAB FOR THE SP-404MKII"
check "the launcher finds the unit" "SP-404MKII found"
check "and offers online" "Unit, online"
check "and offline" "Offline card folder"
check "and a folder" "Open a folder"
snap 38-splash-unit
lit o
check "o works offline: the pads open" "BPM 124"
lit 3
check "PROJECTS says it is offline" "OFFLINE"
check "and that a unit is plugged in" "a unit is plugged in"
lit t
choose "Online or offline"
check "t offers to go online" "Online: the projects of the unit"
snap 39-online-offline
choose "Online: the projects of the unit"
check "the simulated unit is found" "ONLINE"
check "its projects are listed" "PROJECT_02"
check "the level meter is in the panel while a unit is there" "level:"
snap 40b-projects-online
lit a
check "the menu of a unit project" "UNIT PROJECT 1"
snap 29-unit-menu
choose "Back up: write a copy into a folder"
check "the backup asks for a folder" "BACK UP UNIT PROJECT"
snap 29b-backup-folder
key Enter
check "a backup is made, and it says where" "backed up to"
ls "$W"/PROJECT_01/PADCONF.BIN >/dev/null 2>&1 && ok "the backup has the project's files" || bad "no backup folder"
lit a
choose "Edit live"
check "loading asks first when the editor has unsaved work" "NOT SAVED"
lit a
check "live editing starts" "LIVE 1"
snap 30-live
lit 4
check "the files screen" "╭"
lit t
check "the places include the unit's card" "SD card"
snap 31-places
pick "SD card"; key Enter
check "the card of the unit is listed" "IMPORT"
snap 32-unit-card
lit t
choose "Copy, cut, rename, delete, tags"
check "the files menu" "Paste here"
check "the tag entries are at the end of the menu" "Favourite"
snap 33-files-menu-tags
choose "Index SD:"
check "indexing the card" "samples indexed"
lit t
choose "Copy, cut, rename, delete, tags"
choose "Search by tags"
check "search by tags asks for words" "SEARCH THE TAGS"
for ch in k i c k; do lit $ch; done
key Enter
check "the tag search finds the kick" "tagged"
snap 34-tag-search
lit 4

uc UC12 "Save the project unchanged: the files are identical to the real project"
tm kill-session -t $S 2>/dev/null
begin 120 40
lit 6
check "the writing of the project is in the config" "Write it to the card folder"
lit j
lit a
check "written" "written, "
snap 35-saved
dest=$(find $W/out -name PADCONF.BIN | head -1)
if [ -n "$dest" ] && cmp -s "$dest" "$PROJECT"/PADCONF.BIN; then ok "PADCONF.BIN is identical"; else bad "PADCONF.BIN differs or was not written ($dest)"; fi
d=$(dirname "$dest")
if diff -rq "$d/PTN" "$PROJECT"/PTN >/dev/null 2>&1; then ok "the patterns are identical"; else bad "the PTN files differ"; diff -rq "$d/PTN" "$PROJECT"/PTN | head -5; fi
n1=$(ls "$d/SMPL" 2>/dev/null | wc -l); n2=$(ls "$PROJECT"/SMPL | wc -l)
if [ "$n1" = "$n2" ]; then ok "all $n2 sample files written"; else bad "samples: $n1 written, $n2 in the project"; fi

lit 3
check "the written project is in the offline folder" "PROJECT_01"
lit a
check "its menu opens it in the editor" "Open it in the editor"
key Enter
check "it opens and the pads show" "BPM 124"

uc UC13 "Quit, and no crash on the way"
lit q
sleep 1
if [ -f $W/exit.txt ] && grep -q "EXIT=0" $W/exit.txt; then ok "q quits (the project was saved)"; else bad "did not quit"; fi
if grep -qi "panic\|goroutine" $W/stderr.txt 2>/dev/null; then bad "a panic in stderr"; head -20 $W/stderr.txt; else ok "no panic"; fi

finish
