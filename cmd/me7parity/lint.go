package main

import (
	"fmt"
	"os"
	"strings"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/lint"
)

// lintSection is the xdfkit lint findings for these images. A finding is
// printed and not applied. A definition or image that cannot be read is
// omitted. An empty result is no section.
func lintSection(c *ecucorpus.Corpus, stems []string) string {
	if c == nil || len(stems) == 0 {
		return ""
	}
	one := len(stems) == 1
	var b strings.Builder
	for _, stem := range stems {
		defPath := c.Def(stem)
		imgPath, err := c.Path(stem)
		if defPath == "" || err != nil {
			continue
		}
		def, err := os.ReadFile(defPath)
		if err != nil {
			continue
		}
		img, err := os.ReadFile(imgPath)
		if err != nil {
			continue
		}
		rep, err := api.Lint(def, api.LintRequest{Image: img})
		if err != nil {
			writeLintLine(&b, one, stem, "lint", err.Error())
			continue
		}
		for _, f := range rep.Findings {
			writeLintLine(&b, one, stem, f.Rule, f.ID+"  "+f.Message)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "lint\n" + b.String()
}

func writeLintLine(b *strings.Builder, one bool, stem, rule, rest string) {
	if one {
		fmt.Fprintf(b, "  %-4s %s\n", rule, rest)
		return
	}
	fmt.Fprintf(b, "  %s  %-4s %s\n", stem, rule, rest)
}

// lintLines formats findings the way the named report prints them.
func lintLines(one bool, stem string, findings []lint.Finding) string {
	var b strings.Builder
	for _, f := range findings {
		writeLintLine(&b, one, stem, f.Rule, f.ID+"  "+f.Message)
	}
	return b.String()
}
