package ecucorpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Fatal("want an error without corpus.tsv")
	}
	tsv := "name\tsha256\tdef\na-0001\t00\tdefs/a-0001.json\nc-0001\t00\t-\n"
	if err := os.WriteFile(filepath.Join(dir, "corpus.tsv"), []byte(tsv), 0o644); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "list.yaml")
	if err := os.WriteFile(list, []byte("- a-0001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.List(list)
	if err != nil || len(got) != 1 || got[0] != filepath.Join(dir, "images", "a-0001.bin") {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := c.Path("b-0001"); err == nil {
		t.Fatal("want an error for a name outside the manifest")
	}
	if got := c.Def("a-0001"); got != filepath.Join(dir, "defs", "a-0001.json") {
		t.Fatal(got)
	}
	if got := c.Def("c-0001"); got != "" {
		t.Fatal(got)
	}
}
