package ecucorpus_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/generate"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/xdf"
)

// corpusCheckout is ../ecu-corpus beside this module when that manifest
// exists, otherwise this repo's corpus submodule. It does not look in xdfkit.
func corpusCheckout(t *testing.T) *ecucorpus.Corpus {
	t.Helper()
	root, err := ecucorpus.ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "..", "ecu-corpus")
	if _, err := os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		dir = ecucorpus.Dir(root)
	}
	c, err := ecucorpus.Load(dir)
	if errorsUnavailable(err) {
		t.Skip("corpus not available")
	}
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func errorsUnavailable(err error) bool {
	return err != nil && strings.Contains(err.Error(), ecucorpus.ErrUnavailable.Error())
}

func TestKFMLDMXRecord(t *testing.T) {
	c := corpusCheckout(t)
	m, conflicts := locatedModel(t, c, "4Z7907551S-0020")
	checkKFMLDMX(t, m, conflicts)
}

func TestLocatedAligned(t *testing.T) {
	c := corpusCheckout(t)
	other := map[string]int{}
	n := 0
	for _, name := range c.Names() {
		m, conflicts, img, ok := located(t, c, name)
		if !ok {
			continue
		}
		for _, line := range conflicts {
			if strings.Contains(line, "odd address") {
				t.Errorf("%s: %s", name, line)
			}
		}
		b, err := canon.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := api.Lint(b, api.LintRequest{Image: img})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, f := range rep.Findings {
			if f.Rule == "R9" || f.Rule == "R10" {
				t.Errorf("%s: %s %s", name, f.Rule, f.Message)
				continue
			}
			other[f.Rule]++
		}
		if name == "4Z7907551S-0020" {
			checkKFMLDMX(t, m, conflicts)
		}
		n++
	}
	if n == 0 {
		t.Fatal("no images")
	}
	t.Logf("%d images, other rules %v", n, other)
}

func locatedModel(t *testing.T, c *ecucorpus.Corpus, name string) (*model.Model, []string) {
	t.Helper()
	m, conflicts, _, ok := located(t, c, name)
	if !ok {
		t.Fatalf("%s: no image", name)
	}
	return m, conflicts
}

func located(t *testing.T, c *ecucorpus.Corpus, name string) (*model.Model, []string, []byte, bool) {
	t.Helper()
	imgPath, err := c.Path(name)
	if err != nil {
		t.Fatal(err)
	}
	img, err := os.ReadFile(imgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, false
		}
		t.Fatal(err)
	}
	res, err := generate.Generate(generate.Options{Image: img, ImageName: name + ".bin", Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	m, conflicts := xdf.Model(res.Maps, img, name)
	return m, conflicts, img, true
}

func checkKFMLDMX(t *testing.T, m *model.Model, conflicts []string) {
	t.Helper()
	for _, line := range conflicts {
		if strings.Contains(line, "KFMLDMX") && strings.Contains(line, "moved") {
			t.Errorf("KFMLDMX %s", line)
		}
	}
	for _, o := range m.Objects {
		if o.ID != "KFMLDMX" {
			continue
		}
		if o.Address%2 != 0 || o.Address != 0x13D8C || o.Rows != 8 || o.Cols != 9 {
			t.Errorf("KFMLDMX at 0x%X %dx%d", o.Address, o.Rows, o.Cols)
		}
		if o.Y == nil || o.Y.Data == nil || o.Y.Data.Bits != 16 || o.X == nil || o.X.Data == nil || o.X.Data.Bits != 8 {
			t.Errorf("KFMLDMX axes %+v %+v", o.X, o.Y)
		}
		return
	}
	t.Error("KFMLDMX not located")
}
