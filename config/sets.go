package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Set is one map. Names[0] is the plain form.
// RuleEither: one address covers any name.
// RuleBoth: every later name covers the plain name. One does not.
// RuleAny: one located form closes the tuner slot.
type Set struct {
	Rule  string   `yaml:"rule"`
	Names []string `yaml:"names"`
}

const (
	RuleEither = "either"
	RuleBoth   = "both"
	RuleAny    = "any"
)

// Sets reads sets.yaml. A name is in one set.
func Sets() ([]Set, error) {
	doc, err := loadSetsDoc()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range doc.Sets {
		switch s.Rule {
		case RuleEither, RuleAny:
			if len(s.Names) < 2 {
				return nil, fmt.Errorf("%s: %s needs two names", SetsFile, s.Rule)
			}
		case RuleBoth:
			if len(s.Names) < 3 {
				return nil, fmt.Errorf("%s: both needs the plain name and both variants", SetsFile)
			}
		default:
			return nil, fmt.Errorf("%s: rule %q", SetsFile, s.Rule)
		}
		for _, n := range s.Names {
			if n == "" || seen[n] {
				return nil, fmt.Errorf("%s: %q repeated", SetsFile, n)
			}
			seen[n] = true
		}
	}
	return doc.Sets, nil
}

// ReferenceExcept is sets.yaml except: reference names our address supersedes
// when me7parity corpus compares an auto definition to a manual/DAMOS/A2L.
func ReferenceExcept() ([]string, error) {
	doc, err := loadSetsDoc()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), doc.Except...), nil
}

type setsDoc struct {
	Except []string `yaml:"except"`
	Sets   []Set    `yaml:"sets"`
}

func loadSetsDoc() (setsDoc, error) {
	var doc setsDoc
	b, err := Read("", SetsFile)
	if err != nil {
		return doc, err
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return doc, fmt.Errorf("%s: %w", SetsFile, err)
	}
	return doc, nil
}
