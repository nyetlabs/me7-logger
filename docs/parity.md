# me7parity

`me7parity` reads `testdata/parity` and is not in the me7info release. `NAME` is `8D0907551H-0003`. Without the software index, `8D0907551H` matches every index of that part. A `.bin` suffix is ignored. Corpus path and publish flow: [DEVELOPER.md](../DEVELOPER.md). Tuner tiers: [tuner-stages.md](tuner-stages.md). Layout blocks: [layout-blocks.md](layout-blocks.md).

## Report

Each score is hits/total and a percent. No arguments: every image. `me7parity report NAME` limits the report, lists misses under open and block, and lists xdfkit lint findings (rule, id, message). A finding is not applied.

`ecu me7info` is the only hard mark: 100% means every row of that image's legacy ME7Info file matched.

| Column | Meaning |
| --- | --- |
| vs ecu-specific | matched rows / rows in the file; `(+N)` is catalog names located here that the file does not name |
| vs corpus | catalog names located on this image / the full catalog |
| extras | measurement-list names located on this image |
| names | hits in `names/tuner.yaml` |
| tier | highest complete tier, D through S+; `-` means D is open; `*` means below S (except DAMOS/A2L) |
| axis | axes present on maps that hit / axes those maps list (scalar adds nothing) |
| confidence | bodies that hold up / names that hit |
| block | hits in the other names file for this layout block (blank when none) |
| xdf damos / xdf manual | maps (and axes) in that definition found at the same address (axes also need count and width) |

### A names hit

- One address. A second address is a miss.
- A curve or map also needs an axis read from the image. On a staged definition an ordinal axis counts and an unknown one does not.
- On a DAMOS or A2L definition, a name that definition contains has to sit on one of its rows.
- A curve or map hits only when each listed axis is set, and when its cell and each image axis have a unit or an agreed conversion.
- `config/sets.yaml` groups forms of one map. An `any` set is one name (one form located, or every form absent). A `both` set is the plain name, or every later form when the plain name is absent.
- A tier is complete when every name hits or is listed in `names/absent.yaml`.

### Confidence

- Two-axis name: needs row and column count; an image axis must be monotonic. All-zero axis is soft (`all zeros`). Other non-monotonic → hard fail `axis NAME`. Unreadable body → hard fail `body NAME`.
- Missing axis on a listed 2-axis map (`shape`) stays soft when the body is readable.
- Under 16 cells holds up. Larger bodies hold up when each peer in `datasets.yaml` is within two cells or 3%, or when the staged definition is verified.
- More than one cell of zeros holds up only when every peer that has zeros is all zeros. Names in `confidence.yaml` are omitted.

### Named-row extras

open and block are the names that did not hit. `absent` is listed for this layout block (counts toward the grade). `met by <form>` means that family slot is already filled. A tuner name at another address still hits in names and is listed under "manual xdf disagrees".

## Corpus

`me7parity corpus` lists every parity image and what `--submit` would store. `--submit` writes `defs/<stem>.json`, sets that row's `def` column, and does not commit, push, or upload. Columns: status, tier, delta, confidence.

`--grades` prints name, stage, confidence, and source for the download page (no submit). `--staged` reads each zip's definition instead of locating maps again.

- Stage: D→1, C→2, B→3-, A→3, S→3+; open D is `-`.
- Source (xdfkit `docs/model.md`): `verified` (`provenance.verified`); `DAMOS` (origin `damos`/`a2l`); `manual`; `matched` (auto whose `reference` agrees outside `except`); `auto` otherwise.
- Confidence gate: high bodies must be more than half the tuner names that scored; a zero denominator fails. xdfkit does not read tiers.
- Under a named row: manual, open, and block (same marks as the report). Non-parity definitions with placeholder keys are listed with tier `-` and confidence `-`.

### What `--submit` writes

