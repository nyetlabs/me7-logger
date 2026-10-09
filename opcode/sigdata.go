package opcode

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Sig is one signature row. The name and the pattern come from config YAML.
// Pattern is tried in order. single applies to each string, and the hit keeps
// that string's length.
type Sig struct {
	Name    string
	Size    int
	From    uint32
	Pattern []string
	Open    string
	First   bool
	After   string
	In      string
	Absent  string
	Single  bool
	At      int
	Also    []SigAt
	Steps   []SigStep
}

// SigStep is one search in a chain. Skip is how many bytes past the previous
// hit the next search starts. The first step starts at the beginning of the range.
type SigStep struct {
	Pattern string
	Skip    int
}

// CallWords is one bootrom slot: the default table, the 0x402 table, and the 0x602 table.
type CallWords struct {
	Default uint32
	V402    uint32
	V602    uint32
}

// MapSig locates one calibration map. Pattern is a unique window whose
// address word is wild. At is the byte distance from the hit to that word.
// Omitted, At is 2. Add is the byte distance from the decoded pointer to the map body.
// Anchor names a map already located; Add is then the distance from that map.
// Rows and Cols are the axis point counts. XBits and YBits are the breakpoint
// widths, 8 when that count is set and the width is omitted. When the axes
// are prepended, the distance from the counts to the body is the length of
// that axis data. XAt and YAt are byte distances from the pattern hit to an
// F2 operand: the RAM word whose setup stored that header. Nil means this
// row does not name that word. Cols 0 means this row does not give the dimensions.
// Table means the pattern is the breakpoint table: the address is the hit,
// and the row is not a map. XTable and YTable name such a row. The count
// there has to match Cols or Rows.
type MapSig struct {
	Name    string
	Pattern string
	At      int
	Add     int
	Anchor  string
	Rows    int
	Cols    int
	XBits   int
	YBits   int
	XAt     *int
	YAt     *int
	Table   bool
	XTable  string
	YTable  string
	// Plain keeps Rows and Cols when no axis is stored in front of the body.
	Plain bool
	// Open limits the search to the DB00 window around that pattern.
	// First keeps the first copy of Open when several exist.
	// From is the lowest address for that search. Zero starts an open
	// search at 0x820000 and any other search at the start of flash.
	Open   string
	First  bool
	From   uint32
	Single bool
	// Frame is the 6-byte framePtr at At. Far is the 4-byte readFar there.
	// Deref reads that many further words. DerefAt is added to the address
	// before the first of those reads. DerefFar makes that read a readFar.
	Frame    bool
	Far      bool
	Deref    int
	DerefAt  int
	DerefFar bool
}

// SigDoc is the signature list and the call-slot words it references.
type SigDoc struct {
	Rows  []Sig
	Maps  []MapSig
	Calls map[string]CallWords
}

// SigAt is another name stored from the same hit.
// Add, when set, is a byte offset from the row address instead of a new word.
type SigAt struct {
	Name string
	Size int
	At   int
	Add  *int
}

type sigFile struct {
	Sigs  []sigDraft                   `yaml:"signatures"`
	Maps  []mapSigDraft                `yaml:"mapsigs"`
	Calls map[string]map[string]string `yaml:"calls"`
}

// mapSigDraft is one mapsig row. A pattern list is that many consecutive
// rows that differ only in pattern.
type mapSigDraft struct {
	Name     string  `yaml:"name"`
	Patterns sigPats `yaml:"pattern"`
	Pattern  string  `yaml:"-"`
	At       *int    `yaml:"at"`
	Add      *int    `yaml:"add"`
	Anchor   string  `yaml:"anchor"`
	Rows     int     `yaml:"rows"`
	Cols     int     `yaml:"cols"`
	XBits    int     `yaml:"xbits"`
	YBits    int     `yaml:"ybits"`
	XAt      *int    `yaml:"xat"`
	YAt      *int    `yaml:"yat"`
	Table    bool    `yaml:"table"`
	XTable   string  `yaml:"xtable"`
	YTable   string  `yaml:"ytable"`
	Plain    bool    `yaml:"plain"`
	Open     string  `yaml:"open"`
	First    bool    `yaml:"first"`
	From     string  `yaml:"from"`
	Single   bool    `yaml:"single"`
	Frame    bool    `yaml:"frame"`
	Far      bool    `yaml:"far"`
	Deref    int     `yaml:"deref"`
	DerefAt  int     `yaml:"derefat"`
	DerefFar bool    `yaml:"dereffar"`
	Also     []struct {
		Name string `yaml:"name"`
		Add  int    `yaml:"add"`
	} `yaml:"also"`
}

