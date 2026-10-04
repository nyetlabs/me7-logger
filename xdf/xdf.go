// Package xdf writes TunerPro XDF for calibration maps.
// It does not write logging variables, .kp, OLS, DAMOS, or WinOLS scripts.
package xdf

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"me7-logger/opcode"
	"me7-logger/record"
)

// Write emits one XDFFORMAT document. An empty map list writes nothing.
// A map with no name is not written. size is the image length and becomes
// the header region; 0 omits the region. Each map's unique id is its file
// offset. An axis whose address is another map in this file is a link to
// that id. The equation is the one stored on the map.
func Write(w io.Writer, title string, size int, maps []record.Map) error {
	maps = named(maps)
	if len(maps) == 0 {
		return nil
	}
	if title == "" {
		title = "me7info"
	}
	ids := map[uint32]struct{}{}
	for _, m := range maps {
		ids[opcode.FileOffset(m.Addr)] = struct{}{}
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(xdfDoc{header: newHeader(title, size), items: newItems(maps, ids)})
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

func named(maps []record.Map) []record.Map {
	out := make([]record.Map, 0, len(maps))
	for _, m := range maps {
		if m.Name != "" {
			out = append(out, m)
		}
	}
	return out
}

// xdfDoc is the document. MarshalXML writes each map in order. A struct
// field per element would emit every constant before every table.
type xdfDoc struct {
	header xdfHeader
	items  []any
}

func (d xdfDoc) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "XDFFORMAT"}
	start.Attr = []xml.Attr{{Name: xml.Name{Local: "version"}, Value: "1.70"}}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if err := e.Encode(d.header); err != nil {
		return err
	}
	for _, it := range d.items {
		if err := e.Encode(it); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

type xdfHeader struct {
	XMLName  xml.Name    `xml:"XDFHEADER"`
	Title    string      `xml:"deftitle"`
	Region   *xdfRegion  `xml:"REGION,omitempty"`
	Category xdfCategory `xml:"CATEGORY"`
}

type xdfRegion struct {
	Size string `xml:"size,attr"`
}

type xdfCategory struct {
	Index string `xml:"index,attr"`
	Name  string `xml:"name,attr"`
}

type xdfConstant struct {
	XMLName     xml.Name `xml:"XDFCONSTANT"`
	ID          string   `xml:"uniqueid,attr"`
	Title       string   `xml:"title"`
	Description string   `xml:"description"`
	Data        xdfData  `xml:"EMBEDDEDDATA"`
	Units       string   `xml:"units"`
	Math        xdfMath  `xml:"MATH"`
}

type xdfTable struct {
	XMLName     xml.Name  `xml:"XDFTABLE"`
	ID          string    `xml:"uniqueid,attr"`
	Title       string    `xml:"title"`
	Description string    `xml:"description"`
	Axes        []xdfAxis `xml:"XDFAXIS"`
}

type xdfAxis struct {
	XMLName xml.Name   `xml:"XDFAXIS"`
	ID      string     `xml:"id,attr"`
	Data    *xdfData   `xml:"EMBEDDEDDATA,omitempty"`
	Link    *xdfLink   `xml:"embedinfo,omitempty"`
	Units   *string    `xml:"units,omitempty"`
	Count   int        `xml:"indexcount,omitempty"`
	Labels  []xdfLabel `xml:"LABEL,omitempty"`
	Math    xdfMath    `xml:"MATH"`
}

type xdfData struct {
	Addr  string `xml:"mmedaddress,attr"`
	Bits  int    `xml:"mmedelementsizebits,attr"`
	Flags string `xml:"mmedtypeflags,attr"`
	Rows  int    `xml:"mmedrowcount,attr,omitempty"`
	Cols  int    `xml:"mmedcolcount,attr,omitempty"`
}

type xdfLink struct {
	Type int    `xml:"type,attr"`
	Link string `xml:"linkobjid,attr"`
}

type xdfLabel struct {
	Index int    `xml:"index,attr"`
	Value string `xml:"value,attr"`
}

type xdfMath struct {
	Equation string `xml:"equation,attr"`
	Var      xdfVar `xml:"VAR"`
}

type xdfVar struct {
	ID string `xml:"id,attr"`
}

func newHeader(title string, size int) xdfHeader {
	h := xdfHeader{
		Title:    title,
		Category: xdfCategory{Index: "0xFF", Name: "Axes"},
	}
	if size > 0 {
		h.Region = &xdfRegion{Size: hex(uint32(size))}
	}
	return h
}

func newItems(maps []record.Map, ids map[uint32]struct{}) []any {
	items := make([]any, 0, len(maps))
	for _, m := range maps {
		if tableShape(m) {
			items = append(items, newTable(m, ids))
			continue
		}
		items = append(items, newConst(m))
	}
	return items
}

func newConst(m record.Map) xdfConstant {
	bits := width(m.Bits)
	return xdfConstant{
		ID:          hex(opcode.FileOffset(m.Addr)),
		Title:       m.Name,
		Description: m.Comment,
		Data: xdfData{
			Addr:  hex(opcode.FileOffset(m.Addr)),
			Bits:  bits,
			Flags: hexByte(flags(bits, m.Signed)),
		},
		Units: m.Unit,
		Math:  newMath(m.Equation),
	}
}

func newTable(m record.Map, ids map[uint32]struct{}) xdfTable {
	rows, cols := m.Rows, m.Cols
	if rows == 0 {
		rows = 1
	}
	if cols == 0 {
		cols = 1
	}
	bits := width(m.Bits)
	return xdfTable{
		ID:          hex(opcode.FileOffset(m.Addr)),
		Title:       m.Name,
		Description: m.Comment,
		Axes: []xdfAxis{
			newAxis("x", m.X, cols, ids),
			newAxis("y", m.Y, rows, ids),
			zAxis(m, rows, cols, bits),
		},
	}
}

func newAxis(id string, ax *record.Axis, n int, ids map[uint32]struct{}) xdfAxis {
	out := xdfAxis{ID: id, Count: n, Math: newMath("X")}
	if ax == nil {
		return out
	}
	if ax.Count != 0 {
		out.Count = ax.Count
	}
	out.Math = newMath(ax.Equation)
	unit := ax.Unit
	out.Units = &unit
	if ax.Addr == 0 {
		for i, v := range ax.Labels {
			out.Labels = append(out.Labels, xdfLabel{Index: i, Value: trim(v)})
		}
		return out
	}
	off := opcode.FileOffset(ax.Addr)
	if _, ok := ids[off]; ok {
		out.Link = &xdfLink{Type: 3, Link: hex(off)}
		return out
	}
	bits := width(ax.Bits)
	out.Data = &xdfData{Addr: hex(off), Bits: bits, Flags: hexByte(flags(bits, false))}
	return out
}

func zAxis(m record.Map, rows, cols, bits int) xdfAxis {
	unit := m.Unit
	return xdfAxis{
		ID:    "z",
		Units: &unit,
		Data: &xdfData{
			Addr:  hex(opcode.FileOffset(m.Addr)),
			Bits:  bits,
			Flags: hexByte(flags(bits, m.Signed)),
			Rows:  rows,
			Cols:  cols,
		},
		Math: newMath(m.Equation),
	}
}

func newMath(eq string) xdfMath {
	return xdfMath{Equation: equation(eq), Var: xdfVar{ID: "X"}}
}

func equation(s string) string {
	if s == "" {
		return "X"
	}
	return s
}

func width(bits int) int {
	if bits == 0 {
		return 8
	}
	return bits
}

func flags(bits int, signed bool) int {
	f := 0
	if bits >= 16 {
		f |= 0x02
	}
	if signed {
		f |= 0x01
	}
	return f
}

func hex(v uint32) string {
	return fmt.Sprintf("0x%X", v)
}

func hexByte(v int) string {
	return fmt.Sprintf("0x%02X", v)
}

func trim(v float64) string {
	s := strconv.FormatFloat(v, 'g', 8, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
