package opcode

import "encoding/binary"

// c166Len is the length of each opcode. 0 means the opcode ends the case.
// The bytes are opcodeLen.
var c166Len = [256]byte{
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 00
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 10
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 20
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 0, 2, 2, 2, 2, // 30
	2, 2, 4, 4, 0, 0, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 40
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 50
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 60
	2, 2, 4, 4, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 70
	2, 2, 4, 0, 4, 0, 4, 4, 2, 2, 4, 0, 0, 2, 2, 2, // 80
	2, 2, 4, 0, 4, 0, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // 90
	2, 2, 4, 0, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // A0
	2, 2, 4, 0, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // B0
	2, 0, 4, 0, 4, 4, 4, 0, 2, 2, 4, 2, 2, 2, 2, 2, // C0
	2, 2, 4, 0, 4, 4, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // D0
	2, 2, 4, 0, 4, 0, 4, 4, 2, 2, 4, 2, 2, 2, 2, 2, // E0
	2, 2, 4, 4, 4, 0, 4, 4, 0, 0, 4, 2, 2, 2, 2, 2, // F0
}

// c166 is one instruction the way readInsn records it.
type c166 struct {
	at  int
	n   int
	op  byte
	b2  byte
	mem uint16
	raw []byte
}

// loc is the address a template selected.
type loc struct {
	at    int
	addr  uint32
	size  int
	mask  uint16
	name  string
	key   uint16
	keyed bool
	// rt replaces the case result type. 0 keeps the compare immediate.
	rt int
}

// decodeCase walks a case. def is the finishing address scanResults passes
// as both limits. An unconditional jump to it ends the case and is not stored.
// Reaching it stores the instruction that crossed it. A jump past it abandons
// the case. walked is false when the extent is 0.
func decodeCase(data []byte, off int, base, def uint32) (insns []c166, end int, walked bool) {
	end = off
	if def == 0 || off < 0 || off >= len(data) {
		return nil, end, false
	}
	pc := base + uint32(off)
	furthest := pc
	for len(insns) < 0x400 {
		if pc < base {
			return insns, end, true
		}
		i := int(pc - base)
		if i < 0 || i >= len(data) {
			return insns, i, true
		}
		n := int(c166Len[data[i]])
		if n == 0 || i+n > len(data) {
			return insns, i, true
		}
		in := c166{at: i, n: n, op: data[i], b2: data[i+1], raw: data[i : i+n]}
		if n == 4 {
			in.mem = binary.LittleEndian.Uint16(data[i+2 : i+4])
		}
		next := pc + uint32(n)
		if target, cond, ok := branch(in, next); ok {
			if def < target {
				return nil, off, false
			}
			if target != def && next < target && furthest < target {
				furthest = target
			}
			if target == def && !cond && furthest < next {
				return insns, i, true
			}
		}
		insns = append(insns, in)
		if def <= next {
			return insns, i, true
		}
		pc = next
		end = int(pc - base)
	}
	return insns, end, true
}

// branch reports a JMPA, JMPS, JMPR, or bit jump. cond is true when the
// jump does not end the case. The target is relative to the next PC, which
// readInsn has already advanced to.
func branch(in c166, next uint32) (target uint32, cond, ok bool) {
	switch {
	case in.op == 0xEA:
		return (next & 0xFF0000) | uint32(in.mem), in.b2&0xF0 != 0, true
	case in.op == 0xFA:
		return uint32(in.b2)<<16 | uint32(in.mem), false, true
	case in.op&0x0F == 0x0D:
		return uint32(int64(next) + int64(int8(in.b2))*2), in.op&0xF0 != 0, true
	case in.op == 0x8A || in.op == 0x9A || in.op == 0xAA || in.op == 0xBA:
		return uint32(int64(next) + int64(int8(byte(in.mem)))*2), true, true
	default:
		return 0, false, false
	}
}

