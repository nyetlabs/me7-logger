// Package place copies a map catalog onto another image of the same layout.
// An object whose bytes match is kept. One whose bytes differ is moved only
// when the reference image has one code pointer to it and that pointer's
// frame occurs once on the destination. Anything else is left out.
package place

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/xdfkit/model"
)

// Frame context around a pointer word. The word itself, and the EXTP page
// when one sits six bytes before it, are wild.
const (
	frameBefore = 8
	frameAfter  = 8
)

// Report is the three outcomes of a placement.
type Report struct {
	Kept, Moved int
	Unresolved  []string
}

// Format writes the three counts and the unresolved ids.
func (r Report) Format(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "kept %d\nmoved %d\nunresolved %d\n", r.Kept, r.Moved, len(r.Unresolved)); err != nil {
		return err
	}
	for _, id := range r.Unresolved {
		if _, err := fmt.Fprintln(w, id); err != nil {
			return err
		}
	}
	return nil
}

// Place returns a model for dst. Names, shapes, and conversions come from
// refModel. Addresses that differ are not copied.
func Place(refModel *model.Model, ref, dst []byte) (*model.Model, Report, error) {
	return place(refModel, ref, dst, frameBefore, frameAfter)
}

func place(refModel *model.Model, ref, dst []byte, before, after int) (*model.Model, Report, error) {
	if refModel == nil {
		return nil, Report{}, fmt.Errorf("place: no model")
	}
	raw, err := json.Marshal(refModel)
	if err != nil {
		return nil, Report{}, err
	}
	var m model.Model
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, Report{}, err
	}
	spans := objectSpans(m.Objects)
	dppRef, _, _ := opcode.FindDPP(ref)
	dppDst, _, _ := opcode.FindDPP(dst)
	placed := map[uint32]uint32{}
	for _, o := range m.Objects {
		if identical(ref, dst, o) {
			noteKept(o, placed)
		}
	}
	var out []*model.Object
	var rep Report
	for _, o := range m.Objects {
		if identical(ref, dst, o) {
			out = append(out, o)
			rep.Kept++
			continue
		}
		neu, ok := movedBody(ref, dst, dppRef, dppDst, spans, o, before, after)
		if !ok || !placeAxes(ref, dst, o, neu, placed) {
			rep.Unresolved = append(rep.Unresolved, label(o))
			continue
		}
		o.Address = model.Addr(neu)
		out = append(out, o)
		rep.Moved++
	}
	m.Objects = out
	return &m, rep, nil
}

func label(o *model.Object) string {
	if o.Key != "" && o.Key != o.ID {
		return o.Key
	}
	if o.ID != "" {
		return o.ID
	}
	return o.Key
}

// ptrWord is the 16-bit immediate embedAddr emits for a CPU address.
// page is the EXTP page, or -1 when the address sits in a DPP page.
func ptrWord(addr uint32) (word uint16, page int) {
	off := uint16(addr & 0x3FFF)
	p := int(addr >> 14)
	switch p {
	case 3:
		return off | 0xC000, -1
	case 0xE0:
		return off | 0x8000, -1
	case 0x205:
		return off | 0x4000, -1
	case 0x204:
		return off, -1
	default:
		return off, p
	}
}

type span struct{ lo, hi int }

func objectSpans(objs []*model.Object) []span {
	var s []span
	for _, o := range objs {
		s = addSpan(s, int(o.Address), bodyLen(o))
		s = addAxis(s, o.X, axisPoints(o, "x"))
		s = addAxis(s, o.Y, axisPoints(o, "y"))
	}
	return s
}

func addAxis(s []span, a *model.Axis, n int) []span {
	if a == nil || a.Source != "image" || a.Address == nil || n <= 0 {
		return s
	}
	return addSpan(s, int(*a.Address), axisLen(a, n))
}

func addSpan(s []span, off, n int) []span {
	if n <= 0 {
		return s
	}
	return append(s, span{off, off + n})
}

func covered(s []span, off int) bool {
	for _, sp := range s {
		if off >= sp.lo && off < sp.hi {
			return true
		}
	}
	return false
}

func bodyLen(o *model.Object) int {
	n := o.Rows * o.Cols
	if o.Shape == "value" || n == 0 {
		n = 1
	}
	return nBytes(o.Data.Bits, n)
}

func axisPoints(o *model.Object, which string) int {
	switch which {
	case "x":
		if o.Shape == "value" {
			return 0
		}
		return o.Cols
	default:
		if o.Shape != "2d" {
			return 0
		}
		return o.Rows
	}
}

func axisLen(a *model.Axis, n int) int {
	bits := 8
	if a.Data != nil && a.Data.Bits != 0 {
		bits = a.Data.Bits
	}
	return nBytes(bits, n)
}

func nBytes(bits, n int) int {
	if bits <= 0 {
		bits = 8
	}
	return n * bits / 8
}

