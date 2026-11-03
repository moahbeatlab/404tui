#!/bin/bash
# Lays out ./roms (the games card) and ./usbdrive/r36s next to the compose file, like the device:
#   ./prepare.sh path/to/sp404h-arm64
set -e
bin="${1:?usage: prepare.sh path/to/sp404h-arm64 (build: GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o sp404h-arm64 ./cmd/sp404h)}"
here="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$here/roms/ports/404tui" "$here/roms/404/samples" "$here/roms/404/card" "$here/usbdrive/r36s"
cp "$bin" "$here/roms/ports/404tui/sp404h-arm64"
cp "$here/../404tui.sh" "$here/roms/ports/404tui.sh"
chmod 755 "$here/roms/ports/404tui.sh" "$here/roms/ports/404tui/sp404h-arm64"
# the container's user (uid 1002) must be able to write the folders
chmod -R a+rwX "$here/roms" "$here/usbdrive" 2>/dev/null || true   # files the container made (uid 1002) cannot be changed by you, and are already writable
echo "ready: now  docker compose up --build  in $here, then open http://localhost:8405"
