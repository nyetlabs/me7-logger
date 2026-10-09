package cli

import (
	"flag"
	"io"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--maps", "-o", "x", "img"}, true},
		{[]string{"-m", "--core=a", "img"}, true},
		{[]string{"-o", "-maps", "img"}, true},
		{[]string{"img", "-maps"}, true},
		{[]string{"--", "-maps"}, true},
		{[]string{"-maps", "img"}, false},
		{[]string{"-core=a", "img"}, false},
		{[]string{"--m", "img"}, false},
		{[]string{"--o", "x", "img"}, false},
	} {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.Bool("maps", false, "")
		fs.Bool("m", false, "")
		fs.String("core", "", "")
		fs.String("o", "", "")
		if err := Parse(fs, tc.args); (err == nil) != tc.ok {
			t.Errorf("%q: err %v, want ok %v", tc.args, err, tc.ok)
		}
	}
}

func TestShort(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	user := fs.String("user", "", "")
	Short(fs, "u", "user")
	if err := Parse(fs, []string{"-u", "d", "img"}); err != nil {
		t.Fatal(err)
	}
	if *user != "d" || !IsSet(fs, "user") {
		t.Fatalf("user %q set %v", *user, IsSet(fs, "user"))
	}
}
