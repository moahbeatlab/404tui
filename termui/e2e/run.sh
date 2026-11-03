#!/bin/bash
# Builds the program for Linux, builds the image and runs the terminal test in it against a real project.
#   termui/e2e/run.sh [path/to/a/project/folder]     results (text, colours, PNGs) in termui/e2e/out
set -eu
cd "$(dirname "$0")"
root=$(cd ../.. && pwd)
proj=${1:-/home/hermes/test-projects/Construction\ Kits\ House\ 01}
mkdir -p out build
(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o termui/e2e/build/sp404 ./cmd/sp404 && go build -o termui/e2e/build/ansipng ./termui/e2e/ansipng)
docker build -q -t 404tui-e2e . >/dev/null
rm -f out/*
set +e
docker run --rm -v "$PWD/build/sp404:/usr/local/bin/sp404:ro" -v "$proj:/project:ro" -v "$PWD/out:/out" 404tui-e2e
rc=$?
set -e
for f in out/*.ansi; do [ -e "$f" ] && ./build/ansipng "$f" "${f%.ansi}.png"; done
exit $rc
