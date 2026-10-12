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
  me7parity -h, --help

me7parity scores each corpus image against legacy ME7Info output.
  onboard   Bring a corpus image into the parity set.
  corpus    List located models the corpus gate accepts. --submit stores
            them in ecu-corpus and does not commit or upload.

Run "me7parity <command> -h" for that command's flags.

These commands read testdata/parity in this checkout. They are not in
the me7info release.
`, version)
}

const parityHelp = `
Legacy ME7Info parity is the only hard mark, scored one image at a time.
The catalog column is coverage. The measurement list is the extras
column on each ECU row.

The names section scores names/tuner.yaml on every image, and each
other names/*.yaml list on the images of its layout block:
  - A hit is one address and its axes. An axis count of 0 is a scalar,
    so the address alone is the hit. One axis is a curve, two a map.
  - When that image's XDF contains the name, the body address must
    match one of its rows.
  - The axis column counts the axes present on the maps that hit. Its
    denominator is the list's axis count for those maps, so a scalar
    adds nothing. It is 0 only when every map that hit is a scalar.
  - The confidence column is the body bytes of the names that hit. Its
    denominator is that matched set, not the tuner list.
  - A star after the tier means the image has no corpus definition.

The image's corpus definition (corpus.tsv def), when present, is its
address oracle. Its provenance origin (damos, a2l, hand) says whether it
is DAMOS sourced; with none, it is hand made. Only
a DAMOS one turns a name hit into a miss. A hand made one that places a
name elsewhere is listed under "hand xdf disagrees" for review, since hand
made files can be wrong. Its x and y axes that have an address are the axis
column of that section. Origin located is not an oracle.

The names column scores the tuner list. The other names lists of an
image's layout block are scored in the block column.

A pn is an image stem or the part number in front of the software
index: 8D0907551H matches 8D0907551H-0003. The report is those images,
including the tuner and block names that did not hit. An absent name
is marked. A family form whose slot is already located is marked with
that name.
`

func cmdParity(args []string) error {
	fs := cli.NewFlagSet("me7parity", "", "[flags] [pn...]", parityHelp)
	dir := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`: images.yaml, ecu/me7info, and xdf")
	corpus := fs.String("corpus", ecucorpus.Dir("."), "ecu-corpus checkout `<dir>`; XDFKIT_CORPUS sets the default")
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