type sigDraft struct {
	Name    string         `yaml:"name"`
	Size    int            `yaml:"size"`
	From    string         `yaml:"from"`
	Pattern sigPats        `yaml:"pattern"`
	Open    string         `yaml:"open"`
	First   bool           `yaml:"first"`
	After   string         `yaml:"after"`
	In      string         `yaml:"in"`
	Absent  string         `yaml:"absent"`
	Single  bool           `yaml:"single"`
	At      int            `yaml:"at"`
	Also    []sigAtDraft   `yaml:"also"`
	Steps   []sigStepDraft `yaml:"steps"`
}

// sigPats is one pattern string or a list of them.
type sigPats struct {
	items []string
}

func (p *sigPats) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if strings.TrimSpace(n.Value) == "" {
			return fmt.Errorf("pattern is empty")
		}
		p.items = []string{n.Value}
		return nil
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			return fmt.Errorf("pattern list is empty")
		}
		p.items = make([]string, len(n.Content))
		for i, c := range n.Content {
			if c.Kind != yaml.ScalarNode || strings.TrimSpace(c.Value) == "" {
				return fmt.Errorf("pattern list wants strings")
			}
			p.items[i] = c.Value
		}
		return nil
	default:
		return fmt.Errorf("pattern wants a string or a list")
	}
}

type sigStepDraft struct {
	Pattern string `yaml:"pattern"`
	Skip    int    `yaml:"skip"`
}

type sigAtDraft struct {
	Name string `yaml:"name"`
	Size int    `yaml:"size"`
	At   int    `yaml:"at"`
	Add  *int   `yaml:"add"`
}

func shapeOK(row MapSig) error {
	if row.Rows < 0 || row.Cols < 0 {
		return fmt.Errorf("mapsig %s: axis point counts are not negative", row.Name)
	}
	if row.Rows > 0 && row.Cols == 0 {
		return fmt.Errorf("mapsig %s: a row axis needs the column count", row.Name)
	}
	if row.Cols > 0 && row.XBits != 8 && row.XBits != 16 {
		return fmt.Errorf("mapsig %s: xbits is 8 or 16", row.Name)
	}
	if row.Rows > 0 && row.YBits != 8 && row.YBits != 16 {
		return fmt.Errorf("mapsig %s: ybits is 8 or 16", row.Name)
	}
	if row.Cols == 0 && (row.XBits != 0 || row.YBits != 0) {
		return fmt.Errorf("mapsig %s: axis width needs the point count", row.Name)
	}
	return nil
}

