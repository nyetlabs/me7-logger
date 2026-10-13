package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/parity"
)

// version is set from git describe by the Makefile. Do not edit it here.
var version = "dev"

func main() {
	args := os.Args[1:]
	var err error
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		switch {
		case len(args) > 0 && (args[0] == "-h" || args[0] == "--help"):
			usage()
		case len(args) > 0 && (args[0] == "-v" || args[0] == "--version"):
			fmt.Println(version)
		default:
			err = cmdParity(args)
		}
	} else {
		switch args[0] {
		case "onboard":
			err = cmdOnboard(args[1:])
		case "corpus":
			err = cmdCorpus(args[1:])
		case "help":
			usage()
		default:
			err = cmdParity(args)
		}
	}
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "me7parity: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7parity %s

Usage:
  me7parity [flags] [pn...]
  me7parity onboard [candidates|run|triage|promote]
  me7parity corpus [flags] [pn...]
  me7parity -v, --version

`, version)
	fs, _, _ := parityFlags()
	fmt.Fprintf(os.Stderr, "Flags:\n%s%s", fs.FlagUsages(), reportHelp)
}

const reportHelp = `
Prints the parity report. Each score is hits/total and a percent.
With no pn the report is every image in images.yaml. A pn is a stem,
a stem.bin, or the part number in front of the software index:
8D0907551H matches 8D0907551H-0003. An exact stem matches only that
image. A pn adds the names that missed, under open and block.

ecu me7info
    The only hard mark. 100% means every row of that image's legacy
    ME7Info file matched.
    vs ecu-specific: matched rows / rows in the file. (+N) is catalog
    names located here that the file does not name.
    vs corpus: catalog names located on this image / the full catalog.
    extras: measurement-list names located on this image.

names
    Hits in names/tuner.yaml. A hit is one address. A scalar is that
    address, a curve needs its axis, a map needs both. A second
    address is a miss. On a DAMOS or A2L definition, a name that
    definition contains has to sit on one of its rows.
    tier: highest complete tier, from D through S+. "-" means D is
    still open. "*" means below S, except on a DAMOS or A2L
    definition. A tier is complete when every name hits or is listed
    in names/absent.yaml. A family counts as one name: one form
    located, or every form absent. A pair is the plain name, or both
    variants when the plain name is absent.
    axis: axes present on the maps that hit / axes those maps list.
    A scalar adds nothing.
    confidence: bodies that hold up / names that hit. Under 16 cells
    holds up. A larger body holds up when each peer in datasets.yaml
    is within two cells or 3%. A run of zeros holds up only when a
    peer has it and every such peer is zero. Names in confidence.yaml
    are omitted.
    block: hits in the other names file for this layout block. Blank
    when that block has no list.

xdf damos
    Maps in a DAMOS or A2L definition found at the same address.
    axis is those axes found with the same address, count, and width.

xdf hand
    The same two columns for a hand-made definition. A tuner name
    found at another address still hits in names, and is listed under
    "hand xdf disagrees" with ours and xdf file offsets.

open and block, printed for a pn, are the names that did not hit.
    absent: listed for this layout block, so the grade counts it.
    met by NAME: that family slot is already filled.

Reads testdata/parity in this checkout. This program is omitted from
the me7info release. Run "me7parity onboard -h" or
"me7parity corpus -h" for those commands.
`

func parityFlags() (*pflag.FlagSet, *string, *string) {
	fs := cli.NewFlagSet("me7parity", "", "[flags] [pn...]", reportHelp)
	dir := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`: images.yaml, ecu/me7info, names")
	corpus := fs.String("corpus", ecucorpus.Dir("."), "ecu-corpus checkout `<dir>`; XDFKIT_CORPUS sets the default")
	return fs, dir, corpus
}

func cmdParity(args []string) error {
	fs, dir, corpus := parityFlags()
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := ecucorpus.Load(*corpus)
	if err != nil {
		return err
	}
	rep, err := parity.Run(*dir, c)
	if err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fmt.Print(rep.Text())
		return nil
	}
	stems, err := matchStems(reportStems(rep), fs.Args())
	if err != nil {
		return err
	}
	fmt.Print(rep.Detail(stems))
	return nil
}

// reportStems is the tuner image names, without .bin, in report order.
func reportStems(rep *parity.Report) []string {
	if rep == nil {
		return nil
	}
	out := make([]string, len(rep.Tuner))
	for i, im := range rep.Tuner {
		out[i] = strings.TrimSuffix(im.Name, ".bin")
	}
	return out
}

// matchStems selects image stems named by args. An arg is a stem, a stem
// with .bin, or a part number: the stem with its software index removed.
// 8D0907551H matches 8D0907551H-0003. An exact stem does not also match
// a longer one.
func matchStems(stems, args []string) ([]string, error) {
	have := map[string]bool{}
	for _, s := range stems {
		have[s] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, arg := range args {
		arg = strings.TrimSuffix(arg, ".bin")
		var hit []string
		if have[arg] {
			hit = []string{arg}
		} else {
			prefix := arg + "-"
			for _, s := range stems {
				if strings.HasPrefix(s, prefix) {
					hit = append(hit, s)
				}
			}
		}
		if len(hit) == 0 {
			return nil, fmt.Errorf("%s: no parity image", arg)
		}
		for _, s := range hit {
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out, nil
}
