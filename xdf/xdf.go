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
// A map with no name is not written.
func Write(w io.Writer, title string, maps []record.Map) error {
	maps = named(maps)
	if len(maps) == 0 {
		return nil
	}
	if title == "" {
		title = "me7-logger"
	}
	var b strings.Builder
	b.WriteString(xml.Header)
	fmt.Fprintf(&b, "<XDFFORMAT version=\"1.70\">\n<XDFHEADER>\n<deftitle>%s</deftitle>\n</XDFHEADER>\n", xmlEscape(title))
	for _, m := range maps {
		if tableShape(m) {
			writeTable(&b, m)
			continue
		}
		writeConst(&b, m)
	}
	b.WriteString("</XDFFORMAT>\n")
	_, err := io.WriteString(w, b.String())
	return err
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

func writeConst(b *strings.Builder, m record.Map) {
	bits := m.Bits
	if bits == 0 {
		bits = 8
	}
	fmt.Fprintf(b, "<XDFCONSTANT>\n<title>%s</title>\n<description>%s</description>\n", xmlEscape(m.Name), xmlEscape(m.Comment))
	fmt.Fprintf(b, "<EMBEDDEDDATA mmedaddress=\"0x%X\" mmedelementsizebits=\"%d\" mmedtypeflags=\"0x%02X\" />\n", opcode.FileOffset(m.Addr), bits, flags(bits, m.Signed))
	fmt.Fprintf(b, "<units>%s</units>\n<MATH equation=\"%s\"><VAR id=\"X\" /></MATH>\n</XDFCONSTANT>\n", xmlEscape(m.Unit), xmlEscape(equation(m.Equation)))
}

func writeTable(b *strings.Builder, m record.Map) {
	rows, cols := m.Rows, m.Cols
	if rows == 0 {
		rows = 1
	}
	if cols == 0 {
		cols = 1
	}
	bits := m.Bits
	if bits == 0 {
		bits = 8
	}
	fmt.Fprintf(b, "<XDFTABLE>\n<title>%s</title>\n<description>%s</description>\n", xmlEscape(m.Name), xmlEscape(m.Comment))
	writeAxis(b, "x", m.X, cols)
	writeAxis(b, "y", m.Y, rows)
	fmt.Fprintf(b, "<XDFAXIS id=\"z\">\n<EMBEDDEDDATA mmedaddress=\"0x%X\" mmedelementsizebits=\"%d\" mmedtypeflags=\"0x%02X\" mmedrowcount=\"%d\" mmedcolcount=\"%d\" />\n",
		opcode.FileOffset(m.Addr), bits, flags(bits, m.Signed), rows, cols)
	fmt.Fprintf(b, "<units>%s</units>\n<MATH equation=\"%s\"><VAR id=\"X\" /></MATH>\n</XDFAXIS>\n</XDFTABLE>\n",
		xmlEscape(m.Unit), xmlEscape(equation(m.Equation)))
}

func writeAxis(b *strings.Builder, id string, ax *record.Axis, n int) {
	fmt.Fprintf(b, "<XDFAXIS id=\"%s\">\n", id)
	if ax == nil {
		fmt.Fprintf(b, "<indexcount>%d</indexcount>\n<MATH equation=\"X\"><VAR id=\"X\" /></MATH>\n</XDFAXIS>\n", n)
		return
	}
	count := ax.Count
	if count == 0 {
		count = n
	}
	bits := ax.Bits
	if bits == 0 {
		bits = 8
	}
	eq := equation(ax.Equation)
	if ax.Addr != 0 {
		fmt.Fprintf(b, "<EMBEDDEDDATA mmedaddress=\"0x%X\" mmedelementsizebits=\"%d\" mmedtypeflags=\"0x%02X\" />\n", opcode.FileOffset(ax.Addr), bits, flags(bits, false))
	}
	fmt.Fprintf(b, "<units>%s</units>\n<indexcount>%d</indexcount>\n", xmlEscape(ax.Unit), count)
	if ax.Addr == 0 {
		for i, v := range ax.Labels {
			fmt.Fprintf(b, "<LABEL index=\"%d\" value=\"%s\" />\n", i, xmlEscape(trim(v)))
		}
	}
	fmt.Fprintf(b, "<MATH equation=\"%s\"><VAR id=\"X\" /></MATH>\n</XDFAXIS>\n", xmlEscape(eq))
}

func equation(s string) string {
	if s == "" {
		return "X"
	}
	return s
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

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
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
