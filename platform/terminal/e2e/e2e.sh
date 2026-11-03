#!/bin/bash
# The end-to-end test of the PC program (sp404): it runs in a real terminal (tmux) in this container, against a copy of a real
# project (Construction Kits House 01) and its real samples, plays keys as a user does and checks what is on the screen.
# Every step is kept in /out as text (.txt) and with colours (.ansi); the use cases and their results go to /out/usecases.tsv.
#   sp404 in the PATH (/usr/local/bin/sp404 in the container)   PROJECT (/project) the real project, read only
#   WORK (/work) where it works   OUT (/out) the results   TMUX_SOCKET a private tmux socket (e2e)
set -u
S=t
W=${WORK:-/work}
OUT=${OUT:-/out}
PROJECT=${PROJECT:-/project}
SOCK=${TMUX_SOCKET:-e2e}
mkdir -p $W/samples/construction-kit $OUT
rm -rf $W/proj $W/out; cp -r "$PROJECT" $W/proj; mkdir -p $W/out
cp "$PROJECT"/SMPL/*.SMP $W/samples/construction-kit/ 2>/dev/null   # the real samples, for the file browser
fails=0; passes=0
UC=""; : > $OUT/usecases.tsv

tm() { tmux -L $SOCK "$@"; }
cap() { tm capture-pane -p -t $S; }
snap() { cap > $OUT/$1.txt; tm capture-pane -e -p -t $S > $OUT/$1.ansi; LASTSNAP=$1; }
key() { for k in "$@"; do tm send-keys -t $S "$k"; sleep 0.12; done; }
lit() { for k in "$@"; do tm send-keys -t $S -l "$k"; sleep 0.12; done; }
wait_for() { local n=$(( ${2:-10} * 10 )); for ((i=0;i<n;i++)); do cap | grep -qF -- "$1" && return 0; sleep 0.1; done; return 1; }
uc() { UC="$1"; UCTITLE="$2"; echo; echo "== $1 $2"; }
rec() { printf '%s\t%s\t%s\t%s\t%s\n' "$UC" "$UCTITLE" "$1" "$2" "${LASTSNAP:-}" >> $OUT/usecases.tsv; }
ok() { passes=$((passes+1)); echo "ok   $1"; rec pass "$1"; }
bad() { fails=$((fails+1)); echo "FAIL $1"; rec FAIL "$1"; cap | head -50; }
check() { if wait_for "$2"; then ok "$1"; else bad "$1: expected '$2'"; fi; }
checkre() { for ((i=0;i<60;i++)); do cap | grep -qE -- "$2" && { ok "$1"; return; }; sleep 0.1; done; bad "$1: expected /$2/"; }
absent() { if cap | grep -qF -- "$2"; then bad "$1: did not expect '$2'"; else ok "$1"; fi; }
start() { # start cols rows [extra args]
  local c=$1 r=$2; shift 2
  tm kill-session -t $S 2>/dev/null
  tm new-session -d -s $S -x $c -y $r "sp404 -audio silent -samples $W/samples -out $W/out $* 2>$W/stderr.txt; echo EXIT=\$? >$W/exit.txt; sleep 60"
  tm set -g status off
  rm -f $W/exit.txt
}
begin() { start "$@" -project $W/proj; wait_for "SAMPLE LAB" >/dev/null; key Space; wait_for "BPM 124" >/dev/null; }
screenis() { checkre "$1" "^ *404tui +$2"; }

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

uc UC03 "Go to a screen: 1-9, Tab, g h / g l, g g"
key 4
screenis "4 is SLICE (a pad with no sample bounces back)" PADS
key 2
screenis "2 is SEQ" SEQ
key 3
screenis "3 is FILES" FILES
key 9
screenis "9 is LOOP" LOOP
key Tab
screenis "Tab goes on, around to PADS" PADS
key BTab
screenis "Shift-Tab goes back to LOOP" LOOP
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
check "and the one-sample arp" "Arp from this one sample"
snap 09-pad-tools
key Escape
absent "Esc closes the tools" "Arp from this one sample"

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
lit ]
check "] turns to bar 2, numbered 17 to 32" "17"
lit '['
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
lit 4
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
lit 3
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
check "t opens the files menu" "Paste here"
snap 23-files-menu
key Escape

uc UC10 "Other screens: loop, perform, config, unit, help, recording"
lit 9
screenis "the looper" LOOP
snap 24-looper
lit 8
screenis "perform" PERF
check "no MIDI in this build says so" "no MIDI in this build"
snap 25-perform
lit 7
check "config" "BUSES BY PAD NAME"
lit a
check "config edits with a prompt" "WORDS FOR SAMPLES ON BUS 1"
snap 26-config-prompt
key Escape
absent "Esc closes the prompt" "WORDS FOR SAMPLES ON BUS 1"
lit 6
check "the unit screen without a unit" "NO UNIT"
snap 27-unit
lit '?'
check "the help page names the keys" "go to a screen"
snap 28-help
lit '?'
key C-r
checkre "Ctrl-R says why it cannot record" "no recorder|no audio input|cannot record"

uc UC11 "A unit (the simulator): its projects, a backup, live editing, its card, tags and the index of its files"
begin 150 44 -sim
lit 6
check "the simulated unit is found" "CONNECTED"
check "its projects are listed" "PROJECT_02"
lit a
check "the menu of a unit project" "UNIT PROJECT 1"
snap 29-unit-menu
lit j j j
lit a
check "a backup is made" "backed up to"
ls "$W"/backups/*/PADCONF.BIN >/dev/null 2>&1 && ok "the backup has the project's files" || bad "no backup folder"
lit a j; lit a
check "loading asks first when the editor has unsaved work" "NOT SAVED"
lit a
check "live editing starts" "LIVE 1"
snap 30-live
lit 3
check "the files screen" "╭"
lit y x
check "the places include the unit's card" "SD card"
snap 31-places
lit j j; key Enter
check "the card of the unit is listed" "IMPORT"
snap 32-unit-card
lit t
check "the files menu" "Paste here"
lit k
check "the tag entries are at the end of the menu" "Favourite"
snap 33-files-menu-tags
lit k k k k
key Enter
check "indexing the card" "samples indexed"
lit t
lit k k k k
key Enter
check "search by tags asks for words" "SEARCH THE TAGS"
for ch in k i c k; do lit $ch; done
key Enter
check "the tag search finds the kick" "tagged"
snap 34-tag-search
lit 3

