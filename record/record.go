// Package record is one found logging variable or one calibration map.
// An .ecu line needs a mask and a conversion. An XDF map needs a shape, axes, and an equation.
// The two are not the same record: a map is not written to the .ecu file.
package record

// Item is one found logging variable.
type Item struct {
	Name       string
	Alias      string
	Addr       uint32
	Size       int
	Bitmask    uint16
	Unit       string
	Signed     bool
	Inverse    bool
	A, B       float64
	Comment    string
	Guessed    bool
	ResultType int
}

// FormatEq renders A and B. Normal: A * X - B. Inverse: A / (X - B).
func FormatEq(a, b float64, inverse bool) string {
	as := formatFloat(a)
	bs := formatFloat(b)
	if inverse {
		if b == 0 {
			return as + " / X"
		}
		return as + " / (X - " + bs + ")"
	}
	if b == 0 {
		return as + " * X"
	}
	if b < 0 {
		return as + " * X + " + formatFloat(-b)
	}
	return as + " * X - " + bs
}

// Call names the map passed at one CALLS.
// Caller and Interp are function needle names.
// At is the byte distance from the caller label to the CALLS.
// The immediate at that call is the address, and it is not stored here.
// YTable is a breakpoint table. Rows is the count that table must have.
// YBits is its width. When the call leaves the row unset, that table fills it.
type Call struct {
	Name   string
	Caller string
	At     int
	Interp string
	Rows   int
	YBits  int
	YTable string
}

// Map is one calibration map, in raw values. Addr is a CPU address. The XDF
// model stores the file offset. Rows and Cols are the table body. Both zero
// is a constant.
type Map struct {
	Name    string
	Addr    uint32
	Bits    int
	Signed  bool
	Rows    int
	Cols    int
	Unit    string
	Comment string
	X, Y    *Axis
}

// Axis is one map axis. Addr 0 is an ordinal axis.
type Axis struct {
	Unit  string
	Addr  uint32
	Count int
	Bits  int
}
