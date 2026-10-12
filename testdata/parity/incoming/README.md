# Incoming images

This directory is gitignored apart from this file. `me7parity onboard` is the way in.

- `me7parity onboard candidates` lists corpus images that are not in `images.yaml`: `eligible` when `ecus/NAME.ecu` is already there, `needs-ME7Info` when it is not, `unsupported` when the image is not 512 KiB or 1 MiB. The third column is the EPK string. `dup` means that layout id is already scored.
- `me7parity onboard run` writes `ecus/NAME.ecu` with ME7Info.exe. `ME7INFO_EXE` defaults to `../ME7L/bin/ME7Info.exe`. `WINE` defaults to `wine`. An existing file is left in place.
- `me7parity onboard triage` copies those files to `oracle/` and writes `triage.tsv`. `ready` means every ME7Info row is 100%.
- `me7parity onboard promote NAME[=BLOCK]` adds the image and its layout id. `BLOCK` defaults to the scored image with the longest shared name prefix. A layout id already on a scored image is refused. Any ME7Info row under 100% restores both YAML files and the new golden.

`me7parity onboard` with no action runs candidates, then run, then triage.

`me7parity corpus` lists every parity image, and every corpus definition whose key is `obj-N` with a blank id. Confidence has to pass. With no definition, or when the JSON is already origin `located`, the grade must be D or better. `--submit` replaces a `hand` file before that grade when the located model is a strict superset, or when the names match and a description or comment would change. Shared objects keep the hand conversion, units, and layout. The description is the name table and the comment is the locating needle. `damos` and `a2l` are not replaced with a located model. Status `keys` clears those placeholder keys and leaves the origin. A row that is short for another reason ends with `, key N`, and the delta is `key N`. `--submit` does not commit or upload.
