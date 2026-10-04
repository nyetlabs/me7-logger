package mapfile

import (
	"os"
	"strings"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/m.yaml"
	body := `variables:
- rt: "0x0001"
  bitmask: "0x00"
  name: nmot
  size: 0
  unit: rpm
  signed: false
  inverse: false
  factor: 40
  offset: 0
  comment: speed
- rt: "0x0005"
  bitmask: "0x00"
  name: te_w
  size: 0
  unit: ms
  factor: 0.00266667
  offset: 0
  comment: inj
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := Parse(b, path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := tab.Lookup(1, 0)
	if !ok || v.Name != "nmot" || v.A != 40 || v.Size != 0 {
		t.Fatalf("%+v", v)
	}
	bare, err := Parse([]byte("variables:\n- rt: \"0x0010\"\n  name: bare\n  comment: x\n- rt: \"0x0011\"\n  name: zero\n  factor: 0\n  comment: y\n"), "t", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, ok = bare.Lookup(0x10, 0)
	if !ok || v.A != 1 || v.Size != 0 || v.Bitmask != 0 || v.Unit != "" || v.Signed || v.Inverse || v.B != 0 {
		t.Fatalf("bare %+v", v)
	}
	v, ok = bare.Lookup(0x11, 0)
	if !ok || v.A != 0 {
		t.Fatalf("zero %+v", v)
	}
	ratio, err := Parse([]byte("variables:\n- rt: \"0x0020\"\n  name: word\n  factor: 2/0x10000\n- rt: \"0x0021\"\n  name: sixteen\n  factor: 16/0x10000\n- rt: \"0x0022\"\n  name: three\n  factor: -3/4\n"), "t", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, ok = ratio.Lookup(0x20, 0)
	if !ok || v.A != 2.0/65536 {
		t.Fatalf("word %+v", v)
	}
	v, ok = ratio.Lookup(0x21, 0)
	if !ok || v.A != 16.0/65536 {
		t.Fatalf("sixteen %+v", v)
	}
	v, ok = ratio.Lookup(0x22, 0)
	if !ok || v.A != -0.75 {
		t.Fatalf("three %+v", v)
	}
	alias := dir + "/a.yaml"
	if err := os.WriteFile(alias, []byte("aliases:\n- name: nmot\n  alias: EngineSpeed\n  comment: speed\n- name: abo\n  comment: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Aliases(alias)
	if err != nil {
		t.Fatal(err)
	}
	if a["nmot"] != "EngineSpeed" {
		t.Fatal(a)
	}
	if _, ok := a["abo"]; ok {
		t.Fatal("empty alias stored")
	}
}

func TestMergeAnchor(t *testing.T) {
	body := `
scales:
  kw: &kw
    unit: °KW
    signed: true
    factor: 3/4
variables:
- rt: "0x0009"
  name: zwout
  <<: *kw
  comment: hi
- rt: "0x0001"
  name: over
  <<: *kw
  signed: false
  factor: -3/4
`
	tab, err := Parse([]byte(body), "anchor", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	zw, ok := tab.Lookup(9, 0)
	if !ok || zw.Name != "zwout" || zw.Unit != "°KW" || !zw.Signed || zw.A != 0.75 || zw.Comment != "hi" {
		t.Fatalf("%+v", zw)
	}
	over, ok := tab.Lookup(1, 0)
	if !ok || over.Signed || over.A != -0.75 || over.Unit != "°KW" {
		t.Fatalf("%+v", over)
	}
}

func TestNamedScale(t *testing.T) {
	scales, err := ParseScales([]byte(`
scales:
  kw:
    unit: °KW
    signed: true
    factor: 3/4
  kw_word:
    size: 2
    unit: °KW
    factor: 3/16
`), "scales")
	if err != nil {
		t.Fatal(err)
	}
	body := `
variables:
- rt: "0x0009"
  name: zwout
  scale: kw
  comment: hi
- rt: "0x0001"
  name: over
  scale: kw
  signed: false
  factor: -3/4
- rt: "0x0002"
  name: word
  scale: kw_word
- rt: "0x0003"
  name: missing
  scale: no_such
`
	tab, err := Parse([]byte(body), "rows", nil, scales)
	if err == nil || !strings.Contains(err.Error(), "no_such") {
		t.Fatal(err)
	}
	body = strings.Replace(body, "- rt: \"0x0003\"\n  name: missing\n  scale: no_such\n", "", 1)
	tab, err = Parse([]byte(body), "rows", nil, scales)
	if err != nil {
		t.Fatal(err)
	}
	zw, ok := tab.Lookup(9, 0)
	if !ok || zw.Unit != "°KW" || !zw.Signed || zw.A != 0.75 || zw.Size != 0 {
		t.Fatalf("%+v", zw)
	}
	over, ok := tab.Lookup(1, 0)
	if !ok || over.Signed || over.A != -0.75 || over.Unit != "°KW" {
		t.Fatalf("%+v", over)
	}
	word, ok := tab.Lookup(2, 0)
	if !ok || word.Size != 2 || word.A != 3.0/16 || word.Unit != "°KW" {
		t.Fatalf("%+v", word)
	}
}
