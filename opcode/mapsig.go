package opcode

import (
	"encoding/binary"
	"slices"
	"strings"
	"unicode"

	"go.nyet.org/me7-logger/needle"
)

// MapHit is one calibration map located from a mapsig row.
// Addr is the CPU address of the map body. Header is the decoded pointer
// when that pointer is not the body, so a packed count in front of the
// body can be read. Header 0 means the pointer is the body.
type MapHit struct {
	Name   string
	Addr   uint32
	Header uint32
	// Rows, Cols, XBits, and YBits are the axis dimensions. Cols 0 means this
	// hit does not give them. XRam and YRam are RAM words whose setup stored
	// the column and row headers. Zero means this hit does not name that word.
	Rows  int
	Cols  int
	XBits int
	YBits int
	XRam  uint16
	YRam  uint16
	// XTab and YTab are breakpoint tables named by the row. Each is the CPU
	// address of that table's count byte. Zero means this hit does not name one.
	// Plain means Rows and Cols are the shape when no axis sits in front of the body.
	XTab  uint32
	YTab  uint32
	Plain bool
	// YOrd and Bits are the MapSig fields.
	YOrd bool
	Bits int
}

// MapAddrs runs the mapsig rows. A pattern with no window is kept only when
// it occurs once and the address word decodes inside this image. A row with
// open searches the DB00 window around that opener and keeps the first hit
// there. A table row is the breakpoint table: its address is the hit, and it
// is not a map. An anchor is a byte distance from a map this pass already
// located. A later row with the same name fills it only when an earlier
// window missed. These addresses are flash maps, so the RAM filter used by
// the logging signatures does not apply.
func MapAddrs(img []byte, dpp [4]uint16, rows []MapSig) []MapHit {
	return mapHits(img, dpp, rows, nil, func(string) (string, bool) { return "", false })
}

// MapHits is MapAddrs with the measurement addresses and call slots a family
// finder splices into its pattern. known is not overwritten. A map located
// here can be spliced into a later row.
func MapHits(img []byte, dpp [4]uint16, doc SigDoc, known map[string]uint32) []MapHit {
	return mapHits(img, dpp, doc.Maps, known, func(id string) (string, bool) {
		set, ok := doc.Calls[id]
		if !ok || len(img) == 0 {
			return "", false
		}
		word := set.word(bootVer(img, FlashBase+uint32(len(img))))
		if word == 0 {
			return "", false
		}
		return callPat(word), true
	})
}

func mapHits(img []byte, dpp [4]uint16, rows []MapSig, known map[string]uint32, slot func(string) (string, bool)) []MapHit {
	if len(img) == 0 || len(rows) == 0 {
		return nil
	}
	have := map[string]int{}
	embed := map[string]uint32{}
	for name, addr := range known {
		embed[name] = addr
	}
	tabs := TableAddrs(img, rows)
	won := map[string]MapSig{}
	paths := map[string]int{}
	var out []MapHit
	add := func(name string, addr, header uint32, row MapSig, xram, yram uint16) bool {
		if _, ok := have[name]; ok {
			return false
		}
		if _, ok := inFlash(img, addr); !ok {
			return false
		}
		have[name] = len(out)
		won[name] = row
		if _, ok := embed[name]; !ok {
			embed[name] = addr
		}
		out = append(out, MapHit{
			Name: name, Addr: addr, Header: header,
			Rows: row.Rows, Cols: row.Cols, XBits: row.XBits, YBits: row.YBits,
			XRam: xram, YRam: yram, Plain: row.Plain, YOrd: row.YOrd, Bits: row.Bits,
		})
		return true
	}
	end := FlashBase + uint32(len(img))
	for _, row := range rows {
		if row.Code != 0 {
			base, ok := have[row.Anchor]
			tab, tok := have[row.Index]
			if !ok || !tok {
				continue
			}
			if path, ok := pathOf(img, out[tab].Addr, paths[row.Anchor], row.Code); ok {
				add(row.Name, out[base].Addr+uint32(path), 0, row, 0, 0)
			}
			continue
		}
		if row.Table || row.Pattern == "" {
			continue
		}
		if row.Absent != "" {
			absent, ok := expandSig(row.Absent, embed, slot)
			if !ok || findPat(img, FlashBase, end, absent, false) != 0 {
				continue
			}
		}
		var ptr, lo, hi uint32
		var off int
		if mapScoped(row) {
			var hit uint32
			hit, lo, hi = scopedHit(img, end, row, embed, slot)
			if hit == 0 {
				continue
			}
			off = int(hit) - int(FlashBase)
			ptr = decodeMapPtr(img, dpp, ptrOff(hit, row.At), row)
		} else {
			pat, mask, ok := compilePat(row.Pattern)
			if !ok {
				continue
			}
			at, ok := findOne(img, pat, mask)
			if !ok {
				continue
			}
			off = at
			ptr = PtrAt(img, ptrOff(FlashBase+uint32(at), row.At), dpp)
		}
		addr, header := shift(img, ptr, row.Add)
		if addr == 0 {
			continue
		}
		if row.Also {
			header = 0
		}
		if add(row.Name, addr, header, row, f2Word(img, off, row.XAt), f2Word(img, off, row.YAt)) && row.Count != "" {
			if h := findPat(img, lo, hi, row.Count, false); h != 0 {
				paths[row.Name] = int(binary.LittleEndian.Uint16(img[h-FlashBase+2:])) + 1
			}
		}
	}
	for again := true; again; {
		again = false
		for _, row := range rows {
			if row.Anchor == "" || row.Code != 0 {
				continue
			}
			if _, ok := have[row.Name]; ok {
				continue
			}
			base, ok := have[row.Anchor]
			if !ok {
				continue
			}
			addr, _ := shift(img, out[base].Addr, row.Add)
			if addr == 0 {
				continue
			}
			add(row.Name, addr, 0, row, out[base].XRam, out[base].YRam)
			again = true
		}
	}
	for name, idx := range have {
		row := won[name]
		if row.XTable != "" {
			out[idx].XTab = tabs[row.XTable]
		}
		if row.YTable != "" {
			out[idx].YTab = tabs[row.YTable]
		}
	}
	return slices.DeleteFunc(out, func(h MapHit) bool { return helperName(h.Name) })
}

// helperName is a lowercase row name. Later rows anchor to it; it is not a map.
func helperName(name string) bool {
	return name != "" && unicode.IsLower(rune(name[0]))
}

// pathOf is the path whose four words at tab hold code first. The search
// starts at path 1 and stops after path n, as scanCla does.
func pathOf(img []byte, tab uint32, n int, code uint16) (int, bool) {
	off := int64(tab) - int64(FlashBase)
	if off < 0 || n <= 0 {
		return 0, false
	}
	for a := off + 8; a <= off+8*int64(n) && a+2 <= int64(len(img)); a += 2 {
		if binary.LittleEndian.Uint16(img[a:]) == code {
			return int((a - off + 1) >> 3), true
		}
	}
	return 0, false
}

// mapScoped is a row the family finders express: a DB00 window, a segmented
// pointer, a second read, a spliced name, or a search floor.
func mapScoped(row MapSig) bool {
	return row.Open != "" || row.Frame || row.Far || row.Deref > 0 || row.From != 0 || strings.Contains(row.Pattern, "{")
}

// scopedHit is the CPU address of the pattern and the window searched. An
// opener limits it to that DB00 window and keeps the first copy, unless
// single is set. With no opener the pattern has to occur once.
func scopedHit(img []byte, end uint32, row MapSig, have map[string]uint32, slot func(string) (string, bool)) (hit, lo, hi uint32) {
	floor := row.From
	if floor == 0 {
		floor = FlashBase
		if row.Open != "" {
			floor = 0x820000
		}
	}
	lo, hi = floor, end
	if row.Open != "" {
		open, ok := expandSig(row.Open, have, slot)
		if !ok {
			return 0, 0, 0
		}
		h := findPat(img, floor, end, open, !row.First)
		if h == 0 {
			return 0, 0, 0
		}
		db := findLast(img, floor, h, "DB00")
		if db == 0 {
			return 0, 0, 0
		}
		lo = db + 2
		hi = findPat(img, lo, end, "DB00", false)
		if hi == 0 {
			return 0, 0, 0
		}
	}
	pat, ok := expandSig(row.Pattern, have, slot)
	if !ok {
		return 0, 0, 0
	}
	n := patLen(pat)
	if n == 0 {
		return 0, 0, 0
	}
	// The word has to sit in the pattern. A segmented pointer may run past
	// the last matched byte, and a negative at sits in front of the hit.
	if !row.Frame && !row.Far && row.At >= 0 && row.At+2 > n {
		return 0, 0, 0
	}
	if (row.Frame || row.Far) && row.At >= 0 && row.At >= n {
		return 0, 0, 0
	}
	return findPat(img, lo, hi, pat, row.Single || row.Open == ""), lo, hi
}

