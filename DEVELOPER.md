# Developer

## config/user

Every `.yaml` and `.yml` file in `config/user/` is read, in filename order, on top of the shipped lists. Files you add there stay out of git. A missing `config/user` is skipped. `-user` and `ME7_USER` select another directory, and that path must exist.

A release archive includes `config/user/measurements.yaml` and `config/user/maps.yaml`. Each file is the comment header from the shipped list and an empty list.

A later measurement row with the same name replaces the earlier one. A row with no needle is a stub and is not written. `stub: true` on a shipped measurement drops its needle and keeps the scale. A measurement needle belongs on a `measurements` row. The same name under `data` or `functions` is a separate needle.

A map row is added. Two map rows may share a name. One caller and distance may not. A map row needs `name`, `caller`, `at`, and `interp`.

`config/user/needles.yaml` is where `data` and `functions` rows go. Those rows overlay `config/needles.yaml`. The comment at the top of that file is the field list. A `data` row labels a table or other bytes. A `functions` row labels a function entry.

A row with `needle_hex` replaces that name, or adds it:

```yaml
functions:
  - name: my_caller
    needle_hex: "DA ?? ?? ?? DB 00"
    unique: true
```

`??` is one wildcard byte. Spaces are optional. Only word-aligned hits count. An omitted `unique` is true, so more than one hit is reported and skipped. An omitted `back_up` is 0, so the label is the hit. A negative `back_up` moves the label forward. `back_up: [min, max]` needs an even span and `entry_after`. `entry_after` is a list of byte strings, and one of them must precede the label.

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

A log cfg may set `SamplesPerSecond` from 1 through 50. Omitted, it is 10. A line starting with `;` or `#` is a comment. An alias is `name alias` or `name{alias}`. A relative `ECUCharacteristics` path is relative to the cfg file.

## Parity

The sample images and their oracles are in the git repo, under `testdata/parity`. A release archive does not include them. Clone the repo, then run the check from that checkout.

`make parity` builds `build/me7info` and runs `me7info parity -data testdata/parity`. It prints a report and exits 0. `make test` fails when any image is short of 100% on the ME7Info column.

That run reads the shipped YAML. It does not read `config/user`. Adding files there leaves the score unchanged. A drop means a shipped file in `config/` changed.

`testdata/parity/bin/*.bin` are the images. `ecu/me7info/<stem>.ecu` is the legacy ME7Info file for that image. A hit is the same name, address, size, and bitmask. `xdf/s4wiki/names.yaml` is one name list scored on every image. A hit is one address. A name that is not under `values` also needs an axis. `xdf/<stem>.xdf`, when present, is an address oracle for that image. Its x and y axes that have an address are a separate score.

The report sections are:

- `ecu me7info` is the legacy file, a count of catalog names that file does not name, and that image against the full catalog
- `ecu extras` is the measurement list, per image and once across the images
- `xdf s4wiki` is the shared name list
- `xdf` is the per-image address oracle
- `xdf axis` is the axes in that oracle
