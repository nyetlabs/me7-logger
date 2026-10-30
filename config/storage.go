package config

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"gopkg.in/yaml.v3"
)

// Cell is the storage and shape of a Bosch name. Rows and Cols are 1 for
// a scalar and 0 when the defs disagree. Ordinal axes are an index, not a
// table in the image.
type Cell struct {
	Bits    int
	Signed  bool
	Rows    int
	Cols    int
	Ordinal bool
	XSigned bool
}

// Storage is the cell of name. A row listing part wins for that part
// number. A name with no row is unknown.
func Storage(name, part string) (Cell, bool) {
	storeOnce.Do(loadStorage)
	var hit *storeRow
	for i, r := range storage[name] {
		if slices.Contains(r.Parts, part) {
			hit = &storage[name][i]
			break
		}
		if len(r.Parts) == 0 && hit == nil {
			hit = &storage[name][i]
		}
	}
	if hit == nil {
		return Cell{}, false
	}
	if hit.Unsized {
		return Cell{Bits: hit.Bits, Signed: hit.Signed, XSigned: hit.XSigned}, true
	}
	return Cell{hit.Bits, hit.Signed, max(hit.Rows, 1), max(hit.Cols, 1), hit.Ordinal, hit.XSigned}, true
}

var (
	storeOnce sync.Once
	storage   map[string][]storeRow
)

func loadStorage() {
	storage = map[string][]storeRow{}
	b, err := embedded.ReadFile("storage.yaml")
	if err != nil {
		return
	}
	var groups []storeGroup
	if yaml.Unmarshal(b, &groups) != nil {
		return
	}
	for _, g := range groups {
		row := storeRow{Bits: g.Bits, Signed: g.Signed, Rows: g.Rows, Cols: g.Cols, Ordinal: g.Ordinal, Unsized: g.Unsized, XSigned: g.XSigned, Parts: g.Parts}
		for _, name := range g.Names {
			storage[name] = append(storage[name], row)
		}
	}
}

type storeRow struct {
	Name    string   `yaml:"name"`
	Bits    int      `yaml:"bits"`
	Signed  bool     `yaml:"signed"`
	Rows    int      `yaml:"rows"`
	Cols    int      `yaml:"cols"`
	Ordinal bool     `yaml:"ordinal"`
	Unsized bool     `yaml:"unsized"`
	XSigned bool     `yaml:"xsigned"`
	Parts   []string `yaml:"parts"`
}

// storeGroup is one cell and every name that uses it. A parts list is the
// cell those part numbers use instead of the base row.
type storeGroup struct {
	Bits    int      `yaml:"bits"`
	Signed  bool     `yaml:"signed,omitempty"`
	Rows    int      `yaml:"rows,omitempty"`
	Cols    int      `yaml:"cols,omitempty"`
	Ordinal bool     `yaml:"ordinal,omitempty"`
	Unsized bool     `yaml:"unsized,omitempty"`
	XSigned bool     `yaml:"xsigned,omitempty"`
	Parts   []string `yaml:"parts,omitempty"`
	Names   []string `yaml:"names"`
}

const storeHeader = `# Cell width, sign, and shape, from the DAMOS and A2L defs. Names that
# share a cell are one row. A row with parts is the cell those part numbers
# use instead. rows and cols are left out for a scalar. unsized is a width
# whose shape the defs disagree on. ordinal axes are an index, not an image
# table. xsigned is an X axis that is monotonic only when read signed. A name
# that is mostly an 8-bit unsigned scalar has no base row.
`

