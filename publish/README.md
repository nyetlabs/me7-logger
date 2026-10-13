# Publishing map definitions

`me7parity corpus --submit` updates the corpus JSON. `make` in this directory, or `make -C publish` from the repo root, writes one zip per definition into `build/publish/stage/`. `make upload` is the public rsync of that directory. The converter is `../xdfkit/build/xdfkit` when that file exists, otherwise `xdfkit` on `PATH`. `vehicles.yaml` is read with the PHP yaml extension.

Each corpus definition (`defs/IMAGE.json`, a `corpus.tsv` row whose `def` is set) is one zip, `IMAGE-DATE.zip`. `index.php` is the directory page: it reads the zips in the directory, counts the maps in each definition, and gives each image its own columns for the ECU, engine, code, vehicle, year, gearbox, and market. The page opens grouped by vehicle, and the column headers sort. The ECU column is the corpus family. The other columns are `vehicles.yaml`, joined into `vehicles.json` beside the page at stage time. Stage and confidence are `me7parity corpus --grades` on that same run. Stage is the tuner tier: 1, 2, 3-, 3, then S. A star means the grade has not reached S. An open D stays `-`. An image that is not in the parity set leaves both blank. Apache serves it instead of a listing. `make stage` fails when a published image has no vehicle row. KP, CSV and XDF are generated from that JSON. A correction goes into the JSON, never into the zip.

## Layout

`DATE` is `YYYYMMDD`, the later of the last commit of the JSON and, for an ME7 image, `categories.json`. A commit whose message starts `chore(schema):` does not count. A file with no such commit uses its modification time. `make upload` is `rsync -a --delete` to `UPLOAD_DEST` (gitignored `local.mk`; `RSYNC_FLAGS=-n` is a dry run). A new date removes the previous zip. The first upload of this tree also removes the old loose files and `full/`.

`CORPUS` is `../ecu-corpus` when that checkout exists, else the `corpus/` submodule (xdfkit `docs/corpus.md`). A shallow submodule may not reach the commit, so the date is wrong; use the full clone.

Members keep undated names, so an XDF and its metadata file share a stem:

- `IMAGE.json`, copied from the corpus
- `IMAGE.kp`, `IMAGE.csv`, `IMAGE.xdf`, `IMAGE.meta.json`
- when the image family matches `ME7`: `IMAGE-tuner.xdf`, `IMAGE-tuner.meta.json`, `IMAGE-tuner.csv`, from `xdfkit --tuner` and `categories.json`

CSV is the map list only, with no reference columns. `8N0906018CB` is not published: it has no corpus definition.

A definition that changed since it was stamped is published as written, and its conversions are quiet. The JSON in the zip is still the corpus file.

## Located models

`me7parity corpus --submit` stores `defs/NAME.json` and sets that row's `def`. It does not commit, push, bump, or upload. Column and status rules are in `docs/parity.md`. `me7parity onboard` is how a new image enters the parity set (`testdata/parity/incoming/README.md`).
