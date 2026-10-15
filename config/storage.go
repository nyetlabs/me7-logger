package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

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
		return Cell{Bits: hit.Bits, Signed: hit.Signed}, true
	}
	return Cell{hit.Bits, hit.Signed, max(hit.Rows, 1), max(hit.Cols, 1), hit.Ordinal}, true
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
	var rows []storeRow
	if yaml.Unmarshal(b, &rows) != nil {
		return
	}
	for _, r := range rows {
		storage[r.Name] = append(storage[r.Name], r)
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
	Parts   []string `yaml:"parts"`
}

const storeHeader = `# Cell width, sign, and shape by name, from the DAMOS and A2L defs. A row
# with parts is the value those part numbers use instead. rows and cols
# are left out for a scalar. unsized is a width whose shape the defs
# disagree on. ordinal axes are an index, not an image table. A name that
# is mostly an 8-bit unsigned scalar has no base row.
`

// BuildStorage reads the damos and a2l defs in dir and returns storage.yaml.
// dir is an ecu-corpus checkout.
func BuildStorage(dir string) ([]byte, error) {
	ents, err := os.ReadDir(filepath.Join(dir, "defs"))
	if err != nil {
		return nil, err
	}
	by := map[string]map[string]Cell{}
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
					Source string `json:"source"`
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
		if o := pack.Provenance.Origin; o != "damos" && o != "a2l" {
			continue
		}
		part := strings.TrimSuffix(e.Name(), ".json")
		if i := strings.LastIndexByte(part, '-'); i > 0 {
			part = part[:i]
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
				by[name][part] = Cell{o.Data.Bits, o.Data.Signed, max(o.Rows, 1), max(o.Cols, 1), o.X != nil && o.X.Source == "ordinal"}
			}
		}
	}
	names := make([]string, 0, len(by))
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	var out strings.Builder
	out.WriteString(storeHeader)
	for _, n := range names {
		cells := by[n]
		st, okS := majority(cells, func(c Cell) Cell { return Cell{Bits: c.Bits, Signed: c.Signed} })
		base, okD := majority(cells, func(c Cell) Cell { return Cell{Rows: c.Rows, Cols: c.Cols, Ordinal: c.Ordinal} })
		if !okD {
			base = Cell{}
		}
		base.Bits, base.Signed = st.Bits, st.Signed
		over := map[Cell][]string{}
		for p, c := range cells {
			if !okS || c != base {
				over[c] = append(over[c], p)
			}
		}
		var rows []storeRow
		if okS && base != (Cell{8, false, 1, 1, false}) {
			rows = append(rows, storeRow{Name: n, Bits: base.Bits, Signed: base.Signed, Rows: base.Rows, Cols: base.Cols, Ordinal: base.Ordinal})
		}
		keys := slices.Collect(maps.Keys(over))
		slices.SortFunc(keys, func(a, b Cell) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
		for _, c := range keys {
			sort.Strings(over[c])
			rows = append(rows, storeRow{Name: n, Bits: c.Bits, Signed: c.Signed, Rows: c.Rows, Cols: c.Cols, Ordinal: c.Ordinal, Parts: over[c]})
		}
		for _, r := range rows {
			row, err := storeLine(r)
			if err != nil {
				return nil, err
			}
			out.WriteString(row)
		}
	}
	return []byte(out.String()), nil
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

func storeLine(r storeRow) (string, error) {
	q, err := yaml.Marshal(r.Name)
	if err != nil {
		return "", err
	}
	s := fmt.Sprintf("- {name: %s, bits: %d", strings.TrimSpace(string(q)), r.Bits)
	if r.Signed {
		s += ", signed: true"
	}
	switch {
	case r.Rows == 0:
		s += ", unsized: true"
	case r.Rows*r.Cols > 1:
		s += fmt.Sprintf(", rows: %d, cols: %d", r.Rows, r.Cols)
	}
	if r.Ordinal {
		s += ", ordinal: true"
	}
	if len(r.Parts) > 0 {
		s += ", parts: [" + strings.Join(r.Parts, ", ") + "]"
	}
	return s + "}\n", nil
}
