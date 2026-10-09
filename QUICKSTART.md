# Quickstart

Run from the directory containing `config/` (a release archive, or a checkout after `make`).

```bash
me7info probe image.bin       # one line per needle; --maps counts XDF maps
me7info generate image.bin    # writes image.ecu, and image.xdf if maps are found
me7info generate --full-xdf image-full.xdf image.bin
me7logger log -p /dev/tty.usbserial -o log.csv image.bin session.cfg
```

`image.xdf` is the tuner XDF: the maps listed in `config/categories.json` (`testdata/parity/names/tuner.yaml`) and their axes, in tuner categories. `--full-xdf` also writes every located map, with the rest under `Other`.

`session.cfg`:

```ini
ECUCharacteristics=image.ecu
SamplesPerSecond=10
nmot
rl
```

`-1` logs one sample and stops. Add your own rows in `config/user/measurements.yaml` and `config/user/maps.yaml`; the comments there list the fields.
