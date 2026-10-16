package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/parity"
)

func TestStageLabel(t *testing.T) {
	cases := []struct{ tier, want string }{
		{"D", "1"},
		{"C", "2"},
		{"B", "3-"},
		{"A", "3"},
		{"S", "3+"},
		{"S+", "S+"},
		{"-", "-"},
		{"", "-"},
		{"D*", ""},
	}
	for _, c := range cases {
		if got := stageLabel(c.tier); got != c.want {
			t.Errorf("stageLabel(%q) = %q, want %q", c.tier, got, c.want)
		}
	}
}

func TestZipDef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "IMG-0001-20260101.zip")
	z, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(z)
	f, err := w.Create("IMG-0001.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("def")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := zipDef(path, "IMG-0001")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "def" {
		t.Fatalf("got %q", got)
	}
}

func TestInsertImageKeepsComments(t *testing.T) {
	in := "# header\n- AAA\n# note for CCC\n- CCC\n"
	got, err := insertSorted(in, "BBB", "- BBB")
	if err != nil {
		t.Fatal(err)
	}
	want := "# header\n- AAA\n- BBB\n# note for CCC\n- CCC\n"
	if got != want {
		t.Fatalf("got\n%s", got)
	}
	first, err := insertSorted("# header\n- MMM\n", "AAA", "- AAA")
	if err != nil {
		t.Fatal(err)
	}
	if first != "# header\n- AAA\n- MMM\n" {
		t.Fatalf("first\n%s", first)
	}
}

func TestInsertLayout(t *testing.T) {
	in := "# blocks\n25A:\n  - \"1037366370\"\n24EGKS:\n  - \"1037366497\"\n  - \"1037369307\"\n"
	got, err := insertLayout(in, "24EGKS", "1037369044")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "  - \"1037369044\"\n  - \"1037369307\"\n") {
		t.Fatalf("got\n%s", got)
	}
	if !strings.Contains(got, "# blocks\n25A:\n") {
		t.Fatalf("comment dropped\n%s", got)
	}
	if _, err := insertLayout(got, "24EGKS", "1037369044"); err == nil {
		t.Fatal("duplicate id accepted")
	}
}

func TestNearestName(t *testing.T) {
	scored := []string{"4B0906018CM-0008", "4B0906018Q-0008", "06A906032NL-0003"}
	if got := nearestName("4B0906018DH-0004", scored); got != "4B0906018CM-0008" {
		t.Fatalf("nearest %s", got)
	}
}

func TestGateReason(t *testing.T) {
	high := parity.Fraction{Hit: 73, Total: 74}
	half := parity.Fraction{Hit: 37, Total: 74}
	if got := gateReason("D", high, nil, false, ""); got != "" {
		t.Fatalf("missing def: %s", got)
	}
	if got := gateReason("S+", high, nil, true, "auto"); got != "" {
		t.Fatalf("located: %s", got)
	}
	if got := gateReason("-", high, nil, false, ""); got != "grade -" {
		t.Fatalf("open grade: %s", got)
	}
	if got := gateReason("D", half, nil, false, ""); got != "confidence 37/74" {
		t.Fatalf("half: %s", got)
	}
	if got := gateReason("D", parity.Fraction{}, nil, false, ""); got != "confidence 0/0" {
		t.Fatalf("empty: %s", got)
	}
	if got := gateReason("-", high, nil, true, "manual"); got != "" {
		t.Fatalf("manual does not wait for D: %s", got)
	}
	if got := gateReason("D", half, nil, true, "manual"); got != "confidence 37/74" {
		t.Fatalf("manual confidence: %s", got)
	}
	if got := gateReason("C", high, nil, true, "manual"); got != "" {
		t.Fatalf("manual: %s", got)
	}
	if got := gateReason("A", high, nil, true, "damos"); got != "origin damos" {
		t.Fatalf("damos: %s", got)
	}
	if got := gateReason("-", high, nil, true, ""); got != "origin unset" {
		t.Fatalf("unset: %s", got)
	}
	turned := map[string]parity.Conf{
		"KFZW":  {Reason: "axis"},
		"LAMFA": {Reason: "axis-zeros"},
	}
	if got := gateReason("D", high, turned, true, "auto"); got != "axis KFZW" {
		t.Fatalf("hard axis: %s", got)
	}
	if got := gateReason("D", high, map[string]parity.Conf{"LAMFA": {Reason: "axis-zeros"}}, true, "auto"); got != "" {
		t.Fatalf("soft zeros: %s", got)
	}
	if got := gateReason("D", high, map[string]parity.Conf{"KFZW": {Reason: "shape"}}, true, "auto"); got != "" {
		t.Fatalf("soft shape: %s", got)
	}
	if got := gateReason("D", high, map[string]parity.Conf{"KFZW": {Reason: "body"}}, true, "auto"); got != "body KFZW" {
		t.Fatalf("hard body: %s", got)
	}
	both := map[string]parity.Conf{
		"KFZW":  {Reason: "body"},
		"LAMFA": {Reason: "axis"},
	}
	if got := gateReason("D", high, both, true, "auto"); got != "axis LAMFA" {
		t.Fatalf("axis before body: %s", got)
	}
}

