package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/pflag"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/kwp"
	"go.nyet.org/me7-logger/logcfg"
	"go.nyet.org/me7-logger/logger"
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
	case "log":
		err = cmdLog(os.Args[2:])
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
		fmt.Fprintf(os.Stderr, "me7logger: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7logger %s

Usage:
  me7logger log [flags] <image.bin> <config.cfg>
  me7logger -v, --version
  me7logger -h, --help

Commands:
  log  Sample RAM over stock KWP2000 and write CSV. Does not write
       flash or EEPROM. Logging stays at 10400 baud, the init baud.

Run "me7logger log -h" for its flags.
`, version)
}

func cmdLog(args []string) error {
	fs := cli.NewFlagSet("me7logger", "log", "[flags] <image.bin> <config.cfg>", "")
	core := fs.String("core", config.Path("ME7_CORE", config.NeedlesFile), "needle YAML `<file>`")
	names := fs.StringP("names", "n", config.Path("ME7_NAMES", config.NamesFile), "ME7 name YAML `<file>`")
	meas := fs.String("meas", config.Path("ME7_MEAS", config.MeasuresFile), "per-part measurement YAML `<file>`")
	mapPath := fs.String("map", config.Path("ME7_MAP", config.MapDir), "result-type catalog `<dir>`")
	alias := fs.StringP("alias", "a", config.Path("ME7_ALIAS", config.AliasFile), "alias `<file>`")
	port := fs.StringP("port", "p", "", "serial `<port>`, for example /dev/tty.usbserial or COM3 (required)")
	sps := fs.IntP("sps", "s", 0, "`<samples>` per second, overrides the cfg")
	baud := fs.IntP("baud", "b", 0, "`<baud>` override; only 10400 is implemented")
	out := fs.StringP("output", "o", "", "csv output `<file>`, appended to (default stdout)")
	one := fs.BoolP("one", "1", false, "read one sample and stop")
	scale := fs.String("5120", "auto", "mbar scaling `<mode>`: auto, on, or off")
	clock := fs.Int("clock", 0, "CPU clock `<MHz>`: 20, 24, 32, or 40; 0 uses config/names.yaml")
	user := fs.StringP("user", "u", config.Path("ME7_USER", "user"), "`<dir>` of user needles, measurements, and conversions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("log wants a flash image and a cfg file")
	}
	if *port == "" {
		return fmt.Errorf("log wants -p, --port <serial port>")
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

func resolveUser(fs *pflag.FlagSet, dir string) (string, error) {
	return config.ResolveUserDir(dir, os.Getenv("ME7_USER") != "" || fs.Changed("user"))
}
