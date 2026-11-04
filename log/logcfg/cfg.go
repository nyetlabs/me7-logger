// Package logcfg reads an ME7Logger log configuration: the .ecu file name,
// the sample rate, and one variable name per line.
package logcfg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Var is one logged name. Alias overrides the .ecu alias when set.
type Var struct {
	Name  string
	Alias string
}

// File is a .cfg.
type File struct {
	ECU              string
	SamplesPerSecond int
	Vars             []Var
}

// Parse parses cfg text. SamplesPerSecond defaults to 10.
func Parse(text string) (File, error) {
	f := File{SamplesPerSecond: 10}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, ";") || strings.HasPrefix(trim, "#") {
			continue
		}
		if k, v, ok := kv(trim); ok {
			switch k {
			case "ECUCharacteristics":
				f.ECU = v
			case "SamplesPerSecond":
				n, err := strconv.Atoi(v)
				if err != nil {
					return File{}, fmt.Errorf("SamplesPerSecond: %w", err)
				}
				f.SamplesPerSecond = n
			}
			continue
		}
		name, alias := varLine(trim)
		if name == "" {
			continue
		}
		f.Vars = append(f.Vars, Var{Name: name, Alias: alias})
	}
	if f.SamplesPerSecond < 1 || f.SamplesPerSecond > 50 {
		return File{}, fmt.Errorf("SamplesPerSecond %d wants 1..50", f.SamplesPerSecond)
	}
	return f, nil
}

// Load reads a cfg file.
func Load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	return Parse(string(b))
}

func kv(line string) (string, string, bool) {
	if i := strings.Index(line, ";"); i >= 0 {
		line = line[:i]
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func varLine(line string) (string, string) {
	if i := strings.Index(line, "{"); i >= 0 {
		name := strings.TrimSpace(line[:i])
		rest := line[i:]
		if j := strings.Index(rest, "}"); j >= 0 {
			return name, rest[1:j]
		}
		return name, ""
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], fields[1]
}
