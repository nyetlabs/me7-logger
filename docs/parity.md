# me7parity

`me7parity` reads `testdata/parity` in this checkout and is not in the me7info release. `NAME` is `8D0907551H-0003`. Without the software index, `8D0907551H` matches every index of that part. A `.bin` suffix is ignored.

## Report

Each score is hits/total and a percent. With no arguments every image is listed. `me7parity report NAME` limits the report and lists the names that missed, under open and block.

`ecu me7info` is the only hard mark. 100% means every row of that image's legacy ME7Info file matched.

- vs ecu-specific: matched rows / rows in the file. `(+N)` is catalog names located here that the file does not name.
- vs corpus: catalog names located on this image / the full catalog.
- extras: measurement-list names located on this image.

`names` is hits in `names/tuner.yaml`. A hit is one address. A scalar is that address, a curve needs its axis, a map needs both. A second address is a miss. On a DAMOS or A2L definition, a name that definition contains has to sit on one of its rows.

- tier: highest complete tier, from D through S+. `-` means D is still open. `*` means below S, except on a DAMOS or A2L definition. A tier is complete when every name hits or is listed in `names/absent.yaml`. A family counts as one name: one form located, or every form absent. A pair is the plain name, or both variants when the plain name is absent.
- axis: axes present on the maps that hit / axes those maps list. A scalar adds nothing.
- confidence: bodies that hold up / names that hit. Under 16 cells holds up. A larger body holds up when each peer in `datasets.yaml` is within two cells or 3%. A run of zeros holds up only when a peer has it and every such peer is zero. Names in `confidence.yaml` are omitted.
- block: hits in the other names file for this layout block. Blank when that block has no list.

`xdf damos` is maps in a DAMOS or A2L definition found at the same address. axis is those axes found with the same address, count, and width. `xdf hand` is the same two columns for a hand-made definition. A tuner name found at another address still hits in names, and is listed under "hand xdf disagrees" with ours and the xdf file offsets.

open and block, printed for a NAME, are the names that did not hit. `absent` is listed for this layout block, so the grade counts it. `met by <form>` means that family slot is already filled.

## Corpus

`me7parity corpus` lists every parity image and what `--submit` would store. `--submit` writes `defs/<stem>.json`, sets that row's `def` column, and does not commit, push, or upload. A stored object keeps the previous conversion, units, and layout. The description becomes the name table and the comment the needle that located the map. The corpus is `--corpus`, else `CORPUS`, else `../ecu-corpus` when that checkout exists, else `corpus/`.

Confidence has to pass on every row: high bodies are more than half the tuner names that scored. A zero denominator fails the row. A star means the definition is not a DAMOS oracle. xdfkit does not read tiers.

Columns are status, tier, delta, and confidence. `--grades` prints name, stage, and confidence for the download page and does not submit. `--staged` reads each zip's definition instead of locating maps again. Stage is the tuner tier: D is 1, C is 2, B is 3-, A is 3, and S is 3+. A star means the definition is not a DAMOS oracle. 8D0907551L and 8D0907551M are handmade and are the oracle. An open D stays `-`, or `-*` when it is not a DAMOS oracle. Under a named row the command prints hand, open, and block. `absent` and `met by <form>` are the same marks as the report. A definition that is not a parity image is listed when it has placeholder keys. Its tier is `-` and its confidence is `-`.

status

- `eligible`: stored. Origin becomes `located`.
- `identical`: same names, and the description and comment would stay.
- `origin damos` / `origin a2l`: left in place. Added maps and wrong addresses are edited into that JSON by hand.
- `origin unset`: no provenance origin. Left in place.
- `hand short NAME`: that hand name was not located. When other names can move or be added, the status continues with the delta and `--submit` writes those. Origin stays `hand`.
- `hand disagrees NAME`: that name has more than one located address. A following delta is a partial write and the origin stays `hand`. With no delta the row is left as it is.
- `grade -`: the name grade is below D. Origin `hand` is scored without waiting for a grade. With no definition, D or better is eligible.
- `confidence N/M`: high bodies are half or fewer.
- `keys`: blank-id keys named `obj-N`. `--submit` clears them and leaves the origin. Another status ends with `, key N` for the same clear.
- `def unreadable`: the definition file could not be read.

delta

- `+N`: names added. Status is eligible.
- `~N`: descriptions, comments, or addresses brought in line. Status is eligible when nothing is short.
- `~N +N`: both, while a short name remains. Origin stays `hand`.
- `key N`: placeholder keys cleared. On an eligible row the origin still becomes `located`.

`eligible` is a full cover: every current name at the same address and at least one new name, or an empty definition that passed the grade. That replaces the origin. A matching name set whose description or comment would change is also eligible, and its delta is `~N`. A name at a different address is the same kind of correction, and a located name the hand file does not have is added. A short key blocks neither. The status keeps the short and repeats the delta (`hand short KFWPFGR, +8`). `--submit` writes that row, keeps the hand conversion, and leaves the origin until the cover is complete.

Both `KFPED_0_A` and `KFPED_1_A` cover a hand `KFPED`. The same for `KFPEDR`. One variant does not, and a located `KFPED` does not cover a hand `KFPED_0_A`.

## Onboard

`me7parity onboard` scores corpus images that are not in `images.yaml` yet and prints the score. `--submit` adds the ones whose ME7Info rows all match. The layout block is the scored image with the longest shared name prefix. A duplicate layout, a second image of that layout, a short score, or a missing block is reported and left out. `--submit` restores both YAML files and the new golden when any ME7Info row is under 100%.

The flash file is `<corpus>/images/<part>/NAME.bin` (`<part>` is the first 9 characters of the part number), and that stem has to be a row of `<corpus>/corpus.tsv`. A filesystem path is rejected. `--corpus` is `XDFKIT_CORPUS`, otherwise `./corpus`. `ME7INFO_EXE` defaults to `../ME7L/bin/ME7Info.exe`. `WINE` defaults to `wine`. An existing `.ecu` is left in place.

`not scored` columns are name, status, and epk. epk is the EPK string, or `-`.

- `has-ecu`: an `.ecu` is already there, and a duplicate layout kept it out.
- `needs-ME7Info`: ME7Info.exe still has to run.
- `unsupported`: the image is not 512 KiB or 1 MiB.

`ME7Info` columns are name, status, me7info, and epk. me7info is matched rows / ME7Info rows.

- `short`: a ME7Info row missed.
- `eligible`: every ME7Info row matched.
- `unscored`: no `.ecu` to score.
