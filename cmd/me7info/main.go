package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/generate"
	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
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
	case "-v", "--version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "me7info: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7info %s

Usage:
  me7info generate [flags] <image.bin>
  me7info probe [flags] <image.bin>
  me7info parity [flags]
  me7info -v, --version
  me7info -h, --help

Commands:
  generate  Write an .ecu file for the image and, when calibration maps
            were located, a TunerPro XDF. Does not open a serial port.
  probe     Report the DPP block and needle hits. --maps also counts the
            calibration maps generate would write to the XDF.
  parity    Score each corpus image against legacy ME7Info output.

Run "me7info <command> -h" for that command's flags.

Needle names, connect bytes, and per-part addresses are the YAML files
in config/.
`, version)
}

const parityHelp = `
Legacy ME7Info parity is the only hard mark, scored one image at a time.
The catalog column is coverage. The measurement list is the extras
column on each ECU row.

The S4wiki name list is the same on every image:
  - A hit is one address and its axes. An axis count of 0 is a scalar,
    so the address alone is the hit. One axis is a curve, two a map.
  - When that image's XDF contains the name, the body address must
    match one of its rows.
  - The axis column counts the axes present on the maps that hit. Its
    denominator is the list's axis count for those maps, so a scalar
    adds nothing. It is 0 only when every map that hit is a scalar.
  - The confidence column is the body bytes of the names that hit. Its
    denominator is that matched set, not the tuner list.

An XDF, when present, is the address oracle for that image. Its x and y
axes that have an address are the axis column of that section.
`

// defs are the definition file flags shared by generate and probe.
type defs struct {
	core, names, meas, mapPath, alias, user *string
}

func defFlags(fs *flag.FlagSet) defs {
	defer cli.Short(fs, "n", "names", "a", "alias", "u", "user")
	return defs{
		core:    fs.String("core", config.Path("ME7_CORE", config.NeedlesFile), "needle YAML `<file>`"),
		names:   fs.String("names", config.Path("ME7_NAMES", config.NamesFile), "ME7 name YAML `<file>`"),
		meas:    fs.String("meas", config.Path("ME7_MEAS", config.MeasuresFile), "per-part measurement YAML `<file>`"),
		mapPath: fs.String("map", config.Path("ME7_MAP", config.MapDir), "result-type catalog `<dir>`"),
		alias:   fs.String("alias", config.Path("ME7_ALIAS", config.AliasFile), "alias `<file>`"),
		user:    fs.String("user", config.Path("ME7_USER", "user"), "`<dir>` of user needles, measurements, and conversions"),
	}
}

// image parses args, wants one flash image, and resolves --user.
func image(fs *flag.FlagSet, d defs, args []string) (path string, img []byte, userDir string, err error) {
	if err = cli.Parse(fs, args); err != nil {
		return
	}
	if fs.NArg() != 1 {
		err = fmt.Errorf("%s wants one flash image", fs.Name())
		return
	}
	path = fs.Arg(0)
	if img, err = os.ReadFile(path); err != nil {
		return
	}
	userDir, err = resolveUser(fs, *d.user)
	return
}

func (d defs) generate(imgPath string, img []byte, userDir string, clock int, scale, conn string) (*generate.Result, error) {
	return generate.Generate(generate.Options{
		Image: img, ImageName: filepath.Base(imgPath),
		CorePath: *d.core, NamesPath: *d.names, MeasPath: *d.meas,
		MapPath: *d.mapPath, AliasPath: *d.alias,
		Clock: clock, Scale: scale, Connect: conn, UserDir: userDir,
	})
}

func cmdGenerate(args []string) error {
	fs := cli.NewFlagSet("me7info", "generate", "[flags] <image.bin>", "")
	d := defFlags(fs)
	out := fs.String("o", "", "ecu output `<file>`, - for stdout (default <image>.ecu)")
	xdfPath := fs.String("xdf", "", "xdf output `<file>`, - for stdout (default <image>.xdf when maps were located)")
	scale := fs.String("5120", "auto", "mbar scaling `<mode>`: auto, on, or off")
	clock := fs.Int("clock", 0, "CPU clock `<MHz>`: 20, 24, 32, or 40; 0 uses config/names.yaml")
	conn := fs.String("connect", "", "override Connect with `<mode>`, for example SLOW-0x11")
	cli.Short(fs, "x", "xdf", "5", "5120")
	imgPath, img, userDir, err := image(fs, d, args)
	if err != nil {
		return err
	}
	res, err := d.generate(imgPath, img, userDir, *clock, *scale, *conn)
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
	fs := cli.NewFlagSet("me7info", "parity", "[flags]", parityHelp)
	dir := fs.String("data", "testdata/parity", "parity root `<dir>`: images.yaml, ecu/me7info, and xdf")
	cli.Short(fs, "d", "data")
	corpus := fs.String("corpus", ecucorpus.Dir("."), "ecu-corpus checkout `<dir>`; XDFKIT_CORPUS sets the default")
	if err := cli.Parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("parity takes no image; images.yaml names corpus images")
	}
	c, err := ecucorpus.Load(*corpus)
	if err != nil {
		return err
	}
	rep, err := parity.Run(*dir, c)
	if err != nil {
		return err
	}
	fmt.Print(rep.Text())
	return nil
}

func cmdProbe(args []string) error {
	fs := cli.NewFlagSet("me7info", "probe", "[flags] <image.bin>", "")
	d := defFlags(fs)
	maps := fs.Bool("maps", false, "also count the calibration maps generate would write to the XDF")
	imgPath, img, userDir, err := image(fs, d, args)
	if err != nil {
		return err
	}
	ns, err := config.LoadNeedles(*d.core, userDir)
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
	if !*maps {
		return nil
	}
	res, err := d.generate(imgPath, img, userDir, 0, "auto", "")
	if err != nil {
		return err
	}
	var named, scalars, curves, grids int
	for _, m := range res.Maps {
		switch {
		case m.Name == "":
			continue
		case m.Y != nil:
			grids++
		case m.X != nil:
			curves++
		default:
			scalars++
		}
		named++
	}
	fmt.Printf("maps: %d named (%d maps, %d curves, %d scalars), %d unnamed\n",
		named, grids, curves, scalars, len(res.Maps)-named)
	return nil
}

func resolveUser(fs *flag.FlagSet, dir string) (string, error) {
	return config.ResolveUserDir(dir, os.Getenv("ME7_USER") != "" || cli.IsSet(fs, "user"))
}

func fmtOffs(offs []int) string {
	parts := make([]string, len(offs))
	for i, o := range offs {
		parts[i] = fmt.Sprintf("0x%X", o)
	}
	return strings.Join(parts, " ")
}
