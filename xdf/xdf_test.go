package xdf

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"
	kitxdf "go.nyet.org/xdfkit/xdf"

	"go.nyet.org/me7-logger/record"
)

// roundTrip writes m as XDF and reads it back.
func roundTrip(t *testing.T, m *model.Model) (*model.Model, string) {
	t.Helper()
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	b, err := kitxdf.Write(m, make([]byte, 0x100000), "t")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := kitxdf.Read(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got, string(b)
}

func TestModelConstantAndTable(t *testing.T) {
	m, _ := Model([]record.Map{
		{Name: "CONST", Addr: 0x810010, Bits: 16, Unit: "ms", Comment: "constant"},
		{
			Name: "MAP", Addr: 0x820000, Bits: 8, Signed: true, Rows: 2, Cols: 3, Unit: "°KW",
			X: &record.Axis{Count: 3, Unit: "rpm"},
			Y: &record.Axis{Addr: 0x810020, Count: 2, Bits: 8},
		},
		{Addr: 0x830000, Bits: 8},
	}, nil, "t")
	got, text := roundTrip(t, m)
	if len(got.Objects) != 2 {
		t.Fatalf("%d objects", len(got.Objects))
	}
	c, k := got.Objects[0], got.Objects[1]
	if c.ID != "CONST" || c.Shape != "value" || c.Address != 0x10010 || c.Data.Bits != 16 || c.Data.Endian != "little" || c.Value.Units != "ms" || c.Description != "constant" {
		t.Fatalf("constant %+v", c)
	}
	if k.ID != "MAP" || k.Shape != "2d" || k.Address != 0x20000 || k.Rows != 2 || k.Cols != 3 || !k.Data.Signed {
		t.Fatalf("table %+v", k)
	}
	if k.X == nil || k.X.Source != "ordinal" || k.Y == nil || k.Y.Address == nil || *k.Y.Address != 0x10020 || k.Y.Data.Bits != 8 {
		t.Fatalf("axes %+v %+v", k.X, k.Y)
	}
	if strings.Contains(text, "0x810010") || strings.Contains(text, "0x820000") {
		t.Fatal("CPU address leaked into the file")
	}
}

// TestModelLinkedAxis: a row axis at a curve holding its breakpoints links to
// it and keeps its own location.
func TestModelLinkedAxis(t *testing.T) {
	m, _ := Model([]record.Map{
		{Name: "AXIS", Addr: 0x810010, Bits: 8, Rows: 1, Cols: 2},
		{Name: "MAP", Addr: 0x820000, Bits: 8, Rows: 2, Cols: 3, Y: &record.Axis{Addr: 0x810010, Count: 2, Bits: 8}},
	}, nil, "t")
	got, text := roundTrip(t, m)
	if !strings.Contains(text, `linkobjid="0x1"`) {
		t.Fatal("no link to AXIS:\n" + text)
	}
	if y := got.Objects[1].Y; y == nil || y.Address == nil || *y.Address != 0x10010 {
		t.Fatalf("linked axis %+v", y)
	}
}

// TestModelBreakpoints: a constant at table axis addresses becomes a curve of
// their points and is linked, unless the axes there disagree, the points
// don't strictly increase, or the curve runs into another object.
func TestModelBreakpoints(t *testing.T) {
	img := make([]byte, 0x20000)
	copy(img[0x10010:], []byte{100, 0, 200, 0, 0x2C, 1, 0x90, 1})
	copy(img[0x10300:], []byte{1, 2, 3, 4})
	m, conflicts := Model([]record.Map{
		{Name: "SNM", Addr: 0x810010},
		{Name: "KF1", Addr: 0x820000, Rows: 4, Cols: 2, Y: &record.Axis{Addr: 0x810010, Count: 4, Bits: 16}},
		{Name: "KF2", Addr: 0x820100, Rows: 4, Cols: 2, Y: &record.Axis{Addr: 0x810010, Count: 4, Bits: 16}},
		{Name: "SNX", Addr: 0x810100},
		{Name: "KF3", Addr: 0x820200, Rows: 4, Cols: 2, Y: &record.Axis{Addr: 0x810100, Count: 4, Bits: 8}},
		{Name: "KF4", Addr: 0x820300, Rows: 6, Cols: 2, Y: &record.Axis{Addr: 0x810100, Count: 6, Bits: 8}},
		{Name: "SNZ", Addr: 0x810200},
		{Name: "KF5", Addr: 0x820400, Rows: 3, Cols: 2, Y: &record.Axis{Addr: 0x810200, Count: 3, Bits: 8}},
		{Name: "SNO", Addr: 0x810300},
		{Name: "NEXT", Addr: 0x810302},
		{Name: "KF6", Addr: 0x820500, Rows: 4, Cols: 2, Y: &record.Axis{Addr: 0x810300, Count: 4, Bits: 8}},
	}, img, "t")
	if o := m.Objects[0]; o.Shape != "1d" || o.Rows != 1 || o.Cols != 4 || o.Data.Bits != 16 || o.Data.Endian != "little" {
		t.Fatalf("SNM %+v", o)
	}
	want := []string{"SNX at 0x10100: axes of 2", "SNZ at 0x10200: breakpoints not increasing", "SNO at 0x10300: overlaps NEXT"}
	if len(conflicts) != len(want) {
		t.Fatalf("conflicts %q", conflicts)
	}
	for i, w := range want {
		if !strings.HasPrefix(conflicts[i], w) {
			t.Errorf("conflict %q, want %q", conflicts[i], w)
		}
	}
	for _, i := range []int{3, 6, 8} {
		if o := m.Objects[i]; o.Shape != "value" {
			t.Errorf("%s promoted: %+v", o.Key, o)
		}
	}
	if _, text := roundTrip(t, m); strings.Count(text, `linkobjid="0x1"`) != 2 || strings.Count(text, "linkobjid") != 2 {
		t.Fatal("want KF1 and KF2 linked to SNM only:\n" + text)
	}
}

func TestModelUnknownRowsIsConstant(t *testing.T) {
	m, _ := Model([]record.Map{{
		Name: "KFZW", Addr: 0x811C72, Cols: 12,
		X: &record.Axis{Addr: 0x8100FF, Count: 12, Bits: 8},
	}, {
		Name: "KLAF", Addr: 0x812000, Rows: 1, Cols: 6,
		X: &record.Axis{Addr: 0x811FFA, Count: 6, Bits: 8},
	}}, nil, "t")
	if o := m.Objects[0]; o.Shape != "value" || o.Address != 0x11C72 || o.X != nil {
		t.Fatalf("%+v", o)
	}
	if o := m.Objects[1]; o.Shape != "1d" || o.Cols != 6 || o.X == nil || *o.X.Address != 0x11FFA {
		t.Fatalf("%+v", o)
	}
	if p := m.Provenance; p.Format != "image" || p.Origin != "located" || len(p.SHA256) != 64 {
		t.Fatalf("%+v", p)
	}
}

// TestModelCategories: the tuner subset keeps listed maps and the maps their
// axes point at, in the listed map's category; the full set files the rest
// under Other.
func TestModelCategories(t *testing.T) {
	maps := []record.Map{
		{Name: "SNM16ZWUB", Addr: 0x810010, Bits: 8, Rows: 1, Cols: 16},
		{Name: "KFZW", Addr: 0x820000, Bits: 8, Rows: 16, Cols: 12, Y: &record.Axis{Addr: 0x810010, Count: 16, Bits: 8}},
		{Name: "UNLISTED", Addr: 0x830000, Bits: 8},
	}
	cats, err := model.ParseCategoryTable([]byte(`{"schema":1,"categories":{"KFZW":"Timing","ABSENT":"Boost"}}`))
	if err != nil {
		t.Fatal(err)
	}
	tuner, _ := Model(maps, nil, "t")
	if err := tuner.Tuner(cats); err != nil {
		t.Fatal(err)
	}
	full, _ := Model(maps, nil, "t")
	full.Categorize(cats, Other)
	for _, tc := range []struct {
		m    *model.Model
		want string
	}{
		{tuner, "SNM16ZWUB=Timing KFZW=Timing"},
		{full, "SNM16ZWUB=Timing KFZW=Timing UNLISTED=Other"},
	} {
		got, _ := roundTrip(t, tc.m)
		var s []string
		for _, o := range got.Objects {
			s = append(s, o.ID+"="+got.Categories[o.Categories[0]].Name)
		}
		if strings.Join(s, " ") != tc.want {
			t.Errorf("got %q, want %q", strings.Join(s, " "), tc.want)
		}
	}
}
