# 404tui: build, check and cross-compile the sp404 command.
#
#   make                  build ./sp404 for this computer
#   make check            test, build and run the program's own check (sp404 check)
#   make test             vet, gofmt check and the tests with the race detector
#   make dist             every platform below, into dist/
#   make linux | windows | macos
#   make dist VERSION=0.2 stamp a version (else the one in version/version.go stays)
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

.PHONY: site build test check vet fmt dist linux windows macos clean

# MIDI input needs cgo on a Mac (CoreMIDI) and Linux (ALSA headers: libasound2-dev); Windows has it
# always (no cgo). Where the headers are missing the build goes on without MIDI.
MIDITAGS := $(if $(filter Darwin,$(shell uname -s)),-tags midi,$(if $(shell pkg-config --exists alsa 2>/dev/null && echo y),-tags midi,))

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
	rm -rf $(DIST) $(BIN)

FORCE:
