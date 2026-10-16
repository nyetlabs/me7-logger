package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/needle"
	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
	"gopkg.in/yaml.v3"
)

// A scale is the unit for a tuner name we locate. The factor comes from
// the unit row in config/units.yaml for that width. A DAMOS majority that
// is not that row keeps its factor on a maps row. A row with clock set
// takes its factor from the clock in that file. An
// oracle def already carries units for every name this cache does not list.
type scaleRow struct {
	Name       string  `yaml:"name"`
	Role       string  `yaml:"role"`
	Bits       int     `yaml:"bits"`
	Factor     float64 `yaml:"factor,omitempty"`
	Offset     float64 `yaml:"offset,omitempty"`
	Reciprocal bool    `yaml:"reciprocal,omitempty"`
	Table      bool    `yaml:"table,omitempty"`
	Units      string  `yaml:"units,omitempty"`
	Precision  int     `yaml:"precision,omitempty"`
	Clock      string  `yaml:"clock,omitempty"`
	lo, hi     *float64
}

type scaleKey struct {
	name, role string
	bits       int
}

type sample struct {
	file           string
	factor, offset float64
	recip          bool
	units          string
	prec           int
}

// Conversion is the scale for name, role, and width. mhz 0 uses the default
// clock in config/units.yaml. A form in config/sets.yaml with no row uses
// the plain name's row. A _1_A name that is not in a set uses the _0_A row.
// The second result is false when no majority covers that width. An empty
// factor uses the unit row for that width.
func Conversion(name, role string, bits, mhz int) (model.Value, bool) {
	scaleOnce.Do(loadScales)
	k := scaleKey{name, role, bits}
	row, ok := scales[k]
	if !ok && strings.HasSuffix(name, "_1_A") && !openScales[k] {
		row, ok = scales[scaleKey{strings.TrimSuffix(name, "_1_A") + "_0_A", role, bits}]
	}
	if !ok {
		return model.Value{}, false
	}
	if row.Clock != "" {
		f := clockFactor(row.Clock, mhz, bits)
		if f == 0 {
			return model.Value{}, false
		}
		return model.Value{
			Units:      row.Units,
			Precision:  row.Precision,
			Conversion: model.Conversion{Factor: f, Offset: row.Offset, Reciprocal: row.Reciprocal},
		}, true
	}
	if row.Factor == 0 && !row.Reciprocal {
		v, ok := unitScale(bits, row.Units)
		if !ok {
			return model.Value{}, false
		}
		v.Precision = row.Precision
		return v, true
	}
	return model.Value{
		Units:      row.Units,
		Precision:  row.Precision,
		Min:        row.lo,
		Max:        row.hi,
		Conversion: model.Conversion{Factor: row.Factor, Offset: row.Offset, Reciprocal: row.Reciprocal},
	}, true
}

var (
	scaleOnce  sync.Once
	scales     map[scaleKey]scaleRow
	openScales map[scaleKey]bool
	clockMHz   int
	clockTick  map[int]float64
)

// BuildScales reads defs in dir and returns config/units.yaml. dir is an
// ecu-corpus checkout.
func BuildScales(dir string) ([]byte, error) {
	names, err := LoadNames(NamesFile)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(filepath.Join(dir, "defs"))
	if err != nil {
		return nil, err
	}
	by := map[scaleKey][]sample{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "defs", e.Name()))
		if err != nil {
			return nil, err
		}
		var pack model.Model
		if err := json.Unmarshal(b, &pack); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if pack.Provenance == nil || (pack.Provenance.Origin != "damos" && pack.Provenance.Origin != "a2l") {
			continue
		}
		for _, o := range pack.Objects {
			name := objName(o)
			if name == "" {
				continue
			}
			addSample(by, scaleKey{name, "z", o.Data.Bits}, e.Name(), o.Value)
			addAxis(by, name, "x", e.Name(), o.X)
			addAxis(by, name, "y", e.Name(), o.Y)
		}
	}
	var ticks []float64
	for _, f := range names.Clock.Factors {
		ticks = append(ticks, f)
	}
	sort.Float64s(ticks)
	var rows []scaleRow
	keys := make([]scaleKey, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		if roleRank(keys[i].role) != roleRank(keys[j].role) {
			return roleRank(keys[i].role) < roleRank(keys[j].role)
		}
		return keys[i].bits < keys[j].bits
	})
	tuner := nameTiers()
	for _, k := range keys {
		if !tuner[k.name] {
			continue
		}
		row, ok, disagreed := agree(k, by[k], names, ticks)
		if !ok && disagreed {
			row, ok = rpmLoadMajority(k, by[k])
		}
		if !ok {
			continue
		}
		if row.Clock == "" {
			useTable(&row)
		}
		rows = append(rows, row)
	}
	if err := fillHand(dir, &rows, tuner); err != nil {
		return nil, err
	}
	// KRALH and KRANH share rpm steps 10, 20, 30 with DWKRMSN. LDHIA shares
	// LDIATA's intake-temperature axis. 8-bit KFFLLDE, only on DC and CH, is
	// that coolant series on both axes. The gear bytes 1 through 6 are Y on
	// 22m and X on DQ.
	for _, g := range []scaleRow{
		{Name: "KRALH", Role: "x", Bits: 8, Table: true, Units: "Upm", Precision: 1},
		{Name: "KRANH", Role: "x", Bits: 8, Table: true, Units: "Upm", Precision: 1},
		{Name: "LDHIA", Role: "x", Bits: 8, Table: true, Units: "Grad C", Precision: 2},
		{Name: "KFFLLDE", Role: "x", Bits: 8, Table: true, Units: "Grad C", Precision: 2},
		{Name: "KFFLLDE", Role: "y", Bits: 8, Table: true, Units: "Grad C", Precision: 2},
		{Name: "KFDMDADP", Role: "x", Bits: 8, Factor: 1, Units: "-"},
		{Name: "KFDMDADP", Role: "y", Bits: 8, Factor: 1, Units: "-"},
		{Name: "KFDMDARO", Role: "x", Bits: 8, Factor: 1, Units: "-"},
		{Name: "KFDMDARO", Role: "y", Bits: 8, Factor: 1, Units: "-"},
	} {
		if !rowTaken(rows, scaleKey{g.Name, g.Role, g.Bits}) {
			rows = append(rows, g)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		if roleRank(rows[i].Role) != roleRank(rows[j].Role) {
			return roleRank(rows[i].Role) < roleRank(rows[j].Role)
		}
		return rows[i].Bits < rows[j].Bits
	})
	return formatUnits(rows)
}

