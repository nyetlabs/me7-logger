// Package opcode walks result-type selectors and resolves DPP addresses.
// Instruction length and branch targets are the case walker's table.
// A compare arm ends on JMPR (2D) or JMPA (EA).
package opcode

import "encoding/binary"

// FlashBase is the CPU address of file offset 0.
const FlashBase = 0x800000

// StandardDPP is the usual runtime block: DPP0 0x0204, DPP1 0x0205, DPP2 0x00E0, DPP3 0x0003.
var StandardDPP = [4]uint16{0x0204, 0x0205, 0x00E0, 0x0003}

// FileOffset is a CPU address as a file offset. An address below FlashBase
// is returned unchanged.
func FileOffset(addr uint32) uint32 {
	if addr >= FlashBase {
		return addr - FlashBase
	}
	return addr
}

// Physical is dppAddr. An EXTP page overrides the DPP for that operand:
// (page<<14) | (off & 0x3FFF). extpPage < 0 means no override. setPage stores
// that page and clearPage drops it.
func Physical(dpp [4]uint16, mem uint16, extpPage int) uint32 {
	if extpPage >= 0 {
		return (uint32(extpPage) << 14) | uint32(mem&0x3FFF)
	}
	idx := mem >> 14
	off := mem & 0x3FFF
	if int(idx) >= len(dpp) {
		return 0
	}
	return (uint32(dpp[idx]) << 14) | uint32(off)
}

// FindDPP returns the most common MOV DPP0..DPP3,#imm block whose DPP0 is not 0.
// File offsets of every copy of that block are returned too. DPP0 = 0 is the
// reset block; the runtime value is written later.
func FindDPP(data []byte) (vals [4]uint16, offs []int, ok bool) {
	type key struct{ d0, d1, d2, d3 uint16 }
	blocks := map[key][]int{}
	for i := 0; i+16 <= len(data); i += 2 {
		if data[i] != 0xE6 || data[i+1] != 0x00 || data[i+4] != 0xE6 || data[i+5] != 0x01 ||
			data[i+8] != 0xE6 || data[i+9] != 0x02 || data[i+12] != 0xE6 || data[i+13] != 0x03 {
			continue
		}
		k := key{
			binary.LittleEndian.Uint16(data[i+2 : i+4]),
			binary.LittleEndian.Uint16(data[i+6 : i+8]),
			binary.LittleEndian.Uint16(data[i+10 : i+12]),
			binary.LittleEndian.Uint16(data[i+14 : i+16]),
		}
		blocks[k] = append(blocks[k], i)
	}
	if len(blocks) == 0 {
		return StandardDPP, nil, false
	}
	better := func(k key, v []int, best key, bv []int) bool {
		knz, bnz := 0, 0
		if k.d0 != 0 {
			knz = 1
		}
		if best.d0 != 0 {
			bnz = 1
		}
		if knz != bnz {
			return knz > bnz
		}
		if len(v) != len(bv) {
			return len(v) > len(bv)
		}
		return v[0] < bv[0]
	}
	var best key
	have := false
	for k, v := range blocks {
		if !have || better(k, v, best, blocks[best]) {
			best = k
			have = true
		}
	}
	return [4]uint16{best.d0, best.d1, best.d2, best.d3}, blocks[best], true
}

// SelectorEntry is one result-type selector reached from the dispatcher table.
type SelectorEntry struct {
	Index int
	Off   int
	// Finish is the address scanResults passes as both case limits.
	// It comes from the word after 7C4X46FX, in that instruction's segment.
	// Zero when the dispatcher is not the jump-table form.
	Finish uint32
}

// SelectorTable reads the jump table immediately before a selector-0 prefix.
// The prefix is 38 bytes after the anchor. In between, SHR R5,#4 / CMP R5,#max
// / SHL R5,#1 / ADD R5,#table / JMPI [R5] selects entries 0..max. Each word is
// a code offset in the dispatcher's segment. The table address is that immediate
// through the runtime DPP. An entry that is not a selector prefix is skipped.
// Images that SUB R5,#1 instead of shifting do not match.
func SelectorTable(data []byte, prefix int, base uint32, dpp [4]uint16, prefixes [][]byte) []SelectorEntry {
	if prefix < 38 || prefix+2 > len(data) {
		return nil
	}
	if data[prefix-20] != 0x7C || data[prefix-19] != 0x45 ||
		data[prefix-18] != 0x46 || data[prefix-17] != 0xF5 ||
		data[prefix-8] != 0x06 || data[prefix-7] != 0xF5 ||
		data[prefix-4] != 0xA8 || data[prefix-3] != 0x55 ||
		data[prefix-2] != 0x9C || data[prefix-1] != 0x05 {
		return nil
	}
	maxIdx := int(binary.LittleEndian.Uint16(data[prefix-16 : prefix-14]))
	n := maxIdx + 1
	if n <= 0 || n > 128 {
		return nil
	}
	near := binary.LittleEndian.Uint16(data[prefix-6 : prefix-4])
	phys := Physical(dpp, near, -1)
	if phys < base {
		return nil
	}
	table := int(phys - base)
	if table < 0 || table+n*2 > len(data) {
		return nil
	}
	seg := (base + uint32(prefix-2)) & 0xFF0000
	// scanResults takes the segment of the match+6 byte and the word at match+8.
	// The match is the 7C4X46FX at prefix-20. That address, not the 0x3F7
	// branch, is the case limit.
	finishWord := binary.LittleEndian.Uint16(data[prefix-12 : prefix-10])
	finish := ((base + uint32(prefix-14)) & 0xFF0000) | uint32(finishWord)
	var out []SelectorEntry
	for i := 0; i < n; i++ {
		word := binary.LittleEndian.Uint16(data[table+2*i : table+2*i+2])
		cpu := seg | uint32(word)
		if cpu < base {
			continue
		}
		off := int(cpu - base)
		if !IsSelector(data, off, prefixes) {
			continue
		}
		out = append(out, SelectorEntry{Index: i, Off: off, Finish: finish})
	}
	return out
}

// IsSubSelector reports the SUB R5,#1 dispatcher. scanResults tries 28X146FX
// before the SHR form. The label is that instruction.
func IsSubSelector(data []byte, off int) bool {
	if off < 0 || off+4 > len(data) || off%2 != 0 {
		return false
	}
	return data[off] == 0x28 && data[off+1]&0x0F == 0x01 &&
		data[off+2] == 0x46 && data[off+3]&0xF0 == 0xF0
}

// WalkSubSelector reads the jump table in front of a SUB R5,#1 dispatcher.
// The word at the match+4 is the last index. The word at match+8 is the case
// limit in the dispatcher's segment. The word at match+0xe, plus 0x810000, is
// the table. Entry i (1-based) is a case body, and that index is its result type.
func WalkSubSelector(data []byte, off int, base uint32, dpp [4]uint16) []Case {
	if !IsSubSelector(data, off) || off+0x14 > len(data) {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(data[off+4:off+6])) + 1
	if n < 1 || n > 0x1000 {
		return nil
	}
	seg := (base + uint32(off+0x12)) & 0xFF0000
	finish := seg | uint32(binary.LittleEndian.Uint16(data[off+8:off+10]))
	tableCPU := 0x810000 + uint32(binary.LittleEndian.Uint16(data[off+0x0e:off+0x10]))
	if tableCPU < base {
		return nil
	}
	table := int(tableCPU - base)
	if table < 0 || table+n*2 > len(data) {
		return nil
	}
	var cases []Case
	for i := 1; i <= n; i++ {
		word := binary.LittleEndian.Uint16(data[table+2*(i-1) : table+2*i])
		cpu := seg | uint32(word)
		if cpu == finish || cpu > finish || cpu < base {
			continue
		}
		body := int(cpu - base)
		if body >= len(data) {
			continue
		}
		c := Case{ResultType: i, Target: cpu}
		insns, end, walked := decodeCase(data, body, base, finish)
		c.Off, c.End = body, end
		if !walked {
			continue
		}
		cases = append(cases, casesFrom(c, matchTemplates(insns, i, dpp))...)
	}
	return cases
}

// IsSelector reports whether data[off:] begins with one of the selector prefixes
// from config/names.yaml.
func IsSelector(data []byte, off int, prefixes [][]byte) bool {
	return prefixLen(data, off, prefixes) > 0
}

func prefixLen(data []byte, off int, prefixes [][]byte) int {
	if off < 0 {
		return 0
	}
	for _, p := range prefixes {
		if len(p) == 0 || off+len(p) > len(data) {
			continue
		}
		match := true
		for i := range p {
			if data[off+i] != p[i] {
				match = false
				break
			}
		}
		if match {
			return len(p)
		}
	}
	return 0
}

// Case is one compare arm of a result-type selector.
// Off and End are the file range of the case body, [Off, End).
// End is 0 when the body is not in the image.
type Case struct {
	ResultType int
	Bitmask    uint16
	Addr       uint32
	Size       int
	Guessed    bool
	Target     uint32
	Off        int
	End        int
	// MovAt is the file offset of the load. AddrAt is its mem operand.
	// Both stay 0 when the case has no load.
	MovAt  int
	AddrAt int
	// Name is set when the template names the variable (rkaz_w, rkat_w).
	// An empty name is chosen from the catalog by result type.
	Name string
	// Key is the catalog bitmask. Keyed is set when that key is not Bitmask.
	// Mode 0x10 names the row by the bit index and stores the loaded bit in Bitmask.
	Key   uint16
	Keyed bool
}

