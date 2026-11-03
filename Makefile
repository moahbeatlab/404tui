# 404tui: build, check and cross-compile the sp404 command.
#
#   make                  build ./sp404 for this computer
#   make check            test, build and run the program's own check (sp404 check)
#   make test             vet, gofmt check and the tests with the race detector
#   make dist             every platform below, into dist/
#   make linux | windows | macos
#   make dist VERSION=0.2 stamp a version (else the one in version/version.go stays)
#
# The handheld front end (sp404h, for an R36S with ArkOS; see core/README.md and platform/handheld):
#   make handheld          dist/sp404h-arm64, for the handheld (no cgo, builds anywhere)
#   make handheld-card     dist/handheld/roms/..., laid out like the games card: copy its contents onto the card
#   make handheld-local    ./sp404h for this computer (try it in a browser: ./sp404h -demo -silent -web :8405)
#   make handheld-test     the tests of the handheld packages
#   make handheld-emulator the ArkOS-like arm64 container with the screen as a web page (needs Docker)
#
# Windows builds without cgo, from any computer, MIDI included. Linux builds without cgo too, but then
# has no MIDI input (that needs ALSA; the release builds it on Linux). macOS needs cgo (the serial port
# library asks IOKit for the USB ports), so the macOS binaries are built on a Mac; there both
# architectures are built and joined into one universal binary.

BIN     := sp404
PKG     := ./cmd/sp404
DIST    := dist
VERSION ?=

LDFLAGS := -s -w
ifneq ($(VERSION),)
LDFLAGS += -X github.com/moahbeatlab/404tui/version.Version=$(VERSION)
endif
GOBUILD := go build -trimpath -ldflags "$(LDFLAGS)"

LINUX   := linux-amd64 linux-arm64
WINDOWS := windows-amd64 windows-arm64

.PHONY: site build test check vet fmt dist linux windows macos clean lab-wasm handheld handheld-card handheld-local handheld-test handheld-emulator

# MIDI input needs cgo on a Mac (CoreMIDI) and Linux (ALSA headers: libasound2-dev); Windows has it
# always (no cgo). Where the headers are missing the build goes on without MIDI.
MIDITAGS := $(if $(filter Darwin,$(shell uname -s)),-tags midi,$(if $(shell pkg-config --exists alsa 2>/dev/null && echo y),-tags midi,))

# lab-wasm builds the file engine (package lab) as WebAssembly, with Go's loader, for the browser:
# 404jam's Sample Lab loads lab.wasm in a Web Worker. make lab-wasm LAB_OUT=../404jam/public/lab
LAB_OUT ?= dist/lab

lab-wasm:
	mkdir -p $(LAB_OUT)
	GOOS=js GOARCH=wasm go build -trimpath -ldflags "$(LDFLAGS)" -o $(LAB_OUT)/lab.wasm ./cmd/labwasm
	cp -f "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(LAB_OUT)/wasm_exec.js && chmod u+w $(LAB_OUT)/wasm_exec.js

build:
	go build $(MIDITAGS) -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

test: vet fmt
	go test -race ./...

# check is everything: the tests, then the built program checks itself (computer, simulated unit,
# and a plugged-in unit if there is one). The same without make: go test ./... && sp404 check
check: test build
	./$(BIN) check

vet:
	go vet ./...

# fmt fails when a file is not gofmt'ed, and names it.
fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt'ed:"; echo "$$out"; exit 1; fi

dist: linux windows macos

# ---- the handheld (R36S, ArkOS): a 64-bit ARM Linux program. The commit it was built from shows in its corner
# and in logs.log; build from a clean, committed tree to know exactly which one runs (a + marks changes).
HANDHELD     := sp404h
HANDHELD_PKG := ./cmd/sp404h

