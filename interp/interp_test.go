package interp

import (
	"testing"

	"me7-logger/needle"
	"me7-logger/opcode"
	"me7-logger/record"
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
	if got[0].X == nil || got[0].X.Addr != 0x810111 || got[0].X.Count != 4 || got[0].X.Bits != 8 {
		t.Fatalf("%+v", got[0].X)
	}
	if got[1].Addr != 0x810300 || got[1].Cols != 4 || got[1].Comment != ByteB {
		t.Fatalf("%+v", got[1])
	}
	if got[1].X == nil || got[1].X.Addr != 0x810122 || got[1].X.Bits != 16 || got[1].X.Count != 4 {
		t.Fatalf("%+v", got[1].X)
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
