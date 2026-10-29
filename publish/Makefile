# One IMAGE-DATE.zip per corpus definition (README.md).
# Run in publish/, or with make -C publish from the me7-logger root.
#
#   make            stage the zips in build/publish/stage
#   make upload     rsync -a --delete to UPLOAD_DEST (local.mk);
#                   RSYNC_FLAGS=-n for a dry run
#   make clean      remove build/publish
#
# CORPUS is ../ecu-corpus when that checkout exists, else the ecu-corpus
# submodule, so git log dates the zips; a shallow submodule may not reach
# the commit. A file with no such
# commit uses its modification time. DATE skips a commit whose message
# starts chore(schema):. The converter is
# ../xdfkit/build/xdfkit when that file exists, otherwise xdfkit on PATH.

LOGGER := $(abspath ..)
XDFKIT_ROOT := $(abspath $(LOGGER)/../xdfkit)
OUT := $(LOGGER)/build/publish
STAGE := $(OUT)/stage
CORPUS ?= $(or $(wildcard $(abspath $(LOGGER)/../ecu-corpus)),$(LOGGER)/ecu-corpus)
export CORPUS

.PHONY: all stage upload clean

all: stage

stage:
	@if test -d $(XDFKIT_ROOT); then $(MAKE) --no-print-directory -C $(XDFKIT_ROOT) build; fi
	@cd $(LOGGER) && go run ./cmd/me7publish stage \
		--corpus $(CORPUS) \
		--stage $(STAGE) \
		--work $(OUT)/work \
		--publish $(CURDIR) \
		--xdfkit $(XDFKIT_ROOT)/build/xdfkit

upload: stage
	$(if $(UPLOAD_DEST),,$(error set UPLOAD_DEST in local.mk))
	@rsync -a --delete $(RSYNC_FLAGS) $(STAGE)/ $(UPLOAD_DEST)

clean:
	@rm -rf $(OUT)

-include local.mk
