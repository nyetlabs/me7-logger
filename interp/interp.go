// Package interp reads calibration-map addresses from calls to the
// interpolation functions named in config/needles.yaml.
// The needle finds the function. The call passes the map.
package interp

import (
	"encoding/binary"
	"sort"
	"strings"

	"me7-logger/needle"
	"me7-logger/opcode"
	"me7-logger/record"
)

const (
	// Byte is the 8-bit table entry present on every image checked.
	Byte = "map_interp_table8"
	// ByteB is the second 8-bit table entry. Some images do not have it.
	ByteB = "map_interp_table8_b"
)

// Locate returns one map per address passed to a map_interp needle.
// Addr is the CPU address of the map body. Cols and the axis width are read
// from this image's axis header when R13 is that header. Body width and row
// count stay unset until this image says what they are: another ECU, or a
// 16-bit patch of this one, can differ, and an M-box size is not filled in
// for them. The Bosch name is not on the needle. It comes from which caller
// function made the call. calls names that caller slot. An empty list leaves
// every map unnamed.
func Locate(data []byte, ns []needle.Needle, dpp [4]uint16, calls []record.Call) []record.Map {
	var out []record.Map
	seen := map[uint32]int{}
	labels := map[string][]int{}
	for _, n := range ns {
		if !strings.HasPrefix(n.Name, "map_interp") {
			continue
		}
		for _, label := range n.Labels(data) {
			for _, call := range callsTo(data, label) {
				r12, r13, hasR13, ok := mapArgs(data, call)
				if !ok {
					continue
				}
				addr, ok := mapPtr(data, dpp, r12, r13, hasR13)
				if !ok {
					continue
				}
				var ax record.Axis
				axOK := false
				if hasR13 && !pageImm(r13) {
					ax, axOK = axisOf(data, dpp, r13)
				}
				name := slotName(data, ns, labels, calls, call, n.Name)
				if i, dup := seen[addr]; dup {
					if out[i].Name == "" && name != "" {
						out[i].Name = name
					}
					if out[i].X == nil && axOK {
						out[i].Cols = ax.Count
						out[i].X = &ax
					}
					continue
				}
				m := record.Map{Name: name, Addr: addr, Comment: n.Name}
				if axOK {
					m.Cols = ax.Count
					m.X = &ax
				}
				seen[addr] = len(out)
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Addr < out[j].Addr })
	return out
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
	n, ok := needle.ByName(ns, name)
	if !ok {
		labels[name] = nil
		return nil
	}
	v := n.Labels(data)
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
// R12-R15 (E6, F2, C2), a 4-byte F6 or F7, or a 2-byte F0 or C0. The nearest
// such frame wins. hasR13 is set when the next instruction is MOV R13,#imm.
func mapArgs(data []byte, call int) (r12, r13 uint16, hasR13, ok bool) {
	if call < 4 {
		return 0, 0, false, false
	}
	startMin := call - 20
	if startMin < 0 {
		startMin = 0
	}
	for start := call - 4; start >= startMin; start -= 2 {
		if !argFrame(data[start:call]) {
			continue
		}
		r12 = binary.LittleEndian.Uint16(data[start+2 : start+4])
		if call-start >= 8 && data[start+4] == 0xE6 && data[start+5] == 0xFD {
			r13 = binary.LittleEndian.Uint16(data[start+6 : start+8])
			hasR13 = true
		}
		return r12, r13, hasR13, true
	}
	return 0, 0, false, false
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
	off := int(addr - opcode.FlashBase)
	n := int(data[off])
	if n < 1 || n > 24 || off+1 >= len(data) {
		return record.Axis{}, false
	}
	ax := record.Axis{Count: n}
	if data[off+1] == 0 {
		ax.Bits = 16
		ax.Addr = addr + 2
		if off+2+n*2 > len(data) {
			return record.Axis{}, false
		}
		return ax, true
	}
	ax.Bits = 8
	ax.Addr = addr + 1
	if off+1+n > len(data) {
		return record.Axis{}, false
	}
	return ax, true
}
