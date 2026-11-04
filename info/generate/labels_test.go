package generate

import (
	"reflect"
	"testing"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/ecu"
	"go.nyet.org/me7-logger/needle"
	"go.nyet.org/me7-logger/record"
)

func TestLabels(t *testing.T) {
	ns, err := needle.Parse([]byte("functions:\n- name: two\n  needle_hex: \"EE EE\"\n- name: one\n  needle_hex: \"AA BB\"\n- name: two\n  needle_hex: \"CC DD\"\n  unique: false\n- name: one\n  needle_hex: \"AA BB\"\n"), "t")
	if err != nil {
		t.Fatal(err)
	}
	img := []byte{0, 0, 0xAA, 0xBB, 0xCC, 0xDD, 0xCC, 0xDD}
	r := &Result{
		Needles: ns,
		File: &ecu.File{Items: []record.Item{
			{Name: "B_x", Addr: 0x380010, Size: 2, Bitmask: 0x0200, Comment: "flag"},
			{Name: "nmot", Addr: 0x380010, Size: 1, Comment: "speed"},
			{Name: "ONE", Addr: 0x380020, Size: 1},
		}},
	}
	x, y := model.Addr(0x10100), model.Addr(0x10000)
	objs := []*model.Object{
		{ID: "KFX", Shape: "2d", Address: 0x10000, Rows: 2, Cols: 2, Data: model.Data{Bits: 8}, Description: "map", Value: model.Value{Units: "%"},
			X: &model.Axis{Address: &x}, Y: &model.Axis{Address: &y}},
		{ID: "CWX", Shape: "value", Address: 0x10200, Rows: 1, Cols: 1, Data: model.Data{Bits: 16}, Comment: "word"},
	}
	want := []Label{
		{0x380010, "nmot", 1, "speed", false},
		{0x380010, "B_x", 0, "flag mask 0x200", true},
		{0x800002, "one", 0, "me7info needle", false},
		{0x800004, "two_800004", 0, "me7info needle", false},
		{0x800006, "two_800006", 0, "me7info needle", false},
		{0x810000, "KFX", 0, "map [%]", false},
		{0x810100, "KFX_x", 0, "map", false},
		{0x810200, "CWX", 2, "word", false},
	}
	if got := r.Labels(img, objs); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}