func roleRank(r string) int {
	switch r {
	case "z":
		return 0
	case "x":
		return 1
	default:
		return 2
	}
}

func objName(o *model.Object) string {
	if o == nil {
		return ""
	}
	if o.Key != "" {
		return o.Key
	}
	return strings.TrimSpace(o.ID)
}

func addAxis(by map[scaleKey][]sample, name, role, file string, a *model.Axis) {
	if a == nil || a.Data == nil {
		return
	}
	addSample(by, scaleKey{name, role, a.Data.Bits}, file, a.Value)
}

func addSample(by map[scaleKey][]sample, k scaleKey, file string, v model.Value) {
	if k.bits == 0 {
		return
	}
	c := v.Conversion
	s := sample{file, c.Factor, c.Offset, c.Reciprocal, v.Units, v.Precision}
	if s.factor == 0 || rawSample(s) {
		return
	}
	for _, old := range by[k] {
		if old == s {
			return
		}
	}
	by[k] = append(by[k], s)
}

func rawSample(s sample) bool {
	return closeF(s.factor, 1) && closeF(s.offset, 0) && !s.recip && s.units == ""
}

// rpmLoadMajority picks a scale when the defs only disagree by swapping the
// rpm axis with the other one. The 8-bit rpm scale is factor 40. The 16-bit
// rpm scale is factor 0.25. The larger set wins. KFMIZUOF is an even split;
// its rpm axis is X, matching the breakpoints.
func rpmLoadMajority(k scaleKey, ss []sample) (scaleRow, bool) {
	var rpm, other []sample
	for _, s := range ss {
		if s.offset != 0 || s.recip {
			return scaleRow{}, false
		}
		switch {
		case rpmScale(k.bits, s):
			rpm = append(rpm, s)
		case otherScale(k, s):
			other = append(other, s)
		default:
			return scaleRow{}, false
		}
	}
	if len(rpm) == 0 || len(other) == 0 {
		return scaleRow{}, false
	}
	if len(rpm) == len(other) {
		if k.name != "KFMIZUOF" {
			return scaleRow{}, false
		}
		if k.role == "x" {
			return majorityRow(k, rpm), true
		}
		return majorityRow(k, other), true
	}
	use := rpm
	if len(other) > len(rpm) {
		use = other
	}
	return majorityRow(k, use), true
}

// rpmPair is the 16-bit maps whose defs swap a percent axis with the
// factor 0.25 rpm axis. The published breakpoints follow the larger set.
var rpmPair = map[string]bool{
	"KFZW": true, "KFZWOP": true, "KFZWOP2": true, "KFMIOP": true, "KFMIRL": true,
}

func rpmScale(bits int, s sample) bool {
	if bits == 8 {
		return s.units == "Upm" && closeF(s.factor, 40)
	}
	return bits == 16 && s.units == "U/min" && closeF(s.factor, 0.25)
}

