// Package parity scores generated rows against separate oracles.
// Matching each image's ME7Info file is the only hard mark.
// Catalog coverage, extras, the shared tuner list, a supplied XDF, and
// that file's axes are coverage. Outperform counts catalog names that file does not name.
package parity

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
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
// ME7Info is the only hard mark. Extras, Tuner, and XDF are coverage.
// An empty slice means that kind had no oracle.
// Corpus, on a ME7Info row, is catalog names located on that image over the
// full catalog. Extras is the measurement list on that image.
// Axis and Confidence are set on a Tuner row. Axis is the axes on the
// maps that scored, and a hit matches that image's XDF. Confidence is the
// body-byte result for the names that row scored.
// Axis on an XDF or Hand row is every axis in that file.
// XDF rows score DAMOS sourced corpus definitions. Hand rows score hand made
// ones, which are oracles, not targets. Origin located is not an oracle.
// Disagree lists tuner names located at an address a hand made definition
// does not have. Those names still hit.
type Report struct {
	ME7Info  []Image
	Extras   []Image
	Tuner    []Image
	XDF      []Image
	Hand     []Image
	Disagree []Disagreement
}

// Disagreement is one tuner name whose located body is not at any of the
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
// Tuner rows. Tier is the nameGrade of the image. On a Tuner row, Fraction
// is the tuner names and Block is the other names lists of the image's
// layout block. NoDef marks a Tuner row whose image has no corpus
// definition; Text prints it as a star after the tier.
// Every list is sorted by the layout block tier of its image in
// layouts-priority.yaml, then by name.
type Image struct {
	Name string
	Fraction
	Beyond     int
	Corpus     Fraction
	Tier       string
	NoDef      bool
	Axis       Fraction
	Confidence Fraction
	Block      Fraction
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
		block   Fraction
		extras  Fraction
		head    bool
		beyond  int
		me7info bool
		tuner   bool
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
	if len(r.Tuner) > 0 {
		if len(lines) > 0 {
			lines = append(lines, line{})
		}
		lines = append(lines, line{label: "names", head: true, tuner: true})
		for _, im := range r.Tuner {
			tier := im.Tier
			if im.NoDef {
				tier += "*"
			}
			lines = append(lines, line{
				label: "  " + stemName(im.Name), frac: im.Fraction, tier: tier,
				axis: im.Axis, conf: im.Confidence, block: im.Block, tuner: true,
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
	axisW, confW, extrasW, blockW := 0, 0, 0, 0
	counts := make([]string, len(lines))
	blocks := make([]string, len(lines))
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
		if ln.tuner || ln.xdf {
			axes[i] = fmt.Sprintf("%d/%d", ln.axis.Hit, ln.axis.Total)
			if len(axes[i]) > axisW {
				axisW = len(axes[i])
			}
		}
		if ln.tuner && ln.conf.Total > 0 {
			confs[i] = fmt.Sprintf("%d/%d", ln.conf.Hit, ln.conf.Total)
			if len(confs[i]) > confW {
				confW = len(confs[i])
			}
		}
		if ln.tuner && ln.block.Total > 0 {
			blocks[i] = fmt.Sprintf("%d/%d", ln.block.Hit, ln.block.Total)
			if len(blocks[i]) > blockW {
				blockW = len(blocks[i])
			}
		}
	}
	showBlock := blockW > 0
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
		case ln.head && ln.tuner:
			// count, gap, percent, gap, tier, gap, then the same pair for axis and confidence.
			mainSpan := 6 + 2 + countW
			tierStart := nameW + 2 + mainSpan + 2
			axisStart := tierStart + len("tier") + 2
			axisSpan := 6 + 2 + axisW
			confStart := axisStart + axisSpan + 2
			confSpan := 6 + 2 + confW
			end := confStart + confSpan
			if showBlock {
				end += 2 + blockW + 2 + 6
			}
			hdr := []byte(strings.Repeat(" ", end))
			copy(hdr, ln.label)
			copy(hdr[tierStart:], "tier")
			copy(hdr[axisStart+axisSpan-len("axis"):], "axis")
			copy(hdr[confStart+confSpan-len("confidence"):], "confidence")
			if showBlock {
				copy(hdr[end-len("block"):], "block")
			}
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
		case ln.tuner:
			confPct := ""
			if ln.conf.Total > 0 {
				confPct = ln.conf.percent()
			}
			fmt.Fprintf(&b, "%-*s  %*s  %6s  %-4s  %*s  %6s  %*s  %6s",
				nameW, ln.label, countW, counts[i], ln.frac.percent(), ln.tier,
				axisW, axes[i], ln.axis.percent(), confW, confs[i], confPct)
			if showBlock {
				blockPct := ""
				if ln.block.Total > 0 {
					blockPct = ln.block.percent()
				}
				fmt.Fprintf(&b, "  %*s  %6s", blockW, blocks[i], blockPct)
			}
			b.WriteByte('\n')
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
// The names scored are names/tuner.yaml on every image, plus each other
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
	tuner, axes := lists.tuner, lists.dims
	ntier := lists.tier
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
		def := defOf(stem)
		xmaps, xaxes, xrows, kind, err := loadOracle(def)
		if err != nil {
			return nil, err
		}
		var oracle []refRow
		if kind == damosXDF {
			oracle = xrows
		}
		if len(tuner) > 0 {
			blk := block[layoutID(img)]
			want := lists.forBlock(blk)
			scored := tunerMaps(want, axes, maps, oracle)
			if kind == handXDF {
				rep.Disagree = append(rep.Disagree, disagreements(base, want, scored, xrows)...)
			}
			gone := absent[blk]
			rep.Tuner = append(rep.Tuner, Image{
				Name: base, Fraction: countScored(tuner, scored),
				Block: countScored(lists.byBlock[blk], scored),
				Tier: nameGrade(ntier, lists.byBlock[blk], lists.any, func(n string) (bool, bool) {
					_, located := scored[n]
					return located || gone[n], located
				}),
				NoDef: def == "",
				Axis:  scoreAxes(scored, axes),
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
	if len(tuner) > 0 && len(held) > 0 {
		groups, err := loadDatasets(dir)
		if err != nil {
			return nil, err
		}
		skip, err := loadConfidenceSkip(dir)
		if err != nil {
			return nil, err
		}
		for n := range skip {
			if !slices.Contains(tuner, n) {
				return nil, fmt.Errorf("confidence skip: %s is not a tuner name", n)
			}
		}
		confTuner := omitNames(tuner, skip)
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
					img: o.img, maps: o.maps, scored: tunerMaps(tuner, axes, o.maps, o.oracle),
				})
			}
			rep.Tuner[i].Confidence = scoreConfidence(confTuner, axes, h.img, h.maps, h.oracle, peers)
		}
	}
	for _, ims := range [][]Image{rep.ME7Info, rep.Extras, rep.Tuner, rep.XDF, rep.Hand} {
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

// loadOracle reads the image's corpus definition at path and returns its
// kind, or "" when path is "" or the origin is located. Its provenance
// origin (xdfkit docs/corpus.md) damos or a2l is damosXDF; hand, or none,
// is handXDF. located is a program's placement, so it is not scored.
func loadOracle(path string) (maps []Map, axes []Axis, rows []refRow, kind string, err error) {
	if path == "" {
		return nil, nil, nil, "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, "", err
	}
	maps, axes, rows, origin, err := parseModel(b)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("%s: %w", path, err)
	}
	switch origin {
	case "damos", "a2l":
		kind = damosXDF
	case "hand", "":
		kind = handXDF
	case "located":
		return nil, nil, nil, "", nil
	default:
		return nil, nil, nil, "", fmt.Errorf("%s: unknown origin %q", path, origin)
	}
	return maps, axes, rows, kind, nil
}

// disagreements lists the scored names whose body is not at a hand made XDF
// row of that name.
func disagreements(image string, tuner []string, scored map[string]record.Map, rows []refRow) []Disagreement {
	var out []Disagreement
	for _, n := range tuner {
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

// The name lists are names/<source>.yaml. tuner.yaml is scored on every
// image and is the only list with tiers.
const (
	namesDir   = "names"
	tunerList  = "tuner"
	absentFile = "absent.yaml"
)

// nameLists is every names/*.yaml list. tuner is tuner.yaml. byBlock is the
// other names of each layout block, in file order, without tuner names.
// dims is the axis count of every name; tuner.yaml wins a conflict.
// tier is the finder tier of each tuner name. any is the families in
// tuner.yaml: one tier slot, met by any member.
type nameLists struct {
	tuner   []string
	byBlock map[string][]string
	dims    map[string]int
	tier    map[string]string
	any     []family
}

// forBlock is the names scored on an image of block.
func (l nameLists) forBlock(block string) []string {
	if len(l.byBlock[block]) == 0 {
		return l.tuner
	}
	return append(slices.Clip(l.tuner), l.byBlock[block]...)
}

// loadNames reads names/*.yaml. A list other than tuner.yaml names a
// layout block in block. A missing tuner.yaml returns no lists.
func loadNames(dir string, blocks map[string][]string) (nameLists, error) {
	out := nameLists{byBlock: map[string][]string{}, dims: map[string]int{}}
	tuner, _, tier, families, err := loadNameList(filepath.Join(dir, namesDir, tunerList+".yaml"))
	if os.IsNotExist(err) {
		return nameLists{}, nil
	}
	if err != nil {
		return nameLists{}, err
	}
	if tier == nil {
		return nameLists{}, fmt.Errorf("%s.yaml: want tiers", tunerList)
	}
	out.tier = tier
	out.any = families
	for _, n := range tuner {
		out.tuner = append(out.tuner, n.name)
		out.dims[n.name] = n.axes
	}
	paths, err := filepath.Glob(filepath.Join(dir, namesDir, "*.yaml"))
	if err != nil {
		return nameLists{}, err
	}
	for _, p := range paths {
		base := filepath.Base(p)
		if base == tunerList+".yaml" || base == absentFile {
			continue
		}
		list, block, tier, families, err := loadNameList(p)
		if err != nil {
			return nameLists{}, err
		}
		if tier != nil {
			return nameLists{}, fmt.Errorf("%s: only %s.yaml has tiers", base, tunerList)
		}
		if len(families) > 0 {
			return nameLists{}, fmt.Errorf("%s: only %s.yaml has any", base, tunerList)
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

// family is one tier slot. It is met when one name is located.
type family struct {
	tier  string
	names []string
}

// loadNameList reads one list and its block. names maps an axis count (0, 1,
// or 2) to names. tiers instead maps a tierOrder label to such a map, and
// the tier of each name is returned. any is the families, each one slot.
func loadNameList(path string) ([]listName, string, map[string]string, []family, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", nil, nil, err
	}
	base := filepath.Base(path)
	var doc struct {
		Block string                      `yaml:"block"`
		Names map[int][]string            `yaml:"names"`
		Tiers map[string]map[int][]string `yaml:"tiers"`
		Any   []struct {
			Tier  string   `yaml:"tier"`
			Axes  int      `yaml:"axes"`
			Names []string `yaml:"names"`
		} `yaml:"any"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, "", nil, nil, fmt.Errorf("%s: %w", base, err)
	}
	groups := map[string]map[int][]string{"": doc.Names}
	order := []string{""}
	var tier map[string]string
	if doc.Tiers != nil {
		groups, order, tier = doc.Tiers, tierOrder, map[string]string{}
		for t := range doc.Tiers {
			if !slices.Contains(tierOrder, t) {
				return nil, "", nil, nil, fmt.Errorf("%s: tier %q is not one of %v", base, t, tierOrder)
			}
		}
	}
	var out []listName
	seen := map[string]bool{}
	add := func(n, t string, c int) error {
		if seen[n] || n == "" {
			return fmt.Errorf("%s: %q repeated", base, n)
		}
		if c < 0 || c > 2 {
			return fmt.Errorf("%s: axis count %d", base, c)
		}
		seen[n] = true
		out = append(out, listName{n, c})
		if tier != nil {
			tier[n] = t
		}
		return nil
	}
	for _, t := range order {
		for c, names := range groups[t] {
			for _, n := range names {
				if err := add(n, t, c); err != nil {
					return nil, "", nil, nil, err
				}
			}
		}
	}
	var families []family
	for _, g := range doc.Any {
		if !slices.Contains(tierOrder, g.Tier) {
			return nil, "", nil, nil, fmt.Errorf("%s: tier %q is not one of %v", base, g.Tier, tierOrder)
		}
		if len(g.Names) < 2 {
			return nil, "", nil, nil, fmt.Errorf("%s: any group needs two names", base)
		}
		for _, n := range g.Names {
			if err := add(n, g.Tier, g.Axes); err != nil {
				return nil, "", nil, nil, err
			}
		}
		families = append(families, family{g.Tier, g.Names})
	}
	slices.SortStableFunc(out, func(a, b listName) int {
		return cmp.Or(slices.Index(order, tier[a.name])-slices.Index(order, tier[b.name]), strings.Compare(a.name, b.name))
	})
	return out, doc.Block, tier, families, nil
}

// tierOrder is the priority order of a tiers map, highest first.
var tierOrder = []string{"S", "A", "B", "C", "D"}

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
// A family is one name: one member located, or every member absent.
// rest is the untiered names, one level above S. hit reports (counts, located).
func nameGrade(tierOf map[string]string, rest []string, families []family, hit func(string) (bool, bool)) string {
	inFamily := map[string]bool{}
	for _, f := range families {
		for _, n := range f.names {
			inFamily[n] = true
		}
	}
	done := map[string]bool{}
	for _, t := range tierOrder {
		done[t] = true
	}
	for n, t := range tierOf {
		if inFamily[n] {
			continue
		}
		if counts, _ := hit(n); !counts {
			done[t] = false
		}
	}
	for _, f := range families {
		if !familyMet(f, hit) {
			done[f.tier] = false
		}
	}
	grade := "-"
	for i := len(tierOrder) - 1; i >= 0 && done[tierOrder[i]]; i-- {
		grade = tierOrder[i]
	}
	if grade == tierOrder[0] && len(rest) > 0 && !slices.ContainsFunc(rest, func(n string) bool {
		counts, _ := hit(n)
		return !counts
	}) {
		grade = beyondS
	}
	return grade
}

// familyMet is true when one member is located, or every member is absent.
func familyMet(f family, hit func(string) (bool, bool)) bool {
	allAbsent := len(f.names) > 0
	for _, n := range f.names {
		counts, located := hit(n)
		if located {
			return true
		}
		if !counts {
			allAbsent = false
		}
	}
	return allAbsent
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

// tunerScore is the tuner names and, beside them, the axes of the maps that
// scored. A name counts at one address with an axis. An axis count of 0 is a
// scalar, so the address is enough. A second address is a miss. When the XDF
// names that row, the body address has to match. dims is how many axes that
// table has: 1 is the column, 2 is the column and the row. A hit is that axis
// present on the map. The address file is a separate score.
func tunerScore(want []string, dims map[string]int, maps []record.Map, rows []refRow) (names, axes Fraction) {
	scored := tunerMaps(want, dims, maps, rows)
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

// scoreTuner is the name half of tunerScore.
func scoreTuner(want []string, dims map[string]int, maps []record.Map, rows []refRow) Fraction {
	names, _ := tunerScore(want, dims, maps, rows)
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

// located is true when the axis is read from the image at an address. That
// includes a "subtract" axis, which xdfkit writes to XDF as labels.
func located(a *model.Axis) bool {
	return a != nil && a.Source == "image" && a.Address != nil && a.Data != nil
}

// parseModel reads a model JSON. The title is the first word of the id, else
// the description, as in the XDF xdfkit writes from it. origin is the
// provenance origin.
func parseModel(b []byte) (maps []Map, axes []Axis, rows []refRow, origin string, err error) {
	var m model.Model
	if err := canon.Unmarshal(b, &m); err != nil {
		return nil, nil, nil, "", err
	}
	if err := m.Check(); err != nil {
		return nil, nil, nil, "", err
	}
	titles := make([]string, len(m.Objects))
	for i, o := range m.Objects {
		titles[i], _, _ = strings.Cut(strings.TrimSpace(o.ID), " ")
		if titles[i] == "" {
			titles[i] = strings.TrimSpace(o.Description)
		}
	}
	nameOf := titleNamer(titles)
	for i, o := range m.Objects {
		name := nameOf(titles[i])
		row := refRow{name: name, addr: uint32(o.Address)}
		if o.Shape != "value" {
			row.axes = map[string]axisSig{}
			for _, a := range []struct {
				id    string
				ax    *model.Axis
				count int
			}{{"x", o.X, o.Cols}, {"y", o.Y, o.Rows}} {
				if !located(a.ax) {
					continue
				}
				at := uint32(*a.ax.Address)
				axes = append(axes, Axis{Name: name, ID: a.id, Addr: at, Count: a.count, Bits: a.ax.Data.Bits})
				row.axes[a.id] = axisSig{id: a.id, addr: at, count: a.count, bits: a.ax.Data.Bits}
			}
		}
		maps = append(maps, Map{Name: name, Addr: uint32(o.Address)})
		rows = append(rows, row)
	}
	if m.Provenance != nil {
		origin = m.Provenance.Origin
	}
	return maps, axes, rows, origin, nil
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
