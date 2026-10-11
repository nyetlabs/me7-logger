package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/parity"
)

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
	if got := handCover(hand, covered); got != "" {
		t.Fatalf("superset: %s", got)
	}
	merged := mergeKept(hand, covered)
	if len(merged.Objects) != 2 {
		t.Fatalf("%d objects", len(merged.Objects))
	}
	kw := merged.Objects[0]
	if kw.Key != "KFZW" || kw.Value.Conversion.Factor != 0.75 || kw.Value.Units != "grad KW" {
		t.Fatalf("kept %+v", kw.Value)
	}
	if merged.Objects[1].Key != "LAMFA" || merged.Objects[1].Value.Conversion.Factor != 1 {
		t.Fatalf("new %+v", merged.Objects[1].Value)
	}
	moved := modelOf(pubObj("KFZW", 0x300, 1, ""), pubObj("LAMFA", 0x200, 1, ""))
	if got := handCover(hand, moved); got != "hand disagrees KFZW" {
		t.Fatalf("moved: %s", got)
	}
	short := modelOf(pubObj("LAMFA", 0x200, 1, ""))
	two := modelOf(pubObj("KFZW", 0x100, 0.75, "grad KW"), pubObj("LAMFA", 0x180, 1, "%%"))
	if got := handCover(two, short); got != "hand short KFZW" {
		t.Fatalf("short: %s", got)
	}
	same := modelOf(pubObj("KFZW", 0x100, 1, ""))
	if got := handCover(hand, same); got != "hand equal" {
		t.Fatalf("equal: %s", got)
	}
	shifted := mergeKept(hand, modelOf(pubObj("KFZW", 0x300, 1, "raw")))
	if shifted.Objects[0].Value.Conversion.Factor != 1 || shifted.Objects[0].Value.Units != "raw" {
		t.Fatalf("moved object kept the hand scaling: %+v", shifted.Objects[0].Value)
	}
}

func modelOf(objs ...*model.Object) *model.Model {
	return &model.Model{Objects: objs}
}

func pubObj(key string, addr uint32, factor float64, units string) *model.Object {
	return &model.Object{
		Key: key, ID: key, Shape: "value", Address: model.Addr(addr),
		Rows: 1, Cols: 1,
		Value: model.Value{Units: units, Conversion: model.Conversion{Factor: factor}},
	}
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