// ParseSigs reads the signature list. Rows run in order.
func ParseSigs(b []byte) (SigDoc, error) {
	var raw sigFile
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return SigDoc{}, err
	}
	doc := SigDoc{Calls: map[string]CallWords{}}
	for id, words := range raw.Calls {
		set, err := parseCall(id, words)
		if err != nil {
			return SigDoc{}, err
		}
		doc.Calls[id] = set
	}
	doc.Rows = make([]Sig, 0, len(raw.Sigs))
	for i, d := range raw.Sigs {
		if d.Name == "" || d.Size <= 0 || (len(d.Pattern.items) == 0 && len(d.Steps) == 0) {
			return SigDoc{}, fmt.Errorf("signature %d: name, size, and a pattern are required", i+1)
		}
		row := Sig{
			Name: d.Name, Size: d.Size, Pattern: d.Pattern.items,
			Open: d.Open, First: d.First, After: d.After, In: d.In, Absent: d.Absent, Single: d.Single, At: d.At,
		}
		if d.From != "" {
			v, err := parseHex(d.From)
			if err != nil {
				return SigDoc{}, fmt.Errorf("signature %s: from: %w", d.Name, err)
			}
			row.From = v
		}
		for _, a := range d.Also {
			if a.Name == "" || a.Size <= 0 {
				return SigDoc{}, fmt.Errorf("signature %s: also needs a name and a size", d.Name)
			}
			row.Also = append(row.Also, SigAt{Name: a.Name, Size: a.Size, At: a.At, Add: a.Add})
		}
		for _, s := range d.Steps {
			if s.Pattern == "" {
				return SigDoc{}, fmt.Errorf("signature %s: a step needs a pattern", d.Name)
			}
			row.Steps = append(row.Steps, SigStep{Pattern: s.Pattern, Skip: s.Skip})
		}
		doc.Rows = append(doc.Rows, row)
	}
	var maps []mapSigDraft
	for _, d := range raw.Maps {
		if len(d.Patterns.items) == 0 {
			maps = append(maps, d)
		}
		for _, pat := range d.Patterns.items {
			d.Pattern = pat
			maps = append(maps, d)
		}
	}
	seen := map[string]struct{}{}
	for i, d := range maps {
		if d.Name == "" || (d.Pattern == "" && d.Anchor == "") || (d.Pattern != "" && d.Anchor != "") {
			return SigDoc{}, fmt.Errorf("mapsig %d: name and either a pattern or an anchor are required", i+1)
		}
		seen[d.Name] = struct{}{}
		if d.Anchor != "" && d.Add == nil {
			return SigDoc{}, fmt.Errorf("mapsig %s: an anchor needs add", d.Name)
		}
		if d.Table && (d.Pattern == "" || d.Anchor != "" || d.XTable != "" || d.YTable != "") {
			return SigDoc{}, fmt.Errorf("mapsig %s: a breakpoint table is a pattern, not a map", d.Name)
		}
		at := 0
		if d.Pattern != "" && !d.Table {
			at = 2
		}
		if d.At != nil && !d.Table {
			at = *d.At
		}
		xbits, ybits := d.XBits, d.YBits
		if d.Cols > 0 && xbits == 0 {
			xbits = 8
		}
		if d.Rows > 0 && ybits == 0 {
			ybits = 8
		}
		if d.Table && patLen(d.Pattern) == 0 {
			return SigDoc{}, fmt.Errorf("mapsig %s: a breakpoint table pattern is a byte string", d.Name)
		}
		if d.Frame && d.Far {
			return SigDoc{}, fmt.Errorf("mapsig %s: frame and far are different pointers", d.Name)
		}
		if d.Deref < 0 || d.DerefAt < 0 {
			return SigDoc{}, fmt.Errorf("mapsig %s: deref is not negative", d.Name)
		}
		if !d.Table && d.Pattern != "" && !strings.Contains(d.Pattern, "{") {
			n := patLen(d.Pattern)
			if n == 0 {
				return SigDoc{}, fmt.Errorf("mapsig %s: pattern is a byte string", d.Name)
			}
			// A negative at reads the pointer in front of the hit.
			if at >= 0 && !d.Frame && !d.Far && at+2 > n {
				return SigDoc{}, fmt.Errorf("mapsig %s: at must sit on a word inside the pattern", d.Name)
			}
			if at >= 0 && (d.Frame || d.Far) && at >= n {
				return SigDoc{}, fmt.Errorf("mapsig %s: at must sit inside the pattern", d.Name)
			}
		}
		if d.XTable != "" && d.Cols == 0 {
			return SigDoc{}, fmt.Errorf("mapsig %s: xtable needs the column count", d.Name)
		}
		if d.YTable != "" && d.Rows == 0 {
			return SigDoc{}, fmt.Errorf("mapsig %s: ytable needs the row count", d.Name)
		}
		row := MapSig{
			Name: d.Name, Pattern: d.Pattern, At: at, Anchor: d.Anchor,
			Rows: d.Rows, Cols: d.Cols, XBits: xbits, YBits: ybits,
			XAt: d.XAt, YAt: d.YAt, Table: d.Table, XTable: d.XTable, YTable: d.YTable,
			Open: d.Open, First: d.First, Single: d.Single, Plain: d.Plain,
			Frame: d.Frame, Far: d.Far, Deref: d.Deref, DerefAt: d.DerefAt, DerefFar: d.DerefFar,
		}
		if d.From != "" {
			v, err := parseHex(d.From)
			if err != nil {
				return SigDoc{}, fmt.Errorf("mapsig %s: from: %w", d.Name, err)
			}
			row.From = v
		}
		if d.Add != nil {
			row.Add = *d.Add
		}
		if err := shapeOK(row); err != nil {
			return SigDoc{}, err
		}
		doc.Maps = append(doc.Maps, row)
		for _, a := range d.Also {
			if a.Name == "" {
				return SigDoc{}, fmt.Errorf("mapsig %s: also needs a name", d.Name)
			}
			seen[a.Name] = struct{}{}
			more := row
			more.Name, more.Add = a.Name, a.Add
			doc.Maps = append(doc.Maps, more)
		}
	}
	tables := map[string]bool{}
	for _, row := range doc.Maps {
		if row.Table {
			tables[row.Name] = true
		}
	}
	for _, row := range doc.Maps {
		if row.Anchor != "" {
			if _, ok := seen[row.Anchor]; !ok {
				return SigDoc{}, fmt.Errorf("mapsig %s: anchor %s is not a mapsig", row.Name, row.Anchor)
			}
		}
		if row.XTable != "" && !tables[row.XTable] {
			return SigDoc{}, fmt.Errorf("mapsig %s: xtable %s is not a breakpoint table", row.Name, row.XTable)
		}
		if row.YTable != "" && !tables[row.YTable] {
			return SigDoc{}, fmt.Errorf("mapsig %s: ytable %s is not a breakpoint table", row.Name, row.YTable)
		}
	}
	return doc, nil
}

