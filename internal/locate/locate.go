// Package locate registers the definition-file flags and writes the full
// located model. generate, probe, and publish share them.
package locate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/generate"
	"go.nyet.org/me7-logger/record"
	"go.nyet.org/me7-logger/xdf"
)

// Defs are the definition file flags shared by generate, probe, and publish.
type Defs struct {
	Core, Names, Meas, MapPath, Alias, User *string
}

// Flags registers those flags on fs.
func Flags(fs *pflag.FlagSet) Defs {
	return Defs{
		Core:    fs.String("core", config.Path("ME7_CORE", config.NeedlesFile), "needle YAML `<file>`"),
		Names:   fs.StringP("names", "n", config.Path("ME7_NAMES", config.NamesFile), "ME7 name YAML `<file>`"),
		Meas:    fs.String("meas", config.Path("ME7_MEAS", config.MeasuresFile), "per-part measurement YAML `<file>`"),
		MapPath: fs.String("map", config.Path("ME7_MAP", config.MapDir), "result-type catalog `<dir>`"),
		Alias:   fs.StringP("alias", "a", config.Path("ME7_ALIAS", config.AliasFile), "alias `<file>`"),
		User:    fs.StringP("user", "u", config.Path("ME7_USER", "user"), "`<dir>` of user needles, measurements, and conversions"),
	}
}

// Generate locates measurements and maps with the flag paths.
func (d Defs) Generate(imgPath string, img []byte, userDir string, clock int, scale, conn string) (*generate.Result, error) {
	return generate.Generate(generate.Options{
		Image: img, ImageName: filepath.Base(imgPath),
		CorePath: *d.Core, NamesPath: *d.Names, MeasPath: *d.Meas,
		MapPath: *d.MapPath, AliasPath: *d.Alias,
		Clock: clock, Scale: scale, Connect: conn, UserDir: userDir,
	})
}

// UserDir resolves --user. An explicit flag or ME7_USER wins over the default.
func UserDir(fs *pflag.FlagSet, dir string) (string, error) {
	return config.ResolveUserDir(dir, os.Getenv("ME7_USER") != "" || fs.Changed("user"))
}

// Model is the model of the maps for the tuner XDF (the table's maps and
// their axes) or the full XDF (every map, unlisted ones under xdf.Other),
// and the constants xdf.Model couldn't make breakpoint curves.
func Model(imgPath string, img []byte, maps []record.Map, cats *model.CategoryTable, tuner bool) (*model.Model, []string, error) {
	m, conflicts := xdf.Model(maps, img, filepath.Base(imgPath))
	if tuner {
		return m, conflicts, m.Tuner(cats)
	}
	m.Categorize(cats, xdf.Other)
	return m, conflicts, nil
}

// WriteModel writes the full located model as JSON. An empty model writes
// nothing and returns false.
func WriteModel(path, imgPath string, img []byte, maps []record.Map, cats *model.CategoryTable, stamp string) (bool, error) {
	m, _, err := Model(imgPath, img, maps, cats, false)
	if err != nil || len(m.Objects) == 0 {
		return false, err
	}
	b, err := canon.MarshalStamped(m, stamp)
	if err != nil {
		return false, err
	}
	if path == "-" {
		_, err = os.Stdout.Write(b)
		return err == nil, err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return false, err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return true, nil
}
