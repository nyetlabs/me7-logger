// Package needle applies config/needles.yaml. Field rules are needle_hex,
// optional mask_hex, word alignment, back_up, entry_after, and unique.
// An omitted back_up is DefaultBackUp. An omitted unique is true.
// This package does not add pattern fields.
package needle

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

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
	// Pats is every pattern when needle_hex is a list. Pattern and Mask
	// are Pats[0]. Empty means the single Pattern and Mask.
	Pats  [][]byte
	Masks [][]byte
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
	NeedleHex  hexList      `yaml:"needle_hex"`
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
	if !f.NeedleHex.empty() || f.MaskHex != "" || f.BackUp.set || f.Unique != nil || len(f.EntryAfter) > 0 {
		return nil, fmt.Errorf("%s: needles replaces needle_hex", f.Name)
	}
	out := make([]Needle, 0, len(f.Needles))
	for _, p := range f.Needles {
		n, err := (rawNeedle{
			Name: f.Name, NeedleHex: hexList{items: []string{p.NeedleHex}}, MaskHex: p.MaskHex,
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
	if f.NeedleHex.empty() {
		return Needle{}, fmt.Errorf("%s: needle_hex is required", f.Name)
	}
	if len(f.NeedleHex.items) > 1 && strings.TrimSpace(f.MaskHex) != "" {
		return Needle{}, fmt.Errorf("%s: mask_hex applies to one needle_hex", f.Name)
	}
	pats := make([][]byte, len(f.NeedleHex.items))
	masks := make([][]byte, len(f.NeedleHex.items))
	for i, hex := range f.NeedleHex.items {
		hex, err := expandPattern(hex)
		if err != nil {
			return Needle{}, fmt.Errorf("%s: %w", f.Name, err)
		}
		pat, mask, err := Compile(hex)
		if err != nil {
			return Needle{}, fmt.Errorf("%s: %w", f.Name, err)
		}
		pats[i], masks[i] = pat, mask
	}
	if strings.TrimSpace(f.MaskHex) != "" {
		m, err := ParseHex(f.MaskHex)
		if err != nil {
			return Needle{}, fmt.Errorf("%s: mask_hex: %w", f.Name, err)
		}
		if len(m) != len(pats[0]) {
			return Needle{}, fmt.Errorf("%s: mask_hex length %d != needle length %d", f.Name, len(m), len(pats[0]))
		}
		for i := range masks[0] {
			masks[0][i] &= m[i]
		}
	}
	unique := true
	if f.Unique != nil {
		unique = *f.Unique
	}
	n := Needle{
		Name: f.Name, Pattern: pats[0], Mask: masks[0],
		BackUp: DefaultBackUp, Unique: unique,
	}
	if len(pats) > 1 {
		n.Pats, n.Masks = pats, masks
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

// hexList is one needle_hex string or a list of them.
type hexList struct {
	items []string
}

func (h *hexList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		h.items = []string{n.Value}
		return nil
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			return fmt.Errorf("needle_hex list is empty")
		}
		h.items = make([]string, len(n.Content))
		for i, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("needle_hex list wants strings")
			}
			h.items[i] = c.Value
		}
		return nil
	default:
		return fmt.Errorf("needle_hex wants a string or a list")
	}
}

func (h hexList) empty() bool {
	for _, s := range h.items {
		if strings.TrimSpace(s) != "" {
			return false
		}
	}
	return true
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

// expandPattern replaces a repeated instruction with its bytes.
// {push:9876} is MOV [-R0], R9 then R8, R7, R6. Each digit is a register.
// {sub:4} is SUB R0, #4. {extp:0206} is EXTP #0x0206, #1.
func expandPattern(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			return "", fmt.Errorf("unclosed {")
		}
		body := s[i+1 : i+j]
		i += j + 1
		name, arg, ok := strings.Cut(body, ":")
		if !ok || name == "" {
			return "", fmt.Errorf("pattern ref %q wants a name and an argument", body)
		}
		got, err := expandMacro(name, arg)
		if err != nil {
			return "", err
		}
		b.WriteString(got)
	}
	return b.String(), nil
}