func otherScale(k scaleKey, s sample) bool {
	if s.units != "%" {
		return false
	}
	switch {
	case k.bits == 8 && (k.name == "FZWWLRLN" || k.name == "KFMI_UM"):
		return closeF(s.factor, 0.75)
	case k.bits == 8 && k.name == "KFMIZUOF":
		return closeF(s.factor, 0.390625)
	case k.bits == 16 && rpmPair[k.name]:
		return closeF(s.factor, 0.0234375) || closeF(s.factor, 0.00152587890625)
	}
	return false
}

func majorityRow(k scaleKey, use []sample) scaleRow {
	return scaleRow{
		Name: k.name, Role: k.role, Bits: k.bits,
		Factor: modeFactor(use), Units: use[0].units, Precision: precOf(use),
	}
}

func majorityUnits(ss []sample) (string, bool) {
	counts := map[string]int{}
	for _, s := range ss {
		if s.units != "" {
			counts[s.units]++
		}
	}
	best, n := "", 0
	for u, c := range counts {
		if c > n {
			best, n = u, c
		}
	}
	rest := 0
	for u, c := range counts {
		if u == best {
			continue
		}
		if !parenthetical(best, u) {
			return "", false
		}
		rest += c
	}
	if best != "" && n > rest {
		return best, true
	}
	return "", false
}

func parenthetical(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(b, a) && strings.HasPrefix(strings.TrimPrefix(b, a), " (")
}

// agree is the row for these samples. Samples cluster by factor, offset,
// and unit. The largest cluster is stored. A tie is disagreed so the rpm
// and load swap can still break it. A clock split stays a clock row.
func agree(k scaleKey, ss []sample, names Names, ticks []float64) (scaleRow, bool, bool) {
	if len(ss) == 0 {
		return scaleRow{}, false, false
	}
	var clusters [][]sample
	for _, s := range ss {
		placed := false
		for i, c := range clusters {
			if sameScale(c[0], s) {
				clusters[i] = append(c, s)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, []sample{s})
		}
	}
	kind := ""
	switch {
	case names.Clock.Injection[k.name]:
		kind = "injection"
	case k.name == names.Clock.KRKTE:
		kind = "krkte"
	}
	row := scaleRow{Name: k.name, Role: k.role, Bits: k.bits}
	use := ss
	if kind != "" {
		if clk := clockSamples(ss, kind, ticks, k.bits); len(clk) > len(ss)/2 && sameOffset(clk) {
			use = clk
			row.Clock = kind
		}
	}
	if row.Clock == "" {
		if len(clusters) > 1 {
			if use = largest(clusters); use == nil {
				return scaleRow{}, false, true
			}
		} else {
			use = clusters[0]
		}
		row.Factor = modeFactor(use)
	}
	row.Offset, row.Reciprocal = modeOffset(use), use[0].recip
	u, okU := unitsOf(use)
	if !okU {
		if u, okU = majorityUnits(use); !okU {
			return scaleRow{}, false, true
		}
	}
	row.Units, row.Precision = foldUnit(k.bits, u), precOf(use)
	return row, true, false
}

func sameScale(a, b sample) bool {
	return a.recip == b.recip && closeF(a.factor, b.factor) && closeF(a.offset, b.offset) && unitFold(a.units) == unitFold(b.units)
}

// largest is the biggest cluster when it is strictly bigger than the next.
func largest(clusters [][]sample) []sample {
	sort.Slice(clusters, func(i, j int) bool {
		if len(clusters[i]) != len(clusters[j]) {
			return len(clusters[i]) > len(clusters[j])
		}
		if clusters[i][0].factor != clusters[j][0].factor {
			return clusters[i][0].factor < clusters[j][0].factor
		}
		return clusters[i][0].units < clusters[j][0].units
	})
	if len(clusters) > 1 && len(clusters[0]) == len(clusters[1]) {
		return nil
	}
	return clusters[0]
}