func TestReference(t *testing.T) {
	refs, err := loadRefTable("../../testdata/parity")
	if err != nil {
		t.Fatal(err)
	}
	prev := modelOf(
		pubObj("KFZW", 0x100, 1, ""),
		pubObj("ESKONF", 0x180, 1, ""),
		pubObj("KFLBTS", 0x200, 1, ""),
		pubObj("KFPED", 0x300, 1, ""),
		pubObj("LAMFA", 0x400, 1, ""),
	)
	loc := modelOf(
		pubObj("KFZW", 0x100, 1, ""),
		pubObj("ESKONF", 0x190, 1, ""),
		pubObj("KFLBTS_0_A", 0x200, 1, ""),
		pubObj("KFPED_0_A", 0x300, 1, ""),
		pubObj("KFPED_1_A", 0x340, 1, ""),
		pubObj("LAMFA", 0x410, 1, ""),
		pubObj("KFMIRL", 0x500, 1, ""),
	)
	if reference(prev, loc, refs) != nil {
		t.Fatal("reference without provenance")
	}
	prev.Provenance = &model.Provenance{Format: "kp", Origin: "manual", File: "h.kp", SHA256: strings.Repeat("a", 64), Verified: true}
	r := reference(prev, loc, refs)
	if r.Origin != "manual" || r.File != "h.kp" || r.Agree != [2]int{3, 5} || !slices.Equal(r.Except, []string{"ESKONF"}) || !slices.Equal(r.Unreferenced, []string{"KFMIRL"}) || r.Agreed() {
		t.Fatalf("%+v", r)
	}
	path := filepath.Join(t.TempDir(), "d.json")
	merged, err := mergeKept(prev, loc)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePublished(path, merged, false, r); err != nil {
		t.Fatal(err)
	}
	got, err := readModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Provenance; p.Origin != "auto" || !p.Verified || p.Reference.Agree != r.Agree || prev.Provenance.Origin != "manual" {
		t.Fatalf("%+v", p)
	}
}

func TestSourceClass(t *testing.T) {
	agreed := &model.Reference{Agree: [2]int{4, 5}, Except: []string{"ESKONF"}}
	partial := &model.Reference{Agree: [2]int{3, 5}}
	for _, c := range []struct {
		p    *model.Provenance
		want string
	}{
		{nil, "auto"},
		{&model.Provenance{Origin: "manual", Verified: true}, "verified"},
		{&model.Provenance{Origin: "auto", Verified: true}, "verified"},
		{&model.Provenance{Origin: "damos"}, "DAMOS"},
		{&model.Provenance{Origin: "a2l"}, "DAMOS"},
		{&model.Provenance{Origin: "manual"}, "manual"},
		{&model.Provenance{Origin: "auto", Reference: agreed}, "matched"},
		{&model.Provenance{Origin: "auto", Reference: partial}, "auto"},
		{&model.Provenance{Origin: "auto"}, "auto"},
	} {
		if got := sourceClass(c.p); got != c.want {
			t.Errorf("%+v: %s, want %s", c.p, got, c.want)
		}
	}
}

