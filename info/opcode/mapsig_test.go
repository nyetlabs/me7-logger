package opcode

import "testing"

func TestF2WordOdd(t *testing.T) {
	img := []byte{0xF2, 0xF4, 0x00, 0x81, 0xF2, 0xF4, 0x01, 0x81}
	at := 2
	if got := f2Word(img, 0, &at); got != 0x8100 {
		t.Fatalf("even %#x", got)
	}
	at = 2
	if got := f2Word(img, 4, &at); got != 0 {
		t.Fatalf("odd operand %#x", got)
	}
	at = 1
	if got := f2Word(img, 0, &at); got != 0 {
		t.Fatal("odd offset")
	}
}
