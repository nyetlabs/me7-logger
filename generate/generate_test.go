package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/me7-logger/internal/heximg"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
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
	img := readHex(t, "walk-selector.hex")
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
	img := readHex(t, "connect.hex")
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if res.File.Connect != "SLOW-0x11" || res.File.Fast != 0x10 {
		t.Fatalf("connect %q fast %X", res.File.Connect, res.File.Fast)
	}
}

func TestResultSelectorNeedle(t *testing.T) {
	img := readHex(t, "result-selector.hex")
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
	img := readHex(t, "result-selector.hex")
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
	img := readHex(t, "walk-selector.hex")
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
	img := readHex(t, "sfr-bit.hex")
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
	body := "measurements:\n  - name: dzwb\n    needle_hex: \"39 A0 F7 FA ?? ?? 49 A0 DD 02\"\n    back_up: -4\n"
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

func TestMeasureBitWord(t *testing.T) {
	dir := t.TempDir()
	body := "measurements:\n  - name: B_ar\n    bitmask: \"0x0200\"\n    bit: true\n    needle_hex: \"F0 94 9A ?? 04 50\"\n    back_up: -2\n"
	if err := os.WriteFile(filepath.Join(dir, "bit.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 16)
	copy(img, []byte{0xF0, 0x94, 0x9A, 0x4E, 0x04, 0x50})
	res, err := Generate(Options{Image: img, ImageName: "t.bin", Scale: "off", UserDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.File.Items {
		if it.Name != "B_ar" {
			continue
		}
		if it.Addr != 0xFD9C || it.Bitmask != 0x0200 || it.Size != 2 {
			t.Fatalf("%+v", it)
		}
		return
	}
	t.Fatal("B_ar missing")
}

func TestUserConversion(t *testing.T) {
	dir := t.TempDir()
	body := `
measurements:
  - name: dzwb
    factor: -0.5
    needle_hex: "39 A0 F7 FA ?? ?? 49 A0 DD 02"
    back_up: -4
  - name: user_rpm
    size: 1
    unit: rpm
    needle_hex: "AA 55 BB 66 ?? ?? CC 11 DD 22"
    back_up: -4
    unique: true
  - name: user_flag
    size: 1
    bitmask: "0x01"
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
	img := readHex(t, "interp.hex")
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

func TestCallTableFillsUnsetRow(t *testing.T) {
	img := make([]byte, 32)
	copy(img[4:], []byte{6, 1, 2, 3, 4, 5, 6})
	maps := []record.Map{{Name: "KFDMDADP", Addr: 0x800010, Cols: 8}}
	calls := []record.Call{{Name: "KFDMDADP", Rows: 6, YBits: 8, YTable: "SGA06MDUB"}}
	tabs := map[string]uint32{"SGA06MDUB": opcode.FlashBase + 4}
	applyCallTables(img, maps, calls, tabs)
	if maps[0].Y == nil || maps[0].Y.Addr != opcode.FlashBase+5 || maps[0].Y.Count != 6 || maps[0].Y.Bits != 8 || maps[0].Rows != 6 {
		t.Fatalf("%+v", maps[0].Y)
	}
	maps[0].Y = &record.Axis{Addr: 0x800001, Count: 6, Bits: 8}
	applyCallTables(img, maps, calls, tabs)
	if maps[0].Y.Addr != 0x800001 {
		t.Fatalf("%+v", maps[0].Y)
	}
}

func TestNoSerialImport(t *testing.T) {
	b, err := os.ReadFile("generate.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "\"go.nyet.org/me7-logger/kwp\"") || strings.Contains(s, "go.bug.st/serial") {
		t.Fatal("generate must not open a serial port")
	}
}

func readHex(t *testing.T, name string) []byte {
	t.Helper()
	img, err := heximg.Read(filepath.Join("..", "testdata", "opcode", name))
	if err != nil {
		t.Fatal(err)
	}
	return img
}