- Stamp digests cover the document without `stamp` (`stamp.tool` alone does not change the file). An identical row is still stored when digests differ (`forced`); origin stays.
- Stored objects keep previous conversion, units, and layout. A raw value (factor 1, no offset, no units) takes the agreed DAMOS/A2L conversion for that name, role, and width (`config/units.yaml`). Clock names take the image clock factor. `_1_A` with no sample takes `_0_A` for that role and width.
- Description becomes the name table; comment becomes the locating needle. Low body confidence appends a short phrase after the needle. Missing axes add `missing x axis` / `missing y axis`. Expected but missing scales add `no scale` / `no x scale` / `no y scale` (codewords, config bytes, and error classes never get a scale note).
- Auto packs apply agreed scales before those notes. Manual packs keep a non-empty comment; DAMOS and A2L are not rewritten.
- Replacing a manual file records it as `reference` (`except` from `config/sets.yaml`). `verified` and an existing `reference` carry over on auto regenerate.

### status

| Mark | Meaning |
| --- | --- |
| `eligible` | stored; origin becomes `auto` |
| `identical` | same names; description, comment, and layout would stay |
| `forced` | names match; stamp digests of the merged model differ; origin stays |
| `verified` | `provenance.verified`; existing objects stay; missing located names added (`verified, +N`) |
| `origin damos` / `origin a2l` | left in place; edit JSON by hand |
| `origin unset` | no provenance origin; left in place |
| `manual short NAME` | that manual name was not located; other deltas may still write; origin stays `manual` |
| `manual disagrees NAME` | more than one located address; partial write keeps `manual` |
| `grade -` | name grade below D; `manual` scores without waiting for a grade |
| `confidence N/M` | high bodies half or fewer |
| `axis NAME` | axis turns and is not all zeros |
| `body NAME` | body unreadable |
| `keys` | blank-id keys `obj-N`; `--submit` clears them |
| `lint DS01` / `lint DS03` | would-be definition has that finding; row left in place. Other lint codes print only (`autocorrect.md` in xdfkit) |
| `def unreadable` | definition file could not be read |

### delta

| Mark | Meaning |
| --- | --- |
| `+N` | names added (`eligible`) |
| `~N` | descriptions, comments, addresses, or layouts brought in line |
| `~N +N` | both, while a short name remains; origin stays `manual` |
| `layout N` | auto file held back; `--submit` writes only located layouts onto raw objects |
| `key N` | placeholder keys cleared |
| `force` | stamp digests differ; `--submit` writes the merged model |

`eligible` is a full cover: every current name at the same address and at least one new name, or an empty definition that passed the grade. A matching name set whose description or comment would change is also eligible (`~N`). Both `KFPED_0_A` and `KFPED_1_A` cover a manual `KFPED` (same for `KFPEDR`); one variant does not, and a located `KFPED` does not cover a manual `KFPED_0_A`.

## Onboard

`me7parity onboard` scores corpus images not yet in `images.yaml`. `--submit` adds those whose ME7Info rows all match. The layout block is the scored image with the longest shared name prefix. Duplicate layout, second image of that layout, short score, or missing block: reported and left out. Under 100% ME7Info restores both YAML files and the new golden.

Flash path: `<corpus>/images/<part>/NAME.bin` (`<part>` = first 9 characters of the part number); stem must be a row of `<corpus>/corpus.tsv`. Filesystem paths are rejected. `--corpus` is `XDFKIT_CORPUS`, else `./ecu-corpus`. `ME7INFO_EXE` defaults to `../ME7L/bin/ME7Info.exe`; `WINE` defaults to `wine`. An existing `.ecu` is left in place.

### not scored

Columns: name, status, epk.

- `has-ecu`: `.ecu` already there; duplicate layout kept it out
- `needs-ME7Info`: ME7Info.exe still has to run
- `unsupported`: image is not 512 KiB or 1 MiB

### ME7Info

Columns: name, status, me7info (matched / ME7Info rows), epk.

- `short`: a ME7Info row missed
- `eligible`: every ME7Info row matched
- `unscored`: no `.ecu` to score
