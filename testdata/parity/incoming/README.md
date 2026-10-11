# Incoming images

This directory is gitignored apart from this file. `me7info onboard` is the way in.

- `me7info onboard candidates` lists corpus images that are not in `images.yaml`: `staged` when `ecus/NAME.ecu` is already there, `needs-ME7Info` when it is not, `unsupported` when the image is not 512 KiB or 1 MiB. `dup` means that layout id is already scored.
- `me7info onboard run` writes `ecus/NAME.ecu` with ME7Info.exe. `ME7INFO_EXE` defaults to `../ME7L/bin/ME7Info.exe`. `WINE` defaults to `wine`. An existing file is left in place.
- `me7info onboard triage` copies those files to `oracle/` and writes `triage.tsv`. `ready` means every ME7Info row is 100%.
- `me7info onboard promote NAME[=BLOCK]` adds the image and its layout id. `BLOCK` defaults to the scored image with the longest shared name prefix. A layout id already on a scored image is refused. Any ME7Info row under 100% restores both YAML files and the new golden.

`me7info onboard` with no action runs candidates, then run, then triage.

`me7info publish` lists every parity image. Confidence has to pass. With no definition, or when the JSON is already origin `located`, the grade must be D or better. A `hand` file is replaced before that grade when the located model is a strict superset; shared objects keep the hand conversion, units, and description. `damos` and `a2l` are not replaced. It does not commit.