func expandMacro(name, arg string) (string, error) {
	arg = strings.Join(strings.Fields(arg), "")
	switch name {
	case "push":
		if arg == "" {
			return "", fmt.Errorf("push wants registers")
		}
		var b strings.Builder
		for i := 0; i < len(arg); i++ {
			c := arg[i]
			if c >= 'a' && c <= 'f' {
				c = c - 'a' + 'A'
			}
			if !isHexNibble(c) {
				return "", fmt.Errorf("push register %q", arg[i:i+1])
			}
			fmt.Fprintf(&b, "88 %c0 ", c)
		}
		return strings.TrimSpace(b.String()), nil
	case "sub":
		v, err := oneByte(arg)
		if err != nil {
			return "", fmt.Errorf("sub: %w", err)
		}
		return fmt.Sprintf("28 %02X", v), nil
	case "extp":
		if len(arg) != 4 {
			return "", fmt.Errorf("extp wants a 4-digit page")
		}
		page, err := strconv.ParseUint(arg, 16, 16)
		if err != nil {
			return "", fmt.Errorf("extp page %q", arg)
		}
		return fmt.Sprintf("D7 40 %02X %02X", byte(page), byte(page>>8)), nil
	default:
		return "", fmt.Errorf("unknown pattern macro %q", name)
	}
}

func isHexNibble(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f'
}

func oneByte(s string) (byte, error) {
	if s == "" || len(s) > 2 {
		return 0, fmt.Errorf("wants one byte")
	}
	v, err := strconv.ParseUint(s, 16, 8)
	if err != nil {
		return 0, fmt.Errorf("%q", s)
	}
	return byte(v), nil
}

