// Package generate turns a flash image and the config YAML into .ecu records
// and calibration maps. It does not open a serial port. Map addresses come
// from calls to the interpolation needles. Those calls do not carry the Bosch name.
package generate

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"

	"me7-logger/config"
	"me7-logger/connect"
	"me7-logger/ecu"
	"me7-logger/ident"
	"me7-logger/interp"
	"me7-logger/mapfile"
	"me7-logger/opcode"
	"me7-logger/record"
)

// Options selects the image and the definition files in this repo.
type Options struct {
	Image     []byte
	ImageName string
	CorePath  string
	NamesPath string
	MeasPath  string
	MapPath   string
	AliasPath string
	// Clock is 20, 24, 32, or 40. 0 uses the default in config/names.yaml.
	Clock int
	// Scale is auto, on, or off.
	Scale string
	// Connect overrides the value chosen from the slow-init needle.
	Connect string
	// UserDir is an optional directory of overlay YAML. Empty means no overlay.
	UserDir string
}

// Result is the generated definition. File is never nil.
// Maps are written to XDF and are not .ecu rows.
type Result struct {
	File      *ecu.File
	Maps      []record.Map
	ScaleNote string
	MapNote   string
	StubNote  string
}

// Generate runs the needle consumer and the opcode walk.
// Callers write Result.File and Result.Maps.
func Generate(opt Options) (*Result, error) {
	names, err := config.LoadNames(opt.NamesPath)
	if err != nil {
		return nil, err
	}
	measures, err := config.LoadMeasures(opt.MeasPath, opt.NamesPath, opt.UserDir)
	if err != nil {
		return nil, err
	}
	ns, err := config.LoadNeedles(opt.CorePath, opt.UserDir)
	if err != nil {
		return nil, err
	}
	scales, err := config.LoadScales(opt.MeasPath, opt.NamesPath, opt.UserDir)
	if err != nil {
		return nil, err
	}
	tab, mapNote, err := loadMap(opt.MapPath, scales)
	if err != nil {
		return nil, err
	}
	aliases, err := loadAlias(opt.AliasPath)
	if err != nil {
		return nil, err
	}
	calls, err := config.LoadMaps("")
	if err != nil {
		return nil, err
	}
	dpp, _, dppOK := opcode.FindDPP(opt.Image)
	if !dppOK {
		dpp = opcode.StandardDPP
	}
	res := &Result{
		MapNote: mapNote,
		File: &ecu.File{
			Version:   "1.20",
			LogSpeed:  56000,
			ImageName: opt.ImageName,
			ImageSize: len(opt.Image),
			MapName:   names.MapName,
			ID:        ident.Find(opt.Image),
		},
	}
	emitted := map[seenRow]struct{}{}
	policy := connect.Policy{
		Key1: names.Connect.Key1, Key2: names.Connect.Key2,
		Prefer: names.Connect.Prefer, Fallback: names.Connect.Fallback,
		Fast: names.Connect.Fast,
	}
	located := map[string]config.Measure{}
	for _, m := range measures {
		if !m.Stub {
			located[m.Name] = m
		}
	}
	var cases []opcode.Case
	for _, n := range ns {
		labels := n.Labels(opt.Image)
		if n.Name == names.Connect.SlowNeedle && len(labels) == 1 {
			if entries := connect.ParseTable(opt.Image, labels[0]); entries != nil {
				if c, ok := connect.Choose(entries, policy); ok {
					res.File.Connect = c
				}
			}
		}
		if n.Name == names.Connect.FastNeedle && len(labels) == 1 {
			off := labels[0]
			if off >= 0 && off < len(opt.Image) {
				if b, ok := connect.FastTarget(opt.Image[off], names.Connect.Fast); ok {
					res.File.Fast = b
				}
			}
		}
		for _, label := range labels {
			if !opcode.IsSelector(opt.Image, label, names.Selector) {
				continue
			}
			entries := opcode.SelectorTable(opt.Image, label, opcode.FlashBase, dpp, names.Selector)
			if len(entries) == 0 {
				entries = []opcode.SelectorEntry{{Off: label}}
			}
			for _, e := range entries {
				for _, c := range opcode.WalkSelector(opt.Image, e.Off, opcode.FlashBase, dpp, e.Index, names.Selector, names.SelectorEnd, e.Finish) {
					cases = append(cases, c)
				}
			}
		}
	}
	// A needle whose mem operand sits inside a case takes that case's result
	// type, bitmask, and address. The name and the scale stay on the needle.
	// The catalog still names every case no needle joined.
	joined := map[int]struct{}{}
	for _, n := range ns {
		m, ok := located[n.Name]
		if !ok {
			continue
		}
		labels := n.Labels(opt.Image)
		joinedHere := false
		for _, label := range labels {
			idx, c, ok := joinCase(cases, joined, opt.Image, label, dpp)
			if !ok {
				continue
			}
			joined[idx] = struct{}{}
			joinedHere = true
			addItem(&res.File.Items, emitted, joinedItem(m, aliases, c))
		}
		if joinedHere || len(labels) != 1 || labels[0]+2 > len(opt.Image) {
			continue
		}
		mem := binary.LittleEndian.Uint16(opt.Image[labels[0] : labels[0]+2])
		addr := opcode.Physical(dpp, mem, -1)
		sz := m.Size
		if sz == 0 {
			sz = config.DefaultSize
		}
		addItem(&res.File.Items, emitted, ramItem(m.Name, pickAlias(m.Alias, aliases, m.Name), addr, sz, m.Bitmask, m.Unit, m.Comment, m.Signed, m.Inverse, m.A, m.B, false, 0))
	}
	for i, c := range cases {
		if _, ok := joined[i]; ok {
			continue
		}
		if c.Name != "" {
			if it, ok := namedItem(tab, aliases, c); ok {
				addItem(&res.File.Items, emitted, it)
			}
			continue
		}
		if c.Size == 0 {
			continue
		}
		for _, v := range catalogRows(tab, c) {
			sz := v.Size
			if sz == 0 {
				sz = c.Size
			}
			if sz == 0 {
				sz = 1
			}
			mask := c.Bitmask
			if !c.Keyed && mask == 0 {
				mask = v.Bitmask
			}
			addItem(&res.File.Items, emitted, ramItem(v.Name, aliases[v.Name], c.Addr, sz, mask, v.Unit, v.Comment, v.Signed, v.Inverse, v.A, v.B, c.Guessed || c.Addr == 0, c.ResultType))
		}
	}
	res.Maps = interp.Locate(opt.Image, ns, dpp, calls)
	if n := len(res.Maps); n > 0 {
		note := fmt.Sprintf("%d calibration maps located from interpolation calls", n)
		if n == 1 {
			note = "1 calibration map located from interpolation calls"
		}
		if res.MapNote != "" {
			res.MapNote += "; " + note
		} else {
			res.MapNote = note
		}
	}
	res.StubNote = stubNote(measures)
	if opt.Connect != "" {
		res.File.Connect = opt.Connect
	}
	clock := opt.Clock
	if clock == 0 {
		clock = names.Clock.DefaultMHz
	}
	factor := names.Clock.Factors[clock]
	ecu.ApplyClock(res.File.Items, clock, names.Clock.Injection, names.Clock.KRKTE, factor)
	mode, err := ecu.ParseScale(opt.Scale)
	if err != nil {
		return nil, err
	}
	apply, note := ecu.Decide5120(mode, nil, names.Scale.Ambient, names.Scale.Low, names.Scale.High)
	res.ScaleNote = note
	if apply {
		ecu.ScaleMbar(res.File.Items, names.Scale.Units)
	}
	sort.Slice(res.File.Items, func(i, j int) bool {
		return res.File.Items[i].Name < res.File.Items[j].Name
	})
	return res, nil
}

