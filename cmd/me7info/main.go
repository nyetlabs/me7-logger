package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
	kitxdf "go.nyet.org/xdfkit/xdf"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/generate"
	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/locate"
	"go.nyet.org/me7-logger/opcode"
	"go.nyet.org/me7-logger/place"
	"go.nyet.org/me7-logger/record"
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
	case "place":
		err = cmdPlace(os.Args[2:])
	case "-v", "--version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
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
  me7info place [flags] <ref.json> <ref.bin> <dst.bin>
  me7info -v, --version
  me7info -h, --help

Commands:
  generate  Write an .ecu file for the image and, when calibration maps
            were located, a TunerPro XDF. --model writes the full located
            model as JSON. Does not open a serial port.
  probe     Report the DPP block and needle hits. --maps also counts the
            calibration maps generate would write to the XDF.
  place     Copy a map catalog onto another image. An object whose bytes
            match is kept. One code pointer, found once, moves it. Anything
            else is left out. The report is on stderr.

Run "me7info <command> -h" for that command's flags.

Needle names, connect bytes, and per-part addresses are the YAML files
in config/ beside this executable (symlinks resolved).
`, version)
}

// image parses args, wants one flash image, and resolves --user.
func image(fs *pflag.FlagSet, d locate.Defs, args []string) (path string, img []byte, userDir string, err error) {
	if err = fs.Parse(args); err != nil {
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
	userDir, err = locate.UserDir(fs, *d.User)
	return
}

func cmdGenerate(args []string) error {
	fs := cli.NewFlagSet("me7info", "generate", "[flags] <image.bin>", "")
	d := locate.Flags(fs)
	out := fs.StringP("output", "o", "", "ecu output `<file>`, - for stdout (default <image>.ecu)")
	xdfPath := fs.StringP("xdf", "x", "", "tuner xdf output `<file>`, - for stdout (default <image>.xdf when listed maps were located)")
	fullPath := fs.String("full-xdf", "", "also write every located named map to this xdf `<file>`, - for stdout")
	modelPath := fs.String("model", "", "full located model JSON `<file>`, - for stdout")
	scale := fs.String("5120", "auto", "mbar scaling `<mode>`: auto, on, or off")
	clock := fs.Int("clock", 0, "CPU clock `<MHz>`: 20, 24, 32, or 40; 0 uses config/names.yaml")
	conn := fs.String("connect", "", "override Connect with `<mode>`, for example SLOW-0x11")
	imgPath, img, userDir, err := image(fs, d, args)
	if err != nil {
		return err
	}
	res, err := d.Generate(imgPath, img, userDir, *clock, *scale, *conn)
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
	cats, err := config.LoadCategories("")
	if err != nil {
		return err
	}
	if *modelPath != "" {
		if _, err := locate.WriteModel(*modelPath, imgPath, img, res.Maps, cats, "me7info generate"); err != nil {
			return err
		}
	}
	if *fullPath != "" {
		if err := writeXDF(*fullPath, imgPath, img, res.Maps, cats, false); err != nil {
			return err
		}
	}
	return writeXDF(*xdfPath, imgPath, img, res.Maps, cats, true)
}

func writeXDF(path, imgPath string, img []byte, maps []record.Map, cats *model.CategoryTable, tuner bool) error {
	m, _, err := locate.Model(imgPath, img, maps, cats, tuner)
	if err != nil || len(m.Objects) == 0 {
		return err
	}
	b, err := kitxdf.Write(m, img, filepath.Base(imgPath))
	if err != nil {
		return err
	}
	if path == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	if path == "" {
		path = strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + ".xdf"
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}

func cmdProbe(args []string) error {
	fs := cli.NewFlagSet("me7info", "probe", "[flags] <image.bin>", "")
	d := locate.Flags(fs)
	maps := fs.Bool("maps", false, "also count the calibration maps generate would write to the XDF")
	imgPath, img, userDir, err := image(fs, d, args)
	if err != nil {
		return err
	}
	ns, err := config.LoadNeedles(*d.Core, userDir)
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
	res, err := d.Generate(imgPath, img, userDir, 0, "auto", "")
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
	cats, err := config.LoadCategories("")
	if err != nil {
		return err
	}
	fmt.Printf("maps: %d named (%d maps, %d curves, %d scalars), %d unnamed\n",
		named, grids, curves, scalars, len(res.Maps)-named)
	var n [2]int
	var conflicts []string
	for i, tuner := range []bool{true, false} {
		m, c, err := locate.Model(imgPath, img, res.Maps, cats, tuner)
		if err != nil {
			return err
		}
		n[i], conflicts = len(m.Objects), c
	}
	fmt.Printf("xdf: %d in the tuner xdf, %d in the full xdf\n", n[0], n[1])
	for _, c := range conflicts {
		fmt.Println("xdf: not a breakpoint curve:", c)
	}
	return nil
}

func cmdPlace(args []string) error {
	fs := cli.NewFlagSet("me7info", "place", "[flags] <ref.json> <ref.bin> <dst.bin>", "")
	out := fs.StringP("output", "o", "-", "model output `<file>`, - for stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		return fmt.Errorf("place wants a model and two images")
	}
	raw, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var refModel model.Model
	if err := canon.Unmarshal(raw, &refModel); err != nil {
		return err
	}
	ref, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return err
	}
	dst, err := os.ReadFile(fs.Arg(2))
	if err != nil {
		return err
	}
	placed, rep, err := place.Place(&refModel, ref, dst)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(dst)
	placed.Provenance = &model.Provenance{
		Format: "image", Origin: "located",
		File: filepath.Base(fs.Arg(2)), SHA256: hex.EncodeToString(sum[:]),
	}
	if err := placed.Check(); err != nil {
		return err
	}
	text, err := canon.MarshalStamped(placed, "me7info place")
	if err != nil {
		return err
	}
	if *out == "-" {
		if _, err := os.Stdout.Write(text); err != nil {
			return err
		}
	} else if err := os.WriteFile(*out, text, 0o644); err != nil {
		return err
	}
	return rep.Format(os.Stderr)
}

func fmtOffs(offs []int) string {
	parts := make([]string, len(offs))
	for i, o := range offs {
		parts[i] = fmt.Sprintf("0x%X", o)
	}
	return strings.Join(parts, " ")
}
