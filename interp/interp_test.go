package interp

import (
	"testing"

	"go.nyet.org/me7-logger/needle"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

func TestLocateMapFromCall(t *testing.T) {
	const (
		entry  = 0x40
		entryB = 0x60
		sib    = 0x20
	)
	img := make([]byte, 0x10400)
	pro := []byte{
		0x26, 0xF4, 0x00, 0x80, 0x8D, 0x04, 0x7D, 0x06,
		0xE6, 0xF4, 0xFF, 0x7F, 0x0D, 0x03, 0x6D, 0x02,
		0xE6, 0xF4, 0x00, 0x80,
	}
	proB := []byte{0xE4, 0x8F, 0xF6, 0x8E, 0xDE, 0x8F, 0xF6, 0x8E, 0x06, 0x90, 0xF6, 0x8E}
	copy(img[sib:], pro)
	copy(img[entry:], pro)
	copy(img[entryB:], proB)
	// 8-bit axis: count 4, then the breakpoints.
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	// 16-bit axis: count, a zero marker, then values.
	copy(img[0x10120:], []byte{4, 0, 0x10, 0, 0x20, 0, 0x30, 0, 0x40, 0})
	putCall(img, 0xA0, entry, 0x0200, 0x0110)
	putCall(img, 0xC0, entry, 0x0200, 0x0110)
	putCall(img, 0xE0, entryB, 0x0300, 0x0120)
	// A call that does not load R12 is not a map.
	img[0x120] = 0xDA
	img[0x122] = byte(entry)
	img[0x123] = byte(entry >> 8)

	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8
    needle_hex: "26 F4 00 80 8D 04 7D 06 E6 F4 FF 7F 0D 03 6D 02 E6 F4 00 80"
    unique: false
  - name: map_interp_table8_b
    needle_hex: "E4 8F F6 8E DE 8F F6 8E 06 90 F6 8E"
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 2 {
		t.Fatalf("%d maps", len(got))
	}
	if got[0].Name != "" || got[0].Addr != 0x810200 || got[0].Bits != 0 || got[0].Cols != 4 || got[0].Comment != Byte {
		t.Fatalf("%+v", got[0])
	}
	if got[0].X == nil || got[0].X.Addr != 0x810111 || got[0].X.Count != 4 || got[0].X.Bits != 8 || got[0].Y != nil {
		t.Fatalf("%+v", got[0])
	}
	if got[1].Addr != 0x810300 || got[1].Cols != 4 || got[1].Comment != ByteB {
		t.Fatalf("%+v", got[1])
	}
	if got[1].X == nil || got[1].X.Addr != 0x810122 || got[1].X.Bits != 16 || got[1].X.Count != 4 || got[1].Y != nil {
		t.Fatalf("%+v", got[1])
	}
}

func TestRowAxis(t *testing.T) {
	const entry = 0x40
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{
		0x26, 0xF4, 0x00, 0x80, 0x8D, 0x04, 0x7D, 0x06,
		0xE6, 0xF4, 0xFF, 0x7F, 0x0D, 0x03, 0x6D, 0x02,
		0xE6, 0xF4, 0x00, 0x80,
	})
	// Column header: count 4, then the breakpoints. R13 is 0x0110.
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	// Row header: count 16, then the breakpoints. The setup's R12 is 0x00C0.
	row := make([]byte, 17)
	row[0] = 16
	row[1] = 0x0B
	copy(img[0x100C0:], row)
	// R14's setup stores the row header. R15's setup stores the column header.
	copy(img[0x300:], []byte{
		0xE6, 0xFC, 0xC0, 0x00,
		0xC2, 0xFD, 0x00, 0x00,
		0xF2, 0xFE, 0x46, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x46, 0x8F,
	})
	copy(img[0x320:], []byte{
		0xE6, 0xFC, 0x10, 0x01,
		0xC2, 0xFD, 0x00, 0x00,
		0xF2, 0xFF, 0x54, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x54, 0x8F,
	})
	copy(img[0x400:], []byte{
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x10, 0x01,
		0xF2, 0xFE, 0x46, 0x8F,
		0xF2, 0xFF, 0x54, 0x8F,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8
    needle_hex: "26 F4 00 80 8D 04 7D 06 E6 F4 FF 7F 0D 03 6D 02 E6 F4 00 80"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 1 {
		t.Fatalf("%d maps", len(got))
	}
	if got[0].Addr != 0x810200 || got[0].Cols != 4 || got[0].Rows != 16 {
		t.Fatalf("%+v", got[0])
	}
	if got[0].X == nil || got[0].X.Addr != 0x810111 || got[0].X.Count != 4 || got[0].X.Bits != 8 {
		t.Fatalf("col %+v", got[0].X)
	}
	if got[0].Y == nil || got[0].Y.Addr != 0x8100C1 || got[0].Y.Count != 16 || got[0].Y.Bits != 8 || got[0].Y.Equation != "" {
		t.Fatalf("row %+v", got[0].Y)
	}
}

func TestRowAxisF2(t *testing.T) {
	const entry = 0x40
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{
		0x26, 0xF4, 0x00, 0x80, 0x8D, 0x04, 0x7D, 0x06,
		0xE6, 0xF4, 0xFF, 0x7F, 0x0D, 0x03, 0x6D, 0x02,
		0xE6, 0xF4, 0x00, 0x80,
	})
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	row := make([]byte, 17)
	row[0] = 16
	row[1] = 0x0B
	copy(img[0x100C0:], row)
	// F2 of R13 sits between the header immediate and the reload.
	copy(img[0x300:], []byte{
		0xE6, 0xFC, 0xC0, 0x00,
		0xF2, 0xFD, 0x78, 0xF8,
		0xF2, 0xFE, 0x46, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x46, 0x8F,
	})
	copy(img[0x400:], []byte{
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x10, 0x01,
		0xF2, 0xFE, 0x46, 0x8F,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8
    needle_hex: "26 F4 00 80 8D 04 7D 06 E6 F4 FF 7F 0D 03 6D 02 E6 F4 00 80"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 1 {
		t.Fatalf("%d maps", len(got))
	}
	if got[0].Y == nil || got[0].Y.Addr != 0x8100C1 || got[0].Y.Count != 16 || got[0].Y.Bits != 8 {
		t.Fatalf("row %+v", got[0].Y)
	}
}

func TestPageCallAxis(t *testing.T) {
	const entry = 0x80
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{0xD4, 0xA0, 0x00, 0x00, 0xD4, 0xB0, 0x02, 0x00, 0xDC, 0x4F, 0xA9, 0x4E})
	// Column axis: count 4, then the breakpoints. Page 0x0204, low 0x0110.
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	// R12 low 0x0200 and R13 page 0x0204 select the body. R14/R15 select the axis.
	at := 0x200
	img[at] = 0xE6
	img[at+1] = 0xFC
	img[at+2] = 0x00
	img[at+3] = 0x02
	img[at+4] = 0xE6
	img[at+5] = 0xFD
	img[at+6] = 0x04
	img[at+7] = 0x02
	img[at+8] = 0xE6
	img[at+9] = 0xFE
	img[at+10] = 0x10
	img[at+11] = 0x01
	img[at+12] = 0xE6
	img[at+13] = 0xFF
	img[at+14] = 0x04
	img[at+15] = 0x02
	img[at+16] = 0xDA
	img[at+18] = byte(entry)
	img[at+19] = byte(entry >> 8)
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8_page
    needle_hex: "D4 A0 00 00 D4 B0 02 00 DC 4F A9 4E"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 1 {
		t.Fatalf("%d maps", len(got))
	}
	if got[0].Addr != 0x810200 || got[0].Cols != 4 || got[0].X == nil || got[0].X.Addr != 0x810111 || got[0].X.Bits != 8 || got[0].Y != nil {
		t.Fatalf("%+v axis %+v", got[0], got[0].X)
	}
}

func TestSetupWordIsRowAxis(t *testing.T) {
	const entry = 0x80
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{0xD4, 0xA0, 0x00, 0x00, 0xD4, 0xB0, 0x02, 0x00, 0xDC, 0x4F, 0xA9, 0x4E})
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	copy(img[0x10084:], []byte{8, 1, 2, 3, 4, 5, 6, 7, 8})
	copy(img[0x300:], []byte{
		0xE6, 0xFC, 0x84, 0x00,
		0xC2, 0xFD, 0x00, 0x00,
		0xF2, 0xFE, 0x34, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x34, 0x8F,
	})
	copy(img[0x200:], []byte{
		0xF2, 0xF5, 0x34, 0x8F,
		0x88, 0x50,
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x04, 0x02,
		0xE6, 0xFE, 0x10, 0x01,
		0xE6, 0xFF, 0x04, 0x02,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8_page
    needle_hex: "D4 A0 00 00 D4 B0 02 00 DC 4F A9 4E"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 1 || got[0].Y == nil || got[0].Y.Addr != 0x810085 || got[0].Y.Count != 8 || got[0].X == nil || got[0].X.Addr != 0x810111 {
		t.Fatalf("%+v", got)
	}
}

func TestStoreBeforeFrameIsRow(t *testing.T) {
	const entry = 0x80
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{0xD4, 0xA0, 0x00, 0x00, 0xD4, 0xB0, 0x02, 0x00, 0xDC, 0x4F, 0xA9, 0x4E})
	copy(img[0x10110:], []byte{4, 1, 2, 3, 4})
	copy(img[0x10084:], []byte{8, 1, 2, 3, 4, 5, 6, 7, 8})
	copy(img[0x300:], []byte{
		0xE6, 0xFC, 0x84, 0x00,
		0xC2, 0xFD, 0x00, 0x00,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x34, 0x8F,
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x04, 0x02,
		0xE6, 0xFE, 0x10, 0x01,
		0xE6, 0xFF, 0x04, 0x02,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8_page
    needle_hex: "D4 A0 00 00 D4 B0 02 00 DC 4F A9 4E"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 1 || got[0].Y == nil || got[0].Y.Addr != 0x810085 || got[0].Y.Count != 8 || got[0].X == nil || got[0].X.Addr != 0x810111 {
		t.Fatalf("%+v", got)
	}
}

func TestRamAxisUsesDPPField(t *testing.T) {
	img := make([]byte, 0x14220)
	copy(img[0x10100:], []byte{4, 9, 8, 7, 6})
	copy(img[0x14100:], []byte{8, 1, 2, 3, 4, 5, 6, 7, 8})
	copy(img[0x40:], []byte{
		0xE6, 0xFC, 0x00, 0x41,
		0xC2, 0xFD, 0x00, 0x00,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x5A, 0x89,
	})
	got, ok := RamAxis(img, opcode.StandardDPP, 0x895A, 0)
	if !ok || got.Addr != opcode.FlashBase+0x14101 || got.Count != 8 || got.Bits != 8 {
		t.Fatalf("ok=%v %+v", ok, got)
	}
}

func TestRamAxis(t *testing.T) {
	const entry = 0x80
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{0xF0, 0x3E, 0xC0, 0x72, 0x00, 0xC2, 0xDC, 0x4D})
	// DPP0 header: count 8, then the breakpoints. Low 14 bits are 0x0084.
	copy(img[0x10084:], []byte{8, 1, 2, 3, 4, 5, 6, 7, 8})
	// A header on an explicit page. Low 14 bits are 0x0110, page 0x0204.
	copy(img[0x10110:], []byte{4, 9, 8, 7, 6})
	copy(img[0x300:], []byte{
		0xE6, 0xFC, 0x84, 0x00,
		0xC2, 0xFD, 0x00, 0x00,
		0xF2, 0xFE, 0x34, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x34, 0x8F,
	})
	copy(img[0x320:], []byte{
		0xE6, 0xFC, 0x10, 0x01,
		0xE6, 0xFD, 0x04, 0x02,
		0xC2, 0xFE, 0x00, 0x00,
		0xF2, 0xFF, 0x58, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x58, 0x8F,
	})
	// Page call. R14 is loaded from the RAM word the first setup stores.
	copy(img[0x400:], []byte{
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x04, 0x02,
		0xF2, 0xFE, 0x34, 0x8F,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	// Direct call. R13 is loaded from the RAM word the second setup stores.
	copy(img[0x420:], []byte{
		0xE6, 0xFC, 0x00, 0x03,
		0xF2, 0xFD, 0x58, 0x8F,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_curve8
    needle_hex: "F0 3E C0 72 00 C2 DC 4D"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 2 {
		t.Fatalf("%d maps", len(got))
	}
	if got[0].Addr != 0x810200 || got[0].Cols != 8 || got[0].X == nil || got[0].X.Addr != 0x810085 || got[0].X.Bits != 8 || got[0].Y != nil {
		t.Fatalf("page %+v axis %+v", got[0], got[0].X)
	}
	if got[1].Addr != 0x810300 || got[1].Cols != 4 || got[1].X == nil || got[1].X.Addr != 0x810111 || got[1].X.Bits != 8 || got[1].Y != nil {
		t.Fatalf("direct %+v axis %+v", got[1], got[1].X)
	}
}

func TestRamAxisTwoHeaders(t *testing.T) {
	const entry = 0x80
	img := make([]byte, 0x10400)
	copy(img[entry:], []byte{0xF0, 0x3E, 0xC0, 0x72, 0x00, 0xC2, 0xDC, 0x4D})
	copy(img[0x10084:], []byte{8, 1, 2, 3, 4, 5, 6, 7, 8})
	copy(img[0x10110:], []byte{4, 9, 8, 7, 6})
	// Two stores of the same word name two headers. The axis stays unread.
	copy(img[0x300:], []byte{
		0xE6, 0xFC, 0x84, 0x00,
		0xC2, 0xFD, 0x00, 0x00,
		0xF2, 0xFE, 0x34, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x34, 0x8F,
	})
	copy(img[0x320:], []byte{
		0xE6, 0xFC, 0x10, 0x01,
		0xC2, 0xFD, 0x00, 0x00,
		0xF2, 0xFE, 0x34, 0x8F,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x34, 0x8F,
	})
	copy(img[0x400:], []byte{
		0xE6, 0xFC, 0x00, 0x02,
		0xE6, 0xFD, 0x04, 0x02,
		0xF2, 0xFE, 0x34, 0x8F,
		0xDA, 0x00, byte(entry), byte(entry >> 8),
	})
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_curve8
    needle_hex: "F0 3E C0 72 00 C2 DC 4D"
    unique: false
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	got := Locate(img, ns, opcode.StandardDPP, nil)
	if len(got) != 1 || got[0].X != nil {
		t.Fatalf("%+v", got)
	}
}

func TestRamAxisZeroBreakpoint(t *testing.T) {
	img := make([]byte, 0x80)
	// Count 6, then a zero breakpoint. The zero is a point, not a 16-bit marker.
	copy(img[0x10:], []byte{6, 0, 0x0f, 0x32, 0x64, 0xc8, 0xfa})
	copy(img[0x40:], []byte{
		0xE6, 0xFC, 0x10, 0x00,
		0xE6, 0xFD, 0x00, 0x02,
		0xC2, 0xFE, 0x00, 0x00,
		0xDA, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x20, 0x8F,
	})
	got, ok := RamAxis(img, opcode.StandardDPP, 0x8F20, 8)
	if !ok || got.Count != 6 || got.Bits != 8 || got.Addr != opcode.FlashBase+0x11 {
		t.Fatalf("byte ok=%v %+v", ok, got)
	}
	wide, ok := RamAxis(img, opcode.StandardDPP, 0x8F20, 0)
	if !ok || wide.Bits != 16 || wide.Addr != opcode.FlashBase+0x12 {
		t.Fatalf("marker ok=%v %+v", ok, wide)
	}
}

func TestPackedSkipsOddPad(t *testing.T) {
	// The decoded pointer is the pad. Counts start on the next byte: 2 rows
	// of 8-bit breakpoints, then 2 columns of 16-bit breakpoints.
	img := []byte{
		0xFF,
		0x00,
		4, 3,
		1, 2, 3, 4,
		5, 6, 7,
		0xAA,
	}
	rows, cols, x, y, ok := Packed(img, opcode.FlashBase+1, opcode.FlashBase+11)
	if !ok || rows != 4 || cols != 3 || y.Bits != 8 || x.Bits != 8 || y.Addr != opcode.FlashBase+4 || x.Addr != opcode.FlashBase+8 {
		t.Fatalf("ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
}

func TestPackedPadsWideAxis(t *testing.T) {
	// One 8-bit row breakpoint leaves the column axis on an odd address.
	// The pad byte is skipped and the two column breakpoints are 16-bit.
	img := []byte{5, 2, 1, 2, 3, 4, 5, 0x00, 0x10, 0x00, 0x20, 0x00, 0xBB}
	rows, cols, x, y, ok := Packed(img, opcode.FlashBase, opcode.FlashBase+12)
	if !ok || rows != 5 || cols != 2 || y.Bits != 8 || x.Bits != 16 || y.Addr != opcode.FlashBase+2 || x.Addr != opcode.FlashBase+8 {
		t.Fatalf("ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
}

func TestCurve(t *testing.T) {
	img := []byte{4, 1, 2, 3, 4, 0xAA}
	cols, x, ok := Curve(img, opcode.FlashBase, opcode.FlashBase+5)
	if !ok || cols != 4 || x.Bits != 8 || x.Addr != opcode.FlashBase+1 {
		t.Fatalf("ok=%v cols=%d x=%+v", ok, cols, x)
	}
	// Odd header. The word count is the next even address, then four 16-bit breakpoints.
	wide := []byte{0xFF, 0x02, 0x04, 0x00, 0x10, 0x00, 0x20, 0x00, 0x30, 0x00, 0x40, 0x00, 0xBB}
	cols, x, ok = Curve(wide, opcode.FlashBase+1, opcode.FlashBase+12)
	if !ok || cols != 4 || x.Bits != 16 || x.Addr != opcode.FlashBase+4 {
		t.Fatalf("wide ok=%v cols=%d x=%+v", ok, cols, x)
	}
}

func TestShaped(t *testing.T) {
	// 8-bit curve of 4 points. The offset is the count byte plus those points.
	img := []byte{4, 1, 2, 3, 4, 0xAA}
	rows, cols, x, y, ok := Shaped(img, opcode.FlashBase+5, 0, 4, 0, 8)
	if !ok || rows != 0 || cols != 4 || x.Bits != 8 || x.Addr != opcode.FlashBase+1 || y.Addr != 0 {
		t.Fatalf("curve ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
	// 16 by 16: byte counts, 8-bit rows, 16-bit columns. Body follows that data.
	wide := []byte{16, 16}
	for i := 0; i < 16; i++ {
		wide = append(wide, byte(i))
	}
	for i := 0; i < 16; i++ {
		wide = append(wide, byte(i), 0)
	}
	wide = append(wide, 0xBB)
	body := opcode.FlashBase + uint32(len(wide)-1)
	rows, cols, x, y, ok = Shaped(wide, body, 16, 16, 8, 16)
	if !ok || rows != 16 || cols != 16 || y.Bits != 8 || x.Bits != 16 || y.Addr != opcode.FlashBase+2 || x.Addr != opcode.FlashBase+18 {
		t.Fatalf("map ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
	// Different dimensions are not that header.
	if _, _, _, _, ok = Shaped(wide, body, 8, 8, 8, 8); ok {
		t.Fatal("accepted the wrong dimensions")
	}
	// Word counts whose high byte is zero are the same axis as the low byte
	// plus the pad in front of a 16-bit field.
	word := []byte{0x0A, 0x00, 0x0A, 0x00}
	word = append(word, make([]byte, 10*2+10*2)...)
	word = append(word, 0xBB)
	body = opcode.FlashBase + uint32(len(word)-1)
	rows, cols, x, y, ok = Shaped(word, body, 10, 10, 16, 16)
	if !ok || rows != 10 || cols != 10 || y.Addr != opcode.FlashBase+4 || x.Addr != opcode.FlashBase+24 {
		t.Fatalf("word ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
}

func TestPackedHeaderLeadsCounts(t *testing.T) {
	img := []byte{0x00, 0x00, 4, 4, 1, 2, 3, 4, 5, 6, 7, 8, 0xAA}
	rows, cols, x, y, ok := Packed(img, opcode.FlashBase, opcode.FlashBase+12)
	if !ok || rows != 4 || cols != 4 || y.Addr != opcode.FlashBase+4 || x.Addr != opcode.FlashBase+8 {
		t.Fatalf("ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
}

func TestPackedByteCounts(t *testing.T) {
	img := []byte{4, 4, 1, 2, 3, 4, 5, 6, 7, 8, 0xAA}
	rows, cols, x, y, ok := Packed(img, opcode.FlashBase, opcode.FlashBase+10)
	if !ok || rows != 4 || cols != 4 || y.Addr != opcode.FlashBase+2 || x.Addr != opcode.FlashBase+6 || y.Bits != 8 || x.Bits != 8 {
		t.Fatalf("ok=%v rows=%d cols=%d x=%+v y=%+v", ok, rows, cols, x, y)
	}
}

func TestNameFromCallerSlot(t *testing.T) {
	const (
		interpAt = 0x40
		caller   = 0x100
		slot     = 0x5DE
	)
	img := make([]byte, 0x10400)
	copy(img[interpAt:], []byte{
		0x26, 0xF4, 0x00, 0x80, 0x8D, 0x04, 0x7D, 0x06,
		0xE6, 0xF4, 0xFF, 0x7F, 0x0D, 0x03, 0x6D, 0x02,
		0xE6, 0xF4, 0x00, 0x80,
	})
	copy(img[caller:], []byte{0x88, 0x90, 0x88, 0x80, 0x88, 0x70, 0x88, 0x60, 0x9A, 0x58, 0x03, 0x00})
	putCall(img, caller+slot-16, interpAt, 0x0200, 0x0110)
	// A second call in the same function has no slot, so it stays unnamed.
	putCall(img, caller+0x200, interpAt, 0x0300, 0x0110)
	ns, err := needle.Parse([]byte(`
functions:
  - name: map_interp_table8
    needle_hex: "26 F4 00 80 8D 04 7D 06 E6 F4 FF 7F 0D 03 6D 02 E6 F4 00 80"
    unique: false
  - name: ign_zw
    needle_hex: "88 90 88 80 88 70 88 60 9A 58 03 00"
`), "t")
	if err != nil {
		t.Fatal(err)
	}
	calls := []record.Call{{
		Name: "KFZW", Caller: "ign_zw", At: slot, Interp: Byte,
	}}
	got := Locate(img, ns, opcode.StandardDPP, calls)
	if len(got) != 2 {
		t.Fatalf("%d maps", len(got))
	}
	if got[0].Name != "KFZW" || got[0].Addr != 0x810200 {
		t.Fatalf("%+v", got[0])
	}
	if got[1].Name != "" || got[1].Addr != 0x810300 {
		t.Fatalf("%+v", got[1])
	}
}

func TestCallsToHighSegment(t *testing.T) {
	img := make([]byte, 0x30000)
	// File 0x4906 is CPU 0x804906. High code calls it with segment 0x80.
	img[0x20000] = 0xDA
	img[0x20001] = 0x80
	img[0x20002] = 0x06
	img[0x20003] = 0x49
	// File 0x2416C is CPU 0x82416C. The file-offset segment byte is not the call.
	img[0x20100] = 0xDA
	img[0x20101] = 0x82
	img[0x20102] = 0x6C
	img[0x20103] = 0x41
	img[0x20200] = 0xDA
	img[0x20201] = 0x02
	img[0x20202] = 0x6C
	img[0x20203] = 0x41
	if got := callsTo(img, 0x4906); len(got) != 1 || got[0] != 0x20000 {
		t.Fatalf("page call %v", got)
	}
	if got := callsTo(img, 0x2416C); len(got) != 1 || got[0] != 0x20100 {
		t.Fatalf("high call %v", got)
	}
	// The first 64K is still reached with segment 0.
	img[0x100] = 0xDA
	img[0x102] = 0xB8
	img[0x103] = 0x78
	if got := callsTo(img, 0x78B8); len(got) != 1 || got[0] != 0x100 {
		t.Fatalf("low call %v", got)
	}
}

func putCall(img []byte, at, entry int, r12, r13 uint16) {
	img[at] = 0xE6
	img[at+1] = 0xFC
	img[at+2] = byte(r12)
	img[at+3] = byte(r12 >> 8)
	img[at+4] = 0xE6
	img[at+5] = 0xFD
	img[at+6] = byte(r13)
	img[at+7] = byte(r13 >> 8)
	img[at+8] = 0xF2
	img[at+9] = 0xFE
	img[at+12] = 0xF2
	img[at+13] = 0xFF
	img[at+16] = 0xDA
	img[at+17] = byte(entry >> 16)
	img[at+18] = byte(entry)
	img[at+19] = byte(entry >> 8)
}

func TestBreakpoints(t *testing.T) {
	img := make([]byte, 16)
	img[2] = 3
	copy(img[3:], []byte{1, 2, 3})
	ax, ok := Breakpoints(img, opcode.FlashBase+2, 8)
	if !ok || ax.Addr != opcode.FlashBase+3 || ax.Count != 3 || ax.Bits != 8 {
		t.Fatalf("%v %+v", ok, ax)
	}
	img[6] = 2
	copy(img[8:], []byte{0x10, 0x00, 0x20, 0x00})
	ax, ok = Breakpoints(img, opcode.FlashBase+6, 16)
	if !ok || ax.Addr != opcode.FlashBase+8 || ax.Count != 2 || ax.Bits != 16 {
		t.Fatalf("word %v %+v", ok, ax)
	}
	img[2] = 0
	if _, ok := Breakpoints(img, opcode.FlashBase+2, 8); ok {
		t.Fatal("accepted a zero count")
	}
}
