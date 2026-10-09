package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/mapfile"
	"go.nyet.org/me7-logger/needle"
)

// ResolveUserDir returns dir when it exists. An empty dir means no overlay.
// The default directory may be absent. An explicit path that does not exist is an error.
func ResolveUserDir(dir string, explicit bool) (string, error) {
	if dir == "" {
		return "", nil
	}
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return "", nil
		}
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return dir, nil
}

// LoadMeasures reads the measurement list, then user YAML in dir.
// Conversions come from names.yaml, then this file, then user YAML.
// userDir empty skips the overlay. A non-empty missing directory is an error.
// A row with an address is an error. Omitted size, bitmask, unit, signed,
// inverse, factor, and offset are DefaultSize, 0, "", false, false, 1, and 0.
// After every file is merged, a named conversion fills the omitted fields
// it sets. A size and unit then fill omitted signed, inverse, factor, and
// offset. A named conversion is not that size+unit default, so its unit may
// be "". A single-bit bitmask does not use the size+unit table.
// needle_hex on the row locates the mem operand. A row with no needle is a
// stub. stub: true in a user file drops the needle and keeps the scale.
func LoadMeasures(measPath, namesPath, userDir string) ([]Measure, error) {
	label := configLabel(measPath, MeasuresFile)
	b, err := Read(measPath, MeasuresFile)
	if err != nil {
		return nil, err
	}
	root, err := parseRoot(b, label)
	if err != nil {
		return nil, err
	}
	if _, ok := root["functions"]; ok {
		return nil, fmt.Errorf("%s: functions belong in a needle file or in config/user", label)
	}
	if _, ok := root["data"]; ok {
		return nil, fmt.Errorf("%s: data belongs in a needle file or in config/user", label)
	}
	measNode, ok := root["measurements"]
	if !ok {
		return nil, fmt.Errorf("%s: missing measurements list", label)
	}
	rows, err := applyMeasurements(nil, measNode, label, false)
	if err != nil {
		return nil, err
	}
	convs, err := conversionTable(measPath, namesPath, userDir)
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
		if n, ok := doc["measurements"]; ok {
			rows, err = applyMeasurements(rows, n, f, true)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := fillConversions(rows, convs); err != nil {
		return nil, err
	}
	out := make([]Measure, len(rows))
	for i := range rows {
		rows[i].m.Stub = rows[i].m.Needle == nil
		out[i] = rows[i].m
	}
	return out, nil
}

// LoadNeedles reads the needle file, then data and functions from user YAML in dir.
// userDir empty skips the overlay. A non-empty missing directory is an error.
func LoadNeedles(path, userDir string) ([]needle.Needle, error) {
	b, err := Read(path, NeedlesFile)
	if err != nil {
		return nil, err
	}
	ns, err := needle.Parse(b, NeedlesFile)
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
		if _, err := parseRoot(raw, f); err != nil {
			return nil, err
		}
		ns, err = needle.ApplyOverlay(ns, raw, f)
		if err != nil {
			return nil, err
		}
	}
	return ns, nil
}

func configLabel(path, name string) string {
	if path == "" || path == filepath.Join(Dir(), name) {
		return name
	}
	return path
}

// LoadScales reads the shared conversions from names.yaml, the measurement
// file, and user YAML. A catalog row uses a scale only when its size matches.
func LoadScales(measPath, namesPath, userDir string) (map[mapfile.ScaleKey]mapfile.Scale, error) {
	tab, err := conversionTable(measPath, namesPath, userDir)
	if err != nil {
		return nil, err
	}
	out := make(map[mapfile.ScaleKey]mapfile.Scale, len(tab.byScale))
	for k, c := range tab.byScale {
		out[mapfile.ScaleKey{Size: k.size, Unit: k.unit}] = mapfile.Scale{
			Signed: c.signed, Inverse: c.inverse, A: c.factor, B: c.offset,
			SetSigned: c.set.signed, SetInverse: c.set.inverse, SetA: c.set.factor, SetB: c.set.offset,
		}
	}
	return out, nil
}

