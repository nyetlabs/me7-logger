// Package kwp is a clean-room ISO 14230 / KWP2000 client for stock services.
// It reads RAM. It does not implement security access, programming download,
// or a development-session RAM handler.
package kwp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const (
	// Tester is the tester source address.
	Tester = 0xF1

	sidStartComm    = 0x81
	sidStartSession = 0x10
	// SessionDefault is the stock diagnostic session. Programming (0x85) and
	// development (0x86) are not requested.
	SessionDefault = 0x81
	sidReadMemory  = 0x23
	sidReadMemPos  = 0x63
	sidDefineLocal = 0x2C
	sidReadLocal   = 0x21
	sidNeg         = 0x7F

	// DefineByAddress is ISO 14230-3 definitionMode for a memory address.
	DefineByAddress = 0x02

	p2      = 50 * time.Millisecond
	p3      = 55 * time.Millisecond
	p4      = 5 * time.Millisecond
	w1      = 450 * time.Millisecond
	w4      = 25 * time.Millisecond
	bitTime = 200 * time.Millisecond

	initBaud = 10400
)

// ErrTimeout is a read that produced no byte inside the KWP window.
var ErrTimeout = errors.New("kwp: timeout")

// Port is the OS serial line. Break holds space for the duration, then returns
// the line to mark. Implementations must not be required for tests.
type Port interface {
	SetBaud(int) error
	Break(time.Duration) error
	SetDTR(bool) error
	SetRTS(bool) error
	SetReadTimeout(time.Duration) error
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}

// Session is one K-line conversation. Buffers are reused so the read loop does
// not allocate.
type Session struct {
	Port Port
	Tgt  byte
	Src  byte
	tx   [280]byte
	rx   [280]byte
	// Sleep replaces time.Sleep. Tests pass a no-op.
	Sleep func(time.Duration)
	// Now replaces time.Now for the read deadline.
	Now func() time.Time
}

func (s *Session) sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (s *Session) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// SendSlowAddress bit-bangs a 5-baud address as 8N1 with bit 7 clear.
func SendSlowAddress(p Port, addr byte, sleep func(time.Duration)) error {
	if sleep == nil {
		sleep = time.Sleep
	}
	addr &^= 0x80
	if err := p.Break(bitTime); err != nil {
		return err
	}
	for i := 0; i < 8; i++ {
		if addr&(1<<uint(i)) != 0 {
			sleep(bitTime)
			continue
		}
		if err := p.Break(bitTime); err != nil {
			return err
		}
	}
	sleep(bitTime)
	return nil
}

// SlowInit performs 5-baud init and the keyword exchange at 10400 baud.
// The address complement is returned. 0xEE means the session came in on 0x11.
func (s *Session) SlowInit(addr byte) (byte, byte, byte, error) {
	if err := s.Port.SetBaud(initBaud); err != nil {
		return 0, 0, 0, err
	}
	if err := SendSlowAddress(s.Port, addr, s.sleep); err != nil {
		return 0, 0, 0, err
	}
	if err := s.Port.SetBaud(initBaud); err != nil {
		return 0, 0, 0, err
	}
	sync, err := s.readByte(w1)
	if err != nil {
		return 0, 0, 0, err
	}
	if sync != 0x55 {
		return 0, 0, 0, fmt.Errorf("kwp: sync 0x%02X", sync)
	}
	kb1, err := s.readByte(20 * time.Millisecond)
	if err != nil {
		return 0, 0, 0, err
	}
	kb2, err := s.readByte(20 * time.Millisecond)
	if err != nil {
		return 0, 0, 0, err
	}
	s.sleep(w4)
	inv := ^kb2
	if _, err := s.Port.Write([]byte{inv}); err != nil {
		return 0, 0, 0, err
	}
	comp, err := s.readByte(100 * time.Millisecond)
	if err != nil {
		return 0, 0, 0, err
	}
	return kb1, kb2, comp, nil
}

// FastInit sends the 25 ms wake-up and StartCommunication to target.
func (s *Session) FastInit(target byte) error {
	s.Src = Tester
	s.Tgt = target
	if err := s.Port.Break(25 * time.Millisecond); err != nil {
		return err
	}
	s.sleep(25 * time.Millisecond)
	if err := s.Port.SetBaud(initBaud); err != nil {
		return err
	}
	return s.startComm()
}

