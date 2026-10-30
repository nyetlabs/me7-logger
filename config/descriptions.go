package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Description is the canonical sentence for a Bosch name: the 551M wording
// when that pack has one, otherwise the DAMOS or A2L sentence. A name in
// neither source is empty.
func Description(name string) string {
	descOnce.Do(loadDescriptions)
	return descriptions[name]
}

// Codeword reports a hex bitmap. It has no conversion.
func Codeword(name string) bool {
	d := strings.ToLower(Description(name))
	return strings.Contains(d, "codeword") || strings.Contains(d, "codewort") || strings.Contains(d, "code word")
}

// Bitmap is a raw byte: a codeword, a configuration byte, or an error class.
// ESKONF is a byte string and is not one of these.
func Bitmap(name string) bool {
	if Codeword(name) {
		return true
	}
	d := strings.ToLower(Description(name))
	for _, s := range []string{
		"konfigurationsbyte", "configuration byte",
		"fehlerklasse", "error class", "failure code",
		"eurobit", "euro-byte", "euro byte",
	} {
		if strings.Contains(d, s) {
			return true
		}
	}
	return false
}

var (
	descOnce     sync.Once
	descriptions map[string]string
)

func loadDescriptions() {
	descriptions = map[string]string{}
	b, err := embedded.ReadFile("descriptions.yaml")
	if err != nil {
		return
	}
	var rows []descRow
	if yaml.Unmarshal(b, &rows) != nil {
		return
	}
	for _, r := range rows {
		descriptions[r.Name] = r.Description
	}
}

type descRow struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

const descHeader = `# Map descriptions. 8D0907551M wins when it has a sentence.
# DAMOS and A2L supply the rest. Located packs are not a source.
`

// mPack is the original name list. Other manual packs are not a source.
const mPack = "8D0907551M-0002.json"

// BuildDescriptions reads defs in dir and returns descriptions.yaml.
// Origin auto is skipped. dir is an ecu-corpus checkout.
func BuildDescriptions(dir string) ([]byte, error) {
	ents, err := os.ReadDir(filepath.Join(dir, "defs"))
	if err != nil {
		return nil, err
	}
	type bag struct {
		m     map[string]int
		damos map[string]int
	}
	by := map[string]*bag{}
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
				Key         string `json:"key"`
				ID          string `json:"id"`
				Description string `json:"description"`
			} `json:"objects"`
		}
		if err := json.Unmarshal(b, &pack); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if pack.Provenance.Origin == "auto" {
			continue
		}
		mFile := e.Name() == mPack
		damos := pack.Provenance.Origin == "damos" || pack.Provenance.Origin == "a2l"
		if !mFile && !damos {
			continue
		}
		for _, o := range pack.Objects {
			name := o.Key
			if name == "" {
				name = strings.TrimSpace(o.ID)
			}
			desc := strings.TrimSpace(strings.ReplaceAll(o.Description, "\t", " "))
			if name == "" || desc == "" {
				continue
			}
			g := by[name]
			if g == nil {
				g = &bag{m: map[string]int{}, damos: map[string]int{}}
				by[name] = g
			}
			if mFile {
				g.m[desc]++
			}
			if damos {
				g.damos[desc]++
			}
		}
	}
	names := make([]string, 0, len(by))
	for n, g := range by {
		if sentence(n, g.m, g.damos) != "" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	rows := make([]descRow, 0, len(names))
	for _, n := range names {
		g := by[n]
		rows = append(rows, descRow{Name: n, Description: sentence(n, g.m, g.damos)})
	}
	body, err := yaml.Marshal(rows)
	if err != nil {
		return nil, err
	}
	return append([]byte(descHeader), body...), nil
}

// sentence is M's wording, or the DAMOS sentence that appears most often,
// then the longest, then the lexicographically first. A sentence that is
// only the bare name loses to any other DAMOS sentence.
func sentence(name string, m, damos map[string]int) string {
	if s := fromM(m, damos); s != "" {
		return s
	}
	return ranked(name, damos)
}

func fromM(m, damos map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	if len(m) == 1 {
		for s := range m {
			return s
		}
	}
	own := map[string]int{}
	for s, n := range m {
		if damos[s] == 0 {
			own[s] = n
		}
	}
	if len(own) == 0 {
		return ""
	}
	return ranked("", own)
}

func ranked(name string, counts map[string]int) string {
	type row struct {
		s string
		n int
	}
	var rows []row
	for s, n := range counts {
		if s != "" {
			rows = append(rows, row{s, n})
		}
	}
	if name != "" && len(rows) > 1 {
		var real []row
		for _, r := range rows {
			if r.s != name {
				real = append(real, r)
			}
		}
		if len(real) > 0 {
			rows = real
		}
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		ni, nj := utf8.RuneCountInString(rows[i].s), utf8.RuneCountInString(rows[j].s)
		if ni != nj {
			return ni > nj
		}
		return rows[i].s < rows[j].s
	})
	return rows[0].s
}