func conversionTable(measPath, namesPath, userDir string) (convTable, error) {
	namesLabel := configLabel(namesPath, NamesFile)
	nb, err := Read(namesPath, NamesFile)
	if err != nil {
		return convTable{}, err
	}
	namesDoc, err := documentFields(nb, namesLabel)
	if err != nil {
		return convTable{}, err
	}
	var tab convTable
	if n, ok := namesDoc["conversions"]; ok {
		tab, err = applyConversions(tab, n, namesLabel, false)
		if err != nil {
			return convTable{}, err
		}
	}
	measLabel := configLabel(measPath, MeasuresFile)
	mb, err := Read(measPath, MeasuresFile)
	if err != nil {
		return convTable{}, err
	}
	measDoc, err := parseRoot(mb, measLabel)
	if err != nil {
		return convTable{}, err
	}
	if n, ok := measDoc["conversions"]; ok {
		tab, err = applyConversions(tab, n, measLabel, false)
		if err != nil {
			return convTable{}, err
		}
	}
	files, err := userFiles(userDir)
	if err != nil {
		return convTable{}, err
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return convTable{}, err
		}
		doc, err := parseRoot(raw, f)
		if err != nil {
			return convTable{}, err
		}
		if n, ok := doc["conversions"]; ok {
			tab, err = applyConversions(tab, n, f, true)
			if err != nil {
				return convTable{}, err
			}
		}
	}
	return tab, nil
}

func userFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	st, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

func parseRoot(b []byte, name string) (map[string]*yaml.Node, error) {
	fields, err := documentFields(b, name)
	if err != nil {
		return nil, err
	}
	for k := range fields {
		switch k {
		case "conversions", "measurements", "data", "functions", "maps":
		default:
			return nil, fmt.Errorf("%s: unknown field %s", name, k)
		}
	}
	return fields, nil
}

func documentFields(b []byte, name string) (map[string]*yaml.Node, error) {
	if len(strings.TrimSpace(string(b))) == 0 {
		return map[string]*yaml.Node{}, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("%s: expected one document", name)
	}
	n := doc.Content[0]
	if n.Kind == yaml.ScalarNode && (n.Tag == "!!null" || n.Value == "") {
		return map[string]*yaml.Node{}, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping", name)
	}
	fields, err := needle.MappingFields(n)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return fields, nil
}

type convKey struct {
	size int
	unit string
}

type convSet struct {
	size, unit, signed, inverse, factor, offset bool
}

// conversion is one scale. A name is selected by a row. A conversion
// without a name is the size+unit default. A named conversion is not
// also that default, so unit "" can be the torque word and the speed word.
type conversion struct {
	name            string
	size            int
	unit            string
	signed, inverse bool
	factor, offset  float64
	set             convSet
}

type convTable struct {
	byScale map[convKey]conversion
	byName  map[string]conversion
}

func applyConversions(dst convTable, n *yaml.Node, file string, allowDrop bool) (convTable, error) {
	if n.Kind != yaml.SequenceNode {
		return convTable{}, fmt.Errorf("%s: conversions must be a list", file)
	}
	if dst.byScale == nil {
		dst.byScale = map[convKey]conversion{}
	}
	if dst.byName == nil {
		dst.byName = map[string]conversion{}
	}
	for _, item := range n.Content {
		key, conv, drop, err := parseConversion(item, file)
		if err != nil {
			return convTable{}, err
		}
		if conv.name != "" {
			if drop {
				if !allowDrop {
					return convTable{}, fmt.Errorf("%s: drop is only valid in config/user", file)
				}
				if _, ok := dst.byName[conv.name]; !ok {
					return convTable{}, fmt.Errorf("%s: drop conversion %s: no such conversion", file, conv.name)
				}
				delete(dst.byName, conv.name)
				continue
			}
			dst.byName[conv.name] = mergeConversion(dst.byName[conv.name], conv)
			continue
		}
		if drop {
			if !allowDrop {
				return convTable{}, fmt.Errorf("%s: drop is only valid in config/user", file)
			}
			if _, ok := dst.byScale[key]; !ok {
				return convTable{}, fmt.Errorf("%s: drop size %d unit %q: no such conversion", file, key.size, key.unit)
			}
			delete(dst.byScale, key)
			continue
		}
		dst.byScale[key] = mergeConversion(dst.byScale[key], conv)
	}
	return dst, nil
}

