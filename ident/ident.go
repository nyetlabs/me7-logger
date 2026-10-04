// Package ident reads the ASCII identification record ME7 images carry:
// a 12-byte part number, a 20-byte engine id (the .ecu keeps 16), and a
// 4-digit software version, plus the 10-digit hardware and software numbers
// stored near file offset 0x18000.
package ident

import (
	"regexp"
	"strings"
	"unicode"
)

// ID is the [Identification] block of an .ecu file.
type ID struct {
	HWNumber   string
	SWNumber   string
	PartNumber string
	SWVersion  string
	EngineID   string
}

var (
	partRE = regexp.MustCompile(`^[0-9A-Z]{3}[0-9]{6}[0-9A-Z]{0,3} *$`)
	pairRE = regexp.MustCompile(`(0\d{9}).{0,4}(1\d{9})`)
)

// Find scans a flash image. Missing fields stay empty.
func Find(data []byte) ID {
	var id ID
	for i := 0; i+36 <= len(data); i++ {
		chunk := data[i : i+12]
		if chunk[0] == ' ' || !asciiPart(chunk) {
			continue
		}
		s := string(chunk)
		if !partRE.MatchString(s) {
			continue
		}
		eng := data[i+12 : i+32]
		if !printable(eng) {
			continue
		}
		e := string(eng)
		if !strings.Contains(e, "/") && !strings.Contains(e, "VT") {
			continue
		}
		ver := data[i+32 : i+36]
		if !digits(ver) {
			continue
		}
		id.PartNumber = s
		id.EngineID = string(eng[:16])
		id.SWVersion = string(ver)
		break
	}
	lo, hi := 0x17000, 0x1A000
	if hi > len(data) {
		lo, hi = 0, len(data)
	}
	if lo < len(data) {
		if m := pairRE.FindSubmatch(data[lo:hi]); m != nil {
			id.HWNumber = string(m[1])
			id.SWNumber = string(m[2])
		}
	}
	return id
}

func asciiPart(b []byte) bool {
	for _, c := range b {
		if c != ' ' && !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

func digits(b []byte) bool {
	for _, c := range b {
		if !unicode.IsDigit(rune(c)) {
			return false
		}
	}
	return true
}
