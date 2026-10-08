package parity

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

const largeBody = 16

// nearCells is the smallest difference a large body may have.
// nearPercent is the extra room on a bigger map, as a percent of its cells.
const (
	nearCells   = 2
	nearPercent = 3
)

// scoreConfidence judges the body of each name the locator already scored.
// The denominator is that matched set. A name the locator missed is not in it.
// High means the body bytes hold up. A shared axis does not change that.
func scoreConfidence(wiki []string, dims map[string]int, img []byte, maps []record.Map, rows []refRow, peers []binBody) Fraction {
	scored := wikiMaps(wiki, dims, maps, rows)
	high := 0
	for _, n := range wiki {
		m, ok := scored[n]
		if !ok {
			continue
		}
		if confident(img, maps, m, peers) {
			high++
		}
	}
	return Fraction{high, len(scored)}
}

type binBody struct {
	img    []byte
	maps   []record.Map
	scored map[string]record.Map
}

func confident(img []byte, maps []record.Map, m record.Map, peers []binBody) bool {
	cells, _, raw, ok := bodyOf(img, m)
	if !ok {
		return false
	}
	// A one-byte body is one cell. The empty-body rule is for a longer run of zeros.
	if len(raw) > 1 && allZero(raw) {
		return zerosMatch(m, raw, peers)
	}
	if cells < largeBody {
		return true
	}
	for _, p := range peers {
		other, ok := p.scored[m.Name]
		if !ok {
			continue
		}
		if !bodiesMatch(img, maps, m, p.img, p.maps, other) {
			return false
		}
	}
	return true
}

func bodiesMatch(img []byte, maps []record.Map, m record.Map, pimg []byte, pmaps []record.Map, other record.Map) bool {
	diff, cells, ok := diffCells(img, m, pimg, other)
	if !ok {
		return false
	}
	if diff == 0 {
		return true
	}
	if !nearDiff(diff, cells) {
		return false
	}
	return neighborsMatch(img, maps, m, pimg, pmaps)
}

// nearDiff is a large body whose cells are close: two cells, or 3% of the body.
func nearDiff(diff, cells int) bool {
	if cells < largeBody {
		return false
	}
	if diff <= nearCells {
		return true
	}
	return diff*100 <= cells*nearPercent
}

func neighborsMatch(img []byte, maps []record.Map, m record.Map, pimg []byte, pmaps []record.Map) bool {
	prev, next, hasPrev, hasNext := adjacent(maps, m)
	if !hasPrev && !hasNext {
		return false
	}
	if hasPrev && !neighborOK(img, prev, pimg, pmaps) {
		return false
	}
	if hasNext && !neighborOK(img, next, pimg, pmaps) {
		return false
	}
	return true
}

func neighborOK(img []byte, n record.Map, pimg []byte, pmaps []record.Map) bool {
	other, ok := soleMap(pmaps, n.Name)
	if !ok {
		return false
	}
	diff, cells, ok := diffCells(img, n, pimg, other)
	if !ok {
		return false
	}
	return diff == 0 || nearDiff(diff, cells)
}

func diffCells(img []byte, a record.Map, pimg []byte, b record.Map) (diff, cells int, ok bool) {
	ac, aw, ar, aok := bodyOf(img, a)
	bc, bw, br, bok := bodyOf(pimg, b)
	if !aok || !bok || ac != bc || aw != bw || len(ar) != len(br) {
		return 0, 0, false
	}
	step := aw / 8
	n := 0
	for i := 0; i < len(ar); i += step {
		if !bytes.Equal(ar[i:i+step], br[i:i+step]) {
			n++
		}
	}
	return n, ac, true
}

func adjacent(maps []record.Map, m record.Map) (prev, next record.Map, hasPrev, hasNext bool) {
	list := namedMaps(maps)
	off := opcode.FileOffset(m.Addr)
	idx := -1
	for i, p := range list {
		if p.Name == m.Name && opcode.FileOffset(p.Addr) == off {
			idx = i
			break
		}
	}
	if idx < 0 {
		return record.Map{}, record.Map{}, false, false
	}
	if idx > 0 {
		prev, hasPrev = list[idx-1], true
	}
	if idx+1 < len(list) {
		next, hasNext = list[idx+1], true
	}
	return prev, next, hasPrev, hasNext
}

