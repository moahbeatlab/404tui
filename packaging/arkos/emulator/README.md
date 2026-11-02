# Trying 404h without the R36S

An arm64 container with the ArkOS userspace (Ubuntu 19.10), running the **same launcher script** as the device
(`../404h.sh`) and your local `sp404h-arm64`. The screen is the web page of `-web`, with the eight keys.

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o sp404h-arm64 ./cmd/sp404h   # in the repo root
cd packaging/arkos/emulator
./prepare.sh ../../../sp404h-arm64
docker run --privileged --rm tonistiigi/binfmt --install arm64                 # once, on x86 hosts
docker compose up --build
```

Open <http://localhost:8405>. Arrow keys and `a b x y` on the keyboard, or the buttons on the page.

Where things go, mirroring the device: `roms/` is the games card (`/roms`), `usbdrive/r36s/` is the log copy
folder (`/mnt/usbdrive/r36s`). `roms/ports/404h/logs.log` is the log, `roms/404/card/` is where projects are written.

## What it can and cannot show

| Shows | Does not show |
| --- | --- |
| the launcher: folders, the environment report, `logs.log` and its copy to the usb drive | `/dev/fb0` and the Rockchip framebuffer driver (the thing that failed last) |
| startup order, a hang in startup (the 10 s goroutine dump), exit status | the gamepad's real event codes |
| the app itself, ARM code under QEMU, ArkOS's glibc | the sound card; there is none, so audio fails to open |
| the unit transfer with `-sim` (add it to `H404_ARGS`) | USB OTG, the 4.4 kernel, the speed of an RK3326 |

Change what the program gets with `H404_ARGS` (e.g. `-web :8405 -demo -sim`) in `docker-compose.yml`.
`H404_FB=""` means no framebuffer; leave `H404_FB` unset and the launcher passes `-fb /dev/fb0` as on the device.

Tried: image built from ubuntu:eoan, launcher + app started, screen and keys answered over HTTP (Docker 29, x86 host with QEMU).
