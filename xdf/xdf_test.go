package xdf

import (
	"strings"
	"testing"

	"go.nyet.org/me7-logger/parity"
	"go.nyet.org/me7-logger/record"
)

func TestWriteConstantAndTable(t *testing.T) {
	var b strings.Builder
	err := Write(&b, "8D0907551M-0002", 0x100000, []record.Map{
		{Name: "KRKTE", Addr: 0x810010, Bits: 16, Unit: "ms/%", Equation: "0.000111 * X", Comment: "injector"},
		{
			Name: "KFZW", Addr: 0x820000, Bits: 8, Signed: true, Rows: 2, Cols: 3, Unit: "°KW", Equation: "0.75 * X",
			X: &record.Axis{Count: 3, Labels: []float64{800, 2000}, Unit: "rpm"},
			Y: &record.Axis{Addr: 0x810010, Count: 2, Bits: 8},
		},
	}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parity.ParseXDF([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "KRKTE" || got[0].Addr != 0x10010 || got[1].Name != "KFZW" || got[1].Addr != 0x20000 {
		t.Fatalf("%+v", got)
	}
	text := b.String()
	if !strings.Contains(text, `mmedaddress="0x10010"`) || !strings.Contains(text, `equation="0.000111 * X"`) {
		t.Fatal(text)
	}
	if !strings.Contains(text, `uniqueid="0x10010"`) || !strings.Contains(text, `size="0x100000"`) {
		t.Fatal(text)
	}
	if !strings.Contains(text, `index="0xFF"`) || !strings.Contains(text, `name="Axes"`) || !strings.Contains(text, `linkobjid="0x10010"`) {
		t.Fatal(text)
	}
	if strings.Contains(text, "0x810010") || strings.Contains(text, "0x820000") {
		t.Fatal("CPU address leaked into the file")
	}
}

func TestWriteUnknownRowsIsConstant(t *testing.T) {
	var b strings.Builder
	err := Write(&b, "t", 0, []record.Map{{
		Name: "KFZW", Addr: 0x811C72, Cols: 12,
		X: &record.Axis{Addr: 0x8100FF, Count: 12, Bits: 8},
	}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	text := b.String()
	if !strings.Contains(text, "<XDFCONSTANT ") || strings.Contains(text, "mmedrowcount") {
		t.Fatal(text)
	}
	if !strings.Contains(text, `mmedaddress="0x11C72"`) {
		t.Fatal(text)
	}
}

// TestWriteCategories: the tuner xdf keeps listed maps and the maps their
// axes link to, in the listed map's category; the full xdf files the rest
// under Other.
func TestWriteCategories(t *testing.T) {
	maps := []record.Map{
		{Name: "SNM16ZWUB", Addr: 0x810010, Bits: 8, Rows: 1, Cols: 16},
		{Name: "KFZW", Addr: 0x820000, Bits: 8, Rows: 16, Cols: 12, Y: &record.Axis{Addr: 0x810010, Count: 16, Bits: 8}},
		{Name: "UNLISTED", Addr: 0x830000, Bits: 8},
	}
	cats := map[string]string{"KFZW": "Timing", "ABSENT": "Boost"}
	for _, tc := range []struct {
		tuner bool
		n     int
		want  []string
	}{
		{true, 2, []string{`<CATEGORY index="0x0" name="Timing">`, `<CATEGORY index="0xFF" name="Axes">`, `<CATEGORYMEM index="0" category="1">`}},
		{false, 3, []string{`<CATEGORY index="0x0" name="Other">`, `<CATEGORY index="0x1" name="Timing">`, `<CATEGORYMEM index="0" category="2">`}},
	} {
		var b strings.Builder
		if err := Write(&b, "t", 0, maps, cats, tc.tuner); err != nil {
			t.Fatal(err)
		}
		text := b.String()
		got, err := parity.ParseXDF([]byte(text))
		if err != nil || len(got) != tc.n || Count(maps, cats, tc.tuner) != tc.n {
			t.Fatalf("tuner %v: %v %+v", tc.tuner, err, got)
		}
		for _, w := range tc.want {
			if !strings.Contains(text, w) {
				t.Errorf("tuner %v: no %s in\n%s", tc.tuner, w, text)
			}
		}
	}
}

func TestWriteEmpty(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, "t", 0, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := Write(&b, "t", 0, []record.Map{{Addr: 0x810000, Bits: 8}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 0 {
		t.Fatal(b.String())
	}
}