// Compile reads a byte pattern. Spaces are ignored. A byte is two hex digits,
// X is a nibble wildcard, and [VV/MM] is a byte whose bits under MM equal VV.
func Compile(s string) (pat, mask []byte, err error) {
	s = strings.Join(strings.Fields(s), "")
	for i := 0; i < len(s); {
		if s[i] == '[' {
			if i+7 > len(s) || s[i+3] != '/' || s[i+6] != ']' {
				return nil, nil, fmt.Errorf("bad bitmask byte at %q", s[i:])
			}
			v, err1 := strconv.ParseUint(s[i+1:i+3], 16, 8)
			m, err2 := strconv.ParseUint(s[i+4:i+6], 16, 8)
			if err1 != nil || err2 != nil || v&^m != 0 {
				return nil, nil, fmt.Errorf("bad bitmask byte %q", s[i:i+7])
			}
			pat, mask = append(pat, byte(v)), append(mask, byte(m))
			i += 7
			continue
		}
		if i+1 >= len(s) {
			return nil, nil, fmt.Errorf("odd hex length")
		}
		var p, m byte
		for _, c := range []byte{s[i], s[i+1]} {
			p, m = p<<4, m<<4
			if c == 'X' {
				continue
			}
			v, err := strconv.ParseUint(string(c), 16, 8)
			if err != nil {
				return nil, nil, fmt.Errorf("bad hex %q", s[i:i+2])
			}
			p, m = p|byte(v), m|0x0F
		}
		pat, mask = append(pat, p), append(mask, m)
		i += 2
	}
	if len(pat) == 0 {
		return nil, nil, fmt.Errorf("empty pattern")
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
// Every pattern in a needle_hex list is searched. The same offset is one hit.
func (n Needle) Find(data []byte) []int {
	if len(n.Pats) == 0 {
		return scan(data, n.Pattern, n.Mask)
	}
	var hits []int
	for i := range n.Pats {
		hits = append(hits, scan(data, n.Pats[i], n.Masks[i])...)
	}
	if len(hits) < 2 {
		return hits
	}
	sort.Ints(hits)
	w := 1
	for i := 1; i < len(hits); i++ {
		if hits[i] != hits[w-1] {
			hits[w] = hits[i]
			w++
		}
	}
	return hits[:w]
}

func scan(data, pat, mask []byte) []int {
	return Hits(data, pat, mask, 0, len(data), 0)
}

// Hits returns word-aligned offsets of pat under mask in data[lo:hi).
// limit caps the result; 0 keeps every hit. A hit is the first byte of pat.
func Hits(data, pat, mask []byte, lo, hi, limit int) []int {
	if len(pat) == 0 || len(mask) != len(pat) || limit < 0 {
		return nil
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(data) {
		hi = len(data)
	}
	maxStart := hi - len(pat)
	if lo%2 != 0 {
		lo++
	}
	if lo > maxStart {
		return nil
	}
	if ix := indexFor(data); ix != nil {
		if off, posts, ok := ix.window(pat, mask); ok {
			return hitsIndexed(data, pat, mask, posts, off, lo, maxStart, limit)
		}
	}
	off, needle := solidRun(pat, mask)
	if needle == nil {
		var hits []int
		for i := lo; i <= maxStart; i += 2 {
			if !matchAt(data, pat, mask, i) {
				continue
			}
			hits = append(hits, i)
			if limit > 0 && len(hits) >= limit {
				return hits
			}
		}
		return hits
	}
	var hits []int
	pos := lo + off
	end := maxStart + off + len(needle)
	if end > len(data) {
		end = len(data)
	}
	for pos < end {
		j := bytes.Index(data[pos:end], needle)
		if j < 0 {
			break
		}
		abs := pos + j
		start := abs - off
		pos = abs + 1
		if start < lo || start > maxStart || start%2 != 0 {
			continue
		}
		if !matchAt(data, pat, mask, start) {
			continue
		}
		hits = append(hits, start)
		if limit > 0 && len(hits) >= limit {
			return hits
		}
	}
	return hits
}

// LastHit is the greatest word-aligned offset of pat under mask in data[lo:hi),
// or -1 when there is none.
func LastHit(data, pat, mask []byte, lo, hi int) int {
	if len(pat) == 0 || len(mask) != len(pat) {
		return -1
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(data) {
		hi = len(data)
	}
	maxStart := hi - len(pat)
	if lo%2 != 0 {
		lo++
	}
	if lo > maxStart {
		return -1
	}
	if ix := indexFor(data); ix != nil {
		if off, posts, ok := ix.window(pat, mask); ok {
			return lastIndexed(data, pat, mask, posts, off, lo, maxStart)
		}
	}
	off, needle := solidRun(pat, mask)
	if needle == nil {
		for i := maxStart; i >= lo; i-- {
			if i%2 != 0 {
				continue
			}
			if matchAt(data, pat, mask, i) {
				return i
			}
		}
		return -1
	}
	nLo := lo + off
	nHi := maxStart + off + len(needle)
	if nHi > len(data) {
		nHi = len(data)
	}
	for nLo < nHi {
		j := bytes.LastIndex(data[nLo:nHi], needle)
		if j < 0 {
			return -1
		}
		abs := nLo + j
		start := abs - off
		// Keep a needle that overlaps this one and starts earlier.
		// abs+len-1 still holds a needle at abs-1, and drops the one at abs.
		nHi = abs + len(needle) - 1
		if start < lo || start > maxStart || start%2 != 0 {
			continue
		}
		if matchAt(data, pat, mask, start) {
			return start
		}
	}
	return -1
}

// wordIndex lists every file offset of a 2-byte window, grouped by the
// little-endian word.
type wordIndex struct {
	start []int
	pos   []int32
}

// span is the index for one Bind. Bytes must not change until unbind.
type span struct {
	p  *byte
	n  int
	ix *wordIndex
}

// One image is indexed at a time. A Bind that finds the slot taken searches
// without the index, so a second Generate stays correct.
var bound atomic.Pointer[span]

// Bind indexes data for Hits and LastHit until unbind runs.
// The bytes must stay unchanged until then.
func Bind(data []byte) (unbind func()) {
	if len(data) < 2 {
		return func() {}
	}
	s := &span{&data[0], len(data), buildWordIndex(data)}
	if !bound.CompareAndSwap(nil, s) {
		return func() {}
	}
	return func() { bound.CompareAndSwap(s, nil) }
}

func indexFor(data []byte) *wordIndex {
	s := bound.Load()
	if s == nil || len(data) < 2 || s.n != len(data) || &data[0] != s.p {
		return nil
	}
	return s.ix
}

func buildWordIndex(data []byte) *wordIndex {
	n := len(data) - 1
	count := make([]int, 65536)
	for i := 0; i < n; i++ {
		count[int(data[i])|int(data[i+1])<<8]++
	}
	ix := &wordIndex{start: make([]int, 65537), pos: make([]int32, n)}
	for w := 0; w < 65536; w++ {
		ix.start[w+1] = ix.start[w] + count[w]
	}
	cursor := make([]int, 65536)
	copy(cursor, ix.start[:65536])
	for i := 0; i < n; i++ {
		w := int(data[i]) | int(data[i+1])<<8
		ix.pos[cursor[w]] = int32(i)
		cursor[w]++
	}
	return ix
}

// window is the fully specified byte pair with the fewest occurrences.
func (b *wordIndex) window(pat, mask []byte) (off int, posts []int32, ok bool) {
	best := int(^uint(0) >> 1)
	for i := 0; i+1 < len(pat); i++ {
		if mask[i] != 0xFF || mask[i+1] != 0xFF {
			continue
		}
		w := int(pat[i]) | int(pat[i+1])<<8
		n := b.start[w+1] - b.start[w]
		if n < best {
			best = n
			off = i
			posts = b.pos[b.start[w]:b.start[w+1]]
			ok = true
			if n == 0 {
				return off, posts, true
			}
		}
	}
	return off, posts, ok
}

func hitsIndexed(data, pat, mask []byte, posts []int32, off, lo, maxStart, limit int) []int {
	minP := lo + off
	maxP := maxStart + off
	k := sort.Search(len(posts), func(i int) bool { return int(posts[i]) >= minP })
	var hits []int
	for ; k < len(posts); k++ {
		p := int(posts[k])
		if p > maxP {
			break
		}
		start := p - off
		if start < lo || start%2 != 0 {
			continue
		}
		if !matchAt(data, pat, mask, start) {
			continue
		}
		hits = append(hits, start)
		if limit > 0 && len(hits) >= limit {
			return hits
		}
	}
	return hits
}

func lastIndexed(data, pat, mask []byte, posts []int32, off, lo, maxStart int) int {
	minP := lo + off
	maxP := maxStart + off
	k := sort.Search(len(posts), func(i int) bool { return int(posts[i]) > maxP }) - 1
	for ; k >= 0; k-- {
		p := int(posts[k])
		if p < minP {
			break
		}
		start := p - off
		if start < lo || start > maxStart || start%2 != 0 {
			continue
		}
		if matchAt(data, pat, mask, start) {
			return start
		}
	}
	return -1
}

// solidRun is the longest fully specified byte run, used as an index.
func solidRun(pat, mask []byte) (start int, needle []byte) {
	best, bestN := 0, 0
	for i := 0; i < len(pat); {
		if mask[i] != 0xFF {
			i++
			continue
		}
		j := i
		for j < len(pat) && mask[j] == 0xFF {
			j++
		}
		if j-i > bestN {
			best, bestN = i, j-i
		}
		i = j
	}
	if bestN == 0 {
		return 0, nil
	}
	return best, pat[best : best+bestN]
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
		if len(n.Pats) > 1 {
			return nil, fmt.Errorf("%s: %s: mask_hex applies to one needle_hex", file, needleName)
		}
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
	if len(n.Pats) > 0 {
		pats := make([][]byte, len(n.Pats))
		masks := make([][]byte, len(n.Masks))
		for i := range n.Pats {
			pats[i] = append([]byte(nil), n.Pats[i]...)
			masks[i] = append([]byte(nil), n.Masks[i]...)
		}
		n.Pats, n.Masks = pats, masks
	}
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
