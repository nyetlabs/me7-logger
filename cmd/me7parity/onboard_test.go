package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/parity"
)

func TestStageLabel(t *testing.T) {
	cases := []struct{ tier, want string }{
		{"D", "1"},
		{"D*", "1*"},
		{"C*", "2*"},
		{"B", "3-"},
		{"A*", "3*"},
		{"S", "S"},
		{"S+", "S+"},
		{"-", "-"},
		{"-*", "-"},
	}
	for _, c := range cases {
		if got := stageLabel(c.tier); got != c.want {
			t.Errorf("stageLabel(%q) = %q, want %q", c.tier, got, c.want)
		}
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
	in := "# blocks\n4Z7:\n  - \"1037366370\"\n4Z7-like:\n  - \"1037366497\"\n  - \"1037369307\"\n"
	got, err := insertLayout(in, "4Z7-like", "1037369044")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "  - \"1037369044\"\n  - \"1037369307\"\n") {
		t.Fatalf("got\n%s", got)
	}
	if !strings.Contains(got, "# blocks\n4Z7:\n") {
		t.Fatalf("comment dropped\n%s", got)
	}
	if _, err := insertLayout(got, "4Z7-like", "1037369044"); err == nil {
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
	if got := gateReason("D", high, false, ""); got != "" {
		t.Fatalf("missing def: %s", got)
	}
	if got := gateReason("S+", high, true, "located"); got != "" {
		t.Fatalf("located: %s", got)
	}
	if got := gateReason("-", high, false, ""); got != "grade -" {
		t.Fatalf("open grade: %s", got)
	}
	if got := gateReason("D", half, false, ""); got != "confidence 37/74" {
		t.Fatalf("half: %s", got)
	}
	if got := gateReason("D", parity.Fraction{}, false, ""); got != "confidence 0/0" {
		t.Fatalf("empty: %s", got)
	}
	if got := gateReason("-", high, true, "hand"); got != "" {
		t.Fatalf("hand does not wait for D: %s", got)
	}
	if got := gateReason("D", half, true, "hand"); got != "confidence 37/74" {
		t.Fatalf("hand confidence: %s", got)
	}
	if got := gateReason("C", high, true, "hand"); got != "" {
		t.Fatalf("hand: %s", got)
	}
	if got := gateReason("A", high, true, "damos"); got != "origin damos" {
		t.Fatalf("damos: %s", got)
	}
	if got := gateReason("-", high, true, ""); got != "origin unset" {
		t.Fatalf("unset: %s", got)
	}
}

func TestHandCover(t *testing.T) {
	hand := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"))
	covered := modelOf(
		pubObj("KFZW", 0x100, 1, ""),
		pubObj("LAMFA", 0x200, 1, ""),
	)
	if got, n := handCover(hand, covered); got != "" || n != 1 {
		t.Fatalf("superset: %s +%d", got, n)
	}
	hand.Objects[0].Description = "map_interp_table8"
	covered.Objects[0].Description = "optimal ignition angle"
	covered.Objects[0].Comment = "map_interp_table8"
	merged, err := mergeKept(hand, covered)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Objects) != 2 {
		t.Fatalf("%d objects", len(merged.Objects))
	}
	kw := merged.Objects[0]
	if kw.Key != "KFZW" || kw.Address != 0x100 || kw.Value.Conversion.Factor != 0.75 || kw.Value.Units != "grad KW" || kw.Description != "optimal ignition angle" || kw.Comment != "map_interp_table8" {
		t.Fatalf("kept %+v", kw)
	}
	if merged.Objects[1].Key != "LAMFA" || merged.Objects[1].Value.Conversion.Factor != 1 {
		t.Fatalf("new %+v", merged.Objects[1].Value)
	}
	moved := modelOf(pubObj("KFZW", 0x300, 1, ""), pubObj("LAMFA", 0x200, 1, ""))
	if got, _ := handCover(hand, moved); got != "hand disagrees KFZW" {
		t.Fatalf("moved: %s", got)
	}
	short := modelOf(pubObj("LAMFA", 0x200, 1, ""))
	two := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"), pubObj("LAMFA", 0x180, 1, "%%"))
	if got, _ := handCover(two, short); got != "hand short KFZW" {
		t.Fatalf("short: %s", got)
	}
	reason, n, notes := coverAll(two, modelOf(pubObj("OTHER", 0x1, 1, "")))
	if reason != "hand short KFZW" || n != 1 || len(notes) != 3 || notes[0].line() != "short KFZW 0x100" || notes[1].line() != "short LAMFA 0x180" || notes[2].line() != "added OTHER 0x1" {
		t.Fatalf("detail %s +%d %+v", reason, n, notes)
	}
	if _, n, notes = coverAll(hand, covered); n != 1 || len(notes) != 1 || notes[0].line() != "added LAMFA 0x200" {
		t.Fatalf("added %+v +%d", notes, n)
	}
	same := modelOf(pubObj("KFZW", 0x100, 1, ""))
	if got, n := handCover(hand, same); got != "identical" || n != 0 {
		t.Fatalf("identical: %s +%d", got, n)
	}
	shifted, err := mergeKept(hand, modelOf(pubObj("KFZW", 0x300, 1, "raw")))
	if err != nil {
		t.Fatal(err)
	}
	if shifted.Objects[0].Address != 0x100 || shifted.Objects[0].Value.Conversion.Factor != 0.75 || shifted.Objects[0].Value.Units != "grad KW" {
		t.Fatalf("moved name changed the base: %+v", shifted.Objects[0])
	}
	if n := corrections(hand, merged); n != 1 {
		t.Fatalf("corrections %d", n)
	}
	unchanged, err := mergeKept(hand, modelOf(pubObj("KFZW", 0x100, 1, "")))
	if err != nil {
		t.Fatal(err)
	}
	if n := corrections(hand, unchanged); n != 0 {
		t.Fatalf("unchanged %d", n)
	}
	shortMoved := modelOf(pubObj("LAMFA", 0x180, 1, "%%"), pubObj("KFZW", 0x100, 0.75, "grad KW"))
	shortMoved.Objects[1].Description = "old"
	shortMoved.Objects[1].Comment = "oldc"
	locatedMoved := modelOf(pubObj("KFZW", 0x300, 1, "raw"), pubObj("OTHER", 0x400, 2, "rpm"))
	locatedMoved.Objects[0].Description = "optimal"
	locatedMoved.Objects[0].Comment = "needle"
	reason, _, notes = coverAll(shortMoved, locatedMoved)
	if reason != "hand short LAMFA" || moveCount(notes) != 1 || !hasShort(notes) || partialDelta(1, 1) != "~1 +1" {
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
	twoAddrs := []coverNote{{op: "disagrees", key: "KFZW", have: []model.Addr{0x300, 0x400}}}
	if moveCount(twoAddrs) != 0 || applyMoves(hand, locatedMoved, twoAddrs).Objects[0].Address != 0x100 {
		t.Fatal("several addresses")
	}
	pedal := pubObj("KFPED", 0x100, 0.75, "Nm")
	pedal.Categories = []int{3}
	pedalHand := modelOf(pedal, pubObj("LAMFA", 0x180, 1, "%%"))
	pair := modelOf(
		pubObj("KFPED_0_A", 0x100, 1, ""),
		pubObj("KFPED_1_A", 0x200, 1, ""),
		pubObj("LAMFA", 0x180, 1, ""),
	)
	if got, n := handCover(pedalHand, pair); got != "" || n != 2 {
		t.Fatalf("pair covers KFPED: %s +%d", got, n)
	}
	mergedPedal, err := mergeKept(pedalHand, pair)
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
	if got, _ := handCover(pedalHand, one); got != "hand short KFPED" {
		t.Fatalf("one variant: %s", got)
	}
	movedPair := modelOf(pubObj("KFPED_0_A", 0x300, 1, ""), pubObj("KFPED_1_A", 0x200, 1, ""), pubObj("LAMFA", 0x180, 1, ""))
	if got, _ := handCover(pedalHand, movedPair); got != "hand disagrees KFPED" {
		t.Fatalf("pair moved: %s", got)
	}
	plain := modelOf(pubObj("KFPED", 0x100, 1, ""))
	variantHand := modelOf(pubObj("KFPED_0_A", 0x100, 0.5, "Nm"))
	if got, _ := handCover(variantHand, plain); got != "hand short KFPED_0_A" {
		t.Fatalf("plain does not cover the variant: %s", got)
	}
	kept := modelOf(pubObj("KFPED_0_A", 0x100, 0.5, "Nm"), pubObj("LAMFA", 0x180, 1, ""))
	kept.Objects[0].Categories = []int{4}
	added := modelOf(pubObj("KFPED_0_A", 0x100, 1, ""), pubObj("KFPED_1_A", 0x200, 1, ""), pubObj("LAMFA", 0x180, 1, ""))
	if got, n := handCover(kept, added); got != "" || n != 1 {
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
	if got, n := handCover(named, same); got != "identical" || n != 0 {
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