func unitFold(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// foldUnit keeps a spelling the conversions table has, so hPA is hPa.
func foldUnit(bits int, u string) string {
	if u == "" {
		return ""
	}
	if _, ok := unitScale(bits, u); ok {
		return u
	}
	for _, alt := range []string{"hPa", "Upm", "U/min", "rpm", "Grad C", "grad C", "°C", "Grad KW", "grad KW", "%", "-", "ms", "s", "V"} {
		if unitFold(alt) == unitFold(u) {
			if _, ok := unitScale(bits, alt); ok {
				return alt
			}
		}
	}
	return u
}

func clockSamples(ss []sample, kind string, ticks []float64, bits int) []sample {
	var out []sample
	for _, s := range ss {
		if isClock(s.factor, kind, ticks, bits) {
			out = append(out, s)
		}
	}
	return out
}

func isClock(f float64, kind string, ticks []float64, bits int) bool {
	for _, tick := range ticks {
		if closeF(f, clockTickFactor(kind, tick, bits)) {
			return true
		}
	}
	return false
}

func unitsOf(ss []sample) (string, bool) {
	u := ""
	for _, s := range ss {
		if s.units == "" {
			continue
		}
		if u == "" {
			u = s.units
			continue
		}
		if unitFold(u) != unitFold(s.units) {
			return "", false
		}
	}
	return u, true
}

func precOf(ss []sample) int {
	p := make([]int, len(ss))
	for i, s := range ss {
		p[i] = s.prec
	}
	return modePrec(p)
}

func sameOffset(ss []sample) bool {
	for _, s := range ss[1:] {
		if s.recip != ss[0].recip || !closeF(s.offset, ss[0].offset) {
			return false
		}
	}
	return true
}

func modeFactor(ss []sample) float64 {
	counts := map[float64]int{}
	for _, s := range ss {
		counts[s.factor]++
	}
	return mode(counts)
}

func modeOffset(ss []sample) float64 {
	counts := map[float64]int{}
	for _, s := range ss {
		counts[s.offset]++
	}
	return mode(counts)
}

func mode(counts map[float64]int) float64 {
	type pair struct {
		f float64
		n int
	}
	ps := make([]pair, 0, len(counts))
	for f, n := range counts {
		ps = append(ps, pair{f, n})
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].n != ps[j].n {
			return ps[i].n > ps[j].n
		}
		return ps[i].f < ps[j].f
	})
	return ps[0].f
}

// closeF treats a factor rounded to 6 decimal places as the same value
// (0.039063 and 0.0390625, 0.000111 and 1/9000).
func closeF(a, b float64) bool {
	if a == b {
		return true
	}
	return math.Round(a*1e6) == math.Round(b*1e6)
}

var (
	unitOnce sync.Once
	unitTab  convTable
	unitErr  error
)

// useTable drops a factor the conversions row for this unit and width
// already has. The name keeps the unit. A clock or a reciprocal stays.
func useTable(row *scaleRow) {
	if row.Table || row.Clock != "" || row.Reciprocal || row.Units == "" {
		return
	}
	tab, ok := unitScale(row.Bits, row.Units)
	if !ok || !closeF(tab.Conversion.Factor, row.Factor) || !closeF(tab.Conversion.Offset, row.Offset) {
		return
	}
	row.Table = true
	row.Factor = 0
	row.Offset = 0
}

// unitScale is the conversions row for this width and unit. Offset is the
// map form, factor*raw + offset, so the units.yaml offset is negated.
func unitScale(bits int, unit string) (model.Value, bool) {
	if bits == 0 || bits%8 != 0 || unit == "" {
		return model.Value{}, false
	}
	unitOnce.Do(func() { unitTab, unitErr = conversionTable("", "", "") })
	if unitErr != nil {
		return model.Value{}, false
	}
	c, ok := unitTab.byScale[convKey{size: bits / 8, unit: unit}]
	if !ok || !c.set.factor {
		return model.Value{}, false
	}
	v := model.Value{Units: unit, Conversion: model.Conversion{Factor: c.factor, Offset: -c.offset}}
	if c.set.min && c.set.max {
		lo, hi := c.min, c.max
		v.Min, v.Max = &lo, &hi
	}
	return v, true
}

// nameTiers is every name in testdata/parity/names/tuner.yaml.
func nameTiers() map[string]bool {
	root, err := ecucorpus.ModuleRoot()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(root, "testdata", "parity", "names", "tuner.yaml"))
	if err != nil {
		return nil
	}
	var doc struct {
		Tiers map[string]map[string][]string `yaml:"tiers"`
	}
	if yaml.Unmarshal(b, &doc) != nil {
		return nil
	}
	out := map[string]bool{}
	for _, axes := range doc.Tiers {
		for _, names := range axes {
			for _, name := range names {
				out[name] = true
			}
		}
	}
	return out
}

