package generate

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

// The 23g prologues name the same caller slots as 22m. A map keeps one address.
func TestFCallersKeepOneAddress(t *testing.T) {
	tuner := tunerNames(t)
	for _, path := range parityImages(t) {
		stem := strings.TrimSuffix(filepath.Base(path), ".bin")
		img := readBin(t, path)
		res, err := Generate(Options{Image: img, ImageName: filepath.Base(path), Scale: "off"})
		if err != nil {
			t.Fatal(err)
		}
		addrs := map[string]map[uint32]struct{}{}
		for _, m := range res.Maps {
			if m.Name == "" {
				continue
			}
			if addrs[m.Name] == nil {
				addrs[m.Name] = map[uint32]struct{}{}
			}
			addrs[m.Name][opcode.FileOffset(m.Addr)] = struct{}{}
		}
		for _, n := range tuner {
			if len(addrs[n]) > 1 {
				t.Errorf("%s %s addresses %d", stem, n, len(addrs[n]))
			}
		}
	}
}

func TestFNamesTheMovedCalls(t *testing.T) {
	path, err := ecucorpus.Open(t).Path("8D0907551F-0001")
	if err != nil {
		t.Fatal(err)
	}
	img := readBin(t, path)
	res, err := Generate(Options{Image: img, ImageName: filepath.Base(path), Scale: "off"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"KFVPDKSD", "KFVPDKSE", "KFMPED_UM", "KFMDZOF_UM",
		"KFLDRQ2", "LDRQ0DY", "LDRQ1DY", "LDRQ1ST", "KFLDRL",
		"LDIATA", "KFLDIMX", "TLDIAPN",
		"KFLAMKRL", "KFLAMKR", "KFLAFWL",
		"KFFLLDE", "LDPBN", "LDORXN",
		"KFZW", "KFZW2", "KFZWWLNM", "KFZWOP", "KFZWOP2",
		"KFMRES", "KFMRESK", "KFMIZUFIL", "KFMIZUNS", "KFMIZUOF",
		"RLSALUN", "WDKUGDN", "KFLDIOPU", "KFNLLNST",
		"NFSM", "NLLM", "DWKRMSN", "KRFKLN",
	}
	got := map[string]int{}
	axis := map[string]bool{}
	for _, m := range res.Maps {
		if m.Name == "" {
			continue
		}
		got[m.Name]++
		if m.X != nil && m.X.Addr != 0 || m.Y != nil && m.Y.Addr != 0 {
			axis[m.Name] = true
		}
	}
	for _, n := range want {
		if got[n] != 1 || !axis[n] {
			t.Errorf("%s copies %d axis %v", n, got[n], axis[n])
		}
	}
	shared := map[string]uint32{
		"NFSM": 0x182E3, "NLLM": 0x182E3,
		"DWKRMSN": 0x18267, "KRFKLN": 0x18267,
	}
	for _, m := range res.Maps {
		off, ok := shared[m.Name]
		if !ok {
			continue
		}
		if m.X == nil || opcode.FileOffset(m.X.Addr) != off || m.X.Count != int(img[off-1]) {
			t.Errorf("%s x %+v", m.Name, m.X)
		}
	}
}

func tunerNames(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../testdata/parity/names/tuner.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Names map[string]int `yaml:"names"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(doc.Names))
	for n := range doc.Names {
		out = append(out, n)
	}
	return out
}

func TestFamilyFinderMaps(t *testing.T) {
	caller := map[string]bool{}
	b, err := os.ReadFile("../config/maps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Maps []struct {
			Name string `yaml:"name"`
		} `yaml:"maps"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, m := range doc.Maps {
		caller[m.Name] = true
	}
	found := map[string]int{}
	curve := map[string]int{}
	for _, path := range parityImages(t) {
		img := readBin(t, path)
		res, err := Generate(Options{Image: img, ImageName: filepath.Base(path), Scale: "off"})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range res.Maps {
			if m.Name == "" || caller[m.Name] {
				continue
			}
			found[m.Name]++
			switch m.Name {
			case "FZWWLRLN":
				if m.Rows != 12 || m.Cols != 6 || m.X == nil || m.Y == nil {
					t.Errorf("%s FZWWLRLN rows %d cols %d", filepath.Base(path), m.Rows, m.Cols)
				}
			case "KFMI_UM":
				if m.Rows != 8 || m.Cols != 8 || m.X == nil || m.Y == nil {
					t.Errorf("%s KFMI_UM rows %d cols %d", filepath.Base(path), m.Rows, m.Cols)
				}
			case "MLHFM":
				if m.Cols == 0 && m.Rows == 0 {
					break
				}
				curve[m.Name]++
				if m.Rows != 0 || m.Cols != 512 || m.X != nil || m.Y != nil {
					t.Errorf("%s MLHFM rows %d cols %d", filepath.Base(path), m.Rows, m.Cols)
				}
			case "WFRL":
				curve[m.Name]++
				if m.Rows != 0 || (m.Cols != 26 && m.Cols != 31) || m.X != nil || m.Y != nil {
					t.Errorf("%s WFRL rows %d cols %d", filepath.Base(path), m.Rows, m.Cols)
				}
			}
		}
	}
	// These names are not in the caller list. The family-finder rows located them.
	for _, n := range []string{"KFZW2_0_A", "WFRL", "FZWWLRLN", "KFMI_UM", "MLHFM"} {
		if found[n] == 0 {
			t.Errorf("sample did not locate %s", n)
		}
	}
	for _, n := range []string{"MLHFM", "WFRL"} {
		if curve[n] == 0 {
			t.Errorf("sample did not shape %s", n)
		}
	}
}

func TestKfzwopColumnIsLoad(t *testing.T) {
	c := ecucorpus.Open(t)
	for _, stem := range []string{"4B0906018DQ-0010", "8D0907551H-0003", "4D1907558-0004"} {
		path, err := c.Path(stem)
		if err != nil {
			t.Fatal(err)
		}
		img := readBin(t, path)
		res, err := Generate(Options{Image: img, ImageName: stem})
		if err != nil {
			t.Fatal(err)
		}
		var m record.Map
		var miop record.Map
		for _, got := range res.Maps {
			if got.Name == "KFZWOP" {
				m = got
			}
			if got.Name == "KFMIOP" {
				miop = got
			}
		}
		if m.Cols != 11 || m.Rows != 16 || m.X == nil || m.Y == nil || !loadThenRpm(img, m) {
			t.Errorf("%s KFZWOP %dx%d X %v Y %v", stem, m.Cols, m.Rows, m.X, m.Y)
		}
		if miop.X == nil || miop.X.Addr == 0 || miop.Y == nil || !loadThenRpm(img, miop) {
			t.Errorf("%s KFMIOP X %v Y %v", stem, miop.X, miop.Y)
		}
		var op2 record.Map
		for _, got := range res.Maps {
			if got.Name == "KFZWOP2" {
				op2 = got
			}
		}
		if op2.Cols != 11 || op2.Rows != 16 || !loadThenRpm(img, op2) {
			t.Errorf("%s KFZWOP2 %dx%d X %v Y %v", stem, op2.Cols, op2.Rows, op2.X, op2.Y)
		}
	}
}

func loadThenRpm(img []byte, m record.Map) bool {
	if m.X == nil || m.Y == nil || m.X.Bits != 16 || m.Y.Bits != 16 {
		return false
	}
	load := lastByte(img, m.X) * 0.0234375
	rpm := lastByte(img, m.Y) * 0.25
	return load >= 50 && load <= 250 && rpm >= 4000 && rpm <= 8000
}

// TestAxisSlotFollowsDamos keeps FZWWLRLN and KFMI_UM on the slot the
// agreeing defs use. HF and CM put FZWWLRLN's rpm axis on X, so they are
// not the oracle. The 8-bit rpm scale is factor 40.
func TestAxisSlotFollowsDamos(t *testing.T) {
	if v, ok := config.Conversion("FZWWLRLN", "x", 8, 0); !ok || v.Units != "%" || v.Conversion.Factor != 0.75 {
		t.Fatalf("FZWWLRLN x %+v %v", v, ok)
	}
	if v, ok := config.Conversion("FZWWLRLN", "y", 8, 0); !ok || v.Units != "Upm" || v.Conversion.Factor != 40 {
		t.Fatalf("FZWWLRLN y %+v %v", v, ok)
	}
	if v, ok := config.Conversion("KFMI_UM", "x", 8, 0); !ok || v.Units != "Upm" || v.Conversion.Factor != 40 {
		t.Fatalf("KFMI_UM x %+v %v", v, ok)
	}
	if v, ok := config.Conversion("KFMI_UM", "y", 8, 0); !ok || v.Units != "%" || v.Conversion.Factor != 0.75 {
		t.Fatalf("KFMI_UM y %+v %v", v, ok)
	}
	c := ecucorpus.Open(t)
	for _, stem := range []string{"06A906032S-0004", "4Z7907551R-0003", "8D0907551F-0001", "8D0907551G-0002"} {
		path, err := c.Path(stem)
		if err != nil {
			t.Fatal(err)
		}
		img := readBin(t, path)
		res, err := Generate(Options{Image: img, ImageName: stem})
		if err != nil {
			t.Fatal(err)
		}
		want := damosAxes(t, filepath.Join(c.Dir, "defs", stem+".json"))
		got := map[string]record.Map{}
		for _, m := range res.Maps {
			got[m.Name] = m
		}
		for _, name := range []string{"FZWWLRLN", "KFMI_UM"} {
			m, ok := got[name]
			w, wok := want[name]
			if !ok || !wok || m.X == nil || m.Y == nil || opcode.FileOffset(m.X.Addr) != w[0] || opcode.FileOffset(m.Y.Addr) != w[1] {
				t.Errorf("%s %s gen %v want %v", stem, name, axisOff(m), w)
			}
			if !rpmOnScale(img, m) {
				t.Errorf("%s %s rpm axis is not factor 40", stem, name)
			}
		}
	}
}

func axisOff(m record.Map) [2]uint32 {
	if m.X == nil || m.Y == nil {
		return [2]uint32{}
	}
	return [2]uint32{opcode.FileOffset(m.X.Addr), opcode.FileOffset(m.Y.Addr)}
}

func damosAxes(t *testing.T, path string) map[string][2]uint32 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Objects []struct {
			ID string `json:"id"`
			X  struct {
				Address string `json:"address"`
			} `json:"x"`
			Y struct {
				Address string `json:"address"`
			} `json:"y"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string][2]uint32{}
	for _, o := range doc.Objects {
		if o.ID != "FZWWLRLN" && o.ID != "KFMI_UM" {
			continue
		}
		out[o.ID] = [2]uint32{hexAddr(o.X.Address), hexAddr(o.Y.Address)}
	}
	return out
}

func hexAddr(s string) uint32 {
	n, _ := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 32)
	return uint32(n)
}

// rpmOnScale applies factor 40 to the rpm role and factor 0.75 to the other.
// A swapped pair still lands in both ranges, so the address check above is
// the gate. This rejects an axis whose bytes are not an 8-bit axis point.
func rpmOnScale(img []byte, m record.Map) bool {
	xv, xok := config.Conversion(m.Name, "x", 8, 0)
	yv, yok := config.Conversion(m.Name, "y", 8, 0)
	if !xok || !yok || m.X == nil || m.Y == nil {
		return false
	}
	x, y := lastByte(img, m.X)*xv.Conversion.Factor, lastByte(img, m.Y)*yv.Conversion.Factor
	rpm, load := x, y
	if yv.Units == "Upm" {
		rpm, load = y, x
	} else if xv.Units != "Upm" {
		return false
	}
	return rpm >= 4000 && rpm <= 8000 && load >= 40 && load <= 200
}

func lastByte(img []byte, a *record.Axis) float64 {
	if a == nil || a.Count < 1 {
		return 0
	}
	off := int(opcode.FileOffset(a.Addr)) + a.Count - 1
	if a.Bits == 16 {
		off = int(opcode.FileOffset(a.Addr)) + 2*(a.Count-1)
		if off+2 > len(img) {
			return 0
		}
		return float64(binary.LittleEndian.Uint16(img[off:]))
	}
	if off < 0 || off >= len(img) {
		return 0
	}
	return float64(img[off])
}

func parityImages(t *testing.T) []string {
	t.Helper()
	paths, err := ecucorpus.Open(t).List("../testdata/parity/images.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func readBin(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