func mergeConversion(dst, src conversion) conversion {
	if src.name != "" {
		dst.name = src.name
	}
	if src.set.size {
		dst.size = src.size
		dst.set.size = true
	}
	if src.set.unit {
		dst.unit = src.unit
		dst.set.unit = true
	}
	if src.set.signed {
		dst.signed = src.signed
		dst.set.signed = true
	}
	if src.set.inverse {
		dst.inverse = src.inverse
		dst.set.inverse = true
	}
	if src.set.factor {
		dst.factor = src.factor
		dst.set.factor = true
	}
	if src.set.offset {
		dst.offset = src.offset
		dst.set.offset = true
	}
	return dst
}

func parseConversion(n *yaml.Node, file string) (convKey, conversion, bool, error) {
	fields, err := needle.MappingFields(n)
	if err != nil {
		return convKey{}, conversion{}, false, fmt.Errorf("%s: %w", file, err)
	}
	for k := range fields {
		switch k {
		case "name", "size", "unit", "signed", "inverse", "factor", "offset", "drop":
		default:
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion: unknown field %s", file, k)
		}
	}
	var c conversion
	if node, ok := fields["name"]; ok {
		c.name, err = scalarString(node)
		if err != nil || c.name == "" {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion needs a name", file)
		}
	}
	if _, ok := fields["unit"]; !ok && c.name == "" {
		return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion needs a unit", file)
	}
	size := DefaultSize
	if node, ok := fields["size"]; ok {
		size, err = scalarInt(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion size: %w", file, err)
		}
		c.size = size
		c.set.size = true
	}
	if c.name == "" {
		c.size = size
	}
	if node, ok := fields["unit"]; ok {
		c.unit, err = scalarString(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion unit: %w", file, err)
		}
		c.set.unit = true
	}
	key := convKey{size: size, unit: c.unit}
	if node, ok := fields["drop"]; ok {
		drop, err := scalarBool(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion drop: %w", file, err)
		}
		if drop {
			return key, c, true, nil
		}
	}
	if node, ok := fields["signed"]; ok {
		c.signed, err = scalarBool(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion signed: %w", file, err)
		}
		c.set.signed = true
	}
	if node, ok := fields["inverse"]; ok {
		c.inverse, err = scalarBool(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion inverse: %w", file, err)
		}
		c.set.inverse = true
	}
	if node, ok := fields["factor"]; ok {
		c.factor, err = scalarFloat(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion factor: %w", file, err)
		}
		c.set.factor = true
	}
	if node, ok := fields["offset"]; ok {
		c.offset, err = scalarFloat(node)
		if err != nil {
			return convKey{}, conversion{}, false, fmt.Errorf("%s: conversion offset: %w", file, err)
		}
		c.set.offset = true
	}
	return key, c, false, nil
}

type measureSet struct {
	stub, note, size, bitmask, unit, signed, inverse, factor, offset, alias, comment, conversion, bit bool
}

type measureDraft struct {
	m          Measure
	set        measureSet
	conversion string
	needleItem *yaml.Node
}

func applyMeasurements(rows []measureDraft, n *yaml.Node, file string, allowDrop bool) ([]measureDraft, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: measurements must be a list", file)
	}
	for _, item := range n.Content {
		d, drop, err := parseMeasurement(item, file)
		if err != nil {
			return nil, err
		}
		if drop {
			if !allowDrop {
				return nil, fmt.Errorf("%s: drop is only valid in config/user", file)
			}
			next, found := dropMeasure(rows, d.m.Name)
			if !found {
				return nil, fmt.Errorf("%s: drop %s: no such measurement", file, d.m.Name)
			}
			rows = next
			continue
		}
		if allowDrop {
			if i := measureIndex(rows, d.m.Name); i >= 0 {
				merged, err := mergeMeasure(rows[i], d, file)
				if err != nil {
					return nil, err
				}
				rows[i] = merged
				continue
			}
		}
		d, err = attachNeedle(d, nil, file)
		if err != nil {
			return nil, err
		}
		rows = append(rows, d)
	}
	return rows, nil
}

