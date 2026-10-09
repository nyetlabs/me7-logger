// Package config loads the files shipped in config/.
// A file in config/ beside the executable, symlinks resolved, overrides the
// copy embedded at build time.
// An explicit path that does not exist is an error.
// config/user is an optional overlay; an empty user directory skips it.
package config

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.nyet.org/xdfkit/model"
	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/mapfile"
	"go.nyet.org/me7-logger/needle"
)

//go:embed names.yaml needles.yaml measurements.yaml catalog aliases.yaml maps.yaml signatures.yaml categories.json
var embedded embed.FS

const (
	// NamesFile is the ME7 name list.
	NamesFile = "names.yaml"
	// NeedlesFile is the byte-pattern list.
	NeedlesFile = "needles.yaml"
	// MeasuresFile is the measurement list. A row may carry the needle that locates it.
	MeasuresFile = "measurements.yaml"
	// MapDir is the result-type catalog directory.
	MapDir = "catalog"
	// AliasFile is the alias list, aliases.yaml.
	AliasFile = "aliases.yaml"
	// MapsFile is the caller-slot list, maps.yaml.
	MapsFile = "maps.yaml"
	// SigFile is the signature list, signatures.yaml.
	SigFile = "signatures.yaml"
	// CategoriesFile is the corpus category table, copied by make corpus-bump.
	CategoriesFile = "categories.json"
)

// LoadCategories reads the category table: the tuner map names, each with its
// XDF category (xdfkit docs/corpus.md), checked by xdfkit.
func LoadCategories(path string) (*model.CategoryTable, error) {
	b, err := Read(path, CategoriesFile)
	if err != nil {
		return nil, err
	}
	t, err := model.ParseCategoryTable(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", CategoriesFile, err)
	}
	return t, nil
}

// Dir is config/ beside the executable, symlinks resolved when they can be.
// It is "config" when the executable path cannot be read.
var Dir = sync.OnceValue(func() string { return dirFor(os.Executable) })

func dirFor(exe func() (string, error)) string {
	p, err := exe()
	if err != nil {
		return "config"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Join(filepath.Dir(p), "config")
}

// Path is Dir()/<name>, or the environment variable when it is set.
func Path(envKey, name string) string {
	if p := os.Getenv(envKey); p != "" {
		return p
	}
	return filepath.Join(Dir(), name)
}

// Read returns file bytes. The default Dir() path and an empty path use the
// embedded file when nothing is on disk. Any other path must exist.
func Read(path, name string) ([]byte, error) {
	def := filepath.Join(Dir(), name)
	if path == "" || path == def {
		if path == "" {
			path = def
		}
		b, err := os.ReadFile(path)
		if err == nil {
			return b, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		b, err = embedded.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("config: %s: %w", name, err)
		}
		return b, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// LoadCatalog reads the result-type catalog. An empty path or Dir()/catalog
// uses the shipped directory: disk when present, otherwise the embedded copy.
// Files are read in name order. scales.yaml is the named scales, not a row
// list. A later file replaces an earlier row with the same result type and
// bitmask. A path that is one file is parsed alone, so anchors in that file
// still apply.
func LoadCatalog(path string, conv map[mapfile.ScaleKey]mapfile.Scale) (mapfile.Table, error) {
	if path == "" || path == filepath.Join(Dir(), MapDir) {
		return loadShippedCatalog(conv)
	}
	st, err := os.Stat(path)
	if err != nil {
		return mapfile.Table{}, err
	}
	if !st.IsDir() {
		b, err := os.ReadFile(path)
		if err != nil {
			return mapfile.Table{}, err
		}
		return mapfile.Parse(b, path, conv, nil)
	}
	return loadCatalogDir(path, conv)
}

func loadShippedCatalog(conv map[mapfile.ScaleKey]mapfile.Scale) (mapfile.Table, error) {
	dir := filepath.Join(Dir(), MapDir)
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return loadCatalogDir(dir, conv)
	}
	ents, err := fs.ReadDir(embedded, MapDir)
	if err != nil {
		return mapfile.Table{}, fmt.Errorf("config: %s: %w", MapDir, err)
	}
	var files []catalogFile
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := embedded.ReadFile(path.Join(MapDir, e.Name()))
		if err != nil {
			return mapfile.Table{}, err
		}
		files = append(files, catalogFile{name: e.Name(), body: b})
	}
	return assembleCatalog(files, conv)
}

func loadCatalogDir(dir string, conv map[mapfile.ScaleKey]mapfile.Scale) (mapfile.Table, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return mapfile.Table{}, err
	}
	var files []catalogFile
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return mapfile.Table{}, err
		}
		files = append(files, catalogFile{name: e.Name(), body: b})
	}
	return assembleCatalog(files, conv)
}

