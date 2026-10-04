// Package parity scores generated rows against separate oracles.
// Matching each image's ME7Info file is the only hard mark.
// Catalog coverage, extras, the shared S4wiki list, a supplied XDF, and
// that file's axes are coverage. Outperform counts catalog names that file does not name.
package parity

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"me7-logger/config"
	"me7-logger/ecu"
	"me7-logger/generate"
	"me7-logger/opcode"
	"me7-logger/record"
)

// Report is one pass over a parity root.
// ME7Info is the only hard mark. Extras, S4Wiki, XDF, and XDFAxis are coverage.
// An empty slice means that kind had no oracle.
// Corpus, on a ME7Info row, is catalog names located on that image over the
// full catalog. ExtrasAll counts each measurement name once across the bins.
type Report struct {
	ME7Info   []Image
	Extras    []Image
	ExtrasAll Fraction
	S4Wiki    []Image
	XDF       []Image
	XDFAxis   []Image
}

// Image is one binary scored against one oracle.
// Beyond is the count of catalog names located on this image that its
// ME7Info file does not name. Corpus is that image against the full catalog.
// Both are set only on ME7Info rows.
type Image struct {
	Name string
	Fraction
	Beyond int
	Corpus Fraction
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
// Names share a column and the percents share a column.
func (r *Report) Text() string {
	if r == nil {
		return ""
	}
	type line struct {
		label   string
		frac    Fraction
		corpus  Fraction
		head    bool
		beyond  int
		me7info bool
	}
	var lines []line
	addImages := func(title string, images []Image) {
		if len(images) == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, line{})
		}
		lines = append(lines, line{label: title, head: true})
		for _, im := range images {
			lines = append(lines, line{label: "  " + im.Name, frac: im.Fraction})
		}
	}
	if len(r.ME7Info) > 0 {
		lines = append(lines, line{label: "ecu me7info", head: true, me7info: true})
		for _, im := range r.ME7Info {
			lines = append(lines, line{
				label: "  " + im.Name, frac: im.Fraction, corpus: im.Corpus,
				beyond: im.Beyond, me7info: true,
			})
		}
	}
	if len(r.Extras) > 0 {
		if len(lines) > 0 {
			lines = append(lines, line{})
		}
		lines = append(lines, line{label: "ecu extras", head: true})
		for _, im := range r.Extras {
			lines = append(lines, line{label: "  " + im.Name, frac: im.Fraction})
		}
		lines = append(lines, line{label: "  all", frac: r.ExtrasAll})
	}
	addImages("xdf s4wiki", r.S4Wiki)
	addImages("xdf", r.XDF)
	addImages("xdf axis", r.XDFAxis)

	nameW, countW, beyondW, corpusW := 0, 0, 0, 0
	counts := make([]string, len(lines))
	beyonds := make([]string, len(lines))
	corpus := make([]string, len(lines))
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
		}
	}
	if len(r.ME7Info) > 0 && nameW < len("ecu me7info") {
		nameW = len("ecu me7info")
	}
	var b strings.Builder
	for i, ln := range lines {
		switch {
		case ln.label == "":
			b.WriteByte('\n')
		case ln.head && ln.me7info:
			me7Start := nameW + 2
			me7Span := countW + beyondW + 11
			corpusStart := me7Start + me7Span + 2
			corpusSpan := 8 + corpusW
			hdr := []byte(strings.Repeat(" ", corpusStart+corpusSpan))
			copy(hdr, ln.label)
			copy(hdr[me7Start+me7Span-len("vs ecu-specific"):], "vs ecu-specific")
			copy(hdr[corpusStart+corpusSpan-len("vs corpus"):], "vs corpus")
			b.Write(hdr)
			b.WriteByte('\n')
		case ln.head:
			b.WriteString(ln.label)
			b.WriteByte('\n')
		case ln.me7info:
			fmt.Fprintf(&b, "%-*s  %6s  %*s (%*s)  %6s  %*s\n",
				nameW, ln.label, ln.frac.percent(), countW, counts[i], beyondW, beyonds[i],
				ln.corpus.percent(), corpusW, corpus[i])
		default:
			fmt.Fprintf(&b, "%-*s  %6s  %*s\n", nameW, ln.label, ln.frac.percent(), countW, counts[i])
		}
	}
	return b.String()
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