func measureIndex(rows []measureDraft, name string) int {
	for i := range rows {
		if rows[i].m.Name == name {
			return i
		}
	}
	return -1
}

func dropMeasure(rows []measureDraft, name string) ([]measureDraft, bool) {
	out := make([]measureDraft, 0, len(rows))
	found := false
	for _, r := range rows {
		if r.m.Name == name {
			found = true
			continue
		}
		out = append(out, r)
	}
	return out, found
}

func mergeMeasure(dst, src measureDraft, file string) (measureDraft, error) {
	if src.set.stub && src.m.Stub && src.needleItem != nil {
		return measureDraft{}, fmt.Errorf("%s: %s: stub: true drops the needle", file, src.m.Name)
	}
	if src.needleItem != nil {
		var err error
		dst, err = attachNeedle(dst, src.needleItem, file)
		if err != nil {
			return measureDraft{}, err
		}
	}
	if src.set.stub && src.m.Stub {
		dst.m.Needle = nil
		dst.m.Stub = true
		dst.set.stub = true
	}
	if src.set.stub && !src.m.Stub {
		if dst.m.Needle == nil {
			return measureDraft{}, fmt.Errorf("%s: %s: stub: false needs needle_hex", file, src.m.Name)
		}
		dst.m.Stub = false
		dst.set.stub = true
	}
	if src.set.note {
		dst.m.Note = src.m.Note
		dst.set.note = true
	}
	if src.set.size {
		dst.m.Size = src.m.Size
		dst.set.size = true
	}
	if src.set.bitmask {
		dst.m.Bitmask = src.m.Bitmask
		dst.set.bitmask = true
	}
	if src.set.unit {
		dst.m.Unit = src.m.Unit
		dst.set.unit = true
	}
	if src.set.signed {
		dst.m.Signed = src.m.Signed
		dst.set.signed = true
	}
	if src.set.inverse {
		dst.m.Inverse = src.m.Inverse
		dst.set.inverse = true
	}
	if src.set.factor {
		dst.m.A = src.m.A
		dst.set.factor = true
	}
	if src.set.offset {
		dst.m.B = src.m.B
		dst.set.offset = true
	}
	if src.set.alias {
		dst.m.Alias = src.m.Alias
		dst.set.alias = true
	}
	if src.set.comment {
		dst.m.Comment = src.m.Comment
		dst.set.comment = true
	}
	if src.set.bit {
		dst.m.Bit = src.m.Bit
		dst.set.bit = true
	}
	if src.set.conversion {
		dst.conversion = src.conversion
		dst.set.conversion = true
	}
	dst.m.Stub = dst.m.Needle == nil
	return dst, nil
}

// attachNeedle compiles item onto dst. A nil item compiles dst.needleItem
// onto an empty base, which is a new row.
func attachNeedle(dst measureDraft, item *yaml.Node, file string) (measureDraft, error) {
	if item == nil {
		item = dst.needleItem
	}
	if item == nil {
		if dst.set.stub && !dst.m.Stub && dst.m.Needle == nil {
			return measureDraft{}, fmt.Errorf("%s: %s: stub: false needs needle_hex", file, dst.m.Name)
		}
		dst.m.Stub = dst.m.Needle == nil
		return dst, nil
	}
	var base *needle.Needle
	if dst.m.Needle != nil {
		cp := *dst.m.Needle
		base = &cp
	}
	n, err := needle.Revise(base, item, file, false)
	if err != nil {
		return measureDraft{}, err
	}
	if measureBackUpOmitted(item) {
		n.BackUp = DefaultMeasureBackUp
	}
	dst.m.Needle = &n
	dst.m.Stub = false
	dst.needleItem = nil
	return dst, nil
}