func parseCall(id string, words map[string]string) (CallWords, error) {
	var set CallWords
	var err error
	if set.Default, err = parseHex(words["0"]); err != nil {
		return set, fmt.Errorf("call %s: %w", id, err)
	}
	if set.V402, err = parseHex(words["402"]); err != nil {
		return set, fmt.Errorf("call %s: %w", id, err)
	}
	if set.V602, err = parseHex(words["602"]); err != nil {
		return set, fmt.Errorf("call %s: %w", id, err)
	}
	return set, nil
}

func parseHex(s string) (uint32, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), 16, 32)
	return uint32(v), err
}

func (c CallWords) word(ver uint32) uint32 {
	switch ver {
	case 0x402:
		return c.V402
	case 0x602:
		return c.V602
	default:
		return c.Default
	}
}

type sigHit struct {
	at uint32
	n  int
}

// ApplySigs runs the signature rows. known supplies addresses already stored.
// A name already present is left as it is. A row whose embedded name is
// missing is skipped.
func ApplySigs(img []byte, dpp [4]uint16, known map[string]uint32, doc SigDoc) []Named {
	rows := doc.Rows
	if len(img) < 0x4020 || len(rows) == 0 {
		return nil
	}
	end := FlashBase + uint32(len(img)) - 0x20
	have := map[string]uint32{}
	for name, addr := range known {
		have[name] = addr
	}
	slot := func(id string) (string, bool) {
		set, ok := doc.Calls[id]
		if !ok {
			return "", false
		}
		word := set.word(bootVer(img, FlashBase+uint32(len(img))))
		if word == 0 {
			return "", false
		}
		return callPat(word), true
	}
	hits := map[string]sigHit{}
	wins := map[string][2]uint32{}
	var out []Named
	add := func(name string, addr uint32, size int) {
		if _, ok := have[name]; ok || !signAddr(addr) {
			return
		}
		have[name] = addr
		out = append(out, Named{Name: name, Addr: addr, Size: size})
	}
	for _, row := range rows {
		from := row.From
		if from == 0 {
			from = 0x804000
		}
		lo, hi := from, end
		switch {
		case row.After != "":
			prev, ok := hits[row.After]
			if !ok {
				continue
			}
			lo = prev.at + uint32(prev.n)
			hi = findPat(img, lo, end, "DB00", false)
			if hi == 0 {
				continue
			}
		case row.In != "":
			win, ok := wins[row.In]
			if !ok {
				continue
			}
			lo, hi = win[0], win[1]
		case row.Open != "":
			open, ok := expandSig(row.Open, have, slot)
			if !ok {
				continue
			}
			h := findPat(img, from, end, open, !row.First)
			if h == 0 {
				continue
			}
			db := findLast(img, from, h, "DB00")
			if db == 0 {
				continue
			}
			lo = db + 2
			hi = findPat(img, lo, end, "DB00", false)
			if hi == 0 {
				continue
			}
			wins[row.Name] = [2]uint32{lo, hi}
		}
		if row.Absent != "" {
			absent, ok := expandSig(row.Absent, have, slot)
			if !ok || findPat(img, lo, hi, absent, false) != 0 {
				continue
			}
		}
		if len(row.Steps) == 0 {
			applyPats(img, dpp, lo, hi, row, have, slot, add, hits)
			continue
		}
		if h, n := walkSteps(img, lo, hi, row.Steps, have, slot); h != 0 {
			storeSig(img, dpp, h, n, row, add, hits)
		}
	}
	return out
}

