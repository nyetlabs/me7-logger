package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSelector(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, "core.yaml")
	body := "functions:\n  - name: meas\n    needle_hex: \"D440120066F40F00\"\n    unique: true\n"
	if err := os.WriteFile(yml, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "m.yaml")
	mapBody := "variables:\n- rt: \"0x0001\"\n  bitmask: \"0x00\"\n  name: nmot\n  size: 0\n  unit: rpm\n  factor: 40\n  offset: 0\n  comment: speed\n"
	if err := os.WriteFile(mapPath, []byte(mapBody), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(alias, []byte("aliases:\n- name: nmot\n  alias: EngineSpeed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x46, 0xF4, 0x01, 0x00, 0x2D, 9})
	copy(img[14:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 9})
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	res, err := Generate(Options{
		Image: img, ImageName: "t.bin", CorePath: yml, MapPath: mapPath, AliasPath: alias,
		Scale: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.File.Connect != "" {
		t.Fatalf("connect %q", res.File.Connect)
	}
	if len(res.File.Items) != 1 || res.File.Items[0].Name != "nmot" || res.File.Items[0].Addr != 0x380100 {
		t.Fatalf("%+v", res.File.Items)
	}
	if res.File.Items[0].Alias != "EngineSpeed" || res.File.Items[0].A != 40 {
		t.Fatalf("%+v", res.File.Items[0])
	}
	text := string(res.File.Bytes())
	if strings.Contains(text, "Connect      = SLOW") {
		t.Fatal("generate set Connect without a table needle")
	}
	if !strings.Contains(text, "nmot") {
		t.Fatal(text)
	}
}

func TestConnectNeedles(t *testing.T) {
	img := make([]byte, 0x140)
	// A later KWP row (address 0xFE) must not be the table start.
	copy(img[0x40:], []byte{0xFE, 0x55, 0xEF, 0x8F, 0x70, 0x00, 0x01, 0x01, 0x3C, 0x00, 0x2C, 0x01})
	img[0x40+31] = 0x55
	copy(img[0x80:], []byte{
		0x11, 0x55, 0xEF, 0x8F, 0x70, 0x00, 0xEE, 0x01, 0x3C, 0x00, 0x2C, 0x01,
		0x05, 0x00, 0x14, 0x00, 0x00, 0x00, 0x14, 0x00, 0x19, 0x00, 0x32, 0x00,
		0x47, 0x00, 0xC8, 0x02, 0x82, 0x00,
		0x33, 0x55,
	})
	copy(img[0x100:], []byte{
		0xE6, 0xF5, 0x06, 0x02, 0xF6, 0xF4, 0x40, 0xE2, 0xF6, 0xF5, 0x42, 0xE2,
		0xE7, 0xF8, 0x10, 0x00, 0xF7, 0xF8, 0x08, 0xE2,
	})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if res.File.Connect != "SLOW-0x11" || res.File.Fast != 0x10 {
		t.Fatalf("connect %q fast %X", res.File.Connect, res.File.Fast)
	}
}

func TestResultSelectorNeedle(t *testing.T) {
	img := make([]byte, 0x80)
	copy(img, []byte{0xE0, 0x09, 0xE6, 0xF8, 0x10, 0x00, 0xE1, 0x0E, 0xE1, 0x0C, 0xE0, 0x04})
	copy(img[38:], []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[46:], []byte{0x48, 0x41, 0x2D, 23})
	copy(img[50:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 23})
	// JMPR at file 48. Target is 50+rel*2; rel 23 lands at 0x60.
	copy(img[0x60:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range res.File.Items {
		if it.Name != "nmot" {
			continue
		}
		found = true
		if it.Addr != 0x380100 || it.A != 40 || it.Size != 2 || it.ResultType != 1 {
			t.Fatalf("%+v", it)
		}
	}
	if !found {
		t.Fatalf("%+v", res.File.Items)
	}
}

func TestNmotNeedleJoinsCase(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "m.yaml")
	// No shipped needle for this name. The catalog row is the measurement.
	mapBody := "variables:\n- rt: \"0x0001\"\n  name: nmot\n  unit: rpm\n  factor: 99\n  comment: catalog\n"
	if err := os.WriteFile(mapPath, []byte(mapBody), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x80)
	copy(img, []byte{0xE0, 0x09, 0xE6, 0xF8, 0x10, 0x00, 0xE1, 0x0E, 0xE1, 0x0C, 0xE0, 0x04})
	copy(img[38:], []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[46:], []byte{0x48, 0x41, 0x2D, 23})
	copy(img[50:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 23})
	copy(img[0x60:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", MapPath: mapPath, Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, it := range res.File.Items {
		if it.Name != "nmot" {
			continue
		}
		n++
		if it.Addr != 0x380100 || it.A != 99 || it.Size != 2 || it.ResultType != 1 || it.Unit != "rpm" || it.Bitmask != 0 || it.Alias != "RPM_8" || it.Comment != "catalog" {
			t.Fatalf("%+v", it)
		}
	}
	if n != 1 {
		t.Fatalf("nmot rows %d %+v", n, res.File.Items)
	}
}

func TestBitRowsShareCaseAddress(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "core.yaml")
	if err := os.WriteFile(core, []byte("functions:\n  - name: meas\n    needle_hex: \"D440120066F40F00\"\n    unique: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "m.yaml")
	mapBody := "variables:\n- rt: \"0x0001\"\n  bitmask: \"0x02\"\n  name: nmot_bit\n  unit: rpm\n  factor: 40\n- rt: \"0x0001\"\n  bitmask: \"0x01\"\n  name: tmot_bit\n  unit: rpm\n  factor: 40\n"
	if err := os.WriteFile(mapPath, []byte(mapBody), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x46, 0xF4, 0x01, 0x00, 0x2D, 9})
	copy(img[14:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 9})
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", CorePath: core, MapPath: mapPath, Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.File.Items) != 0 {
		t.Fatalf("bit rows %d", len(res.File.Items))
	}
	// A single-bit AND is not a template, so the bit row stays unwritten.
	copy(img[0x20:], []byte{0x66, 0xF4, 0x02, 0x00, 0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	res, err = Generate(Options{Image: img, ImageName: "t.bin", CorePath: core, MapPath: mapPath, Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.File.Items) != 0 {
		t.Fatalf("%+v", res.File.Items)
	}
}

func TestSFRBitRow(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "core.yaml")
	if err := os.WriteFile(core, []byte("functions:\n  - name: meas\n    needle_hex: \"D440120066F40F00\"\n    unique: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "m.yaml")
	if err := os.WriteFile(mapPath, []byte("variables:\n- rt: \"0x0001\"\n  bitmask: \"0x01\"\n  name: B_hsve\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x46, 0xF4, 0x01, 0x00, 0x2D, 9})
	copy(img[14:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 16})
	copy(img[0x20:], []byte{0x9A, 0x20, 0x03, 0xC0, 0xE6, 0xF4, 0x00, 0x81, 0x0D, 0x02, 0x00, 0x00})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", CorePath: core, MapPath: mapPath, Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.File.Items) != 1 || res.File.Items[0].Name != "B_hsve" || res.File.Items[0].Addr != 0xFD40 || res.File.Items[0].Size != 2 || res.File.Items[0].Bitmask != 0x1000 {
		t.Fatalf("%+v", res.File.Items)
	}
}

func TestUserNeedle(t *testing.T) {
	dir := t.TempDir()
	body := "functions:\n  - name: dzwb\n    needle_hex: \"39 A0 F7 FA ?? ?? 49 A0 DD 02\"\n    back_up: -4\n"
	if err := os.WriteFile(filepath.Join(dir, "dzwb.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 16)
	copy(img, []byte{0x39, 0xA0, 0xF7, 0xFA, 0xAD, 0x8C, 0x49, 0xA0, 0xDD, 0x02})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off", UserDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.File.Items {
		if it.Name != "dzwb" {
			continue
		}
		if it.Addr != 0x380CAD || it.Size != 1 || it.A != -0.75 || !it.Signed {
			t.Fatalf("%+v", it)
		}
		return
	}
	t.Fatal("dzwb missing")
}

func TestUserConversion(t *testing.T) {
	dir := t.TempDir()
	body := `
measurements:
  - name: dzwb
    factor: -0.5
  - name: user_rpm
    size: 1
    unit: rpm
  - name: user_flag
    size: 1
    bitmask: "0x01"
functions:
  - name: dzwb
    needle_hex: "39 A0 F7 FA ?? ?? 49 A0 DD 02"
    back_up: -4
  - name: user_rpm
    needle_hex: "AA 55 BB 66 ?? ?? CC 11 DD 22"
    back_up: -4
    unique: true
  - name: user_flag
    needle_hex: "11 22 33 44 ?? ?? 55 66 77 88"
    back_up: -4
    unique: true
`
	if err := os.WriteFile(filepath.Join(dir, "10-user.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 48)
	copy(img, []byte{0x39, 0xA0, 0xF7, 0xFA, 0xAD, 0x8C, 0x49, 0xA0, 0xDD, 0x02})
	copy(img[16:], []byte{0xAA, 0x55, 0xBB, 0x66, 0x00, 0x10, 0xCC, 0x11, 0xDD, 0x22})
	copy(img[32:], []byte{0x11, 0x22, 0x33, 0x44, 0x02, 0x00, 0x55, 0x66, 0x77, 0x88})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off", UserDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var dzwb, rpm, flag bool
	for _, it := range res.File.Items {
		switch it.Name {
		case "dzwb":
			dzwb = true
			if it.Addr != 0x380CAD || it.Size != 1 || it.A != -0.5 || !it.Signed {
				t.Fatalf("%+v", it)
			}
		case "user_rpm":
			rpm = true
			if it.Addr != 0x811000 || it.Size != 1 || it.A != 40 || it.Unit != "rpm" || it.Signed {
				t.Fatalf("%+v", it)
			}
		case "user_flag":
			flag = true
			if it.Addr != 0x810002 || it.A != 1 || it.Unit != "" || it.Bitmask != 0x01 {
				t.Fatalf("%+v", it)
			}
		}
	}
	if !dzwb || !rpm || !flag {
		t.Fatalf("dzwb %v rpm %v flag %v", dzwb, rpm, flag)
	}
}

func TestStubsAreNotWritten(t *testing.T) {
	img := make([]byte, 36)
	copy(img[0:12], []byte("8D0907551M  "))
	copy(img[12:32], []byte("2.7l V6/5VT         "))
	copy(img[32:36], []byte("0002"))
	res, err := Generate(Options{Image: img, ImageName: "8D0907551M.bin", Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.File.Items {
		if it.Name == "dzwb" || it.Name == "mimax_w" {
			t.Fatalf("stub written: %+v", it)
		}
	}
	if !strings.Contains(res.StubNote, "stubs not written") {
		t.Fatal(res.StubNote)
	}
}

func TestInterpCall(t *testing.T) {
	img := make([]byte, 0x10400)
	copy(img[0x40:], []byte{
		0x26, 0xF4, 0x00, 0x80, 0x8D, 0x04, 0x7D, 0x06,
		0xE6, 0xF4, 0xFF, 0x7F, 0x0D, 0x03, 0x6D, 0x02,
		0xE6, 0xF4, 0x00, 0x80,
	})
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	const at = 0xA0
	copy(img[at:], []byte{
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x10, 0x01,
		0xF2, 0xFE, 0x00, 0x00,
		0xF2, 0xFF, 0x00, 0x00,
		0xDA, 0x00, 0x40, 0x00,
	})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Maps) != 1 {
		t.Fatalf("%d maps", len(res.Maps))
	}
	m := res.Maps[0]
	if m.Name != "" || m.Addr != 0x810200 || m.Cols != 4 || m.Bits != 0 {
		t.Fatalf("%+v", m)
	}
	if m.X == nil || m.X.Addr != 0x810111 || m.X.Count != 4 || m.X.Bits != 8 {
		t.Fatalf("%+v", m.X)
	}
	if !strings.Contains(res.MapNote, "1 calibration map located") {
		t.Fatal(res.MapNote)
	}
}

func TestNoSerialImport(t *testing.T) {
	b, err := os.ReadFile("generate.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "\"me7-logger/kwp\"") || strings.Contains(s, "go.bug.st/serial") {
		t.Fatal("generate must not open a serial port")
	}
}