// matchTemplates takes the first template that matches.
// A case of one instruction does not match. Result types 0x1F and 0x20 name
// rkaz/rkat from successive word loads. 0x91..0x93 take the F2 whose next
// instruction is a compare.
func matchTemplates(ins []c166, rt int, dpp [4]uint16) []loc {
	if len(ins) < 2 {
		return nil
	}
	mode := 0
	for _, in := range ins {
		if matchPat(in, "E6F8XXXX") {
			mode = int(in.mem)
			break
		}
		if matchPat(in, "E0X8") {
			mode = int(in.b2 >> 4)
			break
		}
	}
	if mode == 0x10 {
		return match10(ins, rt, dpp)
	}
	if rt == 0x1F || rt == 0x20 {
		return matchAdaption(ins, rt, dpp)
	}
	if rt >= 0x91 && rt <= 0x93 {
		if h, ok := matchF2Compare(ins, dpp); ok {
			return []loc{h}
		}
		return nil
	}
	if h, ok := matchCascade(ins, dpp); ok {
		return []loc{h}
	}
	return nil
}

// match10 is mode 0x10. The E6 F8 10 00 / F0 69 tails are walkMode10, one
// catalog row per variable. E1 and E7 are tail 0x90. A9 E0 is tail 0x91.
// A final F3 FC is one byte load. Anything else is no match.
func match10(ins []c166, rt int, dpp [4]uint16) []loc {
	last := len(ins) - 1
	if last >= 2 && matchPat(ins[last-2], "E6F81000") && matchPat(ins[last], "F069") {
		switch {
		case matchPat(ins[last-1], "E1XE") || matchPat(ins[last-1], "E7FEXXXX"):
			return walkMode10(ins, rt, 0x90, dpp)
		case matchPat(ins[last-1], "A9E0"):
			return walkMode10(ins, rt, 0x91, dpp)
		}
	}
	if matchPat(ins[last], "F3FCXXXX") {
		page := -1
		if last >= 1 {
			page = extpPage(ins[last-1])
		}
		return []loc{one(ins[last], 1, 0, page, dpp)}
	}
	return nil
}

func has(ins []c166, i int, pat string) bool {
	return i >= 0 && i < len(ins) && matchPat(ins[i], pat)
}

// walkMode10 walks backward from the tail, takes a bit index, then the load
// in front of it, and returns one address per variable. The sprintf traces
// and the leftover-register gates are not part of the lookup.
// Tail 0x90 on result type 0xC2 is stored as 0x20C2.
func walkMode10(ins []c166, rt, tail int, dpp [4]uint16) []loc {
	i := len(ins) - 4
	if i < 1 {
		return nil
	}
	stored := rt
	if tail != 0x91 && rt == 0xC2 {
		stored = rt + 0x2000
	}
	var out []loc
	for i > 0 {
		next, bit, ok := bit10(ins, i)
		if !ok {
			break
		}
		h, at, ok := load10(ins, next, dpp)
		if !ok {
			break
		}
		if h.addr != 0 {
			h.key = uint16(bit)
			h.keyed = true
			h.rt = stored
			out = append(out, h)
		}
		i = at
	}
	// The walk is tail-first. The first hit for a bit index keeps this result
	// type. One later hit of that index at another address is the 0x20C2 row.
	type seenBit struct {
		addr uint32
		alt  bool
	}
	seen := map[uint16]seenBit{}
	kept := make([]loc, 0, len(out))
	for _, h := range out {
		if prev, ok := seen[h.key]; ok {
			if prev.addr == h.addr || prev.alt || stored != 0xC2 {
				continue
			}
			h.rt = 0x20C2
			prev.alt = true
			seen[h.key] = prev
		} else {
			seen[h.key] = seenBit{addr: h.addr}
		}
		kept = append(kept, h)
	}
	// 0xC2 and 0x20C2 name the same bit indexes. The walk keeps one type per
	// address. The other type is the same bit at that address, so both are stored.
	extra := make([]loc, 0, len(kept))
	for _, h := range kept {
		switch h.rt {
		case 0xC2:
			h.rt = 0x20C2
		case 0x20C2:
			h.rt = 0xC2
		default:
			continue
		}
		extra = append(extra, h)
	}
	return append(kept, extra...)
}

func bit10(ins []c166, i int) (next int, bit byte, ok bool) {
	for {
		if i < 1 {
			return 0, 0, false
		}
		if has(ins, i, "E0X9") {
			return i - 1, ins[i].b2 >> 4, true
		}
		if has(ins, i, "E6F9XXXX") {
			return i - 1, byte(ins[i].mem), true
		}
		prev := i
		i--
		if prev < 3 || !has(ins, prev-2, "F0X9") || !has(ins, prev, "F09X") {
			continue
		}
		if has(ins, i, "79XX") {
			return prev - 3, ins[i].b2 & 0x0f, true
		}
		if has(ins, i, "77FXXXXX") {
			return prev - 3, byte(ins[i].mem), true
		}
	}
}

// load10 reads the variable at i. next is the index the walker continues from.
func load10(ins []c166, i int, dpp [4]uint16) (loc, int, bool) {
	for i >= 0 {
		if has(ins, i, "F09X") || has(ins, i, "E6F9XXXX") || has(ins, i, "E0X9") {
			return loc{}, i, true
		}
		if h, next, ok := loadF3(ins, i, dpp); ok {
			return h, next, true
		}
		if h, next, ok := loadMove(ins, i, dpp); ok {
			return h, next, true
		}
		if h, next, ok := loadBit(ins, i, dpp); ok {
			return h, next, true
		}
		i--
	}
	return loc{}, 0, true
}

func loadF3(ins []c166, i int, dpp [4]uint16) (loc, int, bool) {
	if i > 1 && has(ins, i-2, "F3F8XXXX") && has(ins, i-1, "47F8XXXX") && has(ins, i, "XDXX") {
		page, skip := extpBefore(ins, i-3)
		h := one(ins[i-2], 1, 0, page, dpp)
		return norm10(h, 0, 0), i - skip - 3, true
	}
	if i > 0 && has(ins, i-1, "F3F8XXXX") && has(ins, i, "XDXX") {
		page, skip := extpBefore(ins, i-2)
		h := one(ins[i-1], 1, 0, page, dpp)
		return norm10(h, 0, 0), i - skip - 2, true
	}
	return loc{}, 0, false
}

func loadMove(ins []c166, i int, dpp [4]uint16) (loc, int, bool) {
	if i > 1 && has(ins, i-2, "C2F4XXXX") && (has(ins, i-1, "66F4XXXX") || has(ins, i-1, "684X")) && has(ins, i, "2DXX") {
		page, skip := extpBefore(ins, i-3)
		h := one(ins[i-2], 1, andMask(ins[i-1], false), page, dpp)
		return norm10(h, 0, h.mask), i - skip - 3, true
	}
	if i > 2 && has(ins, i-3, "C2F4XXXX") && has(ins, i-2, "B840") && (has(ins, i-1, "66F4XXXX") || has(ins, i-1, "684X")) && has(ins, i, "2DXX") {
		page, skip := extpBefore(ins, i-4)
		h := one(ins[i-3], 1, andMask(ins[i-1], false), page, dpp)
		return norm10(h, 0, h.mask), i - skip - 3, true
	}
	if i > 1 && has(ins, i-2, "F2F4XXXX") && (has(ins, i-1, "66F4XXXX") || has(ins, i-1, "684X")) && has(ins, i, "2DXX") {
		page, skip := extpBefore(ins, i-3)
		h := one(ins[i-2], 2, andMask(ins[i-1], true), page, dpp)
		return norm10(h, 0, h.mask), i - skip - 3, true
	}
	if i > 2 && has(ins, i-3, "F2F4XXXX") && (has(ins, i-2, "66F4XXXX") || has(ins, i-2, "684X")) && (has(ins, i-1, "46F4XXXX") || has(ins, i-1, "484X")) && has(ins, i, "3DXX") {
		page, skip := extpBefore(ins, i-4)
		h := one(ins[i-3], 2, andMask(ins[i-2], true), page, dpp)
		cmp := ins[i-1].mem
		if has(ins, i-1, "484X") {
			cmp = uint16(ins[i-1].b2 & 0x0f)
		}
		return norm10(h, cmp, h.mask), i - skip - 3, true
	}
	return loc{}, 0, false
}

