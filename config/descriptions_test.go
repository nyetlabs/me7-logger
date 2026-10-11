package config

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/me7-logger/internal/ecucorpus"
)

var updateDesc = flag.Bool("update", false, "rewrite config/descriptions.yaml from the corpus")

func TestDescriptions(t *testing.T) {
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
	got, err := BuildDescriptions(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config", "descriptions.yaml")
	if *updateDesc {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("config/descriptions.yaml is stale: go test ./config -update")
	}
	if !bytes.Contains(got, []byte("\n- name: KFZW\n")) && !bytes.Contains(got, []byte("name: KFZW\n")) {
		t.Fatal("KFZW missing")
	}
}
