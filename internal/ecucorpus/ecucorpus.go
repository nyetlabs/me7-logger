// Package ecucorpus reads images from the shared ecu-corpus (xdfkit
// docs/corpus.md): the corpus.tsv manifest and images/<name>.bin. The corpus
// is the private git submodule at corpus/; a checkout without access leaves it
// empty, and corpus tests skip unless XDFKIT_REQUIRE_CORPUS is set.
package ecucorpus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrUnavailable is the error Load wraps when dir has no corpus.tsv.
var ErrUnavailable = errors.New("corpus not available")

// Corpus is an opened corpus: its directory and the names in its manifest.
type Corpus struct {
	Dir   string
	names map[string]struct{}
}

// Dir is XDFKIT_CORPUS, else corpus under root.
func Dir(root string) string {
	if d := os.Getenv("XDFKIT_CORPUS"); d != "" {
		return d
	}
	return filepath.Join(root, "corpus")
}

// Load reads dir/corpus.tsv.
func Load(dir string) (*Corpus, error) {
	path := filepath.Join(dir, "corpus.tsv")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w at %s (make corpus; xdfkit docs/corpus.md, Access)", ErrUnavailable, dir)
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	col := -1
	for i, h := range strings.Split(lines[0], "\t") {
		if h == "name" {
			col = i
		}
	}
	if col < 0 {
		return nil, fmt.Errorf("%s: no name column", path)
	}
	c := &Corpus{Dir: dir, names: map[string]struct{}{}}
	for n, line := range lines[1:] {
		f := strings.Split(line, "\t")
		if len(f) <= col {
			return nil, fmt.Errorf("%s:%d: no name", path, n+2)
		}
		c.names[f[col]] = struct{}{}
	}
	return c, nil
}

// Path returns images/<name>.bin for a manifest name.
func (c *Corpus) Path(name string) (string, error) {
	if _, ok := c.names[name]; !ok {
		return "", fmt.Errorf("%s: not in %s", name, filepath.Join(c.Dir, "corpus.tsv"))
	}
	return filepath.Join(c.Dir, "images", name+".bin"), nil
}

// List reads a YAML list of manifest names and returns their image paths.
func (c *Corpus) List(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := yaml.Unmarshal(b, &names); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no image", path)
	}
	paths := make([]string, len(names))
	for i, n := range names {
		if paths[i], err = c.Path(n); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return paths, nil
}

// TB is the part of testing.TB that Open uses.
type TB interface {
	Helper()
	Skip(args ...any)
	Fatal(args ...any)
}

// Open returns the corpus at Dir(module root) for a test. Without it, the test
// skips, or fails when XDFKIT_REQUIRE_CORPUS is set (CI).
func Open(t TB) *Corpus {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(Dir(root))
	if errors.Is(err, ErrUnavailable) && os.Getenv("XDFKIT_REQUIRE_CORPUS") == "" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", fmt.Errorf("no go.mod above the working directory")
		}
		dir = up
	}
}
