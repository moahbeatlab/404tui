# 404tui: build, check and cross-compile the sp404 command.
#
#   make                  build ./sp404 for this computer
#   make check            test, build and run the program's own check (sp404 check)
#   make test             vet, gofmt check and the tests with the race detector
#   make dist             every platform below, into dist/
#   make linux | windows | macos
#   make dist VERSION=0.2 stamp a version (else the one in version/version.go stays)
#
# Linux and Windows build without cgo, from any computer. macOS needs cgo (the serial port
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

.PHONY: build test check vet fmt dist linux windows macos clean

# MIDI input is built in on a Mac (it needs cgo); elsewhere it is left out.
MIDITAGS := $(if $(filter Darwin,$(shell uname -s)),-tags midi,)

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

$(DIST)/$(BIN)-linux-%: FORCE
	CGO_ENABLED=0 GOOS=linux GOARCH=$* $(GOBUILD) -o $@ $(PKG)

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

clean:
	rm -rf $(DIST) $(BIN)

FORCE:
