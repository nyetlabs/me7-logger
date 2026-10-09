// Package interp reads calibration-map addresses from calls to the
// interpolation functions named in config/needles.yaml.
// The needle finds the function. The call passes the map.
package interp

import (
	"encoding/binary"
	"sort"
	"strings"

	"go.nyet.org/me7-logger/needle"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

const (
	// Byte is the 8-bit table entry present on every image checked.
	Byte = "map_interp_table8"
	// ByteB is the second 8-bit table entry. Some images do not have it.
	ByteB = "map_interp_table8_b"
)

// Locate returns one map per address passed to a map_interp needle.
// Addr is the CPU address of the map body. Cols and the column axis are read
// from this image's header when R13 is that header. The row axis is the other
// header a setup stored into a RAM word this call loads. Body width stays
// unset until this image says what it is. An M-box size is not filled in.
// The Bosch name is not on the needle. It comes from which caller function
// made the call. calls names that caller slot. An empty list leaves every
// map unnamed. A slot whose CALLS goes to segment 0 with no needle label
// there takes its interpolator from the slot: on some images the segment 0
// entry is not the routine in the flash image.
func Locate(data []byte, ns []needle.Needle, dpp [4]uint16, calls []record.Call) []record.Map {
	var out []record.Map
	seen := map[uint32]int{}
	labels := map[string][]int{}
	done := map[int]bool{}
	add := func(call int, interp, name string) {
		done[call] = true
		r12, r13, r14, r15, hasR13, hasAxis, r13ram, r14ram, r15ram, hasR13Ram, hasR14Ram, hasR15Ram, setup, ok := mapArgs(data, call)
		if !ok {
			return
		}
		addr, ok := mapPtr(data, dpp, r12, r13, hasR13)
		if !ok {
			return
		}
		x, y, xOK, yOK := callAxes(data, dpp, r13, hasR13, r14, r15, hasAxis, r13ram, r14ram, r15ram, hasR13Ram, hasR14Ram, hasR15Ram, setup)
		if i, dup := seen[addr]; dup {
			if out[i].Name == "" && name != "" {
				out[i].Name = name
			}
			if out[i].X == nil && xOK {
				out[i].Cols = x.Count
				out[i].X = &x
			}
			if out[i].Y == nil && yOK {
				out[i].Rows = y.Count
				out[i].Y = &y
			}
			return
		}
		m := record.Map{Name: name, Addr: addr, Comment: interp}
		if xOK {
			m.Cols = x.Count
			m.X = &x
		}
		if yOK {
			m.Rows = y.Count
			m.Y = &y
		}
		seen[addr] = len(out)
		out = append(out, m)
	}
	for _, n := range ns {
		if !strings.HasPrefix(n.Name, "map_interp") {
			continue
		}
		for _, label := range n.Labels(data) {
			for _, call := range callsTo(data, label) {
				add(call, n.Name, slotName(data, ns, labels, calls, call, n.Name))
			}
		}
	}
	for _, c := range calls {
		for _, label := range callerLabels(data, ns, labels, c.Caller) {
			call := label + c.At
			if !done[call] && call >= 0 && call+4 <= len(data) && data[call] == 0xDA && data[call+1] == 0 {
				add(call, c.Interp, c.Name)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Addr < out[j].Addr })
	return out
}

// callAxes reads the column header this call can see, then the row header.
// R13 is the column header when it is not a page. When R13 is a page, R14 is
// the low 14 bits of the column header and R15 is its page. A load of R14, or
// of R13 when R13 is not an immediate, is the RAM word a setup stored that
// header into. Once the column is known, each RAM word this call loads is
// read the same way. A header whose values are the column axis is the column
// index. A different header is the row axis. Two different row headers leave
// Y unset. setup is a RAM word loaded into another register immediately
// before this frame. A curve keeps the one header that matches the column.
func callAxes(data []byte, dpp [4]uint16, r13 uint16, hasR13 bool, r14, r15 uint16, hasAxis bool, r13ram, r14ram, r15ram uint16, hasR13Ram, hasR14Ram, hasR15Ram bool, setup []uint16) (x, y record.Axis, xOK, yOK bool) {
	if hasR13 && !pageImm(r13) {
		x, xOK = axisOf(data, dpp, r13)
	} else if hasR13 && pageImm(r13) && hasAxis {
		x, xOK = axisAt(data, (uint32(r15)<<14)|uint32(r14&0x3FFF))
	}
	if !xOK && hasR14Ram {
		x, xOK = RamAxis(data, dpp, r14ram, 0)
	} else if !xOK && !hasR13 && hasR13Ram {
		x, xOK = RamAxis(data, dpp, r13ram, 0)
	}
	if !xOK {
		return x, y, false, false
	}
	var words [3]uint16
	n := 0
	if hasR13Ram {
		words[n] = r13ram
		n++
	}
	if hasR14Ram {
		words[n] = r14ram
		n++
	}
	if hasR15Ram {
		words[n] = r15ram
		n++
	}
	y, yOK = rowAxis(data, dpp, x, append(words[:n], setup...))
	return x, y, xOK, yOK
}

// rowAxis is a header stored into one of words, when its values are not the
// column axis. Two headers that name different values leave the row unset.
func rowAxis(data []byte, dpp [4]uint16, x record.Axis, words []uint16) (record.Axis, bool) {
	var y record.Axis
	seen := false
	for _, w := range words {
		ax, ok := RamAxis(data, dpp, w, 0)
		if !ok || ax.Addr == x.Addr {
			continue
		}
		if seen && ax.Addr != y.Addr {
			return record.Axis{}, false
		}
		y = ax
		seen = true
	}
	return y, seen
}

// RamAxis reads the header stored into ram.
// The store is MOV [ram], R4. In front of it the setup passes the header in
// R12. An R13 page immediate overrides the DPP. With no page, the top bits of
// the R12 immediate select the DPP. bits 8 or 16 is that header's width.
// bits 0 uses the zero-marker rule. Two setups that name different headers
// leave the axis unread.
func RamAxis(data []byte, dpp [4]uint16, ram uint16, bits int) (record.Axis, bool) {
	lo, hi := byte(ram), byte(ram>>8)
	var hdr uint32
	n := 0
	for i := 0; i+4 <= len(data); i += 2 {
		if data[i] != 0xF6 || data[i+1] != 0xF4 || data[i+2] != lo || data[i+3] != hi {
			continue
		}
		page, imm, ok := axisSetup(data, i, ram)
		if !ok {
			continue
		}
		ext := -1
		if page != 0 {
			ext = int(page)
		}
		addr := opcode.Physical(dpp, imm, ext)
		if n > 0 && addr != hdr {
			return record.Axis{}, false
		}
		hdr = addr
		n++
	}
	if n == 0 {
		return record.Axis{}, false
	}
	if bits == 8 || bits == 16 {
		return Breakpoints(data, hdr, bits)
	}
	return axisAt(data, hdr)
}

// Breakpoints reads a breakpoint table that is not in front of the map.
// addr is the count. bits 8 keeps a one-byte count and the values start on
// the next byte. bits 16 keeps a count word whose high byte is zero, and the
// values start two bytes later. The axis address is the first breakpoint.
func Breakpoints(data []byte, addr uint32, bits int) (record.Axis, bool) {
	addr, ok := inImage(data, addr)
	if !ok {
		return record.Axis{}, false
	}
	off := int(addr - opcode.FlashBase)
	n := int(data[off])
	if n < 1 || n > 24 || off+1 >= len(data) {
		return record.Axis{}, false
	}
	ax := record.Axis{Count: n, Bits: bits}
	if bits == 16 {
		if data[off+1] != 0 || off+2+n*2 > len(data) {
			return record.Axis{}, false
		}
		ax.Addr = addr + 2
		return ax, true
	}
	if off+1+n > len(data) {
		return record.Axis{}, false
	}
	ax.Addr = addr + 1
	return ax, true
}

// axisSetup is the immediate in front of a store to ram.
// page 0 means the header sits on DPP0. The call between the moves and the
// store is DA. The long form reloads ram just before that call. C2, F2 of
// R13 or R14, or an F0 register move may sit between MOV R12,#imm and that
// reload. Page 0 and an E6 FD page immediate are what e6Before reads. An
// EXTP in front of the store, the reload, or the move pages that memory word
// and is skipped.
func axisSetup(data []byte, store int, ram uint16) (page, imm uint16, ok bool) {
	store = skipExtp(data, store)
	if store < 12 || data[store-4] != 0xDA {
		return 0, 0, false
	}
	f2 := store - 8
	if f2 >= 4 && data[f2] == 0xF2 && (data[f2+1] == 0xFE || data[f2+1] == 0xFF) &&
		binary.LittleEndian.Uint16(data[f2+2:f2+4]) == ram {
		if at := skipExtp(data, f2) - 4; moveBefore(data, at) {
			if page, imm, ok = e6Before(data, skipExtp(data, at)); ok {
				return page, imm, true
			}
		}
		if at := skipExtp(data, f2) - 2; at >= 0 && data[at] == 0xF0 {
			if page, imm, ok = e6Before(data, at); ok {
				return page, imm, true
			}
		}
	}
	if data[f2] != 0xC2 && data[f2] != 0xF2 {
		return 0, 0, false
	}
	return e6Before(data, f2)
}

// skipExtp is at, or the EXTP #pag,#1 in front of it.
func skipExtp(data []byte, at int) int {
	if at >= 4 && data[at-4] == 0xD7 && data[at-3] == 0x40 {
		return at - 4
	}
	return at
}

// moveBefore is the instruction between the header immediate and the reload.
// C2 is that move on the setups already read. F2 of R13 or R14 is the same
// move when the input is a memory operand.
func moveBefore(data []byte, at int) bool {
	if at < 0 || at+2 > len(data) {
		return false
	}
	if data[at] == 0xC2 {
		return true
	}
	return data[at] == 0xF2 && (data[at+1] == 0xFD || data[at+1] == 0xFE)
}

// e6Before reads MOV R12,#imm, and MOV R13,#imm when it is the next instruction.
func e6Before(data []byte, at int) (page, imm uint16, ok bool) {
	e6 := at - 4
	if e6 < 0 || data[e6] != 0xE6 {
		return 0, 0, false
	}
	if data[e6+1] == 0xFC {
		return 0, binary.LittleEndian.Uint16(data[e6+2 : e6+4]), true
	}
	if e6 < 4 || data[e6+1] != 0xFD || data[e6-4] != 0xE6 || data[e6-3] != 0xFC {
		return 0, 0, false
	}
	return binary.LittleEndian.Uint16(data[e6+2 : e6+4]), binary.LittleEndian.Uint16(data[e6-2 : e6]), true
}

// slotName is the Bosch name of the caller slot at this CALLS.
// A slot matches when the CALLS is At bytes after the caller label and the
// callee is the named interpolation needle.
func slotName(data []byte, ns []needle.Needle, labels map[string][]int, calls []record.Call, call int, interp string) string {
	for _, c := range calls {
		if c.Interp != interp {
			continue
		}
		for _, label := range callerLabels(data, ns, labels, c.Caller) {
			if call-label == c.At {
				return c.Name
			}
		}
	}
	return ""
}

func callerLabels(data []byte, ns []needle.Needle, labels map[string][]int, name string) []int {
	if v, ok := labels[name]; ok {
		return v
	}
	var v []int
	for _, n := range ns {
		if n.Name != name {
			continue
		}
		v = append(v, n.Labels(data)...)
	}
	labels[name] = v
	return v
}

// callsTo finds word-aligned CALLS whose target file offset is label.
// CALLS is DA, segment, offset. The segment is the high byte of the CPU
// address. An entry in the first 64K is also reached with segment 0.
func callsTo(data []byte, label int) []int {
	if label < 0 || label+int(opcode.FlashBase) > 0xFFFFFF {
		return nil
	}
	segs := []byte{byte((int(opcode.FlashBase) + label) >> 16)}
	if label < 0x10000 && segs[0] != 0 {
		segs = append(segs, 0)
	}
	var out []int
	for _, seg := range segs {
		pat := [4]byte{0xDA, seg, byte(label), byte(label >> 8)}
		for i := 0; i+4 <= len(data); i += 2 {
			if data[i] == pat[0] && data[i+1] == pat[1] && data[i+2] == pat[2] && data[i+3] == pat[3] {
				out = append(out, i)
			}
		}
	}
	sort.Ints(out)
	return out
}

// mapArgs reads the argument moves in front of the call.
// The frame starts at MOV R12,#imm and reaches the call using only moves of
// R12-R15 (E6, F2, C2), a 4-byte F6, F7, or EXTP #pag,#1, or a 2-byte F0 or
// C0. The nearest such frame wins. hasR13 is set when MOV R13,#imm is in the frame. hasAxis
// is set when MOV R14,#imm and MOV R15,#imm are both in the frame. A MOV of
// R13, R14, or R15 from memory records that RAM word.
func mapArgs(data []byte, call int) (r12, r13, r14, r15 uint16, hasR13, hasAxis bool, r13ram, r14ram, r15ram uint16, hasR13Ram, hasR14Ram, hasR15Ram bool, setup []uint16, ok bool) {
	if call < 4 {
		return 0, 0, 0, 0, false, false, 0, 0, 0, false, false, false, nil, false
	}
	startMin := call - 20
	if startMin < 0 {
		startMin = 0
	}
	for start := call - 4; start >= startMin; start -= 2 {
		if !argFrame(data[start:call]) {
			continue
		}
		var hasR14, hasR15 bool
		for pos := 0; pos+4 <= call-start; {
			op := data[start+pos]
			if op != 0xE6 && op != 0xF2 && op != 0xC2 && op != 0xF6 && op != 0xF7 && op != 0xD7 {
				break
			}
			word := binary.LittleEndian.Uint16(data[start+pos+2 : start+pos+4])
			switch {
			case op == 0xE6 && data[start+pos+1] == 0xFC:
				r12 = word
			case op == 0xE6 && data[start+pos+1] == 0xFD:
				r13 = word
				hasR13 = true
			case op == 0xE6 && data[start+pos+1] == 0xFE:
				r14 = word
				hasR14 = true
			case op == 0xE6 && data[start+pos+1] == 0xFF:
				r15 = word
				hasR15 = true
			case op == 0xF2 && data[start+pos+1] == 0xFD:
				r13ram = word
				hasR13Ram = true
			case op == 0xF2 && data[start+pos+1] == 0xFE:
				r14ram = word
				hasR14Ram = true
			case op == 0xF2 && data[start+pos+1] == 0xFF:
				r15ram = word
				hasR15Ram = true
			}
			pos += 4
		}
		return r12, r13, r14, r15, hasR13, hasR14 && hasR15, r13ram, r14ram, r15ram, hasR13Ram, hasR14Ram, hasR15Ram, setupWords(data, start), true
	}
	return 0, 0, 0, 0, false, false, 0, 0, 0, false, false, false, nil, false
}

// setupWords is each RAM word loaded into R0–R11 immediately before the
// R12 frame, then the RAM word a setup stored when that store is the next
// instruction before those loads. 0x88 is the register move between the loads.
func setupWords(data []byte, frame int) []uint16 {
	var words []uint16
	pos := frame
	limit := frame - 24
	if limit < 0 {
		limit = 0
	}
	for pos-2 >= limit {
		if data[pos-2] == 0x88 {
			pos -= 2
			continue
		}
		if pos-4 < limit || data[pos-4] != 0xF2 || data[pos-3] < 0xF0 || data[pos-3] >= 0xFC {
			break
		}
		words = append(words, binary.LittleEndian.Uint16(data[pos-2:pos]))
		pos -= 4
	}
	if pos-4 >= limit && data[pos-4] == 0xF6 && data[pos-3] == 0xF4 {
		words = append(words, binary.LittleEndian.Uint16(data[pos-2:pos]))
	}
	return words
}

// argFrame reports whether b is MOV R12,#imm followed only by the moves that
// these interpolation calls use between that instruction and the CALLS.
func argFrame(b []byte) bool {
	if len(b) < 4 || b[0] != 0xE6 || b[1] != 0xFC {
		return false
	}
	pos := 0
	for pos < len(b) {
		op := b[pos]
		switch op {
		case 0xE6, 0xF2, 0xC2:
			if pos+4 > len(b) || b[pos+1] < 0xFC {
				return false
			}
			pos += 4
		case 0xF6, 0xF7:
			if pos+4 > len(b) {
				return false
			}
			pos += 4
		case 0xD7:
			if pos+4 > len(b) || b[pos+1] != 0x40 {
				return false
			}
			pos += 4
		case 0xF0, 0xC0:
			if pos+2 > len(b) {
				return false
			}
			pos += 2
		default:
			return false
		}
	}
	return true
}

// pageImm is an R13 immediate that selects a flash page rather than an axis
// header. On the benchmark image those values are 0x0206 and 0x0207.
func pageImm(imm uint16) bool {
	return imm >= 0x0200 && imm <= 0x02FF
}

// mapPtr resolves the pointer the call passed. A page immediate in R13 means
// the map is (page << 14) | (R12 & 0x3FFF). Otherwise R12 is a DPP address.
func mapPtr(data []byte, dpp [4]uint16, r12, r13 uint16, hasR13 bool) (uint32, bool) {
	page := -1
	if hasR13 && pageImm(r13) {
		page = int(r13)
	}
	return inImage(data, opcode.Physical(dpp, r12, page))
}

func inImage(data []byte, addr uint32) (uint32, bool) {
	if addr < opcode.FlashBase {
		return 0, false
	}
	if int(addr-opcode.FlashBase) >= len(data) {
		return 0, false
	}
	return addr, true
}

// axisOf reads the header at mem in this image. The first byte is the point
// count. A zero second byte marks a 16-bit axis whose values start two bytes
// later. Otherwise the values are bytes and start on the next byte. A header
// that does not match this shape is left unread. The M-box count and width
// are not substituted.
func axisOf(data []byte, dpp [4]uint16, mem uint16) (record.Axis, bool) {
	addr, ok := inImage(data, opcode.Physical(dpp, mem, -1))
	if !ok {
		return record.Axis{}, false
	}
	return axisAt(data, addr)
}

func axisAt(data []byte, addr uint32) (record.Axis, bool) {
	addr, ok := inImage(data, addr)
	if !ok {
		return record.Axis{}, false
	}
	off := int(addr - opcode.FlashBase)
	if off+1 >= len(data) {
		return record.Axis{}, false
	}
	bits := 8
	if data[off+1] == 0 {
		bits = 16
	}
	return Breakpoints(data, addr, bits)
}

// Packed reads a row count and a column count when they sit in front of the
// axes and the body, the way addMapAt does. header is the decoded pointer
// and body is the map. An odd header is the pad byte in front of the counts.
// The first count is the row axis and the second is the column axis. A 16-bit
// count or axis that would start on an odd address skips that byte. A span
// that matches none of those layouts, or more than one, is left unset.
func Packed(data []byte, header, body uint32) (rows, cols int, x, y record.Axis, ok bool) {
	if header < opcode.FlashBase || body <= header {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	if rows, cols, x, y, ok = packedAt(data, header, body, false); ok {
		return rows, cols, x, y, true
	}
	// The pointer can sit up to three bytes before the counts. An odd pointer
	// is the pad in front of them. One layout in that window is the header.
	var near []packedAxis
	for d := uint32(1); d <= 3 && header+d < body; d++ {
		if row, col, ax, ay, one := packedAt(data, header+d, body, false); one {
			near = append(near, packedAxis{row, col, ax, ay})
		}
	}
	if len(near) == 1 {
		return near[0].rows, near[0].cols, near[0].x, near[0].y, true
	}
	at := header
	if header&1 == 1 {
		at = header + 1
	}
	return packedAt(data, at, body, true)
}

// packedAt matches layouts between header and body. legacy is the three
// spans with no pad: byte counts and 8-bit axes, or word counts with 8-bit
// or 16-bit axes. The wider search also skips a pad in front of a 16-bit
// count or axis and allows the two axes to differ in width. That search
// keeps a match only when one layout accounts for the span.
func packedAt(data []byte, header, body uint32, wide bool) (rows, cols int, x, y record.Axis, ok bool) {
	if body <= header {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	off := int(header - opcode.FlashBase)
	end := int(body - opcode.FlashBase)
	if off < 0 || end > len(data) {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	var got []packedAxis
	for _, c1 := range []int{1, 2} {
		for _, c2 := range []int{1, 2} {
			for _, e1 := range []int{1, 2} {
				for _, e2 := range []int{1, 2} {
					if !wide && !legacyLayout(c1, c2, e1, e2) {
						continue
					}
					if wide && legacyLayout(c1, c2, e1, e2) {
						continue
					}
					row, col, ax, ay, ok := packLayout(data, header, off, end, c1, c2, e1, e2)
					if !ok {
						continue
					}
					if !wide {
						return row, col, ax, ay, true
					}
					got = append(got, packedAxis{row, col, ax, ay})
				}
			}
		}
	}
	if len(got) != 1 {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	return got[0].rows, got[0].cols, got[0].x, got[0].y, true
}

func legacyLayout(c1, c2, e1, e2 int) bool {
	if e1 != e2 {
		return false
	}
	if c1 == 1 && c2 == 1 && e1 == 1 {
		return true
	}
	return c1 == 2 && c2 == 2
}

type packedAxis struct {
	rows, cols int
	x, y       record.Axis
}

func packLayout(data []byte, header uint32, off, end, c1, c2, e1, e2 int) (rows, cols int, x, y record.Axis, ok bool) {
	yN, pos, ok := takeCount(data, off, c1)
	if !ok {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	xN, pos, ok := takeCount(data, pos, c2)
	if !ok {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	yPos := alignAxis(pos, e1)
	yEnd := yPos + yN*e1
	xPos := alignAxis(yEnd, e2)
	xEnd := xPos + xN*e2
	if xEnd != end && !(xEnd%2 == 1 && xEnd+1 == end) {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	if yPos < 0 || xEnd > len(data) {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	y = record.Axis{Addr: header + uint32(yPos-off), Count: yN, Bits: e1 * 8}
	x = record.Axis{Addr: header + uint32(xPos-off), Count: xN, Bits: e2 * 8}
	return yN, xN, x, y, true
}

func takeCount(data []byte, off, width int) (int, int, bool) {
	if width == 2 && off%2 == 1 {
		off++
	}
	if off < 0 || off+width > len(data) {
		return 0, 0, false
	}
	n := int(data[off])
	if width == 2 {
		n = int(binary.LittleEndian.Uint16(data[off : off+2]))
	}
	if !axisCount(n) {
		return 0, 0, false
	}
	return n, off + width, true
}

func alignAxis(off, elem int) int {
	if elem == 2 && off%2 == 1 {
		off++
	}
	return off
}

func axisCount(n int) bool {
	return n >= 1 && n <= 24
}

// Curve reads one axis in front of the body. That axis is the column axis.
// A 16-bit count or axis that would start on an odd address skips that byte.
// A span that matches none of those layouts, or more than one, is left unset.
// Rows stay unset: this does not invent a row count.
func Curve(data []byte, header, body uint32) (cols int, x record.Axis, ok bool) {
	if header < opcode.FlashBase || body <= header {
		return 0, record.Axis{}, false
	}
	off := int(header - opcode.FlashBase)
	end := int(body - opcode.FlashBase)
	if off < 0 || end > len(data) {
		return 0, record.Axis{}, false
	}
	var got []record.Axis
	for _, c := range []int{1, 2} {
		for _, e := range []int{1, 2} {
			n, pos, ok := takeCount(data, off, c)
			if !ok {
				continue
			}
			start := alignAxis(pos, e)
			axisEnd := start + n*e
			if axisEnd != end && !(axisEnd%2 == 1 && axisEnd+1 == end) {
				continue
			}
			if start < 0 || axisEnd > len(data) {
				continue
			}
			got = append(got, record.Axis{Addr: header + uint32(start-off), Count: n, Bits: e * 8})
		}
	}
	if len(got) != 1 {
		return 0, record.Axis{}, false
	}
	return got[0].Count, got[0], true
}

// Shaped reads axes prepended to the body for a known point count and width.
// The distance from the counts to the body is the length of that axis data.
// Rows 0 is one axis, returned as the column axis. A count width and a pad
// are the alignment of those dimensions. Two different axes for the same
// dimensions are left unset: the axes are not prepended.
func Shaped(data []byte, body uint32, rows, cols, yBits, xBits int) (int, int, record.Axis, record.Axis, bool) {
	if body <= opcode.FlashBase || cols < 1 || (xBits != 8 && xBits != 16) {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	if rows > 0 && yBits != 8 && yBits != 16 {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	base := cols * (xBits / 8)
	if rows > 0 {
		base += rows * (yBits / 8)
	}
	// Count bytes are one or two per axis. A 16-bit field skips an odd byte.
	var hit []prepended
	seen := map[[4]uint32]struct{}{}
	for d := base + 1; d <= base+8 && body >= opcode.FlashBase+uint32(d); d++ {
		header := body - uint32(d)
		var one prepended
		var ok bool
		if rows == 0 {
			one.cols, one.x, ok = shapedCurve(data, header, body, cols, xBits)
		} else {
			one.rows, one.cols, one.x, one.y, ok = shapedMap(data, header, body, rows, cols, yBits, xBits)
		}
		if !ok {
			continue
		}
		key := [4]uint32{one.x.Addr, uint32(one.x.Bits), one.y.Addr, uint32(one.y.Bits)}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		hit = append(hit, one)
	}
	if len(hit) != 1 {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	return hit[0].rows, hit[0].cols, hit[0].x, hit[0].y, true
}

type prepended struct {
	rows, cols int
	x, y       record.Axis
}

func shapedCurve(data []byte, header, body uint32, cols, xBits int) (int, record.Axis, bool) {
	off := int(header - opcode.FlashBase)
	end := int(body - opcode.FlashBase)
	if off < 0 || end > len(data) {
		return 0, record.Axis{}, false
	}
	e := xBits / 8
	var got []record.Axis
	for _, c := range []int{1, 2} {
		n, pos, ok := takeCount(data, off, c)
		if !ok || n != cols {
			continue
		}
		start := alignAxis(pos, e)
		axisEnd := start + n*e
		if axisEnd != end && !(axisEnd%2 == 1 && axisEnd+1 == end) {
			continue
		}
		got = append(got, record.Axis{Addr: header + uint32(start-off), Count: n, Bits: xBits})
	}
	if len(got) != 1 {
		return 0, record.Axis{}, false
	}
	return got[0].Count, got[0], true
}

func shapedMap(data []byte, header, body uint32, rows, cols, yBits, xBits int) (int, int, record.Axis, record.Axis, bool) {
	off := int(header - opcode.FlashBase)
	end := int(body - opcode.FlashBase)
	if off < 0 || end > len(data) {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	e1, e2 := yBits/8, xBits/8
	var got []prepended
	seen := map[[4]uint32]struct{}{}
	for _, c1 := range []int{1, 2} {
		for _, c2 := range []int{1, 2} {
			row, col, x, y, ok := packLayout(data, header, off, end, c1, c2, e1, e2)
			if !ok || row != rows || col != cols {
				continue
			}
			key := [4]uint32{x.Addr, uint32(x.Bits), y.Addr, uint32(y.Bits)}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			got = append(got, prepended{row, col, x, y})
		}
	}
	if len(got) != 1 {
		return 0, 0, record.Axis{}, record.Axis{}, false
	}
	return got[0].rows, got[0].cols, got[0].x, got[0].y, true
}