// WalkSelector parses compare/branch pairs after a selector prefix.
// A leading JMPR (2D) or JMPA (EA 20) is the compare-0 case; its target is
// matched as result type index<<4. The compares follow it. Result type is
// (index<<4) | compare. endImm closes the walk and is not a variable.
// index is the selector number; 0 uses the compare alone.
// prefixes come from config/names.yaml. finish is the address scanResults
// uses as both case limits. When it is 0, the end-compare branch is that limit.
func WalkSelector(data []byte, off int, base uint32, dpp [4]uint16, index int, prefixes [][]byte, endImm int, finish uint32) []Case {
	n := prefixLen(data, off, prefixes)
	if n == 0 {
		return nil
	}
	type arm struct {
		imm    int
		target uint32
	}
	var arms []arm
	def := finish
	i := off + n
	if target, nlead := leadingJump(data, i, base); nlead > 0 {
		arms = append(arms, arm{imm: 0, target: target})
		i += nlead
	}
	for i+2 <= len(data) && i%2 == 0 {
		var imm, insnLen int
		switch {
		case data[i] == 0x46 && (data[i+1] == 0xF4 || data[i+1] == 0xFC) && i+4 <= len(data):
			imm = int(binary.LittleEndian.Uint16(data[i+2 : i+4]))
			insnLen = 4
		case data[i] == 0x48 && (data[i+1]&0xF0) == 0x40:
			// CMP Rn, #imm4. The immediate is the low nibble.
			imm = int(data[i+1] & 0x0F)
			insnLen = 2
		default:
			imm = -1
		}
		if imm < 0 {
			break
		}
		target, n, ok := takenJump(data, i+insnLen, base)
		if !ok {
			break
		}
		if imm == endImm {
			if def == 0 {
				def = target
			}
			break
		}
		arms = append(arms, arm{imm: imm, target: target})
		i += insnLen + n
	}
	cases := make([]Case, 0, len(arms))
	for _, a := range arms {
		c := Case{ResultType: (index << 4) | a.imm, Target: a.target, Guessed: true}
		if body, ok := fileOff(data, a.target, base); ok {
			insns, end, walked := decodeCase(data, body, base, def)
			c.Off, c.End = body, end
			var hits []loc
			if walked {
				hits = matchTemplates(insns, c.ResultType, dpp)
			}
			if len(hits) > 0 {
				cases = append(cases, casesFrom(c, hits)...)
				continue
			}
		}
		cases = append(cases, c)
	}
	return cases
}

// casesFrom copies one case per template hit. An empty hit list copies nothing.
func casesFrom(c Case, hits []loc) []Case {
	out := make([]Case, len(hits))
	for i, h := range hits {
		cc := c
		cc.Addr, cc.Size, cc.Bitmask, cc.Guessed = h.addr, h.size, h.mask, false
		cc.Key, cc.Keyed = h.key, h.keyed
		if h.rt != 0 {
			cc.ResultType = h.rt
		}
		cc.MovAt, cc.AddrAt, cc.Name = h.at, h.at+2, h.name
		out[i] = cc
	}
	return out
}

// leadingJump is the jump before the first compare: JMPR (2D) or JMPA (EA 20).
// Its target is the compare-0 case. Any other opcode, including a compare, returns 0.
func leadingJump(data []byte, off int, base uint32) (uint32, int) {
	target, n, ok := takenJump(data, off, base)
	if !ok || (data[off] == 0xEA && data[off+1] != 0x20) {
		return 0, 0
	}
	return target, n
}

// takenJump is the JMPR or JMPA after a compare. The target uses branch,
// so the compare chain and the case body share one rule. A conditional
// relative jump is not a compare arm.
func takenJump(data []byte, off int, base uint32) (target uint32, n int, ok bool) {
	if off < 0 || off%2 != 0 || off+2 > len(data) {
		return 0, 0, false
	}
	op := data[off]
	if op != 0x2D && op != 0xEA {
		return 0, 0, false
	}
	n = int(c166Len[op])
	if n < 2 || off+n > len(data) {
		return 0, 0, false
	}
	in := c166{n: n, op: op, b2: data[off+1]}
	if n == 4 {
		in.mem = binary.LittleEndian.Uint16(data[off+2 : off+4])
	}
	target, _, ok = branch(in, base+uint32(off)+uint32(n))
	return target, n, ok
}

func fileOff(data []byte, cpu, base uint32) (int, bool) {
	if cpu < base {
		return 0, false
	}
	off := int(cpu - base)
	if off < 0 || off >= len(data) {
		return 0, false
	}
	return off, true
}
