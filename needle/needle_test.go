package needle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchWildcardAndMask(t *testing.T) {
	pat, mask, err := parsePattern("AA ?? BB")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte{0xaa, 0x01, 0xbb, 0xaa, 0xaa, 0x02, 0xbb}
	n := Needle{Pattern: pat, Mask: mask}
	if got := n.Find(data); !sameInts(got, []int{0, 4}) {
		t.Fatalf("wildcard hits %v", got)
	}
	pat, mask, err = parsePattern("aa80")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseHex("fffc")
	if err != nil {
		t.Fatal(err)
	}
	for i := range mask {
		mask[i] &= m[i]
	}
	n = Needle{Pattern: pat, Mask: mask}
	data = []byte{0xaa, 0x83, 0x00, 0xaa, 0x84, 0xaa, 0x80}
	// Word-aligned only: offset 0 (aa 83) and offset 4 would be 84 aa, offset 5 is odd.
	// 0: aa 83 matches aa with mask ff and 80 with mask fc (0x83&0xfc==0x80).
	// 4: 84 aa — 0x84&0xfc==0x80, second byte 0xaa != 0x00. No.
	// 6: 80 then past end. Pattern length 2, offset 6 is the last byte only.
	// Offset 0 only? Python finditer on b"\xaa\x83\x00\xaa\x84\xaa\x80" returns [0, 5].
	// Offset 5 is odd and filtered by find(). So word-aligned result is [0].
	// The python test checks the regex finditer BEFORE the even filter:
	// assert finditer == [0, 5]. Our Find filters to even. Offset 5 is not a hit.
	if got := n.Find(data); !sameInts(got, []int{0}) {
		t.Fatalf("mask hits %v", got)
	}
	// Unfiltered scan includes the odd hit, matching compile_needle.finditer.
	var all []int
	for i := 0; i+len(pat) <= len(data); i++ {
		if matchAt(data, pat, mask, i) {
			all = append(all, i)
		}
	}
	if !sameInts(all, []int{0, 5}) {
		t.Fatalf("unaligned mask hits %v", all)
	}
}

func TestBackUpRange(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, "p.yaml")
	body := "functions:\n  - name: f\n    needle_hex: \"AA BB\"\n    back_up: [0x4, 0xC]\n    entry_after: [\"DB 00\"]\n"
	if err := os.WriteFile(yml, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(yml)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := Parse(raw, yml)
	if err != nil {
		t.Fatal(err)
	}
	n := ns[0]
	data := make([]byte, 0x20)
	data[0x10], data[0x11] = 0xaa, 0xbb
	data[0x04], data[0x05] = 0xdb, 0x00
	data[0x08], data[0x09] = 0xdb, 0x00
	hits := n.Find(data)
	var labels []int
	for _, h := range hits {
		labels = append(labels, n.Label(data, h))
	}
	if !sameInts(labels, []int{0x0A}) {
		t.Fatalf("labels %v", labels)
	}
	data[0x08], data[0x09] = 0, 0
	if n.Label(data, 0x10) != 0x06 {
		t.Fatalf("label %x", n.Label(data, 0x10))
	}
	data[0x04], data[0x05] = 0, 0
	if n.EntryOK(data, n.Label(data, 0x10)) {
		t.Fatal("expected entry miss")
	}
	body = strings.Replace(body, "    entry_after: [\"DB 00\"]\n", "", 1)
	if err := os.WriteFile(yml, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(yml)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw, yml); err == nil {
		t.Fatal("range without entry_after should fail")
	}
}

func TestApplyOverlay(t *testing.T) {
	base, err := Parse([]byte("functions:\n- name: a\n  needle_hex: \"AA BB\"\n  back_up: 4\n  unique: true\n"), "t")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyOverlay(base, []byte("measurements:\n- name: x\n  size: 1\n"), "u")
	if err != nil || len(got) != 1 || got[0].BackUp != 4 {
		t.Fatalf("%v %+v", err, got)
	}
	got, err = ApplyOverlay(base, []byte("functions:\n- name: a\n  back_up: -2\n  mask_hex: \"FF 00\"\n"), "u")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].BackUp != -2 || !got[0].Unique || got[0].Mask[0] != 0xFF || got[0].Mask[1] != 0 || got[0].Pattern[0] != 0xAA {
		t.Fatalf("%+v", got[0])
	}
	if _, err := ApplyOverlay(base, []byte("functions:\n- name: nope\n  unique: true\n"), "u"); err == nil {
		t.Fatal("new needle without needle_hex was accepted")
	}
	bare, err := Parse([]byte("functions:\n- name: bare\n  needle_hex: \"AA\"\n- name: plain\n  needle_hex: \"BB\"\n  unique: false\n"), "t")
	if err != nil {
		t.Fatal(err)
	}
	if bare[0].BackUp != DefaultBackUp || !bare[0].Unique || bare[1].BackUp != 0 || bare[1].Unique {
		t.Fatalf("%+v %+v", bare[0], bare[1])
	}
	got, err = ApplyOverlay(base, []byte("functions:\n- name: a\n  drop: true\n"), "u")
	if err != nil || len(got) != 0 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestNeedleList(t *testing.T) {
	raw := []byte(`
functions:
  - name: f
    needles:
      - needle_hex: "AA BB"
        unique: false
      - needle_hex: "CC DD"
`)
	ns, err := Parse(raw, "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 2 || ns[0].Name != "f" || ns[0].Unique || !ns[1].Unique || ns[1].Pattern[0] != 0xCC {
		t.Fatalf("%+v %+v", ns[0], ns[1])
	}
	if _, err := Parse([]byte("functions:\n- name: f\n  needle_hex: AA\n  needles:\n  - needle_hex: BB\n"), "t"); err == nil {
		t.Fatal("accepted needle_hex and needles")
	}
	base, err := Parse([]byte("functions:\n- name: f\n  needle_hex: \"AA BB\"\n- name: g\n  needle_hex: \"11 22\"\n"), "t")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyOverlay(base, []byte("functions:\n- name: f\n  needles:\n  - needle_hex: \"CC\"\n  - needle_hex: \"DD\"\n    unique: false\n"), "u")
	if err != nil || len(got) != 3 || got[0].Pattern[0] != 0xCC || got[1].Unique || got[2].Name != "g" {
		t.Fatalf("%v %+v", err, got)
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
