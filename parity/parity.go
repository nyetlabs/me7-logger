// Package parity scores generated rows against separate oracles.
// Matching each image's ME7Info file is the only hard mark.
// Catalog coverage, extras, the shared S4wiki list, a supplied XDF, and
// that file's axes are coverage. Outperform counts catalog names that file does not name.
package parity

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/ecu"
	"go.nyet.org/me7-logger/generate"
	"go.nyet.org/me7-logger/ident"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

// Report is one pass over a parity root.
// ME7Info is the only hard mark. Extras, S4Wiki, and XDF are coverage.
// An empty slice means that kind had no oracle.
// Corpus, on a ME7Info row, is catalog names located on that image over the
// full catalog. Extras is the measurement list on that image.
// Axis and Confidence are set on an S4Wiki row. Axis is the axes on the
// maps that scored, and a hit matches that image's XDF. Confidence is the
// body-byte result for the names that row scored.
// Axis on an XDF row is every axis in that file.
type Report struct {
	ME7Info []Image
	Extras  []Image
	S4Wiki  []Image
	XDF     []Image
}

// Image is one binary scored against one oracle.
// Beyond is the count of catalog names located on this image that its
// ME7Info file does not name. Corpus is that image against the full catalog.
// Both are set only on ME7Info rows. Tier, Axis, and Confidence are set on S4Wiki rows.
// Tier is the layout block tier of the image in layouts-priority.yaml.
type Image struct {
	Name string
	Fraction
	Beyond     int
	Corpus     Fraction
	Tier       string
	Axis       Fraction
	Confidence Fraction
}

// Fraction is hits over the oracle row count.
type Fraction struct {
	Hit, Total int
}

// String renders one decimal percent. An empty oracle is 0.0% (0/0).
func (f Fraction) String() string {
	if f.Total == 0 {
		return "0.0% (0/0)"
	}
	return fmt.Sprintf("%s (%d/%d)", f.percent(), f.Hit, f.Total)
}

func (f Fraction) percent() string {
	if f.Total == 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(f.Hit)/float64(f.Total))
}

