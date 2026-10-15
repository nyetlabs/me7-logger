package main

import (
	"fmt"
	"os"
	"strings"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/xdfkit/api"
	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/lint"
	"go.nyet.org/xdfkit/model"
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
		writeFindings(&b, one, stem, rep.Findings)
	}
	if b.Len() == 0 {
		return ""
	}
	return "lint\n" + b.String()
}

// writeFindings prints findings that carry a cause under that cause, then
// the rest. The cause is the field. Nothing is applied.
func writeFindings(b *strings.Builder, one bool, stem string, findings []lint.Finding) {
	var causes []string
	by := map[string][]lint.Finding{}
	var plain []lint.Finding
	for _, f := range findings {
		if f.Cause == "" {
			plain = append(plain, f)
			continue
		}
		if _, ok := by[f.Cause]; !ok {
			causes = append(causes, f.Cause)
		}
		by[f.Cause] = append(by[f.Cause], f)
	}
	for _, cause := range causes {
		if one {
			fmt.Fprintf(b, "  %s\n", cause)
		} else {
			fmt.Fprintf(b, "  %s  %s\n", stem, cause)
		}
		for _, f := range by[cause] {
			writeLintLine(b, one, stem, f.Rule, findingLabel(f))
		}
	}
	for _, f := range plain {
		writeLintLine(b, one, stem, f.Rule, findingLabel(f))
	}
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
	writeFindings(&b, one, stem, findings)
	return b.String()
}

// findingLabel is the finding id, its map key when the model had one, and
// the message. The key is not a fix.
func findingLabel(f lint.Finding) string {
	s := f.ID
	if k := mapKey(f); k != "" {
		s += "  " + k
	}
	return s + "  " + f.Message
}

func mapKey(f lint.Finding) string {
	var out []string
	seen := map[string]bool{}
	for _, m := range f.Maps {
		k := m.Key
		if m.Axis != "" && k != "" {
			k += " " + m.Axis
		}
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return strings.Join(out, ", ")
}

// holdLint lints the model --submit would store. A missing image is no
// finding. R1 and R3 are the block. Other findings are the text. Nothing
// in a finding is applied.
func holdLint(c *ecucorpus.Corpus, name string, m *model.Model) (block, text string, err error) {
	if c == nil || m == nil || len(m.Objects) == 0 {
		return "", "", nil
	}
	imgPath, err := c.Path(name)
	if err != nil {
		return "", "", nil
	}
	img, err := os.ReadFile(imgPath)
	if err != nil {
		return "", "", err
	}
	findings, err := lintModel(m, img)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", name, err)
	}
	return lintStatus(findings), lintLines(true, name, findings), nil
}

func lintModel(m *model.Model, img []byte) ([]lint.Finding, error) {
	b, err := canon.MarshalStamped(m, "me7parity corpus")
	if err != nil {
		return nil, err
	}
	rep, err := api.Lint(b, api.LintRequest{Image: img})
	if err != nil {
		return nil, err
	}
	return rep.Findings, nil
}

// lintStatus is "lint R1", "lint R3", or "lint R1 R3" when those rules
// fired. Any other rule is empty.
func lintStatus(findings []lint.Finding) string {
	var r1, r3 bool
	for _, f := range findings {
		switch f.Rule {
		case "R1":
			r1 = true
		case "R3":
			r3 = true
		}
	}
	switch {
	case r1 && r3:
		return "lint R1 R3"
	case r1:
		return "lint R1"
	case r3:
		return "lint R3"
	}
	return ""
}
