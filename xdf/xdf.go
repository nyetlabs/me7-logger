// Package xdf converts located calibration maps to an xdfkit model, which
// xdfkit's xdf package writes as TunerPro XDF.
package xdf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

// Other is the category of a map that is not in the category table.
const Other = "Other"

// Model returns the named maps of image as a model, with no categories.
// Addresses are file offsets. Values are raw: factor 1, no decimals. title
// is the image file name. A constant at a table's axis address is that
// axis's breakpoints and becomes a curve of its points, so the writer links
// the axis to it. conflicts lists the constants left alone: the axes at
// their address disagree on count or storage, the points read from image
// don't strictly increase, or the curve would run into the next object.
func Model(maps []record.Map, image []byte, title string) (m *model.Model, conflicts []string) {
	sum := sha256.Sum256(image)
	m = &model.Model{
		Schema:     model.SchemaID,
		Provenance: &model.Provenance{Format: "image", Origin: "located", File: title, SHA256: hex.EncodeToString(sum[:])},
		Objects:    []*model.Object{},
	}
	for _, r := range maps {
		if r.Name != "" {
			m.Objects = append(m.Objects, object(r))
		}
	}
	m.AssignKeys()
	return m, breakpoints(m, image)
}

// breakpoints turns each constant at a table's axis address into a curve
// holding that axis's points, unless the axes there disagree, the points
// don't strictly increase, or another object starts inside the curve.
func breakpoints(m *model.Model, image []byte) (conflicts []string) {
	type points struct {
		n int
		d model.Data
	}
	at := map[model.Addr][]points{}
	for _, o := range m.Objects {
		for _, a := range []struct {
			x *model.Axis
			n int
		}{{o.X, o.Cols}, {o.Y, o.Rows}} {
			if a.x != nil {
				p := points{a.n, *a.x.Data}
				if !slices.Contains(at[*a.x.Address], p) {
					at[*a.x.Address] = append(at[*a.x.Address], p)
				}
			}
		}
	}
	for _, o := range m.Objects {
		ps := at[o.Address]
		if o.Shape != "value" || len(ps) == 0 {
			continue
		}
		why := ""
		switch {
		case len(ps) > 1:
			why = fmt.Sprintf("axes of %d different shapes", len(ps))
		case !increasing(image, o.Address, ps[0].n, ps[0].d.Bits/8):
			why = "breakpoints not increasing"
		default:
			end := o.Address + model.Addr(ps[0].n*ps[0].d.Bits/8)
			for _, t := range m.Objects {
				if t != o && t.Address > o.Address && t.Address < end {
					why = "overlaps " + t.Key
					break
				}
			}
		}
		if why != "" {
			conflicts = append(conflicts, fmt.Sprintf("%s at 0x%X: %s", o.Key, uint32(o.Address), why))
			continue
		}
		o.Shape, o.Cols, o.Data = "1d", ps[0].n, ps[0].d
	}
	return conflicts
}

// increasing reports whether the n unsigned little-endian points of w bytes
// at addr lie in image and strictly increase.
func increasing(image []byte, addr model.Addr, n, w int) bool {
	start := int(addr)
	if w < 1 || n < 1 || start+n*w > len(image) {
		return false
	}
	prev := -1
	for i := range n {
		v := 0
		for j, b := range image[start+i*w : start+(i+1)*w] {
			v |= int(b) << (8 * j)
		}
		if v <= prev {
			return false
		}
		prev = v
	}
	return true
}

func object(r record.Map) *model.Object {
	o := &model.Object{
		ID:          r.Name,
		Description: r.Comment,
		Shape:       "value",
		Address:     model.Addr(opcode.FileOffset(r.Addr)),
		Rows:        1,
		Cols:        1,
		Data:        data(r.Bits, r.Signed),
		Value:       value(r.Unit),
	}
	if !tableShape(r) {
		return o
	}
	o.Rows, o.Cols = max(r.Rows, 1), max(r.Cols, 1)
	o.Shape = "2d"
	if o.Rows == 1 {
		o.Shape = "1d"
	}
	o.X, o.Y = axis(r.X), axis(r.Y)
	return o
}

// tableShape is a map whose row count and column count were both read.
// A column count with no row count is written as a constant at the body
// address. The missing row count is not filled in as 1.
func tableShape(m record.Map) bool {
	if m.Rows == 0 {
		return false
	}
	return m.Rows > 1 || m.Cols > 1 || m.X != nil || m.Y != nil
}

// axis is an image axis, or nil (ordinal) when it has no address.
func axis(a *record.Axis) *model.Axis {
	if a == nil || a.Addr == 0 {
		return nil
	}
	addr := model.Addr(opcode.FileOffset(a.Addr))
	d := data(a.Bits, false)
	return &model.Axis{Source: "image", Stored: "absolute", Address: &addr, Data: &d, Value: value(a.Unit)}
}

func data(bits int, signed bool) model.Data {
	if bits == 0 {
		bits = 8
	}
	d := model.Data{Bits: bits, Signed: signed}
	if bits >= 16 {
		d.Endian = "little"
	}
	return d
}

func value(units string) model.Value {
	return model.Value{Units: units, Conversion: model.Conversion{Factor: 1}}
}
