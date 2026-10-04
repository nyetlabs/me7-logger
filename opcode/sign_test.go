package opcode

import (
	"encoding/binary"
	"testing"
)

func TestNmotSignature(t *testing.T) {
	img := readHex(t, "sign-nmot.hex")
	got := Signatures(img, StandardDPP, nil)
	want := []Named{
		{"nmotll", 0x380100, 1},
		{"nmot_w", 0x380102, 2},
		{"nmot", 0x380104, 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestUhfmSingle(t *testing.T) {
	img := readHex(t, "sign-uhfm.hex")
	got := Signatures(img, StandardDPP, nil)
	if len(got) != 1 || got[0].Name != "uhfm_w" || got[0].Size != 2 || got[0].Addr != 0x380110 {
		t.Fatalf("got %v", got)
	}
	copy(img[0x4100:], img[0x4000:0x400E])
	if Signatures(img, StandardDPP, nil) != nil {
		t.Fatal("a second copy must drop uhfm_w")
	}
}

func TestRLFromPrior(t *testing.T) {
	img := readHex(t, "sign-rl.hex")
	got := Signatures(img, StandardDPP, map[string]uint32{"rl": 0x380100})
	if len(got) != 1 || got[0].Name != "rl_w" || got[0].Addr != 0x380120 || got[0].Size != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestPrefixDisasm(t *testing.T) {
	img := readHex(t, "sign-tabgm.hex")
	got := Signatures(img, StandardDPP, map[string]uint32{"tabgm": 0x380100})
	if len(got) != 1 || got[0].Name != "tabgm_w" || got[0].Addr != 0x380120 || got[0].Size != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestSuffixDisasm(t *testing.T) {
	img := readHex(t, "sign-pu.hex")
	got := Signatures(img, StandardDPP, map[string]uint32{"ps_w": 0x380100})
	if len(got) != 1 || got[0].Name != "pu" || got[0].Addr != 0x380120 || got[0].Size != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestVpssFromPssol(t *testing.T) {
	img := make([]byte, 0x4200)
	// pssol_w and pu_w are 0x380100, so each embed is F2 FX 00 81.
	// The F6FX inside the pattern is before instruction 0xC. The word after
	// four more instructions is the one that is stored.
	pat := []byte{
		0xF2, 0xF4, 0x00, 0x81,
		0xF2, 0xF4, 0x00, 0x81,
		0x5C, 0xE0,
		0xF2, 0xF4, 0x00, 0x81,
		0x7C, 0x20,
		0xF6, 0xF4, 0x0C, 0xFE,
		0xF6, 0xF4, 0x0E, 0xFE,
		0x7B, 0x00,
		// EXTP for a different page, then two instructions, then the word.
		// The page belongs to the next instruction, not the later F6FX.
		0xD7, 0x40, 0xE1, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xF6, 0xF4, 0x02, 0x81,
	}
	copy(img[0x4000:], pat)
	got := Signatures(img, StandardDPP, map[string]uint32{
		"pssol_w": 0x380100, "pu_w": 0x380100,
	})
	var hit *Named
	for i := range got {
		if got[i].Name == "vpsspu_w" {
			hit = &got[i]
		}
	}
	if hit == nil || hit.Addr != 0x380102 || hit.Size != 2 {
		t.Fatalf("%v", got)
	}
}

func TestZwmnPair(t *testing.T) {
	img := make([]byte, 0x21000)
	// Default bootrom table, slot 0x17 is DA 00 B8 78. zwmnms and zwopt are
	// both 0x380100, so the embed is F7F8 / F3FX and the bytes 00 81.
	const base = 0x20000
	img[base] = 0xDB
	copy(img[base+0x20:], []byte{
		0xE6, 0xFC, 0x11, 0x11, 0xE6, 0xFD, 0x22, 0x22,
		0xF2, 0xFE, 0x33, 0x33, 0xF2, 0xFF, 0x44, 0x44,
		0xDA, 0x00, 0xB8, 0x78, 0xF7, 0xF8, 0x00, 0x81,
	})
	// F3 FX xx xx F3 FX 00 81 21 xx, word at +2 is DPP2 0x0102 -> 0x380102.
	copy(img[base+0x40:], []byte{0xF3, 0xF0, 0x02, 0x81, 0xF3, 0xF0, 0x00, 0x81, 0x21, 0x00})
	img[base+0x80] = 0xDB
	// F3 FX xx xx 43 FX 02 81 DD xx. 0x380102 embeds as 43 FX 02 81.
	copy(img[base+0x90:], []byte{0xF3, 0xF0, 0x04, 0x81, 0x43, 0xF0, 0x02, 0x81, 0xDD, 0x00})
	known := map[string]uint32{"zwmnms": 0x380100, "zwopt": 0x380100}
	got := Signatures(img, StandardDPP, known)
	var spae, sol Named
	var n int
	for _, h := range got {
		switch h.Name {
		case "zwspae":
			spae = h
			n++
		case "zwsol":
			sol = h
			n++
		}
	}
	if n != 2 || spae.Addr != 0x380102 || spae.Size != 1 || sol.Addr != 0x380104 || sol.Size != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestPtrAtExtp(t *testing.T) {
	img := make([]byte, 0x20)
	// D7 40, page 0x00E0, then the word 0x0100 at offset 6.
	img[0] = 0xD7
	img[1] = 0x40
	binary.LittleEndian.PutUint16(img[2:], 0x00E0)
	binary.LittleEndian.PutUint16(img[6:], 0x0100)
	got := PtrAt(img, FlashBase+6, StandardDPP)
	if got != 0x380100 {
		t.Fatalf("got %#x", got)
	}
}