// StartSlow finishes a slow init with StartCommunication and the default session.
func (s *Session) StartSlow(addr byte) (comp byte, err error) {
	s.Src = Tester
	s.Tgt = addr &^ 0x80
	_, _, comp, err = s.SlowInit(addr)
	if err != nil {
		return 0, err
	}
	s.sleep(p3)
	if err := s.startComm(); err != nil {
		return comp, err
	}
	s.sleep(p3)
	return comp, s.startSession(SessionDefault)
}

func (s *Session) startComm() error {
	n := putPayload(s.tx[:], sidStartComm)
	pay, err := s.exchange(n)
	if err != nil {
		return err
	}
	if len(pay) < 1 || pay[0] != sidStartComm+0x40 {
		return fmt.Errorf("kwp: start communication response")
	}
	return nil
}

// StartSession requests the stock diagnostic session. Programming and
// development sessions are not used.
func (s *Session) StartSession() error {
	return s.startSession(SessionDefault)
}

// Pace waits the minimum gap between a response and the next request.
func (s *Session) Pace() { s.sleep(p3) }

func (s *Session) startSession(kind byte) error {
	s.tx[0] = sidStartSession
	s.tx[1] = kind
	pay, err := s.exchange(2)
	if err != nil {
		return err
	}
	if len(pay) < 1 || pay[0] != sidStartSession+0x40 {
		return fmt.Errorf("kwp: start session response")
	}
	return nil
}

// ReadMemory reads n bytes at addr into dst. dst's length must be at least n.
// The success path does not allocate.
func (s *Session) ReadMemory(addr uint32, n int, dst []byte) error {
	if n <= 0 || n > 254 {
		return fmt.Errorf("kwp: read length %d", n)
	}
	if len(dst) < n {
		return fmt.Errorf("kwp: dst short")
	}
	s.tx[0] = sidReadMemory
	s.tx[1] = byte(addr >> 16)
	s.tx[2] = byte(addr >> 8)
	s.tx[3] = byte(addr)
	s.tx[4] = byte(n)
	pay, err := s.exchange(5)
	if err != nil {
		return err
	}
	if len(pay) < 1+n || pay[0] != sidReadMemPos {
		return fmt.Errorf("kwp: read memory response")
	}
	copy(dst[:n], pay[1:1+n])
	return nil
}

// DefineByMemoryAddress builds a 0x2C defineByMemoryAddress payload into dst.
// The logger samples with ReadMemory. This frame is the other stock service.
func DefineByMemoryAddress(dst []byte, lid byte, addr uint32, n int) int {
	dst[0] = sidDefineLocal
	dst[1] = DefineByAddress
	dst[2] = lid
	dst[3] = byte(addr >> 16)
	dst[4] = byte(addr >> 8)
	dst[5] = byte(addr)
	dst[6] = byte(n)
	return 7
}

// ReadLocal builds a readDataByLocalIdentifier payload.
func ReadLocal(dst []byte, lid byte) int {
	dst[0] = sidReadLocal
	dst[1] = lid
	return 2
}

func putPayload(dst []byte, b byte) int {
	dst[0] = b
	return 1
}

// exchange sends payload bytes already in s.tx[:n] and returns the response
// payload as a subslice of s.rx. The subslice is valid until the next exchange.
func (s *Session) exchange(n int) ([]byte, error) {
	fn, err := Encode(s.rx[:], s.Tgt, s.Src, s.tx[:n])
	if err != nil {
		return nil, err
	}
	// Encode wrote the frame into s.rx. Move it to s.tx without allocating.
	copy(s.tx[:fn], s.rx[:fn])
	if err := s.writeSpaced(s.tx[:fn]); err != nil {
		return nil, err
	}
	rn, err := s.readFrame(p2)
	if err != nil {
		return nil, err
	}
	_, _, pay, err := Decode(s.rx[:rn])
	if err != nil {
		return nil, err
	}
	if len(pay) >= 3 && pay[0] == sidNeg {
		return nil, fmt.Errorf("kwp: negative response to 0x%02X code 0x%02X", pay[1], pay[2])
	}
	return pay, nil
}

