#!/bin/bash
# Runs the lab (sp404 lab) in a terminal (tmux) in this container, against a copy of a real project, and checks what is
# on the screen after keys, as a user would see it. Every step is kept in /out as text (.txt) and with colours (.ansi).
#   sp404 in the PATH (/usr/local/bin/sp404 in the container)   PROJECT (/project) the real project, read only
#   WORK (/work) where it works   OUT (/out) the results   TMUX_SOCKET a private tmux socket (e2e)
set -u
S=t
W=${WORK:-/work}
OUT=${OUT:-/out}
PROJECT=${PROJECT:-/project}
SOCK=${TMUX_SOCKET:-e2e}
mkdir -p $W/samples $OUT
rm -rf $W/proj $W/out; cp -r "$PROJECT" $W/proj; mkdir -p $W/out
fails=0; passes=0

tm() { tmux -L $SOCK "$@"; }
cap() { tm capture-pane -p -t $S; }
snap() { cap > $OUT/$1.txt; tm capture-pane -e -p -t $S > $OUT/$1.ansi; }
key() { for k in "$@"; do tm send-keys -t $S "$k"; sleep 0.12; done; }
lit() { for k in "$@"; do tm send-keys -t $S -l "$k"; sleep 0.12; done; }
wait_for() { # wait_for "text" [seconds]
  local n=$(( ${2:-10} * 10 ))
  for ((i=0;i<n;i++)); do cap | grep -qF -- "$1" && return 0; sleep 0.1; done; return 1; }
ok() { passes=$((passes+1)); echo "ok   $1"; }
bad() { fails=$((fails+1)); echo "FAIL $1"; cap | head -50; }
check() { # check "name" "text" -- the screen shows the text (within 10 s)
  if wait_for "$2"; then ok "$1"; else bad "$1: expected '$2'"; fi; }
absent() { if cap | grep -qF -- "$2"; then bad "$1: did not expect '$2'"; else ok "$1"; fi; }
start() { # start cols rows [extra args]
  local c=$1 r=$2; shift 2
  tm kill-session -t $S 2>/dev/null
  tm new-session -d -s $S -x $c -y $r "sp404 lab -samples $W/samples -out $W/out $* 2>$W/stderr.txt; echo EXIT=\$? >$W/exit.txt; sleep 60"
  tm set -g status off
  rm -f $W/exit.txt
}

echo "== start, splash, size 120x40"
start 120 40 -project $W/proj
check "splash shows" "SAMPLE LAB FOR THE SP-404MKII"
snap 01-splash
key Space
check "pads screen" "BPM 124"
check "pad A3 loaded" "03_Lo"
snap 02-pads-120x40

echo "== the panel is 30% of the width, on every size"
for size in "80 30" "100 34" "120 40" "160 48" "200 56"; do
  set -- $size
  tm resize-window -t $S -x $1 -y $2; sleep 0.6
  line=$(cap | grep -m1 "BPM 124"); prefix=${line%%BPM*}; col=$(( ${#prefix} - 1 ))
  want=$(( $1 * 70 / 100 ))
  if [ "$col" -ge $((want-3)) ] && [ "$col" -le $((want+4)) ]; then ok "panel starts at 70% of $1 columns (column $col)"; else bad "panel column $col, wanted about $want of $1"; fi
  snap 03-size-$1x$2
done
tm resize-window -t $S -x 70 -y 20; sleep 0.6
check "too small says so" "needs a window of at least 80x30"
snap 04-too-small
tm resize-window -t $S -x 120 -y 40; sleep 0.6
check "back to the screen" "BPM 124"

echo "== vim keys and the buttons"
lit l l; sleep 0.3
check "l l moves to pad A3" "03_Loop1_124_HDK"
lit h; sleep 0.3
check "h moves left to A2" "A2"
lit j; sleep 0.3
check "j moves down to A6" "A6"
lit k l; sleep 0.3
lit a
check "a opens the pad panel" "Level"
snap 05-pad-panel
lit ]; lit ]
check "tools page has Slice" "Slice..."
check "tools page has the one-sample arp" "Arp from this one sample"
snap 05b-pad-tools

echo "== arp from one sample"
lit j j j
lit a
check "arp menu opens" "ARP FROM A3"
snap 06-one-sample-arp
lit b b b

echo "== slicer"
lit a; sleep 0.3; lit ]; lit ]; lit j; lit a
check "slicer screen" "ORDER"
lit y a
check "auto cut" "16 slices"
snap 07-slicer
lit y b
check "slice settings" "SLICE SETTINGS"
snap 08-slice-settings
lit ]
check "play tab: grid at the bank's BPM" "of bank A"
snap 09-slice-play-tab
lit b
lit s b
check "slice tools (select+b)" "SLICE TOOLS"
snap 10-slice-tools
lit b

echo "== sequencer"
lit y h; lit y h
check "sequencer screen" "PATTERN A1"
check "tracks of the pattern" "A15"
snap 11-seq
lit b
check "note panel" "Velocity"
snap 12-note-panel
lit b
lit y x
check "pad dialog for a track" "ADD A TRACK"
snap 13-add-track-dialog
lit b
lit s b
check "pattern tools (tabs)" "TRACKS"
snap 14-pattern-tools
lit b

echo "== files: type a search"
lit y l
check "files screen" "TARGET"
lit /
check "search keyboard" "SEARCH"
for ch in k i c k; do lit $ch; done
check "typed search" "kick"
snap 15-file-search
key Enter

echo "== undo and the help page"
lit '?'
check "help page" "arrows or h j k l"
snap 16-help
lit '?'

echo "== save the project unchanged: files identical to the real project"
tm kill-session -t $S 2>/dev/null
start 120 40 -project $W/proj
wait_for "SAMPLE LAB" >/dev/null; key Space; wait_for "BPM 124" >/dev/null
lit y h; lit y h
check "save screen" "WRITE THE PROJECT"
lit a
check "written" "WRITTEN"
snap 17-saved
dest=$(find $W/out -name PADCONF.BIN | head -1)
if [ -n "$dest" ] && cmp -s "$dest" "$PROJECT"/PADCONF.BIN; then ok "PADCONF.BIN is identical"; else bad "PADCONF.BIN differs or was not written ($dest)"; fi
d=$(dirname "$dest")
if diff -rq "$d/PTN" "$PROJECT"/PTN >/dev/null 2>&1; then ok "the patterns are identical"; else bad "the PTN files differ"; diff -rq "$d/PTN" "$PROJECT"/PTN | head -5; fi
n1=$(ls "$d/SMPL" 2>/dev/null | wc -l); n2=$(ls "$PROJECT"/SMPL | wc -l)
if [ "$n1" = "$n2" ]; then ok "all $n2 sample files written"; else bad "samples: $n1 written, $n2 in the project"; fi

echo "== quit"
lit q
sleep 1
if [ -f $W/exit.txt ] && grep -q "EXIT=0" $W/exit.txt; then ok "q quits (project was saved)"; else bad "did not quit"; fi
if grep -qi "panic\|goroutine" $W/stderr.txt $W/out/../lab.log 2>/dev/null; then bad "a panic in stderr"; cat $W/stderr.txt | head -20; else ok "no panic"; fi

echo
echo "passed $passes, failed $fails"
cp $W/stderr.txt $OUT/ 2>/dev/null
[ $fails -eq 0 ]
