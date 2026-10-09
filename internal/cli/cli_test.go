package cli

import (
	"io"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--maps", "-o", "x", "img"}, true},
		{[]string{"-u", "d", "--core=a", "img"}, true},
		{[]string{"--user", "d", "img"}, true},
		{[]string{"-o", "-maps", "img"}, true},
		{[]string{"img", "--maps"}, true},
		{[]string{"--", "-maps"}, true},
		{[]string{"-maps", "img"}, false},
		{[]string{"-core=a", "img"}, false},
		{[]string{"--u", "d", "img"}, false},
	} {
		fs := NewFlagSet("t", "t", "", "")
		fs.SetOutput(io.Discard)
		fs.Bool("maps", false, "")
		fs.String("core", "", "")
		fs.StringP("output", "o", "", "")
		user := fs.StringP("user", "u", "", "")
		err := fs.Parse(tc.args)
		if (err == nil) != tc.ok {
			t.Errorf("%q: err %v, want ok %v", tc.args, err, tc.ok)
		}
		if err == nil && *user != "" && !fs.Changed("user") {
			t.Errorf("%q: --user %q not marked changed", tc.args, *user)
		}
	}
}
