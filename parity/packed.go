package parity

import (
	"fmt"
	"slices"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/record"
)

// Packed is one published definition and the flash image it belongs to.
type Packed struct {
	Stem string
	Img  []byte
	Def  []byte
}

// GradePacked scores the tuner list against each published definition.
// A name counts when that definition stores it. A curve or a map needs
// an axis, including an ordinal one, and a one-cell constant counts
// without one. A second copy of the same name still counts. The star
// uses the definition's origin.
// Confidence reads those bodies from the image. A stem with no definition
// is left out.
func GradePacked(dir string, packed []Packed) ([]Image, error) {
	blocks, err := loadBlocks(dir)
	if err != nil {
		return nil, err
	}
	lists, err := loadNames(dir, blocks)
	if err != nil {
		return nil, err
	}
	if len(lists.tuner) == 0 {
		return nil, nil
	}
	absent, err := loadAbsent(dir, blocks, lists.dims)
	if err != nil {
		return nil, err
	}
	block := blockOf(blocks)
	type held struct {
		stem string
		prov *model.Provenance
		img  []byte
		maps []record.Map
	}
	var rows []held
	for _, p := range packed {
		if len(p.Def) == 0 {
			continue
		}
		maps, prov, err := mapsFromDef(p.Def)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Stem, err)
		}
		// A published file may store two copies of one name. Either copy
		// is the map the zip contains, so the grade keeps one.
		maps = uniqueStored(maps, lists.dims)
		rows = append(rows, held{stem: p.Stem, prov: prov, img: p.Img, maps: maps})
	}
	slices.SortFunc(rows, func(a, b held) int { return strings.Compare(a.stem, b.stem) })
	out := make([]Image, len(rows))
	byStem := map[string]held{}
	for i, h := range rows {
		byStem[h.stem] = h
		blk := block[LayoutID(h.img)]
		dims := publishedDims(h.maps, lists.dims)
		scored := tunerMaps(lists.forBlock(blk), dims, h.maps, nil)
		gone := absent[blk]
		tier := nameGrade(lists.tier, lists.byBlock[blk], lists.any, func(n string) (bool, bool) {
			_, located := scored[n]
			return located || gone[n], located
		})
		out[i] = Image{
			Name:       h.stem + ".bin",
			Tier:       tier,
			NoDef:      noDef(tier, h.prov),
			Provenance: h.prov,
			Axis:       scoreAxes(scored, lists.dims),
			Block:      countScored(lists.byBlock[blk], scored),
		}
	}
	groups, err := loadDatasets(dir)
	if err != nil {
		return nil, err
	}
	skip, err := loadConfidenceSkip(dir)
	if err != nil {
		return nil, err
	}
	for n := range skip {
		if !slices.Contains(lists.tuner, n) {
			return nil, fmt.Errorf("confidence skip: %s is not a tuner name", n)
		}
	}
	confTuner := omitNames(lists.tuner, skip)
	for i, h := range rows {
		var peers []binBody
		for _, stem := range groups[h.stem] {
			o, ok := byStem[stem]
			if !ok {
				continue
			}
			peers = append(peers, binBody{
				stem: stem, img: o.img, maps: o.maps, scored: tunerMaps(lists.tuner, lists.dims, o.maps, nil),
			})
		}
		var byName map[string]Conf
		out[i].Confidence, byName = scoreConfidence(confTuner, publishedDims(h.maps, lists.dims), h.img, h.maps, nil, peers, h.prov != nil && h.prov.Verified)
		out[i].ConfByName = byName
	}
	return out, nil
}

// mapsFromDef reads a published model. Addresses stay file offsets.
// The name is the same one parseModel would use.
func mapsFromDef(b []byte) ([]record.Map, *model.Provenance, error) {
	var m model.Model
	if err := canon.Unmarshal(b, &m); err != nil {
		return nil, nil, err
	}
	if err := m.Check(); err != nil {
		return nil, nil, err
	}
	titles := make([]string, len(m.Objects))
	for i, o := range m.Objects {
		titles[i], _, _ = strings.Cut(strings.TrimSpace(o.ID), " ")
		if titles[i] == "" {
			titles[i] = strings.TrimSpace(o.Description)
		}
	}
	nameOf := titleNamer(titles)
	var maps []record.Map
	for i, o := range m.Objects {
		name := nameOf(titles[i])
		if name == "" {
			continue
		}
		rm := record.Map{
			Name: name, Addr: uint32(o.Address), Bits: o.Data.Bits,
			Signed: o.Data.Signed, Rows: o.Rows, Cols: o.Cols,
		}
		if ax := recAxis(o.X, o.Cols); ax != nil {
			rm.X = ax
		}
		if ax := recAxis(o.Y, o.Rows); ax != nil {
			rm.Y = ax
		}
		maps = append(maps, rm)
	}
	return maps, m.Provenance, nil
}

// uniqueStored keeps one qualifying copy of each name. A scalar needs no
// axis. A curve or a map needs one, including an ordinal axis, unless the
// file stored it as one cell. A name the tuner list does not have is left out.
func uniqueStored(maps []record.Map, dims map[string]int) []record.Map {
	seen := map[string]bool{}
	var out []record.Map
	for _, m := range maps {
		if m.Name == "" || seen[m.Name] {
			continue
		}
		c, known := dims[m.Name]
		if !known || !storedHit(m, c) {
			continue
		}
		seen[m.Name] = true
		out = append(out, m)
	}
	return out
}

// publishedDims lets a name count when its axis is not an image address:
// a one-cell constant, or a map with an ordinal axis.
func publishedDims(maps []record.Map, dims map[string]int) map[string]int {
	out := make(map[string]int, len(dims))
	for k, v := range dims {
		out[k] = v
	}
	for _, m := range maps {
		if _, ok := out[m.Name]; ok && !mapAxis(m) && storedHit(m, 1) {
			out[m.Name] = 0
		}
	}
	return out
}

func storedHit(m record.Map, axes int) bool {
	if mapAxis(m) || axes == 0 || m.X != nil || m.Y != nil {
		return true
	}
	return m.Rows <= 1 && m.Cols <= 1
}

func recAxis(a *model.Axis, count int) *record.Axis {
	if a != nil && a.Source == "ordinal" {
		bits := 0
		if a.Data != nil {
			bits = a.Data.Bits
		}
		return &record.Axis{Count: count, Bits: bits}
	}
	if !located(a) {
		return nil
	}
	return &record.Axis{Addr: uint32(*a.Address), Count: count, Bits: a.Data.Bits}
}
