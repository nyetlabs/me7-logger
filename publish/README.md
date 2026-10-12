# Publishing map definitions

`me7parity corpus --submit` updates the corpus JSON. `make` in this directory, or `make -C publish` from the repo root, writes one zip per definition into `build/publish/stage/`. `make upload` is the public rsync of that directory. The converter is `../xdfkit/build/xdfkit` when that file exists, otherwise `xdfkit` on `PATH`. `vehicles.yaml` is read with the PHP yaml extension.

Each corpus definition (`defs/IMAGE.json`, a `corpus.tsv` row whose `def` is set) is one zip, `IMAGE-DATE.zip`. `index.php` is the directory page: it reads the zips in the directory, counts the maps in each definition, and gives each image its own columns for the ECU, engine, code, vehicle, year, gearbox, and market. The ECU column is the corpus family. The other columns are `vehicles.yaml`, joined into `vehicles.json` beside the page at stage time. Apache serves it instead of a listing. `make stage` fails when a published image has no vehicle row. KP, CSV and XDF are generated from that JSON. A correction goes into the JSON, never into the zip.

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

`me7parity corpus` lists every parity image. A part number limits that list to one image and prints the hand keys and tuner names behind the row. Confidence has to pass. With no definition, or when the JSON is already `located`, the grade must be D or better. Origin `hand` does not wait for that grade: it is eligible when the located model is a strict superset, and also when the names match and a description or comment would change. A name at a different address is corrected even while another key is short, and a located name the hand file does not have is added. The delta is ~N for the addresses and +N for the additions, and the status repeats that delta after the short. `--submit` writes the row. The origin stays `hand`, so the status is not `eligible`, until every short key is gone. `--submit` keeps the hand conversion, units, and layout. The description becomes the name-table sentence and the comment the locating needle. `me7parity corpus --submit` stores `defs/NAME.json` (`provenance.origin` `located`) and sets that row's `def` to `defs/NAME.json`. The corpus is `../ecu-corpus` when that checkout exists, else `corpus/` (`CORPUS=` overrides). The command does not commit, push, bump, or upload. A star after the tier in `me7parity` means the name grade has not reached S. A DAMOS or A2L oracle has no star. xdfkit does not read tiers.

A key `obj-N` with a blank id is a placeholder, not a name. The command ignores it when matching names. Status `keys` and delta `key N` mean `--submit` clears those keys and leaves the origin. That list includes a definition that is not a parity image. A row that is not eligible for another reason keeps that status with `, key N` on the end, and the delta is `key N`; `--submit` still clears the keys. The `def` cell is set to `defs/NAME.json`, which is the path it already has.

`me7parity onboard` is how a new image enters the parity set (`testdata/parity/incoming/README.md`).

- `damos` or `a2l`. `corpus` does not replace them with a located model. Added maps and wrong addresses are edited into that JSON by hand. Placeholder keys are cleared in place, and the origin stays.
- A short hand key stays in the file. The status keeps that short and repeats ~N or +N when `--submit` will still move an address or append a located name. The origin stays `hand`. Both `KFPED_0_A` and `KFPED_1_A` cover a hand `KFPED`. The same for `KFPEDR`. One variant does not, and a located `KFPED` does not cover a hand `KFPED_0_A`.