// BuildStorage reads the damos and a2l defs in dir and returns storage.yaml.
// dir is an ecu-corpus checkout.
func BuildStorage(dir string) ([]byte, error) {
	ents, err := os.ReadDir(filepath.Join(dir, "defs"))
	if err != nil {
		return nil, err
	}
	by := map[string]map[string]Cell{}
	sign := map[string]map[string]bool{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "defs", e.Name()))
		if err != nil {
			return nil, err
		}
		var pack struct {
			Provenance struct {
				Origin string `json:"origin"`
			} `json:"provenance"`
			Objects []struct {
				Key  string `json:"key"`
				ID   string `json:"id"`
				Rows int    `json:"rows"`
				Cols int    `json:"cols"`
				X    *struct {
					Source  string `json:"source"`
					Address string `json:"address"`
					Data    struct {
						Bits   int    `json:"bits"`
						Endian string `json:"endian"`
						Signed bool   `json:"signed"`
					} `json:"data"`
				} `json:"x"`
				Data struct {
					Bits   int  `json:"bits"`
					Signed bool `json:"signed"`
				} `json:"data"`
			} `json:"objects"`
		}
		if err := json.Unmarshal(b, &pack); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		origin := pack.Provenance.Origin
		if origin != "damos" && origin != "a2l" && origin != "auto" {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".json")
		part := stem
		if i := strings.LastIndexByte(part, '-'); i > 0 {
			part = part[:i]
		}
		if origin == "auto" {
			img, err := os.ReadFile(filepath.Join(dir, "images", ecucorpus.Group(stem), stem+".bin"))
			if err != nil {
				continue
			}
			for _, o := range pack.Objects {
				name := o.Key
				if name == "" {
					name = strings.TrimSpace(o.ID)
				}
				if name == "" || o.X == nil || !xSignedOnly(img, o.Cols, o.X.Address, o.X.Data.Bits, o.X.Data.Endian, o.X.Source) {
					continue
				}
				if sign[name] == nil {
					sign[name] = map[string]bool{}
				}
				sign[name][part] = true
			}
			continue
		}
		seen := map[string]bool{}
		for _, o := range pack.Objects {
			name := o.Key
			if name == "" {
				name = strings.TrimSpace(o.ID)
			}
			if name == "" || seen[name] || o.Data.Bits == 0 {
				continue
			}
			seen[name] = true
			if by[name] == nil {
				by[name] = map[string]Cell{}
			}
			if _, ok := by[name][part]; !ok {
				by[name][part] = Cell{o.Data.Bits, o.Data.Signed, max(o.Rows, 1), max(o.Cols, 1), o.X != nil && o.X.Source == "ordinal", false}
			}
		}
	}
	names := make([]string, 0, len(by))
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	var all []storeRow
	for _, n := range names {
		cells := by[n]
		st, okS := majority(cells, func(c Cell) Cell { return Cell{Bits: c.Bits, Signed: c.Signed} })
		base, okD := majority(cells, func(c Cell) Cell { return Cell{Rows: c.Rows, Cols: c.Cols, Ordinal: c.Ordinal} })
		if !okD {
			base = Cell{}
		}
		base.Bits, base.Signed = st.Bits, st.Signed
		for p := range sign[n] {
			if c, ok := cells[p]; ok {
				c.XSigned = true
				cells[p] = c
			}
		}
		over := map[Cell][]string{}
		for p, c := range cells {
			if !okS || c != base {
				over[c] = append(over[c], p)
			}
		}
		var extra []string
		for p := range sign[n] {
			if _, ok := cells[p]; !ok {
				extra = append(extra, p)
			}
		}
		if len(extra) > 0 {
			add := base
			add.XSigned = true
			over[add] = append(over[add], extra...)
		}
		var rows []storeRow
		if okS && base != (Cell{8, false, 1, 1, false, false}) {
			rows = append(rows, storeRow{Name: n, Bits: base.Bits, Signed: base.Signed, Rows: base.Rows, Cols: base.Cols, Ordinal: base.Ordinal, XSigned: base.XSigned})
		}
		keys := slices.Collect(maps.Keys(over))
		slices.SortFunc(keys, func(a, b Cell) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
		for _, c := range keys {
			sort.Strings(over[c])
			rows = append(rows, storeRow{Name: n, Bits: c.Bits, Signed: c.Signed, Rows: c.Rows, Cols: c.Cols, Ordinal: c.Ordinal, XSigned: c.XSigned, Parts: over[c]})
		}
		all = append(all, rows...)
	}
	body, err := storeText(all)
	if err != nil {
		return nil, err
	}
	return append([]byte(storeHeader), body...), nil
}

// majority is the most common key of the part cells, and false on a tie.
func majority(cells map[string]Cell, key func(Cell) Cell) (Cell, bool) {
	n := map[Cell]int{}
	for _, c := range cells {
		n[key(c)]++
	}
	var best Cell
	most, tie := 0, false
	for k, v := range n {
		if v > most {
			best, most, tie = k, v, false
		} else if v == most {
			tie = true
		}
	}
	return best, !tie
}

// xSignedOnly reports an image X axis whose points are monotonic only
// when read signed. A big-endian word is the import's byte order, not a sign.
func xSignedOnly(img []byte, n int, addr string, bits int, endian, source string) bool {
	if source != "image" {
		return false
	}
	a, ok := parseAddr(addr)
	if !ok {
		return false
	}
	return SignedOnly(img, a, n, bits, endian)
}

// SignedOnly reports an image axis whose points are monotonic only when
// read signed. A big-endian word is the import's byte order, not a sign.
// An axis whose unsigned points already rise is false.
func SignedOnly(img []byte, addr, n, bits int, endian string) bool {
	if endian == "big" || bits < 8 || n < 2 {
		return false
	}
	return monotonic(series(img, addr, n, bits, true)) && !monotonic(series(img, addr, n, bits, false))
}

