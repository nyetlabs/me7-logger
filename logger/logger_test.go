package logger

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.nyet.org/me7-logger/csvlog"
	"go.nyet.org/me7-logger/kwp"
	"go.nyet.org/me7-logger/logcfg"
	"go.nyet.org/me7-logger/record"
)

func TestPlanMergesNeighbors(t *testing.T) {
	items := []record.Item{
		{Name: "a", Addr: 0x100, Size: 2, A: 1},
		{Name: "b", Addr: 0x102, Size: 2, A: 1},
		{Name: "c", Addr: 0x110, Size: 1, A: 1, Unit: "rpm"},
	}
	cfg := logcfg.File{Vars: []logcfg.Var{{Name: "c"}, {Name: "a"}, {Name: "b", Alias: "Bee"}}}
	blocks, cols, err := plan(items, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].size != 4 || blocks[1].addr != 0x110 {
		t.Fatalf("%+v", blocks)
	}
	if cols[0].Name != "c" || cols[2].Alias != "Bee" {
		t.Fatalf("%+v", cols)
	}
}

func TestSampleNoAlloc(t *testing.T) {
	resp := []byte{0x63, 0x2C, 0x01}
	frame := make([]byte, 16)
	n, err := kwp.Encode(frame, kwp.Tester, 0x11, resp)
	if err != nil {
		t.Fatal(err)
	}
	p := &memPort{fn: n}
	copy(p.frame[:], frame[:n])
	s := &kwp.Session{Port: p, Tgt: 0x11, Src: kwp.Tester, Sleep: func(time.Duration) {}}
	blocks := []block{{
		addr: 0x100, size: 2,
		vars: []placed{{out: 0, size: 2, a: 10, b: 0}},
	}}
	dst := []float64{0}
	scratch := make([]byte, 8)
	allocs := testing.AllocsPerRun(40, func() {
		p.off = 0
		if err := sample(s, blocks, dst, scratch); err != nil {
			t.Fatalf("sample: %v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs %v", allocs)
	}
	if dst[0] != 3000 {
		t.Fatalf("val %v", dst[0])
	}
}

func TestLogDoesNotDialWithoutConnect(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, "core.yaml")
	if err := os.WriteFile(yml, []byte("functions:\n  - name: nowhere\n    needle_hex: \"AA BB\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	err := Run(context.Background(), Options{
		Image:    []byte{0, 0, 0, 0},
		CorePath: yml,
		MapPath:  filepath.Join(dir, "missing.map"),
		Cfg:      logcfg.File{SamplesPerSecond: 10},
	}, func() (kwp.Port, error) {
		called = true
		return nil, os.ErrNotExist
	}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "Connect") {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dialed without Connect")
	}
}

func TestLogDialsAfterConnect(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, "core.yaml")
	body := "functions:\n  - name: slow_init_table\n    needle_hex: \"11 55 EF 8F\"\n"
	if err := os.WriteFile(yml, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 30)
	img[0], img[1], img[2], img[3] = 0x11, 0x55, 0xEF, 0x8F
	called := false
	err := Run(context.Background(), Options{
		Image: img, CorePath: yml, MapPath: filepath.Join(dir, "missing.map"),
		Cfg: logcfg.File{SamplesPerSecond: 10},
	}, func() (kwp.Port, error) {
		called = true
		return nil, os.ErrNotExist
	}, ioDiscard{})
	if !called {
		t.Fatal("expected dial after Connect was chosen")
	}
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestBenchSpikeSkippedWithoutHardware(t *testing.T) {
	if os.Getenv("ME7_BENCH") == "" {
		t.Skip("bench spike needs ME7_BENCH=1 and a K-line port on an M box using SLOW-0x11")
	}
	t.Fatal("ME7_BENCH is set; run the log command against the car and record sample rate and jitter")
}

func TestLateness(t *testing.T) {
	start := time.Unix(0, 0)
	period := 100 * time.Millisecond
	stamps := []time.Time{start, start.Add(100 * time.Millisecond), start.Add(250 * time.Millisecond)}
	if got := Lateness(stamps, period); got != 50*time.Millisecond {
		t.Fatalf("%s", got)
	}
}

func TestCSVShape(t *testing.T) {
	var b bytes.Buffer
	err := csvlog.WriteHeader(&b, csvlog.Header{ECUFile: "m.ecu", SPS: 10, Baud: 10400}, []csvlog.Col{
		{Name: "nmot", Unit: "rpm", Alias: "EngineSpeed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	row := csvlog.AppendRow(nil, 0, []float64{800})
	b.Write(row)
	text := b.String()
	if !strings.Contains(text, "ME7-Logger") || !strings.Contains(text, "TimeStamp") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "sec.ms") || !strings.Contains(text, "EngineSpeed") {
		t.Fatal(text)
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
func (p *memPort) Write([]byte) (int, error)          { p.off = 0; return 1, nil }
func (p *memPort) Close() error                       { return nil }
func (p *memPort) Read(b []byte) (int, error) {
	if p.off >= p.fn {
		return 0, kwp.ErrTimeout
	}
	n := copy(b, p.frame[p.off:p.fn])
	p.off += n
	return n, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