// Text is the parity report. A kind with no rows is omitted.
// Names share a column. Each score is its fraction, then its percent.
// The image name is the stem, without .bin.
func (r *Report) Text() string {
	if r == nil {
		return ""
	}
	type line struct {
		label   string
		frac    Fraction
		corpus  Fraction
		tier    string
		axis    Fraction
		conf    Fraction
		extras  Fraction
		head    bool
		beyond  int
		me7info bool
		wiki    bool
		xdf     bool
	}
	var lines []line
	extraBy := map[string]Fraction{}
	for _, im := range r.Extras {
		extraBy[im.Name] = im.Fraction
	}
	if len(r.ME7Info) > 0 {
		lines = append(lines, line{label: "ecu me7info", head: true, me7info: true})
		for _, im := range r.ME7Info {
			lines = append(lines, line{
				label: "  " + stemName(im.Name), frac: im.Fraction, corpus: im.Corpus,
				beyond: im.Beyond, extras: extraBy[im.Name], me7info: true,
			})
		}
	}
	if len(r.S4Wiki) > 0 {
		if len(lines) > 0 {
			lines = append(lines, line{})
		}
		lines = append(lines, line{label: "xdf s4wiki", head: true, wiki: true})
		for _, im := range r.S4Wiki {
			lines = append(lines, line{
				label: "  " + stemName(im.Name), frac: im.Fraction, tier: im.Tier,
				axis: im.Axis, conf: im.Confidence, wiki: true,
			})
		}
	}
	if len(r.XDF) > 0 {
		if len(lines) > 0 {
			lines = append(lines, line{})
		}
		lines = append(lines, line{label: "xdf", head: true, xdf: true})
		for _, im := range r.XDF {
			lines = append(lines, line{label: "  " + stemName(im.Name), frac: im.Fraction, axis: im.Axis, xdf: true})
		}
	}

	nameW, countW, beyondW, corpusW := 0, 0, 0, 0
	axisW, confW, extrasW := 0, 0, 0
	counts := make([]string, len(lines))
	beyonds := make([]string, len(lines))
	corpus := make([]string, len(lines))
	axes := make([]string, len(lines))
	confs := make([]string, len(lines))
	extras := make([]string, len(lines))
	for i, ln := range lines {
		if ln.label == "" || ln.head {
			continue
		}
		if len(ln.label) > nameW {
			nameW = len(ln.label)
		}
		counts[i] = fmt.Sprintf("%d/%d", ln.frac.Hit, ln.frac.Total)
		if len(counts[i]) > countW {
			countW = len(counts[i])
		}
		if ln.me7info {
			beyonds[i] = fmt.Sprintf("+%d", ln.beyond)
			if len(beyonds[i]) > beyondW {
				beyondW = len(beyonds[i])
			}
			corpus[i] = fmt.Sprintf("%d/%d", ln.corpus.Hit, ln.corpus.Total)
			if len(corpus[i]) > corpusW {
				corpusW = len(corpus[i])
			}
			if ln.extras.Total > 0 {
				extras[i] = fmt.Sprintf("%d/%d", ln.extras.Hit, ln.extras.Total)
				if len(extras[i]) > extrasW {
					extrasW = len(extras[i])
				}
			}
		}
		if ln.wiki || ln.xdf {
			axes[i] = fmt.Sprintf("%d/%d", ln.axis.Hit, ln.axis.Total)
			if len(axes[i]) > axisW {
				axisW = len(axes[i])
			}
		}
		if ln.wiki && ln.conf.Total > 0 {
			confs[i] = fmt.Sprintf("%d/%d", ln.conf.Hit, ln.conf.Total)
			if len(confs[i]) > confW {
				confW = len(confs[i])
			}
		}
	}
	if axisW < 3 {
		axisW = 3
	}
	if confW < 3 {
		confW = 3
	}
	showExtras := extrasW > 0
	if extrasW < 3 {
		extrasW = 3
	}
	if len(r.ME7Info) > 0 && nameW < len("ecu me7info") {
		nameW = len("ecu me7info")
	}
	var b strings.Builder
	for i, ln := range lines {
		switch {
		case ln.label == "":
			b.WriteByte('\n')
		case ln.head && ln.wiki:
			// count, gap, percent, gap, tier, gap, then the same pair for axis and confidence.
			mainSpan := 6 + 2 + countW
			tierStart := nameW + 2 + mainSpan + 2
			tierSpan := len("tier")
			axisStart := tierStart + tierSpan + 2
			axisSpan := 6 + 2 + axisW
			confStart := axisStart + axisSpan + 2
			confSpan := 6 + 2 + confW
			hdr := []byte(strings.Repeat(" ", confStart+confSpan))
			copy(hdr, ln.label)
			copy(hdr[tierStart:], "tier")
			copy(hdr[axisStart+axisSpan-len("axis"):], "axis")
			copy(hdr[confStart+confSpan-len("confidence"):], "confidence")
			b.Write(hdr)
			b.WriteByte('\n')
		case ln.head && ln.me7info:
			me7Start := nameW + 2
			me7Span := countW + beyondW + 11
			corpusStart := me7Start + me7Span + 2
			corpusSpan := 8 + corpusW
			end := corpusStart + corpusSpan
			if showExtras {
				end += 2 + 6 + 2 + extrasW
			}
			hdr := []byte(strings.Repeat(" ", end))
			copy(hdr, ln.label)
			copy(hdr[me7Start+me7Span-len("vs ecu-specific"):], "vs ecu-specific")
			copy(hdr[corpusStart+corpusSpan-len("vs corpus"):], "vs corpus")
			if showExtras {
				copy(hdr[end-len("extras"):], "extras")
			}
			b.Write(hdr)
			b.WriteByte('\n')
		case ln.head && ln.xdf:
			mainSpan := 6 + 2 + countW
			axisStart := nameW + 2 + mainSpan + 2
			axisSpan := 6 + 2 + axisW
			hdr := []byte(strings.Repeat(" ", axisStart+axisSpan))
			copy(hdr, ln.label)
			copy(hdr[axisStart+axisSpan-len("axis"):], "axis")
			b.Write(hdr)
			b.WriteByte('\n')
		case ln.head:
			b.WriteString(ln.label)
			b.WriteByte('\n')
		case ln.me7info:
			extraPct := ""
			if ln.extras.Total > 0 {
				extraPct = ln.extras.percent()
			}
			fmt.Fprintf(&b, "%-*s  %*s (%*s)  %6s  %*s  %6s  %*s  %6s\n",
				nameW, ln.label, countW, counts[i], beyondW, beyonds[i], ln.frac.percent(),
				corpusW, corpus[i], ln.corpus.percent(), extrasW, extras[i], extraPct)
		case ln.wiki:
			confPct := ""
			if ln.conf.Total > 0 {
				confPct = ln.conf.percent()
			}
			fmt.Fprintf(&b, "%-*s  %*s  %6s  %-4s  %*s  %6s  %*s  %6s\n",
				nameW, ln.label, countW, counts[i], ln.frac.percent(), ln.tier,
				axisW, axes[i], ln.axis.percent(), confW, confs[i], confPct)
		case ln.xdf:
			fmt.Fprintf(&b, "%-*s  %*s  %6s  %*s  %6s\n",
				nameW, ln.label, countW, counts[i], ln.frac.percent(),
				axisW, axes[i], ln.axis.percent())
		default:
			fmt.Fprintf(&b, "%-*s  %*s  %6s\n", nameW, ln.label, countW, counts[i], ln.frac.percent())
		}
	}
	return b.String()
}

