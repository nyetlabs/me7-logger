package kwp

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

// Open opens a K-line serial port. DTR is asserted and RTS is cleared, which
// is the usual idle state for a dumb KKL cable. FTDI latency-timer control is
// not used; the virtual COM port is the interface.
func Open(name string) (Port, error) {
	p, err := serial.Open(name, &serial.Mode{BaudRate: initBaud})
	if err != nil {
		return nil, err
	}
	op := &osPort{p: p}
	if err := op.SetDTR(true); err != nil {
		_ = p.Close()
		return nil, err
	}
	if err := op.SetRTS(false); err != nil {
		_ = p.Close()
		return nil, err
	}
	return op, nil
}

type osPort struct {
	p serial.Port
}

func (o *osPort) SetBaud(baud int) error {
	return o.p.SetMode(&serial.Mode{BaudRate: baud})
}

func (o *osPort) Break(d time.Duration) error {
	return o.p.Break(d)
}

func (o *osPort) SetDTR(v bool) error { return o.p.SetDTR(v) }
func (o *osPort) SetRTS(v bool) error { return o.p.SetRTS(v) }

func (o *osPort) SetReadTimeout(d time.Duration) error {
	return o.p.SetReadTimeout(d)
}

func (o *osPort) Write(b []byte) (int, error) { return o.p.Write(b) }

func (o *osPort) Close() error { return o.p.Close() }

func (o *osPort) Read(b []byte) (int, error) {
	n, err := o.p.Read(b)
	if n == 0 && err == nil {
		return 0, ErrTimeout
	}
	if err != nil {
		return n, fmt.Errorf("kwp: %w", err)
	}
	return n, nil
}
