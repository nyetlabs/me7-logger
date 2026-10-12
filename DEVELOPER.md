# Developer

## Workflow

```mermaid
flowchart LR
  subgraph priv[private, never committed]
    src[DAMOS / A2L / OLS / hand XDF]
  end
  subgraph corpus[ecu-corpus submodule]
    bin[images/*.bin]
    def[defs/*.json + provenance]
    cats[categories.json]
  end
  subgraph win[Windows only]
    exe[ME7Info.exe]
  end
  src -->|xdfkit publish| def
  bin --> exe -->|.ecu| oracle[testdata/parity/ecu/me7info]
  exe -.->|Ghidra decompile| port[ported finders]
  port --> cfg[config/*.yaml]
  bin --> gen[me7info generate]
  cfg --> gen
  cats -->|make corpus-bump copies| cfg
  cats -->|xdfkit publish| tuner[PACK-tuner.xdf]
  gen -->|xdf.Model| kit[xdfkit model + XDF writer]
  gen --> out[.ecu + tuner .xdf, --full-xdf]
  kit --> out
  out --> log[me7logger log] --> csv[CSV]
  out --> par[me7parity]
  oracle --> par
  def --> par
  par -->|misses| cfg
  par -->|definition errors| src
```

Stage new originals under `testdata/parity/incoming/` (gitignored except its README). `me7parity onboard` runs ME7Info.exe, scores the `.ecu`, and adds an image when every ME7Info row is 100%. `me7parity publish` lists every parity image. Confidence has to pass for all of them. A star after the tier is an empty definition or origin `hand`. With no definition, the name grade must be D or better. Origin `hand` does not wait for that grade. `eligible` is a strict improvement: every current name at the same address and at least one new name, or an empty definition that passed the grade. `identical` is an unchanged name set and is not written. A matching name set whose description or comment would change is eligible, and its delta is ~N, the number of those maps. `--write` replaces origin `hand` or `located` only when the row is eligible, and the shared objects keep the previous conversion, units, and layout. The description is the name table and the comment is the locating needle. Origin `damos` and `a2l` are not replaced with a located model. A key `obj-N` with a blank id is a placeholder, not a name. Status `keys` and delta `key N` mean `--write` clears those keys and leaves the origin. A definition that is not a parity image is listed the same way, so `8D0907558E-D03` and `8D0907558M-D01` are included. A row that is short for another reason keeps that status with `, key N` on the end, and the delta is also `key N`. `--write` still clears the keys and sets that row's `def` cell to `defs/NAME.json`. The corpus is `../ecu-corpus` when that checkout exists, otherwise `corpus/` (`CORPUS=` overrides). It does not commit. KP, XDF, and DAMOS still go through xdfkit `publish/import-incoming.sh`. Corpus edits, `categories.json` included, are committed and pushed in `../ecu-corpus`, then pulled with `make corpus-bump`; `corpus/` is a read-only shallow submodule (xdfkit `docs/corpus.md`). ME7Info's `.ecu` output for each image is the only hard 100% oracle.

## Design

`me7info` (`generate`, `probe`, `place`), `me7parity` (the report, plus `onboard` and `publish`), and `me7logger` (`log`) share one Go module. `me7parity` reads `testdata/parity` and is not in the release. `generate` and `me7logger` locate items in the image as `record.Item` and `record.Map`. The `.ecu` writer reads those. `xdf.Model` converts the maps to an xdfkit model (file offsets, raw values, provenance origin `located`), and xdfkit files it with `Tuner` or `Categorize` and writes the XDF. `parity` reads the corpus definitions with xdfkit `canon` and `model`. me7-logger imports xdfkit; xdfkit never imports me7-logger.

`me7info place ref.json ref.bin dst.bin` writes a model for the destination image (`-o`, or stdout). A body that still matches is kept. One code pointer, found once, moves the object. Anything else is omitted. Provenance is format `image`, origin `located`. The report on stderr is the kept, moved, and omitted counts.

The generator is a masked byte search plus a few opcodes (selector, case bounds, `EXTP`), not a disassembler. A RAM address located by bytes is a row in `config/signatures.yaml`. A flash body is a row in `config/mapsigs.yaml`. Neither name, pattern, nor offset is a literal in Go. A row should match a code layout, not one image; check it across the corpus.

Out of scope: `.kp`, OLS, DAMOS, WinOLS, ecuxplot, `mapdump`. Do not copy NefMoto `Communication/`.

