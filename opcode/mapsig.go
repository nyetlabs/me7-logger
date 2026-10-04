package opcode

import (
	"bytes"
	"encoding/binary"
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
}

// MapAddrs runs the mapsig rows. A pattern is kept only when it occurs once
// and the address word decodes inside this image. An anchor is a byte
// distance from a map this pass already located. A later row with the same
// name fills it only when an earlier window missed. These addresses are
// flash maps, so the RAM filter used by the logging signatures does not apply.
func MapAddrs(img []byte, dpp [4]uint16, rows []MapSig) []MapHit {
	if len(img) == 0 || len(rows) == 0 {
		return nil
	}
	have := map[string]int{}
	var out []MapHit
	add := func(name string, addr, header uint32, row MapSig, xram, yram uint16) {
		if _, ok := have[name]; ok {
			return
		}
		if _, ok := inFlash(img, addr); !ok {
			return
		}
		have[name] = len(out)
		out = append(out, MapHit{
			Name: name, Addr: addr, Header: header,
			Rows: row.Rows, Cols: row.Cols, XBits: row.XBits, YBits: row.YBits,
			XRam: xram, YRam: yram,
		})
	}
	for _, row := range rows {
		if row.Pattern == "" {
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
		ptr := PtrAt(img, FlashBase+uint32(at+row.At), dpp)
		addr, header := shift(img, ptr, row.Add)
		if addr == 0 {
			continue
		}
		add(row.Name, addr, header, row, f2Word(img, at, row.XAt), f2Word(img, at, row.YAt))
	}
	for again := true; again; {
		again = false
		for _, row := range rows {
			if row.Anchor == "" {
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
	return out
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
	off, needle := solidNeedle(pat, mask)
	if needle == nil {
		return scanOne(img, pat, mask)
	}
	hit := -1
	from := 0
	for from < len(img) {
		j := bytes.Index(img[from:], needle)
		if j < 0 {
			break
		}
		j += from
		start := j - off
		from = j + 1
		if start < 0 || start+len(pat) > len(img) || start%2 != 0 {
			continue
		}
		if !matchBytes(img[start:start+len(pat)], pat, mask) {
			continue
		}
		if hit >= 0 {
			return 0, false
		}
		hit = start
	}
	if hit < 0 {
		return 0, false
	}
	return hit, true
}

func scanOne(img, pat, mask []byte) (int, bool) {
	hit := -1
	lim := len(img) - len(pat)
	for i := 0; i <= lim; i += 2 {
		if !matchBytes(img[i:i+len(pat)], pat, mask) {
			continue
		}
		if hit >= 0 {
			return 0, false
		}
		hit = i
	}
	if hit < 0 {
		return 0, false
	}
	return hit, true
}

// solidNeedle is the longest run of fully specified bytes, used as an index.
func solidNeedle(pat, mask []byte) (start int, needle []byte) {
	best, bestN := 0, 0
	for i := 0; i < len(pat); {
		if mask[i] != 0xFF {
			i++
			continue
		}
		j := i
		for j < len(pat) && mask[j] == 0xFF {
			j++
		}
		if j-i > bestN {
			best, bestN = i, j-i
		}
		i = j
	}
	if bestN == 0 {
		return 0, nil
	}
	return best, pat[best : best+bestN]
}
