package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"me7-logger/config"
	"me7-logger/kwp"
	"me7-logger/logcfg"
	"me7-logger/logger"
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
	case "version", "-version", "--version":
		fmt.Println(version)
	case "-h", "-help", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "me7logger: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `me7logger %s

  me7logger log [flags] image.bin config.cfg
  me7logger version

log samples RAM over stock KWP2000. It does not write flash or EEPROM.
Logging stays at 10400 baud, which is the init baud.
`, version)
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

func resolveUser(fs *flag.FlagSet, dir string) (string, error) {
	explicit := os.Getenv("ME7_USER") != ""
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "user" {
			explicit = true
		}
	})
	return config.ResolveUserDir(dir, explicit)
}
