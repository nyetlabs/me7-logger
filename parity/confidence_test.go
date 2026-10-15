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
		got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("short repeated", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0, 0x11, 0x22}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("miss stays out", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence([]string{"S", "Gone"}, nil, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("shared zero", func(t *testing.T) {
		img := make([]byte, 32)
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		peer := binBody{img: img, scored: map[string]record.Map{"S": m}}
		got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, []binBody{peer}, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("one cell zero", func(t *testing.T) {
		for _, bits := range []int{8, 16} {
			img := []byte{0, 0}
			peerImg := []byte{1, 1}
			m := record.Map{Name: "S", Addr: opcode.FlashBase, Bits: bits}
			peer := binBody{img: peerImg, scored: map[string]record.Map{"S": m}}
			got := scoreConfidence(tuner, map[string]int{"S": 0}, img, []record.Map{m}, nil, []binBody{peer}, false)
			if got.Hit != 1 || got.Total != 1 {
				t.Fatalf("%d bits: %+v", bits, got)
			}
		}
	})

	t.Run("zero peer filled", func(t *testing.T) {
		img := make([]byte, 32)
		filled := make([]byte, 32)
		filled[0] = 1
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		peer := binBody{img: filled, scored: map[string]record.Map{"S": m}}
		got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, []binBody{peer}, false)
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("short once", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		other := record.Map{Name: "T", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence(tuner, nil, img, []record.Map{m, other}, nil, nil, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("exact", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		m := subject(opcode.FlashBase+16, axis)
		got := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("exact %+v body %x", got, m.Addr)
		}
	})

	t.Run("two cells", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		peer[16], peer[17] = 0x99, 0x98
		got := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
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
		got := scoreConfidence(tuner, nil, img, maps, nil, []binBody{{
			img: peer, maps: maps, scored: map[string]record.Map{"S": maps[1]},
		}}, false)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("six cells of a large map", func(t *testing.T) {
		img := bytes.Repeat([]byte{0x22}, 16+192+16)
		peer := bytes.Repeat([]byte{0x22}, len(img))
		for i := 16; i < 22; i++ {
			peer[i] = byte(i)
		}
		maps := []record.Map{
			{Name: "B", Addr: opcode.FlashBase, Cols: 16, X: axis},
			{Name: "S", Addr: opcode.FlashBase + 16, Cols: 192, X: axis},
			{Name: "C", Addr: opcode.FlashBase + 16 + 192, Cols: 16, X: axis},
		}
		got := scoreConfidence(tuner, nil, img, maps, nil, []binBody{{
			img: peer, maps: maps, scored: map[string]record.Map{"S": maps[1]},
		}}, false)
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("one axis of a map", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence(tuner, map[string]int{"S": 2}, img, []record.Map{m}, nil, nil, false)
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("axis turns", func(t *testing.T) {
		img := []byte{0x11, 0x22, 1, 3, 2, 4}
		x := &record.Axis{Addr: opcode.FlashBase + 2, Count: 4, Bits: 8}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: x}
		if got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false); got.Hit != 0 {
			t.Fatalf("%+v", got)
		}
		img[3], img[4] = 2, 3
		if got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false); got.Hit != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("signed axis", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0xFE, 0xFF, 0, 1}
		x := &record.Axis{Addr: opcode.FlashBase + 2, Count: 4, Bits: 8}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: x}
		if got := scoreConfidence(tuner, nil, img, []record.Map{m}, nil, nil, false); got.Hit != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("verified skips peer", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x77)
		for _, verified := range []bool{false, true} {
			got := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, verified)
			if got.Hit == 1 != verified || got.Total != 1 {
				t.Fatalf("verified %v: %+v", verified, got)
			}
		}
		x := &record.Axis{Addr: opcode.FlashBase, Count: 4, Bits: 8}
		img[1] = 0x10
		maps := trio(x)
		if got := scoreConfidence(tuner, nil, img, maps, nil, []binBody{peerBody(peer, x)}, true); got.Hit != 0 {
			t.Fatalf("axis turns: %+v", got)
		}
	})

	t.Run("neighbor misses", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		peer[16], peer[17] = 0x99, 0x98
		peer[32], peer[33], peer[34] = 0x44, 0x45, 0x46
		got := scoreConfidence(tuner, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)}, false)
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
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
	return binBody{img: img, maps: maps, scored: map[string]record.Map{"S": maps[1]}}
}