func loadBit(ins []c166, i int, dpp [4]uint16) (loc, int, bool) {
	if i > 0 && has(ins, i-1, "F2FXXXXX") && (has(ins, i, "8AFXXXXX") || has(ins, i, "9AFXXXXX")) {
		page, skip := extpBefore(ins, i-2)
		h := one(ins[i-1], 2, bitMask(ins[i]), page, dpp)
		return norm10(h, 0, h.mask), i - skip - 2, true
	}
	if addr, ok := BitWord(ins[i].op, ins[i].b2); ok {
		h := loc{at: ins[i].at, addr: addr, size: 2, mask: bitMask(ins[i])}
		return norm10(h, 0, h.mask), i - 1, true
	}
	return loc{}, 0, false
}

func extpBefore(ins []c166, i int) (page, skip int) {
	if has(ins, i, "D740E100") {
		return extpPage(ins[i]), 1
	}
	return -1, 0
}

// andMask is the AND immediate. A byte move (C2) keeps the low byte of a
// 66 F4. A word move (F2) keeps the whole immediate. 68 4X is a nibble.
func andMask(in c166, word bool) uint16 {
	if matchPat(in, "66F4XXXX") {
		if word {
			return in.mem
		}
		return in.mem & 0xFF
	}
	return uint16(in.b2 & 0x0f)
}

func bitMask(in c166) uint16 {
	return uint16(1) << (in.mem >> 12)
}

// BitWord is the SFR word a direct 8A or 9A names. The second byte is the
// word index, and the word is 0xFD00 plus twice that byte. A negative second
// byte is the other form and is not this address.
func BitWord(op, b2 byte) (uint32, bool) {
	if (op != 0x8A && op != 0x9A) || int8(b2) < 0 {
		return 0, false
	}
	return 0xFD00 + 2*uint32(b2), true
}

// norm10 keeps a single-bit mask when the word has one bit. A compare
// immediate that names a different bit drops the variable. A wider mask is kept.
func norm10(h loc, cmp, bits uint16) loc {
	if bits == 0 {
		return h
	}
	bit := -1
	for i := 0; i < 16; i++ {
		if bits == 1<<uint(i) {
			bit = i
			break
		}
	}
	if bit < 0 {
		h.mask = bits
		return h
	}
	if cmp != 0 {
		cbit := -1
		for i := 0; i < 16; i++ {
			if cmp == 1<<uint(i) {
				cbit = i
				break
			}
		}
		if cbit != bit {
			return loc{}
		}
	}
	h.mask = 1 << uint(bit)
	return h
}

func matchAdaption(ins []c166, rt int, dpp [4]uint16) []loc {
	names := []string{"rkaz_w", "rkat_w", "rkaz2_w", "rkat2_w"}
	last := len(ins) - 1
	idx := 0
	var out []loc
	i := 1
	for i < last {
		page := extpPage(ins[i])
		if page >= 0 {
			i++
			if i >= len(ins) {
				break
			}
		}
		if matchPat(ins[i], "F2FXXXXX") {
			nameAt := idx
			next := idx + 1
			if rt == 0x20 {
				nameAt = idx + 2
			}
			addr := Physical(dpp, ins[i].mem, page)
			if adaptionRange(addr) && nameAt >= 0 && nameAt < len(names) {
				h := one(ins[i], 2, 0, page, dpp)
				h.name = names[nameAt]
				out = append(out, h)
			}
			idx = next
			if next > 1 {
				return out
			}
		}
		i++
	}
	return out
}

func adaptionRange(addr uint32) bool {
	if addr&0xFFFF0000 == 0x380000 && addr <= 0x387FFF {
		return true
	}
	return addr >= 0xE000 && addr <= 0xFFFF
}

func matchF2Compare(ins []c166, dpp [4]uint16) (loc, bool) {
	last := len(ins) - 1
	if last < 2 {
		return loc{}, false
	}
	i := 1
	for {
		if i >= len(ins) {
			return loc{}, false
		}
		page := extpPage(ins[i])
		if page >= 0 {
			i++
			if i >= len(ins) {
				return loc{}, false
			}
		}
		next := i + 1
		if next < len(ins) && matchPat(ins[i], "F2FXXXXX") &&
			(matchPat(ins[next], "7C8X") || matchPat(ins[next], "22FXXXXX")) {
			return one(ins[i], 2, 0, page, dpp), true
		}
		i = next
		if last <= i {
			return loc{}, false
		}
	}
}

// alts is one instruction. Any pattern in the slice may match.
type alts []string

// A rule is one template in matchTemplates. min is the smallest index the last
// instruction may have. end aligns seq with that instruction; otherwise seq
// starts at the first instruction. load is the seq slot whose operand is the
// address. pgAt is one past the slot that holds an EXTP page, or 0 when the
// sequence does not. prior accepts a D740E100 immediately before seq.
type rule struct {
	min, load, size, pgAt int
	end, prior            bool
	seq                   []alts
	kind                  int
}

const (
	kindPlain = iota
	kindWide  // [011] a matching F2 four back is a word
	kindSFR   // [028] bit address, and a bit of 0x81 or more does not match
	kindLow   // [017] only an address below the flash base
	kindF3    // [023] scan ahead for F3 FX
	kindF2    // [029] scan ahead for F2 FX
)

// headRules see the whole case. tailRules see it after a leading EXTP is
// taken as the page. First match wins.
var headRules = []rule{
	{end: true, seq: []alts{{"F3FCXXXX"}}, size: 1, prior: true, kind: kindWide},
	{min: 2, end: true, seq: []alts{{"F2FXXXXX"}, {"7C8X"}, {"F1CX"}}, size: 2, prior: true},
	{min: 3, end: true, seq: []alts{{"C2FCXXXX"}, {"DA8Xxxxx"}, {"F1C8"}}, size: 1, prior: true},
	{min: 3, seq: []alts{{"E6F8XXXX", "E0X8"}, {"E7FEXXXX", "E1XE"}, {"F2F4XXXX"}}, load: 2, size: 2},
	{min: 4, seq: []alts{{"9AXXXXXX"}, {"E009"}, {"0DXX"}, {"F2F4XXXX"}}, load: 3, size: 2},
	{min: 3, seq: []alts{{"E7F8XXXX"}, {"F3FAXXXX"}, {"21A8"}}, load: 1, size: 1},
	{min: 3, seq: []alts{{"9AXXXXXX"}, {"8400XXXX"}}, load: 1, size: 2},
	{min: 4, seq: []alts{{"9AXXXXXX"}, {"D740E100"}, {"F2F4XXXX"}, {"B840"}}, load: 2, size: 2, pgAt: 2},
	{min: 3, seq: []alts{{"E6F8XXXX", "E0X8"}, {"E7FEXXXX", "E1XE"}, {"D2F4XXXX"}}, load: 2, size: 1},
	{min: 3, seq: []alts{{"9AXXXXXX"}, {"C2F4XXXX"}, {"B840"}}, load: 1, size: 1},
	{min: 2, seq: []alts{{"9AXXXXXX"}, {"F2F4XXXX"}}, load: 1, size: 2},
	{min: 3, seq: []alts{{"9AXXXXXX"}, {"E6F4XXXX", "E7F8XXXX", "E1X8"}, {"0DXX"}}, kind: kindSFR},
}

var tailRules = []rule{
	{seq: []alts{{"8400XXXX"}}, size: 2},
	{min: 7, seq: []alts{{"F2F4XXXX"}, {"9AXXXXXX"}, {"0DXX"}}, size: 1, kind: kindF3},
	{min: 7, seq: []alts{{"C2F4XXXX"}, {"66FXXXXX"}, {"EA20XXXX"}}, size: 2, kind: kindF2},
	{seq: []alts{{"F2F4XXXX"}}, size: 2},
	{seq: []alts{{"F3F8XXXX"}}, size: 1, kind: kindLow},
	{seq: []alts{{"D2F4XXXX"}}, size: 1},
	{seq: []alts{{"C2F4XXXX"}}, size: 1},
}

func matchCascade(ins []c166, dpp [4]uint16) (loc, bool) {
	if h, ok := runRules(ins, dpp, headRules, -1); ok {
		return h, true
	}
	page := extpPage(ins[0])
	if page >= 0 {
		ins = ins[1:]
	}
	return runRules(ins, dpp, tailRules, page)
}

