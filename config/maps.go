package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"

	"me7-logger/mapfile"
	"me7-logger/record"
)

// LoadMaps reads config/maps.yaml, then maps lists from user YAML in dir.
// path empty uses the shipped file. userDir empty skips the overlay.
// A user row is appended. One caller and distance may not be repeated.
func LoadMaps(path, userDir string) ([]record.Call, error) {
	label := configLabel(path, MapsFile)
	b, err := Read(path, MapsFile)
	if err != nil {
		return nil, err
	}
	calls, err := ParseMaps(b, label)
	if err != nil {
		return nil, err
	}
	files, err := userFiles(userDir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		doc, err := parseRoot(raw, f)
		if err != nil {
			return nil, err
		}
		n, ok := doc["maps"]
		if !ok {
			continue
		}
		extra, err := parseMapNode(n, f)
		if err != nil {
			return nil, err
		}
		calls, err = appendMapCalls(calls, extra, f)
		if err != nil {
			return nil, err
		}
	}
	return calls, nil
}

// ParseMaps reads a maps list. name is used in errors.
// at is a distance from the caller label, even, and not an address.
// Two rows may share a name. One caller and distance may not.
func ParseMaps(raw []byte, name string) ([]record.Call, error) {
	var doc struct {
		Maps []mapRow `yaml:"maps"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Maps == nil {
		return nil, fmt.Errorf("%s: missing maps list", name)
	}
	return compileMapRows(doc.Maps, name)
}

type mapRow struct {
	Name   string `yaml:"name"`
	Caller string `yaml:"caller"`
	At     atList `yaml:"at"`
	Interp string `yaml:"interp"`
	Rows   int    `yaml:"rows"`
	YBits  int    `yaml:"ybits"`
	YTable string `yaml:"ytable"`
}

// atList is one distance or several on the same caller and interp.
type atList []string

func (a *atList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*a = atList{node.Value}
		return nil
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			return fmt.Errorf("at wants a distance")
		}
		vals := make(atList, 0, len(node.Content))
		for _, c := range node.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("at wants a distance")
			}
			vals = append(vals, c.Value)
		}
		*a = vals
		return nil
	default:
		return fmt.Errorf("at wants a distance or a list")
	}
}

func parseMapNode(n *yaml.Node, file string) ([]record.Call, error) {
	if n.Kind == yaml.ScalarNode && (n.Tag == "!!null" || n.Value == "") {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: maps must be a list", file)
	}
	var rows []mapRow
	if err := n.Decode(&rows); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return compileMapRows(rows, file)
}

func compileMapRows(rows []mapRow, name string) ([]record.Call, error) {
	seen := map[[2]string]string{}
	out := make([]record.Call, 0, len(rows))
	for _, r := range rows {
		if r.Name == "" || r.Caller == "" || r.Interp == "" || len(r.At) == 0 {
			return nil, fmt.Errorf("%s: a map needs name, caller, at, and interp", name)
		}
		if (r.YTable == "") != (r.Rows == 0) {
			return nil, fmt.Errorf("%s: %s: ytable and rows are set together", name, r.Name)
		}
		ybits := r.YBits
		if r.YTable != "" && ybits == 0 {
			ybits = 8
		}
		if ybits != 0 && ybits != 8 && ybits != 16 {
			return nil, fmt.Errorf("%s: %s: ybits is 8 or 16", name, r.Name)
		}
		for _, raw := range r.At {
			at, err := mapfile.ParseUint(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: at: %w", name, r.Name, err)
			}
			if at == 0 || at%2 != 0 {
				return nil, fmt.Errorf("%s: %s: at must be a positive even distance", name, r.Name)
			}
			key := [2]string{r.Caller, strconv.Itoa(int(at))}
			if prev, ok := seen[key]; ok {
				return nil, fmt.Errorf("%s: %s and %s share %s+0x%X", name, prev, r.Name, r.Caller, at)
			}
			seen[key] = r.Name
			out = append(out, record.Call{
				Name: r.Name, Caller: r.Caller, At: int(at), Interp: r.Interp,
				Rows: r.Rows, YBits: ybits, YTable: r.YTable,
			})
		}
	}
	return out, nil
}

func appendMapCalls(base, extra []record.Call, file string) ([]record.Call, error) {
	seen := map[[2]string]string{}
	for _, c := range base {
		seen[[2]string{c.Caller, strconv.Itoa(c.At)}] = c.Name
	}
	for _, c := range extra {
		key := [2]string{c.Caller, strconv.Itoa(c.At)}
		if prev, ok := seen[key]; ok {
			return nil, fmt.Errorf("%s: %s and %s share %s+0x%X", file, prev, c.Name, c.Caller, c.At)
		}
		seen[key] = c.Name
		base = append(base, c)
	}
	return base, nil
}
