package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"me7-logger/config"
	"me7-logger/generate"
	"me7-logger/kwp"
	"me7-logger/logcfg"
	"me7-logger/logger"
	"me7-logger/opcode"
	"me7-logger/parity"
	"me7-logger/record"
	"me7-logger/xdf"
)

// version is set from git describe by the Makefile. Do not edit it here.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = cmdGenerate(os.Args[2:])
	case "log":
		err = cmdLog(os.Args[2:])
	case "probe":
		err = cmdProbe(os.Args[2:])
	case "parity":
		err = cmdParity(os.Args[2:])
	case "version", "-version", "--version":
		fmt.Println(version)
	case "-h", "-help", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "me7-logger: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7-logger %s

  me7-logger generate [flags] image.bin
  me7-logger log [flags] image.bin config.cfg
  me7-logger probe [flags] image.bin
  me7-logger parity [-data testdata/parity]
  me7-logger version

generate writes an .ecu file and, when calibration maps were located, a TunerPro XDF.
It does not open a serial port. log runs
generate, then samples RAM over stock KWP2000. Needle names, connect bytes,
and per-part addresses are the YAML files in config/.
parity prints the percent of testdata/parity matched by .ecu and .xdf output.
`, version)
}

func cmdGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	core := fs.String("core", config.Path("ME7_CORE", config.NeedlesFile), "needle yaml path")
	names := fs.String("names", config.Path("ME7_NAMES", config.NamesFile), "ME7 name yaml path")
	meas := fs.String("meas", config.Path("ME7_MEAS", config.MeasuresFile), "per-part measurement yaml path")
	mapPath := fs.String("map", config.Path("ME7_MAP", config.MapDir), "result-type catalog")
	alias := fs.String("alias", config.Path("ME7_ALIAS", config.AliasFile), "alias file")
	out := fs.String("o", "", "ecu output path (default <image>.ecu)")
	xdfPath := fs.String("xdf", "", "xdf output path (default <image>.xdf when maps were located)")
	scale := fs.String("5120", "auto", "mbar scaling: auto, on, or off")
	clock := fs.Int("clock", 0, "CPU clock MHz: 20, 24, 32, or 40; 0 uses config/names.yaml")
	conn := fs.String("connect", "", "override Connect, for example SLOW-0x11")
	user := fs.String("user", config.Path("ME7_USER", "user"), "directory of user needles, measurements, and conversions")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("generate wants one flash image")
	}
	imgPath := fs.Arg(0)
	img, err := os.ReadFile(imgPath)
	if err != nil {
		return err
	}
	userDir, err := resolveUser(fs, *user)
	if err != nil {
		return err
	}
	res, err := generate.Generate(generate.Options{
		Image: img, ImageName: filepath.Base(imgPath),
		CorePath: *core, NamesPath: *names, MeasPath: *meas,
		MapPath: *mapPath, AliasPath: *alias,
		Clock: *clock, Scale: *scale, Connect: *conn, UserDir: userDir,
	})
	if err != nil {
		return err
	}
	ecuPath := *out
	if ecuPath == "" {
		ecuPath = strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + ".ecu"
	}
	if ecuPath == "-" {
		if _, err := os.Stdout.Write(res.File.Bytes()); err != nil {
			return err
		}
	} else if err := generate.WriteECU(ecuPath, res.File); err != nil {
		return err
	}
	if res.ScaleNote != "" {
		fmt.Fprintln(os.Stderr, res.ScaleNote)
	}
	if res.MapNote != "" {
		fmt.Fprintln(os.Stderr, res.MapNote)
	}
	if res.StubNote != "" {
		fmt.Fprintln(os.Stderr, res.StubNote)
	}
	if res.File.Connect == "" {
		fmt.Fprintln(os.Stderr, "Connect was not set; add the slow-init needle named in config/names.yaml")
	}
	return writeXDF(*xdfPath, imgPath, res.Maps)
}

func writeXDF(path, imgPath string, maps []record.Map) error {
	n := 0
	for _, m := range maps {
		if m.Name != "" {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	if path == "" {
		path = strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + ".xdf"
	}
	title := filepath.Base(imgPath)
	if path == "-" {
		return xdf.Write(os.Stdout, title, maps)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := xdf.Write(f, title, maps); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}

func cmdLog(args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	core := fs.String("core", config.Path("ME7_CORE", config.NeedlesFile), "needle yaml path")
	names := fs.String("names", config.Path("ME7_NAMES", config.NamesFile), "ME7 name yaml path")
	meas := fs.String("meas", config.Path("ME7_MEAS", config.MeasuresFile), "per-part measurement yaml path")
	mapPath := fs.String("map", config.Path("ME7_MAP", config.MapDir), "result-type catalog")
	alias := fs.String("alias", config.Path("ME7_ALIAS", config.AliasFile), "alias file")
	port := fs.String("p", "", "serial port")
	sps := fs.Int("s", 0, "samples per second, overrides the cfg")
	baud := fs.Int("b", 0, "baud override; only 10400 is implemented")
	out := fs.String("o", "", "csv path (default stdout)")
	one := fs.Bool("1", false, "read one sample and stop")
	scale := fs.String("5120", "auto", "mbar scaling: auto, on, or off")
	clock := fs.Int("clock", 0, "CPU clock MHz; 0 uses config/names.yaml")
	user := fs.String("user", config.Path("ME7_USER", "user"), "directory of user needles, measurements, and conversions")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("log wants a flash image and a cfg file")
	}
	if *port == "" {
		return fmt.Errorf("log wants -p <serial port>")
	}
	img, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	cfgPath := fs.Arg(1)
	cfg, err := logcfg.Load(cfgPath)
	if err != nil {
		return err
	}
	myECU := cfg.ECU
	if myECU != "" && !filepath.IsAbs(myECU) {
		myECU = filepath.Join(filepath.Dir(cfgPath), myECU)
	}
	var w io.Writer = os.Stdout
	if *out != "" && *out != "-" {
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	userDir, err := resolveUser(fs, *user)
	if err != nil {
		return err
	}
	name := *port
	return logger.Run(ctx, logger.Options{
		Image: img, ImageName: filepath.Base(fs.Arg(0)),
		CorePath: *core, NamesPath: *names, MeasPath: *meas,
		MapPath: *mapPath, AliasPath: *alias,
		Cfg: cfg, MyECU: myECU, SPS: *sps, Baud: *baud, One: *one,
		Clock: *clock, Scale: *scale, UserDir: userDir,
	}, func() (kwp.Port, error) {
		return kwp.Open(name)
	}, w)
}

func cmdParity(args []string) error {
	fs := flag.NewFlagSet("parity", flag.ContinueOnError)
	dir := fs.String("data", "testdata/parity", "directory of image, .ecu oracle, and .xdf oracle")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("parity takes no image; the oracle directory holds it")
	}
	rep, err := parity.Run(*dir)
	if err != nil {
		return err
	}
	fmt.Printf(".ecu %s\n.xdf %s\n", rep.ECU, rep.XDF)
	return nil
}

func cmdProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	core := fs.String("core", config.Path("ME7_CORE", config.NeedlesFile), "needle yaml path")
	user := fs.String("user", config.Path("ME7_USER", "user"), "directory of user needles, measurements, and conversions")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("probe wants one flash image")
	}
	img, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	userDir, err := resolveUser(fs, *user)
	if err != nil {
		return err
	}
	ns, err := config.LoadNeedles(*core, userDir)
	if err != nil {
		return err
	}
	if dpp, offs, ok := opcode.FindDPP(img); ok {
		fmt.Printf("dpp: DPP0=0x%04X DPP1=0x%04X DPP2=0x%04X DPP3=0x%04X (file+%s)\n",
			dpp[0], dpp[1], dpp[2], dpp[3], fmtOffs(offs))
	} else {
		fmt.Println("dpp: no full DPP0-3 init block found")
	}
	for _, n := range ns {
		for _, ln := range n.Report(img) {
			fmt.Println(ln)
		}
	}
	return nil
}

func resolveUser(fs *flag.FlagSet, dir string) (string, error) {
	explicit := os.Getenv("ME7_USER") != ""
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "user" {
			explicit = true
		}
	})
	return config.ResolveUserDir(dir, explicit)
}

func fmtOffs(offs []int) string {
	parts := make([]string, len(offs))
	for i, o := range offs {
		parts[i] = fmt.Sprintf("0x%X", o)
	}
	return strings.Join(parts, " ")
}