func stemName(name string) string {
	return strings.TrimSuffix(name, ".bin")
}

type ecuKey struct {
	name string
	addr uint32
	size int
	mask uint16
}

type mapKey struct {
	name string
	addr uint32
}

type imageGen func(name string, img []byte) ([]record.Item, []record.Map, error)

// Run generates each image and scores it.
// The images are the corpus names in images.yaml. Legacy rows are ecu/me7info/<image>.ecu.
// S4wiki names are xdf/s4wiki/names.yaml, one list for every image.
// An address oracle is xdf/<image>.xdf when that file exists.
func Run(dir string, c *ecucorpus.Corpus) (*Report, error) {
	images, err := c.List(filepath.Join(dir, "images.yaml"))
	if err != nil {
		return nil, err
	}
	return run(dir, images, generateImage)
}

func generateImage(name string, img []byte) ([]record.Item, []record.Map, error) {
	res, err := generate.Generate(generate.Options{
		Image: img, ImageName: name, Scale: "off",
	})
	if err != nil {
		return nil, nil, err
	}
	return res.File.Items, res.Maps, nil
}

func run(dir string, images []string, gen imageGen) (*Report, error) {
	cat, err := catalogNames()
	if err != nil {
		return nil, err
	}
	meas, err := measurementNames()
	if err != nil {
		return nil, err
	}
	catSet := nameSet(cat)
	measSet := nameSet(meas)
	wiki, axes, err := loadWiki(dir)
	if err != nil {
		return nil, err
	}
	if _, err := loadNamesPriority(dir, axes); err != nil {
		return nil, err
	}
	layout, err := loadLayoutTiers(dir)
	if err != nil {
		return nil, err
	}
	tierOf := map[string]string{}
	rep := &Report{}
	type kept struct {
		base, stem string
		img        []byte
		maps       []record.Map
		oracle     []refRow
	}
	var held []kept
	for _, path := range images {
		img, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(path)
		stem := strings.TrimSuffix(base, ".bin")
		tierOf[base] = layout[layoutID(img)]
		items, maps, err := gen(base, img)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", base, err)
		}
		names := itemNames(items)
		if len(meas) > 0 {
			rep.Extras = append(rep.Extras, Image{Name: base, Fraction: cover(meas, [][]string{names})})
		}
		if frac, beyond, ok, err := scoreME7(dir, stem, items, catSet, measSet); err != nil {
			return nil, err
		} else if ok {
			rep.ME7Info = append(rep.ME7Info, Image{
				Name: base, Fraction: frac, Beyond: beyond,
				Corpus: cover(cat, [][]string{names}),
			})
		}
		xmaps, xaxes, oracle, hasOracle, err := loadOracle(dir, stem)
		if err != nil {
			return nil, err
		}
		if len(wiki) > 0 {
			scored := wikiMaps(wiki, axes, maps, oracle)
			rep.S4Wiki = append(rep.S4Wiki, Image{
				Name: base, Fraction: countScored(wiki, scored),
				Tier: tierOf[base], Axis: scoreAxes(scored, axes),
			})
			held = append(held, kept{base: base, stem: stem, img: img, maps: maps, oracle: oracle})
		}
		if hasOracle {
			h, n := matchMaps(locatedMaps(maps), xmaps)
			ah, an := matchAxes(locatedAxes(maps), xaxes)
			rep.XDF = append(rep.XDF, Image{
				Name: base, Fraction: Fraction{h, n}, Axis: Fraction{ah, an},
			})
		}
	}
	if len(wiki) > 0 && len(held) > 0 {
		groups, err := loadDatasets(dir)
		if err != nil {
			return nil, err
		}
		skip, err := loadConfidenceSkip(dir)
		if err != nil {
			return nil, err
		}
		for n := range skip {
			if _, ok := axes[n]; !ok {
				return nil, fmt.Errorf("confidence skip: %s is not an s4wiki name", n)
			}
		}
		confWiki := omitNames(wiki, skip)
		byStem := map[string]kept{}
		for _, h := range held {
			byStem[h.stem] = h
		}
		for i, h := range held {
			var peers []binBody
			for _, stem := range groups[h.stem] {
				o, ok := byStem[stem]
				if !ok {
					continue
				}
				peers = append(peers, binBody{
					img: o.img, maps: o.maps, scored: wikiMaps(wiki, axes, o.maps, o.oracle),
				})
			}
			rep.S4Wiki[i].Confidence = scoreConfidence(confWiki, axes, h.img, h.maps, h.oracle, peers)
		}
	}
	for _, ims := range [][]Image{rep.ME7Info, rep.Extras, rep.S4Wiki, rep.XDF} {
		sortImages(ims, tierOf)
	}
	return rep, nil
}

