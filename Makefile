# Build and test me7-logger.
#
#   make                  test, then build build/me7-logger
#   make test             go test ./...
#   make build            build/me7-logger
#   make version          git describe (tags vX.Y.Z and vX.Y.Z-rcN)
#   make parity           percent of testdata/parity for .ecu and .xdf
#
# Version comes from git tags only. Do not edit a version by hand.

SHELL := /bin/bash

VERSION ?= $(patsubst v%,%,$(shell git describe --tags --match 'v[0-9]*' --dirty --always 2>/dev/null))

.PHONY: all test build version parity clean help

all: test build

test:
	go test ./...

build:
	mkdir -p build
	go build -ldflags "-X main.version=$(VERSION)" -o build/me7-logger ./cmd/me7-logger

version:
	@echo $(VERSION)

parity: build
	./build/me7-logger parity -data testdata/parity

clean:
	rm -rf build

help:
	@sed -n '2,9p' Makefile