// decodeMapPtr reads the map pointer at addr. frame is the 6-byte form and
// far is the 4-byte form. deref reads another word at that address.
func decodeMapPtr(img []byte, dpp [4]uint16, addr uint32, row MapSig) uint32 {
	var p uint32
	switch {
	case row.Frame:
		p = segPtr(img, addr, 4)
	case row.Far:
		p = segPtr(img, addr, 2)
	default:
		p = PtrAt(img, addr, dpp)
	}
	for i := 0; i < row.Deref && p != 0; i++ {
		at := p
		if i == 0 {
			at += uint32(row.DerefAt)
		}
		if i == 0 && row.DerefFar {
			p = segPtr(img, at, 2)
			continue
		}
		p = PtrAt(img, at, dpp)
	}
	return p
}

// segPtr is the segmented pointer at addr. The page word is pageAt bytes in.
// framePtr keeps that word at byte 4. readFar keeps it at byte 2.
func segPtr(img []byte, addr uint32, pageAt int) uint32 {
	off := int64(addr) - int64(FlashBase)
	if off < 0 || off+int64(pageAt)+2 > int64(len(img)) {
		return 0
	}
	b := img[off:]
	offset := uint32(b[0]) | uint32(b[1]&0x3f)<<8
	page := uint32(b[pageAt]) | uint32(b[pageAt+1])<<8
	return offset | page<<14
}

// TableAddrs is the CPU address of each breakpoint table whose pattern occurs
// once. The address is the hit. A table is not a map.
func TableAddrs(img []byte, rows []MapSig) map[string]uint32 {
	tabs := map[string]uint32{}
	if len(img) == 0 {
		return tabs
	}
	for _, row := range rows {
		if !row.Table || row.Pattern == "" {
			continue
		}
		pat, mask, ok := compilePat(row.Pattern)
		if !ok {
			continue
		}
		at, ok := findOne(img, pat, mask)
		if !ok {
			continue
		}
		sum := at + row.Add
		if sum < 0 || sum >= len(img) {
			continue
		}
		tabs[row.Name] = FlashBase + uint32(sum)
	}
	return tabs
}

// f2Word is the RAM operand of an F2 at hit+rel. rel nil, or a byte that is
// not that operand, is no word.
func f2Word(img []byte, hit int, rel *int) uint16 {
	if rel == nil {
		return 0
	}
	off := hit + *rel
	if off < 2 || off+2 > len(img) || img[off-2] != 0xF2 {
		return 0
	}
	return binary.LittleEndian.Uint16(img[off : off+2])
}

// shift adds a byte distance to a decoded pointer and keeps the result only
// when it is inside the image. A non-zero distance reports the pointer as
// the header.
func shift(img []byte, ptr uint32, add int) (addr, header uint32) {
	if ptr < FlashBase {
		return 0, 0
	}
	sum := int64(ptr) + int64(add)
	if sum < int64(FlashBase) || sum >= int64(FlashBase)+int64(len(img)) {
		return 0, 0
	}
	addr = uint32(sum)
	if add > 0 {
		header = ptr
	}
	return addr, header
}

func inFlash(img []byte, addr uint32) (uint32, bool) {
	if addr < FlashBase || int(addr-FlashBase) >= len(img) {
		return 0, false
	}
	return addr, true
}

// findOne returns the file offset of the only even-aligned match.
func findOne(img, pat, mask []byte) (int, bool) {
	hits := needle.Hits(img, pat, mask, 0, len(img), 2)
	if len(hits) != 1 {
		return 0, false
	}
	return hits[0], true
}
