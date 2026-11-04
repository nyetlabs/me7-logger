# Layout blocks

A block is the set of images that hit the same variants of the needles in `config/needles.yaml`. A near match stays in the block. The layout id is the Bosch software number from `ident.Find`.

- `testdata/parity/layouts.yaml` — images grouped by block
- `testdata/parity/layouts-priority.yaml` — block order for finders (authoritative; not duplicated here)

`me7parity onboard` assigns a new image to the scored image with the longest shared name prefix. Name tiers: [tuner-stages.md](tuner-stages.md). Engine codes are from `publish/vehicles.yaml` where a row exists.

## Blocks

Letter suffixes are listed only when they matter for the group. The same base can sit in more than one block.

| Block | Engine | Images |
| --- | --- | --- |
| 22m | 2.7T (APB, AZB, AZA) | 8D0907551 (G/N=AZB; H, L, M=APB), 4B0907551 (S=AZA, T=APB) |
| 22N | 2.7T (APB) | 8D0907551 (T, AA) — identical |
| 24BCD | 1.8T (AWP, AUM, AUQ, BEA, BAM) | 06A906032 (DL, FC, HF=AWP, HJ=AUM, HN=AUQ, HS=AWP, LP=AWP, LQ=AWP, NL=AWP, RP, TL), 8N0906018 (AL=AWP, BB=BEA, BC, BD, BH, CB=BAM, CH, CJ=BEA, Q) |
| 24bH | 1.8T (AVJ, AWM) | 4B0906018 (CH=AVJ, CM=AWM) — split when DAMOS coverage differs from 24BCD |
| 24E | 1.8T (AWT) | 4B0906018DC — NMAXF in DAMOS; no SNM16OPUW |
| 24G | 1.8T (AWT, BFB) | 4B0906018DH, 8E0909518AA — SNM16OPUW in DAMOS/generate; no NMAXF |
| 24KS | 1.8T (AWM, AVJ, BEX) | 4B0906018DQ, 8E0909518 (AK, AS), 8E0909559E — neither NMAXF nor SNM16OPUW in DAMOS |
| 25A | 2.7T (BEL) | 4Z7907551 (AA, Q, R, S, T) |
| 05A | 4.2 (no code) | 006410010A0 — same needles as 24E/24G/24KS; LDR finders do not open |
| 23g | 2.7T (ASJ, AZR) | 8D0907551 (F=ASJ; K, Q=AZR) |
| 25C | 4.2T (BCY) | 4D1907558 |
| X22ka | 2.7T (AZB) | 8D0907551 (A, B) — `result_selector_sub`; N is in 22m |
| X22 | 1.8T (APH) | 06A906032 (S=APH), 4B0906018 (Q) — `result_selector_sub` |

Membership is whatever `layouts.yaml` lists; this table is a readable view of the current set. The EPK project string does not separate blocks. The dataset token in `datasets.yaml` is calibration contents, not code layout. Letter suffixes without a `=` have no engine code in `vehicles.yaml` yet.
