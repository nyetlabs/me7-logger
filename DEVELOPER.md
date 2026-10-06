# Developer

## Programs

One Go module, two programs: `me7info` (`generate`, `probe`, `parity`) and `me7logger` (`log`). `log` builds the definition from the image, then samples RAM. Each found item is one record (`record.Item` and `record.Map`). The `.ecu` writer and the XDF writer are peers of that record. An `.ecu` line needs a mask, a conversion, units, and a comm header. An XDF map needs a shape, axes, and an equation.

The generator is a masked search plus a closed set of opcodes (the result-type selector, case bounds, and `EXTP`). It is not a C166 disassembler. A template that only matches after a real decode is a failed needle. Fix that needle in `config/needles.yaml`. A Bosch name located by a signature, a window, or a chain is a row in `config/signatures.yaml`. The bytes, the window, and the call slot are fields on that row.

`Connect` is set from the slow-init needle named by `connect.slow_init_needle` in `config/names.yaml` when that needle has one hit. The line is `SLOW-` plus `prefer_address` when that address is the KWP2000 row, and `fallback_address` otherwise. `-connect` overrides it.

Out of scope: `.kp`, OLS, DAMOS, WinOLS scripts, ecuxplot, and `mapdump`. This tree does not load a needle list from another checkout.

This tree is MIT ([LICENSE](LICENSE)). Do not copy NefMoto `Communication/`. The higher-rate RAM handler is not in this tree.

## Config files

`config/needles.yaml` (`ME7_CORE`, `-core`) holds the result selector, the 5-baud and fast-init rows, and the table and curve interpolation entries (8-bit or 16-bit).

`config/signatures.yaml` is embedded with the other config files. `generate` runs those rows after the selector and after `opcode/sign.go`, so a row can embed a name already stored. `{name:MID}` splices that address in ME7Info's EXTP form. `{slot:N}` is `callsSlot(N)`: the word comes from the `calls` table after the bootrom version is chosen by counting calls. `from` is the CPU search floor. `open` is one hit; the `DB00` window around it limits `pattern`, and a later row names that window with `in`. `first` uses the first copy when several exist. Without it, `open` requires one copy. `after` continues from an earlier row's hit through the next `DB00`. `absent` skips the row when that pattern is in the range. `steps` is a chain; `skip` is the byte distance from the previous hit. `at` is the byte distance from the last hit to the address word and may be negative. `single` keeps the row only when the pattern occurs once. `also` stores more names from the same hit. A name already stored is left as it is. A name that is not in the catalog is kept so a later row can embed it, and it is not written. New signature rows go in this file.

`config/signatures.yaml` `mapsigs` locates a map the caller list does not name. `pattern` is one window and `XX` is the address word. `at` is the distance from the hit to that word, 2 when omitted. `add` is the byte distance from the decoded pointer to the body. `anchor` names a map this list already locates, and `add` is then the distance from that map. `rows` and `cols` are the axis point counts when those axes are prepended to the body, or when `xat` and `yat` name them. `xbits` and `ybits` are the breakpoint widths, 8 when that count is set and the width is omitted. The distance from the counts to the body is the length of that axis data. A curve sets `cols`. `xat` and `yat` are byte distances from the hit to an F2 operand, the RAM word whose setup stored that header. The count byte there has to match `cols` or `rows`. An anchor uses the base row's words when it does not name its own. Axes that are neither prepended nor named that way have no dimensions on the row. A pattern is kept only when it occurs once and the address is inside the image. A later row with the same name fills it only when an earlier window missed. When that pointer sits in front of the axes and the counts there account for every byte up to the body, those counts are the rows and the columns. A count the image does not hold stays unset. Do not invent a row count of 1. A located map is written to the XDF only when it has a name. The XDF unique id is the file offset, the header region is the image length, and an axis whose address is another map in the file links to that id.

`config/maps.yaml` names the caller slots. A second function prologue is another entry under `needles`, and both labels are searched. A call passes the map address in R12, or a page in R13 and the low 14 bits in R12. The Bosch name is which caller function makes that call. `at` is a positive even byte distance, or a list of them when the caller and the interp stay the same. A different caller or interp stays its own row. One axis header is read from R13 when R13 is not a page: the first byte is the point count, and a zero second byte marks a 16-bit axis. When R13 is a page, R14 is the low 14 bits of that header and R15 is its page. When the call loads R14 or R15, or R13 when R13 is not an immediate, from a RAM word, that word holds the header a setup stored: the low 14 bits in R12, and the page in R13 when the header is not on DPP0. F2 of R13 may sit between that immediate and the reload of the word. The column axis is the header in R13, or that RAM word when R13 is not a header. Once the column is known, a RAM word whose header is a different axis is the row. A word whose header is the column is the column index. Two different row headers leave the row unset. A page call that does not load the row's RAM word leaves the row unset. The axis equation is not filled from an address file.