func TestHandCover(t *testing.T) {
	manual := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"))
	covered := modelOf(
		pubObj("KFZW", 0x100, 1, ""),
		pubObj("LAMFA", 0x200, 1, ""),
	)
	if got, n, _ := coverAll(manual, covered); got != "" || n != 1 {
		t.Fatalf("superset: %s +%d", got, n)
	}
	manual.Objects[0].Description = "map_interp_table8"
	manual.Objects[0].Comment = "hand note"
	covered.Objects[0].Description = "optimal ignition angle"
	covered.Objects[0].Comment = "map_interp_table8"
	merged, err := mergeKept(manual, covered)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Objects) != 2 {
		t.Fatalf("%d objects", len(merged.Objects))
	}
	kw := merged.Objects[0]
	if kw.Key != "KFZW" || kw.Address != 0x100 || kw.Value.Conversion.Factor != 0.75 || kw.Value.Units != "grad KW" || kw.Description != "optimal ignition angle" || kw.Comment != "hand note" {
		t.Fatalf("kept %+v", kw)
	}
	if merged.Objects[1].Key != "LAMFA" || merged.Objects[1].Value.Conversion.Factor != 1 {
		t.Fatalf("new %+v", merged.Objects[1].Value)
	}
	moved := modelOf(pubObj("KFZW", 0x300, 1, ""), pubObj("LAMFA", 0x200, 1, ""))
	if got, _, _ := coverAll(manual, moved); got != "manual disagrees KFZW" {
		t.Fatalf("moved: %s", got)
	}
	short := modelOf(pubObj("LAMFA", 0x200, 1, ""))
	two := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"), pubObj("LAMFA", 0x180, 1, "%%"))
	if got, _, _ := coverAll(two, short); got != "manual short KFZW" {
		t.Fatalf("short: %s", got)
	}
	reason, n, notes := coverAll(two, modelOf(pubObj("OTHER", 0x1, 1, "")))
	if reason != "manual short KFZW" || n != 1 || len(notes) != 3 || notes[0].line() != "short KFZW 0x100" || notes[1].line() != "short LAMFA 0x180" || notes[2].line() != "added OTHER 0x1" {
		t.Fatalf("detail %s +%d %+v", reason, n, notes)
	}
	if _, n, notes = coverAll(manual, covered); n != 1 || len(notes) != 1 || notes[0].line() != "added LAMFA 0x200" {
		t.Fatalf("added %+v +%d", notes, n)
	}
	same := modelOf(pubObj("KFZW", 0x100, 1, ""))
	if got, n, _ := coverAll(manual, same); got != "identical" || n != 0 {
		t.Fatalf("identical: %s +%d", got, n)
	}
	shifted, err := mergeKept(manual, modelOf(pubObj("KFZW", 0x300, 1, "raw")))
	if err != nil {
		t.Fatal(err)
	}
	if shifted.Objects[0].Address != 0x100 || shifted.Objects[0].Value.Conversion.Factor != 0.75 || shifted.Objects[0].Value.Units != "grad KW" {
		t.Fatalf("moved name changed the base: %+v", shifted.Objects[0])
	}
	if n := corrections(manual, merged); n != 1 {
		t.Fatalf("corrections %d", n)
	}
	unchanged, err := mergeKept(manual, modelOf(pubObj("KFZW", 0x100, 1, "")))
	if err != nil {
		t.Fatal(err)
	}
	if n := corrections(manual, unchanged); n != 0 {
		t.Fatalf("unchanged %d", n)
	}
	shortMoved := modelOf(pubObj("LAMFA", 0x180, 1, "%%"), pubObj("KFZW", 0x100, 0.75, "grad KW"))
	shortMoved.Objects[1].Description = "old"
	shortMoved.Objects[1].Comment = "oldc"
	locatedMoved := modelOf(pubObj("KFZW", 0x300, 1, "raw"), pubObj("OTHER", 0x400, 2, "rpm"))
	locatedMoved.Objects[0].Description = "optimal"
	locatedMoved.Objects[0].Comment = "needle"
	reason, _, notes = coverAll(shortMoved, locatedMoved)
	if reason != "manual short LAMFA" || moveCount(notes) != 1 || !hasShort(notes) || partialDelta(1, 1) != "~1 +1" {
		t.Fatalf("short correction %s %+v", reason, notes)
	}
	fixed := applyMoves(shortMoved, locatedMoved, notes)
	zw, lam := fixed.Objects[1], fixed.Objects[0]
	if zw.Address != 0x300 || zw.Value.Conversion.Factor != 0.75 || zw.Value.Units != "grad KW" || zw.Description != "optimal" || zw.Comment != "needle" {
		t.Fatalf("corrected %+v", zw)
	}
	if lam.Address != 0x180 || lam.Key != "LAMFA" {
		t.Fatalf("short kept %+v", lam)
	}
	if len(fixed.Objects) != 3 || fixed.Objects[2].Key != "OTHER" || fixed.Objects[2].Address != 0x400 || fixed.Objects[2].Value.Units != "rpm" {
		t.Fatalf("added %+v", fixed.Objects)
	}
	if shortMoved.Objects[1].Address != 0x100 {
		t.Fatal("base mutated")
	}
	autoPrev := modelOf(pubObj("KFZW", 0x100, 1, ""))
	autoPrev.Provenance = &model.Provenance{Origin: "auto"}
	autoPrev.Objects[0].Comment = "old-needle"
	autoLoc := modelOf(pubObj("KFZW", 0x100, 1, ""))
	autoLoc.Objects[0].Comment = "map_interp_table8"
	autoMerged, err := mergeKept(autoPrev, autoLoc)
	if err != nil {
		t.Fatal(err)
	}
	if autoMerged.Objects[0].Comment != "map_interp_table8" {
		t.Fatalf("auto comment replace: %q", autoMerged.Objects[0].Comment)
	}
	annotateConfidence(autoMerged, map[string]parity.Conf{
		"KFZW": {High: true, Reason: "short"},
	})
	if autoMerged.Objects[0].Comment != "map_interp_table8" {
		t.Fatalf("high keeps needle: %q", autoMerged.Objects[0].Comment)
	}
	annotateConfidence(autoMerged, map[string]parity.Conf{
		"KFZW": {High: false, Reason: "peer-diff 6/192 peerA"},
	})
	if autoMerged.Objects[0].Comment != "map_interp_table8: differs from peerA by 6/192 cells" {
		t.Fatalf("low annotate: %q", autoMerged.Objects[0].Comment)
	}
	annotateConfidence(autoMerged, map[string]parity.Conf{
		"KFZW": {High: false, Reason: "peer-diff 6/192 peerA"},
	})
	if autoMerged.Objects[0].Comment != "map_interp_table8: differs from peerA by 6/192 cells" {
		t.Fatalf("annotate twice: %q", autoMerged.Objects[0].Comment)
	}
	emptyNeedle := modelOf(pubObj("LAMFA", 0x200, 1, ""))
	annotateConfidence(emptyNeedle, map[string]parity.Conf{"LAMFA": {High: false, Reason: "zeros"}})
	annotateConfidence(emptyNeedle, map[string]parity.Conf{"LAMFA": {High: false, Reason: "zeros"}})
	if emptyNeedle.Objects[0].Comment != "zero body with no zero peer" {
		t.Fatalf("empty needle doubled: %q", emptyNeedle.Objects[0].Comment)
	}
	keptConv := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"))
	keptConv.Objects[0].Comment = "hand"
	annotateConfidence(keptConv, map[string]parity.Conf{
		"KFZW": {High: false, Reason: "zeros"},
	})
	if keptConv.Objects[0].Comment != "hand" {
		t.Fatalf("raw-only annotate: %q", keptConv.Objects[0].Comment)
	}
	twoAddrs := []coverNote{{op: "disagrees", key: "KFZW", have: []model.Addr{0x300, 0x400}}}
	if moveCount(twoAddrs) != 0 || applyMoves(manual, locatedMoved, twoAddrs).Objects[0].Address != 0x100 {
		t.Fatal("several addresses")
	}
	pedal := pubObj("KFPED", 0x100, 0.75, "Nm")
	pedal.Categories = []int{3}
	pedalManual := modelOf(pedal, pubObj("LAMFA", 0x180, 1, "%%"))
	pair := modelOf(
		pubObj("KFPED_0_A", 0x100, 1, ""),
		pubObj("KFPED_1_A", 0x200, 1, ""),
		pubObj("LAMFA", 0x180, 1, ""),
	)
	if got, n, _ := coverAll(pedalManual, pair); got != "" || n != 2 {
		t.Fatalf("pair covers KFPED: %s +%d", got, n)
	}
	mergedPedal, err := mergeKept(pedalManual, pair)
	if err != nil {
		t.Fatal(err)
	}
	gotKeys := map[string]*model.Object{}
	for _, o := range mergedPedal.Objects {
		gotKeys[o.Key] = o
	}
	if gotKeys["KFPED"] != nil || gotKeys["KFPED_0_A"] == nil || gotKeys["KFPED_1_A"] == nil {
		t.Fatalf("pedal keys %+v", gotKeys)
	}
	if gotKeys["KFPED_0_A"].Address != 0x100 || gotKeys["KFPED_0_A"].Value.Conversion.Factor != 0.75 || gotKeys["KFPED_0_A"].Value.Units != "Nm" {
		t.Fatalf("kept pedal %+v", gotKeys["KFPED_0_A"])
	}
	if len(gotKeys["KFPED_0_A"].Categories) != 1 || gotKeys["KFPED_0_A"].Categories[0] != 3 || gotKeys["KFPED_1_A"].Categories[0] != 3 {
		t.Fatalf("pedal categories %+v %+v", gotKeys["KFPED_0_A"].Categories, gotKeys["KFPED_1_A"].Categories)
	}
	if gotKeys["LAMFA"].Value.Units != "%%" {
		t.Fatalf("unlocated %+v", gotKeys["LAMFA"])
	}
	one := modelOf(pubObj("KFPED_0_A", 0x100, 1, ""), pubObj("LAMFA", 0x180, 1, ""))
	if got, _, _ := coverAll(pedalManual, one); got != "manual short KFPED" {
		t.Fatalf("one variant: %s", got)
	}
	movedPair := modelOf(pubObj("KFPED_0_A", 0x300, 1, ""), pubObj("KFPED_1_A", 0x200, 1, ""), pubObj("LAMFA", 0x180, 1, ""))
	if got, _, _ := coverAll(pedalManual, movedPair); got != "manual disagrees KFPED" {
		t.Fatalf("pair moved: %s", got)
	}
	plain := modelOf(pubObj("KFPED", 0x100, 1, ""))
	variantManual := modelOf(pubObj("KFPED_0_A", 0x100, 0.5, "Nm"))
	if got, _, _ := coverAll(variantManual, plain); got != "manual short KFPED_0_A" {
		t.Fatalf("plain does not cover the variant: %s", got)
	}
	kept := modelOf(pubObj("KFPED_0_A", 0x100, 0.5, "Nm"), pubObj("LAMFA", 0x180, 1, ""))
	kept.Objects[0].Categories = []int{4}
	added := modelOf(pubObj("KFPED_0_A", 0x100, 1, ""), pubObj("KFPED_1_A", 0x200, 1, ""), pubObj("LAMFA", 0x180, 1, ""))
	if got, n, _ := coverAll(kept, added); got != "" || n != 1 {
		t.Fatalf("existing variant: %s +%d", got, n)
	}
	mergedKept, err := mergeKept(kept, added)
	if err != nil {
		t.Fatal(err)
	}
	var old, neu *model.Object
	for _, o := range mergedKept.Objects {
		switch o.Key {
		case "KFPED_0_A":
			old = o
		case "KFPED_1_A":
			neu = o
		}
	}
	if old == nil || old.Value.Conversion.Factor != 0.5 || neu == nil || neu.Categories[0] != 4 {
		t.Fatalf("kept variant %+v next %+v", old, neu)
	}
	ph := pubObj("obj-2", 0x50, 1, "")
	ph.ID = ""
	named := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"), ph)
	if got, n, _ := coverAll(named, same); got != "identical" || n != 0 {
		t.Fatalf("placeholder is a name: %s +%d", got, n)
	}
	if placeholderCount(named) != 1 || placeholderCount(modelOf(pubObj("obj-2", 0x50, 1, ""))) != 0 {
		t.Fatal("placeholder count")
	}
	cleared := clearedCopy(named)
	if cleared.Objects[1].Key != "" || named.Objects[1].Key != "obj-2" {
		t.Fatalf("cleared %q base %q", cleared.Objects[1].Key, named.Objects[1].Key)
	}
}

func TestMergeKeptRelayout(t *testing.T) {
	for _, origin := range []string{"auto", "manual"} {
		prev := modelOf(pubObj("KFMIRL", 0x100, 1, ""), pubObj("KFZW", 0x200, 0.75, "grad KW"))
		prev.Provenance = &model.Provenance{Format: "image", Origin: origin, SHA256: strings.Repeat("ab", 32)}
		loc := modelOf(pubObj("KFMIRL", 0x100, 1, ""), pubObj("KFZW", 0x200, 1, ""))
		for _, o := range loc.Objects {
			o.Data = model.Data{Bits: 16, Endian: "little"}
		}
		merged, err := mergeKept(prev, loc)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if origin == "auto" {
			want = 16
		}
		if merged.Objects[0].Data.Bits != want || merged.Objects[1].Data.Bits != 0 {
			t.Fatalf("%s: %+v %+v", origin, merged.Objects[0].Data, merged.Objects[1].Data)
		}
		if n := corrections(prev, merged); n != want/16 {
			t.Fatalf("%s: corrections %d", origin, n)
		}
	}
}

func TestCorrectionsDuplicate(t *testing.T) {
	first := pubObj("CDSLS", 0x181AE, 1, "")
	first.Description = "CDSLS - Euro-Byte"
	second := pubObj("CDSLS", 0x181AE, 1, "")
	second.Description = "Euro-Byte"
	prev := modelOf(first, second)
	if n := corrections(prev, clearedCopy(prev)); n != 0 {
		t.Fatalf("duplicate descriptions: %d", n)
	}
	changed := clearedCopy(prev)
	changed.Objects[0].Data.Bits = 16
	if n := corrections(prev, changed); n != 1 {
		t.Fatalf("one layout: %d", n)
	}
}

func modelOf(objs ...*model.Object) *model.Model {
	return &model.Model{Schema: model.SchemaID, Objects: objs}
}

func pubObj(key string, addr uint32, factor float64, units string) *model.Object {
	return &model.Object{
		Key: key, ID: key, Shape: "value", Address: model.Addr(addr),
		Rows: 1, Cols: 1,
		Value: model.Value{Units: units, Conversion: model.Conversion{Factor: factor}},
	}
}

func TestMatchStems(t *testing.T) {
	stems := []string{"8D0907551H-0003", "8D0907551M-0002", "4D1907558-0002", "4D1907558-0004"}
	got, err := matchStems(stems, []string{"8D0907551H"})
	if err != nil || len(got) != 1 || got[0] != "8D0907551H-0003" {
		t.Fatalf("%v %v", got, err)
	}
	got, err = matchStems(stems, []string{"8D0907551H-0003.bin"})
	if err != nil || len(got) != 1 || got[0] != "8D0907551H-0003" {
		t.Fatalf("stem %v %v", got, err)
	}
	got, err = matchStems(stems, []string{"4D1907558"})
	if err != nil || len(got) != 2 || got[0] != "4D1907558-0002" || got[1] != "4D1907558-0004" {
		t.Fatalf("family %v %v", got, err)
	}
	if _, err := matchStems(stems, []string{"8D0907551"}); err == nil {
		t.Fatal("short part number matched")
	}
	if _, err := matchStems(stems, nil); err != nil || len(must(matchStems(stems, nil))) != 0 {
		t.Fatal("empty args")
	}
}

func must(v []string, err error) []string {
	if err != nil {
		panic(err)
	}
	return v
}

func TestSetCorpusDef(t *testing.T) {
	dir := t.TempDir()
	body := "name\tsha256\tsize\tdef\nA\th\t1\t-\nB\th\t1\tdefs/B.json\n"
	path := filepath.Join(dir, "corpus.tsv")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setCorpusDef(dir, "A", "defs/A.json"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "name\tsha256\tsize\tdef\nA\th\t1\tdefs/A.json\nB\th\t1\tdefs/B.json\n"
	if string(got) != want {
		t.Fatalf("got %q", got)
	}
}