uc UC12 "Save the project unchanged: the files are identical to the real project"
tm kill-session -t $S 2>/dev/null
begin 120 40
lit 5
check "the save screen" "WRITE THE PROJECT"
lit a
check "written" "WRITTEN"
snap 35-saved
dest=$(find $W/out -name PADCONF.BIN | head -1)
if [ -n "$dest" ] && cmp -s "$dest" "$PROJECT"/PADCONF.BIN; then ok "PADCONF.BIN is identical"; else bad "PADCONF.BIN differs or was not written ($dest)"; fi
d=$(dirname "$dest")
if diff -rq "$d/PTN" "$PROJECT"/PTN >/dev/null 2>&1; then ok "the patterns are identical"; else bad "the PTN files differ"; diff -rq "$d/PTN" "$PROJECT"/PTN | head -5; fi
n1=$(ls "$d/SMPL" 2>/dev/null | wc -l); n2=$(ls "$PROJECT"/SMPL | wc -l)
if [ "$n1" = "$n2" ]; then ok "all $n2 sample files written"; else bad "samples: $n1 written, $n2 in the project"; fi

uc UC13 "Quit, and no crash on the way"
lit q
sleep 1
if [ -f $W/exit.txt ] && grep -q "EXIT=0" $W/exit.txt; then ok "q quits (the project was saved)"; else bad "did not quit"; fi
if grep -qi "panic\|goroutine" $W/stderr.txt 2>/dev/null; then bad "a panic in stderr"; head -20 $W/stderr.txt; else ok "no panic"; fi

echo
echo "passed $passes, failed $fails"
cp $W/stderr.txt $OUT/ 2>/dev/null
[ $fails -eq 0 ]
