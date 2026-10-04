package xdf

import (
	"strings"
	"testing"

	"me7-logger/parity"
	"me7-logger/record"
)

func TestWriteConstantAndTable(t *testing.T) {
	var b strings.Builder
	err := Write(&b, "8D0907551M-0002", []record.Map{
		{Name: "KRKTE", Addr: 0x810010, Bits: 16, Unit: "ms/%", Equation: "0.000111 * X", Comment: "injector"},
		{
			Name: "KFZW", Addr: 0x820000, Bits: 8, Signed: true, Rows: 2, Cols: 3, Unit: "°KW", Equation: "0.75 * X",
			X: &record.Axis{Count: 3, Labels: []float64{800, 2000}, Unit: "rpm"},
			Y: &record.Axis{Addr: 0x820100, Count: 2, Bits: 8},
		},
	})
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
	if strings.Contains(text, "0x810010") || strings.Contains(text, "0x820000") {
		t.Fatal("CPU address leaked into the file")
	}
}

func TestWriteUnknownRowsIsConstant(t *testing.T) {
	var b strings.Builder
	err := Write(&b, "t", []record.Map{{
		Name: "KFZW", Addr: 0x811C72, Cols: 12,
		X: &record.Axis{Addr: 0x8100FF, Count: 12, Bits: 8},
	}})
	if err != nil {
		t.Fatal(err)
	}
	text := b.String()
	if !strings.Contains(text, "<XDFCONSTANT>") || strings.Contains(text, "mmedrowcount") {
		t.Fatal(text)
	}
	if !strings.Contains(text, `mmedaddress="0x11C72"`) {
		t.Fatal(text)
	}
}

func TestWriteEmpty(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, "t", nil); err != nil {
		t.Fatal(err)
	}
	if err := Write(&b, "t", []record.Map{{Addr: 0x810000, Bits: 8}}); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 0 {
		t.Fatal(b.String())
	}
}
