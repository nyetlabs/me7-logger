package opcode

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"me7-logger/internal/heximg"
)

func TestFindDPPPrefersRuntime(t *testing.T) {
	boot := []byte{0xE6, 0x00, 0x00, 0x00, 0xE6, 0x01, 0x05, 0x02, 0xE6, 0x02, 0xE0, 0x00, 0xE6, 0x03, 0x03, 0x00}
	run := []byte{0xE6, 0x00, 0x04, 0x02, 0xE6, 0x01, 0x05, 0x02, 0xE6, 0x02, 0xE0, 0x00, 0xE6, 0x03, 0x03, 0x00}
	data := append(append(boot, 0, 0), run...)
	vals, offs, ok := FindDPP(data)
	if !ok {
		t.Fatal("expected a block")
	}
	if vals != StandardDPP {
		t.Fatalf("dpp %x", vals)
	}
	if len(offs) != 1 || offs[0] != 18 {
		t.Fatalf("offs %v", offs)
	}
}

func TestPhysicalStandardDPP(t *testing.T) {
	dpp := StandardDPP
	if got := Physical(dpp, 0x8100, -1); got != 0x380100 {
		t.Fatalf("dpp2 %X", got)
	}
	if got := Physical(dpp, 0xC100, -1); got != 0xC100 {
		t.Fatalf("dpp3 %X", got)
	}
	if got := Physical(dpp, 0x1234, -1); got != 0x811234 {
		t.Fatalf("dpp0 %X", got)
	}
	mem := uint16(0x4000 | 0x0123)
	if got := Physical(dpp, mem, -1); got != 0x810000+uint32(mem&0x7FFF) {
		t.Fatalf("dpp1 %X", got)
	}
	if got := Physical(dpp, 0x0100, 0xE1); got != 0x384100 {
		t.Fatalf("extp %X", got)
	}
}

func TestWalkSelector(t *testing.T) {
	img := readHex(t, "walk-selector.hex")
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 {
		t.Fatalf("cases %d", len(cases))
	}
	c := cases[0]
	if c.ResultType != 1 || c.Size != 2 || c.Addr != 0x380100 || c.Guessed {
		t.Fatalf("%+v", c)
	}
	if c.Off != 0x20 || c.End != 0x24 || c.MovAt != 0x20 || c.AddrAt != 0x22 {
		t.Fatalf("extent off %x end %x mov %x addr %x", c.Off, c.End, c.MovAt, c.AddrAt)
	}
}

func TestLeadingJumpSkipped(t *testing.T) {
	img := readHex(t, "leading-jmpr.hex")
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if c, ok := caseRT(cases, 1); !ok || c.Addr != 0x380100 || c.Size != 2 {
		t.Fatalf("%+v", cases)
	}
	img = readHex(t, "leading-jmpa.hex")
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if c, ok := caseRT(cases, 1); !ok || c.Addr != 0x380100 {
		t.Fatalf("%+v", cases)
	}
	img = readHex(t, "leading-ea00.hex")
	if cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0); len(cases) != 0 {
		t.Fatalf("EA 00 %d", len(cases))
	}
}

func TestWalkNibbleCompare(t *testing.T) {
	img := nibbleImg(t, []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].ResultType != 1 || cases[0].Addr != 0x380100 {
		t.Fatalf("%+v", cases)
	}
}

func TestJumpEndsCase(t *testing.T) {
	// A one-instruction case does not match. The MOV after the jump is the next case.
	img := nibbleImg(t, []byte{0xF2, 0xF4, 0x00, 0x81, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x00, 0xF2, 0xF4, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	img = nibbleImg(t, []byte{0xF2, 0xF4, 0x00, 0x81, 0xEA, 0x20, 0x00, 0x00, 0xF2, 0xF4, 0x00, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
}

func TestAlternateMOV(t *testing.T) {
	// MOV [Rwn], mem. The pad keeps the case at two instructions.
	img := nibbleImg(t, []byte{0x84, 0x00, 0x00, 0x81, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	// C2 FC is not template [019]. That one wants C2 F4 at the start.
	img = nibbleImg(t, []byte{0xC2, 0xFC, 0x67, 0x8A, 0xEA, 0x00, 0x00, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Size != 0 {
		t.Fatalf("%+v", cases)
	}
	// F3 FC matches only as the last instruction.
	img = nibbleImg(t, []byte{0xF3, 0xFC, 0x00, 0x81, 0xC2, 0xF4, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Size != 0 {
		t.Fatalf("%+v", cases)
	}
}

func TestBitAndSelectsMask(t *testing.T) {
	img := nibbleImg(t, []byte{0x66, 0xF4, 0x02, 0x00, 0xF3, 0xF8, 0x00, 0x81, 0xDB, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Bitmask != 0 || cases[0].Size != 0 {
		t.Fatalf("%+v", cases)
	}
}

func TestSFRBit(t *testing.T) {
	img := nibbleImg(t, []byte{0x9A, 0x20, 0x03, 0xC0, 0xE6, 0xF4, 0x00, 0x81, 0x0D, 0x02, 0xF2, 0xF4, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0xFD40 || cases[0].Size != 2 || cases[0].Bitmask != 0x1000 {
		t.Fatalf("%+v", cases)
	}
	// E1 x8 is the other load in front of the 0D.
	img = nibbleImg(t, []byte{0x9A, 0x63, 0x02, 0xA0, 0xE1, 0xD8, 0x0D, 0x01})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0xFDC6 || cases[0].Bitmask != 0x0400 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	// A bit offset at or above 0x81 is not [028].
	img = nibbleImg(t, []byte{0x9A, 0xF4, 0x03, 0x00, 0xE6, 0xF4, 0x11, 0x22, 0x0D, 0x02, 0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Addr != 0 {
		t.Fatalf("%+v", cases)
	}
}

func TestByteReadR10(t *testing.T) {
	img := readHex(t, "byte-r10.hex")
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Size != 1 || cases[0].Addr != 0x380AA6 || cases[0].ResultType != 0x25 {
		t.Fatalf("%+v", cases)
	}
}

func TestEXTPBeforeMOV(t *testing.T) {
	img := readHex(t, "extp-mov.hex")
	cases := WalkSelector(img, 0, 0, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 {
		t.Fatalf("cases %d", len(cases))
	}
	if cases[0].Size != 1 || cases[0].Addr != 0x384100 || cases[0].ResultType != 2 {
		t.Fatalf("%+v", cases[0])
	}
}

func caseRT(cases []Case, rt int) (Case, bool) {
	for _, c := range cases {
		if c.ResultType == rt {
			return c, true
		}
	}
	return Case{}, false
}

func testPrefixes() [][]byte {
	return [][]byte{
		{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00},
		{0xD4, 0x40, 0x14, 0x00, 0x66, 0xF4, 0x0F, 0x00},
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

func nibbleImg(t *testing.T, body []byte) []byte {
	t.Helper()
	img := readHex(t, "walk-nibble.hex")
	copy(img[0x20:], body)
	return img
}

func TestJMPATarget(t *testing.T) {
	data := make([]byte, 8)
	data[0] = 0xEA
	data[1] = 0x20
	binary.LittleEndian.PutUint16(data[2:], 0x1234)
	target, n, ok := takenJump(data, 0, FlashBase)
	if !ok || n != 4 || target != FlashBase+0x1234 {
		t.Fatalf("%x %d %v", target, n, ok)
	}
	if !bytes.Equal(data[:1], []byte{0xEA}) {
		t.Fatal("mutated")
	}
}

func TestSelectorTable(t *testing.T) {
	img := readHex(t, "selector-table.hex")
	dpp := [4]uint16{0x0200, 0x0201, 0x00E0, 0x0003}
	got := SelectorTable(img, 0x80, FlashBase, dpp, testPrefixes())
	if len(got) != 2 || got[0].Index != 0 || got[0].Off != 0x80 || got[1].Index != 2 || got[1].Off != 0xC0 || got[0].Finish != FlashBase {
		t.Fatalf("%+v", got)
	}
	img[0x6C] = 0x28
	img[0x6D] = 0x51
	if SelectorTable(img, 0x80, FlashBase, dpp, testPrefixes()) != nil {
		t.Fatal("sub form matched")
	}
}

func insn4(op, b2 byte, mem uint16) c166 {
	raw := []byte{op, b2, byte(mem), byte(mem >> 8)}
	return c166{n: 4, op: op, b2: b2, mem: mem, raw: raw}
}

func insn2(op, b2 byte) c166 {
	return c166{n: 2, op: op, b2: b2, raw: []byte{op, b2}}
}

func TestMode10Walk(t *testing.T) {
	// E6 F8 10 00 / E1 / F0 69 walks the bit in front of the tail.
	// F2 F4 00 81 is DPP2 0x380100. 8A F0 00 10 is bit 1.
	ins := []c166{
		insn2(0x00, 0x00),
		insn4(0xF2, 0xF4, 0x8100),
		insn4(0x8A, 0xF0, 0x1000),
		insn2(0xE0, 0x19),
		insn4(0xE6, 0xF8, 0x0010),
		insn2(0xE1, 0x0E),
		insn2(0xF0, 0x69),
	}
	got := matchTemplates(ins, 1, StandardDPP)
	if len(got) != 1 || got[0].addr != 0x380100 || got[0].size != 2 || got[0].mask != 0x0002 || !got[0].keyed || got[0].key != 1 || got[0].rt != 1 {
		t.Fatalf("%+v", got)
	}
	// Tail 0x90 on compare 0xC2 is catalog 0x20C2. Tail 0x91 stays 0xC2.
	got = matchTemplates(ins, 0xC2, StandardDPP)
	if len(got) != 2 || got[0].rt != 0x20C2 || got[0].key != 1 || got[1].rt != 0xC2 || got[1].addr != got[0].addr {
		t.Fatalf("%+v", got)
	}
	ins[len(ins)-2] = insn2(0xA9, 0xE0)
	got = matchTemplates(ins, 0xC2, StandardDPP)
	if len(got) != 2 || got[0].rt != 0xC2 || got[1].rt != 0x20C2 || got[1].addr != got[0].addr {
		t.Fatalf("%+v", got)
	}
	// The same mode with a final F3 FC is one byte, mask 0.
	tail := []c166{
		insn4(0xE6, 0xF8, 0x0010),
		insn4(0xF3, 0xFC, 0x8100),
	}
	got = matchTemplates(tail, 1, StandardDPP)
	if len(got) != 1 || got[0].addr != 0x380100 || got[0].size != 1 || got[0].mask != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestSubSelector(t *testing.T) {
	if IsSubSelector([]byte{0x7C, 0x45, 0x46, 0xF5}, 0) {
		t.Fatal("SHR is not the SUB dispatcher")
	}
	img := make([]byte, 0x10010)
	off := 0x100
	img[off] = 0x28
	img[off+1] = 0x51
	img[off+2] = 0x46
	img[off+3] = 0xF5
	binary.LittleEndian.PutUint16(img[off+8:], 0x0300)
	// The case is one word load and a pad. The jump target is the case limit.
	copy(img[0x200:], []byte{0xF2, 0xF4, 0x00, 0x81, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x03})
	binary.LittleEndian.PutUint16(img[0x10000:], 0x0200)
	if !IsSubSelector(img, off) {
		t.Fatal("dispatcher")
	}
	cases := WalkSubSelector(img, off, FlashBase, StandardDPP)
	if len(cases) != 1 || cases[0].ResultType != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
}
