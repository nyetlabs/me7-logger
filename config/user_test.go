package config

import (
	"os"
	"path/filepath"
	"testing"

	"me7-logger/mapfile"
	"me7-logger/needle"
)

func TestUserOverlay(t *testing.T) {
	dir := t.TempDir()
	first := `
conversions:
  - size: 1
    unit: custom
    factor: 3
    signed: true
    offset: 1
measurements:
  - name: dzwb
    factor: -0.5
  - name: user_rpm
    size: 1
    unit: rpm
  - name: user_custom
    size: 1
    unit: custom
  - name: kept_factor
    size: 1
    unit: rpm
    factor: 7
  - name: bare_word
    size: 2
  - name: only
  - name: default_press
    unit: mbar
  - name: plain
    size: 2
  - name: temp
    size: 1
    unit: "°C"
  - name: press
    size: 2
    unit: mbar
  - name: nword
    size: 2
    unit: rpm
  - name: air
    size: 2
    unit: g/s
  - name: flag_rpm
    size: 1
    unit: rpm
    bitmask: "0x01"
  - name: flag_plain
    size: 2
    bitmask: "0x0200"
  - name: multi
    size: 1
    unit: rpm
    bitmask: "0x0003"
functions:
  - name: dzwb
    needle_hex: "39 A0 F7 FA ?? ?? 49 A0 DD 02"
    back_up: -8
  - name: user_rpm
    needle_hex: "AA 55 ?? ??"
    back_up: -2
    unique: true
`
	second := `
conversions:
  - size: 1
    unit: rpm
    factor: 10
  - size: 1
    unit: "°KW"
    drop: true
measurements:
  - name: user_rpm
    comment: later
  - name: user_rpm2
    size: 1
    unit: rpm
  - name: bare_kw
    size: 1
    unit: "°KW"
  - name: flag_set
    size: 1
    bitmask: "0x04"
    factor: 2
  - name: plain
    drop: true
functions:
  - name: user_rpm
    unique: false
  - name: slow_init_table
    drop: true
`
	if err := os.WriteFile(filepath.Join(dir, "10-add.yaml"), []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20-override.yaml"), []byte(second), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := LoadNeedles("", "")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := LoadMeasures("", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Measure{}
	for _, m := range ms {
		if _, ok := got[m.Name]; ok {
			t.Fatalf("duplicate %s", m.Name)
		}
		got[m.Name] = m
	}
	dzwb := got["dzwb"]
	if dzwb.A != -0.5 || dzwb.Signed || dzwb.Size != 1 || dzwb.Unit != "°KW" || dzwb.Stub {
		t.Fatalf("dzwb %+v", dzwb)
	}
	rpm := got["user_rpm"]
	if rpm.A != 10 || rpm.Signed || rpm.B != 0 || rpm.Comment != "later" || rpm.Stub {
		t.Fatalf("user_rpm %+v", rpm)
	}
	if got["user_rpm2"].A != 10 || got["user_rpm2"].Signed {
		t.Fatalf("user_rpm2 %+v", got["user_rpm2"])
	}
	custom := got["user_custom"]
	if custom.A != 3 || !custom.Signed || custom.B != 1 {
		t.Fatalf("user_custom %+v", custom)
	}
	if got["kept_factor"].A != 7 {
		t.Fatalf("kept_factor %+v", got["kept_factor"])
	}
	word := got["bare_word"]
	if word.A != 0.25 || word.B != 0 || word.Unit != "" || word.Bitmask != 0 || word.Signed || word.Inverse {
		t.Fatalf("bare_word %+v", word)
	}
	only := got["only"]
	if only.A != 0.25 || only.B != 0 || only.Unit != "" || only.Bitmask != 0 || only.Signed || only.Inverse || only.Size != DefaultSize {
		t.Fatalf("only %+v", only)
	}
	if got["default_press"].Size != DefaultSize || got["default_press"].A != 0.0390625 || got["default_press"].Unit != "mbar" {
		t.Fatalf("default_press %+v", got["default_press"])
	}
	temp := got["temp"]
	if temp.A != 0.75 || temp.B != 48 || temp.Signed {
		t.Fatalf("temp %+v", temp)
	}
	if got["press"].A != 0.0390625 {
		t.Fatalf("press %+v", got["press"])
	}
	if got["nword"].A != 0.25 {
		t.Fatalf("nword %+v", got["nword"])
	}
	if got["air"].A != 1.0/36 {
		t.Fatalf("air %+v", got["air"])
	}
	flag := got["flag_rpm"]
	if flag.A != 1 || flag.Unit != "rpm" || flag.Bitmask != 0x01 {
		t.Fatalf("flag_rpm %+v", flag)
	}
	plain := got["flag_plain"]
	if plain.A != 1 || plain.Unit != "" || plain.Bitmask != 0x0200 {
		t.Fatalf("flag_plain %+v", plain)
	}
	if got["multi"].A != 10 || got["multi"].Bitmask != 0x0003 {
		t.Fatalf("multi %+v", got["multi"])
	}
	kw := got["bare_kw"]
	if kw.A != 1 || kw.Signed || kw.B != 0 || kw.Unit != "°KW" {
		t.Fatalf("bare_kw %+v", kw)
	}
	if got["flag_set"].A != 2 || got["flag_set"].Bitmask != 0x04 {
		t.Fatalf("flag_set %+v", got["flag_set"])
	}
	if _, ok := got["plain"]; ok {
		t.Fatal("dropped name still present")
	}

	ns, err := LoadNeedles("", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := needle.ByName(ns, "slow_init_table"); ok {
		t.Fatal("slow_init_table was not dropped")
	}
	if ns[0].Name != "fast_init_physical" {
		t.Fatalf("first needle %s", ns[0].Name)
	}
	if len(ns) != len(base)+1 {
		t.Fatalf("needles %d base %d", len(ns), len(base))
	}
	dzwbN, ok := needle.ByName(ns, "dzwb")
	if !ok || dzwbN.BackUp != -8 || len(dzwbN.Pattern) != 10 || dzwbN.Pattern[0] != 0x39 {
		t.Fatalf("dzwb needle %+v", dzwbN)
	}
	user, ok := needle.ByName(ns, "user_rpm")
	if !ok || user.Unique || user.BackUp != -2 || len(user.Pattern) != 4 || user.Pattern[0] != 0xAA || user.Mask[2] != 0 {
		t.Fatalf("user_rpm needle %+v", user)
	}
	if ns[len(ns)-1].Name != "user_rpm" {
		t.Fatalf("user_rpm not appended, last is %s", ns[len(ns)-1].Name)
	}
}

func TestUserFileRejectsAddress(t *testing.T) {
	dir := t.TempDir()
	body := "measurements:\n- name: dzwb\n  address: \"0x1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMeasures("", "", dir); err == nil {
		t.Fatal("address was accepted")
	}
}

func TestUserNeedleNeedsPattern(t *testing.T) {
	dir := t.TempDir()
	body := "functions:\n- name: nope\n  unique: true\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNeedles("", dir); err == nil {
		t.Fatal("new needle without needle_hex was accepted")
	}
}