handheld:
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(DIST)/$(HANDHELD)-arm64 $(HANDHELD_PKG)
	@echo "built $(DIST)/$(HANDHELD)-arm64, commit $$(go version -m $(DIST)/$(HANDHELD)-arm64 | sed -n 's/.*vcs.revision=\(.......\).*/\1/p')$$(go version -m $(DIST)/$(HANDHELD)-arm64 | grep -q 'vcs.modified=true' && echo ' + changes that are not committed')"

# handheld-card lays out what goes on the games card: dist/handheld/roms/ports/404tui.sh and
# dist/handheld/roms/ports/404tui/sp404h-arm64. Copy the contents of dist/handheld/roms onto the card's roms
# folder (or a usb drive), then restart EmulationStation.
handheld-card: handheld
	rm -rf $(DIST)/handheld
	mkdir -p $(DIST)/handheld/roms/ports/404tui
	cp packaging/arkos/404tui.sh $(DIST)/handheld/roms/ports/404tui.sh
	cp $(DIST)/$(HANDHELD)-arm64 $(DIST)/handheld/roms/ports/404tui/sp404h-arm64
	chmod 755 $(DIST)/handheld/roms/ports/404tui.sh $(DIST)/handheld/roms/ports/404tui/sp404h-arm64
	@echo "copy the contents of $(DIST)/handheld/roms onto the card (its /roms folder)"

handheld-local:
	go build -ldflags "-s -w" -o $(HANDHELD) $(HANDHELD_PKG)

handheld-test:
	go vet ./core ./platform/... ./cmd/sp404h ./lab
	go test -race ./core ./platform/... ./lab ./formats

handheld-emulator: handheld
	cd packaging/arkos/emulator && ./prepare.sh ../../../$(DIST)/$(HANDHELD)-arm64 && docker compose up --build

linux: $(addprefix $(DIST)/$(BIN)-,$(LINUX))

windows: $(addprefix $(DIST)/$(BIN)-,$(addsuffix .exe,$(WINDOWS)))

# The Linux build for this computer's own architecture has MIDI when the ALSA headers are installed (it
# needs cgo); the other architecture is cross-built without cgo, so without MIDI.
$(DIST)/$(BIN)-linux-%: FORCE
	@if [ "$$(uname -s)" = Linux ] && [ "$$(go env GOARCH)" = "$*" ] && pkg-config --exists alsa 2>/dev/null; then \
		echo "linux-$* with MIDI"; CGO_ENABLED=1 GOOS=linux GOARCH=$* $(GOBUILD) -tags midi -o $@ $(PKG); \
	else \
		echo "linux-$* without MIDI (needs cgo and the ALSA headers on a Linux computer of that architecture)"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$* $(GOBUILD) -o $@ $(PKG); \
	fi

$(DIST)/$(BIN)-windows-%.exe: FORCE
	CGO_ENABLED=0 GOOS=windows GOARCH=$* $(GOBUILD) -o $@ $(PKG)

macos:
ifeq ($(shell uname -s),Darwin)
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GOBUILD) -tags midi -o $(DIST)/$(BIN)-darwin-amd64 $(PKG)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GOBUILD) -tags midi -o $(DIST)/$(BIN)-darwin-arm64 $(PKG)
	lipo -create -output $(DIST)/$(BIN)-darwin-universal $(DIST)/$(BIN)-darwin-amd64 $(DIST)/$(BIN)-darwin-arm64
else
	@echo "the macOS binaries need cgo for the serial port: build them on a Mac"; exit 1
endif

# The page without a server (web/static with mode.js set), for GitHub Pages: it talks to the unit by Web MIDI.
site:
	rm -rf $(DIST)/site && mkdir -p $(DIST)/site && cp web/static/* $(DIST)/site/
	echo "window.SP404_LOCAL = true; // the unit is reached by Web MIDI, there is no server" > $(DIST)/site/mode.js
	touch $(DIST)/site/.nojekyll

clean:
	rm -rf $(DIST) $(BIN) $(HANDHELD)

FORCE:
