package opcode

import (
	"bytes"
	"encoding/binary"
	"testing"
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
	// prefix, compare 1 + JMPR to the MOV, end compare 0x3F7 + JMPR to RETS.
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x46, 0xF4, 0x01, 0x00, 0x2D, 9})
	copy(img[14:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 9})
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
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
	// JMPR before the first compare. Without the skip this selector has no arms.
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x2D, 0x00, 0x46, 0xF4, 0x01, 0x00, 0x2D, 8})
	copy(img[16:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 8})
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].ResultType != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	// JMPA EA 20 is the other leading form. EA 00 is not, and the walk stops.
	copy(img[8:], []byte{0xEA, 0x20, 0x00, 0x00, 0x46, 0xF4, 0x01, 0x00, 0x2D, 7})
	copy(img[18:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 7})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].ResultType != 1 || cases[0].Addr != 0x380100 {
		t.Fatalf("%+v", cases)
	}
	copy(img[8:], []byte{0xEA, 0x00, 0x00, 0x00, 0x46, 0xF4, 0x01, 0x00, 0x2D, 7})
	if cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0); len(cases) != 0 {
		t.Fatalf("EA 00 %d", len(cases))
	}
}

func TestWalkNibbleCompare(t *testing.T) {
	img := make([]byte, 0x30)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	// 48 41 is compare #1. JMPR at off 10: target 0x20, rel = 10.
	copy(img[8:], []byte{0x48, 0x41, 0x2D, 10})
	copy(img[12:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 16})
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].ResultType != 1 || cases[0].Addr != 0x380100 {
		t.Fatalf("%+v", cases)
	}
}

func TestJumpEndsCase(t *testing.T) {
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x48, 0x41, 0x2D, 10})
	copy(img[12:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 16})
	// Word MOV, a second instruction, then JMPA cc_UC. A one-instruction
	// case does not match. The MOV after the jump belongs to the next case.
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x00, 0xF2, 0xF4, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	// JMPA cc_EQ also ends the case. The fallthrough MOV is the next case.
	copy(img[0x20:], []byte{0xF2, 0xF4, 0x00, 0x81, 0xEA, 0x20, 0x00, 0x00, 0xF2, 0xF4, 0x00, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
}

func TestAlternateMOV(t *testing.T) {
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x48, 0x41, 0x2D, 10})
	copy(img[12:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 16})
	// MOV [Rwn], mem. No F2/F3 in the case. The pad keeps the case
	// at two instructions; a lone MOV does not match.
	copy(img[0x20:], []byte{0x84, 0x00, 0x00, 0x81, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0x380100 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	// C2 FC is not template [019] (that one is C2 F4 at the start). No match.
	copy(img[0x20:], []byte{0xC2, 0xFC, 0x67, 0x8A, 0xEA, 0x00, 0x00, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Size != 0 {
		t.Fatalf("%+v", cases)
	}
	// F3 FC is [011] only when it is the last instruction. A following C2 F4
	// is not [019] either, because that template wants C2 F4 at the start.
	copy(img[0x20:], []byte{0xF3, 0xFC, 0x00, 0x81, 0xC2, 0xF4, 0x00, 0x00, 0xEA, 0x00, 0x00, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Size != 0 {
		t.Fatalf("%+v", cases)
	}
}

func TestBitAndSelectsMask(t *testing.T) {
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x48, 0x41, 0x2D, 10})
	copy(img[12:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 16})
	// A single-bit AND is not a template. The mask stays unset.
	copy(img[0x20:], []byte{0x66, 0xF4, 0x02, 0x00, 0xF3, 0xF8, 0x00, 0x81, 0xDB, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Bitmask != 0 || cases[0].Size != 0 {
		t.Fatalf("%+v", cases)
	}
}

func TestSFRBit(t *testing.T) {
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x48, 0x41, 0x2D, 10})
	copy(img[12:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 16})
	// 9A bit 0x20, top nibble 0xC, then MOV and JMPR. The later word load is not the bit.
	copy(img[0x20:], []byte{0x9A, 0x20, 0x03, 0xC0, 0xE6, 0xF4, 0x00, 0x81, 0x0D, 0x02, 0xF2, 0xF4, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0xFD40 || cases[0].Size != 2 || cases[0].Bitmask != 0x1000 {
		t.Fatalf("%+v", cases)
	}
	// E1 x8 is the other load in front of the 0D.
	copy(img[0x20:], []byte{0x9A, 0x63, 0x02, 0xA0, 0xE1, 0xD8, 0x0D, 0x01})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Addr != 0xFDC6 || cases[0].Bitmask != 0x0400 || cases[0].Size != 2 {
		t.Fatalf("%+v", cases)
	}
	// A bit offset at or above 0x81 is not [028], and the cascade does not
	// then take a later word load.
	copy(img[0x20:], []byte{0x9A, 0xF4, 0x03, 0x00, 0xE6, 0xF4, 0x11, 0x22, 0x0D, 0x02, 0xF2, 0xF4, 0x00, 0x81, 0xDB, 0x00})
	cases = WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || !cases[0].Guessed || cases[0].Addr != 0 {
		t.Fatalf("%+v", cases)
	}
}

func TestByteReadR10(t *testing.T) {
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x46, 0xFC, 0x25, 0x00, 0x2D, 9})
	copy(img[14:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 12})
	// [020] is E7 F8, F3 FA, 21 A8. F3 FA is the byte read into R10.
	copy(img[0x20:], []byte{0xE7, 0xF8, 0x26, 0x00, 0xF3, 0xFA, 0xA6, 0x8A, 0x21, 0xA8, 0x00, 0x00})
	cases := WalkSelector(img, 0, FlashBase, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 || cases[0].Size != 1 || cases[0].Addr != 0x380AA6 || cases[0].ResultType != 0x25 {
		t.Fatalf("%+v", cases)
	}
}

func TestEXTPBeforeMOV(t *testing.T) {
	img := make([]byte, 0x40)
	copy(img, []byte{0xD4, 0x40, 0x14, 0x00, 0x66, 0xF4, 0x0F, 0x00})
	copy(img[8:], []byte{0x46, 0xFC, 0x02, 0x00, 0x2D, 9})
	copy(img[14:], []byte{0x46, 0xF4, 0xF7, 0x03, 0x2D, 10})
	// [011]: the last instruction is F3 FC, and the EXTP before it sets the page.
	copy(img[0x20:], []byte{0xD7, 0x40, 0xE1, 0x00, 0xF3, 0xFC, 0x00, 0x01})
	cases := WalkSelector(img, 0, 0, StandardDPP, 0, testPrefixes(), 0x3F7, 0)
	if len(cases) != 1 {
		t.Fatalf("cases %d", len(cases))
	}
	if cases[0].Size != 1 || cases[0].Addr != 0x384100 || cases[0].ResultType != 2 {
		t.Fatalf("%+v", cases[0])
	}
}

func testPrefixes() [][]byte {
	return [][]byte{
		{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00},
		{0xD4, 0x40, 0x14, 0x00, 0x66, 0xF4, 0x0F, 0x00},
	}
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
	img := make([]byte, 0x100)
	prefix := []byte{0xD4, 0x40, 0x12, 0x00, 0x66, 0xF4, 0x0F, 0x00}
	copy(img[0x5A:], []byte{
		0xE0, 0x09, 0xE6, 0xF8, 0x00, 0x00, 0xE1, 0x0E, 0xE1, 0x0C, 0xE0, 0x04,
		0xB8, 0x40, 0xD4, 0x50, 0x12, 0x00, 0x7C, 0x45, 0x46, 0xF5, 0x02, 0x00,
		0xEA, 0xE0, 0x00, 0x00, 0x5C, 0x15, 0x06, 0xF5, 0x20, 0x00, 0xA8, 0x55, 0x9C, 0x05,
	})
	copy(img[0x80:], prefix)
	copy(img[0xC0:], prefix)
	// max index 2. The middle word is not a prefix, so its index is not reused.
	binary.LittleEndian.PutUint16(img[0x20:], 0x0080)
	binary.LittleEndian.PutUint16(img[0x22:], 0x0000)
	binary.LittleEndian.PutUint16(img[0x24:], 0x00C0)
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
	if len(got) != 1 || got[0].addr != 0x380100 || got[0].size != 2 || got[0].mask != 0x0002 || !got[0].keyed || got[0].key != 1 {
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
