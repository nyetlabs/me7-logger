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

// Conf is one tuner's body-confidence grade and a short reason.
// Reason is a fixed token, or a peer token plus stem and cell detail.
type Conf struct {
	High   bool
	Reason string
}

// Comment formats a low-confidence note for a raw auto object.
// High confidence leaves the needle alone (empty string when needle is empty).
// Low reasons are short phrases, not token soup. Calling again with a
// previous Comment result does not double the note.
func (c Conf) Comment(needle string) string {
	needle = stripConfNote(needle)
	if c.High || c.Reason == "" {
		return needle
	}
	detail := lowPhrase(c.Reason)
	if needle == "" || needle == detail {
		return detail
	}
	return needle + ": " + detail
}

// stripConfNote keeps the locate needle when comment is "needle: note".
func stripConfNote(s string) string {
	if i := strings.Index(s, ": "); i >= 0 {
		return s[:i]
	}
	if i := strings.Index(s, " confidence "); i >= 0 {
		return s[:i]
	}
	return s
}

func lowPhrase(reason string) string {
	switch reason {
	case "axis":
		return "axis not monotonic"
	case "axis-zeros":
		return "all zeros"
	case "shape":
		return "missing row or column axis"
	case "body":
		return "body unreadable"
	case "zeros":
		return "zero body with no zero peer"
	}
	if stem, ok := strings.CutPrefix(reason, "zeros "); ok {
		return "zero body, " + stem + " is not zero"
	}
	if rest, ok := strings.CutPrefix(reason, "peer-diff "); ok {
		if n, m, stem, ok := splitPeerDetail(rest); ok {
			return fmt.Sprintf("differs from %s by %d/%d cells", stem, n, m)
		}
		return "cannot compare body to " + rest
	}
	if rest, ok := strings.CutPrefix(reason, "peer-neighbor "); ok {
		if n, m, stem, ok := splitPeerDetail(rest); ok {
			return fmt.Sprintf("close to %s (%d/%d) but neighbors differ", stem, n, m)
		}
		return "neighbors differ vs " + rest
	}
	return reason
}

// splitPeerDetail parses "diff/cells stem" from a peer reason tail.
func splitPeerDetail(s string) (diff, cells int, stem string, ok bool) {
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		return 0, 0, "", false
	}
	nm := strings.SplitN(parts[0], "/", 2)
	if len(nm) != 2 {
		return 0, 0, "", false
	}
	if _, err := fmt.Sscanf(nm[0]+" "+nm[1], "%d %d", &diff, &cells); err != nil {
		return 0, 0, "", false
	}
	return diff, cells, parts[1], true
}

// scoreConfidence judges the body of each name the locator already scored.
// The denominator is that matched set. A name the locator missed is not in it.
// High means the body bytes hold up. A shared axis does not change that.
// A verified definition skips the peer comparison: a sibling calibration
// may differ. byName has one entry per scored name.
func scoreConfidence(tuner []string, dims map[string]int, img []byte, maps []record.Map, rows []refRow, peers []binBody, verified bool) (Fraction, map[string]Conf) {
	scored := tunerMaps(tuner, dims, maps, rows)
	byName := make(map[string]Conf, len(scored))
	high := 0
	for _, n := range tuner {
		m, ok := scored[n]
		if !ok {
			continue
		}
		c, known := dims[n]
		if !known {
			c = -1
		}
		ok, reason := plausible(img, m, c)
		if ok {
			ok, reason = confident(img, maps, m, peers, verified)
		}
		byName[n] = Conf{High: ok, Reason: reason}
		if ok {
			high++
		}
	}
	return Fraction{high, len(scored)}, byName
}

// plausible needs no peer. A map listed with two axes needs a row and a
// column count, an image axis rises or falls without a turn, and a byte body
// whose every other byte is closer than its neighbors is a word table.
func plausible(img []byte, m record.Map, axes int) (bool, string) {
	if axes == 2 && (m.Rows == 0 && m.Y == nil || m.Cols == 0 && m.X == nil) {
		return false, "shape"
	}
	for _, a := range []*record.Axis{m.X, m.Y} {
		if a == nil {
			continue
		}
		if reason := axisFail(img, a); reason != "" {
			return false, reason
		}
	}
	return true, ""
}

