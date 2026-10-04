package opcode

import "testing"

func TestParseSigRequiresName(t *testing.T) {
	if _, err := ParseSigs([]byte("signatures:\n- size: 1\n  pattern: AA\n")); err == nil {
		t.Fatal("accepted a row without a name")
	}
}

func TestApplySigSkipsMissingEmbed(t *testing.T) {
	img := make([]byte, 0x4200)
	doc, err := ParseSigs([]byte("signatures:\n- name: got\n  size: 1\n  pattern: \"F2FXxxxx{rk_w:F2FX}1BXX\"\n  at: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if ApplySigs(img, StandardDPP, nil, doc) != nil {
		t.Fatal("a missing embedded name stored a row")
	}
}

func TestApplySigAfterWindow(t *testing.T) {
	// rk_w is 0x380100, so {rk_w:F2FX} is F2FX0081.
	// The follow-up row starts after that hit and stops at DB00.
	body := []byte(`
signatures:
- name: first
  size: 1
  pattern: "F2FXxxxx{rk_w:F2FX}1BXX"
  at: 2
- name: second
  size: 1
  after: first
  pattern: "F6FXxxxx"
  at: 2
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x4200)
	copy(img[0x4000:], []byte{
		0xF2, 0xF4, 0x02, 0x81, 0xF2, 0xF4, 0x00, 0x81, 0x1B, 0x00,
		0xF6, 0xF4, 0x04, 0x81,
		0xDB, 0x00,
		0xF6, 0xF4, 0x06, 0x81,
	})
	got := ApplySigs(img, StandardDPP, map[string]uint32{"rk_w": 0x380100}, doc)
	var first, second Named
	for _, h := range got {
		switch h.Name {
		case "first":
			first = h
		case "second":
			second = h
		}
	}
	if first.Addr != 0x380102 || first.Size != 1 || second.Addr != 0x380104 || second.Size != 1 {
		t.Fatalf("%v", got)
	}
}

func TestApplySigSteps(t *testing.T) {
	body := []byte(`
signatures:
- name: got
  size: 1
  at: 2
  steps:
  - pattern: "11XX"
  - pattern: "22XX"
    skip: 4
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x4200)
	copy(img[0x4000:], []byte{0x11, 0x00, 0x00, 0x00, 0x22, 0x00, 0x02, 0x81})
	got := ApplySigs(img, StandardDPP, nil, doc)
	if len(got) != 1 || got[0].Name != "got" || got[0].Addr != 0x380102 {
		t.Fatalf("%v", got)
	}
}

func TestApplySigSlot(t *testing.T) {
	body := []byte(`
calls:
  "1":
    "0": "0x1234"
    "402": "0x0000"
    "602": "0x0000"
signatures:
- name: got
  size: 1
  pattern: "{slot:1}"
  at: 6
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x4200)
	copy(img[0x4000:], []byte{0xDA, 0x00, 0x34, 0x12, 0xF2, 0xF0, 0x02, 0x81})
	got := ApplySigs(img, StandardDPP, nil, doc)
	if len(got) != 1 || got[0].Addr != 0x380102 {
		t.Fatalf("%v", got)
	}
}

func TestApplySigSingle(t *testing.T) {
	doc, err := ParseSigs([]byte("signatures:\n- name: got\n  size: 1\n  pattern: CCDD\n  single: true\n  at: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x4200)
	copy(img[0x4000:], []byte{0xCC, 0xDD, 0x02, 0x81})
	copy(img[0x4010:], []byte{0xCC, 0xDD, 0x04, 0x81})
	if ApplySigs(img, StandardDPP, nil, doc) != nil {
		t.Fatal("a second copy stored a row")
	}
}

func TestApplySigNegativeAt(t *testing.T) {
	doc, err := ParseSigs([]byte("signatures:\n- name: got\n  size: 2\n  pattern: CCDD\n  at: -4\n"))
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x4200)
	copy(img[0x4000:], []byte{0x02, 0x81, 0xAA, 0xBB, 0xCC, 0xDD})
	got := ApplySigs(img, StandardDPP, nil, doc)
	if len(got) != 1 || got[0].Addr != 0x380102 || got[0].Size != 2 {
		t.Fatalf("%v", got)
	}
}

func TestApplySigAdd(t *testing.T) {
	body := []byte(`
signatures:
- name: base
  size: 1
  pattern: "F2F40081"
  at: 2
  also:
  - name: next
    size: 1
    add: 1
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x4200)
	copy(img[0x4000:], []byte{0xF2, 0xF4, 0x00, 0x81})
	got := ApplySigs(img, StandardDPP, nil, doc)
	have := map[string]uint32{}
	for _, h := range got {
		have[h.Name] = h.Addr
	}
	if have["base"] != 0x380100 || have["next"] != 0x380101 {
		t.Fatalf("%v", got)
	}
}

func TestMapSigPatternAndAnchor(t *testing.T) {
	body := []byte(`
mapsigs:
- name: BASE
  pattern: "D74000020000XXXX"
  at: 6
- name: NEXT
  anchor: BASE
  add: 2
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 32)
	copy(img, []byte{0xD7, 0x40, 0x00, 0x02, 0x00, 0x00, 0x10, 0x00})
	got := MapAddrs(img, StandardDPP, doc.Maps)
	have := map[string]uint32{}
	for _, h := range got {
		have[h.Name] = h.Addr
	}
	if have["BASE"] != 0x800010 || have["NEXT"] != 0x800012 {
		t.Fatalf("%+v", got)
	}
	img[16] = 0xD7
	copy(img[16:], []byte{0xD7, 0x40, 0x00, 0x02, 0x00, 0x00, 0x10, 0x00})
	if MapAddrs(img, StandardDPP, doc.Maps) != nil {
		t.Fatal("a second copy of the window stored a map")
	}
}

func TestMapSigLaterRowFillsAMiss(t *testing.T) {
	body := []byte(`
mapsigs:
- name: BASE
  pattern: "D74000020000XXXX"
  at: 6
- name: BASE
  pattern: "D7400002AABBXXXX"
  at: 6
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 32)
	copy(img[8:], []byte{0xD7, 0x40, 0x00, 0x02, 0xAA, 0xBB, 0x10, 0x00})
	got := MapAddrs(img, StandardDPP, doc.Maps)
	if len(got) != 1 || got[0].Name != "BASE" || got[0].Addr != 0x800010 {
		t.Fatalf("%+v", got)
	}
}

func TestMapSigRamWord(t *testing.T) {
	body := []byte(`
mapsigs:
- name: BASE
  pattern: "D74000020000XXXX"
  at: 6
  xat: 10
  yat: 14
- name: NEXT
  anchor: BASE
  add: 2
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 32)
	copy(img, []byte{
		0xD7, 0x40, 0x00, 0x02, 0x00, 0x00, 0x10, 0x00,
		0xF2, 0xF4, 0x68, 0x8F,
		0xF2, 0xF5, 0x66, 0x8F,
	})
	got := MapAddrs(img, StandardDPP, doc.Maps)
	if len(got) != 2 || got[0].Addr != 0x800010 || got[0].XRam != 0x8F68 || got[0].YRam != 0x8F66 {
		t.Fatalf("base %+v", got)
	}
	if got[1].Addr != 0x800012 || got[1].XRam != 0x8F68 || got[1].YRam != 0x8F66 {
		t.Fatalf("anchor %+v", got[1])
	}
}

func TestMapSigShape(t *testing.T) {
	body := []byte(`
mapsigs:
- name: BASE
  pattern: "D74000020000XXXX"
  at: 6
  cols: 4
  rows: 2
`)
	doc, err := ParseSigs(body)
	if err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 32)
	copy(img, []byte{0xD7, 0x40, 0x00, 0x02, 0x00, 0x00, 0x10, 0x00})
	got := MapAddrs(img, StandardDPP, doc.Maps)
	if len(got) != 1 || got[0].Cols != 4 || got[0].Rows != 2 || got[0].XBits != 8 || got[0].YBits != 8 || got[0].Addr != 0x800010 {
		t.Fatalf("%+v", got)
	}
	omitted, err := ParseSigs([]byte("mapsigs:\n- name: A\n  pattern: \"0000XXXX\"\n  cols: 4\n"))
	if err != nil || omitted.Maps[0].At != 2 || omitted.Maps[0].XBits != 8 {
		t.Fatalf("%v %+v", err, omitted.Maps)
	}
	kept, err := ParseSigs([]byte("mapsigs:\n- name: A\n  pattern: XXXX\n  at: 0\n  cols: 4\n  xbits: 16\n"))
	if err != nil || kept.Maps[0].At != 0 || kept.Maps[0].XBits != 16 {
		t.Fatalf("%v %+v", err, kept.Maps)
	}
	if _, err := ParseSigs([]byte("mapsigs:\n- name: A\n  pattern: D74000020000XXXX\n  at: 6\n  cols: 4\n  xbits: 12\n")); err == nil {
		t.Fatal("accepted a width that is not 8 or 16")
	}
}

func TestMapSigRejects(t *testing.T) {
	if _, err := ParseSigs([]byte("mapsigs:\n- name: A\n  anchor: B\n")); err == nil {
		t.Fatal("accepted an anchor with no add")
	}
	if _, err := ParseSigs([]byte("mapsigs:\n- name: A\n  pattern: AA\n  anchor: B\n  add: 1\n")); err == nil {
		t.Fatal("accepted a pattern and an anchor")
	}
}
