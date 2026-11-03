#!/bin/bash
# 404tui on an R36S under ArkOS: copy this file to the Ports folder of the games card (/roms/ports/, or
# /roms2/ports/ with a second card) and the program, sp404tui-arm64, to a folder 404tui/ beside it. Samples go in
# 404/samples on the games card; projects are written to 404/card, the folder to copy to the SP-404MKII's card.
#
# Only partly tried on a device: see logs.log in the 404tui folder (copied to /mnt/usbdrive/r36s when that exists).
# What the program prints when it dies without being able to log (a crash of the Go runtime) is in stderr.log.
DIR="$(dirname "$(readlink -f "$0")")/404tui"
# the games card: /roms2 when a second card is mounted there, else /roms (one card, as on the R36S)
if mountpoint -q /roms2 2>/dev/null; then GAMES=/roms2; else GAMES=/roms; fi
SAMPLES=$GAMES/404/samples
CARD=$GAMES/404/card
LOG="$DIR/logs.log"
mkdir -p "$DIR" 2>/dev/null
mkdir -p "$SAMPLES" "$CARD" || echo "launcher: cannot create $SAMPLES / $CARD" >>"$LOG"

# diag writes what the device looks like to logs.log, to paste back when something does not work.
diag() {
  echo "==== environment $(date '+%F %T')"
  echo "-- who: $(id) tty=$(tty 2>&1) script=$(readlink -f "$0")"
  echo "-- uname: $(uname -a)"
  grep -m1 -i -E 'PRETTY_NAME' /etc/os-release 2>/dev/null
  cat /etc/arkos-version /home/ark/.config/.update_info 2>/dev/null | head -3
  echo "-- uptime: $(cut -d' ' -f1 /proc/uptime) s, load $(cut -d' ' -f1-3 /proc/loadavg)"
  echo "-- cpu/mem:"; grep -m2 -i -E 'model name|hardware' /proc/cpuinfo; grep -E 'MemTotal|MemAvailable' /proc/meminfo
  echo "-- program:"; ls -l "$DIR/sp404tui-arm64" 2>&1; file "$DIR/sp404tui-arm64" 2>&1 | head -1
  echo "-- sudo without password: $(sudo -n true 2>&1 && echo yes)"
  echo "-- folders:"; ls -ld /roms /roms2 "$SAMPLES" "$CARD" 2>&1
  echo "   samples: $(ls "$SAMPLES" 2>/dev/null | wc -l) entries"
  df -h /roms /roms2 "$DIR" 2>&1 | sort -u
  echo "-- display:"
  ls -l /dev/fb* /dev/dri/* /dev/tty1 2>&1
  for f in /sys/class/graphics/fb0/*; do
    case "$f" in */virtual_size|*/bits_per_pixel|*/stride|*/rotate|*/modes|*/name|*/mode) echo "   $(basename "$f"): $(cat "$f" 2>&1)";; esac
  done
  fbset -i 2>&1 | head -20
  echo "-- earlier copies of this program and launcher (stale ones are stopped below):"
  pgrep -a -f '404tui-run|sp404tui-arm64|404tui.sh' 2>&1 | grep -v pgrep | head -8
  echo "-- memory:"; grep -E 'MemTotal|MemFree|MemAvailable|SwapTotal|SwapFree|Dirty|Writeback:' /proc/meminfo
  echo "-- biggest processes (rss kB):"; ps -eo rss,pid,args --sort=-rss 2>/dev/null | head -6 | cut -c1-110
  echo "-- who has the screen:"; pgrep -a -f 'emulationstation|retroarch|sdl|weston|Xorg' 2>&1 | head -5
  echo "-- input:"
  ls -l /dev/input/ 2>&1
  grep -E '^(N|H):' /proc/bus/input/devices 2>&1
  echo "-- audio:"; cat /proc/asound/cards 2>&1; ls -l /dev/snd 2>&1 | head -8
  echo "-- usb:"; lsusb 2>&1 | head -10; ls -l /dev/ttyACM* /dev/ttyUSB* 2>&1
  echo "-- usb role: $(cat /sys/class/udc/*/state 2>&1 | head -1) $(cat /sys/kernel/debug/usb/*/mode 2>&1 | head -1)"
  ls /sys/class/extcon 2>&1 | head -3
  echo "-- env: TERM=$TERM HOME=$HOME SDL_VIDEODRIVER=$SDL_VIDEODRIVER DISPLAY=$DISPLAY"
  echo "-- dmesg (fb, usb, drm, input):"; dmesg 2>/dev/null | grep -i -E 'fb0|drm|panel|dwc2|otg|usb.*(host|gadget)|joypad' | tail -15
  echo "==== end of environment"
}
diag >>"$LOG" 2>&1

