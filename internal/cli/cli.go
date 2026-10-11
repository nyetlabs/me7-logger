// Package cli holds the flag conventions shared by the programs:
// pflag, so a one-letter flag takes one hyphen and a long flag takes two.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

// NewFlagSet returns a flag set for "prog name" whose -h prints synopsis,
// the flags, and detail. An empty name is omitted, so the report command
// prints "prog synopsis".
func NewFlagSet(prog, name, synopsis, detail string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.SortFlags = false
	fs.Usage = func() {
		head := prog
		if name != "" {
			head += " " + name
		}
		fmt.Fprintf(fs.Output(), "Usage: %s %s\n\nFlags:\n%s%s", head, synopsis, fs.FlagUsages(), detail)
	}
	return fs
}
