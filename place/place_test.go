package place

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
)

func obj(id string, addr, cols int, shape string) *model.Object {
	o := &model.Object{
		Key: id, ID: id, Shape: shape, Address: model.Addr(addr),
		Rows: 1, Cols: cols, Data: model.Data{Bits: 8},
	}
	if shape == "value" {
		o.Rows, o.Cols = 1, 1
	}
	return o
}

func TestPlaceKeepsMovesOmits(t *testing.T) {
	// File 0x40 is the body. CPU 0x800040 is EXTP page 0x200, word 0x0040.
	// The caller is D7 40 | page | pad | E6 FC | word, with a fixed tail.
	ref := make([]byte, 0x120)
	dst := make([]byte, 0x120)
	copy(ref[0x20:0x24], []byte{1, 2, 3, 4})
	copy(dst[0x20:0x24], []byte{1, 2, 3, 4})
	copy(ref[0x30:0x34], []byte{9, 9, 9, 9})
	copy(dst[0x30:0x34], []byte{8, 8, 8, 8})
	copy(ref[0x40:0x44], []byte{5, 5, 5, 5})
	copy(dst[0x60:0x64], []byte{6, 6, 6, 6})
	// EXTP is six bytes before the pointer word: D7 40 pp pp E6 FC lo hi.
	putPtr := func(img []byte, at int, word uint16) {
		img[at] = 0xD7
		img[at+1] = 0x40
		img[at+2] = 0x00
		img[at+3] = 0x02
		img[at+4] = 0xE6
		img[at+5] = 0xFC
		img[at+6] = byte(word)
		img[at+7] = byte(word >> 8)
		img[at+8] = 0x33
		img[at+9] = 0x44
	}
	putPtr(ref, 0x80, 0x0040)
	putPtr(dst, 0xA0, 0x0060)
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{
		obj("SAME", 0x20, 4, "value"),
		obj("GONE", 0x30, 4, "value"),
		obj("MOVE", 0x40, 4, "value"),
	}}
	got, rep, err := Place(m, ref, dst)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kept != 1 || rep.Moved != 1 || len(rep.Unresolved) != 1 || rep.Unresolved[0] != "GONE" {
		t.Fatalf("kept %d moved %d unresolved %v", rep.Kept, rep.Moved, rep.Unresolved)
	}
	if got.Objects[0].ID != "SAME" || got.Objects[0].Address != 0x20 {
		t.Fatalf("kept %+v", got.Objects[0])
	}
	if got.Objects[1].ID != "MOVE" || got.Objects[1].Address != 0x60 {
		t.Fatalf("moved %+v", got.Objects[1])
	}
}

func TestMeasure4D(t *testing.T) {
	def := filepath.Join("..", "corpus", "defs", "4D1907558-0002.json")
	refPath := filepath.Join("..", "corpus", "images", "4D1907558-0002.bin")
	dstPath := filepath.Join("..", "corpus", "images", "4D1907558-0004.bin")
	b, err := os.ReadFile(def)
	if err != nil {
		t.Skip(err)
	}
	ref, err := os.ReadFile(refPath)
	if err != nil {
		t.Skip(err)
	}
	dst, err := os.ReadFile(dstPath)
	if err != nil {
		t.Skip(err)
	}
	var m model.Model
	if err := canon.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	spans := objectSpans(m.Objects)
	dppRef, _, _ := opcode.FindDPP(ref)
	dppDst, _, _ := opcode.FindDPP(dst)
	want := map[string][4]int{"1d": {62, 7, 4, 0}, "2d": {70, 17, 6, 1}}
	for _, shape := range []string{"1d", "2d"} {
		one, framed, held, drift := 0, 0, 0, 0
		n := 0
		for _, o := range m.Objects {
			if o.Shape != shape {
				continue
			}
			n++
			hits := refs(ref, dppRef, opcode.FlashBase+uint32(o.Address), spans)
			if len(hits) != 1 {
				continue
			}
			one++
			f, ok := frameAt(ref, hits[0], frameBefore, frameAfter)
			if !ok {
				continue
			}
			at := findFrame(dst, f)
			if len(at) != 1 {
				continue
			}
			framed++
			if !identical(ref, dst, o) {
				continue
			}
			phys := opcode.PtrAt(dst, opcode.FlashBase+uint32(at[0]+frameBefore), dppDst)
			if phys-opcode.FlashBase == uint32(o.Address) {
				held++
			} else {
				drift++
			}
		}
		w := want[shape]
		if drift != 0 || n != w[0] || one != w[1] || framed != w[2] || held != w[3] {
			t.Fatalf("%s n %d unique %d frame %d held %d drift %d", shape, n, one, framed, held, drift)
		}
	}
	orig := map[string]*model.Object{}
	for _, o := range m.Objects {
		orig[o.Key] = o
	}
	got, rep, err := Place(&m, ref, dst)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kept != 182 || rep.Moved != 16 || len(rep.Unresolved) != 339 {
		t.Fatalf("kept %d moved %d unresolved %d", rep.Kept, rep.Moved, len(rep.Unresolved))
	}
	for _, o := range got.Objects {
		was := orig[o.Key]
		if o.Address != was.Address && identical(ref, dst, was) {
			t.Fatalf("%s identical at 0x%X moved to 0x%X", o.ID, was.Address, o.Address)
		}
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := canon.Unmarshal(raw, &model.Model{}); err != nil {
		t.Fatal(err)
	}
}