# keep a copy of the logs on the usb drive, so they can be read on a computer. The folder is never created: when
# the drive is not there the logs just stay next to the program.
USB=/mnt/usbdrive/r36s
copylogs() {
  [ -d "$USB" ] || return 0
  cp -f "$DIR/logs.log" "$DIR/stderr.log" "$USB"/ 2>/dev/null
  sync -f "$USB" 2>/dev/null
}
[ -d "$USB" ] && echo "launcher: copying logs to $USB" >>"$LOG" || echo "launcher: $USB not found, logs stay in $DIR" >>"$LOG"
copylogs

# update: with a network, the program is replaced by the newest build of a channel (a rolling pre-release on GitHub,
# built on every push, see .github/workflows/handheld-dev.yml). The channel is the first word of update.conf in the
# 404tui folder: dev-cli-lab (without the file), "off" for no update. The old program is kept as sp404tui-arm64.prev.
# Everything is logged; without a network, or when anything fails, the program that is there runs as before.
UPDATE_URL=https://github.com/moahbeatlab/404tui/releases/download
fetch() { # fetch URL FILE: curl or wget, short timeouts so a missing network costs seconds
  if command -v curl >/dev/null 2>&1; then curl -fsSL --connect-timeout 5 --max-time "${3:-20}" -o "$2" "$1"
  else wget -q -T 5 -O "$2" "$1"; fi
}
update() {
  local ch; ch=$(awk 'NF{print $1; exit}' "$DIR/update.conf" 2>/dev/null); ch=${ch:-dev-cli-lab}
  [ "$ch" = off ] && { echo "launcher: update is off" >>"$LOG"; return 0; }
  local bin="$DIR/sp404tui-arm64" tmp="$DIR/sp404tui-arm64.new"
  rm -f "$tmp" "$tmp.sha256"
  fetch "$UPDATE_URL/$ch/sp404tui-arm64.sha256" "$tmp.sha256" 10 2>>"$LOG" || { echo "launcher: update: no answer from $ch (offline?)" >>"$LOG"; return 0; }
  local want have; want=$(awk '{print $1}' "$tmp.sha256"); have=$(sha256sum "$bin" 2>/dev/null | awk '{print $1}')
  rm -f "$tmp.sha256"
  [ -n "$want" ] || { echo "launcher: update: no checksum" >>"$LOG"; return 0; }
  [ "$want" = "$have" ] && { echo "launcher: update: $ch is the program already" >>"$LOG"; return 0; }
  printf '\033cupdating 404tui from %s ...\n' "$ch" >/dev/tty1
  fetch "$UPDATE_URL/$ch/sp404tui-arm64" "$tmp" 300 2>>"$LOG" || { echo "launcher: update: download failed" >>"$LOG"; rm -f "$tmp"; return 0; }
  [ "$(sha256sum "$tmp" | awk '{print $1}')" = "$want" ] || { echo "launcher: update: checksum differs, kept the old program" >>"$LOG"; rm -f "$tmp"; return 0; }
  chmod 755 "$tmp"
  "$tmp" -version >/dev/null 2>&1; local rc=$?
  if [ $rc -eq 126 ] || [ $rc -eq 127 ]; then echo "launcher: update: the new program does not run (status $rc), kept the old one" >>"$LOG"; rm -f "$tmp"; return 0; fi
  [ -f "$bin" ] && mv -f "$bin" "$bin.prev"
  mv -f "$tmp" "$bin" && echo "launcher: update: now $ch ($want)" >>"$LOG"
}
# the program had another name before (sp404h-arm64): an old copy is renamed, so that the first start works too
[ -f "$DIR/sp404h-arm64" ] && [ ! -f "$DIR/sp404tui-arm64" ] && mv -f "$DIR/sp404h-arm64" "$DIR/sp404tui-arm64"
update

