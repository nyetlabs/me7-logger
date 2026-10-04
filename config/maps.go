package config

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"me7-logger/mapfile"
	"me7-logger/record"
)

// LoadMaps reads config/maps.yaml. path empty uses the shipped file.
func LoadMaps(path string) ([]record.Call, error) {
	b, err := Read(path, MapsFile)
	if err != nil {
		return nil, err
	}
	return ParseMaps(b, MapsFile)
}

// ParseMaps reads a maps list. name is used in errors.
// at is a distance from the caller label, even, and not an address.
// Two rows may share a name. One caller and distance may not.
func ParseMaps(raw []byte, name string) ([]record.Call, error) {
	var doc struct {
		Maps []struct {
			Name   string `yaml:"name"`
			Caller string `yaml:"caller"`
			At     string `yaml:"at"`
			Interp string `yaml:"interp"`
		} `yaml:"maps"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Maps == nil {
		return nil, fmt.Errorf("%s: missing maps list", name)
	}
	seen := map[[2]string]string{}
	out := make([]record.Call, 0, len(doc.Maps))
	for _, r := range doc.Maps {
		if r.Name == "" || r.Caller == "" || r.Interp == "" || r.At == "" {
			return nil, fmt.Errorf("%s: a map needs name, caller, at, and interp", name)
		}
		at, err := mapfile.ParseUint(r.At)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: at: %w", name, r.Name, err)
		}
		if at == 0 || at%2 != 0 {
			return nil, fmt.Errorf("%s: %s: at must be a positive even distance", name, r.Name)
		}
		key := [2]string{r.Caller, r.At}
		if prev, ok := seen[key]; ok {
			return nil, fmt.Errorf("%s: %s and %s share %s+%s", name, prev, r.Name, r.Caller, r.At)
		}
		seen[key] = r.Name
		out = append(out, record.Call{
			Name: r.Name, Caller: r.Caller, At: int(at), Interp: r.Interp,
		})
	}
	return out, nil
}