`config/catalog/` (`ME7_MAP`, `-map`; a single file is still accepted) is the RAM result-type catalog: `scales.yaml`, `bits.yaml` for bitmask rows, and `values.yaml` for the rest. The `.ecu` map name stays `catalog.yaml`. An omitted catalog field is size 0, bitmask 0, unit "", signed false, inverse false, factor 1, offset 0. A size and unit pair applies only when the size is 1 or 2. A single-bit bitmask is a flag and does not take that pair. A field written on the row is kept.

`config/names.yaml` (`ME7_NAMES`, `-names`) holds ME7 names and conversions. An omitted conversion size is 2. A conversion fills omitted signed, inverse, factor, and offset from a size and a unit. A named conversion is selected with `conversion`. That conversion is not the size-and-unit default, so its unit may be "".

`config/measurements.yaml` (`ME7_MEAS`, `-meas`) is the measurement list. Overlay and field defaults are in the next section.

`config/aliases.yaml` (`ME7_ALIAS`, `-alias`) renames the catalog name. `was` is the previous alias. The Bosch variable name is unchanged. `$1` in an alias is a regex reference. ecuxplot uses it in `loggers.yaml`. It is not a literal.

## config/user

Every `.yaml` and `.yml` file in `config/user/` is read, in filename order, on top of the shipped lists. A later file replaces a needle or measurement of the same name. Files you add there stay out of git. A missing `config/user` is skipped. `-user` and `ME7_USER` select another directory, and that path must exist. The same flag is on `me7info generate`, `me7info probe`, and `me7logger log`.

A release archive includes `config/user/measurements.yaml` and `config/user/maps.yaml`. Each file is the comment header from the shipped list and an empty list.

A later measurement row with the same name replaces the earlier one. A row with no needle is a stub and is not written. `stub: true` on a shipped measurement drops its needle and keeps the scale. A measurement needle belongs on a `measurements` row. The same name under `data` or `functions` is a separate needle.

A measurement row carries the scale and, when the bytes are known, the needle that locates the mem operand. `generate` writes a row that has a needle and does not write a stub. `bit: true` means the label is an 8A or 9A and the word is 0xFD00 plus twice the next byte. When that needle's address is the load in a selector case, the case supplies the result type and the catalog is not asked for that case. An omitted measurement field is size 2, bitmask 0, unit "", signed false, inverse false, factor 1, offset 0. A measurement row replaces only the fields it lists. `needle_hex` on the row attaches or replaces the needle. An omitted `back_up` on a measurement row is -4. A failed measurement pattern is fixed on that row. A failed function pattern stays in `config/needles.yaml`.

A map row is added. Two map rows may share a name. `at` may be a list of distances on that caller and interp. One caller and distance may not. A map row needs `name`, `caller`, `at`, and `interp`.

`config/user/needles.yaml` is where `data` and `functions` rows go. Those rows overlay `config/needles.yaml`. The comment at the top of that file is the field list. A `data` row labels a table or other bytes. A `functions` row labels a function entry.

A row with `needle_hex` replaces that name, or adds it:

```yaml
functions:
  - name: my_caller
    needle_hex: "DA ?? ?? ?? DB 00"
    unique: true
```

`??` is one wildcard byte. Spaces are optional. Only word-aligned hits count. An omitted `unique` is true, so more than one hit is reported and skipped. An omitted `back_up` on a needle row is 0, so the label is the hit. A negative `back_up` moves the label forward. `back_up: [min, max]` needs an even span and `entry_after`. `entry_after` is a list of byte strings, and one of them must precede the label.

A row that omits `needle_hex` updates only the fields it lists, on a needle that already exists:

```yaml
data:
  - name: slow_init_table
    back_up: -2
```

`mask_hex` on that kind of row is the same length as the pattern and is combined with the stored mask by a bitwise AND. A new name without `needle_hex` is an error.

`drop: true` removes a needle that already exists:

```yaml
functions:
  - name: map_interp_table8
    drop: true
```

`config/examples/needles.yaml` holds checksum and flash-kernel rows. Copy the rows you want into `config/user/needles.yaml`. The examples file is not loaded. The programs leave flash and EEPROM alone.

After editing, `me7info probe image.bin` prints `hit`, `miss`, `ambig`, or `noent` for each needle.

