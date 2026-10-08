package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/generate"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/parity"
	"go.nyet.org/me7-logger/record"
	"go.nyet.org/me7-logger/xdf"
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
		fmt.Fprintf(os.Stderr, "me7info: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7info %s

  me7info generate [flags] image.bin
  me7info probe [flags] image.bin
  me7info parity [-data testdata/parity]
  me7info version

generate writes an .ecu file and, when calibration maps were located, a TunerPro XDF.
It does not open a serial port. probe reports the DPP block and needle hits.
parity scores each image. Legacy ME7Info parity is the only hard mark, one image at a time.
The catalog is coverage. The measurement list is the extras column on each ECU row.
The S4wiki name list is the same on every image. A hit is one address and an axis. An axis count of 0 is a scalar, so one address is the hit. When that image's XDF contains the name, the body address must match one row. The axis denominator is the axis count on that list for the maps that hit. A count of 0 adds nothing. That denominator is 0 only when every map that hit has a count of 0. One axis is a curve and two axes are a map. A hit is that axis present on the map. The confidence column is the body bytes of the names that hit. Its denominator is that matched set, not the tuner list. An XDF file is an address oracle for that image when one is present. Its x and y axes that have an address are the axis column of that section.
Needle names, connect bytes, and per-part addresses are the YAML files in config/.
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
	return writeXDF(*xdfPath, imgPath, len(img), res.Maps)
}

func writeXDF(path, imgPath string, size int, maps []record.Map) error {
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
		return xdf.Write(os.Stdout, title, size, maps)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := xdf.Write(f, title, size, maps); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}

func cmdParity(args []string) error {
	fs := flag.NewFlagSet("parity", flag.ContinueOnError)
	dir := fs.String("data", "testdata/parity", "parity root: bin, ecu/me7info, and xdf")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("parity takes no image; the parity root holds it")
	}
	rep, err := parity.Run(*dir)
	if err != nil {
		return err
	}
	fmt.Print(rep.Text())
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