func scoreME7(dir, stem string, items []record.Item, cat, meas map[string]struct{}) (Fraction, int, bool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "ecu", "me7info", stem+".ecu"))
	if os.IsNotExist(err) {
		return Fraction{}, 0, false, nil
	}
	if err != nil {
		return Fraction{}, 0, false, err
	}
	want, err := ecu.ParseBytes(raw)
	if err != nil {
		return Fraction{}, 0, false, err
	}
	h, n := matchECU(items, want.Items)
	if n == 0 {
		return Fraction{}, 0, false, nil
	}
	return Fraction{h, n}, beyondME7(items, want.Items, cat, meas), true, nil
}

// beyondME7 counts catalog names located here that this ME7Info file does not
// name. A measurement name is not counted.
func beyondME7(items, want []record.Item, cat, meas map[string]struct{}) int {
	inFile := map[string]struct{}{}
	for _, it := range want {
		if it.Name != "" {
			inFile[it.Name] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	n := 0
	for _, it := range items {
		if it.Name == "" {
			continue
		}
		if _, ok := seen[it.Name]; ok {
			continue
		}
		seen[it.Name] = struct{}{}
		if _, ok := meas[it.Name]; ok {
			continue
		}
		if _, ok := inFile[it.Name]; ok {
			continue
		}
		if _, ok := cat[it.Name]; !ok {
			continue
		}
		n++
	}
	return n
}

func nameSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n != "" {
			out[n] = struct{}{}
		}
	}
	return out
}

func loadOracle(dir, stem string) (maps []Map, axes []Axis, rows []refRow, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, "xdf", stem+".xdf"))
	if os.IsNotExist(err) {
		return nil, nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, nil, false, err
	}
	maps, axes, rows, err = parseXDF(raw)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("%s.xdf: %w", stem, err)
	}
	return maps, axes, rows, true, nil
}

