package config

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/ident"
)

const (
	// LayoutsFile is the SW-number / EPK to layout-block map.
	LayoutsFile = "layouts.yaml"
	// LayoutsPriorityFile is finder order for layout blocks, highest first.
	LayoutsPriorityFile = "layouts-priority.yaml"
	// MapSigShardsFile lists mapsig shards per layout block.
	MapSigShardsFile = "mapsig-shards.yaml"
)

var epkRE = regexp.MustCompile(`[0-9]+/[0-9]+/ME7[!-~]*`)

// EPK is the calibration string in the image, empty when it has none.
func EPK(img []byte) string {
	return string(epkRE.Find(img))
}

// LayoutID is the id layouts.yaml lists an image by: the Bosch software
// number, or the EPK string when the image carries none.
func LayoutID(img []byte) string {
	if sw := ident.Find(img).SWNumber; sw != "" {
		return sw
	}
	return EPK(img)
}

// LoadLayouts reads block -> layout ids from layouts.yaml.
func LoadLayouts() (map[string][]string, error) {
	b, err := Read("", LayoutsFile)
	if err != nil {
		return nil, err
	}
	var blocks map[string][]string
	if err := yaml.Unmarshal(b, &blocks); err != nil {
		return nil, fmt.Errorf("config: %s: %w", LayoutsFile, err)
	}
	return blocks, nil
}

// LayoutBlock returns the layout block for a LayoutID, or "" if unknown.
func LayoutBlock(layoutID string) (string, error) {
	blocks, err := LoadLayouts()
	if err != nil {
		return "", err
	}
	for block, ids := range blocks {
		for _, id := range ids {
			if id == layoutID {
				return block, nil
			}
		}
	}
	return "", nil
}

// LoadLayoutsPriority reads the block order from layouts-priority.yaml
// (highest first). Every key in blocks must appear exactly once.
func LoadLayoutsPriority(blocks map[string][]string) ([]string, error) {
	b, err := Read("", LayoutsPriorityFile)
	if err != nil {
		return nil, err
	}
	var order []string
	if err := yaml.Unmarshal(b, &order); err != nil {
		return nil, fmt.Errorf("config: %s: %w", LayoutsPriorityFile, err)
	}
	seen := map[string]bool{}
	for _, block := range order {
		if _, ok := blocks[block]; !ok {
			return nil, fmt.Errorf("config: %s: %s is not a layout block", LayoutsPriorityFile, block)
		}
		if seen[block] {
			return nil, fmt.Errorf("config: %s: %s repeated", LayoutsPriorityFile, block)
		}
		seen[block] = true
	}
	for block := range blocks {
		if !seen[block] {
			return nil, fmt.Errorf("config: %s: %s is not listed", LayoutsPriorityFile, block)
		}
	}
	return order, nil
}

// MapSigSelection is the mapsig shards resolved for a LayoutID.
type MapSigSelection struct {
	LayoutID string   // SW number or EPK; empty for ReadMapSigs("")
	Block    string   // layout block; empty when unknown / all-shards fallback
	Shards   []string // filenames, e.g. common.yaml, family.yaml, 23g.yaml
	Parts    [][]byte // same order as Shards
}

// SelectMapSigs resolves mapsig shards for a LayoutID: common.yaml plus the
// block's list from mapsig-shards.yaml. An unknown or empty id, or a block
// omitted from the manifest, loads every shard (common first) with Block "".
func SelectMapSigs(layoutID string) (MapSigSelection, error) {
	names, block, err := mapSigShardNames(layoutID)
	if err != nil {
		return MapSigSelection{}, err
	}
	parts, err := readMapSigFiles(names)
	if err != nil {
		return MapSigSelection{}, err
	}
	return MapSigSelection{
		LayoutID: layoutID,
		Block:    block,
		Shards:   names,
		Parts:    parts,
	}, nil
}

// mapSigShardNames returns mapsig filenames and the block used for the
// manifest list. Block is empty when falling back to every shard.
func mapSigShardNames(layoutID string) ([]string, string, error) {
	all, err := listMapSigNames()
	if err != nil {
		return nil, "", err
	}
	byName := map[string]bool{}
	for _, n := range all {
		byName[n] = true
	}
	if !byName["common.yaml"] {
		return nil, "", fmt.Errorf("config: %s: missing common.yaml", MapSigDir)
	}
	if layoutID == "" {
		return allMapSigOrder(all), "", nil
	}
	block, err := LayoutBlock(layoutID)
	if err != nil {
		return nil, "", err
	}
	shards, err := loadMapSigShards()
	if err != nil {
		return nil, "", err
	}
	list, ok := shards[block]
	if block == "" || !ok {
		return allMapSigOrder(all), "", nil
	}
	out := []string{"common.yaml"}
	seen := map[string]bool{"common.yaml": true}
	for _, stem := range list {
		name := stem + ".yaml"
		if !byName[name] {
			return nil, "", fmt.Errorf("config: %s: shard %q not in %s", MapSigShardsFile, name, MapSigDir)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, block, nil
}

func loadMapSigShards() (map[string][]string, error) {
	b, err := Read("", MapSigShardsFile)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Shards map[string][]string `yaml:"shards"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("config: %s: %w", MapSigShardsFile, err)
	}
	if doc.Shards == nil {
		return nil, fmt.Errorf("config: %s: missing shards", MapSigShardsFile)
	}
	return doc.Shards, nil
}

func listMapSigNames() ([]string, error) {
	def := filepath.Join(Dir(), MapSigDir)
	if st, err := os.Stat(def); err == nil && st.IsDir() {
		return listMapSigNamesFS(os.DirFS(def), ".")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return listMapSigNamesFS(embedded, MapSigDir)
}

func listMapSigNamesFS(fsys fs.FS, root string) ([]string, error) {
	ents, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", MapSigDir, err)
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("config: %s: no *.yaml files", MapSigDir)
	}
	return names, nil
}

// allMapSigOrder is common.yaml first, then the other shards in name order.
func allMapSigOrder(all []string) []string {
	var rest []string
	for _, n := range all {
		if n != "common.yaml" {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	return append([]string{"common.yaml"}, rest...)
}

// ReadMapSigsFor returns SelectMapSigs(layoutID).Parts.
func ReadMapSigsFor(layoutID string) ([][]byte, error) {
	sel, err := SelectMapSigs(layoutID)
	if err != nil {
		return nil, err
	}
	return sel.Parts, nil
}

func readMapSigFiles(names []string) ([][]byte, error) {
	def := filepath.Join(Dir(), MapSigDir)
	if st, err := os.Stat(def); err == nil && st.IsDir() {
		return readNamedMapSigs(os.DirFS(def), ".", names)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return readNamedMapSigs(embedded, MapSigDir, names)
}

func readNamedMapSigs(fsys fs.FS, root string, names []string) ([][]byte, error) {
	out := make([][]byte, 0, len(names))
	for _, name := range names {
		b, err := fs.ReadFile(fsys, path.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("config: %s/%s: %w", MapSigDir, name, err)
		}
		out = append(out, b)
	}
	return out, nil
}
