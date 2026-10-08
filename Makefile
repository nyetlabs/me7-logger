# Build and test me7info and me7logger.
#
#   make                  test, then build both binaries
#   make test             go test ./...; ME7Info parity must be 100% (image tests skip without corpus/)
#   make build            build/me7info and build/me7logger
#   make version          git describe (tags vX.Y.Z and vX.Y.Z-rcN)
#   make parity           legacy ME7Info parity, plus coverage of the YAML lists, the S4wiki names, and supplied XDF address oracles
#   make package          dist archives for macos, linux, and windows
#   make corpus           fetch the corpus submodule at its pinned commit (needs access)
#   make corpus-bump      move the corpus submodule to the ecu-corpus head (commit it yourself)
#
# Version comes from git tags only. Do not edit a version by hand.

SHELL := /bin/bash

VERSION ?= $(patsubst v%,%,$(shell git describe --tags --match 'v[0-9]*' --dirty --always 2>/dev/null))

.PHONY: all test build version parity package corpus corpus-bump clean help

# Archive name uses macos; the Go port is darwin.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

all: test build

test:
	go test ./...

build:
	mkdir -p build
	go build -ldflags "-X main.version=$(VERSION)" -o build/me7info ./cmd/me7info
	go build -ldflags "-X main.version=$(VERSION)" -o build/me7logger ./cmd/me7logger

version:
	@echo $(VERSION)

# Each archive is me7info, me7logger, README.md, QUICKSTART.md, DEVELOPER.md, LICENSE, and config/.
# macOS and Linux are tar.gz. Windows is zip. Run from the unpacked directory.
package:
	rm -rf dist
	mkdir -p dist
	@set -euo pipefail; \
	command -v zip >/dev/null; \
	for spec in $(PLATFORMS); do \
		goos=$${spec%/*}; \
		arch=$${spec#*/}; \
		os=$$goos; \
		ext=; \
		if [[ $$goos == darwin ]]; then os=macos; fi; \
		if [[ $$goos == windows ]]; then ext=.exe; fi; \
		name=me7-logger-$(VERSION)-$$os-$$arch; \
		stage=dist/$$name; \
		rm -rf "$$stage"; \
		mkdir -p "$$stage/config/catalog" "$$stage/config/examples" "$$stage/config/user"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$arch go build -ldflags "-X main.version=$(VERSION)" -o "$$stage/me7info$$ext" ./cmd/me7info; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$arch go build -ldflags "-X main.version=$(VERSION)" -o "$$stage/me7logger$$ext" ./cmd/me7logger; \
		cp README.md QUICKSTART.md DEVELOPER.md LICENSE "$$stage/"; \
		cp config/*.yaml "$$stage/config/"; \
		cp config/catalog/*.yaml "$$stage/config/catalog/"; \
		cp config/examples/*.yaml "$$stage/config/examples/"; \
		awk 'NF && substr($$1,1,1) != "#" { exit } { print }' config/measurements.yaml > "$$stage/config/user/measurements.yaml"; \
		printf '%s\n' 'measurements: []' >> "$$stage/config/user/measurements.yaml"; \
		awk 'NF && substr($$1,1,1) != "#" { exit } { print }' config/maps.yaml > "$$stage/config/user/maps.yaml"; \
		printf '%s\n' 'maps: []' >> "$$stage/config/user/maps.yaml"; \
		if [[ $$goos == windows ]]; then \
			( cd dist && COPYFILE_DISABLE=1 zip -r -q -X "$$name.zip" "$$name" ); \
		else \
			COPYFILE_DISABLE=1 tar -C dist -czf "dist/$$name.tar.gz" "$$name"; \
		fi; \
		rm -rf "$$stage"; \
	done

parity: build
	./build/me7info parity -data testdata/parity

# --checkout overrides update = none in .gitmodules.
corpus:
	git submodule update --init --checkout --depth 1 corpus

corpus-bump:
	git submodule update --init --checkout --remote --depth 1 corpus
	@git -C corpus log -1 --format='corpus now at %h %s'
	@git status --short corpus

clean:
	rm -rf build dist

help:
	@sed -n '2,12p' Makefile
