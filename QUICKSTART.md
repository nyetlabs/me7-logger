# Quickstart

The programs read `config/` beside the executable, symlinks resolved: the unpacked release, `build/` after `make`, or the `make install` directory (`/usr/local/lib/me7-logger`). A file missing there uses the built-in copy.

```bash
me7info probe image.bin       # one line per needle; --maps counts XDF maps
me7info generate image.bin    # writes ecus/image.ecu, and image.xdf beside the image if maps are found
me7info generate --full-xdf image-full.xdf image.bin
me7info place ref.json ref.bin dst.bin   # model on stdout, counts on stderr
me7logger log -p /dev/tty.usbserial -o log.csv image.bin session.cfg
```

`image.xdf` is the tuner XDF: the maps listed in `config/categories.json` (`testdata/parity/names/tuner.yaml`) plus the maps at their axis addresses, in tuner categories. `--full-xdf` also writes every located map, with the rest under `Other`.

`--5120 auto` reads located PUMN, PUMX, and PUE and doubles mbar factors when only the doubled conversion sits in the 400–1200 mbar band. `--5120 on` doubles them. `--5120 off` leaves them.

`place` keeps a body that still matches on `dst.bin`. One code pointer, found once, moves the object. Anything else is left out. `-o` names the model file. Its origin is `auto`.

`session.cfg`:

```ini
ECUCharacteristics=ecus/image.ecu
SamplesPerSecond=10
nmot
rl
```

`-1` logs one sample and stops. Add your own rows in `config/user/measurements.yaml` and `config/user/maps.yaml`; the comments there list the fields.
