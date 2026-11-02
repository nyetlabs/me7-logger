# Tuner stages

Guidance for `testdata/parity/names/tuner.yaml`. Names are filed under the section of <https://s4wiki.com/wiki/Tuning> that tunes them; when a name appears in several sections, it goes to the lowest stage that needs it.

`tuner.yaml` maps an axis count to Bosch names. An `any` entry is one slot met by any of its forms. The report `tier` is the highest complete tier, counted up from D. A tier is complete when every name outside an `any` family is located or listed in `absent.yaml`. A family is met when one form is located, or every form is absent.

| Stage | Tier | Scope |
| --- | --- | --- |
| 1 | D | Boost (requested boost, I limit KFLDIMX/LDIATA, KFLDRL, I-regulation adaptation, over/underboost DTCs), timing (base maps, IAT correction, torque monitoring, knock), BTS and open-loop AFR (LAMBTS, LAMFAW, LAMFAWKR) |
| 2 | C | Injector and FPR (KRKTE, TVUB, FKKVS, KFLF, idle LTFT and idle misfires, consumption), exhaust DTCs (ESKONF, rear O2, cat, EGT, secondary air, catalyst heating) |
| 3- | B | Bigger turbos: base boost and wastegate spring (KFVPDKSD, KFVPDKSE, KFMSNWDK, KFWDKMSN, WDKUGDN), LDR PID (LDRQ0DY, LDRQ0S, LDRQ1DY, LDRQ1ST, KFLDRQ2, UMDYLDR), MAP sensor limit (LDEIAP, TLDIAPN, DSLOFS), 5120 ambient check (PUE, PUMN, PUMX). Exhaust change stays on KFLDIMX/KFLDRL (stage 1) |
| 3 | A | MAF and intake: MAF scaling (MLHFM, MLOFS, MLMAX, KFMLDMX, KFKHFM), VE model and cams (KFPBRK, KFPBRKNW, KFPRG, KFURL, KFNW, KFNWWL), ARMD (KFDMDADP, KFDMDARO, KIFZGHG) |
| nicety | S | Speed and rev limiters, left-foot braking, ASR, MSR, launch control, wheel speed, larger displacement, DTC disables outside the exhaust (CDLDP, CDTES) |

KFZW2 is stage 1. NFSM and NLLM are stage 2. NLL2M is nicety.