func catalogNames() ([]string, error) {
	scales, err := config.LoadScales("", "", "")
	if err != nil {
		return nil, err
	}
	tab, err := config.LoadCatalog("", scales)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(tab.ByName))
	for name := range tab.ByName {
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func measurementNames() ([]string, error) {
	ms, err := config.LoadMeasures("", "", "")
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range ms {
		if m.Name == "" {
			continue
		}
		if _, ok := seen[m.Name]; ok {
			continue
		}
		seen[m.Name] = struct{}{}
		out = append(out, m.Name)
	}
	return out, nil
}

func loadWiki(dir string) ([]string, map[string]int, error) {
	b, err := os.ReadFile(filepath.Join(dir, "xdf", "s4wiki", "names.yaml"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Names yaml.Node `yaml:"names"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("s4wiki names: %w", err)
	}
	if doc.Names.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("s4wiki names: want a map of axis counts")
	}
	out := make([]string, 0, len(doc.Names.Content)/2)
	axes := map[string]int{}
	for i := 0; i+1 < len(doc.Names.Content); i += 2 {
		n := strings.TrimSpace(doc.Names.Content[i].Value)
		var c int
		if err := doc.Names.Content[i+1].Decode(&c); err != nil || c < 0 || c > 2 {
			return nil, nil, fmt.Errorf("s4wiki names: %s axis count", n)
		}
		if _, ok := axes[n]; ok || n == "" {
			return nil, nil, fmt.Errorf("s4wiki names: %s repeated", n)
		}
		axes[n] = c
		out = append(out, n)
	}
	return out, axes, nil
}

// tierOrder is the priority order of a *-priority.yaml file, highest first.
var tierOrder = []string{"S", "A", "B", "C", "D"}

// loadNamesPriority reads xdf/s4wiki/names-priority.yaml, the finder tier of each S4wiki name.
func loadNamesPriority(dir string, axes map[string]int) (map[string]string, error) {
	return loadTiers(filepath.Join(dir, "xdf", "s4wiki", "names-priority.yaml"), axes)
}

// loadTiers reads a priority file: a tiers map from a tierOrder label to members.
// Every key of want is in exactly one tier. A missing file returns nil.
func loadTiers[V any](path string, want map[string]V) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	var doc struct {
		Tiers map[string][]string `yaml:"tiers"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	out := map[string]string{}
	for tier, members := range doc.Tiers {
		if !slices.Contains(tierOrder, tier) {
			return nil, fmt.Errorf("%s: tier %q is not one of %v", name, tier, tierOrder)
		}
		for _, m := range members {
			if _, ok := want[m]; !ok {
				return nil, fmt.Errorf("%s: %s is not listed in the file it ranks", name, m)
			}
			if _, ok := out[m]; ok {
				return nil, fmt.Errorf("%s: %s repeated", name, m)
			}
			out[m] = tier
		}
	}
	for m := range want {
		if _, ok := out[m]; !ok {
			return nil, fmt.Errorf("%s: %s has no tier", name, m)
		}
	}
	return out, nil
}

var epkRE = regexp.MustCompile(`[0-9]+/[0-9]+/ME7[!-~]*`)

// layoutID is the id layouts.yaml lists an image by: the Bosch software
// number, or the EPK string when the image carries none.
func layoutID(img []byte) string {
	if sw := ident.Find(img).SWNumber; sw != "" {
		return sw
	}
	return string(epkRE.Find(img))
}

// loadBlocks reads layouts.yaml, the layout ids of each code layout block.
// A missing file returns nil.
func loadBlocks(dir string) (map[string][]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "layouts.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var blocks map[string][]string
	if err := yaml.Unmarshal(b, &blocks); err != nil {
		return nil, fmt.Errorf("layouts.yaml: %w", err)
	}
	return blocks, nil
}

// loadLayoutTiers maps each layout id to the tier of its block in layouts-priority.yaml.
func loadLayoutTiers(dir string) (map[string]string, error) {
	blocks, err := loadBlocks(dir)
	if err != nil || blocks == nil {
		return nil, err
	}
	tiers, err := loadTiers(filepath.Join(dir, "layouts-priority.yaml"), blocks)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for block, ids := range blocks {
		for _, id := range ids {
			out[id] = tiers[block]
		}
	}
	return out, nil
}

// tierRank is the index of label in tierOrder. No tier sorts last.
func tierRank(label string) int {
	if i := slices.Index(tierOrder, label); i >= 0 {
		return i
	}
	return len(tierOrder)
}

// sortImages orders rows by the layout tier of their image, then by name.
func sortImages(ims []Image, tierOf map[string]string) {
	slices.SortStableFunc(ims, func(a, b Image) int {
		if c := cmp.Compare(tierRank(tierOf[a.Name]), tierRank(tierOf[b.Name])); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// cover counts each wanted name once. A hit is a name located on any bin.
func cover(want []string, located [][]string) Fraction {
	have := map[string]struct{}{}
	for _, bin := range located {
		for _, n := range bin {
			if n != "" {
				have[n] = struct{}{}
			}
		}
	}
	seen := map[string]struct{}{}
	hit := 0
	for _, n := range want {
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		if _, ok := have[n]; ok {
			hit++
		}
	}
	return Fraction{hit, len(seen)}
}

func itemNames(items []record.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Name != "" {
			out = append(out, it.Name)
		}
	}
	return out
}

// wikiScore is the tuner names and, beside them, the axes of the maps that
// scored. A name counts at one address with an axis. An axis count of 0 is a
// scalar, so the address is enough. A second address is a miss. When the XDF
// names that row, the body address has to match. dims is how many axes that
// table has: 1 is the column, 2 is the column and the row. A hit is that axis
// present on the map. The address file is a separate score.
func wikiScore(want []string, dims map[string]int, maps []record.Map, rows []refRow) (names, axes Fraction) {
	scored := wikiMaps(want, dims, maps, rows)
	return countScored(want, scored), scoreAxes(scored, dims)
}

func countScored(want []string, scored map[string]record.Map) Fraction {
	hit := 0
	for _, n := range want {
		if _, ok := scored[n]; ok {
			hit++
		}
	}
	return Fraction{hit, len(want)}
}

// scoreWiki is the name half of wikiScore.
func scoreWiki(want []string, dims map[string]int, maps []record.Map, rows []refRow) Fraction {
	names, _ := wikiScore(want, dims, maps, rows)
	return names
}

func scoreAxes(scored map[string]record.Map, dims map[string]int) Fraction {
	hit, total := 0, 0
	for name, m := range scored {
		n, ok := dims[name]
		if ok && n == 0 {
			continue
		}
		if !ok {
			n = 1
			if axisPresent(m, "y") {
				n = 2
			}
		}
		total += n
		if n >= 1 && axisPresent(m, "x") {
			hit++
		}
		if n >= 2 && axisPresent(m, "y") {
			hit++
		}
	}
	return Fraction{hit, total}
}

func axisPresent(m record.Map, id string) bool {
	a := axisByID(m, id)
	return a != nil && a.Addr != 0
}

func axisByID(m record.Map, id string) *record.Axis {
	switch id {
	case "x":
		return m.X
	case "y":
		return m.Y
	}
	return nil
}

func mapAxis(m record.Map) bool {
	return axisPresent(m, "x") || axisPresent(m, "y")
}

type axisSig struct {
	id    string
	addr  uint32
	count int
	bits  int
}

// refRow is one constant or table in the image's XDF. addr is the body file offset.
type refRow struct {
	name string
	addr uint32
	axes map[string]axisSig
}

// referenceHit is true when this image has no XDF row of that name, or one row
// has this body address. The axes are scored on their own.
func referenceHit(m record.Map, rows []refRow) bool {
	if len(rows) == 0 {
		return true
	}
	off := opcode.FileOffset(m.Addr)
	seen := false
	for _, row := range rows {
		if row.name != m.Name {
			continue
		}
		seen = true
		if row.addr == off {
			return true
		}
	}
	return !seen
}

func matchECU(got []record.Item, want []record.Item) (int, int) {
	have := map[ecuKey]int{}
	for _, it := range got {
		if it.Name == "" {
			continue
		}
		have[ecuKey{it.Name, it.Addr, it.Size, it.Bitmask}]++
	}
	hit := 0
	for _, it := range want {
		if it.Name == "" {
			continue
		}
		k := ecuKey{it.Name, it.Addr, it.Size, it.Bitmask}
		if have[k] == 0 {
			continue
		}
		have[k]--
		hit++
	}
	return hit, len(want)
}

func locatedMaps(maps []record.Map) []Map {
	out := make([]Map, 0, len(maps))
	for _, m := range maps {
		if m.Name == "" {
			continue
		}
		out = append(out, Map{Name: m.Name, Addr: m.Addr})
	}
	return out
}

func matchMaps(got, want []Map) (int, int) {
	have := map[mapKey]int{}
	for _, m := range got {
		have[mapKey{m.Name, opcode.FileOffset(m.Addr)}]++
	}
	hit := 0
	for _, m := range want {
		k := mapKey{m.Name, opcode.FileOffset(m.Addr)}
		if have[k] == 0 {
			continue
		}
		have[k]--
		hit++
	}
	return hit, len(want)
}

type axisKey struct {
	name  string
	id    string
	addr  uint32
	count int
	bits  int
}

func locatedAxes(maps []record.Map) []Axis {
	var out []Axis
	for _, m := range maps {
		if m.Name == "" {
			continue
		}
		if m.X != nil && m.X.Addr != 0 {
			out = append(out, Axis{Name: m.Name, ID: "x", Addr: m.X.Addr, Count: m.X.Count, Bits: m.X.Bits})
		}
		if m.Y != nil && m.Y.Addr != 0 {
			out = append(out, Axis{Name: m.Name, ID: "y", Addr: m.Y.Addr, Count: m.Y.Count, Bits: m.Y.Bits})
		}
	}
	return out
}

func matchAxes(got, want []Axis) (int, int) {
	have := map[axisKey]int{}
	for _, a := range got {
		have[axisKey{a.Name, a.ID, opcode.FileOffset(a.Addr), a.Count, a.Bits}]++
	}
	hit := 0
	for _, a := range want {
		k := axisKey{a.Name, a.ID, opcode.FileOffset(a.Addr), a.Count, a.Bits}
		if have[k] == 0 {
			continue
		}
		have[k]--
		hit++
	}
	return hit, len(want)
}

// Map is one located calibration map. Addr may be a file offset or a CPU address.
type Map struct {
	Name string
	Addr uint32
}

// Axis is one x or y axis that has an address. Addr may be a file offset or a CPU address.
// Count is the point count. Bits is the element width.
type Axis struct {
	Name  string
	ID    string
	Addr  uint32
	Count int
	Bits  int
}

// ParseXDF reads map titles and the address of each constant or table body.
type xdfFile struct {
	Constants []xdfConst `xml:"XDFCONSTANT"`
	Tables    []xdfTable `xml:"XDFTABLE"`
}

type xdfConst struct {
	Title string  `xml:"title"`
	Data  xdfData `xml:"EMBEDDEDDATA"`
}

type xdfTable struct {
	Title string    `xml:"title"`
	Axes  []xdfAxis `xml:"XDFAXIS"`
}

type xdfAxis struct {
	ID    string  `xml:"id,attr"`
	Data  xdfData `xml:"EMBEDDEDDATA"`
	Count int     `xml:"indexcount"`
}

type xdfData struct {
	Addr string `xml:"mmedaddress,attr"`
	Bits int    `xml:"mmedelementsizebits,attr"`
}

// ParseXDF returns one row per constant and one per table body.
func ParseXDF(b []byte) ([]Map, error) {
	maps, _, _, err := parseXDF(b)
	return maps, err
}

// ParseXDFAxes returns each x or y axis that carries an address.
// A label list with no address is left out. The table body is not an axis.
func ParseXDFAxes(b []byte) ([]Axis, error) {
	_, axes, _, err := parseXDF(b)
	return axes, err
}

func parseXDF(b []byte) ([]Map, []Axis, []refRow, error) {
	var doc xdfFile
	if err := xml.Unmarshal(b, &doc); err != nil {
		return nil, nil, nil, err
	}
	var maps []Map
	var axes []Axis
	var rows []refRow
	for _, c := range doc.Constants {
		addr, err := parseAddr(c.Data.Addr)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", c.Title, err)
		}
		maps = append(maps, Map{Name: c.Title, Addr: addr})
		rows = append(rows, refRow{name: c.Title, addr: addr})
	}
	for _, t := range doc.Tables {
		var addr uint32
		var found bool
		ax := map[string]axisSig{}
		for _, a := range t.Axes {
			if a.ID == "z" {
				if a.Data.Addr == "" {
					continue
				}
				z, err := parseAddr(a.Data.Addr)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("%s: %w", t.Title, err)
				}
				addr, found = z, true
				continue
			}
			if a.ID != "x" && a.ID != "y" || a.Data.Addr == "" {
				continue
			}
			at, err := parseAddr(a.Data.Addr)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("%s %s: %w", t.Title, a.ID, err)
			}
			axes = append(axes, Axis{Name: t.Title, ID: a.ID, Addr: at, Count: a.Count, Bits: a.Data.Bits})
			ax[a.ID] = axisSig{id: a.ID, addr: at, count: a.Count, bits: a.Data.Bits}
		}
		if !found {
			return nil, nil, nil, fmt.Errorf("%s: no table address", t.Title)
		}
		maps = append(maps, Map{Name: t.Title, Addr: addr})
		rows = append(rows, refRow{name: t.Title, addr: addr, axes: ax})
	}
	return maps, axes, rows, nil
}

func parseAddr(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("bad address %q", s)
	}
	return uint32(v), nil
}
