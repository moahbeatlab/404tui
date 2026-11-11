#!/bin/bash
# Shared by the end-to-end scripts: the helpers that start the program in tmux, send keys and check the screen.
set -u
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
S=t
W=${WORK:-/work}
OUT=${OUT:-/out}
PROJECT=${PROJECT:-/project}
SOCK=${TMUX_SOCKET:-e2e}
mkdir -p $W/samples/construction-kit $OUT
rm -rf $W/proj $W/out; cp -r "$PROJECT" $W/proj; mkdir -p $W/out
cp "$PROJECT"/SMPL/*.SMP $W/samples/construction-kit/ 2>/dev/null   # the real samples, for the file browser
fails=0; passes=0
UC=""; [ -n "${APPEND:-}" ] || : > $OUT/usecases.tsv

tm() { tmux -L $SOCK "$@"; }
cap() { tm capture-pane -p -t $S; }
snap() { cap > $OUT/$1.txt; tm capture-pane -e -p -t $S > $OUT/$1.ansi; LASTSNAP=$1; }
key() { local k; for k in "$@"; do tm send-keys -t $S "$k"; sleep 0.12; done; }
lit() { local k; for k in "$@"; do tm send-keys -t $S -l "$k"; sleep 0.12; done; }
wait_for() { local n=$(( ${2:-10} * 10 )); for ((i=0;i<n;i++)); do cap | grep -qF -- "$1" && return 0; sleep 0.1; done; return 1; }
uc() { UC="$1"; UCTITLE="$2"; echo; echo "== $1 $2"; }
rec() { printf '%s\t%s\t%s\t%s\t%s\n' "$UC" "$UCTITLE" "$1" "$2" "${LASTSNAP:-}" >> $OUT/usecases.tsv; }
ok() { passes=$((passes+1)); echo "ok   $1"; rec pass "$1"; }
bad() { fails=$((fails+1)); echo "FAIL $1"; rec FAIL "$1"; cap | head -50; }
check() { if wait_for "$2"; then ok "$1"; else bad "$1: expected '$2'"; fi; }
checkre() { for ((i=0;i<60;i++)); do cap | grep -qE -- "$2" && { ok "$1"; return; }; sleep 0.1; done; bad "$1: expected /$2/"; }
absent() { if cap | grep -qF -- "$2"; then bad "$1: did not expect '$2'"; else ok "$1"; fi; }
start() { # start cols rows [extra args]
  local c=$1 r=$2 i; shift 2
  tm kill-session -t $S 2>/dev/null
  # the last session gone, the server quits: wait for it, or the new session goes down with it ("server exited unexpectedly")
  for ((i=0;i<30;i++)); do tm ls >/dev/null 2>&1 || break; sleep 0.1; done
  local cmd="sp404 -audio silent -samples $W/samples -out $W/out $* 2>$W/stderr.txt; echo EXIT=\$? >$W/exit.txt; sleep 60"
  tm new-session -d -s $S -x $c -y $r "$cmd" || { sleep 0.5; tm new-session -d -s $S -x $c -y $r "$cmd"; }
  tm set -g status off
  rm -f $W/exit.txt
}
begin() { start "$@" -project $W/proj; wait_for "SAMPLE LAB" >/dev/null; key Space; wait_for "BPM 124" >/dev/null; }
screenis() { checkre "$1" "^ *404tui +$2"; }


# pick "text": move down in the menu in front until the row with that text is the lit one (it is drawn on the amber ground).
pick() {
  local label="$1" n dir=j
  label=$(printf '%s' "$label" | tr -d ' ')
  for ((n=0;n<120;n++)); do
    sleep 0.15
    if tm capture-pane -e -p -t $S | python3 "$HERE/hl.py" | tr -d ' ' | grep -qF -- "$label"; then
      sleep 0.2   # the screen has settled: the same entry is still lit
      tm capture-pane -e -p -t $S | python3 "$HERE/hl.py" | tr -d ' ' | grep -qF -- "$label" && return 0
    fi
    [ $n -eq 45 ] && dir=k    # a menu does not wrap: when the entry is not below, look above
    key $dir
  done
  return 1
}
# gopage NAME: in a menu with pages, turn to the page of that name (a menu opens on the page it was left on)
gopage() {
  local want i
  want=$(printf '%s' "$1" | tr -d ' ')
  for ((i=0;i<6;i++)); do
    sleep 0.2
    tm capture-pane -e -p -t $S | python3 "$HERE/hl.py" | tr -d ' ' | grep -qx -- "$want" && return 0
    lit ']'
  done
  bad "could not turn to the page $1"; return 1
}
choose() { if pick "$1"; then key Enter; sleep 0.3; else bad "could not find the menu entry '$1'"; fi; }
# the number of notes of the track in front, from the panel
notes() { cap | grep -oE "Notes +[0-9]+" | head -1 | grep -oE "[0-9]+$"; }
checknotes() { # checknotes "name" N
  for ((i=0;i<40;i++)); do [ "$(notes)" = "$2" ] && { ok "$1"; return; }; sleep 0.1; done; bad "$1: expected $2 notes, saw $(notes)"; }
slices() { cap | grep -oE "[0-9]+ slices" | head -1 | grep -oE "^[0-9]+"; }
checkslices() { for ((i=0;i<40;i++)); do [ "$(slices)" = "$2" ] && { ok "$1"; return; }; sleep 0.1; done; bad "$1: expected $2 slices, saw $(slices)"; }
# seqrow A9: on the sequencer, move down to the track of that pad (the panel names the track in front)
seqrow() { local i; for ((i=0;i<14;i++)); do cap | grep -qE "Pad +$1( |\$)" && return 0; lit j; sleep 0.15; done; bad "could not get to the track $1"; return 1; }
finish() {
  echo; echo "passed $passes, failed $fails"
  cp $W/stderr.txt $OUT/ 2>/dev/null
  [ $fails -eq 0 ]
}
