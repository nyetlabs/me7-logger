package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/opcode"
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
				if m.Rows != 6 || m.Cols != 12 || m.X == nil || m.Y == nil {
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