// joinCase reports the case whose address is the mem operand at label and
// whose body contains that label. A hit outside every case is not a join.
func joinCase(cases []opcode.Case, joined map[int]struct{}, img []byte, label int, dpp [4]uint16) (int, opcode.Case, bool) {
	if label < 0 || label+2 > len(img) {
		return 0, opcode.Case{}, false
	}
	mem := binary.LittleEndian.Uint16(img[label : label+2])
	addr := opcode.Physical(dpp, mem, -1)
	if addr == 0 {
		return 0, opcode.Case{}, false
	}
	for i, c := range cases {
		if _, ok := joined[i]; ok {
			continue
		}
		if c.Addr != addr || c.Size == 0 || c.End <= c.Off {
			continue
		}
		if label < c.Off || label >= c.End {
			continue
		}
		return i, c, true
	}
	return 0, opcode.Case{}, false
}

type seenRow struct {
	name string
	addr uint32
}

func addItem(dst *[]record.Item, seen map[seenRow]struct{}, it record.Item) {
	key := seenRow{it.Name, it.Addr}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*dst = append(*dst, it)
}

func pickAlias(own string, aliases map[string]string, name string) string {
	if own != "" {
		return own
	}
	return aliases[name]
}

func ramItem(name, alias string, addr uint32, size int, mask uint16, unit, comment string, signed, inverse bool, a, b float64, guessed bool, rt int) record.Item {
	return record.Item{
		Name: name, Alias: alias, Addr: addr, Size: size, Bitmask: mask,
		Unit: unit, Comment: comment, Signed: signed, Inverse: inverse,
		A: a, B: b, Guessed: guessed, ResultType: rt,
	}
}

