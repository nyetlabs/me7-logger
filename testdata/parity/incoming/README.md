# Incoming images

This directory is gitignored apart from this file. `me7parity onboard` is the way in, after the image is in the corpus.

A new flash image goes in the full clone, `../ecu-corpus/images/<part>/PART-VERSION.bin` (`<part>` is the first 9 characters of the part number). The name is the part number and software version stored in the image. `make` in that repo rebuilds `corpus.tsv` and places the file. Commit it there. `make corpus-bump` in this repo moves the `./corpus` submodule up to that commit. `me7parity onboard` reads `./corpus`, scores every stem in `corpus.tsv` that is not yet in `images.yaml`, and prints the score. `--submit` adds them when every ME7Info row matches. The layout block is the scored image with the longest shared name prefix. A duplicate layout, a second image of that layout, a short score, or a missing block is reported and left out. NAME without a software index limits the run to every index of that part. `ME7INFO_EXE` defaults to `../ME7L/bin/ME7Info.exe`. `WINE` defaults to `wine`. An existing `.ecu` is left in place.

`--submit` restores both YAML files and the new golden when any ME7Info row is under 100%.

Corpus columns and `--submit` rules are in `docs/parity.md`.
