package interp

import (
	"encoding/binary"
	"slices"

	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

// Record is an ME7 record read back from its values: one count per inline
// axis (a word for an axis-point table), the axes, then the values at an
// even address. Rows and Y are zero for a curve; a table has no axes.
type Record struct {
	Start  uint32
	Values uint32
	Rows   int
	Cols   int
	X, Y   record.Axis
}

// maxPoints bounds a count byte read as an axis size.
const maxPoints = 32

// Aligned is the one record whose 16-bit values start a byte either side of
// body, an odd address no ME7 record puts 16-bit values on. The counts must
// be in range and every axis must strictly increase. A record that starts
// inside another one's counts and axes is that one's axis read as a count.
// Two different records, or none, is not ok.
func Aligned(data []byte, body uint32) (Record, bool) {
	var hit []Record
	for _, v := range []uint32{body - 1, body + 1} {
		if v%2 == 1 {
			continue
		}
		hit = append(hit, tablesAt(data, v)...)
		hit = append(hit, curvesAt(data, v)...)
		hit = append(hit, mapsAt(data, v)...)
	}
	hit = slices.DeleteFunc(slices.Clone(hit), func(r Record) bool {
		return slices.ContainsFunc(hit, func(o Record) bool { return o.Start < r.Start && r.Start < o.Values })
	})
	if len(hit) != 1 {
		return Record{}, false
	}
	return hit[0], true
}

// tablesAt is an axis-point table: a count word, then that many increasing
// words at v.
func tablesAt(data []byte, v uint32) []Record {
	if v < opcode.FlashBase+2 {
		return nil
	}
	n := int(word(data, v-2))
	if n < 2 || n > maxPoints || !rising(data, v, n, 16) {
		return nil
	}
	return []Record{{Start: v - 2, Values: v, Rows: 1, Cols: n}}
}

// curvesAt is a count byte, its axis, and the values at v.
func curvesAt(data []byte, v uint32) []Record {
	var out []Record
	for _, xb := range []int{8, 16} {
		for c := 2; c <= maxPoints; c++ {
			for _, x := range axisBefore(v, c, xb, true) {
				for _, h := range countBefore(x, 1, xb) {
					if count(data, h) == c && rising(data, x, c, xb) {
						out = append(out, Record{Start: h, Values: v, Rows: 1, Cols: c, X: record.Axis{Addr: x, Count: c, Bits: xb}})
					}
				}
			}
		}
	}
	return out
}

// mapsAt is two count bytes, the row axis, the column axis, and the values
// at v.
func mapsAt(data []byte, v uint32) []Record {
	var out []Record
	for _, yb := range []int{8, 16} {
		for _, xb := range []int{8, 16} {
			for cols := 2; cols <= maxPoints; cols++ {
				for _, x := range axisBefore(v, cols, xb, true) {
					for rows := 2; rows <= maxPoints; rows++ {
						for _, y := range axisBefore(x, rows, yb, xb == 16) {
							for _, h := range countBefore(y, 2, yb) {
								if count(data, h) == rows && count(data, h+1) == cols && rising(data, y, rows, yb) && rising(data, x, cols, xb) {
									out = append(out, Record{Start: h, Values: v, Rows: rows, Cols: cols,
										X: record.Axis{Addr: x, Count: cols, Bits: xb}, Y: record.Axis{Addr: y, Count: rows, Bits: yb}})
								}
							}
						}
					}
				}
			}
		}
	}
	return out
}

// axisBefore is where an axis of n points of bits starts when it ends at
// next, or one byte earlier when the field at next is 16-bit (wide) and the
// axis ends on an odd address, which a pad fills. A 16-bit axis starts on an
// even address.
func axisBefore(next uint32, n, bits int, wide bool) []uint32 {
	var out []uint32
	for _, end := range []uint32{next, next - 1} {
		if end != next && (!wide || end%2 == 0) {
			continue
		}
		a := end - uint32(n*bits/8)
		if bits == 16 && a%2 == 1 || a < opcode.FlashBase {
			continue
		}
		out = append(out, a)
	}
	return out
}

// countBefore is where k count bytes sit in front of an axis of bits at a:
// right before it, or before a pad that puts a 16-bit axis on an even address.
func countBefore(a uint32, k, bits int) []uint32 {
	out := []uint32{a - uint32(k)}
	if bits == 16 && (a-uint32(k))%2 == 0 {
		out = append(out, a-uint32(k)-1)
	}
	return out
}

func count(data []byte, a uint32) int {
	if a < opcode.FlashBase || int(a-opcode.FlashBase) >= len(data) {
		return -1
	}
	return int(data[a-opcode.FlashBase])
}

func word(data []byte, a uint32) uint16 {
	off := int(a - opcode.FlashBase)
	if a < opcode.FlashBase || off+2 > len(data) {
		return 0
	}
	return binary.LittleEndian.Uint16(data[off:])
}

// rising is n points of bits at a that strictly increase, read unsigned or
// signed.
func rising(data []byte, a uint32, n, bits int) bool {
	off := int(a) - opcode.FlashBase
	w := bits / 8
	if a < opcode.FlashBase || off+n*w > len(data) {
		return false
	}
	up, sup := true, true
	for i := 1; i < n; i++ {
		p, q := point(data[off+(i-1)*w:], w), point(data[off+i*w:], w)
		up = up && q > p
		sup = sup && signed(q, w) > signed(p, w)
	}
	return up || sup
}

func point(b []byte, w int) int {
	if w == 2 {
		return int(binary.LittleEndian.Uint16(b))
	}
	return int(b[0])
}

func signed(v, w int) int {
	if v >= 1<<(8*w-1) {
		return v - 1<<(8*w)
	}
	return v
}