// fillHand stores a tuner unit from the verified hand definition when the
// DAMOS and A2L defs left that role empty. A factor the conversions table
// already has becomes a table row. M's own factor is kept otherwise.
func fillHand(dir string, rows *[]scaleRow, want map[string]bool) error {
	if len(want) == 0 {
		return nil
	}
	mb, err := os.ReadFile(filepath.Join(dir, "defs", "8D0907551M-0002.json"))
	if err != nil {
		return nil
	}
	var pack model.Model
	if json.Unmarshal(mb, &pack) != nil || pack.Provenance == nil || !pack.Provenance.Verified {
		return nil
	}
	byName := map[string][]*model.Object{}
	for _, o := range pack.Objects {
		if name := objName(o); want[name] {
			byName[name] = append(byName[name], o)
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, role := range []string{"z", "x", "y"} {
			v, bits, ok := handRole(byName[name], role)
			if !ok {
				continue
			}
			addHand(rows, scaleKey{name, role, bits}, v)
		}
	}
	return nil
}

func handRole(objs []*model.Object, role string) (model.Value, int, bool) {
	var first model.Value
	var firstBits int
	var tab model.Value
	var tabBits int
	seen, tabSeen := false, false
	for _, o := range objs {
		v, n, ok := roleValue(o, role)
		if !ok || rawSample(sample{factor: v.Conversion.Factor, offset: v.Conversion.Offset, units: v.Units}) {
			continue
		}
		if !seen {
			first, firstBits, seen = v, n, true
		}
		t, okT := unitScale(n, v.Units)
		if okT && closeF(t.Conversion.Factor, v.Conversion.Factor) && closeF(t.Conversion.Offset, v.Conversion.Offset) {
			tab, tabBits, tabSeen = v, n, true
		}
	}
	if tabSeen {
		return tab, tabBits, true
	}
	return first, firstBits, seen
}

func roleValue(o *model.Object, role string) (model.Value, int, bool) {
	if role == "z" {
		if o.Data.Bits == 0 {
			return model.Value{}, 0, false
		}
		return o.Value, o.Data.Bits, true
	}
	ax := o.X
	if role == "y" {
		ax = o.Y
	}
	if ax == nil || ax.Source != "image" || ax.Data == nil || ax.Data.Bits == 0 {
		return model.Value{}, 0, false
	}
	return ax.Value, ax.Data.Bits, true
}

func addHand(rows *[]scaleRow, k scaleKey, v model.Value) {
	if rowTaken(*rows, k) {
		return
	}
	row := scaleRow{
		Name: k.name, Role: k.role, Bits: k.bits,
		Factor: v.Conversion.Factor, Offset: v.Conversion.Offset,
		Reciprocal: v.Conversion.Reciprocal, Units: v.Units, Precision: v.Precision,
	}
	useTable(&row)
	*rows = append(*rows, row)
}

func rowTaken(rows []scaleRow, k scaleKey) bool {
	for _, r := range rows {
		if r.Name == k.name && r.Role == k.role && r.Bits == k.bits {
			return true
		}
	}
	return false
}

type unitKey struct {
	bits           int
	units, clock   string
	factor, offset int64
	recip          bool
}

type unitBucket struct {
	z, x, y        []string
	prec           []int
	factor, offset []float64
	recip          bool
}

func micro(f float64) int64 {
	return int64(math.Round(f * 1e6))
}

type scaleGroup struct {
	bits           int
	units, clock   string
	factor, offset float64
	recip          bool
	prec           int
	z, x, y        []string
}

// heldMaps is a maps row the cluster did not store. A name the cluster
// stores is left out, so the next rebuild keeps only the rows still written
// by hand.
func heldMaps(hands []handUnit, rows []scaleRow) []scaleRow {
	have := map[scaleKey]bool{}
	for _, r := range rows {
		have[scaleKey{r.Name, r.Role, r.Bits}] = true
	}
	var out []scaleRow
	for _, h := range hands {
		if !h.maps {
			continue
		}
		bits := handBits(h)
		add := func(role string, names []string) {
			for _, n := range names {
				if have[scaleKey{n, role, bits}] {
					continue
				}
				row := scaleRow{
					Name: n, Role: role, Bits: bits, Units: h.unit,
					Factor: h.factor, Precision: h.precision,
					Reciprocal: h.recip, Clock: h.clock,
				}
				if h.offsetSet {
					row.Offset = -h.offset
				}
				out = append(out, row)
			}
		}
		add("z", h.z)
		add("x", h.x)
		add("y", h.y)
	}
	return out
}

func formatUnits(rows []scaleRow) ([]byte, error) {
	root, err := ecucorpus.ModuleRoot()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(root, "config", UnitsFile))
	if err != nil {
		return nil, err
	}
	head, hands, err := parseUnits(b)
	if err != nil {
		return nil, err
	}
	// A maps row the cluster did not store stays in the file. A name the
	// cluster stores is left to that cluster.
	rows = append(rows, heldMaps(hands, rows)...)
	var keep []handUnit
	for _, h := range hands {
		if h.maps {
			continue
		}
		h.z, h.x, h.y = nil, nil, nil
		h.precision = 0
		keep = append(keep, h)
	}
	for _, g := range scaleGroups(rows) {
		if i := matchHand(keep, g); i >= 0 {
			keep[i].z = uniq(append(keep[i].z, g.z...))
			keep[i].x = uniq(append(keep[i].x, g.x...))
			keep[i].y = uniq(append(keep[i].y, g.y...))
			if keep[i].precision == 0 {
				keep[i].precision = g.prec
			}
			continue
		}
		extra := handUnit{
			bits: g.bits, bitsSet: true, unit: g.units,
			recip: g.recip, clock: g.clock, precision: g.prec, maps: true,
			z: uniq(g.z), x: uniq(g.x), y: uniq(g.y),
		}
		if g.factor != 0 {
			extra.factor = g.factor
			extra.factorSet = true
			extra.factorText = ratioOf(g.factor)
		}
		if g.offset != 0 {
			extra.offset = -g.offset
			extra.offsetSet = true
		}
		keep = append(keep, extra)
	}
	return []byte(head + emitHands(keep)), nil
}