# a copy of the program from an earlier start that is still running (it was left by the power button) is stopped:
# only processes whose own name is the program's, never anything wider
for p in $(pgrep -f '404tui-run|sp404tui-arm64'); do
  case "$(cat /proc/$p/comm 2>/dev/null)" in
    404tui-run*|sp404tui-arm64*) echo "launcher: stopping an earlier copy of the program, pid $p" >>"$LOG"; kill "$p" 2>>"$LOG";;
  esac
done
rm -f /dev/shm/404tui-run* /tmp/404tui-run* 2>/dev/null   # their copies (removing a file that runs is harmless)
sudo chmod 666 /dev/fb0 /dev/tty1 /dev/input/event* 2>>"$LOG" || echo "launcher: chmod of the devices failed" >>"$LOG"
printf '\033c' >/dev/tty1
setterm -cursor off >/dev/tty1 2>/dev/null
echo "$(date '+%F %T') launcher: starting $DIR/sp404tui-arm64" >>"$DIR/logs.log"
( while sleep 3; do copylogs; done ) & COPIER=$!   # also while it runs: a program that hangs never gets to the end
# H404_FB and H404_ARGS are for the emulator (emulator/README.md): no framebuffer there, a web page instead.
FBARGS=(-fb /dev/fb0)
if [ "${H404_FB+set}" = set ]; then FBARGS=(); [ -n "$H404_FB" ] && FBARGS=(-fb "$H404_FB"); fi
# run the program from memory (/dev/shm or /tmp, when they allow running programs): its code is then not read from
# the card while it runs. Anything that does not run falls back to the program where it is.
RUN="$DIR/sp404tui-arm64"
for d in /dev/shm /tmp; do
  if cp "$DIR/sp404tui-arm64" "$d/404tui-run.$$" 2>>"$LOG" && chmod 755 "$d/404tui-run.$$"; then
    "$d/404tui-run.$$" -version >/dev/null 2>&1; rc=$?
    if [ $rc -ne 126 ] && [ $rc -ne 127 ]; then RUN="$d/404tui-run.$$"; break; fi
    echo "launcher: $d does not run programs (status $rc)" >>"$LOG"; rm -f "$d/404tui-run.$$"
  fi
done
echo "$(date '+%F %T') launcher: version $("$RUN" -version 2>&1)" >>"$LOG"
echo "$(date '+%F %T') launcher: running $RUN, uptime $(cut -d' ' -f1 /proc/uptime) s, load $(cut -d' ' -f1-3 /proc/loadavg)" >>"$LOG"
"$RUN" -log "$LOG" -samples "$SAMPLES" -out "$CARD" "${FBARGS[@]}" $H404_ARGS >/dev/null 2>"$DIR/stderr.log" &
PID=$!
# where is it, while it runs: the state of the process (R running, S sleeping, D waiting for the disk or a driver)
# and the kernel function it waits in. A program that is stuck says where.
( for t in 1 2 3 5 7 10 15; do
    sleep $((t - ${prev:-0})); prev=$t
    [ -d /proc/$PID ] || break
    echo "$(date '+%F %T') launcher: +${t}s pid $PID state $(awk '{print $3}' /proc/$PID/stat 2>&1) wchan $(cat /proc/$PID/wchan 2>&1) threads $(ls /proc/$PID/task 2>/dev/null | wc -l) rss $(awk '/VmRSS/{print $2" kB"}' /proc/$PID/status 2>&1)" >>"$LOG"
    if [ "$t" = 5 ]; then sudo cat /proc/$PID/stack >>"$LOG" 2>&1; for tk in /proc/$PID/task/*; do echo "  task ${tk##*/}: $(awk '{print $3}' $tk/stat 2>&1) $(cat $tk/wchan 2>&1)" >>"$LOG"; done; fi
  done ) &
wait $PID
STATUS=$?
echo "$(date '+%F %T') launcher: program ended with status $STATUS" >>"$LOG"
kill $COPIER 2>/dev/null
[ "$RUN" != "$DIR/sp404tui-arm64" ] && rm -f "$RUN"
copylogs
printf '\033c' >/dev/tty1