func (s *Session) writeSpaced(frame []byte) error {
	if len(frame) == 0 {
		return nil
	}
	if _, err := s.Port.Write(frame[:1]); err != nil {
		return err
	}
	for i := 1; i < len(frame); i++ {
		s.sleep(p4)
		if _, err := s.Port.Write(frame[i : i+1]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) readByte(window time.Duration) (byte, error) {
	var b [1]byte
	deadline := s.now().Add(window)
	for {
		if err := s.Port.SetReadTimeout(window); err != nil {
			return 0, err
		}
		n, err := s.Port.Read(b[:])
		if n == 1 {
			return b[0], nil
		}
		if err != nil && !errors.Is(err, ErrTimeout) {
			return 0, err
		}
		if !s.now().Before(deadline) {
			return 0, ErrTimeout
		}
	}
}

func (s *Session) readFrame(window time.Duration) (int, error) {
	deadline := s.now().Add(window)
	got := 0
	for {
		need, ok := frameSize(s.rx[:got])
		if ok && got >= need {
			return need, nil
		}
		if !s.now().Before(deadline) {
			return 0, ErrTimeout
		}
		left := deadline.Sub(s.now())
		if err := s.Port.SetReadTimeout(left); err != nil {
			return 0, err
		}
		n, err := s.Port.Read(s.rx[got:])
		got += n
		if got >= len(s.rx) {
			return 0, fmt.Errorf("kwp: frame too long")
		}
		if n == 0 {
			if err != nil && !errors.Is(err, ErrTimeout) {
				return 0, err
			}
			if !s.now().Before(deadline) {
				return 0, ErrTimeout
			}
		}
	}
}

func frameSize(b []byte) (int, bool) {
	if len(b) < 1 {
		return 0, false
	}
	ln := int(b[0] & 0x3F)
	header := 3
	if ln == 0 {
		if len(b) < 4 {
			return 0, false
		}
		ln = int(b[3])
		header = 4
	}
	if (b[0] & 0xC0) == 0 {
		header = 1
	}
	return header + ln + 1, true
}

// Encode writes a physical-address KWP frame into dst and returns its length.
func Encode(dst []byte, tgt, src byte, payload []byte) (int, error) {
	if len(payload) > 255 {
		return 0, fmt.Errorf("kwp: payload %d", len(payload))
	}
	var n int
	if len(payload) > 0 && len(payload) <= 63 {
		if len(dst) < 4+len(payload) {
			return 0, fmt.Errorf("kwp: dst short")
		}
		dst[0] = 0x80 | byte(len(payload))
		dst[1] = tgt
		dst[2] = src
		copy(dst[3:], payload)
		n = 3 + len(payload)
	} else {
		if len(dst) < 5+len(payload) {
			return 0, fmt.Errorf("kwp: dst short")
		}
		dst[0] = 0x80
		dst[1] = tgt
		dst[2] = src
		dst[3] = byte(len(payload))
		copy(dst[4:], payload)
		n = 4 + len(payload)
	}
	dst[n] = checksum(dst[:n])
	return n + 1, nil
}

// Decode checks a frame and returns target, source, and payload.
// Payload aliases frame.
func Decode(frame []byte) (tgt, src byte, payload []byte, err error) {
	need, ok := frameSize(frame)
	if !ok || len(frame) < need || need < 4 {
		return 0, 0, nil, fmt.Errorf("kwp: short frame")
	}
	frame = frame[:need]
	if checksum(frame[:need-1]) != frame[need-1] {
		return 0, 0, nil, fmt.Errorf("kwp: bad checksum")
	}
	ln := int(frame[0] & 0x3F)
	header := 3
	if ln == 0 {
		ln = int(frame[3])
		header = 4
	}
	if (frame[0] & 0xC0) == 0 {
		return 0, 0, frame[1 : need-1], nil
	}
	return frame[1], frame[2], frame[header : header+ln], nil
}

func checksum(b []byte) byte {
	var s byte
	for _, c := range b {
		s += c
	}
	return s
}

// Word reads a little-endian value from a RAM image.
func Word(b []byte, off, size int) uint16 {
	if off < 0 || off+size > len(b) {
		return 0
	}
	if size == 1 {
		return uint16(b[off])
	}
	return binary.LittleEndian.Uint16(b[off : off+2])
}
