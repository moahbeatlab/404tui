#!/bin/bash
# 404h on an R36S under ArkOS: copy this file to the Ports folder of the games card (/roms/ports/, or
# /roms2/ports/ with a second card) and the program, sp404h-arm64, to a folder 404h/ beside it. Samples go in
# 404/samples on the games card; projects are written to 404/card, the folder to copy to the SP-404MKII's card.
#
# Only partly tried on a device: see logs.log in the 404h folder (copied to /mnt/usbdrive/r36s when that exists).
# What the program prints when it dies without being able to log (a crash of the Go runtime) is in stderr.log.
DIR="$(dirname "$(readlink -f "$0")")/404h"
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
  echo "-- cpu/mem:"; grep -m2 -i -E 'model name|hardware' /proc/cpuinfo; grep -E 'MemTotal|MemAvailable' /proc/meminfo
  echo "-- program:"; ls -l "$DIR/sp404h-arm64" 2>&1; file "$DIR/sp404h-arm64" 2>&1 | head -1
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
  sync -f "$USB" 2>/dev/null || sync
}
[ -d "$USB" ] && echo "launcher: copying logs to $USB" >>"$LOG" || echo "launcher: $USB not found, logs stay in $DIR" >>"$LOG"
copylogs

sudo chmod 666 /dev/fb0 /dev/tty1 /dev/input/event* 2>>"$LOG" || echo "launcher: chmod of the devices failed" >>"$LOG"
printf '\033c' >/dev/tty1
setterm -cursor off >/dev/tty1 2>/dev/null
echo "$(date '+%F %T') launcher: starting $DIR/sp404h-arm64" >>"$DIR/logs.log"
( while sleep 5; do copylogs; done ) & COPIER=$!   # also while it runs: a program that hangs never gets to the end
# H404_FB and H404_ARGS are for the emulator (emulator/README.md): no framebuffer there, a web page instead.
FBARGS=(-fb /dev/fb0)
if [ "${H404_FB+set}" = set ]; then FBARGS=(); [ -n "$H404_FB" ] && FBARGS=(-fb "$H404_FB"); fi
"$DIR/sp404h-arm64" -samples "$SAMPLES" -out "$CARD" "${FBARGS[@]}" $H404_ARGS >/dev/null 2>"$DIR/stderr.log"
echo "$(date '+%F %T') launcher: program ended with status $?" >>"$DIR/logs.log"
kill $COPIER 2>/dev/null
copylogs
printf '\033c' >/dev/tty1
