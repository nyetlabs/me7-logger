package config

import (
	"bytes"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/xdfkit/model"
)

var update = flag.Bool("update", false, "rewrite config/descriptions.yaml, config/storage.yaml, and config/units.yaml from the corpus")

func corpusDir(t *testing.T) (root, dir string) {
	t.Helper()
	var err error
	if root, err = ecucorpus.ModuleRoot(); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, "..", "ecu-corpus")
	if _, err = os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		dir = ecucorpus.Dir(root)
	}
	if _, err = os.Stat(filepath.Join(dir, "corpus.tsv")); err != nil {
		if os.Getenv("XDFKIT_REQUIRE_CORPUS") != "" {
			t.Fatal(err)
		}
		t.Skip("corpus not available")
	}
	return root, dir
}

// built checks config/file against build(corpus), or rewrites it with -update.
func built(t *testing.T, file string, build func(string) ([]byte, error)) []byte {
	t.Helper()
	root, dir := corpusDir(t)
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

func TestScales(t *testing.T) {
	got := built(t, "units.yaml", BuildScales)
	if !bytes.Contains(got, []byte(`bits: 8, unit: "grad KW"`)) || !bytes.Contains(got, []byte("KFZW")) {
		t.Fatal("KFZW missing")
	}
	if !bytes.Contains(got, []byte("clock: ")) {
		t.Fatal("clock row missing")
	}
	if v, ok := Conversion("KFZW", "z", 8, 0); !ok || v.Conversion.Factor != 0.75 {
		t.Fatalf("embedded KFZW %+v ok %v", v, ok)
	}
	if v, ok := Conversion("FZWWLRLN", "y", 8, 0); !ok || v.Units != "Upm" || v.Conversion.Factor != 40 {
		t.Fatalf("FZWWLRLN y %+v %v", v, ok)
	}
	if v, ok := Conversion("KFMI_UM", "x", 8, 0); !ok || v.Units != "Upm" || v.Conversion.Factor != 40 {
		t.Fatalf("KFMI_UM x %+v %v", v, ok)
	}
	if v, ok := Conversion("KFZW", "y", 16, 0); !ok || v.Units != "U/min" || v.Conversion.Factor != 0.25 {
		t.Fatalf("KFZW y %+v %v", v, ok)
	}
	if v, ok := Conversion("KFZWOP", "x", 16, 0); !ok || v.Units != "%" || v.Conversion.Factor != 0.0234375 {
		t.Fatalf("KFZWOP x %+v %v", v, ok)
	}
	if v, ok := Conversion("KFFWL_0_A", "x", 8, 0); !ok || v.Units != "Grad C" || v.Conversion.Factor != 0.75 {
		t.Fatalf("KFFWL_0_A x %+v %v", v, ok)
	}
	if v, ok := Conversion("MLHFM", "x", 16, 0); !ok || v.Units != "V" || v.Conversion.Factor != 640.0/0x10000 {
		t.Fatalf("MLHFM x %+v %v", v, ok)
	}
	if v, ok := Conversion("KRALH", "z", 8, 0); !ok || v.Units != "%" || v.Conversion.Factor != 0.75 || v.Min == nil || *v.Min != 0 || v.Max == nil || *v.Max != 100 {
		t.Fatalf("KRALH %+v %v", v, ok)
	}
	zw, _ := Conversion("KFZW", "z", 8, 0)
	if v, ok := Conversion("KFZW2", "z", 8, 0); !ok || v.Units != zw.Units || v.Conversion.Factor != zw.Conversion.Factor {
		t.Fatalf("KFZW2 z %+v %v", v, ok)
	}
	for _, form := range []string{"KFZW2_0_A", "KFZW2_1_A"} {
		if v, ok := Conversion(form, "z", 8, 0); !ok || v.Units != zw.Units || v.Conversion.Factor != zw.Conversion.Factor {
			t.Fatalf("%s z %+v %v", form, v, ok)
		}
	}
	if _, ok := Conversion("KFFWL", "z", 8, 0); ok {
		t.Fatal("plain name does not take a form's row")
	}
	if !Codeword("CWLAMFAW") || !Codeword("CWMDAPP") || Codeword("KRALH") {
		t.Fatal("codeword")
	}
	if !Bitmap("CATR") || !Bitmap("CLAATR") || Bitmap("ESKONF") || Bitmap("KRALH") {
		t.Fatal("bitmap")
	}
	if v, ok := Conversion("TEMIN", "z", 16, 0); !ok || v.Units != "ms" || v.Conversion.Factor == 0 {
		t.Fatalf("TEMIN %+v %v", v, ok)
	}
	// s4wiki Tuning: KRKTE ms/% is the injection tick / 24.
	word := map[int]float64{24: 1.0 / 375 / 24, 32: 1.0 / 312.5 / 24, 40: 1.0 / 250 / 24}
	for mhz, want := range word {
		v, ok := Conversion("KRKTE", "z", 16, mhz)
		if !ok || v.Units != "ms/%" || math.Abs(v.Conversion.Factor-want) > 1e-18 {
			t.Fatalf("KRKTE %d MHz %+v %v", mhz, v, ok)
		}
		b, ok := Conversion("KRKTE", "z", 8, mhz)
		if !ok || math.Abs(b.Conversion.Factor-want*16) > 1e-18 {
			t.Fatalf("KRKTE byte %d MHz %+v %v", mhz, b, ok)
		}
	}
}

func TestFillAuto(t *testing.T) {
	if !*update {
		t.Skip("rewrites origin-auto defs; go test ./config -run TestFillAuto -update")
	}
	_, dir := corpusDir(t)
	if n, err := FillAuto(dir); err != nil || n == 0 {
		t.Fatal(err)
	}
}

func TestApplyScalesOrdinal(t *testing.T) {
	m := &model.Model{Objects: []*model.Object{{
		ID:   "MLHFM",
		Data: model.Data{Bits: 16},
		X:    &model.Axis{Source: "ordinal", Value: model.Value{Conversion: model.Conversion{Factor: 1}}},
	}}}
	if n := ApplyScales(m); n != 1 {
		t.Fatalf("filled %d", n)
	}
	v := m.Objects[0].X.Value
	if v.Units != "V" || v.Conversion.Factor != 640.0/0x10000 {
		t.Fatalf("%+v", v)
	}
	if n := ApplyScales(m); n != 0 {
		t.Fatalf("filled again %d", n)
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
		{"KFTARX", "", Cell{16, false, 0, 0, false, false}, true}, // unsized: defs disagree on shape
		{"KRKTE", "06A906032S", Cell{8, false, 1, 1, false, false}, true},
		{"KRKTE", "8D0907551M", Cell{16, false, 1, 1, false, false}, true},
		{"KFDMDADP", "8D0907551L", Cell{16, false, 0, 0, false, true}, true}, // unsized: LP 6x8 vs HF 8x6
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

func TestVariantScale(t *testing.T) {
	scaleOnce.Do(loadScales)
	base, own := scaleKey{"T_0_A", "z", 8}, scaleKey{"T_1_A", "z", 8}
	scales[base] = scaleRow{Factor: 0.5, Units: "x"}
	t.Cleanup(func() { delete(scales, base); delete(scales, own); delete(openScales, own) })
	if v, ok := Conversion("T_1_A", "z", 8, 0); !ok || v.Conversion.Factor != 0.5 || v.Units != "x" {
		t.Fatalf("%+v %v", v, ok)
	}
	if _, ok := Conversion("T_1_A", "z", 16, 0); ok {
		t.Fatal("width")
	}
	scales[own] = scaleRow{Factor: 0.25}
	if v, ok := Conversion("T_1_A", "z", 8, 0); !ok || v.Conversion.Factor != 0.25 {
		t.Fatalf("%+v %v", v, ok)
	}
	delete(scales, own)
	openScales[own] = true
	if _, ok := Conversion("T_1_A", "z", 8, 0); ok {
		t.Fatal("open")
	}
}
