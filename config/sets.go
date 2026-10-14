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
	b, err := Read("", SetsFile)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Sets []Set `yaml:"sets"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", SetsFile, err)
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
