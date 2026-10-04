// Package logger samples RAM over stock KWP2000 and writes a CSV log.
// The port is opened only after generate has produced a Connect value.
package logger

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"me7-logger/csvlog"
	"me7-logger/ecu"
	"me7-logger/generate"
	"me7-logger/kwp"
	"me7-logger/logcfg"
	"me7-logger/record"
)

// Options is one logging run.
type Options struct {
	Image     []byte
	ImageName string
	CorePath  string
	NamesPath string
	MeasPath  string
	MapPath   string
	AliasPath string
	Cfg       logcfg.File
	CfgPath   string
	MyECU     string
	SPS       int
	Baud      int
	One       bool
	Clock     int
	Scale     string
	// UserDir is an optional directory of overlay YAML. Empty means no overlay.
	UserDir string
}

// Run generates the definition, then dials the port and samples.
// dial is not called when Connect is empty or generate fails.
func Run(ctx context.Context, opt Options, dial func() (kwp.Port, error), w io.Writer) error {
	res, err := generate.Generate(generate.Options{
		Image: opt.Image, ImageName: opt.ImageName,
		CorePath: opt.CorePath, NamesPath: opt.NamesPath, MeasPath: opt.MeasPath,
		MapPath: opt.MapPath, AliasPath: opt.AliasPath,
		Clock: opt.Clock, Scale: opt.Scale, UserDir: opt.UserDir,
	})
	if err != nil {
		return err
	}
	if opt.MyECU != "" {
		if err := ecu.MergeMy(opt.MyECU, res.File); err != nil {
			return err
		}
	}
	if res.File.Connect == "" {
		return fmt.Errorf("ecu has no Connect value; set Connect in the .ecu or add the slow-init needle from config/names.yaml")
	}
	if strings.HasPrefix(res.File.Connect, "SLOW-0x00") {
		return fmt.Errorf("SLOW-0x00 is the engine-stopped KW1281 detour and is not implemented")
	}
	fast, addr, err := parseConnect(res.File.Connect)
	if err != nil {
		return err
	}
	if opt.Baud != 0 && opt.Baud != 10400 {
		return fmt.Errorf("logging baud %d is not implemented; the init baud is 10400", opt.Baud)
	}
	blocks, cols, err := plan(res.File.Items, opt.Cfg)
	if err != nil {
		return err
	}
	if dial == nil {
		return fmt.Errorf("no serial port")
	}
	port, err := dial()
	if err != nil {
		return err
	}
	defer port.Close()
	sess := &kwp.Session{Port: port, Src: kwp.Tester}
	var comp byte
	if fast {
		target := addr
		if res.File.Fast != 0 {
			target = res.File.Fast
		}
		if err := sess.FastInit(target); err != nil {
			return err
		}
		sess.Pace()
		if err := sess.StartSession(); err != nil {
			return err
		}
	} else {
		comp, err = sess.StartSlow(addr)
		if err != nil {
			return err
		}
	}
	_ = comp
	sps := opt.SPS
	if sps == 0 {
		sps = opt.Cfg.SamplesPerSecond
	}
	if sps < 1 || sps > 50 {
		return fmt.Errorf("samples per second %d wants 1..50", sps)
	}
	note := identNote(res.File)
	if res.File.LogSpeed != 0 && res.File.LogSpeed != 10400 {
		note += fmt.Sprintf("ECU LogSpeed %d is not switched; logging at 10400\n", res.File.LogSpeed)
	}
	ecuName := opt.Cfg.ECU
	if ecuName == "" {
		ecuName = opt.ImageName
	}
	if err := csvlog.WriteHeader(w, csvlog.Header{
		ECUFile: ecuName, SPS: sps, Baud: 10400, Mode: "HM0",
		Started: time.Now(), IdentNote: note,
	}, cols); err != nil {
		return err
	}
	vals := make([]float64, len(cols))
	scratch := make([]byte, 254)
	row := make([]byte, 0, 4096)
	start := time.Now()
	period := time.Second / time.Duration(sps)
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if err := sample(sess, blocks, vals, scratch); err != nil {
			return err
		}
		row = csvlog.AppendRow(row[:0], time.Since(start).Seconds(), vals)
		if _, err := w.Write(row); err != nil {
			return err
		}
		if opt.One {
			return nil
		}
		next := start.Add(time.Duration(i+1) * period)
		if err := waitUntil(ctx, next); err != nil {
			return nil
		}
	}
}

