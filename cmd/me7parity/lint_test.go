package main

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/lint"
	"go.nyet.org/xdfkit/model"
)

func TestLintLines(t *testing.T) {
	got := lintLines(true, "stem", []lint.Finding{{
		Rule: "R6", ID: "R6@0x100:4", Message: "all values identical: wrong address or size",
	}})
	want := "  R6   R6@0x100:4  all values identical: wrong address or size\n"
	if got != want {
		t.Fatalf("%q", got)
	}
	got = lintLines(true, "stem", []lint.Finding{
		{Rule: "R9", ID: "R9@0x10:2", Message: "odd", Cause: "shape"},
		{Rule: "R6", ID: "R6@0x20:4", Message: "same"},
		{Rule: "R10", ID: "R10@0x30:2", Message: "start", Cause: "record-start"},
	})
	want = "  shape\n  R9   R9@0x10:2  odd\n  record-start\n  R10  R10@0x30:2  start\n  R6   R6@0x20:4  same\n"
	if got != want {
		t.Fatalf("%q", got)
	}
	got = lintLines(true, "stem", []lint.Finding{{
		Rule: "R3", ID: "R3@0x20:4", Message: "monotonic only with the opposite signedness",
		Maps: []lint.AxisRef{{Key: "KFDMDARO", Axis: "x"}},
	}})
	if !strings.Contains(got, "KFDMDARO x") {
		t.Fatalf("%q", got)
	}
}

func TestLintFinding(t *testing.T) {
	img := make([]byte, 0x120)
	copy(img, []byte("40/1/ME7.1/5/6005.01//22m/DstC2o/011200/"))
	copy(img[0x100:], []byte{5, 5, 5, 5})
	def := []byte(`{"schema":"xdfkit-model/1","provenance":{"format":"kp","origin":"damos"},"objects":[{
		"id":"CONST","key":"CONST","shape":"1d","address":"0x200","rows":1,"cols":4,
		"data":{"bits":8},
		"x":{"source":"image","stored":"absolute","address":"0x100","data":{"bits":8}}
	}]}`)
	rep, err := api.Lint(def, api.LintRequest{Image: img})
	if err != nil {
		t.Fatal(err)
	}
	text := lintLines(true, "stem", rep.Findings)
	if !strings.Contains(text, "R6") || !strings.Contains(text, "R6@0x100:4") {
		t.Fatalf("%q", text)
	}
	if lintStatus(rep.Findings) != "" {
		t.Fatalf("R6 blocked: %s", lintStatus(rep.Findings))
	}
}

func TestLintHold(t *testing.T) {
	if got := lintStatus([]lint.Finding{{Rule: "R3"}, {Rule: "R1"}, {Rule: "R6"}}); got != "lint R1 R3" {
		t.Fatalf("%q", got)
	}
	img := make([]byte, 0x40)
	copy(img, []byte("40/1/ME7.1/5/6005.01//22m/DstC2o/011200/"))
	copy(img[0x20:], []byte{0x52, 0xF8, 0xB8, 0xFE, 0x48, 0x01, 0x33, 0x03})
	addr := model.Addr(0x20)
	m := &model.Model{
		Schema:     model.SchemaID,
		Provenance: &model.Provenance{Format: "image", Origin: "auto", SHA256: strings.Repeat("ab", 32)},
		Objects: []*model.Object{{
			Key: "KFDMDARO", ID: "KFDMDARO", Shape: "1d", Address: 0x10,
			Rows: 1, Cols: 4, Data: model.Data{Bits: 8},
			X: &model.Axis{
				Source: "image", Stored: "absolute", Address: &addr,
				Data: &model.Data{Bits: 16, Endian: "little"},
			},
		}},
	}
	findings, err := lintModel(m, img)
	if err != nil {
		t.Fatal(err)
	}
	if got := lintStatus(findings); got != "lint R3" {
		t.Fatalf("%s %#v", got, findings)
	}
	if !strings.Contains(lintLines(true, "stem", findings), "KFDMDARO") {
		t.Fatalf("%q", lintLines(true, "stem", findings))
	}
}