func namedMaps(maps []record.Map) []record.Map {
	out := make([]record.Map, 0, len(maps))
	for _, m := range maps {
		if m.Name != "" {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return opcode.FileOffset(out[i].Addr) < opcode.FileOffset(out[j].Addr)
	})
	return out
}

func soleMap(maps []record.Map, name string) (record.Map, bool) {
	var found record.Map
	n := 0
	for _, m := range maps {
		if m.Name != name {
			continue
		}
		n++
		found = m
	}
	if n != 1 {
		return record.Map{}, false
	}
	return found, true
}

func bodyOf(img []byte, m record.Map) (cells, width int, raw []byte, ok bool) {
	width = m.Bits
	if width != 8 && width != 16 {
		width = 8
	}
	switch {
	case m.Rows == 0 && m.Cols == 0 && m.X != nil && m.Y != nil && m.X.Count > 0 && m.Y.Count > 0:
		cells = m.X.Count * m.Y.Count
	case m.Rows == 0 && m.Cols == 0 && m.X != nil && m.X.Count > 0:
		cells = m.X.Count
	case m.Rows == 0 && m.Cols == 0 && m.Y != nil && m.Y.Count > 0:
		cells = m.Y.Count
	case m.Rows == 0 && m.Cols == 0:
		cells = 1
	case m.Rows == 0:
		cells = m.Cols
	case m.Cols == 0:
		cells = m.Rows
	default:
		cells = m.Rows * m.Cols
	}
	n := cells * width / 8
	off := int(opcode.FileOffset(m.Addr))
	if n == 0 || off < 0 || off+n > len(img) {
		return cells, width, nil, false
	}
	return cells, width, img[off : off+n], true
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// zerosMatch is high when every sibling that has this name is zero too.
// A body with no sibling stays low.
func zerosMatch(m record.Map, raw []byte, peers []binBody) bool {
	if len(peers) == 0 {
		return false
	}
	seen := false
	for _, p := range peers {
		other, ok := p.scored[m.Name]
		if !ok {
			continue
		}
		_, _, praw, pok := bodyOf(p.img, other)
		if !pok || len(praw) != len(raw) || !allZero(praw) {
			return false
		}
		seen = true
	}
	return seen
}

func wikiMaps(want []string, dims map[string]int, maps []record.Map, rows []refRow) map[string]record.Map {
	addrs := map[string]map[uint32]record.Map{}
	axis := map[string]map[uint32]struct{}{}
	for _, m := range maps {
		if m.Name == "" {
			continue
		}
		off := opcode.FileOffset(m.Addr)
		if addrs[m.Name] == nil {
			addrs[m.Name] = map[uint32]record.Map{}
			axis[m.Name] = map[uint32]struct{}{}
		}
		addrs[m.Name][off] = m
		if mapAxis(m) {
			axis[m.Name][off] = struct{}{}
		}
	}
	out := map[string]record.Map{}
	for _, n := range want {
		if len(addrs[n]) != 1 {
			continue
		}
		for off, m := range addrs[n] {
			c, known := dims[n]
			if _, ok := axis[n][off]; !ok && !(known && c == 0) {
				continue
			}
			if referenceHit(m, rows) {
				out[n] = m
			}
		}
	}
	return out
}

func loadDatasets(dir string) (map[string][]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "datasets.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var groups map[string][]string
	if err := yaml.Unmarshal(b, &groups); err != nil {
		return nil, err
	}
	peers := map[string][]string{}
	for _, stems := range groups {
		for _, stem := range stems {
			var others []string
			for _, o := range stems {
				if o != stem {
					others = append(others, o)
				}
			}
			peers[stem] = others
		}
	}
	return peers, nil
}

// loadConfidenceSkip reads testdata/parity/confidence.yaml.
// A missing file leaves every scored name in the confidence denominator.
func loadConfidenceSkip(dir string) (map[string]struct{}, error) {
	b, err := os.ReadFile(filepath.Join(dir, "confidence.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Skip []string `yaml:"skip"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("confidence skip: %w", err)
	}
	out := make(map[string]struct{}, len(doc.Skip))
	for _, n := range doc.Skip {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := out[n]; ok {
			return nil, fmt.Errorf("confidence skip: %s repeated", n)
		}
		out[n] = struct{}{}
	}
	return out, nil
}

func omitNames(names []string, skip map[string]struct{}) []string {
	if len(skip) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := skip[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}
