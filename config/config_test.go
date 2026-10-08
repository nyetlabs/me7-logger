package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/me7-logger/mapfile"
	"go.nyet.org/me7-logger/needle"
)

func TestShippedSignatures(t *testing.T) {
	b, err := Read("", SigFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "frkte_w") || !strings.Contains(string(b), "zwgru") {
		t.Fatalf("signature list missing rows: %s", b)
	}
}

func TestShippedNames(t *testing.T) {
	n, err := LoadNames("")
	if err != nil {
		t.Fatal(err)
	}
	if n.Connect.SlowNeedle != "slow_init_table" || n.Connect.Key1 != 0xEF || n.Connect.Prefer != 0x11 {
		t.Fatalf("%+v", n.Connect)
	}
	if !n.Clock.Injection["te_w"] || n.Clock.KRKTE != "KRKTE" || n.Clock.Factors[32] != 1.0/250 || n.Clock.Factors[20] != 1.0/312.5 || n.Clock.Factors[24] != 1.0/375 || n.Clock.Factors[40] != n.Clock.Factors[20] {
		t.Fatalf("%+v", n.Clock)
	}
	if !n.Scale.Ambient["PUMX"] || n.Scale.Low != 400 || n.Scale.High != 1200 {
		t.Fatalf("%+v", n.Scale)
	}
	if len(n.Selector) != 2 || n.SelectorEnd != 0x3F7 {
		t.Fatalf("selector %d end %X", len(n.Selector), n.SelectorEnd)
	}
}

func TestShippedNeedles(t *testing.T) {
	b, err := Read("", NeedlesFile)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := needle.Parse(b, NeedlesFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ref_offset") {
		t.Fatal("ref_offset is not a needle field")
	}
	if strings.Contains(string(b), "function:") {
		t.Fatal("function is the list, not a field")
	}
	if len(ns) != 56 {
		t.Fatalf("needles %d", len(ns))
	}
	if ns[0].Name != "slow_init_table" {
		t.Fatalf("%+v", ns[0])
	}
	slow, ok := needle.ByName(ns, "slow_init_table")
	if !ok || slow.BackUp != 0 || !slow.Unique || slow.Function || slow.Mask[0] != 0xEF || slow.Mask[6] != 0xEF {
		t.Fatalf("%+v", slow)
	}
	fast, ok := needle.ByName(ns, "fast_init_physical")
	if !ok || fast.BackUp != -14 || !fast.Unique {
		t.Fatalf("%+v", fast)
	}
	sel, ok := needle.ByName(ns, "result_selector_0")
	if !ok || sel.BackUp != -38 || !sel.Unique || len(sel.Pattern) != 46 {
		t.Fatalf("%+v", sel)
	}
	interp, ok := needle.ByName(ns, "map_interp_table8")
	if !ok || interp.Unique || !interp.Function || len(interp.Pattern) != 20 {
		t.Fatalf("%+v", interp)
	}
	interpb, ok := needle.ByName(ns, "map_interp_table8_b")
	if !ok || !interpb.Unique || !interpb.Function || len(interpb.Pattern) != 12 {
		t.Fatalf("%+v", interpb)
	}
	curve, ok := needle.ByName(ns, "map_interp_curve8")
	if !ok || curve.Unique || !curve.Function || len(curve.Pattern) != 8 {
		t.Fatalf("%+v", curve)
	}
	ign, ok := needle.ByName(ns, "ZWGRU_ign_zw")
	if !ok || !ign.Unique || !ign.Function || len(ign.Pattern) != 12 {
		t.Fatalf("%+v", ign)
	}
	ldrq, ok := needle.ByName(ns, "LDRPID_ldrq")
	if !ok || !ldrq.Unique || !ldrq.Function || len(ldrq.Pattern) != 14 {
		t.Fatalf("%+v", ldrq)
	}
	if _, ok := needle.ByName(ns, "KFZW"); ok {
		t.Fatal("map names do not belong in needles.yaml")
	}
	if _, ok := needle.ByName(ns, "dzwb"); ok {
		t.Fatal("measurement needles belong on the measurement row")
	}
	if _, ok := needle.ByName(ns, "CrcTableRef"); ok {
		t.Fatal("checksum needles belong in config/examples")
	}
}

