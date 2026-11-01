#!/bin/bash
# 404h on an R36S under ArkOS: copy this file to the Ports folder of the games card (roms2/ports/) and the
# program, sp404h-arm64, to roms2/ports/404h/. Samples go in roms2/404/samples; projects are written to
# roms2/404/card, the folder to copy to (or to use as) the SP-404MKII's card.
#
# NOT TRIED ON A DEVICE YET: the framebuffer, the input device and the permissions below are what ArkOS
# is expected to have; a log is written to /tmp/404h.log.
DIR="$(dirname "$(readlink -f "$0")")/404h"
SAMPLES=/roms2/404/samples
CARD=/roms2/404/card
mkdir -p "$SAMPLES" "$CARD"
sudo chmod 666 /dev/fb0 /dev/tty1 /dev/input/event* 2>/dev/null
printf '\033c' >/dev/tty1
setterm -cursor off >/dev/tty1 2>/dev/null
"$DIR/sp404h-arm64" -samples "$SAMPLES" -out "$CARD" -fb /dev/fb0 >/tmp/404h.log 2>&1
printf '\033c' >/dev/tty1