func identNote(f *ecu.File) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ECU identification requested:\n")
	fmt.Fprintf(&b, "HWNumber    = %s\n", f.ID.HWNumber)
	fmt.Fprintf(&b, "SWNumber    = %s\n", f.ID.SWNumber)
	fmt.Fprintf(&b, "PartNumber  = %s\n", strings.TrimSpace(f.ID.PartNumber))
	fmt.Fprintf(&b, "SWVersion   = %s\n", f.ID.SWVersion)
	fmt.Fprintf(&b, "EngineId    = %s\n", strings.TrimRight(f.ID.EngineID, " "))
	return b.String()
}

func parseConnect(s string) (fast bool, addr byte, err error) {
	switch {
	case strings.HasPrefix(s, "SLOW-0x"):
		addr, err = parseAddr(s[len("SLOW-0x"):])
		return false, addr, err
	case strings.HasPrefix(s, "FAST-0x"):
		addr, err = parseAddr(s[len("FAST-0x"):])
		return true, addr, err
	default:
		return false, 0, fmt.Errorf("connect %q", s)
	}
}

func parseAddr(s string) (byte, error) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " ;"); i >= 0 {
		s = s[:i]
	}
	v, err := strconv.ParseUint(s, 16, 8)
	if err != nil {
		return 0, fmt.Errorf("connect address %q", s)
	}
	return byte(v), nil
}

func waitUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Lateness is the worst sample lag against a fixed period.
func Lateness(stamps []time.Time, period time.Duration) time.Duration {
	if len(stamps) == 0 {
		return 0
	}
	var max time.Duration
	for i, ts := range stamps {
		late := ts.Sub(stamps[0]) - time.Duration(i)*period
		if late > max {
			max = late
		}
	}
	return max
}

type placed struct {
	out             int
	off             int
	size            int
	mask            uint16
	signed, inverse bool
	a, b            float64
}

type block struct {
	addr uint32
	size int
	vars []placed
}

func plan(items []record.Item, cfg logcfg.File) ([]block, []csvlog.Col, error) {
	byName := map[string]record.Item{}
	for _, it := range items {
		byName[it.Name] = it
	}
	type sel struct {
		it    record.Item
		alias string
		ord   int
	}
	var sels []sel
	for i, v := range cfg.Vars {
		it, ok := byName[v.Name]
		if !ok {
			return nil, nil, fmt.Errorf("variable %s is not in the ecu file", v.Name)
		}
		alias := it.Alias
		if v.Alias != "" {
			alias = v.Alias
		}
		sels = append(sels, sel{it: it, alias: alias, ord: i})
	}
	sort.SliceStable(sels, func(i, j int) bool { return sels[i].it.Addr < sels[j].it.Addr })
	var blocks []block
	cols := make([]csvlog.Col, len(cfg.Vars))
	for _, s := range sels {
		cols[s.ord] = csvlog.Col{Name: s.it.Name, Unit: s.it.Unit, Alias: s.alias}
		sz := s.it.Size
		if sz == 0 {
			sz = 1
		}
		if len(blocks) > 0 {
			bl := &blocks[len(blocks)-1]
			end := bl.addr + uint32(bl.size)
			newEnd := s.it.Addr + uint32(sz)
			if s.it.Addr <= end && newEnd-bl.addr <= 254 {
				bl.size = int(newEnd - bl.addr)
				bl.vars = append(bl.vars, placed{
					out: s.ord, off: int(s.it.Addr - bl.addr), size: sz,
					mask: s.it.Bitmask, signed: s.it.Signed, inverse: s.it.Inverse,
					a: s.it.A, b: s.it.B,
				})
				continue
			}
		}
		blocks = append(blocks, block{
			addr: s.it.Addr,
			size: sz,
			vars: []placed{{
				out: s.ord, off: 0, size: sz,
				mask: s.it.Bitmask, signed: s.it.Signed, inverse: s.it.Inverse,
				a: s.it.A, b: s.it.B,
			}},
		})
	}
	return blocks, cols, nil
}

// sample reads every block into scratch and fills dst. It does not allocate.
func sample(s *kwp.Session, blocks []block, dst []float64, scratch []byte) error {
	for i, bl := range blocks {
		if i > 0 {
			s.Pace()
		}
		if len(scratch) < bl.size {
			return fmt.Errorf("scratch")
		}
		if err := s.ReadMemory(bl.addr, bl.size, scratch); err != nil {
			return err
		}
		for _, v := range bl.vars {
			raw := kwp.Word(scratch, v.off, v.size)
			dst[v.out] = csvlog.Convert(raw, v.size, v.mask, v.signed, v.inverse, v.a, v.b)
		}
	}
	return nil
}
