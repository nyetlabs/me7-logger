// Package cli holds the flag conventions shared by me7info and me7logger:
// a one-letter flag takes one hyphen, every longer flag takes two.
package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// NewFlagSet returns a flag set for "prog name" whose -h prints synopsis,
// the flags, and detail.
func NewFlagSet(prog, name, synopsis, detail string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s %s %s\n\nFlags:\n", prog, name, synopsis)
		PrintDefaults(fs)
		fmt.Fprint(fs.Output(), detail)
	}
	return fs
}

const shortFor = "short for --"

// Short registers -short as another name for each --long flag, given as
// short, long pairs. Pick a letter no other long flag of the program starts with.
func Short(fs *flag.FlagSet, pairs ...string) {
	for i := 0; i+1 < len(pairs); i += 2 {
		fs.Var(fs.Lookup(pairs[i+1]).Value, pairs[i], shortFor+pairs[i+1])
	}
}

// IsSet reports whether --long, or its Short name, was on the command line.
func IsSet(fs *flag.FlagSet, long string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == long || f.Usage == shortFor+long {
			set = true
		}
	})
	return set
}

// Dash is the hyphen prefix for a flag name.
func Dash(name string) string {
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

// Parse rejects a long flag with one hyphen or a one-letter flag with two,
// then parses args.
func Parse(fs *flag.FlagSet, args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || len(a) < 2 || a[0] != '-' {
			break
		}
		long := strings.HasPrefix(a, "--")
		name, _, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if long != (len(name) > 1) {
			fs.Usage()
			return fmt.Errorf("%s: use %s", a, Dash(name))
		}
		if f := fs.Lookup(name); f != nil && !hasVal && !isBool(f) {
			i++
		}
	}
	return fs.Parse(args)
}

// PrintDefaults is flag.PrintDefaults with Dash prefixes. A Short name
// is printed on its long flag's line.
func PrintDefaults(fs *flag.FlagSet) {
	w := fs.Output()
	short := map[string]string{}
	fs.VisitAll(func(f *flag.Flag) {
		if long, ok := strings.CutPrefix(f.Usage, shortFor); ok {
			short[long] = f.Name
		}
	})
	fs.VisitAll(func(f *flag.Flag) {
		if strings.HasPrefix(f.Usage, shortFor) {
			return
		}
		arg, usage := flag.UnquoteUsage(f)
		line := "  " + Dash(f.Name)
		if s, ok := short[f.Name]; ok {
			line = "  " + Dash(s) + ", " + Dash(f.Name)
		}
		if arg != "" {
			line += " " + arg
		}
		if len(line) <= 4 {
			line += "\t"
		} else {
			line += "\n    \t"
		}
		line += usage
		switch d := f.DefValue; {
		case d == "" || d == "0" || d == "false":
		case isNumber(d):
			line += fmt.Sprintf(" (default %s)", d)
		default:
			line += fmt.Sprintf(" (default %q)", d)
		}
		fmt.Fprintln(w, line)
	})
}

func isBool(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