func parseMeasurement(n *yaml.Node, file string) (measureDraft, bool, error) {
	fields, err := needle.MappingFields(n)
	if err != nil {
		return measureDraft{}, false, fmt.Errorf("%s: %w", file, err)
	}
	for k := range fields {
		switch k {
		case "name", "stub", "note", "size", "bitmask", "unit", "signed", "inverse",
			"factor", "offset", "alias", "comment", "drop", "address", "conversion",
			"needle_hex", "mask_hex", "back_up", "unique", "entry_after", "bit":
		default:
			return measureDraft{}, false, fmt.Errorf("%s: measurement: unknown field %s", file, k)
		}
	}
	nameNode, ok := fields["name"]
	if !ok {
		return measureDraft{}, false, fmt.Errorf("%s: measurement missing name", file)
	}
	name, err := scalarString(nameNode)
	if err != nil || name == "" {
		return measureDraft{}, false, fmt.Errorf("%s: measurement missing name", file)
	}
	if _, ok := fields["address"]; ok {
		return measureDraft{}, false, fmt.Errorf("%s: %s: fixed address is not accepted; add needle_hex or leave the row a stub", file, name)
	}
	if node, ok := fields["drop"]; ok {
		drop, err := scalarBool(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: drop: %w", file, name, err)
		}
		if drop {
			return measureDraft{m: Measure{Name: name}}, true, nil
		}
	}
	// Omitted size, bitmask, unit, signed, inverse, factor, and offset are
	// DefaultSize, 0, "", false, false, 1, and 0. set stays false so a
	// size+unit pair can still replace signed, inverse, factor, and offset.
	d := measureDraft{m: Measure{Name: name, Size: DefaultSize, A: 1}}
	if node, ok := fields["stub"]; ok {
		d.m.Stub, err = scalarBool(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: stub: %w", file, name, err)
		}
		d.set.stub = true
	}
	if node, ok := fields["note"]; ok {
		d.m.Note, err = scalarString(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: note: %w", file, name, err)
		}
		d.set.note = true
	}
	if node, ok := fields["size"]; ok {
		d.m.Size, err = scalarInt(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: size: %w", file, name, err)
		}
		d.set.size = true
	}
	if node, ok := fields["bitmask"]; ok {
		mask, err := scalarBitmask(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: bitmask: %w", file, name, err)
		}
		d.m.Bitmask = mask
		d.set.bitmask = true
	}
	if node, ok := fields["unit"]; ok {
		d.m.Unit, err = scalarString(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: unit: %w", file, name, err)
		}
		d.set.unit = true
	}
	if node, ok := fields["signed"]; ok {
		d.m.Signed, err = scalarBool(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: signed: %w", file, name, err)
		}
		d.set.signed = true
	}
	if node, ok := fields["inverse"]; ok {
		d.m.Inverse, err = scalarBool(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: inverse: %w", file, name, err)
		}
		d.set.inverse = true
	}
	if node, ok := fields["factor"]; ok {
		d.m.A, err = scalarFloat(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: factor: %w", file, name, err)
		}
		d.set.factor = true
	}
	if node, ok := fields["offset"]; ok {
		d.m.B, err = scalarFloat(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: offset: %w", file, name, err)
		}
		d.set.offset = true
	}
	if node, ok := fields["alias"]; ok {
		d.m.Alias, err = scalarString(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: alias: %w", file, name, err)
		}
		d.set.alias = true
	}
	if node, ok := fields["comment"]; ok {
		d.m.Comment, err = scalarString(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: comment: %w", file, name, err)
		}
		d.set.comment = true
	}
	if node, ok := fields["bit"]; ok {
		d.m.Bit, err = scalarBool(node)
		if err != nil {
			return measureDraft{}, false, fmt.Errorf("%s: %s: bit: %w", file, name, err)
		}
		d.set.bit = true
	}
	if node, ok := fields["conversion"]; ok {
		d.conversion, err = scalarString(node)
		if err != nil || d.conversion == "" {
			return measureDraft{}, false, fmt.Errorf("%s: %s: conversion needs a name", file, name)
		}
		d.set.conversion = true
	}
	d.needleItem = needleItem(fields)
	if d.needleItem != nil && d.set.stub && d.m.Stub {
		return measureDraft{}, false, fmt.Errorf("%s: %s: stub: true drops the needle", file, name)
	}
	return d, false, nil
}

