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
			err = cmdParity(args, "")
		}
	} else {
		switch args[0] {
		case "report":
			err = cmdParity(args[1:], "report")
		case "onboard":
			err = cmdOnboard(args[1:])
		case "corpus":
			err = cmdCorpus(args[1:])
		case "help":
			usage()
		default:
			fmt.Fprintf(os.Stderr, "me7parity: unknown command %q\nRun me7parity -h\n", args[0])
			os.Exit(1)
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
  me7parity [flags]
  me7parity report [NAME...]
  me7parity corpus [--submit] [NAME...]
  me7parity onboard [--submit] [NAME...]
  me7parity -v, --version

`, version)
	fs, _, _ := parityFlags("")
	fmt.Fprintf(os.Stderr, "Flags:\n%s%s", fs.FlagUsages(), reportHelp)
}

const nameArg = `
NAME is 8D0907551H-0003. Without the software index, 8D0907551H matches
every index of that part. A .bin suffix is ignored.
`

const reportHelp = `
Prints the parity report. Each score is hits/total and a percent.
With no arguments every image is listed. me7parity report NAME
limits the report and lists the names that missed.

ecu me7info   ME7Info rows. 100% is the only hard mark.
names         tuner names: tier, axis, confidence, block.
xdf damos     DAMOS or A2L maps at the same address.
xdf manual    the same columns for a manual definition.

open and block are the misses. absent counts toward the grade.
axis means the table was located and an axis it lists is not set.
scale means the table was located and a cell or image axis has no scale.
met by <form> means that family slot is already filled.
lint lists xdfkit findings for the named images. They are not applied.

Column rules are in docs/parity.md. Run "me7parity report -h",
"me7parity onboard -h", or "me7parity corpus -h" for those commands.
` + nameArg

func parityFlags(cmd string) (*pflag.FlagSet, *string, *string) {
	synopsis := "[flags]"
	if cmd == "report" {
		synopsis = "[NAME...]"
	}
	fs := cli.NewFlagSet("me7parity", cmd, synopsis, reportHelp)
	dir := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`: images.yaml, ecu/me7info, names")
	corpus := fs.String("corpus", ecucorpus.Dir("."), "ecu-corpus checkout `<dir>`; XDFKIT_CORPUS sets the default")
	return fs, dir, corpus
}

func cmdParity(args []string, cmd string) error {
	fs, dir, corpus := parityFlags(cmd)
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
	if s := lintSection(c, stems); s != "" {
		fmt.Print("\n" + s)
	}
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

// matchStems expands each NAME. NAME-VER matches that stem. NAME matches
// every stem NAME-*. A .bin suffix is ignored. An exact stem does not
// also match a longer one.
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
			return nil, fmt.Errorf("%s: no match", arg)
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
