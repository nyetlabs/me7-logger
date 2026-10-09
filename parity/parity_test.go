package parity

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/needle"
	"go.nyet.org/me7-logger/record"
)

func TestMatchECU(t *testing.T) {
	got := []record.Item{{Name: "nmot", Addr: 0xF878, Size: 1, Bitmask: 0}}
	want := []record.Item{
		{Name: "nmot", Addr: 0xF878, Size: 1},
		{Name: "rl", Addr: 0x380100, Size: 2},
	}
	hit, n := matchECU(got, want)
	if hit != 1 || n != 2 {
		t.Fatalf("%d/%d", hit, n)
	}
}

func TestMatchMapsFileOffset(t *testing.T) {
	got := []Map{{Name: "KFZW", Addr: needleBase + 0x1234}}
	want := []Map{{Name: "KFZW", Addr: 0x1234}, {Name: "LAMFA", Addr: 0x2000}}
	hit, n := matchMaps(got, want)
	if hit != 1 || n != 2 {
		t.Fatalf("%d/%d", hit, n)
	}
}

const needleBase = 0x800000

func TestParseXDF(t *testing.T) {
	b := []byte(`<?xml version="1.0"?>
<XDFFORMAT>
<XDFCONSTANT><title>KRKTE</title><EMBEDDEDDATA mmedaddress="0x10" /></XDFCONSTANT>
<XDFTABLE><title>LAMFA</title>
<XDFAXIS id="x"><EMBEDDEDDATA mmedaddress="0x1" /></XDFAXIS>
<XDFAXIS id="z"><EMBEDDEDDATA mmedaddress="0x20" /></XDFAXIS>
</XDFTABLE>
</XDFFORMAT>`)
	got, err := ParseXDF(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "KRKTE" || got[0].Addr != 0x10 || got[1].Name != "LAMFA" || got[1].Addr != 0x20 {
		t.Fatalf("%+v", got)
	}
}

func TestParseXDFAxes(t *testing.T) {
	b := []byte(`<?xml version="1.0"?>
<XDFFORMAT>
<XDFTABLE><title>LAMFA</title>
<XDFAXIS id="x">
<EMBEDDEDDATA mmedaddress="0x100FF" mmedelementsizebits="8" />
<indexcount>12</indexcount>
</XDFAXIS>
<XDFAXIS id="y"><indexcount>16</indexcount></XDFAXIS>
<XDFAXIS id="z"><EMBEDDEDDATA mmedaddress="0x20" mmedelementsizebits="8" /></XDFAXIS>
</XDFTABLE>
</XDFFORMAT>`)
	got, err := ParseXDFAxes(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "LAMFA" || got[0].ID != "x" || got[0].Addr != 0x100FF || got[0].Count != 12 || got[0].Bits != 8 {
		t.Fatalf("%+v", got)
	}
}

func TestMatchAxes(t *testing.T) {
	got := []Axis{{Name: "LAMFA", ID: "x", Addr: needleBase + 0x100FF, Count: 12, Bits: 8}}
	want := []Axis{
		{Name: "LAMFA", ID: "x", Addr: 0x100FF, Count: 12, Bits: 8},
		{Name: "LAMFA", ID: "y", Addr: 0x100C2, Count: 16, Bits: 8},
	}
	hit, n := matchAxes(got, want)
	if hit != 1 || n != 2 {
		t.Fatalf("%d/%d", hit, n)
	}
	got[0].Bits = 16
	hit, n = matchAxes(got, want)
	if hit != 0 || n != 2 {
		t.Fatalf("width %d/%d", hit, n)
	}
}

func TestCoverCountsNameOnce(t *testing.T) {
	got := cover([]string{"nmot", "rl"}, [][]string{{"nmot"}, {"nmot"}})
	if got.String() != "50.0% (1/2)" {
		t.Fatal(got)
	}
}

func TestCoverMissUntilLocated(t *testing.T) {
	miss := cover([]string{"wkrdy"}, [][]string{{"nmot"}})
	if miss.String() != "0.0% (0/1)" {
		t.Fatal(miss)
	}
	hit := cover([]string{"wkrdy"}, [][]string{{"nmot"}, {"wkrdy"}})
	if hit.String() != "100.0% (1/1)" {
		t.Fatal(hit)
	}
}

func TestScoreWikiOneAddress(t *testing.T) {
	maps := []record.Map{
		{Name: "KFZW", Addr: needleBase + 0x10, X: &record.Axis{Addr: needleBase + 0x11, Count: 8, Bits: 8}},
		{Name: "KFZW", Addr: needleBase + 0x10},
		{Name: "LAMFA", Addr: needleBase + 0x20, X: &record.Axis{Addr: needleBase + 0x21, Count: 4, Bits: 8}},
		{Name: "LAMFA", Addr: needleBase + 0x30, X: &record.Axis{Addr: needleBase + 0x31, Count: 4, Bits: 8}},
		{Name: "KFKHFM", Addr: needleBase + 0x40},
	}
	got := scoreWiki([]string{"KFZW", "LAMFA", "KFKHFM"}, nil, maps, nil)
	if got.String() != "33.3% (1/3)" {
		t.Fatal(got)
	}
	got = scoreWiki([]string{"KFZW", "LAMFA", "KFKHFM"}, map[string]int{"KFKHFM": 0}, maps, nil)
	if got.String() != "66.7% (2/3)" {
		t.Fatal(got)
	}
}

func TestScoreWikiAxisMatchesReference(t *testing.T) {
	maps := []record.Map{{
		Name: "KFZW", Addr: 0x10,
		X: &record.Axis{Addr: 0x20, Count: 8, Bits: 8},
		Y: &record.Axis{Addr: 0x30, Count: 6, Bits: 16},
	}}
	row := refRow{
		name: "KFZW", addr: 0x10,
		axes: map[string]axisSig{
			"x": {id: "x", addr: 0x20, count: 8, bits: 8},
			"y": {id: "y", addr: 0x30, count: 6, bits: 16},
		},
	}
	if got := scoreWiki([]string{"KFZW"}, nil, maps, []refRow{row}); got.String() != "100.0% (1/1)" {
		t.Fatal(got)
	}
	row.axes["y"] = axisSig{id: "y", addr: 0x30, count: 3, bits: 16}
	if got := scoreWiki([]string{"KFZW"}, nil, maps, []refRow{row}); got.Hit != 1 {
		t.Fatal(got)
	}
	dims := map[string]int{"KFZW": 2}
	_, axes := wikiScore([]string{"KFZW"}, dims, maps, []refRow{row})
	if axes.Hit != 2 || axes.Total != 2 {
		t.Fatal(axes)
	}
	maps[0].Y = nil
	_, axes = wikiScore([]string{"KFZW"}, dims, maps, nil)
	if axes.Hit != 1 || axes.Total != 2 {
		t.Fatal(axes)
	}
	other := []refRow{{name: "OTHER", addr: 0x10}}
	if got := scoreWiki([]string{"KFZW"}, nil, maps, other); got.Hit != 1 {
		t.Fatal(got)
	}
	_, axes = wikiScore([]string{"KFZW", "KFKHFM"}, map[string]int{"KFZW": 2, "KFKHFM": 0}, []record.Map{
		{Name: "KFZW", Addr: 0x10, X: &record.Axis{Addr: 0x20, Count: 8, Bits: 8}, Y: &record.Axis{Addr: 0x30, Count: 6, Bits: 16}},
		{Name: "KFKHFM", Addr: 0x40},
	}, nil)
	if axes.Hit != 2 || axes.Total != 2 {
		t.Fatal(axes)
	}
	_, axes = wikiScore([]string{"KFKHFM"}, map[string]int{"KFKHFM": 0}, []record.Map{
		{Name: "KFKHFM", Addr: 0x40},
	}, nil)
	if axes.Total != 0 {
		t.Fatal(axes)
	}
}

func TestRunLayout(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ecu := "[Measurements]\nnmot,{},0xF878,1,0,rpm,0,0,40,0,speed\nrl,{},0x380100,2,0,%,0,0,0.01,0,load\n"
	xdf := "<?xml version=\"1.0\"?><XDFFORMAT><XDFCONSTANT><title>KRKTE</title><EMBEDDEDDATA mmedaddress=\"0x10\" /></XDFCONSTANT><XDFCONSTANT><title>EXTRA</title><EMBEDDEDDATA mmedaddress=\"0x20\" /></XDFCONSTANT></XDFFORMAT>"
	write("a.bin", "")
	write("b.bin", "")
	write("ecu/me7info/a.ecu", ecu)
	write("ecu/me7info/b.ecu", "")
	write("xdf/s4wiki/names.yaml", "names:\n  KFZW: 2\n  LAMFA: 2\n")
	write("xdf/a.xdf", xdf)
	gen := func(name string, _ []byte) ([]record.Item, []record.Map, error) {
		switch name {
		case "a.bin":
			return []record.Item{{Name: "nmot", Addr: 0xF878, Size: 1}},
				[]record.Map{{Name: "KFZW", Addr: needleBase + 0x10, X: &record.Axis{Addr: needleBase + 0x11, Count: 8, Bits: 8}}}, nil
		case "b.bin":
			return []record.Item{{Name: "nmot", Addr: 0xF900, Size: 1}, {Name: "wkrdy", Addr: 1, Size: 1}},
				[]record.Map{{Name: "KFZW", Addr: needleBase + 0x99}}, nil
		default:
			t.Fatalf("unexpected %s", name)
			return nil, nil, nil
		}
	}
	got, err := run(dir, []string{filepath.Join(dir, "a.bin"), filepath.Join(dir, "b.bin")}, gen)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ME7Info) != 1 || got.ME7Info[0].Name != "a.bin" || got.ME7Info[0].String() != "50.0% (1/2)" {
		t.Fatalf("me7info %+v", got.ME7Info)
	}
	if got.ME7Info[0].Corpus.Hit != 1 || got.ME7Info[0].Corpus.Total <= 1 {
		t.Fatalf("catalog %+v", got.ME7Info[0].Corpus)
	}
	if len(got.Extras) != 2 || got.Extras[0].Name != "a.bin" || got.Extras[0].Hit != 1 || got.Extras[1].Hit != 2 {
		t.Fatalf("extras %+v", got.Extras)
	}
	if len(got.S4Wiki) != 2 || got.S4Wiki[0].Name != "a.bin" || got.S4Wiki[1].Name != "b.bin" {
		t.Fatalf("s4wiki %+v", got.S4Wiki)
	}
	if got.S4Wiki[0].String() != "50.0% (1/2)" || got.S4Wiki[1].String() != "0.0% (0/2)" {
		t.Fatalf("s4wiki %+v", got.S4Wiki)
	}
	if len(got.XDF) != 1 || got.XDF[0].Name != "a.bin" || got.XDF[0].String() != "0.0% (0/2)" || got.XDF[0].Axis.Total != 0 {
		t.Fatalf("xdf %+v", got.XDF)
	}
}

func TestSortImages(t *testing.T) {
	ims := []Image{{Name: "c"}, {Name: "b"}, {Name: "a"}, {Name: "d"}}
	sortImages(ims, map[string]string{"a": "C", "b": "S", "c": "S"})
	var got []string
	for _, im := range ims {
		got = append(got, im.Name)
	}
	if want := []string{"b", "c", "a", "d"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReportText(t *testing.T) {
	rep := &Report{
		ME7Info: []Image{{
			Name: "a.bin", Fraction: Fraction{1, 2}, Beyond: 3, Corpus: Fraction{2, 10},
		}},
		Extras: []Image{{Name: "a.bin", Fraction: Fraction{1, 4}}},
		S4Wiki: []Image{
			{
				Name: "a.bin", Fraction: Fraction{1, 2},
				Axis: Fraction{1, 2}, Confidence: Fraction{1, 1},
			},
			{Name: "bb.bin", Fraction: Fraction{0, 2}},
		},
		XDF: []Image{{Name: "a.bin", Fraction: Fraction{0, 3}, Axis: Fraction{1, 4}}},
	}
	got := rep.Text()
	want := "" +
		"ecu me7info   vs ecu-specific     vs corpus       extras\n" +
		"  a          1/2 (+3)   50.0%  2/10   20.0%  1/4   25.0%\n" +
		"\n" +
		"xdf s4wiki                       axis   confidence\n" +
		"  a          1/2   50.0%  1/2   50.0%  1/1  100.0%\n" +
		"  bb         0/2    0.0%  0/0    0.0%             \n" +
		"\n" +
		"xdf                              axis\n" +
		"  a          0/3    0.0%  1/4   25.0%\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestConfidenceSkipFile(t *testing.T) {
	dir := filepath.Join("..", "testdata", "parity")
	skip, err := loadConfidenceSkip(dir)
	if err != nil {
		t.Fatal(err)
	}
	wiki, _, err := loadWiki(dir)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]struct{}{}
	for _, n := range wiki {
		have[n] = struct{}{}
	}
	if len(skip) == 0 {
		t.Fatal("empty skip")
	}
	for n := range skip {
		if _, ok := have[n]; !ok {
			t.Errorf("%s is not an s4wiki name", n)
		}
	}
}

func TestNamesPriorityFile(t *testing.T) {
	dir := filepath.Join("..", "testdata", "parity")
	_, axes, err := loadWiki(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadNamesPriority(dir, axes); err != nil {
		t.Fatal(err)
	}
	delete(axes, "KFZW")
	if _, err := loadNamesPriority(dir, axes); err == nil {
		t.Fatal("want an error for a name outside names.yaml")
	}
}

func TestLayoutBlocks(t *testing.T) {
	dir := filepath.Join("..", "testdata", "parity")
	blocks, err := loadBlocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadLayoutTiers(dir); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "config", "needles.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ns, err := needle.Parse(raw, "needles.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var variants []needle.Needle
	for _, n := range ns {
		if len(n.Pats) == 0 {
			variants = append(variants, needle.Needle{Pattern: n.Pattern, Mask: n.Mask})
		}
		for i := range n.Pats {
			variants = append(variants, needle.Needle{Pattern: n.Pats[i], Mask: n.Masks[i]})
		}
	}

	bins, err := ecucorpus.Open(t).List(filepath.Join(dir, "images.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	hits := map[string][]bool{}
	count := make([]int, len(variants))
	for _, b := range bins {
		img, err := os.ReadFile(b)
		if err != nil {
			t.Fatal(err)
		}
		id := layoutID(img)
		if id == "" {
			t.Fatalf("%s: no software number or EPK", filepath.Base(b))
		}
		if _, ok := hits[id]; ok {
			t.Fatalf("%s: id %s repeated", filepath.Base(b), id)
		}
		h := make([]bool, len(variants))
		for i, v := range variants {
			if len(v.Find(img)) > 0 {
				h[i] = true
				count[i]++
			}
		}
		hits[id] = h
	}
	varying := func(id string) map[int]bool {
		out := map[int]bool{}
		for i, ok := range hits[id] {
			if ok && count[i] < len(bins) {
				out[i] = true
			}
		}
		return out
	}

	seen := map[string]string{}
	for label, ids := range blocks {
		var core map[int]bool
		for _, id := range ids {
			if prev, ok := seen[id]; ok {
				t.Errorf("%s in %s and %s", id, prev, label)
			}
			seen[id] = label
			if _, ok := hits[id]; !ok {
				t.Errorf("%s: %s is not a scored image", label, id)
				continue
			}
			v := varying(id)
			if core == nil {
				core = v
				continue
			}
			for i := range core {
				if !v[i] {
					delete(core, i)
				}
			}
		}
		if len(ids) > 1 && len(core) == 0 {
			t.Errorf("%s: members share no varying needle", label)
		}
	}
	for id := range hits {
		if _, ok := seen[id]; !ok {
			t.Errorf("%s is in no block", id)
		}
	}
}

func TestBeyondME7(t *testing.T) {
	cat := map[string]struct{}{"ti_avg": {}, "nmot": {}}
	meas := map[string]struct{}{"wkrdy": {}}
	items := []record.Item{
		{Name: "nmot"},
		{Name: "ti_avg"},
		{Name: "ti_avg"},
		{Name: "wkrdy"},
		{Name: "other"},
	}
	want := []record.Item{{Name: "nmot"}}
	if n := beyondME7(items, want, cat, meas); n != 1 {
		t.Fatalf("%d", n)
	}
}

func TestME7InfoParity(t *testing.T) {
	rep, err := Run(filepath.Join("..", "testdata", "parity"), ecucorpus.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ME7Info) == 0 {
		t.Fatal("no ME7Info image")
	}
	for _, im := range rep.ME7Info {
		if im.Hit != im.Total {
			t.Errorf("%s %s", im.Name, im.Fraction)
		}
	}
}

func TestFraction(t *testing.T) {
	if (Fraction{1, 4}).String() != "25.0% (1/4)" {
		t.Fatal((Fraction{1, 4}).String())
	}
	if (Fraction{}).String() != "0.0% (0/0)" {
		t.Fatal((Fraction{}).String())
	}
}
