// Package mapfile reads the result-type catalog and the alias file.
// The copies shipped with this repo are config/catalog/ and config/aliases.yaml.
package mapfile

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Var is one (result type, bitmask) row.
type Var struct {
	ResultType int
	Bitmask    uint16
	Name       string
	Size       int
	Unit       string
	Signed     bool
	Inverse    bool
	A, B       float64
	Comment    string
}

// Table is keyed by result type, then bitmask.
// ByName keeps the first row of each name. Later rows that share a result
// type and bitmask still replace ByRT. The signature pass looks up by name.
type Table struct {
	ByRT   map[int]map[uint16]Var
	ByName map[string]Var
}

// Rows returns every catalog row for a result type, ordered by bitmask.
func (t Table) Rows(rt int) []Var {
	if t.ByRT == nil {
		return nil
	}
	m := t.ByRT[rt]
	if len(m) == 0 {
		return nil
	}
	masks := make([]int, 0, len(m))
	for mask := range m {
		masks = append(masks, int(mask))
	}
	sort.Ints(masks)
	out := make([]Var, len(masks))
	for i, mask := range masks {
		out[i] = m[uint16(mask)]
	}
	return out
}

// Lookup returns the map row for a result type and bitmask.
func (t Table) Lookup(rt int, mask uint16) (Var, bool) {
	if t.ByRT == nil {
		return Var{}, false
	}
	m, ok := t.ByRT[rt]
	if !ok {
		return Var{}, false
	}
	v, ok := m[mask]
	return v, ok
}

// ScaleKey is the size and unit a shared conversion applies to.
type ScaleKey struct {
	Size int
	Unit string
}

// Scale is one shared conversion. Set fields replace omitted row fields.
type Scale struct {
	Signed     bool
	Inverse    bool
	A, B       float64
	SetSigned  bool
	SetInverse bool
	SetA       bool
	SetB       bool
}

// RowScale is a named catalog scale, selected by a row's scale field.
// Set fields replace omitted row fields. A field written on the row wins.
type RowScale struct {
	Size      int
	Unit      string
	Signed    bool
	A, B      float64
	SetSize   bool
	SetUnit   bool
	SetSigned bool
	SetA      bool
	SetB      bool
}

// Parse parses catalog YAML. name is used in errors.
// An omitted size is 0, so the image chooses the width.
// An omitted bitmask, unit, signed, inverse, factor, and offset are
// 0, "", false, false, 1, and 0. A factor written on the row is kept, including 0.
// scales fills omitted signed, inverse, factor, and offset when the row's size
// and unit match. A nil map leaves those defaults. Size 0 does not match a
// word or byte scale.
// named fills omitted fields from a row's scale name. A nil map leaves a
// scale name as an error. A field written on the row wins over both.
// A later row with the same result type and bitmask replaces an earlier one.
func Parse(b []byte, name string, scales map[ScaleKey]Scale, named map[string]RowScale) (Table, error) {
	var raw struct {
		Variables []catalogRow `yaml:"variables"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return Table{}, fmt.Errorf("%s: %w", name, err)
	}
	t := Table{ByRT: map[int]map[uint16]Var{}, ByName: map[string]Var{}}
	for i, r := range raw.Variables {
		if len(r.RT) == 0 {
			return Table{}, fmt.Errorf("%s: row %d: %s: missing rt", name, i+1, r.Name)
		}
		for _, key := range r.RT {
			v, err := r.varRow(key, scales, named)
			if err != nil {
				return Table{}, fmt.Errorf("%s: row %d: %w", name, i+1, err)
			}
			if t.ByRT[v.ResultType] == nil {
				t.ByRT[v.ResultType] = map[uint16]Var{}
			}
			t.ByRT[v.ResultType][v.Bitmask] = v
			if _, ok := t.ByName[v.Name]; !ok {
				t.ByName[v.Name] = v
			}
		}
	}
	if len(t.ByRT) == 0 {
		return t, fmt.Errorf("%s: no map rows", name)
	}
	return t, nil
}

// catalogRow is one name at one or more result types. Each rt entry is a
// result type, or result type/bitmask for a bit row.
type catalogRow struct {
	RT      oneOrMore  `yaml:"rt"`
	Name    string     `yaml:"name"`
	Scale   string     `yaml:"scale"`
	Size    *int       `yaml:"size"`
	Unit    *string    `yaml:"unit"`
	Signed  *bool      `yaml:"signed"`
	Inverse bool       `yaml:"inverse"`
	Factor  *flexFloat `yaml:"factor"`
	Offset  *flexFloat `yaml:"offset"`
	Comment string     `yaml:"comment"`
}

// ParseScales reads the named catalog scales. name is used in errors.
func ParseScales(b []byte, name string) (map[string]RowScale, error) {
	var raw struct {
		Scales map[string]scaleBody `yaml:"scales"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(raw.Scales) == 0 {
		return nil, fmt.Errorf("%s: no scales", name)
	}
	out := make(map[string]RowScale, len(raw.Scales))
	for k, body := range raw.Scales {
		if strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("%s: empty scale name", name)
		}
		out[k] = body.rowScale()
	}
	return out, nil
}

type scaleBody struct {
	Size   *int       `yaml:"size"`
	Unit   *string    `yaml:"unit"`
	Signed *bool      `yaml:"signed"`
	Factor *flexFloat `yaml:"factor"`
	Offset *flexFloat `yaml:"offset"`
}