// Run generates each bin and scores it.
// Bins are bin/*.bin. Legacy rows are ecu/me7info/<stem>.ecu.
// S4wiki names are xdf/s4wiki/names.yaml, one list for every bin.
// An address oracle is xdf/<stem>.xdf when that file exists.
func Run(dir string) (*Report, error) {
	return run(dir, generateImage)
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

func run(dir string, gen imageGen) (*Report, error) {
	bins, err := filepath.Glob(filepath.Join(dir, "bin", "*.bin"))
	if err != nil {
		return nil, err
	}
	sort.Strings(bins)
	if len(bins) == 0 {
		return nil, fmt.Errorf("%s: no image", dir)
	}
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
	wiki, values, err := loadWiki(dir)
	if err != nil {
		return nil, err
	}
	rep := &Report{}
	var located [][]string
	for _, bin := range bins {
		img, err := os.ReadFile(bin)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(bin)
		stem := strings.TrimSuffix(base, ".bin")
		items, maps, err := gen(base, img)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", base, err)
		}
		names := itemNames(items)
		located = append(located, names)
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
		if len(wiki) > 0 {
			rep.S4Wiki = append(rep.S4Wiki, Image{Name: base, Fraction: scoreWiki(wiki, values, maps)})
		}
		if body, axes, ok, err := scoreXDF(dir, stem, maps); err != nil {
			return nil, err
		} else if ok {
			rep.XDF = append(rep.XDF, Image{Name: base, Fraction: body})
			if axes.Total > 0 {
				rep.XDFAxis = append(rep.XDFAxis, Image{Name: base, Fraction: axes})
			}
		}
	}
	rep.ExtrasAll = cover(meas, located)
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

func scoreXDF(dir, stem string, maps []record.Map) (body, axes Fraction, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, "xdf", stem+".xdf"))
	if os.IsNotExist(err) {
		return Fraction{}, Fraction{}, false, nil
	}
	if err != nil {
		return Fraction{}, Fraction{}, false, err
	}
	want, wantAxes, err := parseXDF(raw)
	if err != nil {
		return Fraction{}, Fraction{}, false, fmt.Errorf("%s.xdf: %w", stem, err)
	}
	h, n := matchMaps(locatedMaps(maps), want)
	ah, an := matchAxes(locatedAxes(maps), wantAxes)
	return Fraction{h, n}, Fraction{ah, an}, true, nil
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

func loadWiki(dir string) ([]string, map[string]struct{}, error) {
	b, err := os.ReadFile(filepath.Join(dir, "xdf", "s4wiki", "names.yaml"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Names  []string `yaml:"names"`
		Values []string `yaml:"values"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("s4wiki names: %w", err)
	}
	seen := map[string]struct{}{}
	var out []string
	for _, n := range doc.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	values := map[string]struct{}{}
	for _, n := range doc.Values {
		n = strings.TrimSpace(n)
		if _, ok := seen[n]; ok {
			values[n] = struct{}{}
		}
	}
	return out, values, nil
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

// scoreWiki counts a name when the image locates it at one address and that
// map has an axis. A name in values has no axis in the image, so the address
// is enough. A second address is a miss. Any other body with no axis is a miss.
func scoreWiki(want []string, values map[string]struct{}, maps []record.Map) Fraction {
	addrs := map[string]map[uint32]struct{}{}
	axis := map[string]map[uint32]struct{}{}
	for _, m := range maps {
		if m.Name == "" {
			continue
		}
		off := opcode.FileOffset(m.Addr)
		if addrs[m.Name] == nil {
			addrs[m.Name] = map[uint32]struct{}{}
			axis[m.Name] = map[uint32]struct{}{}
		}
		addrs[m.Name][off] = struct{}{}
		if mapAxis(m) {
			axis[m.Name][off] = struct{}{}
		}
	}
	hit := 0
	for _, n := range want {
		if len(addrs[n]) != 1 {
			continue
		}
		for off := range addrs[n] {
			if _, ok := axis[n][off]; ok {
				hit++
				continue
			}
			if _, ok := values[n]; ok {
				hit++
			}
		}
	}
	return Fraction{hit, len(want)}
}

func mapAxis(m record.Map) bool {
	return m.X != nil && m.X.Addr != 0 || m.Y != nil && m.Y.Addr != 0
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
	maps, _, err := parseXDF(b)
	return maps, err
}

// ParseXDFAxes returns each x or y axis that carries an address.
// A label list with no address is left out. The table body is not an axis.
func ParseXDFAxes(b []byte) ([]Axis, error) {
	_, axes, err := parseXDF(b)
	return axes, err
}

func parseXDF(b []byte) ([]Map, []Axis, error) {
	var doc xdfFile
	if err := xml.Unmarshal(b, &doc); err != nil {
		return nil, nil, err
	}
	var maps []Map
	var axes []Axis
	for _, c := range doc.Constants {
		addr, err := parseAddr(c.Data.Addr)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", c.Title, err)
		}
		maps = append(maps, Map{Name: c.Title, Addr: addr})
	}
	for _, t := range doc.Tables {
		var addr uint32
		var found bool
		for _, ax := range t.Axes {
			if ax.ID == "z" {
				if ax.Data.Addr == "" {
					continue
				}
				a, err := parseAddr(ax.Data.Addr)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: %w", t.Title, err)
				}
				addr, found = a, true
				continue
			}
			if ax.ID != "x" && ax.ID != "y" || ax.Data.Addr == "" {
				continue
			}
			a, err := parseAddr(ax.Data.Addr)
			if err != nil {
				return nil, nil, fmt.Errorf("%s %s: %w", t.Title, ax.ID, err)
			}
			axes = append(axes, Axis{Name: t.Title, ID: ax.ID, Addr: a, Count: ax.Count, Bits: ax.Data.Bits})
		}
		if !found {
			return nil, nil, fmt.Errorf("%s: no table address", t.Title)
		}
		maps = append(maps, Map{Name: t.Title, Addr: addr})
	}
	return maps, axes, nil
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
