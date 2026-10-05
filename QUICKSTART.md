# Quickstart

Run `me7info` and `me7logger` from the directory that contains `config/`. A release archive is that directory. From a checkout, `make` writes `build/me7info` and `build/me7logger`. On Windows the programs are `me7info.exe` and `me7logger.exe`.

## Generate

```bash
me7info probe image.bin
me7info generate image.bin
```

`probe` prints one line per needle. `generate` writes `image.ecu` next to the image, and `image.xdf` when it locates maps. It does not open a serial port.

## Log

```ini
ECUCharacteristics=image.ecu
SamplesPerSecond=10

nmot
rl
```

```bash
me7logger log -p /dev/tty.usbserial -o log.csv image.bin session.cfg
```

Logging stays at 10400 baud. `-1` reads one sample and stops.

## Your rows

Add rows in `config/user/measurements.yaml` and `config/user/maps.yaml`. The comments in those files are the field lists. Overlay rules, needles, and the parity check are in [DEVELOPER.md](DEVELOPER.md).
