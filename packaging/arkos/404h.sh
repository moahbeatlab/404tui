#!/bin/bash
# 404h on an R36S under ArkOS: copy this file to the Ports folder of the games card (roms2/ports/) and the
# program, sp404h-arm64, to roms2/ports/404h/. Samples go in roms2/404/samples; projects are written to
# roms2/404/card, the folder to copy to (or to use as) the SP-404MKII's card.
#
# NOT TRIED ON A DEVICE YET: the framebuffer, the input device and the permissions below are what ArkOS
# is expected to have; the program writes logs.log next to itself (roms2/ports/404h/logs.log), and what it prints when it dies
# without being able to log (a crash of the Go runtime) goes to stderr.log beside it.
DIR="$(dirname "$(readlink -f "$0")")/404h"
SAMPLES=/roms2/404/samples
CARD=/roms2/404/card
mkdir -p "$SAMPLES" "$CARD"
LOG="$DIR/logs.log"
mkdir -p "$DIR" 2>/dev/null

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

sudo chmod 666 /dev/fb0 /dev/tty1 /dev/input/event* 2>>"$LOG" || echo "launcher: chmod of the devices failed" >>"$LOG"
printf '\033c' >/dev/tty1
setterm -cursor off >/dev/tty1 2>/dev/null
echo "$(date '+%F %T') launcher: starting $DIR/sp404h-arm64" >>"$DIR/logs.log"
"$DIR/sp404h-arm64" -samples "$SAMPLES" -out "$CARD" -fb /dev/fb0 >/dev/null 2>"$DIR/stderr.log"
echo "$(date '+%F %T') launcher: program ended with status $?" >>"$DIR/logs.log"
printf '\033c' >/dev/tty1
