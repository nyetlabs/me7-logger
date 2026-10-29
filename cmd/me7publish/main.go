package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
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
			usage()
			os.Exit(2)
		}
	} else {
		switch args[0] {
		case "stage":
			err = cmdStage(args[1:])
		case "help":
			usage()
		default:
			fmt.Fprintf(os.Stderr, "me7publish: unknown command %q\nRun me7publish -h\n", args[0])
			os.Exit(1)
		}
	}
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "me7publish: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7publish %s

Usage:
  me7publish stage [flags]
  me7publish -v, --version

stage builds one IMAGE-DATE.zip per corpus definition into --stage, and
writes vehicles.json for the download page. Source comes from each
definition's provenance. Stage and confidence are - for DAMOS. See publish/README.md.

`, version)
}
