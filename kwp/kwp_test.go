package kwp

import (
	"bytes"
	"testing"
	"time"
)

func TestEncodeKnownFrames(t *testing.T) {
	var dst [16]byte
	n, err := Encode(dst[:], 0x10, Tester, []byte{0x81})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dst[:n], []byte{0x81, 0x10, 0xF1, 0x81, 0x03}) {
		t.Fatalf("% X", dst[:n])
	}
	n, err = Encode(dst[:], 0x01, Tester, []byte{0x81})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dst[:n], []byte{0x81, 0x01, 0xF1, 0x81, 0xF4}) {
		t.Fatalf("% X", dst[:n])
	}
	tgt, src, pay, err := Decode(dst[:n])
	if err != nil || tgt != 0x01 || src != Tester || len(pay) != 1 || pay[0] != 0x81 {
		t.Fatalf("decode %02X %02X %X %v", tgt, src, pay, err)
	}
}

func TestSlowAddressBits(t *testing.T) {
	p := &bitPort{}
	if err := SendSlowAddress(p, 0x11, p.mark); err != nil {
		t.Fatal(err)
	}
	// start 0, then 0x11 LSB first (1,0,0,0,1,0,0,0), stop 1. Bit 7 is 0.
	want := []bool{false, true, false, false, false, true, false, false, false, true}
	if len(p.bits) != len(want) {
		t.Fatalf("bits %v", p.bits)
	}
	for i := range want {
		if p.bits[i] != want[i] {
			t.Fatalf("bit %d got %v want %v (%v)", i, p.bits[i], want[i], p.bits)
		}
	}
}

type bitPort struct {
	bits []bool
}

func (p *bitPort) mark(time.Duration) { p.bits = append(p.bits, true) }
func (p *bitPort) Break(time.Duration) error {
	p.bits = append(p.bits, false)
	return nil
}
func (p *bitPort) SetBaud(int) error                  { return nil }
func (p *bitPort) SetDTR(bool) error                  { return nil }
func (p *bitPort) SetRTS(bool) error                  { return nil }
func (p *bitPort) SetReadTimeout(time.Duration) error { return nil }
func (p *bitPort) Write([]byte) (int, error)          { return 0, nil }
func (p *bitPort) Read([]byte) (int, error)           { return 0, ErrTimeout }
func (p *bitPort) Close() error                       { return nil }

func TestReadMemoryNoAlloc(t *testing.T) {
	respPay := []byte{sidReadMemPos, 0x34, 0x12}
	frame := make([]byte, 16)
	n, err := Encode(frame, Tester, 0x11, respPay)
	if err != nil {
		t.Fatal(err)
	}
	p := &memPort{fn: n}
	copy(p.frame[:], frame[:n])
	s := &Session{Port: p, Tgt: 0x11, Src: Tester, Sleep: func(time.Duration) {}}
	dst := make([]byte, 2)
	var got uint16
	allocs := testing.AllocsPerRun(50, func() {
		p.off = 0
		if err := s.ReadMemory(0x380100, 2, dst); err != nil {
			t.Fatalf("read: %v", err)
		}
		got = Word(dst, 0, 2)
	})
	if allocs != 0 {
		t.Fatalf("allocs %v", allocs)
	}
	if got != 0x1234 {
		t.Fatalf("word %x", got)
	}
}

type memPort struct {
	frame [32]byte
	fn    int
	off   int
}

func (p *memPort) SetBaud(int) error                  { return nil }
func (p *memPort) Break(time.Duration) error          { return nil }
func (p *memPort) SetDTR(bool) error                  { return nil }
func (p *memPort) SetRTS(bool) error                  { return nil }
func (p *memPort) SetReadTimeout(time.Duration) error { return nil }
func (p *memPort) Write(b []byte) (int, error) {
	p.off = 0
	return len(b), nil
}
func (p *memPort) Close() error { return nil }
func (p *memPort) Read(b []byte) (int, error) {
	if p.off >= p.fn {
		return 0, ErrTimeout
	}
	n := copy(b, p.frame[p.off:p.fn])
	p.off += n
	return n, nil
}

func TestDefineLocalFrame(t *testing.T) {
	var dst [8]byte
	n := DefineByMemoryAddress(dst[:], 0xF0, 0x380100, 2)
	if !bytes.Equal(dst[:n], []byte{sidDefineLocal, DefineByAddress, 0xF0, 0x38, 0x01, 0x00, 2}) {
		t.Fatalf("% X", dst[:n])
	}
	if ReadLocal(dst[:], 0xF0) != 2 || dst[0] != sidReadLocal {
		t.Fatal(dst[:2])
	}
}