// measureBackUpOmitted reports a new needle that does not set back_up.
// A patch that leaves the pattern alone keeps the back_up it already has.
func measureBackUpOmitted(item *yaml.Node) bool {
	fields, err := needle.MappingFields(item)
	if err != nil {
		return false
	}
	_, hex := fields["needle_hex"]
	_, back := fields["back_up"]
	return hex && !back
}

// needleItem is the needle fields on a measurement row.
// Nil means the row does not change the needle.
func needleItem(fields map[string]*yaml.Node) *yaml.Node {
	keys := []string{"name", "needle_hex", "mask_hex", "back_up", "unique", "entry_after"}
	n := &yaml.Node{Kind: yaml.MappingNode}
	found := false
	for _, k := range keys {
		v, ok := fields[k]
		if !ok {
			continue
		}
		if k != "name" {
			found = true
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, v)
	}
	if !found {
		return nil
	}
	return n
}

// fillConversions writes a named conversion onto omitted fields, then the
// size+unit table onto omitted signed, inverse, factor, and offset.
// A named conversion is not a size+unit default. Its unit may be "".
// A field the row sets is kept. A single-bit mask skips the size+unit table.
// Fields still omitted stay at bitmask 0, unit "", signed false, inverse
// false, factor 1, and offset 0.
func fillConversions(rows []measureDraft, tab convTable) error {
	for i := range rows {
		if rows[i].conversion != "" {
			c, ok := tab.byName[rows[i].conversion]
			if !ok {
				return fmt.Errorf("measurement %s: unknown conversion %s", rows[i].m.Name, rows[i].conversion)
			}
			applyNamed(&rows[i], c)
		}
		if singleBit(rows[i].m.Bitmask) {
			if !rows[i].set.factor {
				rows[i].m.A = 1
			}
			continue
		}
		c, ok := tab.byScale[convKey{rows[i].m.Size, rows[i].m.Unit}]
		if !ok {
			continue
		}
		if !rows[i].set.signed && c.set.signed {
			rows[i].m.Signed = c.signed
		}
		if !rows[i].set.inverse && c.set.inverse {
			rows[i].m.Inverse = c.inverse
		}
		if !rows[i].set.factor && c.set.factor {
			rows[i].m.A = c.factor
		}
		if !rows[i].set.offset && c.set.offset {
			rows[i].m.B = c.offset
		}
	}
	return nil
}

func applyNamed(row *measureDraft, c conversion) {
	if !row.set.size && c.set.size {
		row.m.Size = c.size
		row.set.size = true
	}
	if !row.set.unit && c.set.unit {
		row.m.Unit = c.unit
		row.set.unit = true
	}
	if !row.set.signed && c.set.signed {
		row.m.Signed = c.signed
		row.set.signed = true
	}
	if !row.set.inverse && c.set.inverse {
		row.m.Inverse = c.inverse
		row.set.inverse = true
	}
	if !row.set.factor && c.set.factor {
		row.m.A = c.factor
		row.set.factor = true
	}
	if !row.set.offset && c.set.offset {
		row.m.B = c.offset
		row.set.offset = true
	}
}

func singleBit(mask uint16) bool {
	return mask != 0 && mask&(mask-1) == 0
}

func scalarBitmask(n *yaml.Node) (uint16, error) {
	if n.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("expected a bitmask")
	}
	s := strings.TrimSpace(n.Value)
	if s == "" || s == "~" || s == "null" {
		return 0, nil
	}
	v, err := mapfile.ParseUint(s)
	if err != nil {
		return 0, err
	}
	if v > 0xffff {
		return 0, fmt.Errorf("%s does not fit in a bitmask", n.Value)
	}
	return uint16(v), nil
}

func scalarString(n *yaml.Node) (string, error) {
	var v string
	if err := n.Decode(&v); err != nil {
		return "", err
	}
	return v, nil
}

func scalarBool(n *yaml.Node) (bool, error) {
	var v bool
	if err := n.Decode(&v); err != nil {
		return false, err
	}
	return v, nil
}

func scalarInt(n *yaml.Node) (int, error) {
	var v int
	if err := n.Decode(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func scalarFloat(n *yaml.Node) (float64, error) {
	if n.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("expected a number")
	}
	return mapfile.ParseRatio(n.Value)
}
