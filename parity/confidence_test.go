package parity

import (
	"bytes"
	"testing"

	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/record"
)

func TestConfidence(t *testing.T) {
	axis := &record.Axis{Addr: opcode.FlashBase + 0x40, Count: 4, Bits: 8}
	tuner := []string{"S"}

	t.Run("zeros", func(t *testing.T) {
		img := make([]byte, 32)
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 0 || got.Total != 1 || by["S"].Reason != "zeros" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("short repeated", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0, 0x11, 0x22}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "short" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("miss stays out", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got, by := scoreConfidence([]string{"S", "Gone"}, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 1 || got.Total != 1 || by["Gone"].Reason != "" || by["S"].Reason != "short" {
			t.Fatalf("%+v %+v", got, by)
		}
	})

	t.Run("shared zero", func(t *testing.T) {
		img := make([]byte, 32)
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		peer := binBody{stem: "peerA", img: img, scored: map[string]record.Map{"S": m}}
		got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, []binBody{peer}, false)
		if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "peer-same peerA" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("one cell zero", func(t *testing.T) {
		for _, bits := range []int{8, 16} {
			img := []byte{0, 0}
			peerImg := []byte{1, 1}
			m := record.Map{Name: "S", Addr: opcode.FlashBase, Bits: bits}
			peer := binBody{stem: "peerA", img: peerImg, scored: map[string]record.Map{"S": m}}
			got, by := scoreConfidence(tuner, map[string]int{"S": 0}, img, []record.Map{m}, nil, []binBody{peer}, false)
			if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "short" {
				t.Fatalf("%d bits: %+v %+v", bits, got, by["S"])
			}
		}
	})

	t.Run("zero peer filled", func(t *testing.T) {
		img := make([]byte, 32)
		filled := make([]byte, 32)
		filled[0] = 1
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		peer := binBody{stem: "peerA", img: filled, scored: map[string]record.Map{"S": m}}
		got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, []binBody{peer}, false)
		if got.Hit != 0 || got.Total != 1 || by["S"].Reason != "zeros peerA" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("short once", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		other := record.Map{Name: "T", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got, by := scoreConfidence(tuner, nil, img, []record.Map{m, other}, nil, nil, false)
		if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "short" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("exact", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		m := subject(opcode.FlashBase+16, axis)
		got, by := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, false)
		if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "peer-same peerA" {
			t.Fatalf("exact %+v %+v body %x", got, by["S"], m.Addr)
		}
	})

	t.Run("two cells", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		peer[16], peer[17] = 0x99, 0x98
		got, by := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, false)
		if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "peer-near 2/16 peerA" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("four cells of a large map", func(t *testing.T) {
		img := bytes.Repeat([]byte{0x22}, 16+192+16)
		peer := bytes.Repeat([]byte{0x22}, len(img))
		peer[16], peer[17], peer[18], peer[19] = 1, 2, 3, 4
		axis := &record.Axis{Addr: opcode.FlashBase + 0x1000, Count: 4, Bits: 8}
		maps := []record.Map{
			{Name: "B", Addr: opcode.FlashBase, Cols: 16, X: axis},
			{Name: "S", Addr: opcode.FlashBase + 16, Cols: 192, X: axis},
			{Name: "C", Addr: opcode.FlashBase + 16 + 192, Cols: 16, X: axis},
		}
		got, by := scoreConfidence(tuner, nil, img, maps, nil, []binBody{{
			stem: "peerA", img: peer, maps: maps, scored: map[string]record.Map{"S": maps[1]},
		}}, false)
		if got.Hit != 1 || got.Total != 1 || by["S"].Reason != "peer-near 4/192 peerA" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("six cells of a large map", func(t *testing.T) {
		img := bytes.Repeat([]byte{0x22}, 16+192+16)
		peer := bytes.Repeat([]byte{0x22}, len(img))
		for i := 16; i < 22; i++ {
			peer[i] = byte(i)
		}
		axis := &record.Axis{Addr: opcode.FlashBase + 0x1000, Count: 4, Bits: 8}
		maps := []record.Map{
			{Name: "B", Addr: opcode.FlashBase, Cols: 16, X: axis},
			{Name: "S", Addr: opcode.FlashBase + 16, Cols: 192, X: axis},
			{Name: "C", Addr: opcode.FlashBase + 16 + 192, Cols: 16, X: axis},
		}
		got, by := scoreConfidence(tuner, nil, img, maps, nil, []binBody{{
			stem: "peerA", img: peer, maps: maps, scored: map[string]record.Map{"S": maps[1]},
		}}, false)
		if got.Hit != 0 || got.Total != 1 || by["S"].Reason != "peer-diff 6/192 peerA" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("one axis of a map", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got, by := scoreConfidence(tuner, map[string]int{"S": 2}, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 0 || got.Total != 1 || by["S"].Reason != "shape" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("axis turns", func(t *testing.T) {
		img := []byte{0x11, 0x22, 1, 3, 2, 4}
		x := &record.Axis{Addr: opcode.FlashBase + 2, Count: 4, Bits: 8}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: x}
		if got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false); got.Hit != 0 || by["S"].Reason != "axis" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
		img[3], img[4] = 2, 3
		if got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false); got.Hit != 1 || by["S"].Reason != "short" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("axis all zeros", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0, 0, 0}
		x := &record.Axis{Addr: opcode.FlashBase + 2, Count: 4, Bits: 8}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: x}
		got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 0 || by["S"].Reason != "axis-zeros" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
		if got := by["S"].Comment("map_interp_table8"); got != "map_interp_table8: all zeros" {
			t.Fatalf("comment %q", got)
		}
	})

	t.Run("signed axis", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0xFE, 0xFF, 0, 1}
		x := &record.Axis{Addr: opcode.FlashBase + 2, Count: 4, Bits: 8}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: x}
		if got, by := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false); got.Hit != 1 || by["S"].Reason != "short" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("verified skips peer", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x77)
		for _, verified := range []bool{false, true} {
			got, by := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, verified)
			if got.Hit == 1 != verified || got.Total != 1 {
				t.Fatalf("verified %v: %+v %+v", verified, got, by["S"])
			}
			if verified && by["S"].Reason != "verified" {
				t.Fatalf("verified reason %+v", by["S"])
			}
			if !verified && by["S"].Reason != "peer-diff 16/16 peerA" {
				t.Fatalf("unverified reason %+v", by["S"])
			}
		}
		x := &record.Axis{Addr: opcode.FlashBase, Count: 4, Bits: 8}
		img[1] = 0x10
		maps := trio(x)
		if got, by := scoreConfidence(tuner, nil, img, maps, nil, []binBody{peerBody(peer, x)}, true); got.Hit != 0 || by["S"].Reason != "axis" {
			t.Fatalf("axis turns: %+v %+v", got, by["S"])
		}
	})

	t.Run("neighbor misses", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		peer[16], peer[17] = 0x99, 0x98
		peer[32], peer[33], peer[34] = 0x44, 0x45, 0x46
		got, by := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, false)
		if got.Hit != 0 || got.Total != 1 || by["S"].Reason != "peer-neighbor 2/16 peerA" {
			t.Fatalf("%+v %+v", got, by["S"])
		}
	})

	t.Run("comment form", func(t *testing.T) {
		c := Conf{High: true, Reason: "peer-near 2/16 peerA"}
		if got := c.Comment("map_interp_table8"); got != "map_interp_table8" {
			t.Fatalf("high %q", got)
		}
		if got := (Conf{High: false, Reason: "zeros"}).Comment(""); got != "zero body with no zero peer" {
			t.Fatalf("zeros %q", got)
		}
		if got := (Conf{High: false, Reason: "peer-diff 6/192 peerA"}).Comment("map_interp_table8"); got != "map_interp_table8: differs from peerA by 6/192 cells" {
			t.Fatalf("peer-diff %q", got)
		}
		if got := (Conf{High: false, Reason: "peer-neighbor 2/16 peerA"}).Comment("n"); got != "n: close to peerA (2/16) but neighbors differ" {
			t.Fatalf("peer-neighbor %q", got)
		}
		zeros := Conf{High: false, Reason: "zeros"}
		once := zeros.Comment("")
		if once != "zero body with no zero peer" {
			t.Fatalf("once %q", once)
		}
		if got := zeros.Comment(once); got != once {
			t.Fatalf("doubled %q", got)
		}
		withNeedle := zeros.Comment("map_interp_table8")
		if got := zeros.Comment(withNeedle); got != withNeedle {
			t.Fatalf("needle doubled %q", got)
		}
	})
}

func bodyImage(cell byte) []byte {
	img := make([]byte, 48)
	for i := 0; i < 16; i++ {
		img[i] = 0x11
		img[16+i] = cell
		img[32+i] = 0x33
	}
	return img
}

func subject(addr uint32, axis *record.Axis) record.Map {
	return record.Map{Name: "S", Addr: addr, Cols: 16, X: axis}
}

func trio(axis *record.Axis) []record.Map {
	return []record.Map{
		{Name: "B", Addr: opcode.FlashBase, Cols: 16, X: axis},
		subject(opcode.FlashBase+16, axis),
		{Name: "C", Addr: opcode.FlashBase + 32, Cols: 16, X: axis},
	}
}

func peerBody(img []byte, axis *record.Axis) binBody {
	maps := trio(axis)
	return binBody{stem: "peerA", img: img, maps: maps, scored: map[string]record.Map{"S": maps[1]}}
}
