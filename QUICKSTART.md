# Quickstart

Run from the directory containing `config/` (a release archive, or a checkout after `make`).

```bash
me7info probe image.bin       # one line per needle; --maps counts XDF maps
me7info generate image.bin    # writes image.ecu, and image.xdf if maps are found
me7logger log -p /dev/tty.usbserial -o log.csv image.bin session.cfg
```

`session.cfg`:

```ini
ECUCharacteristics=image.ecu
SamplesPerSecond=10
nmot
rl
```

`-1` logs one sample and stops. Add your own rows in `config/user/measurements.yaml` and `config/user/maps.yaml`; the comments there list the fields.
