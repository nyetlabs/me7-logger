// Package parity scores generated .ecu and .xdf rows against testdata/parity.
// The directory is 8D0907551M-0002: an ME7Info list plus torque and extras rows,
// and the S4 wiki maps that the M-box XDF locates.
package parity

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"me7-logger/ecu"
	"me7-logger/generate"
	"me7-logger/opcode"
	"me7-logger/record"
)

// Report is the fraction of oracle rows the image emission matches.
type Report struct {
	ECU, XDF Fraction
}

// Fraction is hits over the oracle row count.
type Fraction struct {
	Hit, Total int
}

// String renders one decimal percent. An empty oracle is 0.0% (0/0).
func (f Fraction) String() string {
	if f.Total == 0 {
		return "0.0% (0/0)"
	}
	return fmt.Sprintf("%.1f%% (%d/%d)", 100*float64(f.Hit)/float64(f.Total), f.Hit, f.Total)
}

type ecuKey struct {
	name string
	addr uint32
	size int
	mask uint16
}

type mapKey struct {
	name string
	addr uint32
}

// Run generates each image in dir and scores it against the .ecu and .xdf
// of the same name.
func Run(dir string) (Report, error) {
	bins, err := filepath.Glob(filepath.Join(dir, "*.bin"))
	if err != nil {
		return Report{}, err
	}
	sort.Strings(bins)
	if len(bins) == 0 {
		return Report{}, fmt.Errorf("%s: no image", dir)
	}
	var rep Report
	for _, bin := range bins {
		img, err := os.ReadFile(bin)
		if err != nil {
			return Report{}, err
		}
		stem := strings.TrimSuffix(bin, ".bin")
		res, err := generate.Generate(generate.Options{
			Image: img, ImageName: filepath.Base(bin), Scale: "off",
		})
		if err != nil {
			return Report{}, err
		}
		wantECU, err := ecu.Parse(stem + ".ecu")
		if err != nil {
			return Report{}, err
		}
		h, n := matchECU(res.File.Items, wantECU.Items)
		rep.ECU.Hit += h
		rep.ECU.Total += n
		xdf, err := os.ReadFile(stem + ".xdf")
		if err != nil {
			return Report{}, err
		}
		wantMaps, err := ParseXDF(xdf)
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", stem+".xdf", err)
		}
		h, n = matchMaps(located(res.Maps), wantMaps)
		rep.XDF.Hit += h
		rep.XDF.Total += n
	}
	return rep, nil
}

func matchECU(got []record.Item, want []record.Item) (int, int) {
	have := map[ecuKey]int{}
	for _, it := range got {
		if it.Name == "" {
			continue
		}
		have[ecuKey{it.Name, it.Addr, it.Size, it.Bitmask}]++
	}
	hit := 0
	for _, it := range want {
		if it.Name == "" {
			continue
		}
		k := ecuKey{it.Name, it.Addr, it.Size, it.Bitmask}
		if have[k] == 0 {
			continue
		}
		have[k]--
		hit++
	}
	return hit, len(want)
}

func located(maps []record.Map) []Map {
	out := make([]Map, 0, len(maps))
	for _, m := range maps {
		if m.Name == "" {
			continue
		}
		out = append(out, Map{Name: m.Name, Addr: m.Addr})
	}
	return out
}

func matchMaps(got, want []Map) (int, int) {
	have := map[mapKey]int{}
	for _, m := range got {
		have[mapKey{m.Name, opcode.FileOffset(m.Addr)}]++
	}
	hit := 0
	for _, m := range want {
		k := mapKey{m.Name, opcode.FileOffset(m.Addr)}
		if have[k] == 0 {
			continue
		}
		have[k]--
		hit++
	}
	return hit, len(want)
}

// Map is one located calibration map. Addr may be a file offset or a CPU address.
type Map struct {
	Name string
	Addr uint32
}

// ParseXDF reads map titles and the address of each constant or table body.
type xdfFile struct {
	Constants []xdfConst `xml:"XDFCONSTANT"`
	Tables    []xdfTable `xml:"XDFTABLE"`
}

type xdfConst struct {
	Title string  `xml:"title"`
	Data  xdfData `xml:"EMBEDDEDDATA"`
}

type xdfTable struct {
	Title string    `xml:"title"`
	Axes  []xdfAxis `xml:"XDFAXIS"`
}

type xdfAxis struct {
	ID   string  `xml:"id,attr"`
	Data xdfData `xml:"EMBEDDEDDATA"`
}

type xdfData struct {
	Addr string `xml:"mmedaddress,attr"`
}

// ParseXDF returns one row per constant and one per table body.
func ParseXDF(b []byte) ([]Map, error) {
	var doc xdfFile
	if err := xml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	var out []Map
	for _, c := range doc.Constants {
		addr, err := parseAddr(c.Data.Addr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.Title, err)
		}
		out = append(out, Map{Name: c.Title, Addr: addr})
	}
	for _, t := range doc.Tables {
		var addr uint32
		var found bool
		for _, ax := range t.Axes {
			if ax.ID != "z" || ax.Data.Addr == "" {
				continue
			}
			a, err := parseAddr(ax.Data.Addr)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", t.Title, err)
			}
			addr, found = a, true
		}
		if !found {
			return nil, fmt.Errorf("%s: no table address", t.Title)
		}
		out = append(out, Map{Name: t.Title, Addr: addr})
	}
	return out, nil
}

func parseAddr(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("bad address %q", s)
	}
	return uint32(v), nil
}
