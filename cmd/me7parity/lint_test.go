package main

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/lint"
)

func TestLintLines(t *testing.T) {
	got := lintLines(true, "stem", []lint.Finding{{
		Rule: "R6", ID: "R6@0x100:4", Message: "all values identical: wrong address or size",
	}})
	want := "  R6   R6@0x100:4  all values identical: wrong address or size\n"
	if got != want {
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
}