func parseAddr(s string) (int, bool) {
	h, ok := strings.CutPrefix(strings.TrimSpace(s), "0x")
	if !ok {
		h, ok = strings.CutPrefix(strings.TrimSpace(s), "0X")
	}
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	return int(v), err == nil
}

func series(img []byte, addr, n, bits int, signed bool) []int {
	w := bits / 8
	if w < 1 || n < 1 || addr < 0 || addr+n*w > len(img) {
		return nil
	}
	out := make([]int, n)
	for i := range n {
		u := 0
		for j := range w {
			u |= int(img[addr+i*w+j]) << (8 * j)
		}
		if signed && u >= 1<<(bits-1) {
			u -= 1 << bits
		}
		out[i] = u
	}
	return out
}

func monotonic(v []int) bool {
	if len(v) < 2 {
		return false
	}
	inc, dec := true, true
	for i := 1; i < len(v); i++ {
		if v[i] <= v[i-1] {
			inc = false
		}
		if v[i] >= v[i-1] {
			dec = false
		}
	}
	return inc || dec
}

type cellKey struct {
	bits, rows, cols                  int
	signed, xsigned, ordinal, unsized bool
	parts                             string
}

func keyOf(r storeRow) cellKey {
	unsized := r.Rows == 0
	rows, cols := r.Rows, r.Cols
	if unsized || rows*cols <= 1 {
		rows, cols = 0, 0
	}
	return cellKey{r.Bits, rows, cols, r.Signed, r.XSigned, r.Ordinal, unsized, strings.Join(r.Parts, ",")}
}

func storeText(rows []storeRow) ([]byte, error) {
	grouped := map[cellKey][]string{}
	for _, r := range rows {
		k := keyOf(r)
		grouped[k] = append(grouped[k], r.Name)
	}
	keys := slices.Collect(maps.Keys(grouped))
	slices.SortFunc(keys, func(a, b cellKey) int {
		return cmp.Or(
			cmp.Compare(booln(a.parts != ""), booln(b.parts != "")),
			cmp.Compare(a.bits, b.bits),
			cmp.Compare(booln(a.signed), booln(b.signed)),
			cmp.Compare(booln(a.xsigned), booln(b.xsigned)),
			cmp.Compare(booln(a.unsized), booln(b.unsized)),
			cmp.Compare(a.rows, b.rows),
			cmp.Compare(a.cols, b.cols),
			cmp.Compare(booln(a.ordinal), booln(b.ordinal)),
			cmp.Compare(a.parts, b.parts),
		)
	})
	var out strings.Builder
	for _, k := range keys {
		names := grouped[k]
		sort.Strings(names)
		s, err := emitGroup(k, names)
		if err != nil {
			return nil, err
		}
		out.WriteString(s)
	}
	return []byte(out.String()), nil
}

func booln(v bool) int {
	if v {
		return 1
	}
	return 0
}

func yamlScalar(s string) (string, error) {
	b, err := yaml.Marshal(s)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func joinScalars(ss []string) (string, error) {
	q := make([]string, len(ss))
	for i, s := range ss {
		v, err := yamlScalar(s)
		if err != nil {
			return "", err
		}
		q[i] = v
	}
	return strings.Join(q, ", "), nil
}

// emitGroup writes one cell. A long name list wraps inside the flow sequence.
func emitGroup(k cellKey, names []string) (string, error) {
	head := fmt.Sprintf("- {bits: %d", k.bits)
	if k.signed {
		head += ", signed: true"
	}
	if k.xsigned {
		head += ", xsigned: true"
	}
	switch {
	case k.unsized:
		head += ", unsized: true"
	case k.rows*k.cols > 1:
		head += fmt.Sprintf(", rows: %d, cols: %d", k.rows, k.cols)
	}
	if k.ordinal {
		head += ", ordinal: true"
	}
	if k.parts != "" {
		parts, err := joinScalars(strings.Split(k.parts, ","))
		if err != nil {
			return "", err
		}
		head += ", parts: [" + parts + "]"
	}
	qs := make([]string, len(names))
	for i, n := range names {
		q, err := yamlScalar(n)
		if err != nil {
			return "", err
		}
		qs[i] = q
	}
	one := head + ", names: [" + strings.Join(qs, ", ") + "]}\n"
	if len(one) <= 100 {
		return one, nil
	}
	var b strings.Builder
	b.WriteString(head)
	b.WriteString(", names: [\n")
	col := 0
	for i, n := range qs {
		piece := n
		if i+1 < len(qs) {
			piece += ","
		}
		if col > 0 && col+1+len(piece) > 96 {
			b.WriteString("\n")
			col = 0
		}
		if col == 0 {
			b.WriteString("    ")
			col = 4
		} else {
			b.WriteString(" ")
			col++
		}
		b.WriteString(piece)
		col += len(piece)
	}
	b.WriteString("]}\n")
	return b.String(), nil
}
