# me7-logger

Cross-platform logger and definition generator for Bosch ME7. It reads and writes ME7Logger `.ecu`, `.cfg`, and CSV files, and it writes a TunerPro XDF when calibration maps were located. It reads RAM over K-line. It does not write flash or EEPROM.

Byte patterns live in `config/needles.yaml`: the result selector, the 5-baud and fast-init rows, and the table and curve interpolation entries (8-bit or 16-bit). A call to one of those entries passes the map address in R12, or a page in R13 and the low 14 bits in R12. The Bosch name comes from which caller function makes that call. Those slots are in `config/maps.yaml`. The axis point count and whether that axis is 8-bit or 16-bit are read from the image. The map's row count is not. A located map is not written to the XDF until it has a name. RAM variables come from that selector walk and `config/catalog/`. Checksum and flash-kernel patterns are in `config/examples/needles.yaml` and are not loaded. ME7 names live in `config/names.yaml`. Measurements live in `config/measurements.yaml`, with factors and comments and no addresses. A measurement row is written when a needle of the same name is supplied; a stub is not. Edit those files. This project does not read them from another checkout, and it does not include Ghidra.

`config/user/` is an optional directory of extra YAML files (`ME7_USER` or `-user` on generate, log, and probe). Files are read in name order. A later file replaces a needle or measurement of the same name. `drop: true` removes one. A measurement row replaces only the fields it lists. A measurement may omit size, bitmask, unit, signed, inverse, factor, and offset. Omitted, those are 2, 0, "", false, false, 1, and 0. Conversions live in `config/names.yaml`. An omitted conversion size is 2. A conversion maps a size and a unit onto omitted signed, inverse, factor, and offset. A named conversion is selected with `conversion` and is not that default, so its unit may be "". A catalog row uses a size and unit pair only when its size is 1 or 2. Repeated catalog scales are names in `config/catalog/scales.yaml`, selected with `scale`. A field written on the row is kept. A single-bit measurement bitmask is a flag and does not take the size and unit pair. YAML you add under `config/user/` stays out of git.

The original ME7Logger program (mki / setzi62, 2010-2013) is a different tool. This one is a workalike.

## Build

`make` runs the tests and writes `build/me7-logger`. `make test` runs `go test ./...`. `make parity` prints the percent of `testdata/parity` matched by `.ecu` and `.xdf` output. That directory is the M-box image `8D0907551M-0002`: the ME7Info rows, the uncommented torque and extras rows ME7Info does not name, and the S4 wiki maps that the M-box XDF locates. The version string comes from `git describe`.

`config/needles.yaml`, `config/names.yaml`, and `config/measurements.yaml` are the defaults (`ME7_CORE`, `ME7_NAMES`, `ME7_MEAS`, or `-core`, `-names`, `-meas`). The result-type catalog is the directory `config/catalog/` (`ME7_MAP` or `-map`; a single file is still accepted): `scales.yaml`, `bits.yaml` for bitmask rows, and `values.yaml` for the rest. Aliases are `config/aliases.yaml` (`ME7_ALIAS` or `-alias`). A catalog row may omit size, bitmask, unit, signed, inverse, factor, and offset. Omitted, those are 0, 0, "", false, false, 1, and 0. The `.ecu` map name stays `catalog.yaml`. A renamed alias has `was` set to the previous alias; the Bosch variable name is unchanged. `$1` in an alias is a regex reference. ecuxplot uses it in `loggers.yaml`. It is not a literal.

## Use

```bash
me7-logger probe image.bin
me7-logger generate -o out.ecu image.bin
me7-logger log -p /dev/tty.usbserial -1 -o log.csv image.bin session.cfg
```

`generate` writes `<image>.xdf` when it located calibration maps. It does not open a serial port. `log` runs generate, then samples with stock KWP2000 `readMemoryByAddress`. `Connect` stays unset until `config/needles.yaml` contains the slow-init needle named in `config/names.yaml`. Logging stays at 10400 baud, which is the init baud. A higher `LogSpeed` in the `.ecu` file is not switched.

`config/measurements.yaml` lists measurements. `generate` writes one when `config/user` supplies a needle of that name, and does not write a stub. When that needle's address is the load in a selector case, the case supplies the result type and the catalog is not asked for that case.

## Releases

The version comes from git tags only (`vX.Y.Z`, `vX.Y.Z-rcN`). Do not edit a version by hand. Commit subjects use the prefixes in `cliff.toml`: `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`. A `v*` tag runs `.github/workflows/release.yml`, which publishes `build/me7-logger` and git-cliff notes. `vX.Y.Z-rcN` is a prerelease. Stable notes skip rc tags.
