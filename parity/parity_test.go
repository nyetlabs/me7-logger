package parity

import (
	"testing"

	"me7-logger/record"
)

func TestMatchECU(t *testing.T) {
	got := []record.Item{{Name: "nmot", Addr: 0xF878, Size: 1, Bitmask: 0}}
	want := []record.Item{
		{Name: "nmot", Addr: 0xF878, Size: 1},
		{Name: "rl", Addr: 0x380100, Size: 2},
	}
	hit, n := matchECU(got, want)
	if hit != 1 || n != 2 {
		t.Fatalf("%d/%d", hit, n)
	}
}

func TestMatchMapsFileOffset(t *testing.T) {
	got := []Map{{Name: "KFZW", Addr: needleBase + 0x1234}}
	want := []Map{{Name: "KFZW", Addr: 0x1234}, {Name: "LAMFA", Addr: 0x2000}}
	hit, n := matchMaps(got, want)
	if hit != 1 || n != 2 {
		t.Fatalf("%d/%d", hit, n)
	}
}

const needleBase = 0x800000

func TestParseXDF(t *testing.T) {
	b := []byte(`<?xml version="1.0"?>
<XDFFORMAT>
<XDFCONSTANT><title>KRKTE</title><EMBEDDEDDATA mmedaddress="0x10" /></XDFCONSTANT>
<XDFTABLE><title>LAMFA</title>
<XDFAXIS id="x"><EMBEDDEDDATA mmedaddress="0x1" /></XDFAXIS>
<XDFAXIS id="z"><EMBEDDEDDATA mmedaddress="0x20" /></XDFAXIS>
</XDFTABLE>
</XDFFORMAT>`)
	got, err := ParseXDF(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "KRKTE" || got[0].Addr != 0x10 || got[1].Name != "LAMFA" || got[1].Addr != 0x20 {
		t.Fatalf("%+v", got)
	}
}

func TestFraction(t *testing.T) {
	if (Fraction{1, 4}).String() != "25.0% (1/4)" {
		t.Fatal((Fraction{1, 4}).String())
	}
	if (Fraction{}).String() != "0.0% (0/0)" {
		t.Fatal((Fraction{}).String())
	}
}
