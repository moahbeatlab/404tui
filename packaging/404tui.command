#!/bin/sh
# Double-click this file on a Mac: it opens Terminal and starts 404tui from the folder it is in.
cd "$(dirname "$0")" || exit 1
xattr -d com.apple.quarantine ./sp404 2>/dev/null
./sp404
