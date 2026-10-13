package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/parity"
)

const onboardHelp = `
Scores corpus images that are not in the parity set yet, and prints
the score. --submit adds the ones whose ME7Info rows all match.
With no NAME, every such image is scored. A duplicate layout, a
short score, or a missing block is left out.

not scored
    has-ecu         an .ecu is present, and a duplicate layout kept it out
    needs-ME7Info   ME7Info.exe still has to run
    unsupported     the image is not a usable size
ME7Info
    short      a ME7Info row missed
    eligible   every ME7Info row matched
    unscored   no .ecu to score

Paths, ME7INFO_EXE, and WINE are in docs/parity.md.
` + nameArg

const (
	statusHasECU      = "has-ecu"
	statusNeeds       = "needs-ME7Info"
	statusUnsupported = "unsupported"
	statusEligible    = "eligible"
	statusShort       = "short"
	statusUnscored    = "unscored"
)

func cmdOnboard(args []string) error {
	fs := cli.NewFlagSet("me7parity", "onboard", "[--submit] [NAME...]", onboardHelp)
	data := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`")
	incoming := fs.String("incoming", "", "incoming `<dir>` (default <data>/incoming)")
	corpus := fs.String("corpus", ecucorpus.Dir("."), "ecu-corpus checkout `<dir>`: corpus.tsv and images/<part>/<stem>.bin; XDFKIT_CORPUS sets the default")
	submit := fs.Bool("submit", false, "add the eligible images")
	if err := fs.Parse(args); err != nil {
		return err
	}
	in := *incoming
	if in == "" {
		in = filepath.Join(*data, "incoming")
	}
	c, err := ecucorpus.Load(*corpus)
	if err != nil {
		return err
	}
	o := &onboard{data: *data, in: in, c: c}
	only := fs.Args()
	if len(only) > 0 {
		if only, err = o.resolve(only); err != nil {
			return err
		}
	}
	return o.all(only, *submit)
}

func (o *onboard) all(only []string, submit bool) error {
	cands, err := o.picked(only)
	if err != nil {
		return err
	}
	var needs []string
	for _, row := range cands {
		if row.status == statusNeeds {
			needs = append(needs, row.name)
		}
	}
	if len(needs) > 0 {
		if err := o.run(needs); err != nil {
			fmt.Fprintf(os.Stderr, "me7parity: %v\n", err)
		}
		cands, err = o.picked(only)
		if err != nil {
			return err
		}
	}
	var score []string
	var skipped []candidate
	for _, row := range cands {
		if row.status == statusHasECU && !strings.Contains(row.note, "dup:") {
			score = append(score, row.name)
			continue
		}
		skipped = append(skipped, row)
	}
	if err := o.printRows("not scored", skipped); err != nil {
		return err
	}
	if len(score) == 0 {
		return nil
	}
	if len(skipped) > 0 {
		fmt.Println()
	}
	rows, err := o.triage(score)
	if err != nil {
		return err
	}
	var eligible []string
	for _, row := range rows {
		if row.status == statusEligible {
			eligible = append(eligible, row.name)
		}
	}
	return o.promoteReady(eligible, submit)
}

type onboard struct {
	data string
	in   string
	c    *ecucorpus.Corpus
}

type candidate struct {
	name, status, id, epk, note string
}

func (o *onboard) resolve(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	rows, err := o.picked(args)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.name
	}
	return out, nil
}

func (o *onboard) picked(only []string) ([]candidate, error) {
	rows, err := o.candidates()
	if err != nil {
		return nil, err
	}
	if len(only) == 0 {
		return rows, nil
	}
	names := make([]string, len(rows))
	by := map[string]candidate{}
	for i, row := range rows {
		names[i], by[row.name] = row.name, row
	}
	hit, err := matchStems(names, only)
	if err != nil {
		return nil, err
	}
	out := make([]candidate, len(hit))
	for i, n := range hit {
		out[i] = by[n]
	}
	return out, nil
}

func (o *onboard) printRows(heading string, rows []candidate) error {
	if len(rows) == 0 {
		return nil
	}
	fmt.Println(heading)
	nameW, statusW := len("name"), len("status")
	for _, row := range rows {
		nameW = max(nameW, len(row.name))
		statusW = max(statusW, len(row.status))
	}
	fmt.Printf("%-*s  %-*s  %s\n", nameW, "name", statusW, "status", "epk")
	for _, row := range rows {
		epk := row.epk
		if epk == "" {
			epk = "-"
		}
		line := fmt.Sprintf("%-*s  %-*s  %s", nameW, row.name, statusW, row.status, epk)
		if row.note != "" {
			line += "  " + row.note
		}
		fmt.Println(line)
	}
	return nil
}

func (o *onboard) candidates() ([]candidate, error) {
	scored, err := yamlNames(filepath.Join(o.data, "images.yaml"))
	if err != nil {
		return nil, err
	}
	scoredSet := map[string]struct{}{}
	for _, n := range scored {
		scoredSet[n] = struct{}{}
	}
	scoredID := map[string]string{}
	for _, n := range scored {
		id, err := o.layoutOf(n)
		if err != nil || id == "" {
			continue
		}
		if _, ok := scoredID[id]; !ok {
			scoredID[id] = n
		}
	}
	var rows []candidate
	ids := map[string][]string{}
	for _, n := range o.c.Names() {
		if _, ok := scoredSet[n]; ok {
			continue
		}
		row, err := o.oneCandidate(n)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
		if row.id != "" {
			ids[row.id] = append(ids[row.id], n)
		}
	}
	for i, row := range rows {
		var notes []string
		if other := scoredID[row.id]; other != "" {
			notes = append(notes, "dup:"+other)
		}
		if row.id != "" && len(ids[row.id]) > 1 {
			var shared []string
			for _, n := range ids[row.id] {
				if n != row.name {
					shared = append(shared, n)
				}
			}
			notes = append(notes, "shared:"+strings.Join(shared, ","))
		}
		rows[i].note = strings.Join(notes, " ")
	}
	return rows, nil
}

func (o *onboard) oneCandidate(name string) (candidate, error) {
	row := candidate{name: name, status: statusNeeds}
	if o.ecuPath(name) != "" {
		row.status = statusHasECU
	}
	path, err := o.c.Path(name)
	if err != nil {
		return row, err
	}
	st, err := os.Stat(path)
	if err != nil {
		row.status = statusUnsupported
		return row, nil
	}
	if row.status == statusNeeds && st.Size() != 512*1024 && st.Size() != 1024*1024 {
		row.status = statusUnsupported
	}
	img, err := os.ReadFile(path)
	if err != nil {
		return row, nil
	}
	row.id = parity.LayoutID(img)
	row.epk = parity.EPK(img)
	return row, nil
}

func (o *onboard) layoutOf(name string) (string, error) {
	path, err := o.c.Path(name)
	if err != nil {
		return "", err
	}
	img, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return parity.LayoutID(img), nil
}

// ecuPath is the raw ME7Info output for name: NAME.ecu, or the part
// before the version suffix when that file is the one on disk.
func (o *onboard) ecuPath(name string) string {
	raw := filepath.Join(o.in, "ecus")
	for _, n := range []string{name, barePart(name)} {
		p := filepath.Join(raw, n+".ecu")
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return p
		}
	}
	return ""
}

func barePart(name string) string {
	if i := strings.LastIndex(name, "-"); i > 0 {
		return name[:i]
	}
	return name
}

func (o *onboard) run(names []string) error {
	exe := os.Getenv("ME7INFO_EXE")
	if exe == "" {
		root, err := ecucorpus.ModuleRoot()
		if err != nil {
			return err
		}
		exe = filepath.Join(root, "..", "ME7L", "bin", "ME7Info.exe")
	}
	wine := os.Getenv("WINE")
	if wine == "" {
		wine = "wine"
	}
	if err := os.MkdirAll(filepath.Join(o.in, "ecus"), 0o755); err != nil {
		return err
	}
	var failed int
	for _, name := range names {
		if err := o.runOne(wine, exe, name); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d ME7Info run(s) failed", failed)
	}
	return nil
}

func (o *onboard) runOne(wine, exe, name string) error {
	out := filepath.Join(o.in, "ecus", name+".ecu")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return nil
	}
	bin, err := o.c.Path(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("ME7INFO_EXE %s: %w", exe, err)
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	absBin, err := filepath.Abs(bin)
	if err != nil {
		return err
	}
	cmd := exec.Command(wine, filepath.Base(exe), "-o", absOut, absBin)
	cmd.Dir = filepath.Dir(exe)
	log, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, tail(string(log), 20))
	}
	st, statErr := os.Stat(out)
	if statErr != nil || st.Size() == 0 {
		return fmt.Errorf("ME7Info wrote no .ecu\n%s", tail(string(log), 20))
	}
	return nil
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

type triageRow struct {
	name, status, frac, epk string
}

func (o *onboard) triage(names []string) ([]triageRow, error) {
	var out []triageRow
	oracles := map[string]string{}
	var score []string
	for _, name := range names {
		src := o.ecuPath(name)
		if src == "" {
			out = append(out, triageRow{name: name, status: statusUnscored, frac: "0/0", epk: o.epkOf(name)})
			continue
		}
		dst, err := o.copyOracle(name, src)
		if err != nil {
			return nil, err
		}
		oracles[name] = dst
		score = append(score, name)
	}
	if len(score) > 0 {
		got, err := o.scoreNames(score, oracles)
		if err != nil {
			return nil, err
		}
		for i := range got {
			got[i].epk = o.epkOf(got[i].name)
		}
		out = append(out, got...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].status != out[j].status {
			return statusRank(out[i].status) < statusRank(out[j].status)
		}
		return out[i].name < out[j].name
	})
	var b strings.Builder
	nameW, statusW, fracW := len("name"), len("status"), len("me7info")
	for _, row := range out {
		nameW = max(nameW, len(row.name))
		statusW = max(statusW, len(row.status))
		fracW = max(fracW, len(row.frac))
		fmt.Fprintf(&b, "%s\t%s\t%s\n", row.name, row.status, row.frac)
	}
	if len(out) > 0 {
		fmt.Printf("ME7Info\n%-*s  %-*s  %*s  %s\n", nameW, "name", statusW, "status", fracW, "me7info", "epk")
		for _, row := range out {
			epk := row.epk
			if epk == "" {
				epk = "-"
			}
			fmt.Printf("%-*s  %-*s  %*s  %s\n", nameW, row.name, statusW, row.status, fracW, row.frac, epk)
		}
	}
	if err := os.MkdirAll(o.in, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(o.in, "triage.tsv"), []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	return out, nil
}

func (o *onboard) epkOf(name string) string {
	path, err := o.c.Path(name)
	if err != nil {
		return ""
	}
	img, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return parity.EPK(img)
}

func (o *onboard) promoteReady(names []string, submit bool) error {
	if len(names) == 0 {
		return nil
	}
	scored, err := yamlNames(filepath.Join(o.data, "images.yaml"))
	if err != nil {
		return err
	}
	blocks, err := layoutBlocks(filepath.Join(o.data, "layouts.yaml"))
	if err != nil {
		return err
	}
	nameBlock := map[string]string{}
	for _, n := range scored {
		id, err := o.layoutOf(n)
		if err != nil || id == "" {
			continue
		}
		nameBlock[n] = blocks[id]
	}
	sort.SliceStable(names, func(i, j int) bool {
		return layoutBlock(names[i], scored, nameBlock) != "" && layoutBlock(names[j], scored, nameBlock) == ""
	})
	claimed := map[string]string{}
	var todo []string
	var failed int
	for _, name := range names {
		id, err := o.layoutOf(name)
		if err != nil {
			return err
		}
		if id == "" {
			fmt.Fprintf(os.Stderr, "%s: no layout id\n", name)
			failed++
			continue
		}
		if prev := claimed[id]; prev != "" {
			fmt.Printf("%s: same layout as %s\n", name, prev)
			continue
		}
		if layoutBlock(name, scored, nameBlock) == "" {
			fmt.Fprintf(os.Stderr, "%s: no layout block\n", name)
			failed++
			continue
		}
		claimed[id] = name
		todo = append(todo, name)
	}
	if !submit {
		return nil
	}
	for _, name := range todo {
		if err := o.promote([]string{name}); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			failed++
			continue
		}
		fmt.Printf("added %s\n", name)
	}
	if failed > 0 {
		return fmt.Errorf("%d not added", failed)
	}
	return nil
}

func statusRank(status string) int {
	switch status {
	case statusShort:
		return 0
	case statusEligible:
		return 1
	case statusUnscored:
		return 2
	}
	return 3
}

func layoutBlock(name string, scored []string, nameBlock map[string]string) string {
	return nameBlock[nearestName(name, scored)]
}

func (o *onboard) copyOracle(name, src string) (string, error) {
	b, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(b), "[Measurements]") {
		return "", fmt.Errorf("%s: no [Measurements] section", name)
	}
	dir := filepath.Join(o.in, "oracle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name+".ecu")
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func (o *onboard) scoreNames(names []string, oracles map[string]string) ([]triageRow, error) {
	tmp, err := os.MkdirTemp("", "me7-onboard-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	for _, name := range []string{"names", "layouts.yaml", "layouts-priority.yaml", "datasets.yaml", "confidence.yaml"} {
		if err := os.Symlink(absJoin(o.data, name), filepath.Join(tmp, name)); err != nil {
			return nil, err
		}
	}
	var list strings.Builder
	for _, n := range names {
		fmt.Fprintf(&list, "- %s\n", n)
	}
	if err := os.WriteFile(filepath.Join(tmp, "images.yaml"), []byte(list.String()), 0o644); err != nil {
		return nil, err
	}
	ecu := filepath.Join(tmp, "ecu", "me7info")
	if err := os.MkdirAll(ecu, 0o755); err != nil {
		return nil, err
	}
	for _, n := range names {
		if err := os.Symlink(oracles[n], filepath.Join(ecu, n+".ecu")); err != nil {
			return nil, err
		}
	}
	rep, err := parity.Run(tmp, o.c)
	if err != nil {
		return nil, err
	}
	got := map[string]parity.Fraction{}
	for _, im := range rep.ME7Info {
		got[strings.TrimSuffix(im.Name, ".bin")] = im.Fraction
	}
	var out []triageRow
	for _, n := range names {
		frac, ok := got[n]
		row := triageRow{name: n, status: statusUnscored, frac: "0/0"}
		if ok {
			row.frac = fmt.Sprintf("%d/%d", frac.Hit, frac.Total)
			row.status = statusShort
			if frac.Total > 0 && frac.Hit == frac.Total {
				row.status = statusEligible
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func absJoin(elem ...string) string {
	p := filepath.Join(elem...)
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

type promoteSpec struct {
	name, block, id string
}

func (o *onboard) promote(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("promote wants NAME")
	}
	ready, err := o.readySet()
	if err != nil {
		return err
	}
	scored, err := yamlNames(filepath.Join(o.data, "images.yaml"))
	if err != nil {
		return err
	}
	scoredSet := map[string]struct{}{}
	for _, n := range scored {
		scoredSet[n] = struct{}{}
	}
	blocks, err := layoutBlocks(filepath.Join(o.data, "layouts.yaml"))
	if err != nil {
		return err
	}
	scoredID := map[string]string{}
	nameBlock := map[string]string{}
	for _, n := range scored {
		id, err := o.layoutOf(n)
		if err != nil || id == "" {
			continue
		}
		if _, ok := scoredID[id]; !ok {
			scoredID[id] = n
		}
		nameBlock[n] = blocks[id]
	}
	var specs []promoteSpec
	seen := map[string]string{}
	for _, name := range args {
		if _, ok := scoredSet[name]; ok {
			return fmt.Errorf("%s is already in images.yaml", name)
		}
		if ready[name] != statusEligible {
			return fmt.Errorf("%s is not eligible in triage.tsv", name)
		}
		if o.oracleFile(name) == "" {
			return fmt.Errorf("%s: no oracle .ecu", name)
		}
		id, err := o.layoutOf(name)
		if err != nil {
			return err
		}
		if id == "" {
			return fmt.Errorf("%s: no layout id", name)
		}
		if other := scoredID[id]; other != "" {
			return fmt.Errorf("%s: layout id %s is already scored as %s", name, id, other)
		}
		if prev := seen[id]; prev != "" {
			return fmt.Errorf("%s: layout id %s is shared with %s", name, id, prev)
		}
		seen[id] = name
		block := nameBlock[nearestName(name, scored)]
		if block == "" {
			return fmt.Errorf("%s: no layout block", name)
		}
		specs = append(specs, promoteSpec{name, block, id})
	}
	imgPath := filepath.Join(o.data, "images.yaml")
	layPath := filepath.Join(o.data, "layouts.yaml")
	imgBak, err := os.ReadFile(imgPath)
	if err != nil {
		return err
	}
	layBak, err := os.ReadFile(layPath)
	if err != nil {
		return err
	}
	images := string(imgBak)
	layouts := string(layBak)
	for _, spec := range specs {
		images, err = insertSorted(images, spec.name, "- "+spec.name)
		if err != nil {
			return err
		}
		layouts, err = insertLayout(layouts, spec.block, spec.id)
		if err != nil {
			return err
		}
	}
	changed := false
	var added []string
	restore := func() {
		if !changed {
			return
		}
		_ = os.WriteFile(imgPath, imgBak, 0o644)
		_ = os.WriteFile(layPath, layBak, 0o644)
		for _, p := range added {
			_ = os.Remove(p)
		}
	}
	if err := os.WriteFile(imgPath, []byte(images), 0o644); err != nil {
		return err
	}
	changed = true
	if err := os.WriteFile(layPath, []byte(layouts), 0o644); err != nil {
		restore()
		return err
	}
	golden := filepath.Join(o.data, "ecu", "me7info")
	for _, spec := range specs {
		dst := filepath.Join(golden, spec.name+".ecu")
		if err := copyFile(dst, o.oracleFile(spec.name)); err != nil {
			restore()
			return err
		}
		added = append(added, dst)
	}
	rep, err := parity.Run(o.data, o.c)
	if err != nil {
		restore()
		return err
	}
	if miss := me7Misses(rep, goldens(o.data, scored), specs); len(miss) > 0 {
		restore()
		return fmt.Errorf("ME7Info parity:\n%s", strings.Join(miss, "\n"))
	}
	return nil
}

func (o *onboard) readySet() (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(o.in, "triage.tsv"))
	if err != nil {
		return nil, fmt.Errorf("triage.tsv: %w", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) >= 2 {
			out[f[0]] = f[1]
		}
	}
	return out, nil
}

func (o *onboard) oracleFile(name string) string {
	p := filepath.Join(o.in, "oracle", name+".ecu")
	if st, err := os.Stat(p); err == nil && st.Size() > 0 {
		return p
	}
	return ""
}

// goldens is the images.yaml names that already have a ME7Info file.
// A row with no golden is not a score, so a later add does not fail on it.
func goldens(dir string, names []string) []string {
	var out []string
	for _, n := range names {
		st, err := os.Stat(filepath.Join(dir, "ecu", "me7info", n+".ecu"))
		if err == nil && st.Size() > 0 {
			out = append(out, n)
		}
	}
	return out
}

func me7Misses(rep *parity.Report, prev []string, specs []promoteSpec) []string {
	got := map[string]parity.Fraction{}
	for _, im := range rep.ME7Info {
		got[strings.TrimSuffix(im.Name, ".bin")] = im.Fraction
	}
	var miss []string
	for _, n := range prev {
		frac, ok := got[n]
		if !ok {
			miss = append(miss, n+" missing")
			continue
		}
		if frac.Hit != frac.Total {
			miss = append(miss, fmt.Sprintf("%s %d/%d", n, frac.Hit, frac.Total))
		}
	}
	for _, spec := range specs {
		frac, ok := got[spec.name]
		if !ok {
			miss = append(miss, spec.name+" unscored")
			continue
		}
		if frac.Total == 0 || frac.Hit != frac.Total {
			miss = append(miss, fmt.Sprintf("%s %d/%d", spec.name, frac.Hit, frac.Total))
		}
	}
	return miss
}

func nearestName(name string, scored []string) string {
	best, n := "", 0
	for _, s := range scored {
		k := 0
		for k < len(name) && k < len(s) && name[k] == s[k] {
			k++
		}
		if k > n {
			n, best = k, s
		}
	}
	return best
}

func yamlNames(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(b), "\n") {
		if key, ok := listKey(line); ok {
			names = append(names, key)
		}
	}
	return names, nil
}

func listKey(line string) (string, bool) {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "- ") {
		return "", false
	}
	s = strings.Trim(strings.TrimSpace(s[2:]), `"`)
	return s, s != ""
}

