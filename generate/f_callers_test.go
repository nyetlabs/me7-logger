package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"me7-logger/opcode"
)

// The 23g prologues name the same caller slots as 22m. A map keeps one address.
func TestFCallersKeepOneAddress(t *testing.T) {
	wiki := wikiNames(t)
	bins, err := filepath.Glob("../testdata/parity/bin/*.bin")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range bins {
		stem := strings.TrimSuffix(filepath.Base(path), ".bin")
		img := readBin(t, path)
		res, err := Generate(Options{Image: img, ImageName: stem + ".bin", Scale: "off"})
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
		for _, n := range wiki {
			if len(addrs[n]) > 1 {
				t.Errorf("%s %s addresses %d", stem, n, len(addrs[n]))
			}
		}
	}
}

func TestFNamesTheMovedCalls(t *testing.T) {
	img := readBin(t, "../testdata/parity/bin/8D0907551F.bin")
	res, err := Generate(Options{Image: img, ImageName: "8D0907551F.bin", Scale: "off"})
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

func wikiNames(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../testdata/parity/xdf/s4wiki/names.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Names []string `yaml:"names"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Names
}

func readBin(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
