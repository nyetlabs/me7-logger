// Package xdf converts located calibration maps to an xdfkit model, which
// xdfkit's xdf package writes as TunerPro XDF.
package xdf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/ident"
	"go.nyet.org/me7-logger/interp"
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
		Provenance: &model.Provenance{Format: "image", Origin: "auto", File: title, SHA256: hex.EncodeToString(sum[:])},
		Objects:    []*model.Object{},
	}
	part := strings.TrimSpace(ident.Find(image).PartNumber)
	for _, r := range maps {
		if r.Name != "" {
			m.Objects = append(m.Objects, object(r, part))
		}
	}
	m.AssignKeys()
	conflicts = breakpoints(m, image)
	return m, append(conflicts, aligned(m, image)...)
}

// aligned keeps every 16- or 32-bit integer field on an even address, where
// ME7 stores it. An object on an odd address moves onto the values of the
// one record a byte either side (interp.Aligned) and takes that record's
// shape and axes; with no such record it is left out, as is an object whose
// image axis is on an odd address. Each is a conflict.
func aligned(m *model.Model, image []byte) (conflicts []string) {
	m.Objects = slices.DeleteFunc(m.Objects, func(o *model.Object) bool {
		for _, a := range []*model.Axis{o.X, o.Y} {
			if a != nil && a.Address != nil && a.Data != nil && odd(*a.Address, *a.Data) {
				conflicts = append(conflicts, fmt.Sprintf("%s at 0x%X: %d-bit axis at an odd address 0x%X, left out", o.Key, uint32(o.Address), a.Data.Bits, uint32(*a.Address)))
				return true
			}
		}
		if !odd(o.Address, o.Data) {
			return false
		}
		r, ok := interp.Aligned(image, opcode.FlashBase+uint32(o.Address))
		if !ok {
			conflicts = append(conflicts, fmt.Sprintf("%s at 0x%X: %d-bit data at an odd address and no record there, left out", o.Key, uint32(o.Address), o.Data.Bits))
			return true
		}
		conflicts = append(conflicts, fmt.Sprintf("%s at 0x%X: %d-bit data at an odd address, moved to the record's values at 0x%X", o.Key, uint32(o.Address), o.Data.Bits, opcode.FileOffset(r.Values)))
		o.Address, o.Rows, o.Cols, o.Shape = model.Addr(opcode.FileOffset(r.Values)), r.Rows, r.Cols, "1d"
		if r.Rows > 1 {
			o.Shape = "2d"
		}
		if r.X.Addr != 0 {
			o.X = moved(o.X, r.X)
		}
		if r.Y.Addr != 0 {
			o.Y = moved(o.Y, r.Y)
		} else {
			o.Y = nil
		}
		return false
	})
	return conflicts
}

func odd(a model.Addr, d model.Data) bool { return a%2 == 1 && d.Bits >= 16 && !d.Float }

// moved is the record's axis r, keeping what old axis says it means.
func moved(old *model.Axis, r record.Axis) *model.Axis {
	a := axis(&r, false)
	if old != nil {
		a.Value = old.Value
	}
	return a
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
			if a.x != nil && a.x.Address != nil {
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
			conflicts = append(conflicts, fmt.Sprintf("%s at 0x%X: not a breakpoint curve: %s", o.Key, uint32(o.Address), why))
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

// object takes a width the locator did not read from config.Storage for
// part, the image's part number.
func object(r record.Map, part string) *model.Object {
	cell, known := config.Storage(r.Name, part)
	if r.Bits == 0 {
		r.Bits, r.Signed = cell.Bits, cell.Signed
	}
	o := &model.Object{
		ID:          r.Name,
		Description: config.Description(r.Name),
		Comment:     r.Comment,
		Shape:       "value",
		Address:     model.Addr(opcode.FileOffset(r.Addr)),
		Rows:        1,
		Cols:        1,
		Data:        data(r.Bits, r.Signed),
		Value:       value(r.Unit),
	}
	rows, cols, ok := dims(r, cell, known)
	if !ok {
		return o
	}
	o.Rows, o.Cols, o.Shape = rows, cols, "2d"
	if rows == 1 {
		o.Shape = "1d"
	}
	o.X = axis(r.X, cell.Ordinal)
	if rows > 1 {
		o.Y = axis(r.Y, cell.Ordinal)
	}
	return o
}

// dims is the shape the locator read when it read a row count, with the
// config.Storage column count when it read only that row count. Otherwise
// it is the config.Storage shape, with the column count the locator read
// for a curve. Which axis is x comes from the locator, which reads it from
// the lookup call. A map whose column or axis count disagrees with
// config.Storage, or a table with neither, is written as a constant.
func dims(m record.Map, c config.Cell, known bool) (int, int, bool) {
	sized := known && c.Rows*c.Cols > 1
	if m.Rows > 0 {
		if m.Cols == 0 && sized && m.Rows == c.Rows && fits(m.X, c.Cols) {
			return c.Rows, c.Cols, true
		}
		return m.Rows, max(m.Cols, 1), m.Rows > 1 || m.Cols > 1 || m.X != nil || m.Y != nil
	}
	if !sized {
		return 0, 0, false
	}
	if c.Rows == 1 && m.Cols > 1 && fits(m.X, m.Cols) {
		return 1, m.Cols, true
	}
	if m.Cols > 0 && m.Cols != c.Cols || !fits(m.X, c.Cols) || !fits(m.Y, c.Rows) {
		return 0, 0, false
	}
	return c.Rows, c.Cols, true
}

// fits is an axis with no count read, or n points.
func fits(a *record.Axis, n int) bool {
	return a == nil || a.Count == 0 || a.Count == n
}

// axis is an image axis. A located count with no address is ordinal. An
// axis not located is ordinal when the DAMOS and A2L defs index it, and
// unknown otherwise.
func axis(a *record.Axis, ordinal bool) *model.Axis {
	if a != nil && a.Addr == 0 && a.Count > 0 {
		ordinal = true
	}
	if a == nil || a.Addr == 0 {
		return &model.Axis{Source: map[bool]string{true: "ordinal", false: "unknown"}[ordinal], Value: value("")}
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