func TestShippedMaps(t *testing.T) {
	calls, err := LoadMaps("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 95 {
		t.Fatalf("maps %d", len(calls))
	}
	ns, err := LoadNeedles("", "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]int{}
	for _, c := range calls {
		if c.At%2 != 0 {
			t.Fatalf("%+v", c)
		}
		caller, ok := needle.ByName(ns, c.Caller)
		if !ok || !caller.Function {
			t.Fatalf("%+v", c)
		}
		interp, ok := needle.ByName(ns, c.Interp)
		if !ok || !interp.Function {
			t.Fatalf("%+v", c)
		}
		got[c.Name] = append(got[c.Name], c.At)
		switch c.Name {
		case "KFZW", "KFZW2", "KFZWWLNM":
			if c.Interp != "map_interp_table8" {
				t.Fatalf("%+v", c)
			}
			if c.Name == "KFZWWLNM" && c.Caller != "ZWGRU_ign_zw" && c.Caller != "ZWGRU_ign_wl" {
				t.Fatalf("%+v", c)
			}
			if c.Name != "KFZWWLNM" && c.Caller != "ZWGRU_ign_zw" {
				t.Fatalf("%+v", c)
			}
		case "KFLDRQ2", "KFLDRL":
			if c.Caller != "LDRPID_ldrq" || c.Interp != "map_interp_table16_page" {
				t.Fatalf("%+v", c)
			}
		case "LDRQ0DY", "LDRQ1DY", "LDRQ1ST":
			if c.Caller != "LDRPID_ldrq" || c.Interp != "map_interp_curve16" {
				t.Fatalf("%+v", c)
			}
		case "KFDMDADP", "KFDMDARO":
			if c.Caller != "ARMD_kfdmd" || c.YTable != "SGA06MDUB" || c.Rows != 6 || c.YBits != 8 {
				t.Fatalf("%+v", c)
			}
		}
	}
	has := func(ats []int, want int) bool {
		for _, a := range ats {
			if a == want {
				return true
			}
		}
		return false
	}
	if got["KFZW"][0] != 0x48 || !has(got["KFZW"], 0x5DE) || !has(got["KFZW"], 0x620) {
		t.Fatalf("%v", got["KFZW"])
	}
	if got["KFZW2"][0] != 0x22 || !has(got["KFZW2"], 0x5B8) || !has(got["KFZW2"], 0x5FC) {
		t.Fatalf("%v", got["KFZW2"])
	}
	if !has(got["KFZWWLNM"], 0x6BC) || !has(got["KFZWWLNM"], 0x2A) {
		t.Fatalf("%v", got["KFZWWLNM"])
	}
	if len(got["KFLDRQ2"]) != 1 || got["KFLDRQ2"][0] != 0x70 || got["LDRQ0DY"][0] != 0x86 || got["LDRQ1DY"][0] != 0xA2 || got["LDRQ1ST"][0] != 0xC8 || got["KFLDRL"][0] != 0x228 {
		t.Fatalf("%v", got)
	}
	if _, err := ParseMaps([]byte("maps:\n- name: KFZW\n  caller: ign_zw\n  at: '0x5'\n  interp: map_interp_table8\n"), "t"); err == nil {
		t.Fatal("odd at accepted")
	}
	listed, err := ParseMaps([]byte("maps:\n- name: KFZW\n  caller: ign_zw\n  at: ['0x22', '0x48']\n  interp: map_interp_table8\n"), "t")
	if err != nil || len(listed) != 2 || listed[0].At != 0x22 || listed[1].At != 0x48 {
		t.Fatalf("%v %+v", err, listed)
	}
}

func TestExampleNeedles(t *testing.T) {
	b, err := os.ReadFile("examples/needles.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ns, err := needle.Parse(b, "examples/needles.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ref_offset") {
		t.Fatal("ref_offset is not a needle field")
	}
	if strings.Contains(string(b), "function:") {
		t.Fatal("function is the list, not a field")
	}
	if len(ns) != 9 {
		t.Fatalf("needles %d", len(ns))
	}
	xor, ok := needle.ByName(ns, "XORChkSumGenerate")
	if !ok || xor.BackUp != 0x14 || !xor.Unique || !xor.Function {
		t.Fatalf("%+v", xor)
	}
	crc, ok := needle.ByName(ns, "CrcTableRef")
	if !ok || crc.BackUp != 0 || crc.Unique || crc.Function {
		t.Fatalf("%+v", crc)
	}
	k27, ok := needle.ByName(ns, "k_27_SecurityAccess")
	if !ok || k27.BackUpMax == nil || *k27.BackUpMax != 0x188 {
		t.Fatalf("%+v", k27)
	}
}

func TestShippedCatalog(t *testing.T) {
	scales, err := LoadScales("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	tab, err := LoadCatalog("", scales)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := tab.Lookup(1, 0)
	if !ok || v.Name != "nmot" || v.A != 40 {
		t.Fatalf("%+v %v", v, ok)
	}
	ab, err := Read("", AliasFile)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := mapfile.ParseAliases(ab)
	if err != nil {
		t.Fatal(err)
	}
	if aliases["nmot_w"] != "RPM" || aliases["nmot"] != "RPM_8" || aliases["rl"] != "EngineLoad_8" {
		t.Fatalf("nmot_w %q nmot %q rl %q", aliases["nmot_w"], aliases["nmot"], aliases["rl"])
	}
	if aliases["gangi"] != "Gear" || aliases["wkr_0"] != "IgnitionRetardCyl$1" {
		t.Fatalf("gangi %q wkr_0 %q", aliases["gangi"], aliases["wkr_0"])
	}
	text := string(ab)
	if !strings.Contains(text, "was: SelectedGear") || !strings.Contains(text, "was: EngineSpeed") {
		t.Fatal("rename notes missing")
	}
	if strings.ContainsRune(text, '\uFFFD') || !strings.Contains(text, "Öl") {
		t.Fatal("alias comments are not UTF-8 German")
	}
	zw, ok := tab.Lookup(0x0009, 0)
	if !ok || zw.Name != "zwout" || !strings.Contains(zw.Comment, "Zündwinkel") {
		t.Fatalf("%+v", zw)
	}
	fcm, ok := tab.Lookup(0x000D, 0x80)
	if !ok || fcm.Name != "fcmEnd" || fcm.A != 1 || fcm.Size != 0 || fcm.Unit != "#" {
		t.Fatalf("%+v", fcm)
	}
	dl, ok := tab.Lookup(0x00DE, 0)
	if !ok || dl.Name != "dlahi_w" || dl.A != 2.0/65536 {
		t.Fatalf("%+v", dl)
	}
	zb, ok := tab.Lookup(0, 0)
	if !ok || zb.Name != "zwbas" || zb.Size != 1 || !zb.Signed || zb.A != 0.75 || zb.Unit != "°KW" {
		t.Fatalf("%+v", zb)
	}
}

func TestCatalogDirOrder(t *testing.T) {
	dir := t.TempDir()
	scales := "scales:\n  kw:\n    unit: °KW\n    factor: 3/4\n    signed: true\n"
	if err := os.WriteFile(filepath.Join(dir, "scales.yaml"), []byte(scales), 0o644); err != nil {
		t.Fatal(err)
	}
	first := "variables:\n- rt: \"0x0001\"\n  name: first\n  scale: kw\n"
	second := "variables:\n- rt: \"0x0001\"\n  name: second\n  unit: rpm\n  factor: 40\n"
	if err := os.WriteFile(filepath.Join(dir, "00.yaml"), []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01.yaml"), []byte(second), 0o644); err != nil {
		t.Fatal(err)
	}
	tab, err := LoadCatalog(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := tab.Lookup(1, 0)
	if !ok || v.Name != "second" || v.Unit != "rpm" || v.A != 40 {
		t.Fatalf("%+v", v)
	}
	one := "scales:\n  kw: &kw\n    unit: °KW\n    factor: 3/4\nvariables:\n- rt: \"0x0009\"\n  name: zwout\n  <<: *kw\n"
	path := filepath.Join(dir, "one.yaml")
	if err := os.WriteFile(path, []byte(one), 0o644); err != nil {
		t.Fatal(err)
	}
	tab, err = LoadCatalog(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	zw, ok := tab.Lookup(9, 0)
	if !ok || zw.Name != "zwout" || zw.Unit != "°KW" || zw.A != 0.75 {
		t.Fatalf("%+v", zw)
	}
}

func TestShippedMeasures(t *testing.T) {
	raw, err := Read("", MeasuresFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "address:") {
		t.Fatal("measurement file contains an address")
	}
	ms, err := LoadMeasures("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) < 100 {
		t.Fatalf("rows %d", len(ms))
	}
	var dzwb, mimax, wkrdy, flag, mibas, dmzms, half, amp Measure
	stubs := 0
	for _, m := range ms {
		if m.Stub {
			stubs++
		}
		switch m.Name {
		case "mibas_w":
			mibas = m
		case "dmzms_w":
			dmzms = m
		case "etazaist":
			half = m
		case "fdar":
			amp = m
		case "nmot":
			if !m.Stub || m.Needle != nil || m.Size != 2 || m.A != 40 || m.Unit != "rpm" || m.Comment != "Motordrehzahl" {
				t.Fatalf("nmot %+v", m)
			}
		case "mizsol_div_redist_ne_0_w":
			if m.Needle == nil || m.Needle.BackUp != -6 {
				t.Fatalf("mizsol %+v", m)
			}
		case "dzwb":
			dzwb = m
		case "mimax_w":
			mimax = m
		case "wkrdy":
			wkrdy = m
		case "B_ldipos":
			flag = m
		case "B_ar":
			if !m.Bit || m.Needle == nil || m.Needle.BackUp != -2 || m.Bitmask != 0x0200 || len(m.Needle.Pats) != 2 {
				t.Fatalf("B_ar %+v", m)
			}
		case "misol_w":
			t.Fatal("commented misol_w should not be loaded")
		}
	}
	if stubs != 1 {
		t.Fatalf("stubs %d", stubs)
	}
	if dzwb.Stub || dzwb.Needle == nil || dzwb.Needle.Function || len(dzwb.Needle.Pats) != 0 || dzwb.Needle.BackUp != -4 || dzwb.Size != 1 || dzwb.A != -0.75 || !dzwb.Signed {
		t.Fatalf("dzwb %+v", dzwb)
	}
	if wkrdy.Signed || wkrdy.A != -0.75 || wkrdy.Unit != "°KW" {
		t.Fatalf("wkrdy %+v", wkrdy)
	}
	if flag.A != 1 || flag.Unit != "" || flag.Bitmask != 0x02 {
		t.Fatalf("B_ldipos %+v", flag)
	}
	if mimax.Alias != "TorqueLimit" || mimax.Size != DefaultSize {
		t.Fatalf("mimax %+v", mimax)
	}
	if mibas.Unit != "" || mibas.A != 100.0/65536 || mibas.Size != DefaultSize || mibas.Signed {
		t.Fatalf("mibas %+v", mibas)
	}
	if !dmzms.Signed || dmzms.A != 100.0/65536 || dmzms.Unit != "" {
		t.Fatalf("dmzms %+v", dmzms)
	}
	if half.Size != 1 || half.Unit != "%" || half.A != 0.5 {
		t.Fatalf("etazaist %+v", half)
	}
	if amp.Size != 1 || amp.Unit != "" || amp.A != 800.0/65536 {
		t.Fatalf("fdar %+v", amp)
	}
}
