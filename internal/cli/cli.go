// Package cli holds the flag conventions shared by me7info and me7logger:
// pflag, so a one-letter flag takes one hyphen and a long flag takes two.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

// NewFlagSet returns a flag set for "prog name" whose -h prints synopsis,
// the flags, and detail.
func NewFlagSet(prog, name, synopsis, detail string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.SortFlags = false
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s %s %s\n\nFlags:\n%s%s", prog, name, synopsis, fs.FlagUsages(), detail)
	}
	return fs
}
