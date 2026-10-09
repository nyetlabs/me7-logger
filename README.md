# me7-logger

`me7info` writes an ME7Logger `.ecu` and, when it locates calibration maps, a TunerPro XDF. `me7logger` reads RAM over K-line into a CSV. Neither writes flash or EEPROM.

`make` runs the tests and builds `build/me7info` and `build/me7logger`. See [QUICKSTART.md](QUICKSTART.md) to use them and [DEVELOPER.md](DEVELOPER.md) to change them.

## Releases

`make package` writes `dist/`. Tag `vX.Y.Z` publishes it; `-rcN` is a prerelease. Run the programs from the unpacked directory so they read its `config/`.

## WARNING

Under heavy development, and history is rewritten often. Expect to `git reset --hard origin/master`. Nothing is a public contract yet: config formats, flags, and output can change without notice.
