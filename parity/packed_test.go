package parity

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
)

func TestGradePackedUsesDef(t *testing.T) {
	def := packedModel(t, "manual", &model.Object{
		ID: "KRKTE", Shape: "value", Address: 0, Data: model.Data{Bits: 8},
	})
	img := []byte{0x10}
	got, err := GradePacked("../testdata/parity", []Packed{{Stem: "img", Img: img, Def: def}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "img.bin" || got[0].Tier != "-" || !got[0].NoDef {
		t.Fatalf("row %+v", got)
	}
	if got[0].Confidence.Hit != 1 || got[0].Confidence.Total != 1 {
		t.Fatalf("confidence %d/%d", got[0].Confidence.Hit, got[0].Confidence.Total)
	}
}

func TestGradePackedOracleHasNoStar(t *testing.T) {
	def := packedModel(t, "damos", &model.Object{
		ID: "KRKTE", Shape: "value", Address: 0, Data: model.Data{Bits: 8},
	})
	got, err := GradePacked("../testdata/parity", []Packed{{Stem: "img", Img: []byte{0x10}, Def: def}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].NoDef {
		t.Fatalf("row %+v", got)
	}
}

func TestGradePackedKeepsDuplicate(t *testing.T) {
	def := packedModel(t, "manual",
		&model.Object{ID: "KRKTE", Shape: "value", Address: 0, Data: model.Data{Bits: 8}},
		&model.Object{ID: "KRKTE", Shape: "value", Address: 1, Data: model.Data{Bits: 8}},
	)
	got, err := GradePacked("../testdata/parity", []Packed{{Stem: "img", Img: []byte{0x10, 0x20}, Def: def}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Confidence.Hit != 1 || got[0].Confidence.Total != 1 {
		t.Fatalf("confidence %d/%d", got[0].Confidence.Hit, got[0].Confidence.Total)
	}
}

func TestGradePackedCountsConstantCurve(t *testing.T) {
	def := packedModel(t, "manual", &model.Object{
		ID: "LDPBN", Shape: "value", Address: 0, Rows: 1, Cols: 1, Data: model.Data{Bits: 8},
	})
	got, err := GradePacked("../testdata/parity", []Packed{{Stem: "img", Img: []byte{0x10}, Def: def}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Confidence.Hit != 1 || got[0].Confidence.Total != 1 {
		t.Fatalf("confidence %d/%d", got[0].Confidence.Hit, got[0].Confidence.Total)
	}
}

func TestGradePackedCountsOrdinalAxis(t *testing.T) {
	def := packedModel(t, "damos", &model.Object{
		ID: "LAMFA", Shape: "2d", Address: 0, Rows: 2, Cols: 2, Data: model.Data{Bits: 8},
		X: &model.Axis{Source: "ordinal"}, Y: &model.Axis{Source: "ordinal"},
	})
	got, err := GradePacked("../testdata/parity", []Packed{{Stem: "img", Img: []byte{1, 2, 3, 4}, Def: def}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Confidence.Total != 1 {
		t.Fatalf("confidence %d/%d", got[0].Confidence.Hit, got[0].Confidence.Total)
	}
}

func TestGradePackedSkipsMapWithoutAxis(t *testing.T) {
	def := packedModel(t, "auto", &model.Object{
		ID: "LAMFA", Shape: "2d", Address: 0, Rows: 2, Cols: 2, Data: model.Data{Bits: 8},
	})
	got, err := GradePacked("../testdata/parity", []Packed{{Stem: "img", Img: make([]byte, 8), Def: def}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Confidence.Total != 0 {
		t.Fatalf("confidence %d/%d", got[0].Confidence.Hit, got[0].Confidence.Total)
	}
}

func packedModel(t *testing.T, origin string, objs ...*model.Object) []byte {
	t.Helper()
	m := &model.Model{
		Schema:     model.SchemaID,
		Provenance: &model.Provenance{Format: "image", Origin: origin, SHA256: strings.Repeat("ab", 32)},
		Objects:    objs,
	}
	b, err := canon.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