## xdfkit

`go.mod` pins xdfkit; there is no committed `replace`. `make go.work` writes the gitignored `go.work` (`use . ../xdfkit`) when that file is missing, so builds and tests use the sibling checkout. Delete the file to build the pin. Before pushing a change that needs new xdfkit code, push xdfkit, then `make xdfkit-bump` (pins its `master` as a pseudo-version) and `make check-pinned` (builds and vets; `make test` runs it), and fold `go.mod` and `go.sum` into the commit. Bump again after xdfkit history is rewritten. Releases should pin a tagged xdfkit; `make package` warns on a pseudo-version.

## Config

Each file in `config/` documents its fields in its header. `config/user/*.yaml` overlays them in filename order; a row of the same name replaces the shipped one, and `drop: true` removes a needle. `--user`/`ME7_USER` picks another directory. `config/examples/needles.yaml` is not loaded.

Files load from `config/` beside the executable (symlinks resolved) when present, else the embedded copy. `make build` mirrors `config/` into `build/config/` with `rsync --delete`, leaving `build/config/user/` alone, so edits reach `build/me7info` on the next build. `go run` and `go test` binaries have no `config/` beside them and use the embedded copy. `--core`, `--names`, `--meas`, `--map`, `--alias` (or `ME7_*`) replace one file.

Flags use pflag (`internal/cli`): a long name takes two hyphens (`--user`), its one-letter short name one (`-u`), and `-user` is an error.

`signatures.yaml` and `mapsigs.yaml` rows run top to bottom, RAM first, and the first row that hits fills a name. Prepended axis counts become rows and columns only when they account for every byte up to the body; never invent a count of 1. Only named maps reach the XDF.

`config/categories.json` is the corpus `categories.json` (xdfkit `docs/corpus.md`), copied by `make corpus-bump`; a test fails when they differ. It files each XDF map under a category. The tuner XDF holds only its names plus the maps at their axis addresses; `--full-xdf` holds every named map, with the rest under `Other`. `--model` writes that full model as JSON for the corpus (`xdfkit` `publish/README.md`).

A constant at a table axis address becomes a 1d breakpoint curve with that axis's count and data, so xdfkit links the axis to it. That happens only when every axis there has the same shape, the image values strictly increase, and no other map starts inside the curve. Otherwise it stays a constant, and `probe --maps` lists it as `xdf: not a breakpoint curve`.

### Axes on an interpolator call

The body is R12, or R13 page plus R12 low bits. The column header is the first of: an R13 immediate; R15 page with R14 low bits; a RAM load of R14 or R13; the header immediate stored through R12 before `MOV [ram], R4`. A second header loaded near the call is the row; conflicting or missing rows stay unset for `ytable` to fill. A `CALLS` into segment 0 outside the flash image uses the row's own `interp`.

## Logging

KWP `$23` reads at 10400 baud, up to 254 contiguous bytes per request, 55 ms apart, so scattered addresses cut the sample rate. `SamplesPerSecond` is 1 to 50, default 10.

## Parity

Images are in the private [ecu-corpus](https://github.com/nyetlabs/ecu-corpus) submodule (`make corpus`, `make corpus-bump`); oracles are in `testdata/parity`, whose files document themselves. `XDFKIT_CORPUS` points elsewhere; `XDFKIT_REQUIRE_CORPUS=1` fails on a missing corpus.

`me7parity --data testdata/parity` prints the report from shipped config only. `make test` fails only when an image is below 100% against its legacy `ecu/me7info/<image>.ecu`; every other score is coverage.

A name hits when it has an address and its listed axes. Each image's definition is its corpus model JSON. Fix definitions in the corpus, not here. `provenance.origin` `damos`/`a2l` is the reference, and a different address is a miss. `hand` or unset is an oracle only: disagreements still hit and are listed under `hand xdf disagrees`. `located` is read and left out of both sections.

The tier grade is the highest tier in `names/tuner.yaml` where every name hits or is in `names/absent.yaml`. A star after the tier is an empty corpus definition or origin `hand`. An `any` family is one name: one member located, or every member absent. `confidence` is high for bodies under 16 cells or matching a peer in `datasets.yaml` within two cells or 3%, and low for zero bodies unless every peer is zero.

## Version

`git describe` is the version. Commit subjects use the `cliff.toml` prefixes.