func runRules(ins []c166, dpp [4]uint16, rules []rule, page int) (loc, bool) {
	if len(ins) == 0 {
		return loc{}, false
	}
	last := len(ins) - 1
	for _, r := range rules {
		if last < r.min {
			continue
		}
		at := 0
		if r.end {
			at = last - len(r.seq) + 1
		}
		if at < 0 || at+len(r.seq) > len(ins) || !matchSeq(ins[at:], r.seq) {
			continue
		}
		pg := page
		if r.prior && at > 0 {
			if p := extpPage(ins[at-1]); p >= 0 {
				pg = p
			}
		}
		if r.pgAt > 0 {
			pg = int(ins[at+r.pgAt-1].mem)
		}
		src := ins[at+r.load]
		switch r.kind {
		case kindWide:
			size := r.size
			if i := at + r.load; i > 3 && matchPat(ins[i-4], "F2F4XXXX") && ins[i-4].mem == src.mem {
				size = 2
			}
			return one(src, size, 0, pg, dpp), true
		case kindSFR:
			if ins[at].b2 >= 0x81 {
				continue
			}
			return loc{
				at:   ins[at].at,
				addr: 0xFD00 + 2*uint32(ins[at].b2),
				size: 2,
				mask: uint16(1) << (ins[at].mem >> 12),
			}, true
		case kindLow:
			h := one(src, r.size, 0, pg, dpp)
			if h.addr >= 0x800000 {
				continue
			}
			return h, true
		case kindF3, kindF2:
			pat := "F3FXXXXX"
			if r.kind == kindF2 {
				pat = "F2FXXXXX"
			}
			if h, ok := scanLoad(ins, at+len(r.seq), last, pat, r.size, dpp); ok {
				return h, true
			}
		default:
			return one(src, r.size, 0, pg, dpp), true
		}
	}
	return loc{}, false
}

func matchSeq(ins []c166, seq []alts) bool {
	for i, alt := range seq {
		hit := false
		for _, pat := range alt {
			if matchPat(ins[i], pat) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// scanLoad is the forward search in [023] and [029]. An EXTP clears and
// replaces the page for the following load. The page does not survive a
// later instruction that is not itself an EXTP.
func scanLoad(ins []c166, i, last int, pat string, size int, dpp [4]uint16) (loc, bool) {
	for i <= last {
		page := extpPage(ins[i])
		if page >= 0 {
			i++
			if i > last {
				break
			}
		}
		if matchPat(ins[i], pat) {
			return one(ins[i], size, 0, page, dpp), true
		}
		i++
	}
	return loc{}, false
}

func one(in c166, size int, mask uint16, page int, dpp [4]uint16) loc {
	return loc{at: in.at, addr: Physical(dpp, in.mem, page), size: size, mask: mask}
}

// extpPage is the page of a D740E100, or -1 when the instruction is not that EXTP.
func extpPage(in c166) int {
	if matchPat(in, "D740E100") {
		return int(in.mem)
	}
	return -1
}

// matchPat matches one stored instruction. parsePat accepts X, x, and ? as
// nibble wildcards. The pattern length must be the instruction length.
func matchPat(in c166, pat string) bool {
	if len(pat) == 0 || len(pat)%2 != 0 {
		return false
	}
	n := len(pat) / 2
	if n > 4 || in.n != n || len(in.raw) < n {
		return false
	}
	val := make([]byte, 4)
	mask := make([]byte, 4)
	for i := 0; i < n; i++ {
		hi, ok1 := patNibble(pat[i*2])
		lo, ok2 := patNibble(pat[i*2+1])
		if !ok1 || !ok2 {
			return false
		}
		if hi >= 0 {
			val[i] = byte(hi) << 4
			mask[i] = 0xF0
		}
		if lo >= 0 {
			val[i] |= byte(lo)
			mask[i] |= 0x0F
		}
	}
	if mask[0]&in.raw[0] != val[0] || mask[1]&in.raw[1] != val[1] {
		return false
	}
	if n == 4 {
		mw := binary.LittleEndian.Uint16(mask[2:4])
		vw := binary.LittleEndian.Uint16(val[2:4])
		if in.mem&mw != vw {
			return false
		}
	}
	return true
}

// patNibble returns 0..15, or -2 for a wildcard. ok is false for any other byte.
func patNibble(c byte) (int, bool) {
	switch {
	case c == '?' || c == 'x' || c == 'X':
		return -2, true
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	default:
		return 0, false
	}
}