func storeSig(img []byte, dpp [4]uint16, h uint32, n int, row Sig, add func(string, uint32, int), hits map[string]sigHit) {
	hits[row.Name] = sigHit{at: h, n: n}
	base := PtrAt(img, ptrOff(h, row.At), dpp)
	add(row.Name, base, row.Size)
	for _, a := range row.Also {
		addr := base
		if a.Add != nil {
			addr += uint32(*a.Add)
		} else {
			addr = PtrAt(img, ptrOff(h, a.At), dpp)
		}
		add(a.Name, addr, a.Size)
	}
}

// applyPats stores the first string that hits. single applies to that string.
// A hit that does not store the row name tries the next string. Names added
// beside it stay.
func applyPats(img []byte, dpp [4]uint16, lo, hi uint32, row Sig, have map[string]uint32, slot func(string) (string, bool), add func(string, uint32, int), hits map[string]sigHit) {
	for _, raw := range row.Pattern {
		pat, ok := expandSig(raw, have, slot)
		if !ok {
			continue
		}
		h := findPat(img, lo, hi, pat, row.Single)
		if h == 0 {
			continue
		}
		storeSig(img, dpp, h, patLen(pat), row, add, hits)
		if _, ok := have[row.Name]; ok {
			return
		}
	}
}

func ptrOff(h uint32, at int) uint32 {
	return uint32(int64(h) + int64(at))
}

func walkSteps(img []byte, lo, hi uint32, steps []SigStep, have map[string]uint32, slot func(string) (string, bool)) (uint32, int) {
	var h uint32
	var n int
	start := lo
	for i, step := range steps {
		pat, ok := expandSig(step.Pattern, have, slot)
		if !ok {
			return 0, 0
		}
		if i > 0 {
			start = h + uint32(step.Skip)
		}
		h = findPat(img, start, hi, pat, false)
		if h == 0 {
			return 0, 0
		}
		n = patLen(pat)
	}
	return h, n
}

// expandSig replaces {name:MID} with a stored address and {slot:N} with the
// call word for this image. A missing name or slot reports false.
func expandSig(pat string, have map[string]uint32, slot func(string) (string, bool)) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(pat); {
		if pat[i] != '{' {
			b.WriteByte(pat[i])
			i++
			continue
		}
		j := strings.IndexByte(pat[i:], '}')
		if j < 0 {
			return "", false
		}
		name, mid, ok := strings.Cut(pat[i+1:i+j], ":")
		if !ok || name == "" || mid == "" {
			return "", false
		}
		if name == "slot" {
			got, ok := slot(mid)
			if !ok {
				return "", false
			}
			b.WriteString(got)
		} else {
			addr := have[name]
			if addr == 0 {
				return "", false
			}
			b.WriteString(embedAddr(addr, mid))
		}
		i += j + 1
	}
	return b.String(), true
}