type catalogFile struct {
	name string
	body []byte
}

func assembleCatalog(files []catalogFile, conv map[mapfile.ScaleKey]mapfile.Scale) (mapfile.Table, error) {
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	var scaleName string
	var scaleBody []byte
	var rows []catalogFile
	for _, f := range files {
		base := filepath.Base(f.name)
		if base == "scales.yaml" {
			scaleName = f.name
			scaleBody = f.body
			continue
		}
		if strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml") {
			rows = append(rows, f)
		}
	}
	if scaleBody == nil {
		return mapfile.Table{}, fmt.Errorf("catalog: missing scales.yaml")
	}
	named, err := mapfile.ParseScales(scaleBody, scaleName)
	if err != nil {
		return mapfile.Table{}, err
	}
	tab := mapfile.Table{ByRT: map[int]map[uint16]mapfile.Var{}, ByName: map[string]mapfile.Var{}}
	for _, f := range rows {
		part, err := mapfile.Parse(f.body, filepath.Base(f.name), conv, named)
		if err != nil {
			return mapfile.Table{}, err
		}
		for rt, m := range part.ByRT {
			if tab.ByRT[rt] == nil {
				tab.ByRT[rt] = map[uint16]mapfile.Var{}
			}
			for mask, v := range m {
				tab.ByRT[rt][mask] = v
			}
		}
		for name, v := range part.ByName {
			if _, ok := tab.ByName[name]; !ok {
				tab.ByName[name] = v
			}
		}
	}
	if len(tab.ByRT) == 0 {
		return tab, fmt.Errorf("catalog: no map rows")
	}
	return tab, nil
}

// Names is the editable ME7 name list.
type Names struct {
	MapName     string
	Connect     Connect
	Clock       Clock
	Scale       Scale
	Selector    [][]byte
	SelectorEnd int
}

// Connect names the 5-baud needles and the key bytes that mean KWP2000.
type Connect struct {
	SlowNeedle string
	FastNeedle string
	Key1       byte
	Key2       byte
	Prefer     byte
	Fallback   byte
	Fast       []byte
}

// Clock is the injection-time name list and the milliseconds-per-tick factors.
type Clock struct {
	DefaultMHz int
	KRKTE      string
	Injection  map[string]bool
	Factors    map[int]float64
}

// Scale is the 5120 mbar detector.
type Scale struct {
	Units   []string
	Ambient map[string]bool
	Low     float64
	High    float64
}