func (s scaleBody) rowScale() RowScale {
	var out RowScale
	if s.Size != nil {
		out.Size = *s.Size
		out.SetSize = true
	}
	if s.Unit != nil {
		out.Unit = *s.Unit
		out.SetUnit = true
	}
	if s.Signed != nil {
		out.Signed = *s.Signed
		out.SetSigned = true
	}
	if s.Factor != nil {
		out.A = float64(*s.Factor)
		out.SetA = true
	}
	if s.Offset != nil {
		out.B = float64(*s.Offset)
		out.SetB = true
	}
	return out
}

// flexFloat is a decimal, a hex integer, or a ratio such as 2/0x10000.
// An absent factor stays nil so the row default remains 1.
type flexFloat float64

func (f *flexFloat) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected a number")
	}
	v, err := ParseRatio(n.Value)
	if err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

// oneOrMore is a scalar or a list of scalars.
type oneOrMore []string

func (o *oneOrMore) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*o = []string{n.Value}
		return nil
	}
	var l []string
	if err := n.Decode(&l); err != nil {
		return err
	}
	*o = l
	return nil
}

func (r catalogRow) varRow(key string, scales map[ScaleKey]Scale, named map[string]RowScale) (Var, error) {
	if strings.TrimSpace(r.Name) == "" {
		return Var{}, fmt.Errorf("missing name")
	}
	rts, masks, _ := strings.Cut(key, "/")
	rt, err := ParseUint(rts)
	if err != nil {
		return Var{}, fmt.Errorf("%s rt: %w", r.Name, err)
	}
	var mask uint64
	if strings.TrimSpace(masks) != "" {
		mask, err = ParseUint(masks)
		if err != nil {
			return Var{}, fmt.Errorf("%s bitmask: %w", r.Name, err)
		}
	}
	size := 0
	sizeSet := r.Size != nil
	if sizeSet {
		size = *r.Size
	}
	unit := ""
	unitSet := r.Unit != nil
	if unitSet {
		unit = *r.Unit
	}
	signed := false
	signedSet := r.Signed != nil
	if signedSet {
		signed = *r.Signed
	}
	factor := 1.0
	factorSet := r.Factor != nil
	if factorSet {
		factor = float64(*r.Factor)
	}
	offset := 0.0
	offsetSet := r.Offset != nil
	if offsetSet {
		offset = float64(*r.Offset)
	}
	if r.Scale != "" {
		s, ok := named[r.Scale]
		if !ok {
			return Var{}, fmt.Errorf("%s: unknown scale %s", r.Name, r.Scale)
		}
		if !sizeSet && s.SetSize {
			size = s.Size
			sizeSet = true
		}
		if !unitSet && s.SetUnit {
			unit = s.Unit
			unitSet = true
		}
		if !signedSet && s.SetSigned {
			signed = s.Signed
			signedSet = true
		}
		if !factorSet && s.SetA {
			factor = s.A
			factorSet = true
		}
		if !offsetSet && s.SetB {
			offset = s.B
			offsetSet = true
		}
	}
	if s, ok := scales[ScaleKey{size, unit}]; ok {
		if !signedSet && s.SetSigned {
			signed = s.Signed
		}
		if !factorSet && s.SetA {
			factor = s.A
		}
		if !offsetSet && s.SetB {
			offset = s.B
		}
	}
	return Var{
		ResultType: int(rt),
		Bitmask:    uint16(mask),
		Name:       r.Name,
		Size:       size,
		Unit:       unit,
		Signed:     signed,
		Inverse:    r.Inverse,
		A:          factor,
		B:          offset,
		Comment:    r.Comment,
	}, nil
}

// Aliases maps a variable name to its alias. Empty aliases are omitted.
// An empty path returns an empty map.
func Aliases(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseAliases(b)
}

// ParseAliases parses an aliases map of name to alias. An empty alias is omitted.
func ParseAliases(b []byte) (map[string]string, error) {
	var raw struct {
		Aliases map[string]string `yaml:"aliases"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for name, alias := range raw.Aliases {
		name, alias = strings.TrimSpace(name), strings.TrimSpace(alias)
		if name == "" {
			return nil, fmt.Errorf("alias %q: missing name", alias)
		}
		if alias != "" {
			out[name] = alias
		}
	}
	return out, nil
}

// ParseUint accepts a decimal or a 0x hex integer.
func ParseUint(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	}
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	return strconv.ParseUint(s, base, 32)
}

// ParseRatio accepts a decimal, a hex integer, or a ratio such as 100/0x10000.
// 100/0x10000 is one percent of a 16-bit word. 1/0x10000 is the fraction of the word.
func ParseRatio(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		num, err := parseSigned(s[:i])
		if err != nil {
			return 0, err
		}
		den, err := parseSigned(s[i+1:])
		if err != nil {
			return 0, err
		}
		if den == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return num / den, nil
	}
	return parseSigned(s)
}

func parseSigned(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = strings.TrimSpace(s[1:])
	}
	var v float64
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		u, err := strconv.ParseUint(s[2:], 16, 64)
		if err != nil {
			return 0, fmt.Errorf("bad number %q", s)
		}
		v = float64(u)
	} else {
		var err error
		v, err = strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("bad number %q", s)
		}
	}
	if neg {
		v = -v
	}
	return v, nil
}
