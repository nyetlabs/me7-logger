package record

import (
	"strconv"
	"strings"
)

func formatFloat(v float64) string {
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
