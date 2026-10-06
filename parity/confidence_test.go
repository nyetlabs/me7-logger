package parity

import (
	"testing"

	"me7-logger/opcode"
	"me7-logger/record"
)

func TestConfidence(t *testing.T) {
	axis := &record.Axis{Addr: opcode.FlashBase + 0x40, Count: 4, Bits: 8}
	wiki := []string{"S"}

	t.Run("zeros", func(t *testing.T) {
		img := make([]byte, 32)
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		got := scoreConfidence(wiki, nil, img, []record.Map{m}, nil, nil)
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("short repeated", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0, 0x11, 0x22}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence(wiki, nil, img, []record.Map{m}, nil, nil)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("miss stays out", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence([]string{"S", "Gone"}, nil, img, []record.Map{m}, nil, nil)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("shared zero", func(t *testing.T) {
		img := make([]byte, 32)
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		peer := binBody{img: img, scored: map[string]record.Map{"S": m}}
		got := scoreConfidence(wiki, nil, img, []record.Map{m}, nil, []binBody{peer})
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("zero peer filled", func(t *testing.T) {
		img := make([]byte, 32)
		filled := make([]byte, 32)
		filled[0] = 1
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Rows: 4, Cols: 4, X: axis}
		peer := binBody{img: filled, scored: map[string]record.Map{"S": m}}
		got := scoreConfidence(wiki, nil, img, []record.Map{m}, nil, []binBody{peer})
		if got.Hit != 0 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("short once", func(t *testing.T) {
		img := []byte{0x11, 0x22, 0, 0}
		m := record.Map{Name: "S", Addr: opcode.FlashBase, Cols: 2, X: axis}
		other := record.Map{Name: "T", Addr: opcode.FlashBase, Cols: 2, X: axis}
		got := scoreConfidence(wiki, nil, img, []record.Map{m, other}, nil, nil)
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("exact", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		m := subject(opcode.FlashBase+16, axis)
		got := scoreConfidence(wiki, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)})
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("exact %+v body %x", got, m.Addr)
		}
	})

	t.Run("two cells", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		peer[16], peer[17] = 0x99, 0x98
		got := scoreConfidence(wiki, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)})
		if got.Hit != 1 || got.Total != 1 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("neighbor misses", func(t *testing.T) {
		img := bodyImage(0x22)
		peer := bodyImage(0x22)
		peer[16], peer[17] = 0x99, 0x98
		peer[32], peer[33], peer[34] = 0x44, 0x45, 0x46
		got := scoreConfidence(wiki, nil, img, trio(axis), nil, []binBody{peerBody(peer, axis)})
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
