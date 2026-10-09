# Build and test me7info and me7logger.
#
#   make                  test, then build both binaries
#   make test             go test ./...; ME7Info parity must be 100% (image tests skip without corpus/)
#   make build            build/me7info, build/me7logger, and build/config (user/ is kept)
#   make version          git describe (tags vX.Y.Z and vX.Y.Z-rcN)
#   make parity           legacy ME7Info parity, plus coverage of the YAML lists, the tuner names, and the corpus definitions
#   make package          dist archives for macos, linux, and windows
#   make corpus           fetch the corpus submodule at its pinned commit (needs access)
#   make corpus-bump      move the corpus submodule to the ecu-corpus head and copy its
#                         categories.json to config/ (commit them yourself)
#   make work             gitignored go.work that builds against ../xdfkit
#   make xdfkit-bump      pin go.mod to the pushed xdfkit master (pseudo-version)
#   make check-pinned     build and vet against the pinned xdfkit, without go.work
#
# Version comes from git tags only. Do not edit a version by hand.

SHELL := /bin/bash

VERSION ?= $(patsubst v%,%,$(shell git describe --tags --match 'v[0-9]*' --dirty --always 2>/dev/null))

.PHONY: all test build version parity package corpus corpus-bump work xdfkit-bump check-pinned clean help

# Archive name uses macos; the Go port is darwin.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

# The binaries read config/ beside themselves. user/ is the reader's own, so
# --delete leaves it alone.
CONFIG_SYNC := rsync -a --delete --exclude '*.go' --exclude 'user/' --exclude '.DS_Store' config/

all: test build

test: check-pinned
	go test ./...

build:
	mkdir -p build
	go build -ldflags "-X main.version=$(VERSION)" -o build/me7info ./cmd/me7info
	go build -ldflags "-X main.version=$(VERSION)" -o build/me7logger ./cmd/me7logger
	$(CONFIG_SYNC) build/config/

version:
	@echo $(VERSION)

# Each archive is me7info, me7logger, README.md, QUICKSTART.md, DEVELOPER.md, LICENSE, and config/.
# macOS and Linux are tar.gz. Windows is zip.
package:
	@GOWORK=off go list -m -f '{{.Version}}' go.nyet.org/xdfkit | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$$' || \
		echo 'warning: xdfkit is pinned to a pseudo-version, not a tag' >&2
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
		mkdir -p "$$stage/config/user"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$arch go build -ldflags "-X main.version=$(VERSION)" -o "$$stage/me7info$$ext" ./cmd/me7info; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$arch go build -ldflags "-X main.version=$(VERSION)" -o "$$stage/me7logger$$ext" ./cmd/me7logger; \
		cp README.md QUICKSTART.md DEVELOPER.md LICENSE "$$stage/"; \
		$(CONFIG_SYNC) "$$stage/config/"; \
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
	./build/me7info parity --data testdata/parity

# --checkout overrides update = none in .gitmodules.
corpus:
	git submodule update --init --checkout --depth 1 corpus

corpus-bump:
	git submodule update --init --checkout --remote --depth 1 corpus
	cp corpus/categories.json config/categories.json
	@git -C corpus log -1 --format='corpus now at %h %s'
	@git status --short corpus config/categories.json

# go.work and go.work.sum are gitignored; DEVELOPER.md has the cross-repo workflow.
work:
	go work init . ../xdfkit

# GOPROXY=direct: the proxy lags on commits pushed minutes ago.
xdfkit-bump:
	GOWORK=off GOPROXY=direct go get go.nyet.org/xdfkit@master
	GOWORK=off go mod tidy
	@GOWORK=off go list -m -f 'xdfkit now at {{.Version}}' go.nyet.org/xdfkit

check-pinned:
	GOWORK=off go build ./...
	GOWORK=off go vet ./...

clean:
	rm -rf build dist

help:
	@sed -n '2,15p' Makefile