// LoadNames reads the name list.
func LoadNames(path string) (Names, error) {
	b, err := Read(path, NamesFile)
	if err != nil {
		return Names{}, err
	}
	var raw struct {
		MapName string `yaml:"map_name"`
		Connect struct {
			Slow     string   `yaml:"slow_init_needle"`
			Fast     string   `yaml:"fast_init_needle"`
			Key1     string   `yaml:"kwp_key1"`
			Key2     string   `yaml:"kwp_key2"`
			Prefer   string   `yaml:"prefer_address"`
			Fallback string   `yaml:"fallback_address"`
			Targets  []string `yaml:"fast_targets"`
		} `yaml:"connect"`
		Clock struct {
			Default   int       `yaml:"default_mhz"`
			KRKTE     string    `yaml:"krkte"`
			Injection []string  `yaml:"injection"`
			Factors   yaml.Node `yaml:"factors"`
		} `yaml:"clock"`
		Scale struct {
			Units   []string `yaml:"units"`
			Ambient []string `yaml:"ambient"`
			Low     float64  `yaml:"low_mbar"`
			High    float64  `yaml:"high_mbar"`
		} `yaml:"scale_5120"`
		Prefixes []string `yaml:"selector_prefixes"`
		End      string   `yaml:"selector_end"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return Names{}, err
	}
	n := Names{MapName: raw.MapName}
	n.Connect.SlowNeedle = raw.Connect.Slow
	n.Connect.FastNeedle = raw.Connect.Fast
	if n.Connect.Key1, err = parseByte(raw.Connect.Key1); err != nil {
		return Names{}, fmt.Errorf("kwp_key1: %w", err)
	}
	if n.Connect.Key2, err = parseByte(raw.Connect.Key2); err != nil {
		return Names{}, fmt.Errorf("kwp_key2: %w", err)
	}
	if n.Connect.Prefer, err = parseByte(raw.Connect.Prefer); err != nil {
		return Names{}, fmt.Errorf("prefer_address: %w", err)
	}
	if n.Connect.Fallback, err = parseByte(raw.Connect.Fallback); err != nil {
		return Names{}, fmt.Errorf("fallback_address: %w", err)
	}
	for _, t := range raw.Connect.Targets {
		b, err := parseByte(t)
		if err != nil {
			return Names{}, fmt.Errorf("fast_targets: %w", err)
		}
		n.Connect.Fast = append(n.Connect.Fast, b)
	}
	n.Clock.DefaultMHz = raw.Clock.Default
	n.Clock.KRKTE = raw.Clock.KRKTE
	n.Clock.Injection = map[string]bool{}
	for _, name := range raw.Clock.Injection {
		n.Clock.Injection[name] = true
	}
	n.Clock.Factors = map[int]float64{}
	if raw.Clock.Factors.Kind != 0 {
		fields, err := needle.MappingFields(&raw.Clock.Factors)
		if err != nil {
			return Names{}, fmt.Errorf("clock factors: %w", err)
		}
		for k, node := range fields {
			mhz, err := strconv.Atoi(k)
			if err != nil {
				return Names{}, fmt.Errorf("clock factor %q: %w", k, err)
			}
			v, err := scalarFloat(node)
			if err != nil {
				return Names{}, fmt.Errorf("clock factor %s: %w", k, err)
			}
			n.Clock.Factors[mhz] = v
		}
	}
	n.Scale.Units = raw.Scale.Units
	n.Scale.Low = raw.Scale.Low
	n.Scale.High = raw.Scale.High
	n.Scale.Ambient = map[string]bool{}
	for _, name := range raw.Scale.Ambient {
		n.Scale.Ambient[name] = true
	}
	for _, p := range raw.Prefixes {
		b, err := needle.ParseHex(p)
		if err != nil {
			return Names{}, fmt.Errorf("selector_prefixes: %w", err)
		}
		n.Selector = append(n.Selector, b)
	}
	if raw.End == "" {
		return Names{}, fmt.Errorf("%s: selector_end is required", NamesFile)
	}
	end, err := mapfile.ParseUint(raw.End)
	if err != nil {
		return Names{}, fmt.Errorf("selector_end: %w", err)
	}
	n.SelectorEnd = int(end)
	if n.Connect.SlowNeedle == "" || n.Clock.KRKTE == "" || len(n.Selector) == 0 {
		return Names{}, fmt.Errorf("%s: connect needle, krkte, and selector_prefixes are required", NamesFile)
	}
	return n, nil
}

// DefaultSize is the measurement width used when a row omits size.
// Two bytes is the most common width in config/measurements.yaml.
const DefaultSize = 2

// DefaultMeasureBackUp is the label offset when a measurement omits back_up.
// The mem operand is the word four bytes after the hit.
const DefaultMeasureBackUp = -4

// Measure is one named RAM row. A stub has no needle and is not written.
// Needle locates the mem operand. An address in this file is rejected.
// Omitted size, bitmask, unit, signed, inverse, factor, and offset are
// DefaultSize, 0, "", false, false, 1, and 0. A size+unit preset replaces
// omitted signed, inverse, factor, and offset. A single-bit bitmask does not
// use that preset.
type Measure struct {
	Name    string
	Alias   string
	Note    string
	Size    int
	Bitmask uint16
	Unit    string
	Signed  bool
	Inverse bool
	A, B    float64
	Comment string
	Stub    bool
	// Bit means the needle label is an 8A or 9A. The word is 0xFD00 plus
	// twice the next byte, the same address a selector case reads.
	Bit    bool
	Needle *needle.Needle
}

func parseByte(s string) (byte, error) {
	v, err := mapfile.ParseUint(s)
	if err != nil {
		return 0, err
	}
	if v > 0xff {
		return 0, fmt.Errorf("%s does not fit in a byte", s)
	}
	return byte(v), nil
}
