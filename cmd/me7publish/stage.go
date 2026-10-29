package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/parity"
)

const stageHelp = `
Builds one IMAGE-DATE.zip per corpus definition into --stage, copies
index.php, and writes vehicles.json. DATE is YYYYMMDD from the last
non-chore(schema) commit of the definition (and categories.json for ME7),
else the file mtime. KP, CSV, and XDF come from xdfkit. Source is each
definition's provenance. Stage and confidence are locate grades; both
are - for DAMOS.
`

func cmdStage(args []string) error {
	fs := cli.NewFlagSet("me7publish", "stage", "[flags]", stageHelp)
	root, err := ecucorpus.ModuleRoot()
	if err != nil {
		return err
	}
	corpusFlag := fs.String("corpus", "", "ecu-corpus checkout `<dir>` (default CORPUS, else ../ecu-corpus, else ecu-corpus/)")
	stageDir := fs.String("stage", filepath.Join(root, "build/publish/stage"), "output `<dir>` for zips and vehicles.json")
	workDir := fs.String("work", filepath.Join(root, "build/publish/work"), "scratch `<dir>`")
	publishDir := fs.String("publish", filepath.Join(root, "publish"), "publish `<dir>` with vehicles.yaml and index.php")
	data := fs.StringP("data", "d", filepath.Join(root, "testdata/parity"), "parity root `<dir>`")
	xdfkitFlag := fs.String("xdfkit", "", "xdfkit `<binary>` (default ../xdfkit/build/xdfkit, else PATH)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	corpus := corpusDir(root, *corpusFlag)
	c, err := ecucorpus.Load(corpus)
	if err != nil {
		return err
	}
	xdfkit, err := findXdfkit(*xdfkitFlag, root)
	if err != nil {
		return err
	}
	vehicles, err := loadVehicles(filepath.Join(*publishDir, "vehicles.yaml"))
	if err != nil {
		return err
	}
	names := c.WithDefs()
	if err := checkVehicles(c, vehicles, names); err != nil {
		return err
	}
	if err := os.MkdirAll(*stageDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(*workDir, 0o755); err != nil {
		return err
	}
	cats := filepath.Join(corpus, "categories.json")
	keep := map[string]bool{}
	for _, name := range names {
		zipPath, err := stageOne(c, name, *stageDir, *workDir, xdfkit, cats)
		if err != nil {
			return err
		}
		keep[zipPath] = true
	}
	if err := pruneZips(*stageDir, keep); err != nil {
		return err
	}
	grades, err := loadGrades(*data, c)
	if err != nil {
		return err
	}
	if err := writeVehiclesJSON(filepath.Join(*stageDir, "vehicles.json"), c, vehicles, grades); err != nil {
		return err
	}
	return copyFile(filepath.Join(*stageDir, "index.php"), filepath.Join(*publishDir, "index.php"))
}

func corpusDir(root, flag string) string {
	if flag != "" {
		return flag
	}
	if d := os.Getenv("CORPUS"); d != "" {
		return d
	}
	full := filepath.Join(root, "..", "ecu-corpus")
	if _, err := os.Stat(filepath.Join(full, "corpus.tsv")); err == nil {
		return full
	}
	return ecucorpus.Dir(root)
}

func findXdfkit(flag, root string) (string, error) {
	if flag != "" {
		if _, err := os.Stat(flag); err != nil {
			return "", fmt.Errorf("xdfkit: %w", err)
		}
		return flag, nil
	}
	built := filepath.Join(root, "..", "xdfkit", "build", "xdfkit")
	if st, err := os.Stat(built); err == nil && !st.IsDir() {
		return built, nil
	}
	path, err := exec.LookPath("xdfkit")
	if err != nil {
		return "", fmt.Errorf("xdfkit: not found")
	}
	return path, nil
}

type vehicle struct {
	Engine, Code, Vehicle, Year, Gearbox, Market string
}

func loadVehicles(path string) (map[string]vehicle, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	fields := []string{"engine", "code", "vehicle", "year", "gearbox", "market"}
	out := map[string]vehicle{}
	for name, row := range raw {
		if row == nil {
			return nil, fmt.Errorf("%s: %s: bad entry", path, name)
		}
		cells := map[string]string{}
		for _, f := range fields {
			cells[f] = ""
		}
		for k, v := range row {
			if _, ok := cells[k]; !ok {
				return nil, fmt.Errorf("%s: %s: unknown field %s", path, name, k)
			}
			cells[k] = fmt.Sprint(v)
		}
		out[name] = vehicle{
			Engine: cells["engine"], Code: cells["code"], Vehicle: cells["vehicle"],
			Year: cells["year"], Gearbox: cells["gearbox"], Market: cells["market"],
		}
	}
	return out, nil
}

func checkVehicles(c *ecucorpus.Corpus, vehicles map[string]vehicle, names []string) error {
	missing := false
	for _, name := range names {
		if _, ok := vehicles[name]; !ok {
			fmt.Fprintf(os.Stderr, "%s: no vehicle information\n", name)
			missing = true
		}
		if fam := c.Family(name); fam == "" || fam == "-" {
			fmt.Fprintf(os.Stderr, "%s: no family in corpus.tsv\n", name)
			missing = true
		}
	}
	if missing {
		return fmt.Errorf("missing vehicle or family rows")
	}
	return nil
}

func stageOne(c *ecucorpus.Corpus, name, stageDir, workDir, xdfkit, cats string) (string, error) {
	rel := c.DefRel(name)
	src := c.Def(name)
	img, err := c.Path(name)
	if err != nil {
		return "", err
	}
	d, err := fileDate(c.Dir, src, rel)
	if err != nil {
		return "", err
	}
	tuner := strings.HasPrefix(c.Family(name), "ME7")
	if tuner {
		cd, err := fileDate(c.Dir, cats, "categories.json")
		if err != nil {
			return "", err
		}
		if cd > d {
			d = cd
		}
	}
	zipPath := filepath.Join(stageDir, name+"-"+d+".zip")
	if zipFresh(zipPath, src, img, xdfkit, cats, tuner) {
		return zipPath, nil
	}
	w := filepath.Join(workDir, name)
	if err := os.RemoveAll(w); err != nil {
		return "", err
	}
	if err := os.MkdirAll(w, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(w, name+".json")
	if err := copyFile(dst, src); err != nil {
		return "", err
	}
	if err := runLint(xdfkit, img, dst, name); err != nil {
		return "", err
	}
	members := []string{name + ".json", name + ".kp", name + ".csv", name + ".xdf", name + ".meta.json"}
	if err := run(xdfkit, "--force", dst, filepath.Join(w, name+".kp")); err != nil {
		return "", err
	}
	if err := run(xdfkit, "-f", "csv", "--force", "-i", img, dst, filepath.Join(w, name+".csv")); err != nil {
		return "", err
	}
	if err := run(xdfkit, "-f", "xdf", "--force", "-i", img, dst, filepath.Join(w, name+".xdf")); err != nil {
		return "", err
	}
	if tuner {
		if err := run(xdfkit, "-f", "csv", "--force", "--tuner", cats, "-i", img, dst, filepath.Join(w, name+"-tuner.csv")); err != nil {
			return "", err
		}
		if err := run(xdfkit, "-f", "xdf", "--force", "--tuner", cats, "-i", img, dst, filepath.Join(w, name+"-tuner.xdf")); err != nil {
			return "", err
		}
		members = append(members, name+"-tuner.csv", name+"-tuner.xdf", name+"-tuner.meta.json")
	}
	if err := writeZip(zipPath, w, members); err != nil {
		return "", err
	}
	return zipPath, nil
}

func zipFresh(zipPath, src, img, xdfkit, cats string, tuner bool) bool {
	zst, err := os.Stat(zipPath)
	if err != nil {
		return false
	}
	newer := func(path string) bool {
		st, err := os.Stat(path)
		return err == nil && !zst.ModTime().After(st.ModTime())
	}
	if newer(src) || newer(img) || newer(xdfkit) {
		return false
	}
	return !tuner || !newer(cats)
}

func runLint(xdfkit, img, def, name string) error {
	cmd := exec.Command(xdfkit, "lint", "-i", img, def)
	out, err := cmd.CombinedOutput()
	st := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			st = ee.ExitCode()
		} else {
			return err
		}
	}
	if st == 2 {
		return fmt.Errorf("%s", strings.TrimSuffix(string(out), "\n"))
	}
	if st != 0 && st != 1 {
		return fmt.Errorf("xdfkit lint: exit %d\n%s", st, strings.TrimSuffix(string(out), "\n"))
	}
	fmt.Println(lintSummary(name, string(out)))
	return nil
}

var lintRule = regexp.MustCompile(`^(R[0-9]+)@`)

func lintSummary(name, out string) string {
	counts := map[string]int{}
	n := 0
	for _, line := range strings.Split(out, "\n") {
		m := lintRule.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		counts[m[1]]++
		n++
	}
	if n == 0 {
		return "lint " + name + ": 0"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "lint %s:", name)
	for r, c := range counts {
		fmt.Fprintf(&b, " %s=%d", r, c)
	}
	return b.String()
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeZip(path, dir string, members []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, name := range members {
		if err := zipFile(zw, filepath.Join(dir, name), name); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func zipFile(zw *zip.Writer, path, name string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(st)
	if err != nil {
		return err
	}
	hdr.Name = name
	hdr.Method = zip.Store
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}

func pruneZips(stageDir string, keep map[string]bool) error {
	_ = os.RemoveAll(filepath.Join(stageDir, "full"))
	ents, err := os.ReadDir(stageDir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(stageDir, e.Name())
		if !strings.HasSuffix(e.Name(), ".zip") {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		if !keep[path] {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

type grade struct {
	stage, conf, source string
}

func loadGrades(data string, c *ecucorpus.Corpus) (map[string]grade, error) {
	rep, err := parity.Run(data, c)
	if err != nil {
		return nil, err
	}
	out := map[string]grade{}
	for _, im := range rep.Tuner {
		name := strings.TrimSuffix(im.Name, ".bin")
		out[name] = grade{
			stage:  parity.StageLabel(im.Tier),
			conf:   fmt.Sprintf("%d/%d", im.Confidence.Hit, im.Confidence.Total),
			source: parity.SourceClass(im.Provenance),
		}
	}
	return out, nil
}

func writeVehiclesJSON(path string, c *ecucorpus.Corpus, vehicles map[string]vehicle, grades map[string]grade) error {
	joined := map[string][]string{}
	missing := false
	for name, v := range vehicles {
		fam := c.Family(name)
		if fam == "" || fam == "-" {
			fmt.Fprintf(os.Stderr, "%s: no family in corpus.tsv\n", name)
			missing = true
			continue
		}
		g := grades[name]
		// Source is always the definition's provenance, including images
		// me7info cannot score. Stage and confidence are locate grades;
		// "-" for DAMOS (canonical), where those are not trust of the file.
		if p, err := readProvenance(c.Def(name)); err == nil && p != nil {
			g.source = parity.SourceClass(p)
		}
		if g.source == "DAMOS" {
			g.stage = "-"
			g.conf = "-"
		}
		joined[name] = []string{fam, v.Engine, v.Code, v.Vehicle, v.Year, v.Gearbox, v.Market, g.stage, g.conf, g.source}
	}
	if missing {
		return fmt.Errorf("missing family rows")
	}
	b, err := json.MarshalIndent(joined, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func readProvenance(path string) (*model.Provenance, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m model.Model
	if err := canon.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m.Provenance, nil
}

// fileDate is YYYYMMDD of the last non-chore(schema) commit of rel, else mtime.
func fileDate(corpus, abs, rel string) (string, error) {
	cmd := exec.Command("git", "-C", corpus, "log", "-1",
		"--format=%cd", "--date=format:%Y%m%d",
		"--invert-grep", "--grep=^chore(schema):", "--", rel)
	out, err := cmd.Output()
	if err == nil {
		if d := strings.TrimSpace(string(out)); d != "" {
			return d, nil
		}
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	return st.ModTime().UTC().Format("20060102"), nil
}

func copyFile(dst, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
