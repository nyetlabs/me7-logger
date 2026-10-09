// Package parity scores generated rows against separate oracles.
// Matching each image's ME7Info file is the only hard mark.
// Catalog coverage, extras, the shared S4wiki list, a supplied XDF, and
// that file's axes are coverage. Outperform counts catalog names that file does not name.
package parity

import (
	"cmp"
	"encoding/json"
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
// Axis on an XDF or Hand row is every axis in that file.
// XDF rows score DAMOS sourced corpus definitions. Hand rows score hand made
// ones, which are oracles, not targets.
// Disagree lists s4wiki names located at an address a hand made definition
// does not have. Those names still hit.
type Report struct {
	ME7Info  []Image
	Extras   []Image
	S4Wiki   []Image
	XDF      []Image
	Hand     []Image
	Disagree []Disagreement
}

// Disagreement is one s4wiki name whose located body is not at any of the
// hand made XDF's addresses for that name. Addresses are file offsets.
type Disagreement struct {
	Image, Name string
	Ours        uint32
	XDF         []uint32
}

// Image is one binary scored against one oracle.
// Beyond is the count of catalog names located on this image that its
// ME7Info file does not name. Corpus is that image against the full catalog.
// Both are set only on ME7Info rows. Tier, Axis, and Confidence are set on
// S4Wiki rows. Tier is the nameGrade of the image.
// Every list is sorted by the layout block tier of its image in
// layouts-priority.yaml, then by name.
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
		lines = append(lines, line{label: "names", head: true, wiki: true})
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
		lines = append(lines, line{label: "xdf damos", head: true, xdf: true})
		for _, im := range r.XDF {
			lines = append(lines, line{label: "  " + stemName(im.Name), frac: im.Fraction, axis: im.Axis, xdf: true})
		}
	}
	if len(r.Hand) > 0 {
		if len(lines) > 0 {
			lines = append(lines, line{})
		}
		lines = append(lines, line{label: "xdf hand", head: true, xdf: true})
		for _, im := range r.Hand {
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
			axisStart := tierStart + len("tier") + 2
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
	if len(r.Disagree) > 0 {
		b.WriteString("\nhand xdf disagrees\n")
		for _, d := range r.Disagree {
			xs := make([]string, len(d.XDF))
			for i, a := range d.XDF {
				xs[i] = fmt.Sprintf("0x%X", a)
			}
			fmt.Fprintf(&b, "  %s  %s  ours 0x%X  xdf %s\n", stemName(d.Image), d.Name, d.Ours, strings.Join(xs, ","))
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
// The names scored are names/s4wiki.yaml on every image, plus each other
// names/*.yaml list on the images of its layout block.
// An address oracle is the image's corpus definition, when the manifest names
// one. Only a DAMOS one can turn a name hit into a miss.
func Run(dir string, c *ecucorpus.Corpus) (*Report, error) {
	images, err := c.List(filepath.Join(dir, "images.yaml"))
	if err != nil {
		return nil, err
	}
	return run(dir, images, c.Def, generateImage)
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

// defOf returns the path of an image's definition, by stem, or "" for none.
func run(dir string, images []string, defOf func(string) string, gen imageGen) (*Report, error) {
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
	blocks, err := loadBlocks(dir)
	if err != nil {
		return nil, err
	}
	lists, err := loadNames(dir, blocks)
	if err != nil {
		return nil, err
	}
	wiki, axes := lists.wiki, lists.dims
	ntier, err := loadNamesPriority(dir, wiki)
	if err != nil {
		return nil, err
	}
	layout, err := loadLayoutTiers(dir, blocks)
	if err != nil {
		return nil, err
	}
	absent, err := loadAbsent(dir, blocks, axes)
	if err != nil {
		return nil, err
	}
	block := blockOf(blocks)
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
		xmaps, xaxes, xrows, kind, err := loadOracle(defOf(stem))
		if err != nil {
			return nil, err
		}
		var oracle []refRow
		if kind == damosXDF {
			oracle = xrows
		}
		if len(wiki) > 0 {
			blk := block[layoutID(img)]
			want := lists.forBlock(blk)
			scored := wikiMaps(want, axes, maps, oracle)
			if kind == handXDF {
				rep.Disagree = append(rep.Disagree, disagreements(base, want, scored, xrows)...)
			}
			gone := absent[blk]
			rep.S4Wiki = append(rep.S4Wiki, Image{
				Name: base, Fraction: countScored(want, scored),
				Tier: nameGrade(ntier, lists.byBlock[blk], func(n string) bool {
					_, ok := scored[n]
					return ok || gone[n]
				}),
				Axis: scoreAxes(scored, axes),
			})
			held = append(held, kept{base: base, stem: stem, img: img, maps: maps, oracle: oracle})
		}
		if kind != "" {
			h, n := matchMaps(locatedMaps(maps), xmaps)
			ah, an := matchAxes(locatedAxes(maps), xaxes)
			im := Image{Name: base, Fraction: Fraction{h, n}, Axis: Fraction{ah, an}}
			if kind == damosXDF {
				rep.XDF = append(rep.XDF, im)
			} else {
				rep.Hand = append(rep.Hand, im)
			}
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
			if !slices.Contains(wiki, n) {
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
	for _, ims := range [][]Image{rep.ME7Info, rep.Extras, rep.S4Wiki, rep.XDF, rep.Hand} {
		sortImages(ims, tierOf)
	}
	slices.SortStableFunc(rep.Disagree, func(a, b Disagreement) int {
		if c := cmp.Compare(tierRank(tierOf[a.Image]), tierRank(tierOf[b.Image])); c != 0 {
			return c
		}
		return cmp.Or(strings.Compare(a.Image, b.Image), strings.Compare(a.Name, b.Name))
	})
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

// The oracle kinds.
const (
	damosXDF = "damos"
	handXDF  = "hand"
)

// damosMaps is the map count above which a definition is DAMOS or A2L
// sourced, even when it came through a KP.
const damosMaps = 500

// loadOracle reads the image's corpus definition at path and returns its
// kind, or "" when path is "".
func loadOracle(path string) (maps []Map, axes []Axis, rows []refRow, kind string, err error) {
	if path == "" {
		return nil, nil, nil, "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, "", err
	}
	maps, axes, rows, err = parseModel(b)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("%s: %w", path, err)
	}
	kind = handXDF
	if len(rows) > damosMaps {
		kind = damosXDF
	}
	return maps, axes, rows, kind, nil
}

// disagreements lists the scored names whose body is not at a hand made XDF
// row of that name.
func disagreements(image string, wiki []string, scored map[string]record.Map, rows []refRow) []Disagreement {
	var out []Disagreement
	for _, n := range wiki {
		m, ok := scored[n]
		if !ok || referenceHit(m, rows) {
			continue
		}
		d := Disagreement{Image: image, Name: n, Ours: opcode.FileOffset(m.Addr)}
		for _, r := range rows {
			if r.name == n && !slices.Contains(d.XDF, r.addr) {
				d.XDF = append(d.XDF, r.addr)
			}
		}
		out = append(out, d)
	}
	return out
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

// The name lists are names/<source>.yaml. s4wiki.yaml is scored on every
// image and is the only list with tiers, in s4wiki-priority.yaml.
const (
	namesDir   = "names"
	wikiList   = "s4wiki"
	absentFile = "absent.yaml"
)

// nameLists is every names/*.yaml list. wiki is s4wiki.yaml. byBlock is the
// other names of each layout block, in file order, without s4wiki names.
// dims is the axis count of every name; s4wiki.yaml wins a conflict.
type nameLists struct {
	wiki    []string
	byBlock map[string][]string
	dims    map[string]int
}

// forBlock is the names scored on an image of block.
func (l nameLists) forBlock(block string) []string {
	if len(l.byBlock[block]) == 0 {
		return l.wiki
	}
	return append(slices.Clip(l.wiki), l.byBlock[block]...)
}

// loadNames reads names/*.yaml. A list other than s4wiki.yaml names a
// layout block in block. A missing s4wiki.yaml returns no lists.
func loadNames(dir string, blocks map[string][]string) (nameLists, error) {
	out := nameLists{byBlock: map[string][]string{}, dims: map[string]int{}}
	wiki, _, err := loadNameList(filepath.Join(dir, namesDir, wikiList+".yaml"))
	if os.IsNotExist(err) {
		return nameLists{}, nil
	}
	if err != nil {
		return nameLists{}, err
	}
	for _, n := range wiki {
		out.wiki = append(out.wiki, n.name)
		out.dims[n.name] = n.axes
	}
	paths, err := filepath.Glob(filepath.Join(dir, namesDir, "*.yaml"))
	if err != nil {
		return nameLists{}, err
	}
	for _, p := range paths {
		base := filepath.Base(p)
		if base == wikiList+".yaml" || base == absentFile || strings.HasSuffix(base, "-priority.yaml") {
			continue
		}
		list, block, err := loadNameList(p)
		if err != nil {
			return nameLists{}, err
		}
		if _, ok := blocks[block]; !ok {
			return nameLists{}, fmt.Errorf("%s: block %q is not in layouts.yaml", base, block)
		}
		for _, n := range list {
			if _, ok := out.dims[n.name]; ok {
				continue
			}
			out.dims[n.name] = n.axes
			out.byBlock[block] = append(out.byBlock[block], n.name)
		}
	}
	return out, nil
}

type listName struct {
	name string
	axes int
}

// loadNameList reads one list: names, a map of name to axis count, and block.
func loadNameList(path string) ([]listName, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	base := filepath.Base(path)
	var doc struct {
		Block string    `yaml:"block"`
		Names yaml.Node `yaml:"names"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, "", fmt.Errorf("%s: %w", base, err)
	}
	if doc.Names.Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("%s: want a map of axis counts", base)
	}
	out := make([]listName, 0, len(doc.Names.Content)/2)
	seen := map[string]bool{}
	for i := 0; i+1 < len(doc.Names.Content); i += 2 {
		n := strings.TrimSpace(doc.Names.Content[i].Value)
		var c int
		if err := doc.Names.Content[i+1].Decode(&c); err != nil || c < 0 || c > 2 {
			return nil, "", fmt.Errorf("%s: %s axis count", base, n)
		}
		if seen[n] || n == "" {
			return nil, "", fmt.Errorf("%s: %s repeated", base, n)
		}
		seen[n] = true
		out = append(out, listName{n, c})
	}
	return out, doc.Block, nil
}

// tierOrder is the priority order of a *-priority.yaml file, highest first.
var tierOrder = []string{"S", "A", "B", "C", "D"}

// loadNamesPriority reads names/s4wiki-priority.yaml, the finder tier of each S4wiki name.
func loadNamesPriority(dir string, wiki []string) (map[string]string, error) {
	return loadTiers(filepath.Join(dir, namesDir, wikiList+"-priority.yaml"), nameSet(wiki))
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
func loadLayoutTiers(dir string, blocks map[string][]string) (map[string]string, error) {
	if blocks == nil {
		return nil, nil
	}
	tiers, err := loadTiers(filepath.Join(dir, "layouts-priority.yaml"), blocks)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for id, block := range blockOf(blocks) {
		out[id] = tiers[block]
	}
	return out, nil
}

// blockOf maps each layout id to its block.
func blockOf(blocks map[string][]string) map[string]string {
	out := map[string]string{}
	for block, ids := range blocks {
		for _, id := range ids {
			out[id] = block
		}
	}
	return out
}

// loadAbsent reads names/absent.yaml: the listed names each layout block
// does not have. A missing file returns nil.
func loadAbsent(dir string, blocks map[string][]string, names map[string]int) (map[string]map[string]bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, namesDir, absentFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Absent map[string][]string `yaml:"absent"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("absent.yaml: %w", err)
	}
	out := map[string]map[string]bool{}
	for block, list := range doc.Absent {
		if _, ok := blocks[block]; !ok {
			return nil, fmt.Errorf("absent.yaml: %s is not a block in layouts.yaml", block)
		}
		out[block] = map[string]bool{}
		for _, n := range list {
			if _, ok := names[n]; !ok {
				return nil, fmt.Errorf("absent.yaml: %s is not in a name list", n)
			}
			out[block][n] = true
		}
	}
	return out, nil
}

// beyondS is the grade of an image that has S and every untiered name of
// its block's lists.
const beyondS = "S+"

// nameGrade is the highest name tier complete, counted up from D, or "-"
// when D is not. A tier is complete when each of its names is hit or absent.
// rest is the untiered names, one level above S.
func nameGrade(tierOf map[string]string, rest []string, hit func(string) bool) string {
	done := map[string]bool{}
	for _, t := range tierOrder {
		done[t] = true
	}
	for n, t := range tierOf {
		if !hit(n) {
			done[t] = false
		}
	}
	grade := "-"
	for i := len(tierOrder) - 1; i >= 0 && done[tierOrder[i]]; i-- {
		grade = tierOrder[i]
	}
	if grade == tierOrder[0] && len(rest) > 0 && !slices.ContainsFunc(rest, func(n string) bool { return !hit(n) }) {
		grade = beyondS
	}
	return grade
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
// has this body address. A DAMOS export lists a table whose axes are stored in
// front of the body with no axis addresses, at the count header in front of
// the first axis. A row whose 16-bit axis starts on the odd pad byte and whose
// body starts where that axis ends is one byte early. The axes are scored on
// their own.
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
		if row.addr == off || len(row.axes) == 0 && countHeader(m) == row.addr || padShifted(row) && row.addr+1 == off {
			return true
		}
	}
	return !seen
}

// padShifted is true when a 16-bit axis of row starts on an odd address and
// ends at the row body.
func padShifted(row refRow) bool {
	for _, a := range row.axes {
		if a.bits == 16 && a.addr%2 == 1 && a.addr+uint32(2*a.count) == row.addr {
			return true
		}
	}
	return false
}

// countHeader is the file offset of the counts in front of the first axis of
// a map whose axes are stored in front of its body. 0 is none.
func countHeader(m record.Map) uint32 {
	var first *record.Axis
	axes := 0
	for _, a := range []*record.Axis{m.X, m.Y} {
		if a == nil || a.Addr == 0 || a.Addr >= m.Addr {
			continue
		}
		axes++
		if first == nil || a.Addr < first.Addr {
			first = a
		}
	}
	if first == nil {
		return 0
	}
	width := 1
	if first.Bits == 16 {
		width = 2
	}
	return opcode.FileOffset(first.Addr) - uint32(axes*width)
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
	var titles []string
	for _, c := range doc.Constants {
		titles = append(titles, c.Title)
	}
	for _, t := range doc.Tables {
		titles = append(titles, t.Title)
	}
	nameOf := titleNamer(titles)
	var maps []Map
	var axes []Axis
	var rows []refRow
	for _, c := range doc.Constants {
		addr, err := parseAddr(c.Data.Addr)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", c.Title, err)
		}
		c.Title = nameOf(c.Title)
		maps = append(maps, Map{Name: c.Title, Addr: addr})
		rows = append(rows, refRow{name: c.Title, addr: addr})
	}
	for _, t := range doc.Tables {
		t.Title = nameOf(t.Title)
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

// modelDoc is the part of an xdfkit model JSON (corpus defs/) that locates
// maps.
type modelDoc struct {
	Objects []struct {
		ID          string     `json:"id"`
		Description string     `json:"description"`
		Shape       string     `json:"shape"`
		Address     string     `json:"address"`
		Rows        int        `json:"rows"`
		Cols        int        `json:"cols"`
		X           *modelAxis `json:"x"`
		Y           *modelAxis `json:"y"`
	} `json:"objects"`
}

type modelAxis struct {
	Source  string `json:"source"`
	Stored  string `json:"stored"`
	Address string `json:"address"`
	Data    *struct {
		Bits int `json:"bits"`
	} `json:"data"`
}

// located is true when the axis is read from the image at an address. That
// includes a "subtract" axis, which xdfkit writes to XDF as labels.
func (a *modelAxis) located() bool {
	return a != nil && a.Source == "image" && a.Address != "" && a.Data != nil
}

// parseModel reads a model JSON. The title is the first word of the id, else
// the description, as in the XDF xdfkit writes from it.
func parseModel(b []byte) ([]Map, []Axis, []refRow, error) {
	var doc modelDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, nil, err
	}
	titles := make([]string, len(doc.Objects))
	for i, o := range doc.Objects {
		titles[i], _, _ = strings.Cut(strings.TrimSpace(o.ID), " ")
		if titles[i] == "" {
			titles[i] = strings.TrimSpace(o.Description)
		}
	}
	nameOf := titleNamer(titles)
	var maps []Map
	var axes []Axis
	var rows []refRow
	for i, o := range doc.Objects {
		name := nameOf(titles[i])
		addr, err := parseAddr(o.Address)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		row := refRow{name: name, addr: addr}
		if o.Shape != "value" {
			row.axes = map[string]axisSig{}
			for _, a := range []struct {
				id    string
				ax    *modelAxis
				count int
			}{{"x", o.X, o.Cols}, {"y", o.Y, o.Rows}} {
				if !a.ax.located() {
					continue
				}
				at, err := parseAddr(a.ax.Address)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("%s %s: %w", name, a.id, err)
				}
				axes = append(axes, Axis{Name: name, ID: a.id, Addr: at, Count: a.count, Bits: a.ax.Data.Bits})
				row.axes[a.id] = axisSig{id: a.id, addr: at, count: a.count, bits: a.ax.Data.Bits}
			}
		}
		maps = append(maps, Map{Name: name, Addr: addr})
		rows = append(rows, row)
	}
	return maps, axes, rows, nil
}

var titleName = regexp.MustCompile(`\(([A-Z][A-Z0-9_]*)\)\s*$`)

// titleNamer maps a title to its name. A file whose titles are mostly a
// description followed by the Bosch name in parentheses is named by the
// parentheses. Any other file keeps its titles.
func titleNamer(titles []string) func(string) string {
	n := 0
	for _, s := range titles {
		if titleName.MatchString(s) {
			n++
		}
	}
	if 2*n <= len(titles) {
		return func(s string) string { return s }
	}
	return func(s string) string {
		if m := titleName.FindStringSubmatch(s); m != nil {
			return m[1]
		}
		return s
	}
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
