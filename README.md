# me7-logger

`me7info` writes an ME7Logger `.ecu` and, when it locates calibration maps, a TunerPro XDF. `me7logger` reads RAM over K-line and writes a CSV log. Neither writes flash or EEPROM. The 2010–2013 ME7Logger program is a different tool.

## Build

`make` runs the tests and writes `build/me7info` and `build/me7logger`.

## Use

```bash
me7info probe image.bin
me7info generate -o out.ecu -xdf out.xdf image.bin
me7logger log -p /dev/tty.usbserial -1 -o log.csv image.bin session.cfg
```

`generate` does not open a serial port. Without `-o` and `-xdf` it writes the `.ecu` and `.xdf` next to the image. `log` stays at 10400 baud. See [Quickstart](QUICKSTART.md). Overlay rules and `make parity` are in [DEVELOPER.md](DEVELOPER.md).

Shipped behavior comes from the YAML in `config/`. Files you add under `config/user/` stay out of git.

## Releases

`make package` writes `dist/`. A tag `vX.Y.Z` publishes those archives. `vX.Y.Z-rcN` is a prerelease. macOS and Linux archives are `.tar.gz`. Windows is `.zip`.

Each archive contains `me7info` and `me7logger` (`.exe` on Windows), this file, `QUICKSTART.md`, `DEVELOPER.md`, `LICENSE`, and `config/`. Run the programs from the unpacked directory so they read `config/`. Rows added under that folder's `config/user/` stay in the folder. Replacing the folder drops them.

## WARNING

This repo is under heavy development. It may not work as expected. DO NOT CLONE unless you are willing to do `git --hard reset origin/master` often, as the history here will be rewritten frequently.
