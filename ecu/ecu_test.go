package ecu

import (
	"strings"
	"testing"

	"me7-logger/ident"
	"me7-logger/record"
)

func TestRoundTrip(t *testing.T) {
	f := &File{
		Connect:   "SLOW-0x11",
		Fast:      0x01,
		ImageName: "m.bin",
		ImageSize: 16,
		ID: ident.ID{
			HWNumber: "0261207143", SWNumber: "1037360857",
			PartNumber: "8D0907551M  ", SWVersion: "0002", EngineID: "2.7l V6/5VT",
		},
		Items: []record.Item{{
			Name: "nmot", Alias: "EngineSpeed",
			Addr: 0x00F88A, Size: 1, Unit: "rpm", A: 40, Comment: "speed",
		}, {
			Name: "ps", Unit: "mbar", Addr: 0x380010,
			Size: 2, A: 0.0390625, Guessed: true,
		}},
	}
	got, err := ParseBytes(f.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.Connect != "SLOW-0x11" || got.Fast != 0x01 {
		t.Fatalf("connect %s fast %02X", got.Connect, got.Fast)
	}
	if got.ID.HWNumber != "0261207143" || !strings.Contains(got.ID.PartNumber, "8D0907551M") {
		t.Fatalf("id %+v", got.ID)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items %d", len(got.Items))
	}
	if got.Items[0].Name != "nmot" || got.Items[0].A != 40 || got.Items[0].Alias != "EngineSpeed" {
		t.Fatalf("nmot %+v", got.Items[0])
	}
	if !got.Items[1].Guessed || got.Items[1].Unit != "mbar" {
		t.Fatalf("ps %+v", got.Items[1])
	}
}

func TestScaleAndClock(t *testing.T) {
	items := []record.Item{
		{Name: "te_w", A: 0.0026666667, Unit: "ms"},
		{Name: "KRKTE", A: 0.0001},
		{Name: "ps_w", A: 0.0390625, Unit: "mbar"},
		{Name: "rl", A: 0.75, Unit: "%"},
	}
	inj := map[string]bool{"te_w": true}
	ApplyClock(items, 32, inj, "KRKTE", 0.004)
	if items[0].A != 0.004 || items[1].A != 0.004/24 {
		t.Fatalf("clock %+v", items[:2])
	}
	ambient := map[string]bool{"PUMN": true, "PUMX": true, "PUE": true}
	on, note := Decide5120(ScaleOn, nil, ambient, 400, 1200)
	if !on || !strings.Contains(note, "override on") {
		t.Fatal(note)
	}
	ScaleMbar(items, []string{"mbar", "mBar"})
	if items[2].A != 0.0390625*2 || items[3].A != 0.75 {
		t.Fatalf("scaled %+v", items[2:])
	}
	apply, note := Decide5120(ScaleAuto, []Const{{Name: "PUMX", Raw: 1000, A: 1, B: 0}}, ambient, 400, 1200)
	if apply || !strings.Contains(note, "stock") {
		t.Fatal(note)
	}
	apply, note = Decide5120(ScaleAuto, []Const{{Name: "PUMX", Raw: 1000, A: 0.5, B: 0}}, ambient, 400, 1200)
	if apply {
		t.Fatal(note)
	}
	apply, note = Decide5120(ScaleAuto, []Const{{Name: "PUE", Raw: 1000, A: 0.3, B: 0}}, ambient, 400, 1200)
	if !apply || !strings.Contains(note, "doubled") {
		t.Fatal(note)
	}
	off, note := Decide5120(ScaleOff, []Const{{Name: "PUE", Raw: 2000, A: 1}}, ambient, 400, 1200)
	if off || !strings.Contains(note, "override off") {
		t.Fatal(note)
	}
	if _, note := Decide5120(ScaleAuto, nil, ambient, 400, 1200); !strings.Contains(note, "no pressure constants") {
		t.Fatal(note)
	}
}

func TestParseOracleShape(t *testing.T) {
	m := "" +
		"[Communication]\n" +
		"Connect = SLOW-0x11 ; Possible values: SLOW-0x11, FAST-0x01\n" +
		"[Identification]\n" +
		"HWNumber = {0261207143}\n" +
		"PartNumber = {8D0907551M  }\n" +
		"[Measurements]\n" +
		"nmot, {EngineSpeed}, 0x380100, 2, 0x0000, {rpm}, 0, 0, 40, 0, {" + string([]byte{0xFC}) + "}\n"
	f, err := ParseBytes([]byte(m))
	if err != nil {
		t.Fatal(err)
	}
	if f.Connect != "SLOW-0x11" || f.Fast != 0x01 {
		t.Fatalf("connect %s fast %02X", f.Connect, f.Fast)
	}
	if strings.TrimSpace(f.ID.PartNumber) != "8D0907551M" || f.ID.HWNumber != "0261207143" {
		t.Fatalf("id %+v", f.ID)
	}
	if f.Items[0].Comment != "\u00fc" {
		t.Fatalf("comment %q", f.Items[0].Comment)
	}
	nl := "Connect = SLOW-0x11 ; Possible values: SLOW-0x11, FAST-0x10\n"
	f, err = ParseBytes([]byte(nl))
	if err != nil {
		t.Fatal(err)
	}
	if f.Connect != "SLOW-0x11" || f.Fast != 0x10 {
		t.Fatalf("nl connect %s fast %02X", f.Connect, f.Fast)
	}
}
