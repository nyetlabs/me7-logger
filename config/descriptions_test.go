package config

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/me7-logger/internal/ecucorpus"
)

var update = flag.Bool("update", false, "rewrite config/descriptions.yaml and config/storage.yaml from the corpus")

// built checks config/file against build(corpus), or rewrites it with -update.
func built(t *testing.T, file string, build func(string) ([]byte, error)) []byte {
	t.Helper()
	root, err := ecucorpus.ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "..", "ecu-corpus")
	if _, err := os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		dir = ecucorpus.Dir(root)
	}
	if _, err := os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		if os.Getenv("XDFKIT_REQUIRE_CORPUS") != "" {
			t.Fatal(err)
		}
		t.Skip("corpus not available")
	}
	got, err := build(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config", file)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("config/%s is stale: go test ./config -update", file)
	}
	return got
}

func TestDescriptions(t *testing.T) {
	got := built(t, "descriptions.yaml", BuildDescriptions)
	if !bytes.Contains(got, []byte("\n- name: KFZW\n")) && !bytes.Contains(got, []byte("name: KFZW\n")) {
		t.Fatal("KFZW missing")
	}
}

func TestStorage(t *testing.T) {
	built(t, "storage.yaml", BuildStorage)
	for _, c := range []struct {
		name, part string
		want       Cell
		ok         bool
	}{
		{"KFMIRL", "", Cell{16, false, 16, 16, false, false}, true},
		{"KFMIRL", "8D0907551G", Cell{16, false, 16, 12, false, false}, true},
		{"KFLDRL", "", Cell{16, true, 16, 10, false, false}, true},
		{"LDRXN", "", Cell{16, false, 1, 16, false, false}, true},
		{"MLHFM", "", Cell{16, false, 1, 512, true, false}, true},
		{"KFTARX", "", Cell{Bits: 16}, true},
		{"KRKTE", "06A906032S", Cell{8, false, 1, 1, false, false}, true},
		{"KRKTE", "8D0907551M", Cell{16, false, 1, 1, false, false}, true},
		{"KFDMDADP", "8D0907551L", Cell{Bits: 16, XSigned: true}, true},
		{"KFDMDADP", "06A906032HF", Cell{16, false, 8, 6, false, false}, true},
		{"KFWDKPP", "4D1907558", Cell{8, false, 12, 12, false, true}, true},
		{"KFMIZUFIL", "", Cell{8, false, 8, 8, false, false}, true},
		{"NOSUCHNAME", "", Cell{}, false},
	} {
		got, ok := Storage(c.name, c.part)
		if got != c.want || ok != c.ok {
			t.Errorf("%s %s: %+v %v", c.name, c.part, got, ok)
		}
	}
}