func scaleGroups(rows []scaleRow) []scaleGroup {
	g := map[unitKey]*unitBucket{}
	for _, r := range rows {
		k := unitKey{r.Bits, r.Units, r.Clock, micro(r.Factor), micro(r.Offset), r.Reciprocal}
		u := g[k]
		if u == nil {
			u = &unitBucket{recip: r.Reciprocal}
			g[k] = u
		}
		u.factor = append(u.factor, r.Factor)
		u.offset = append(u.offset, r.Offset)
		switch r.Role {
		case "x":
			u.x = append(u.x, r.Name)
		case "y":
			u.y = append(u.y, r.Name)
		default:
			u.z = append(u.z, r.Name)
		}
		u.prec = append(u.prec, r.Precision)
	}
	keys := make([]unitKey, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.bits != b.bits {
			return a.bits < b.bits
		}
		if a.units != b.units {
			return a.units < b.units
		}
		if a.factor != b.factor {
			return a.factor < b.factor
		}
		if a.offset != b.offset {
			return a.offset < b.offset
		}
		if a.clock != b.clock {
			return a.clock < b.clock
		}
		return !a.recip && b.recip
	})
	out := make([]scaleGroup, 0, len(keys))
	for _, k := range keys {
		u := g[k]
		out = append(out, scaleGroup{
			bits: k.bits, units: k.units, clock: k.clock,
			factor: mode(countsOf(u.factor)), offset: mode(countsOf(u.offset)),
			recip: u.recip, prec: modePrec(u.prec), z: u.z, x: u.x, y: u.y,
		})
	}
	return out
}

func countsOf(v []float64) map[float64]int {
	n := map[float64]int{}
	for _, f := range v {
		n[f]++
	}
	return n
}

func modePrec(v []int) int {
	n := map[int]int{}
	best, most := 0, 0
	for _, p := range v {
		n[p]++
		if n[p] > most || (n[p] == most && p > best) {
			best, most = p, n[p]
		}
	}
	return best
}

type handUnit struct {
	bits, precision    int
	bitsSet, offsetSet bool
	factorSet, signed  bool
	recip, maps        bool
	unit, name         string
	factorText, clock  string
	factor, offset     float64
	min, max           *float64
	z, x, y            []string
}

func parseUnits(b []byte) (string, []handUnit, error) {
	s := string(b)
	i := strings.Index(s, "\nunits:\n")
	if i < 0 {
		return "", nil, fmt.Errorf("%s: units list missing", UnitsFile)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return "", nil, err
	}
	if len(doc.Content) == 0 {
		return "", nil, fmt.Errorf("%s: empty", UnitsFile)
	}
	fields, err := needle.MappingFields(doc.Content[0])
	if err != nil {
		return "", nil, err
	}
	n := fields["units"]
	if n == nil || n.Kind != yaml.SequenceNode {
		return "", nil, fmt.Errorf("%s: units list missing", UnitsFile)
	}
	var rows []handUnit
	for _, item := range n.Content {
		h, err := parseHand(item)
		if err != nil {
			return "", nil, err
		}
		rows = append(rows, h)
	}
	return s[:i+1], rows, nil
}