// axisFail is empty when the axis is ordered, "axis-zeros" when the
// breakpoint bytes are all zero, or "axis" for any other turn or flat run.
func axisFail(img []byte, a *record.Axis) string {
	if a.Addr == 0 || a.Count < 2 {
		return ""
	}
	step := 1
	if a.Bits == 16 {
		step = 2
	}
	off := int(opcode.FileOffset(a.Addr))
	n := a.Count * step
	if off < 0 || off+n > len(img) {
		return ""
	}
	raw := img[off : off+n]
	u := make([]int, a.Count)
	s := make([]int, a.Count)
	for i := range u {
		if step == 2 {
			w := uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
			u[i], s[i] = int(w), int(int16(w))
		} else {
			u[i], s[i] = int(raw[i]), int(int8(raw[i]))
		}
	}
	if monotonic(u) || monotonic(s) {
		return ""
	}
	if allZero(raw) {
		return "axis-zeros"
	}
	return "axis"
}

func ordered(img []byte, a *record.Axis) bool {
	return axisFail(img, a) == ""
}

func monotonic(v []int) bool {
	up, down := true, true
	for i := 1; i < len(v); i++ {
		up = up && v[i] >= v[i-1]
		down = down && v[i] <= v[i-1]
	}
	return (up || down) && v[0] != v[len(v)-1]
}

type binBody struct {
	stem   string
	img    []byte
	maps   []record.Map
	scored map[string]record.Map
}

func confident(img []byte, maps []record.Map, m record.Map, peers []binBody, verified bool) (bool, string) {
	cells, _, raw, ok := bodyOf(img, m)
	if !ok {
		return false, "body"
	}
	// The empty-body rule is for more than one cell of zeros.
	if cells > 1 && allZero(raw) {
		return zerosMatch(m, raw, peers)
	}
	if verified {
		return true, "verified"
	}
	if cells < largeBody {
		return true, "short"
	}
	var stems []string
	nearReason := ""
	for _, p := range peers {
		other, ok := p.scored[m.Name]
		if !ok {
			continue
		}
		ok, reason := bodiesMatch(img, maps, m, p.img, p.maps, other, p.stem)
		if !ok {
			return false, reason
		}
		stems = append(stems, p.stem)
		if strings.HasPrefix(reason, "peer-near") {
			nearReason = reason
		}
	}
	if len(stems) == 0 {
		return true, "no-peer"
	}
	if nearReason != "" {
		return true, nearReason
	}
	return true, "peer-same " + strings.Join(stems, ",")
}

func bodiesMatch(img []byte, maps []record.Map, m record.Map, pimg []byte, pmaps []record.Map, other record.Map, stem string) (bool, string) {
	diff, cells, ok := diffCells(img, m, pimg, other)
	if !ok {
		return false, "peer-diff " + stem
	}
	detail := fmt.Sprintf("%d/%d %s", diff, cells, stem)
	if diff == 0 {
		return true, "peer-same " + stem
	}
	if !nearDiff(diff, cells) {
		return false, "peer-diff " + detail
	}
	if !neighborsMatch(img, maps, m, pimg, pmaps) {
		return false, "peer-neighbor " + detail
	}
	return true, "peer-near " + detail
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
func zerosMatch(m record.Map, raw []byte, peers []binBody) (bool, string) {
	if len(peers) == 0 {
		return false, "zeros"
	}
	var stems []string
	for _, p := range peers {
		other, ok := p.scored[m.Name]
		if !ok {
			continue
		}
		_, _, praw, pok := bodyOf(p.img, other)
		if !pok || len(praw) != len(raw) || !allZero(praw) {
			return false, "zeros " + p.stem
		}
		stems = append(stems, p.stem)
	}
	if len(stems) == 0 {
		return false, "zeros"
	}
	return true, "peer-same " + strings.Join(stems, ",")
}

func tunerMaps(want []string, dims map[string]int, maps []record.Map, rows []refRow) map[string]record.Map {
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