func identical(ref, dst []byte, o *model.Object) bool {
	if !same(ref, dst, int(o.Address), bodyLen(o)) {
		return false
	}
	return axisSame(ref, dst, o.X, axisPoints(o, "x")) && axisSame(ref, dst, o.Y, axisPoints(o, "y"))
}

func axisSame(a, b []byte, ax *model.Axis, n int) bool {
	if ax == nil || ax.Source != "image" || ax.Address == nil || n <= 0 {
		return true
	}
	return same(a, b, int(*ax.Address), axisLen(ax, n))
}

func same(a, b []byte, off, n int) bool {
	if n <= 0 || off < 0 || off+n > len(a) || off+n > len(b) {
		return false
	}
	return bytes.Equal(a[off:off+n], b[off:off+n])
}

func noteKept(o *model.Object, placed map[uint32]uint32) {
	placed[uint32(o.Address)] = uint32(o.Address)
	noteAxis(o.X, axisPoints(o, "x"), placed)
	noteAxis(o.Y, axisPoints(o, "y"), placed)
}

func noteAxis(a *model.Axis, n int, placed map[uint32]uint32) {
	if a == nil || a.Source != "image" || a.Address == nil || n <= 0 {
		return
	}
	placed[uint32(*a.Address)] = uint32(*a.Address)
}

// refs are word-aligned immediates outside the catalog whose page decode
// is cpu. More than one means the pointer is not a unique caller.
func refs(img []byte, dpp [4]uint16, cpu uint32, spans []span) []int {
	word, _ := ptrWord(cpu)
	lo, hi := byte(word), byte(word>>8)
	var hits []int
	for i := 0; i+2 <= len(img); i += 2 {
		if img[i] != lo || img[i+1] != hi || covered(spans, i) {
			continue
		}
		if opcode.PtrAt(img, opcode.FlashBase+uint32(i), dpp) == cpu {
			hits = append(hits, i)
		}
	}
	return hits
}

type pat struct {
	raw, mask []byte
}

func frameAt(img []byte, hit, before, after int) (pat, bool) {
	start, end := hit-before, hit+2+after
	if start < 0 || end > len(img) || before < 0 || after < 0 {
		return pat{}, false
	}
	raw := append([]byte(nil), img[start:end]...)
	mask := bytes.Repeat([]byte{0xff}, len(raw))
	mask[before], mask[before+1] = 0, 0
	if before >= 6 && img[hit-6] == 0xD7 && img[hit-5]&0xC0 == 0x40 {
		mask[before-4], mask[before-3] = 0, 0
	}
	return pat{raw, mask}, true
}

func findFrame(img []byte, f pat) []int {
	var hits []int
	last := len(img) - len(f.raw)
	for i := 0; i <= last; i += 2 {
		if matchAt(img[i:], f) {
			hits = append(hits, i)
		}
	}
	return hits
}

func matchAt(b []byte, f pat) bool {
	for i := range f.raw {
		if b[i]&f.mask[i] != f.raw[i]&f.mask[i] {
			return false
		}
	}
	return true
}

func movedBody(ref, dst []byte, dppRef, dppDst [4]uint16, spans []span, o *model.Object, before, after int) (uint32, bool) {
	cpu := opcode.FlashBase + uint32(o.Address)
	hits := refs(ref, dppRef, cpu, spans)
	if len(hits) != 1 {
		return 0, false
	}
	f, ok := frameAt(ref, hits[0], before, after)
	if !ok {
		return 0, false
	}
	got := findFrame(dst, f)
	if len(got) != 1 {
		return 0, false
	}
	at := got[0] + before
	if at < 0 || at+2 > len(dst) {
		return 0, false
	}
	phys := opcode.PtrAt(dst, opcode.FlashBase+uint32(at), dppDst)
	if phys < opcode.FlashBase {
		return 0, false
	}
	file := phys - opcode.FlashBase
	if int(file)+bodyLen(o) > len(dst) {
		return 0, false
	}
	return file, true
}

func placeAxes(ref, dst []byte, o *model.Object, newBody uint32, placed map[uint32]uint32) bool {
	return placeAxis(ref, dst, o.X, axisPoints(o, "x"), uint32(o.Address), newBody, placed) &&
		placeAxis(ref, dst, o.Y, axisPoints(o, "y"), uint32(o.Address), newBody, placed)
}

func placeAxis(ref, dst []byte, a *model.Axis, n int, oldBody, newBody uint32, placed map[uint32]uint32) bool {
	if a == nil || a.Source != "image" || a.Address == nil || n <= 0 {
		return true
	}
	old := uint32(*a.Address)
	neu, ok := placed[old]
	if !ok {
		if same(ref, dst, int(old), axisLen(a, n)) {
			neu = old
		} else {
			delta := int64(old) - int64(oldBody)
			next := int64(newBody) + delta
			if next < 0 || int(next)+axisLen(a, n) > len(dst) {
				return false
			}
			neu = uint32(next)
		}
		placed[old] = neu
	}
	*a.Address = model.Addr(neu)
	return true
}