func parseHand(n *yaml.Node) (handUnit, error) {
	fields, err := needle.MappingFields(n)
	if err != nil {
		return handUnit{}, err
	}
	var h handUnit
	if node, ok := fields["name"]; ok {
		h.name, err = scalarString(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["unit"]; ok {
		h.unit, err = scalarString(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["bits"]; ok {
		h.bits, err = scalarInt(node)
		if err != nil {
			return handUnit{}, err
		}
		h.bitsSet = true
	} else if node, ok := fields["size"]; ok {
		n, err := scalarInt(node)
		if err != nil {
			return handUnit{}, err
		}
		h.bits, h.bitsSet = n*8, true
	}
	if node, ok := fields["factor"]; ok {
		h.factorText = node.Value
		h.factor, err = scalarFloat(node)
		if err != nil {
			return handUnit{}, err
		}
		h.factorSet = true
	}
	if node, ok := fields["offset"]; ok {
		h.offset, err = scalarFloat(node)
		if err != nil {
			return handUnit{}, err
		}
		h.offsetSet = true
	}
	if node, ok := fields["signed"]; ok {
		h.signed, err = scalarBool(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["reciprocal"]; ok {
		h.recip, err = scalarBool(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["maps"]; ok {
		h.maps, err = scalarBool(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["clock"]; ok {
		h.clock, err = scalarString(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["precision"]; ok {
		h.precision, err = scalarInt(node)
		if err != nil {
			return handUnit{}, err
		}
	}
	if node, ok := fields["min"]; ok {
		v, err := scalarFloat(node)
		if err != nil {
			return handUnit{}, err
		}
		h.min = &v
	}
	if node, ok := fields["max"]; ok {
		v, err := scalarFloat(node)
		if err != nil {
			return handUnit{}, err
		}
		h.max = &v
	}
	for _, role := range []struct {
		k string
		d *[]string
	}{{"z", &h.z}, {"x", &h.x}, {"y", &h.y}} {
		node, ok := fields[role.k]
		if !ok {
			continue
		}
		if err := node.Decode(role.d); err != nil {
			return handUnit{}, err
		}
	}
	return h, nil
}

func handBits(h handUnit) int {
	if h.bitsSet {
		return h.bits
	}
	return DefaultSize * 8
}

func matchHand(hands []handUnit, g scaleGroup) int {
	for i, h := range hands {
		if h.maps || h.unit != g.units || handBits(h) != g.bits {
			continue
		}
		if g.clock != "" {
			if h.clock == g.clock {
				return i
			}
			continue
		}
		if g.factor == 0 && !g.recip {
			if h.name == "" && h.clock == "" && !h.recip {
				return i
			}
			continue
		}
		off := 0.0
		if h.offsetSet {
			off = -h.offset
		}
		if h.clock == "" && h.recip == g.recip && h.factorSet && closeF(h.factor, g.factor) && closeF(off, g.offset) {
			return i
		}
	}
	return -1
}

func uniq(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	sort.Strings(v)
	out := v[:1]
	for _, s := range v[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

func emitHands(rows []handUnit) string {
	var b strings.Builder
	b.WriteString("units:\n")
	for _, h := range rows {
		b.WriteString(emitHand(h))
	}
	return b.String()
}

func emitHand(h handUnit) string {
	head := "- {"
	head += fmt.Sprintf("bits: %d, ", handBits(h))
	if h.name != "" {
		head += "name: " + h.name + ", "
	}
	head += "unit: " + strconv.Quote(h.unit)
	if h.factorText != "" {
		head += ", factor: " + h.factorText
	}
	if h.offsetSet {
		head += ", offset: " + numText(h.offset)
	}
	if h.signed {
		head += ", signed: true"
	}
	if h.recip {
		head += ", reciprocal: true"
	}
	if h.clock != "" {
		head += ", clock: " + h.clock
	}
	if h.min != nil {
		head += ", min: " + numText(*h.min)
	}
	if h.max != nil {
		head += ", max: " + numText(*h.max)
	}
	if h.precision != 0 {
		head += fmt.Sprintf(", precision: %d", h.precision)
	}
	if h.maps {
		head += ", maps: true"
	}
	type role struct {
		k string
		n []string
	}
	var lists []role
	for _, r := range []role{{"z", h.z}, {"x", h.x}, {"y", h.y}} {
		if len(r.n) == 0 {
			continue
		}
		lists = append(lists, r)
	}
	if len(lists) == 0 {
		return head + "}\n"
	}
	parts := make([]string, len(lists))
	for i, r := range lists {
		parts[i] = r.k + ": [" + strings.Join(r.n, ", ") + "]"
	}
	one := head + ", " + strings.Join(parts, ", ") + "}\n"
	if len(one) <= 100 {
		return one
	}
	var b strings.Builder
	b.WriteString(head)
	for _, r := range lists {
		b.WriteString(", " + r.k + ": [\n")
		col := 0
		for i, n := range r.n {
			piece := n
			if i+1 < len(r.n) {
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
		b.WriteString("]")
	}
	b.WriteString("}\n")
	return b.String()
}

func numText(f float64) string {
	if math.Abs(f-math.Round(f)) < 1e-9 && math.Abs(f) < 1e6 {
		return strconv.FormatFloat(math.Round(f), 'f', 0, 64)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func ratioOf(f float64) string {
	if f == 0 {
		return ""
	}
	sign := ""
	if f < 0 {
		sign = "-"
		f = -f
	}
	if math.Abs(f-math.Round(f)) < 1e-9 && f < 1e6 {
		return sign + strconv.FormatFloat(math.Round(f), 'f', 0, 64)
	}
	denoms := []struct {
		d float64
		s string
	}{
		{2, "2"}, {4, "4"}, {8, "8"}, {16, "16"}, {36, "36"},
		{100, "100"}, {128, "128"}, {250, "250"}, {255, "255"},
		{256, "256"}, {375, "375"}, {312.5, "312.5"},
		{0x10000, "0x10000"},
	}
	for _, d := range denoms {
		n := f * d.d
		r := math.Round(n)
		if math.Abs(n-r) < 1e-4 && r != 0 && math.Abs(r) < 1e6 {
			return sign + strconv.FormatFloat(r, 'f', 0, 64) + "/" + d.s
		}
	}
	return sign + strconv.FormatFloat(f, 'f', -1, 64)
}

func loadScales() {
	scales = map[scaleKey]scaleRow{}
	openScales = map[scaleKey]bool{}
	b, err := embedded.ReadFile(UnitsFile)
	if err != nil {
		return
	}
	_, hands, err := parseUnits(b)
	if err != nil {
		return
	}
	for _, h := range hands {
		bits := handBits(h)
		off := 0.0
		if h.offsetSet {
			off = -h.offset
		}
		base := scaleRow{
			Bits: bits, Factor: h.factor, Offset: off, Reciprocal: h.recip,
			Units: h.unit, Precision: h.precision, Clock: h.clock,
			lo: h.min, hi: h.max,
		}
		put := func(role string, names []string) {
			for _, n := range names {
				row := base
				row.Name, row.Role = n, role
				scales[scaleKey{n, role, bits}] = row
			}
		}
		put("z", h.z)
		put("x", h.x)
		put("y", h.y)
	}
	// Empty path uses the file beside the executable, then the embed. The
	// short name is a cwd-relative read and misses the clock when the
	// process is not started in config/.
	names, err := LoadNames("")
	if err != nil {
		return
	}
	clockMHz = names.Clock.DefaultMHz
	clockTick = names.Clock.Factors
	inheritSets()
}

// inheritSets copies each plain name's rows onto a form that has none for
// that role and width. The form's own row wins.
func inheritSets() {
	sets, err := Sets()
	if err != nil {
		return
	}
	for _, s := range sets {
		if len(s.Names) < 2 {
			continue
		}
		var rows []scaleRow
		for k, row := range scales {
			if k.name == s.Names[0] {
				rows = append(rows, row)
			}
		}
		for _, form := range s.Names[1:] {
			for _, row := range rows {
				k := scaleKey{form, row.Role, row.Bits}
				if _, ok := scales[k]; ok || openScales[k] {
					continue
				}
				row.Name = form
				scales[k] = row
			}
		}
	}
}

func clockFactor(kind string, mhz, bits int) float64 {
	if mhz == 0 {
		mhz = clockMHz
	}
	tick := clockTick[mhz]
	if tick == 0 {
		return 0
	}
	return clockTickFactor(kind, tick, bits)
}

// clockTickFactor is the milliseconds per raw count. KRKTE is the injection
// tick divided by 24. An 8-bit KRKTE is that word divided by 16, so the
// factor is 16 times larger and the displayed ms/% stays the same.
func clockTickFactor(kind string, tick float64, bits int) float64 {
	if kind != "krkte" {
		return tick
	}
	f := tick / 24
	if bits == 8 {
		f *= 16
	}
	return f
}

// ApplyScales fills a raw value (factor 1, no offset, no units) from
// Conversion. A value that already has a factor or units stays. An ordinal
// axis has no stored width, so the lookup uses the cell width. It returns
// how many values changed.
func ApplyScales(m *model.Model) int {
	n := 0
	for _, o := range m.Objects {
		name := objName(o)
		if name == "" {
			continue
		}
		if fillRaw(&o.Value, name, "z", o.Data.Bits) {
			n++
		}
		if fillAxis(name, "x", o.X, o.Data.Bits) {
			n++
		}
		if fillAxis(name, "y", o.Y, o.Data.Bits) {
			n++
		}
	}
	return n
}

func fillAxis(name, role string, a *model.Axis, bits int) bool {
	if a == nil {
		return false
	}
	if a.Data != nil && a.Data.Bits != 0 {
		bits = a.Data.Bits
	} else if a.Source != "ordinal" {
		return false
	}
	return fillRaw(&a.Value, name, role, bits)
}

func fillRaw(v *model.Value, name, role string, bits int) bool {
	c := v.Conversion
	if !(c.Factor == 1 && c.Offset == 0 && !c.Reciprocal && v.Units == "") {
		return false
	}
	got, ok := Conversion(name, role, bits, 0)
	if !ok {
		return false
	}
	*v = got
	return true
}

// FillAuto writes the agreed scale onto raw values in origin-auto defs.
// Address, shape, comment, origin, and a value that already has a factor
// or units stay. It returns how many values changed.
func FillAuto(dir string) (int, error) {
	ents, err := os.ReadDir(filepath.Join(dir, "defs"))
	if err != nil {
		return 0, err
	}
	total := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, "defs", e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		var m model.Model
		if err := canon.Unmarshal(b, &m); err != nil {
			return 0, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if m.Provenance == nil || m.Provenance.Origin != "auto" {
			continue
		}
		n := ApplyScales(&m)
		if n == 0 {
			continue
		}
		out, err := canon.MarshalStamped(&m, "me7-logger scales")
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}
