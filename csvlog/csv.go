// Package csvlog writes an ME7Logger-compatible CSV. ECUxPlot detects the
// file from the text "ME7-Logger" and a TimeStamp column, then reads a name
// row, a unit row, and an alias row.
package csvlog

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"me7-logger/record"
)

// Col is one CSV column after TimeStamp.
type Col struct {
	Name  string
	Unit  string
	Alias string
}

// Header describes the comment block above the three header rows.
type Header struct {
	ECUFile   string
	SPS       int
	Baud      int
	Mode      string
	Started   time.Time
	IdentNote string
}

// WriteHeader writes the comment block and the three header rows.
func WriteHeader(w io.Writer, h Header, cols []Col) error {
	if h.Mode == "" {
		h.Mode = "HM0"
	}
	if h.Started.IsZero() {
		h.Started = time.Now()
	}
	var b []byte
	b = append(b, "###################################################################################\n"...)
	b = append(b, "Logfile created by me7-logger (ME7-Logger compatible)\n\n"...)
	if h.ECUFile != "" {
		b = append(b, "Used EcuDefinition file: "...)
		b = append(b, h.ECUFile...)
		b = append(b, '\n', '\n')
	}
	if h.IdentNote != "" {
		b = append(b, h.IdentNote...)
		if h.IdentNote[len(h.IdentNote)-1] != '\n' {
			b = append(b, '\n')
		}
		b = append(b, '\n')
	}
	b = fmtAppend(b, "Logging with:    %d samples/second\n", h.SPS)
	b = fmtAppend(b, "Used speed is:   %d baud\n", h.Baud)
	b = append(b, "Used mode is:    "...)
	b = append(b, h.Mode...)
	b = append(b, '\n')
	b = append(b, "Log started at:  "...)
	b = append(b, h.Started.Format("02.01.2006 15:04:05.000")...)
	b = append(b, '\n', '\n')
	b = append(b, "TimeStamp"...)
	for _, c := range cols {
		b = append(b, ',', ' ')
		b = append(b, c.Name...)
	}
	b = append(b, '\n')
	b = append(b, "sec.ms"...)
	for _, c := range cols {
		b = append(b, ',', ' ')
		b = append(b, c.Unit...)
	}
	b = append(b, '\n')
	b = append(b, `"TIME"`...)
	for _, c := range cols {
		b = append(b, ',')
		b = strconv.AppendQuote(b, c.Alias)
	}
	b = append(b, '\n')
	_, err := w.Write(b)
	return err
}

func fmtAppend(b []byte, format string, a int) []byte {
	return append(b, fmt.Sprintf(format, a)...)
}

// AppendRow appends one sample to buf. t is seconds since start.
// The buffer is reused by the caller so a row does not allocate when cap is enough.
func AppendRow(buf []byte, t float64, vals []float64) []byte {
	buf = strconv.AppendFloat(buf, t, 'f', 3, 64)
	for _, v := range vals {
		buf = append(buf, ',', ' ')
		buf = strconv.AppendFloat(buf, v, 'g', 8, 64)
	}
	return append(buf, '\n')
}

// Convert turns a raw little-endian cell into a physical value.
// A non-zero mask yields 0 or 1 before A and B are applied.
func Convert(raw uint16, size int, mask uint16, signed, inverse bool, a, b float64) float64 {
	var iv float64
	if mask != 0 {
		if raw&mask != 0 {
			iv = 1
		}
	} else if size <= 1 {
		if signed {
			iv = float64(int8(raw))
		} else {
			iv = float64(raw & 0xFF)
		}
	} else if signed {
		iv = float64(int16(raw))
	} else {
		iv = float64(raw)
	}
	if inverse {
		return a / (iv - b)
	}
	return a*iv - b
}

// Physical is Convert for a record.
func Physical(raw uint16, it record.Item) float64 {
	return Convert(raw, it.Size, it.Bitmask, it.Signed, it.Inverse, it.A, it.B)
}
