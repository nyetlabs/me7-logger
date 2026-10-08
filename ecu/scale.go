package ecu

import (
	"fmt"
	"strings"

	"go.nyet.org/me7-logger/record"
)

// ApplyClock replaces injection-time factors. mhz 0 leaves the catalog defaults.
// injection is the name set from config/names.yaml. krkte is the constant that
// uses factor/24. factor is the milliseconds-per-tick value for mhz.
func ApplyClock(items []record.Item, mhz int, injection map[string]bool, krkte string, factor float64) {
	if mhz == 0 || factor == 0 {
		return
	}
	for i := range items {
		name := items[i].Name
		switch {
		case injection[name]:
			items[i].A = factor
		case krkte != "" && name == krkte:
			items[i].A = factor / 24
		}
	}
}

// ScaleMode is the 5120 mbar option.
type ScaleMode int

const (
	// ScaleAuto doubles mbar factors only when ambient constants fit the 5.12 bar scaling.
	ScaleAuto ScaleMode = iota
	// ScaleOn doubles every mbar factor.
	ScaleOn
	// ScaleOff leaves mbar factors as they are.
	ScaleOff
)

// ParseScale reads auto, on, or off.
func ParseScale(s string) (ScaleMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return ScaleAuto, nil
	case "on", "true", "1":
		return ScaleOn, nil
	case "off", "false", "0":
		return ScaleOff, nil
	default:
		return ScaleAuto, fmt.Errorf("5120 scale %q wants auto, on, or off", s)
	}
}

// Const is a pressure constant read from the image.
type Const struct {
	Name string
	Raw  uint16
	A, B float64
}

// Decide5120 reports whether mbar factors should be doubled, and the evidence.
// ambient is the pressure-constant name set. low and high are the mbar window.
// Auto scales when the doubled factor lands in that window and the stock factor does not.
func Decide5120(mode ScaleMode, consts []Const, ambient map[string]bool, low, high float64) (bool, string) {
	switch mode {
	case ScaleOn:
		return true, "5120: override on, mbar factors doubled"
	case ScaleOff:
		return false, "5120: override off"
	}
	in, out := 0, 0
	var ev []string
	for _, c := range consts {
		if !ambient[c.Name] {
			continue
		}
		stock := c.A*float64(c.Raw) - c.B
		dbl := 2*c.A*float64(c.Raw) - c.B
		stockOK := stock >= low && stock <= high
		dblOK := dbl >= low && dbl <= high
		switch {
		case dblOK && !stockOK:
			out++
			ev = append(ev, fmt.Sprintf("%s raw %d stock %s mbar, doubled %s", c.Name, c.Raw, formatNum(stock), formatNum(dbl)))
		case stockOK:
			in++
			ev = append(ev, fmt.Sprintf("%s raw %d fits stock %s mbar", c.Name, c.Raw, formatNum(stock)))
		}
	}
	if in+out == 0 {
		return false, "5120: no pressure constants, factors unchanged"
	}
	if out > in {
		return true, "5120: doubled scaling fits (" + strings.Join(ev, "; ") + ")"
	}
	return false, "5120: stock scaling fits (" + strings.Join(ev, "; ") + ")"
}

// ScaleMbar doubles the A factor of every item whose unit is in units.
func ScaleMbar(items []record.Item, units []string) {
	for i := range items {
		for _, u := range units {
			if items[i].Unit == u {
				items[i].A *= 2
				break
			}
		}
	}
}