// joinedItem takes the name and scale from the measurement and the result
// type, bitmask, address, and width from the case.
func joinedItem(m config.Measure, aliases map[string]string, c opcode.Case) record.Item {
	sz := c.Size
	if sz == 0 {
		sz = m.Size
	}
	if sz == 0 {
		sz = config.DefaultSize
	}
	return ramItem(m.Name, pickAlias(m.Alias, aliases, m.Name), c.Addr, sz, c.Bitmask, m.Unit, m.Comment, m.Signed, m.Inverse, m.A, m.B, c.Guessed || c.Addr == 0, c.ResultType)
}

// namedItem is a variable the template names itself (rkaz_w, rkat_w).
// The scale comes from the catalog row of that name. The address does not.
func namedItem(tab mapfile.Table, aliases map[string]string, c opcode.Case) (record.Item, bool) {
	if c.Addr == 0 || c.Size == 0 {
		return record.Item{}, false
	}
	v, ok := varByName(tab, c.Name)
	sz := c.Size
	if ok && v.Size != 0 {
		sz = v.Size
	}
	it := ramItem(c.Name, aliases[c.Name], c.Addr, sz, c.Bitmask, "", "", false, false, 0, 0, false, c.ResultType)
	if ok {
		it.Unit = v.Unit
		it.Signed = v.Signed
		it.Inverse = v.Inverse
		it.A = v.A
		it.B = v.B
		it.Comment = v.Comment
	}
	return it, true
}

func varByName(tab mapfile.Table, name string) (mapfile.Var, bool) {
	for _, m := range tab.ByRT {
		for _, v := range m {
			if v.Name == name {
				return v, true
			}
		}
	}
	return mapfile.Var{}, false
}

// catalogRows is the map entry for this case. A value row stands alone.
// A bit is emitted only when the case extracted that mask, or when the
// result type has a single bit row and the case named the bit. Other bit
// rows stay on their own result types. Mode 0x10 names the row by the bit
// index and does not fall through to the value row.
func catalogRows(tab mapfile.Table, c opcode.Case) []mapfile.Var {
	if c.Keyed {
		if v, ok := tab.Lookup(c.ResultType, c.Key); ok {
			return []mapfile.Var{v}
		}
		return nil
	}
	if c.Bitmask != 0 {
		if v, ok := tab.Lookup(c.ResultType, c.Bitmask); ok {
			return []mapfile.Var{v}
		}
	}
	rows := tab.Rows(c.ResultType)
	var bits []mapfile.Var
	for _, v := range rows {
		if v.Bitmask == 0 {
			return []mapfile.Var{v}
		}
		bits = append(bits, v)
	}
	if c.Bitmask != 0 && len(bits) == 1 {
		return bits
	}
	return nil
}

func loadMap(path string, scales map[mapfile.ScaleKey]mapfile.Scale) (mapfile.Table, string, error) {
	if path == "" || path == config.Path("ME7_MAP", config.MapDir) {
		tab, err := config.LoadCatalog("", scales)
		return tab, "", err
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mapfile.Table{ByRT: map[int]map[uint16]mapfile.Var{}}, "map file not found: " + path, nil
		}
		return mapfile.Table{}, "", err
	}
	if st.IsDir() {
		tab, err := config.LoadCatalog(path, scales)
		return tab, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return mapfile.Table{}, "", err
	}
	tab, err := mapfile.Parse(b, path, scales, nil)
	return tab, "", err
}

func loadAlias(path string) (map[string]string, error) {
	if path == "" || path == config.Path("ME7_ALIAS", config.AliasFile) {
		b, err := config.Read(path, config.AliasFile)
		if err != nil {
			return nil, err
		}
		return mapfile.ParseAliases(b)
	}
	a, err := mapfile.Aliases(path)
	if err == nil {
		return a, nil
	}
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	return nil, err
}

func stubNote(ms []config.Measure) string {
	n := 0
	for _, m := range ms {
		if m.Stub {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d measurement stubs not written; add a needle under config/user", n)
}

// WriteECU writes the characteristics file.
func WriteECU(path string, f *ecu.File) error {
	if f == nil {
		return fmt.Errorf("no ecu")
	}
	return os.WriteFile(path, f.Bytes(), 0o644)
}