func insertSorted(text, key, line string) (string, error) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	at := len(lines)
	for i, ln := range lines {
		k, ok := listKey(ln)
		if !ok {
			continue
		}
		if k == key {
			return "", fmt.Errorf("%s already listed", key)
		}
		if k > key {
			at = i
			for at > 0 && strings.HasPrefix(strings.TrimSpace(lines[at-1]), "#") {
				at--
			}
			// A comment above the first entry is the file header.
			if at == 0 {
				at = i
			}
			break
		}
	}
	return splice(lines, at, line), nil
}

func insertLayout(text, block, id string) (string, error) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	start := -1
	for i, ln := range lines {
		if ln == block+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("no layout block %s", block)
	}
	end := start + 1
	for end < len(lines) && strings.HasPrefix(lines[end], "  - ") {
		end++
	}
	line := "  - " + strconv.Quote(id)
	at := end
	for i := start + 1; i < end; i++ {
		k, ok := listKey(lines[i])
		if !ok {
			continue
		}
		if k == id {
			return "", fmt.Errorf("%s already in %s", id, block)
		}
		if k > id {
			at = i
			break
		}
	}
	return splice(lines, at, line), nil
}

func splice(lines []string, at int, line string) string {
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, line)
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n") + "\n"
}

// layoutBlocks maps a layout id to its block label.
func layoutBlocks(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	block := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "  - ") {
			if id, ok := listKey(line); ok && block != "" {
				out[id] = block
			}
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#") {
			block = strings.TrimSuffix(line, ":")
		}
	}
	return out, nil
}

func copyFile(dst, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