## Loading

`config.Path` is `config/<name>` relative to the working directory, or the environment variable when it is set. `Read` and `LoadCatalog` use that directory when it exists, and the embedded copy only when it does not. The user overlay defaults to `config/user`, also relative to the working directory.

Running from the repo reads the source files. Running from anywhere else uses the embed until some other directory happens to be named `config`.

An explicit flag or environment variable replaces one file: `-core`, `-names`, `-meas`, `-map`, `-alias`, and `ME7_CORE`, `ME7_NAMES`, `ME7_MEAS`, `ME7_MAP`, `ME7_ALIAS`. `signatures.yaml` has no override flag. `config/examples/needles.yaml` is not embedded.

Editing a shipped file and running `go test` or `go run` rebuilds the embedded copy. A binary that was already built does not see that edit unless a flag or environment variable points at the file.

## Logging

`me7logger` samples with KWP `$23` (`ReadMemoryByAddress`). Names that sit next to each other are one read of at most 254 bytes. Each further span is another request, and `Pace` waits P3 (55 ms) between them. One contiguous span can approach the samples-per-second cap. Scattered addresses divide the rate by the number of spans. `$2C` defines one address and is not the sample path.

Logging stays at the init baud, 10400. A higher `LogSpeed` in the `.ecu` is left as written and is not switched.

A log cfg may set `SamplesPerSecond` from 1 through 50. Omitted, it is 10. A line starting with `;` or `#` is a comment. An alias is `name alias` or `name{alias}`. A relative `ECUCharacteristics` path is relative to the cfg file.

## Parity

The sample images and their oracles are in the git repo, under `testdata/parity`. A release archive does not include them. Clone the repo, then run the check from that checkout.

`make parity` builds `build/me7info` and runs `me7info parity -data testdata/parity`. It prints a report and exits 0. `make test` fails when any image is short of 100% on the ME7Info column. That column is `vs ecu-specific`: the image against `ecu/me7info/<stem>.ecu`, matched on name, address, size, and bitmask. It is the only hard 100%. A row that file does not name stays. Catalog names located on that image which the file does not name are a separate count. Extras are not in that count. The other scores are coverage. A shortfall does not fail the image.

`vs corpus` is that image against every name in `config/catalog/` (`values.yaml` and `bits.yaml`). It is not 100% on every binary.

Do not point `generate` at the oracle files without `-o` and `-xdf` aimed somewhere else.

That run reads the shipped YAML. It does not read `config/user`. Adding files there leaves the score unchanged. A drop means a shipped file in `config/` changed.

`testdata/parity/bin/*.bin` are the images. `ecu/me7info/<stem>.ecu` is the legacy ME7Info file for that image. `xdf/s4wiki/names.yaml` is one name list scored on every image. A hit is one address and an axis. A name under `values` has no axis in the image, so one address is the hit. Any other body with no axis is a miss. It is not a per-CPU list, and not a second 100%. The tuner set is <https://s4wiki.com/wiki/Tuning>. `xdf/<stem>.xdf`, when present, checks body addresses for that image. A missing file is omitted. It is not the S4wiki list, and it does not locate maps. `testdata/parity/incoming/` is not scored.

An x or y axis in that file that has an address is a separate score. A hit is the same address, point count, and width. An axis with no address is not in the score. The column axis is the one the caller passes, including a RAM word a setup filled with the header. The row axis is another RAM word the call loads, when that setup stored a different header. A page call that does not load that word leaves the row unset. A mapsig row names that header with `yat` when a setup stored it in a RAM word. A 16-bit axis or body is the even address. The odd byte in front of it is a pad, not a value.

Maps are located from the caller that passes the map to the interpolator, on that image. `addMapAt` reads the row and column counts when they sit in front of the axes. The caller slot reads the row axis from a RAM word the call loads when that header is not the column. A page call that does not load that word leaves it unset. A count the image does not hold stays unset, and that map is written as a constant.

The report sections are:

- `ecu me7info` is the legacy file, a count of catalog names that file does not name, and that image against the full catalog
- `ecu extras` is the measurement list, per image and once across the images. The torque scale is the `torque` conversion in `config/names.yaml`
- `xdf s4wiki` is the shared name list
- `xdf` is the per-image address oracle
- `xdf axis` is the axes in that oracle

## Version

`git describe` is the only version string (`vX.Y.Z`, `vX.Y.Z-rcN`). Do not edit one by hand. Commit subjects use the prefixes in `cliff.toml`. `.github/workflows/release.yml` publishes `build/me7info` and `build/me7logger`. Stable notes skip rc tags.
