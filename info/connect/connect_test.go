package connect

import "testing"

func TestChooseGroups(t *testing.T) {
	// Group A: 0x11 is KWP2000, 0x01 is KW1281.
	a := []Entry{
		{Address: 0x11, Key1: 0xEF, Key2: 0x8F, Complement: 0xEE},
		{Address: 0x33, Key1: 0x08, Key2: 0x08},
		{Address: 0x01, Key1: 0x01, Key2: 0x8A, KW1281: true},
	}
	p := Policy{Key1: 0xEF, Key2: 0x8F, Prefer: 0x11, Fallback: 0x01, Fast: []byte{0x01, 0x10}}
	got, ok := Choose(a, p)
	if !ok || got != "SLOW-0x11" {
		t.Fatalf("%s %v", got, ok)
	}
	// Group E: 0x01 is KWP2000, no 0x11.
	e := []Entry{{Address: 0x01, Key1: 0xEF, Key2: 0x8F, Complement: 0xFE}}
	got, ok = Choose(e, p)
	if !ok || got != "SLOW-0x01" {
		t.Fatalf("%s %v", got, ok)
	}
	if _, ok := Choose([]Entry{{Address: 0x33, Key1: 0x08, Key2: 0x08}}, p); ok {
		t.Fatal("OBD-only table should not choose a connect mode")
	}
	if _, ok := FastTarget(0x01, p.Fast); !ok {
		t.Fatal("0x01")
	}
	if _, ok := FastTarget(0x33, p.Fast); ok {
		t.Fatal("0x33 is not a physical target")
	}
}

func TestParseTable(t *testing.T) {
	row := make([]byte, 30)
	row[0] = 0x11
	row[1] = 0x55
	row[2] = 0xEF
	row[3] = 0x8F
	row[6] = 0xEE
	data := append(append([]byte{}, row...), row...)
	data[30] = 0x01
	data[32] = 0x01
	data[33] = 0x8A
	data[35] = 1
	ents := ParseTable(data, 0)
	if len(ents) != 2 || ents[0].Complement != 0xEE || !ents[1].KW1281 {
		t.Fatalf("%+v", ents)
	}
	c, ok := Choose(ents, Policy{Key1: 0xEF, Key2: 0x8F, Prefer: 0x11, Fallback: 0x01})
	if !ok || c != "SLOW-0x11" {
		t.Fatalf("%s %v", c, ok)
	}
}
