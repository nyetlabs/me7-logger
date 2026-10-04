package opcode

import (
	"encoding/binary"
	"fmt"
)

// Named is one variable the dedicated signature pass located.
// The address is kept only when it is RAM 0x380000–0x387FFF, near RAM
// 0xC000–0xDFFF, or SFR 0xE000–0xFFFF.
// A name the selector already stored is not overwritten; the caller drops it.
type Named struct {
	Name string
	Addr uint32
	Size int
}

// Signatures is the part of signVars that locates a variable from a fixed
// pattern, or from a pattern built out of an address already stored.
// known is that stored set: a name already present is not overwritten, and
// its address is what a later pattern embeds. Search is word-aligned from
// CPU 0x804000 through flashEnd-0x20. X, x, and ? are nibble wildcards.
// The first hit is kept. A unique search is dropped when a second copy exists.
func Signatures(img []byte, dpp [4]uint16, known map[string]uint32) []Named {
	if len(img) < 0x4020 {
		return nil
	}
	start := uint32(0x804000)
	end := FlashBase + uint32(len(img)) - 0x20
	have := map[string]uint32{}
	for name, addr := range known {
		have[name] = addr
	}
	var out []Named
	add := func(name string, addr uint32, size int) {
		if _, ok := have[name]; ok || !signAddr(addr) {
			return
		}
		have[name] = addr
		out = append(out, Named{Name: name, Addr: addr, Size: size})
	}
	if h := findPat(img, start, end, "9AXXXXXXF78EXXXXF68EXXXXF78EXXXXDB00", false); h != 0 {
		add("nmotll", PtrAt(img, h+6, dpp), 1)
		add("nmot_w", PtrAt(img, h+10, dpp), 2)
		add("nmot", PtrAt(img, h+0xe, dpp), 1)
	} else if h := findPat(img, start, end, "9AXXXXXXF68EXXXXF78EXXXXDB00", false); h != 0 {
		add("nmot_w", PtrAt(img, h+6, dpp), 2)
		add("nmot", PtrAt(img, h+10, dpp), 1)
	}
	plsolW(img, dpp, start, end, have, add)
	plgrus(img, dpp, start, end, have, add)
	pvdksFamily(img, dpp, start, end, have, add)
	pvdkPair(img, dpp, start, end, have, add)
	// findWithPrefix / findWithSuffix. The middle fragment is the string at
	// that call: F7FX at 00441326, F6FX at 00441375, F2FX at 004413ac,
	// 22FX at 004414c2, F3FX at 0044155a. A negative offset disassembles
	// the bytes before the hit. A suffix disassembles forward from it.
	relPrefix(img, dpp, start, end, have, add, "tabgm_w", 2, "7C8X", "F7FX", "tabgm", -1, false)
	relSuffix(img, dpp, start, end, have, add, "pu", 1, "F2FX", "ps_w", "7C7XF1XX", 3, true)
	relPrefix(img, dpp, start, end, have, add, "pu_w", 2, "7C7X", "F7FX", "pu", -1, false)
	relPrefix(img, dpp, start, end, have, add, "fho_w", 2, "7C8X", "F7FX", "fho", -1, false)
	relPrefix(img, dpp, start, end, have, add, "rl_w", 2, "F2FXXXXX7C5X", "F7FX", "rl", 0, true)
	relSuffix(img, dpp, start, end, have, add, "wped", 1, "F6FX", "wped_w", "7C8X", 2, true)
	if have["wped"] == 0 {
		relSuffix(img, dpp, start, end, have, add, "wped", 1, "F2FX", "wped_w", "7C8XF7FXxxxx", 2, true)
	}
	relSuffix(img, dpp, start, end, have, add, "pssol_w", 2, "F2FX", "pvdkdsl_w", "F2FXXXXX5CEX", 1, true)
	relSuffix(img, dpp, start, end, have, add, "vpsspls_w", 2, "F6FX", "pssol_w", "F2FXxxxx5CFX", 1, true)
	vpss(img, dpp, start, end, have, add, "vpsspu_w", "pu_w")
	vpss(img, dpp, start, end, have, add, "vpssplg_w", "plgrus_w")
	relPrefix(img, dpp, start, end, have, add, "rlroh_w", 2, "DB00F2FXxxxxF2FXxxxx", "22FX", "rl_w", 6, true)
	relPrefix(img, dpp, start, end, have, add, "fpvdk_w", 2, "F6FXxxxx7C8X", "F7FX", "fpvdk", 0, false)
	relPrefix(img, dpp, start, end, have, add, "ftvdk", 1, "C2FXxxxx5C8X", "F2FX", "fpvdk_w", 0, false)
	relSuffix(img, dpp, start, end, have, add, "zwnws", 1, "", "", "DA00xxxxF1E8F7FXxxxx", 2, false)
	if h := findPat(img, start, end, "F2FXxxxx66FXFF03F6FXXXXXECFX", true); h != 0 {
		add("uhfm_w", PtrAt(img, h+10, dpp), 2)
	}
	fnwue(img, dpp, start, end, add)
	lamfa(img, dpp, start, end, have, add)
	zwistChain(img, dpp, start, end, have, add)
	redistEvz(img, dpp, start, end, have, add)
	zwmnVars(img, dpp, end, have, add)
	wkraChain(img, dpp, start, end, have, add)
	return out
}

// zwistChain is the zwbasar, zwoutar, and zwmnms blocks. zwbasar and zwmnms
// embed zwist as F3FX. zwoutar is a fixed pair of patterns.
func zwistChain(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	if addr := have["zwist"]; addr != 0 {
		emb := embedAddr(addr, "F3FX")
		if h := findPat(img, start, end, emb+"43FXXXXX3DXX", true); h != 0 {
			addSeries(add, "zwbasar_%d", suffixDisasm(img, dpp, h, 1), 4)
		}
		h := findPat(img, start, end, emb+"43FXXXXXDDXX", true)
		if h == 0 {
			h = findPat(img, start, end, emb+"43FXXXXXADXX", true)
		}
		if h != 0 {
			add("zwmnms", suffixDisasm(img, dpp, h, 1), 1)
		}
	}
	// zwoutar's pattern sits below the usual 0x804000 search floor.
	h := findPat(img, FlashBase, end, "E6FDxxxxE6FC6000998D", true)
	off := uint32(2)
	if h == 0 {
		h = findPat(img, start, end, "E6FC6000E6FDxxxx998D", true)
		off = 6
	}
	if h != 0 {
		addSeries(add, "zwoutar_%d", PtrAt(img, h+off, dpp), 4)
	}
}

func addSeries(add func(string, uint32, int), name string, base uint32, n int) {
	for i := 0; i < n; i++ {
		add(fmt.Sprintf(name, i), base+uint32(i), 1)
	}
}

// redistEvz searches the DB00 window in front of the unique miist_w load.
// zwopt is the F3FA embed of zwist in that same window. evz_austot then
// embeds the redist address.
func redistEvz(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	lo, hi, ok := miistWindow(img, start, end, have["miist_w"])
	if !ok {
		return
	}
	if h := findPat(img, lo, hi, "C2F4xxxxF0545C25", false); h != 0 {
		add("redist", PtrAt(img, h+2, dpp), 1)
	}
	if addr := have["zwist"]; addr != 0 {
		if h := findPat(img, lo, hi, embedAddr(addr, "F3FA")+"F3FXxxxx21XX", false); h != 0 {
			add("zwopt", PtrAt(img, h+6, dpp), 1)
		}
	}
	addr := have["redist"]
	if addr == 0 {
		return
	}
	pat := "C2FXxxxxC0XX60XX2D02" + embedAddr(addr, "258F")
	if h := findPat(img, start, end, pat, true); h != 0 {
		add("evz_austot", PtrAt(img, h+2, dpp), 1)
	}
}

// vpss is the signVars pair that embeds pssol_w and either pu_w or plgrus_w.
// The first unique copy wins. Instruction 0xC onward is the first F6FX, and
// an EXTP earlier in that window supplies its page.
func vpss(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int), name, other string) {
	pssol, addr := have["pssol_w"], have[other]
	if pssol == 0 || addr == 0 {
		return
	}
	a, b := embedAddr(pssol, "F2FX"), embedAddr(addr, "F2FX")
	h := findPat(img, start, end, a+"E00XF0XX5CEX5CEX7C2X70XX"+b+"DAxxxxxx", true)
	if h == 0 {
		h = findPat(img, start, end, b+a+"5CEX"+a+"7C2XF6FX0CFEF6FX0EFE7BXX", true)
	}
	if h == 0 {
		return
	}
	if got := f6From(img, dpp, h); signAddr(got) {
		add(name, got, 2)
	}
}

// f6From disassembles 0x40 bytes and returns the first F6FX at instruction
// 0xC or later. An EXTP applies only to the instruction that follows it.
func f6From(img []byte, dpp [4]uint16, hit uint32) uint32 {
	ins := walkInsns(img, hit, hit+0x40)
	page := -1
	for i := range ins {
		p := page
		page = -1
		if matchPat(ins[i], "D74XXXXX") {
			page = int(ins[i].mem)
			continue
		}
		if i < 0xc || !matchPat(ins[i], "F6FXxxxx") {
			continue
		}
		return Physical(dpp, ins[i].mem, p)
	}
	return 0
}

// zwmnVars is the scanZwmn pair that follows zwmnms. The signature is unique
// from 0x820000. zwspae is the F3FX embed of zwopt in that DB00 window, and
// zwsol is the 43FX embed of zwspae. Slot 0x17 is the calls word clock()
// selects: 0x78B8, 0, or 0x2B24.
func zwmnVars(img []byte, dpp [4]uint16, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	zwmnms, zwopt := have["zwmnms"], have["zwopt"]
	if zwmnms == 0 || zwopt == 0 {
		return
	}
	call := slot17(img, FlashBase+uint32(len(img)))
	if call == "" {
		return
	}
	const from = uint32(0x820000)
	sig := "E6FCxxxxE6FDxxxxF2FExxxxF2FFxxxx" + call + embedAddr(zwmnms, "F7F8")
	h := findPat(img, from, end, sig, true)
	if h == 0 {
		return
	}
	db := findLast(img, from, h, "DB00")
	if db == 0 {
		return
	}
	lo := db + 2
	hi := findPat(img, lo, end, "DB00", false)
	if hi == 0 {
		return
	}
	h = findPat(img, lo, hi, "F3FXxxxx"+embedAddr(zwopt, "F3FX")+"21XX", false)
	if h == 0 {
		return
	}
	spae := PtrAt(img, h+2, dpp)
	add("zwspae", spae, 1)
	if spae == 0 {
		return
	}
	h = findPat(img, from, end, "F3FXxxxx"+embedAddr(spae, "43FX")+"DDXX", true)
	if h != 0 {
		add("zwsol", PtrAt(img, h+2, dpp), 1)
	}
}

// bootVer is the table clock() selects: 0x602, 0x402, or 0 for the default.
// A conflict between the three call counts also selects the default.
func bootVer(img []byte, end uint32) uint32 {
	from := uint32(0x820000)
	n602 := countPat(img, from, end, callPat(0x0F62))
	n512 := countPat(img, from, end, callPat(0x1342))
	n402 := countPat(img, from, end, callPat(0x120A))
	conflict := (n512 > 0x1e || n402 > 0x14) && n602 > 0x1e || n512 > 0x1e && n402 > 0x14
	if conflict {
		return 0
	}
	switch {
	case n402 > 0x14:
		return 0x402
	case n602 > 0x1e:
		return 0x602
	}
	return 0
}

// slot17 is callsSlot(0x17) after clock() has chosen the bootrom table.
func slot17(img []byte, end uint32) string {
	word := uint32(0x78B8)
	switch bootVer(img, end) {
	case 0x402:
		word = 0
	case 0x602:
		word = 0x2B24
	}
	if word == 0 {
		return ""
	}
	return callPat(word)
}

func callPat(addr uint32) string {
	return fmt.Sprintf("DA%02X%02X%02X", byte(addr>>16), byte(addr), byte(addr>>8))
}

func countPat(img []byte, start, end uint32, pattern string) int {
	n := 0
	for start+4 <= end {
		h := findPat(img, start, end, pattern, false)
		if h == 0 {
			return n
		}
		n++
		start = h + 4
	}
	return n
}

// miistWindow is the open range after the DB00 before the miist_w load, up to
// the next DB00. signVars searches redist and zwopt inside it.
func miistWindow(img []byte, start, end, addr uint32) (uint32, uint32, bool) {
	if addr == 0 {
		return 0, 0, false
	}
	h := findPat(img, start, end, embedAddr(addr, "F6FX"), true)
	if h == 0 {
		return 0, 0, false
	}
	db := findLast(img, start, h, "DB00")
	if db == 0 {
		return 0, 0, false
	}
	lo := db + 2
	hi := findPat(img, lo, end, "DB00", false)
	if hi == 0 {
		return 0, 0, false
	}
	return lo, hi, true
}

// wkraChain is the 46F4A000 window. The first E48 is wkra_0, the next is the
// wkr array, and the one after that is zkrvf. zzylkr and wkraa embed wkra_0.
// The wkr and wkraa counts are the cylinder count.
func wkraChain(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	h := findPat(img, start, end, "46F4A000CDFxDAxxxxxx", true)
	if h == 0 {
		return
	}
	db := findLast(img, start, h, "DB00")
	if db == 0 {
		return
	}
	base := db + 2
	ins := walkInsns(img, base, base+0x50)
	var at []int
	for i := range ins {
		if matchPat(ins[i], "E48Xxxxx") {
			at = append(at, i)
		}
	}
	if len(at) == 0 {
		return
	}
	addr := dppAt(ins, at[0], -1, dpp)
	add("wkra_0", addr, 1)
	add("wkra_159", addr+0x9F, 1)
	const cyl = 8 // DAT_005009ec counts up to eight slots
	if len(at) > 1 {
		addSeries(add, "wkr_%d", dppAt(ins, at[1], -1, dpp), cyl)
	}
	if len(at) > 2 {
		addSeries(add, "zkrvf_%d", dppAt(ins, at[2], -1, dpp), 8)
	}
	wk := have["wkra_0"]
	if wk == 0 {
		return
	}
	h = findPat(img, start, end, embedAddr(wk, "F4A4")+"C2F4xxxxE4A4xxxx", true)
	if h == 0 {
		return
	}
	ins = walkInsns(img, h, h+0x10)
	zz, idx := suffixAt(ins, 1, dpp)
	add("zzylkr", zz, 1)
	b := idx + 1
	page := -1
	if b >= 0 && b < len(ins) && matchPat(ins[b], "D74XXXXX") {
		page = int(ins[b].mem)
		b++
	}
	addSeries(add, "wkraa_%d", dppAt(ins, b, page, dpp), cyl)
}

// plgrus follows plsol_w. The first pattern disassembles eight bytes before
// the hit and the word load after E00X. The fallback disassembles forward.
// plsolr_w is the word load after an E00X in that same window.
func plgrus(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	addr := have["plsol_w"]
	if addr == 0 {
		return
	}
	emb := embedAddr(addr, "F2FX")
	if h := findPat(img, start, end, emb+"20XX9D01E00X", true); h != 0 {
		ins := walkInsns(img, h-8, h+0x14)
		add("plgrus_w", prefixPick(ins, "D740XXXX", dpp), 2)
		plsolr(ins, dpp, add)
		return
	}
	if h := findPat(img, start, end, emb+"22FXXXXX9D01E00X", true); h != 0 {
		ins := walkInsns(img, h, h+0x20)
		addr, _ := suffixAt(ins, 1, dpp)
		add("plgrus_w", addr, 2)
		plsolr(ins, dpp, add)
	}
}

func plsolr(ins []c166, dpp [4]uint16, add func(string, uint32, int)) {
	for i := 3; i < len(ins)-1; i++ {
		if matchPat(ins[i], "E00X") {
			add("plsolr_w", dppAt(ins, i+1, -1, dpp), 2)
			return
		}
	}
}

// pvdksFamily is the unique F6 chain around pvdks_w. pvdkdsl_w is what
// pssol_w embeds later.
func pvdksFamily(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	addr := have["pvdks_w"]
	if addr == 0 {
		return
	}
	pat := "F6FXxxxx" + embedAddr(addr, "F6FX") + "F6FXxxxxF6FXxxxx7C8XF7FXxxxx"
	h := findPat(img, start, end, pat, true)
	if h == 0 {
		return
	}
	add("pvdksf_w", PtrAt(img, h+2, dpp), 2)
	add("pvdkdsl_w", PtrAt(img, h+10, dpp), 2)
	add("pvdkds", PtrAt(img, h+0x14, dpp), 1)
}

// pvdkPair is the first F6/7C8X/F7 chain that embeds pvdkds_w.
func pvdkPair(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	addr := have["pvdkds_w"]
	if addr == 0 {
		return
	}
	h := findPat(img, start, end, "F6FXxxxx7C8XF7FXxxxx"+embedAddr(addr, "F2FX"), false)
	if h == 0 {
		return
	}
	add("pvdk_w", PtrAt(img, h+2, dpp), 2)
	add("pvdk", PtrAt(img, h+8, dpp), 1)
}

// lamfa is the long E6FX0010 chain around lamfa_w. The second pattern is
// the same chain with an EXTP before each F6.
func lamfa(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	addr := have["lamfa_w"]
	if addr == 0 {
		return
	}
	emb := embedAddr(addr, "F6FX")
	tail := "E6FX0010F6FXxxxxE6FX0010F6FXxxxxE6FX0010F6FXxxxxE6FX0010F6FXxxxxE6FX0010F6FXxxxx"
	if h := findPat(img, start, end, "E6FX0010"+emb+tail, true); h != 0 {
		add("lamfaws_w", PtrAt(img, h+0xe, dpp), 2)
		add("lamfawkr_w", PtrAt(img, h+0x16, dpp), 2)
		add("lamfaw_w", PtrAt(img, h+0x1e, dpp), 2)
		add("lamfwl_w", PtrAt(img, h+0x26, dpp), 2)
		add("lamrlmn_w", PtrAt(img, h+0x2e, dpp), 2)
		return
	}
	ext := "E6FX0010D740E100F6FXxxxx"
	h := findPat(img, start, end, "E6FX0010"+emb+ext+ext+ext+ext+ext, true)
	if h == 0 {
		return
	}
	add("lamfaws_w", PtrAt(img, h+0x16, dpp), 2)
	add("lamfawkr_w", PtrAt(img, h+0x22, dpp), 2)
	add("lamfaw_w", PtrAt(img, h+0x2e, dpp), 2)
	add("lamfwl_w", PtrAt(img, h+0x3a, dpp), 2)
	add("lamrlmn_w", PtrAt(img, h+0x46, dpp), 2)
}

func plsolW(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int)) {
	if addr := have["plsol"]; addr != 0 {
		pat := "F2FXXXXX7C8X" + embedAddr(addr, "F7FX")
		if h := findPat(img, start, end, pat, false); h != 0 {
			add("plsol_w", PtrAt(img, h+2, dpp), 2)
			return
		}
	}
	if h := findPat(img, start, end, "F6FXXXXX0D029400XXXXF2FXXXXX7C8XF7FXXXXX", false); h != 0 {
		add("plsol_w", PtrAt(img, h+2, dpp), 2)
	}
}

// relPrefix is findWithPrefix. at < 0 disassembles the eight bytes before
// the hit. Otherwise the address word is at hit+at+2.
func relPrefix(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int), name string, size int, prefix, mid, prior string, at int, single bool) {
	addr := have[prior]
	if addr == 0 {
		return
	}
	h := findPat(img, start, end, prefix+embedAddr(addr, mid), single)
	if h == 0 {
		return
	}
	if at < 0 {
		add(name, prefixPick(walkInsns(img, h-8, h+4), "D74XXXXX", dpp), size)
		return
	}
	add(name, PtrAt(img, h+uint32(at)+2, dpp), size)
}

// relSuffix is findWithSuffix. The pattern is the embedded address plus the
// suffix, or just the suffix when there is no prior name. The address is
// dppAddr of an instruction in the 0x20 bytes at the hit.
func relSuffix(img []byte, dpp [4]uint16, start, end uint32, have map[string]uint32, add func(string, uint32, int), name string, size int, mid, prior, suffix string, at int, single bool) {
	pat := suffix
	if prior != "" {
		addr := have[prior]
		if addr == 0 {
			return
		}
		pat = embedAddr(addr, mid) + suffix
	}
	h := findPat(img, start, end, pat, single)
	if h == 0 {
		return
	}
	add(name, suffixDisasm(img, dpp, h, at), size)
}

// prefixPick selects the second instruction, or the third when the first is
// two bytes. An EXTP matching extp on the first instruction supplies the page.
func prefixPick(ins []c166, extp string, dpp [4]uint16) uint32 {
	if len(ins) == 0 {
		return 0
	}
	page := -1
	if matchPat(ins[0], extp) {
		page = int(ins[0].mem)
	}
	idx := 1
	if ins[0].n == 2 {
		idx = 2
	}
	return dppAt(ins, idx, page, dpp)
}

// suffixDisasm is findWithSuffix. An EXTP at the hit moves the index one
// ahead. An EXTP at that index supplies the page and the address is the
// following instruction.
func suffixDisasm(img []byte, dpp [4]uint16, hit uint32, at int) uint32 {
	addr, _ := suffixAt(walkInsns(img, hit, hit+0x20), at, dpp)
	return addr
}

func suffixAt(ins []c166, at int, dpp [4]uint16) (uint32, int) {
	idx := at
	if len(ins) > 0 && matchPat(ins[0], "D74XXXXX") {
		idx++
	}
	page := -1
	if idx >= 0 && idx < len(ins) && matchPat(ins[idx], "D74XXXXX") {
		page = int(ins[idx].mem)
		idx++
	}
	return dppAt(ins, idx, page, dpp), idx
}

// dppAt is dppAddr. page >= 0 is the EXTP page from setPage.
func dppAt(ins []c166, idx, page int, dpp [4]uint16) uint32 {
	if idx < 0 || idx >= len(ins) || ins[idx].n != 4 {
		return 0
	}
	return Physical(dpp, ins[idx].mem, page)
}

func walkInsns(img []byte, from, until uint32) []c166 {
	var out []c166
	pc := from
	for len(out) < 0x400 && pc < until {
		if pc < FlashBase {
			break
		}
		i := int(pc - FlashBase)
		if i < 0 || i >= len(img) {
			break
		}
		n := int(c166Len[img[i]])
		if n == 0 || i+n > len(img) {
			break
		}
		in := c166{at: i, n: n, op: img[i], b2: img[i+1], raw: img[i : i+n]}
		if n == 4 {
			in.mem = binary.LittleEndian.Uint16(img[i+2 : i+4])
		}
		out = append(out, in)
		pc += uint32(n)
	}
	return out
}

// embedAddr is the EXTP-or-DPP text inserted for a stored address.
// DPP3, DPP2, DPP1, and DPP0 are the page tests 3, 0xE0, 0x205, and 0x204.
func embedAddr(addr uint32, mid string) string {
	off := addr & 0x3FFF
	page := addr >> 14
	word := off
	extp := ""
	switch page {
	case 3:
		word = off | 0xC000
	case 0xE0:
		word = off | 0x8000
	case 0x205:
		word = off | 0x4000
	case 0x204:
	default:
		extp = fmt.Sprintf("D740%02X%02X", byte(page), byte(page>>8))
	}
	return extp + mid + fmt.Sprintf("%02X%02X", byte(word), byte(word>>8))
}

func fnwue(img []byte, dpp [4]uint16, start, end uint32, add func(string, uint32, int)) {
	h := findPat(img, start, end, "E6FXFFFF7C8XD740E100F7FXXXXXD740E100F78EXXXX", true)
	if h != 0 {
		h += 0xc
	} else if h = findPat(img, start, end, "F2FX0EFE0D02E6FXFFFF7C8XD740E100F7FXxxxxF2FXxxxx", true); h != 0 {
		h += 0x12
	} else if h = findPat(img, start, end, "F2FX0EFE0D02E6FXFFFF7C8XF7FXxxxxF2FXxxxx", true); h != 0 {
		h += 0xe
	} else if h = findPat(img, start, end, "E6FXFFFF7C8XF7FXXXXXF78EXXXX", true); h != 0 {
		h += 8
	} else if h = findPat(img, start, end, "E6FXFFFFF0XX46FXFF00CD03E7FXFF000D01F0XXF7FXxxxx", true); h != 0 {
		h += 0x16
	} else if h = findPat(img, start, end, "E6FXFFFF46FXFF009D04F0XXF7FXxxxx", true); h != 0 {
		h += 0xe
	} else {
		return
	}
	add("fnwue", PtrAt(img, h, dpp), 1)
}

func signAddr(addr uint32) bool {
	if addr >= 0x380000 && addr <= 0x387FFF {
		return true
	}
	if addr >= 0xC000 && addr <= 0xDFFF {
		return true
	}
	return addr >= 0xE000 && addr <= 0xFFFF
}

// PtrAt reads the word at a CPU address. An EXTP (D7 40) six bytes earlier
// supplies the page. Otherwise the word is dppAddr.
func PtrAt(img []byte, addr uint32, dpp [4]uint16) uint32 {
	if addr < FlashBase {
		return 0
	}
	off := int(addr - FlashBase)
	if off < 0 || off+2 > len(img) {
		return 0
	}
	mem := binary.LittleEndian.Uint16(img[off : off+2])
	if off >= 6 && img[off-6] == 0xD7 && img[off-5]&0xC0 == 0x40 {
		page := binary.LittleEndian.Uint16(img[off-4 : off-2])
		return (uint32(page) << 14) | uint32(mem&0x3FFF)
	}
	return Physical(dpp, mem, -1)
}

func findLast(img []byte, start, end uint32, pattern string) uint32 {
	return scanPat(img, start, end, pattern, false, true)
}

// findPat returns the CPU address of a word-aligned hit. single is findSingle:
// a second copy makes the result 0. Otherwise the first hit is returned.
func findPat(img []byte, start, end uint32, pattern string, single bool) uint32 {
	return scanPat(img, start, end, pattern, single, false)
}

func scanPat(img []byte, start, end uint32, pattern string, single, last bool) uint32 {
	pat, mask, ok := compilePat(pattern)
	if !ok || start >= end {
		return 0
	}
	lo, hi := patSpan(img, start, end)
	lim := hi - len(pat)
	var hit uint32
	n := 0
	for i := lo; i <= lim; i += 2 {
		if !matchBytes(img[i:i+len(pat)], pat, mask) {
			continue
		}
		n++
		hit = FlashBase + uint32(i)
		if last {
			continue
		}
		if !single {
			return hit
		}
		if n > 1 {
			return 0
		}
	}
	if n == 0 || (single && n != 1) {
		return 0
	}
	return hit
}

func patSpan(img []byte, start, end uint32) (lo, hi int) {
	if start > FlashBase {
		lo = int(start - FlashBase)
	}
	hi = len(img)
	if end > FlashBase {
		if e := int(end - FlashBase); e < hi {
			hi = e
		}
	}
	if lo < 0 {
		lo = 0
	}
	if lo%2 != 0 {
		lo++
	}
	return lo, hi
}

func matchBytes(b, pat, mask []byte) bool {
	for i := range pat {
		if b[i]&mask[i] != pat[i] {
			return false
		}
	}
	return true
}

func compilePat(s string) (pat, mask []byte, ok bool) {
	if len(s) == 0 || len(s)%2 != 0 {
		return nil, nil, false
	}
	pat = make([]byte, len(s)/2)
	mask = make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, ok1 := patNibble(s[i])
		lo, ok2 := patNibble(s[i+1])
		if !ok1 || !ok2 {
			return nil, nil, false
		}
		if hi >= 0 {
			pat[i/2] |= byte(hi) << 4
			mask[i/2] |= 0xF0
		}
		if lo >= 0 {
			pat[i/2] |= byte(lo)
			mask[i/2] |= 0x0F
		}
	}
	return pat, mask, true
}
