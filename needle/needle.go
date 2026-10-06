// Package needle applies config/needles.yaml. Field rules are needle_hex,
// optional mask_hex, word alignment, back_up, entry_after, and unique.
// An omitted back_up is DefaultBackUp. An omitted unique is true.
// This package does not add pattern fields.
package needle

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultBackUp is the label offset when a needle omits back_up.
// The label is the hit.
const DefaultBackUp = 0

// Needle is one masked, word-aligned search.
type Needle struct {
	Name       string
	Pattern    []byte
	Mask       []byte
	BackUp     int
	BackUpMax  *int
	Unique     bool
	Function   bool
	EntryAfter [][]byte
}

// Parse reads data and functions lists from YAML bytes. name is used in errors.
// A data label is a table or other bytes. A functions label is a function entry.
// The list is that distinction. At least one of the two lists is required.
func Parse(raw []byte, name string) ([]Needle, error) {
	var doc struct {
		Data      []rawNeedle `yaml:"data"`
		Functions []rawNeedle `yaml:"functions"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Data == nil && doc.Functions == nil {
		return nil, fmt.Errorf("%s: missing data or functions list", name)
	}
	out, err := compileList(doc.Data, false)
	if err != nil {
		return nil, err
	}
	fn, err := compileList(doc.Functions, true)
	if err != nil {
		return nil, err
	}
	return append(out, fn...), nil
}

func compileList(list []rawNeedle, function bool) ([]Needle, error) {
	out := make([]Needle, 0, len(list))
	for _, f := range list {
		ns, err := f.compileAll()
		if err != nil {
			return nil, err
		}
		for i := range ns {
			ns[i].Function = function
		}
		out = append(out, ns...)
	}
	return out, nil
}

type rawNeedle struct {
	Name       string       `yaml:"name"`
	NeedleHex  string       `yaml:"needle_hex"`
	MaskHex    string       `yaml:"mask_hex"`
	BackUp     numOrRange   `yaml:"back_up"`
	Unique     *bool        `yaml:"unique"`
	EntryAfter []string     `yaml:"entry_after"`
	Needles    []needlePart `yaml:"needles"`
}

// needlePart is one prologue. unique, back_up, and entry_after stay on it.
type needlePart struct {
	NeedleHex  string     `yaml:"needle_hex"`
	MaskHex    string     `yaml:"mask_hex"`
	BackUp     numOrRange `yaml:"back_up"`
	Unique     *bool      `yaml:"unique"`
	EntryAfter []string   `yaml:"entry_after"`
}

// numOrRange is a back_up scalar or a two-value [min, max] sequence.
type numOrRange struct {
	lo, hi int
	ranged bool
	set    bool
}

func (n *numOrRange) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		if len(node.Content) != 2 {
			return fmt.Errorf("back_up range wants two values")
		}
		lo, err := parseNum(node.Content[0].Value)
		if err != nil {
			return err
		}
		hi, err := parseNum(node.Content[1].Value)
		if err != nil {
			return err
		}
		n.lo, n.hi, n.ranged, n.set = lo, hi, true, true
		return nil
	case yaml.ScalarNode:
		lo, err := parseNum(node.Value)
		if err != nil {
			return err
		}
		n.lo, n.set = lo, true
		return nil
	default:
		return fmt.Errorf("back_up wants a number or a two-value range")
	}
}

func (f rawNeedle) compileAll() ([]Needle, error) {
	if f.Name == "" {
		return nil, fmt.Errorf("needle missing name")
	}
	if len(f.Needles) == 0 {
		n, err := f.compile()
		if err != nil {
			return nil, err
		}
		return []Needle{n}, nil
	}
	if f.NeedleHex != "" || f.MaskHex != "" || f.BackUp.set || f.Unique != nil || len(f.EntryAfter) > 0 {
		return nil, fmt.Errorf("%s: needles replaces needle_hex", f.Name)
	}
	out := make([]Needle, 0, len(f.Needles))
	for _, p := range f.Needles {
		n, err := (rawNeedle{
			Name: f.Name, NeedleHex: p.NeedleHex, MaskHex: p.MaskHex,
			BackUp: p.BackUp, Unique: p.Unique, EntryAfter: p.EntryAfter,
		}).compile()
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func (f rawNeedle) compile() (Needle, error) {
	if f.Name == "" {
		return Needle{}, fmt.Errorf("needle missing name")
	}
	if strings.TrimSpace(f.NeedleHex) == "" {
		return Needle{}, fmt.Errorf("%s: needle_hex is required", f.Name)
	}
	pat, mask, err := parsePattern(f.NeedleHex)
	if err != nil {
		return Needle{}, fmt.Errorf("%s: %w", f.Name, err)
	}
	if strings.TrimSpace(f.MaskHex) != "" {
		m, err := ParseHex(f.MaskHex)
		if err != nil {
			return Needle{}, fmt.Errorf("%s: mask_hex: %w", f.Name, err)
		}
		if len(m) != len(pat) {
			return Needle{}, fmt.Errorf("%s: mask_hex length %d != needle length %d", f.Name, len(m), len(pat))
		}
		for i := range mask {
			mask[i] &= m[i]
		}
	}
	unique := true
	if f.Unique != nil {
		unique = *f.Unique
	}
	n := Needle{
		Name: f.Name, Pattern: pat, Mask: mask,
		BackUp: DefaultBackUp, Unique: unique,
	}
	if f.BackUp.set {
		lo, hi := f.BackUp.lo, f.BackUp.hi
		n.BackUp = lo
		if f.BackUp.ranged {
			if lo > hi || (hi-lo)%2 != 0 || len(f.EntryAfter) == 0 {
				return Needle{}, fmt.Errorf("%s: back_up range needs min <= max, even span, entry_after", f.Name)
			}
			n.BackUpMax = &hi
		}
	}
	for _, e := range f.EntryAfter {
		b, err := ParseHex(e)
		if err != nil {
			return Needle{}, fmt.Errorf("%s: entry_after: %w", f.Name, err)
		}
		n.EntryAfter = append(n.EntryAfter, b)
	}
	return n, nil
}

func parseNum(s string) (int, error) {
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	}
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return 0, fmt.Errorf("bad number %q", s)
	}
	out := int(v)
	if neg {
		out = -out
	}
	return out, nil
}

// parsePattern splits hex byte tokens. "??" is a wildcard (mask 0).
func parsePattern(s string) (pat, mask []byte, err error) {
	s = strings.Join(strings.Fields(s), "")
	if len(s)%2 != 0 {
		return nil, nil, fmt.Errorf("odd hex length")
	}
	for i := 0; i < len(s); i += 2 {
		tok := s[i : i+2]
		if tok == "??" {
			pat = append(pat, 0)
			mask = append(mask, 0)
			continue
		}
		v, err := strconv.ParseUint(tok, 16, 8)
		if err != nil {
			return nil, nil, fmt.Errorf("bad hex %q", tok)
		}
		pat = append(pat, byte(v))
		mask = append(mask, 0xff)
	}
	return pat, mask, nil
}

// ParseHex reads a byte string. Spaces are ignored.
func ParseHex(s string) ([]byte, error) {
	s = strings.Join(strings.Fields(s), "")
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd hex length")
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		v, err := strconv.ParseUint(s[i:i+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("bad hex %q", s[i:i+2])
		}
		out[i/2] = byte(v)
	}
	return out, nil
}

// Find returns word-aligned hit offsets. C166 instructions are word aligned.
func (n Needle) Find(data []byte) []int {
	var hits []int
	last := len(data) - len(n.Pattern)
	for i := 0; i <= last; i += 2 {
		if matchAt(data, n.Pattern, n.Mask, i) {
			hits = append(hits, i)
		}
	}
	return hits
}

func matchAt(data, pat, mask []byte, i int) bool {
	for j := range pat {
		if data[i+j]&mask[j] != pat[j]&mask[j] {
			return false
		}
	}
	return true
}

// Label is the hit minus back_up. A back_up range walks from the hit toward
// the far end, two bytes at a time, and returns the first label that passes
// EntryOK. If none do, the label is hit-min and EntryOK is false.
func (n Needle) Label(data []byte, hit int) int {
	if n.BackUpMax != nil {
		for label := hit - n.BackUp; label >= hit-*n.BackUpMax; label -= 2 {
			if n.EntryOK(data, label) {
				return label
			}
		}
	}
	return hit - n.BackUp
}

// EntryOK reports whether a label is preceded by one of entry_after.
// An empty list passes every label.
func (n Needle) EntryOK(data []byte, label int) bool {
	if len(n.EntryAfter) == 0 {
		return true
	}
	for _, e := range n.EntryAfter {
		if label < len(e) || label > len(data) {
			continue
		}
		ok := true
		for i := range e {
			if data[label-len(e)+i] != e[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// Report lines match me7probe: hit, miss, ambig, noent.
func (n Needle) Report(data []byte) []string {
	hits := n.Find(data)
	if len(hits) == 0 {
		return []string{fmt.Sprintf("  miss  %s", n.Name)}
	}
	if n.Unique && len(hits) > 1 {
		parts := make([]string, len(hits))
		for i, h := range hits {
			parts[i] = fmt.Sprintf("0x%X", h)
		}
		return []string{fmt.Sprintf("  ambig %s: %s, expected 1", n.Name, strings.Join(parts, " "))}
	}
	var lines []string
	for _, h := range hits {
		label := n.Label(data, h)
		if !n.EntryOK(data, label) {
			lines = append(lines, fmt.Sprintf("  noent %s @ 0x%X: not after RETS/padding", n.Name, label))
			continue
		}
		lines = append(lines, fmt.Sprintf("  hit   %s @ file+0x%X", n.Name, label))
	}
	return lines
}

// Labels returns entry-ok labels. A unique needle with several hits returns none.
func (n Needle) Labels(data []byte) []int {
	hits := n.Find(data)
	if n.Unique && len(hits) > 1 {
		return nil
	}
	var out []int
	for _, h := range hits {
		label := n.Label(data, h)
		if n.EntryOK(data, label) {
			out = append(out, label)
		}
	}
	return out
}

// Revise merges one needle mapping onto base.
// A nil base is a new needle and needs needle_hex.
// function is stored on the result. The mapping is needle fields only.
func Revise(base *Needle, item *yaml.Node, file string, function bool) (Needle, error) {
	var list []Needle
	if base != nil {
		list = []Needle{*base}
	}
	out, err := applyNeedle(list, item, file, function)
	if err != nil {
		return Needle{}, err
	}
	if len(out) != 1 {
		return Needle{}, fmt.Errorf("%s: needle missing", file)
	}
	return out[0], nil
}

// ApplyOverlay merges data and functions from a user file onto base.
// A file with neither list leaves base unchanged.
// An entry under functions is a function entry. An entry under data is not.
// drop: true removes that name. A row with needle_hex replaces the needle.
// A row without needle_hex changes only the fields it lists.
func ApplyOverlay(base []Needle, raw []byte, name string) ([]Needle, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return base, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("%s: expected one document", name)
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && (root.Tag == "!!null" || root.Value == "") {
		return base, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping", name)
	}
	fields, err := MappingFields(root)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	out := base
	for _, key := range []string{"data", "functions"} {
		node, ok := fields[key]
		if !ok {
			continue
		}
		if node.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%s: %s must be a list", name, key)
		}
		for _, item := range node.Content {
			out, err = applyNeedle(out, item, name, key == "functions")
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func applyNeedle(base []Needle, item *yaml.Node, file string, function bool) ([]Needle, error) {
	fields, err := MappingFields(item)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	for k := range fields {
		switch k {
		case "name", "needle_hex", "mask_hex", "back_up", "unique",
			"entry_after", "drop", "needles":
		default:
			return nil, fmt.Errorf("%s: needle: unknown field %s", file, k)
		}
	}
	nameNode, ok := fields["name"]
	if !ok || nameNode.Kind != yaml.ScalarNode || nameNode.Value == "" {
		return nil, fmt.Errorf("%s: needle missing name", file)
	}
	needleName := nameNode.Value
	if node, ok := fields["drop"]; ok {
		var drop bool
		if err := node.Decode(&drop); err != nil {
			return nil, fmt.Errorf("%s: %s: drop: %w", file, needleName, err)
		}
		if drop {
			return dropNeedle(base, file, needleName)
		}
	}
	var raw rawNeedle
	if err := item.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: %s: %w", file, needleName, err)
	}
	if _, ok := fields["needles"]; ok {
		if _, ok := fields["needle_hex"]; ok {
			return nil, fmt.Errorf("%s: %s: needles replaces needle_hex", file, needleName)
		}
		ns, err := raw.compileAll()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for i := range ns {
			ns[i].Function = function
		}
		return replaceNeedles(base, ns), nil
	}
	if _, ok := fields["needle_hex"]; ok {
		n, err := raw.compile()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		n.Function = function
		return replaceNeedle(base, n), nil
	}
	idx := -1
	for i := range base {
		if base[i].Name == needleName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%s: %s: needle_hex is required for a new needle", file, needleName)
	}
	n := cloneNeedle(base[idx])
	if _, ok := fields["entry_after"]; ok {
		n.EntryAfter = nil
		for _, e := range raw.EntryAfter {
			b, err := ParseHex(e)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: entry_after: %w", file, needleName, err)
			}
			n.EntryAfter = append(n.EntryAfter, b)
		}
	}
	if _, ok := fields["back_up"]; ok {
		if !raw.BackUp.set {
			return nil, fmt.Errorf("%s: %s: back_up wants a number or a two-value range", file, needleName)
		}
		n.BackUp = raw.BackUp.lo
		n.BackUpMax = nil
		if raw.BackUp.ranged {
			if raw.BackUp.lo > raw.BackUp.hi || (raw.BackUp.hi-raw.BackUp.lo)%2 != 0 || len(n.EntryAfter) == 0 {
				return nil, fmt.Errorf("%s: %s: back_up range needs min <= max, even span, entry_after", file, needleName)
			}
			hi := raw.BackUp.hi
			n.BackUpMax = &hi
		}
	}
	if _, ok := fields["mask_hex"]; ok {
		m, err := ParseHex(raw.MaskHex)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: mask_hex: %w", file, needleName, err)
		}
		if len(m) != len(n.Pattern) {
			return nil, fmt.Errorf("%s: %s: mask_hex length %d != needle length %d", file, needleName, len(m), len(n.Pattern))
		}
		for i := range n.Mask {
			n.Mask[i] &= m[i]
		}
	}
	if _, ok := fields["unique"]; ok {
		if raw.Unique == nil {
			return nil, fmt.Errorf("%s: %s: unique wants true or false", file, needleName)
		}
		n.Unique = *raw.Unique
	}
	n.Function = function
	base[idx] = n
	return base, nil
}

func replaceNeedles(base, ns []Needle) []Needle {
	if len(ns) == 0 {
		return base
	}
	name := ns[0].Name
	out := make([]Needle, 0, len(base)+len(ns))
	placed := false
	for _, n := range base {
		if n.Name != name {
			out = append(out, n)
			continue
		}
		if !placed {
			out = append(out, ns...)
			placed = true
		}
	}
	if !placed {
		out = append(out, ns...)
	}
	return out
}

func replaceNeedle(base []Needle, n Needle) []Needle {
	for i := range base {
		if base[i].Name == n.Name {
			base[i] = n
			return base
		}
	}
	return append(base, n)
}

func dropNeedle(base []Needle, file, name string) ([]Needle, error) {
	out := make([]Needle, 0, len(base))
	found := false
	for _, n := range base {
		if n.Name == name {
			found = true
			continue
		}
		out = append(out, n)
	}
	if !found {
		return nil, fmt.Errorf("%s: drop %s: no such needle", file, name)
	}
	return out, nil
}

func cloneNeedle(n Needle) Needle {
	n.Pattern = append([]byte(nil), n.Pattern...)
	n.Mask = append([]byte(nil), n.Mask...)
	if n.BackUpMax != nil {
		v := *n.BackUpMax
		n.BackUpMax = &v
	}
	if len(n.EntryAfter) > 0 {
		cp := make([][]byte, len(n.EntryAfter))
		for i, e := range n.EntryAfter {
			cp[i] = append([]byte(nil), e...)
		}
		n.EntryAfter = cp
	}
	return n
}

// MappingFields returns the keys of a YAML mapping.
func MappingFields(n *yaml.Node) (map[string]*yaml.Node, error) {
	if n.Kind != yaml.MappingNode || len(n.Content)%2 != 0 {
		return nil, fmt.Errorf("expected a mapping")
	}
	out := make(map[string]*yaml.Node, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("mapping key is not a scalar")
		}
		if _, ok := out[k.Value]; ok {
			return nil, fmt.Errorf("duplicate key %s", k.Value)
		}
		out[k.Value] = n.Content[i+1]
	}
	return out, nil
}

// ByName returns the needle with that name.
func ByName(ns []Needle, name string) (Needle, bool) {
	for _, n := range ns {
		if n.Name == name {
			return n, true
		}
	}
	return Needle{}, false
}