func TestResolveUserDir(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveUserDir(filepath.Join(dir, "missing"), false)
	if err != nil || got != "" {
		t.Fatalf("default missing: %q %v", got, err)
	}
	if _, err := ResolveUserDir(filepath.Join(dir, "missing"), true); err == nil {
		t.Fatal("explicit missing directory was accepted")
	}
	got, err = ResolveUserDir(dir, true)
	if err != nil || got != dir {
		t.Fatalf("existing: %q %v", got, err)
	}
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveUserDir(file, true); err == nil {
		t.Fatal("file was accepted as a user directory")
	}
}

func TestFactorRatio(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"1/256", 1.0 / 256},
		{"10/256", 10.0 / 256},
		{"100/256", 100.0 / 256},
		{"100/0x10000", 100.0 / 0x10000},
		{"1/0x10000", 1.0 / 0x10000},
		{"1/0xffff", 1.0 / 0xffff},
		{"-3/4", -0.75},
		{"1/36", 1.0 / 36},
	}
	for _, c := range cases {
		got, err := mapfile.ParseRatio(c.in)
		if err != nil || got != c.want {
			t.Fatalf("%s: got %v err %v", c.in, got, err)
		}
	}
	dir := t.TempDir()
	body := "measurements:\n- name: byte\n  size: 1\n  factor: 1/256\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := LoadMeasures("", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range ms {
		if m.Name != "byte" {
			continue
		}
		found = true
		if m.A != 1.0/256 {
			t.Fatalf("%v", m.A)
		}
	}
	if !found {
		t.Fatal("byte missing")
	}
}

func TestConversionOmitsSize(t *testing.T) {
	dir := t.TempDir()
	body := "conversions:\n- unit: custom2\n  factor: 7\nmeasurements:\n- name: word\n  unit: custom2\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := LoadMeasures("", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range ms {
		if m.Name != "word" {
			continue
		}
		found = true
		if m.Size != DefaultSize || m.A != 7 {
			t.Fatalf("size %d A %v", m.Size, m.A)
		}
	}
	if !found {
		t.Fatal("word missing")
	}
}

func TestNamedConversionKeepsBlankUnit(t *testing.T) {
	dir := t.TempDir()
	body := "conversions:\n- name: blank\n  unit: \"\"\n  factor: 7\nmeasurements:\n- name: word\n  conversion: blank\n- name: kept\n  conversion: blank\n  unit: \"%\"\n  signed: true\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := LoadMeasures("", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Measure{}
	for _, m := range ms {
		got[m.Name] = m
	}
	word := got["word"]
	if word.Size != DefaultSize || word.Unit != "" || word.A != 7 {
		t.Fatalf("%+v", word)
	}
	kept := got["kept"]
	if kept.Unit != "%" || !kept.Signed || kept.A != 7 {
		t.Fatalf("%+v", kept)
	}
	drop := "conversions:\n- name: torque\n  drop: true\n"
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(drop), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMeasures("", "", dir); err == nil {
		t.Fatal("dropped torque was still applied")
	}
}
