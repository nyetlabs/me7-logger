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
candidates lists corpus images missing from images.yaml: staged when
incoming/ecus already has an .ecu, needs-ME7Info when ME7Info.exe still
has to run, unsupported when the image is not 512 KiB or 1 MiB. The
layout id is the Bosch software number, or the EPK string. dup names a
scored image that already has that id.

run writes incoming/ecus/NAME.ecu with ME7Info.exe. ME7INFO_EXE defaults
to ../ME7L/bin/ME7Info.exe and WINE defaults to wine. An existing file
is left in place.

triage copies those files to incoming/oracle and scores them. ready in
incoming/triage.tsv means every ME7Info row is 100%.

promote NAME[=BLOCK] inserts the image and its layout id, copies the
oracle to ecu/me7info, and scores. BLOCK defaults to the scored image
with the longest shared name prefix. A layout id already on a scored
image is refused. Any ME7Info row under 100% restores both YAML files
and the new golden.

With no action, onboard runs candidates, then run, then triage.
`

const (
	statusStaged      = "staged"
	statusNeeds       = "needs-ME7Info"
	statusUnsupported = "unsupported"
	statusReady       = "ready"
	statusShort       = "short"
	statusUnscored    = "unscored"
)

func cmdOnboard(args []string) error {
	action := "all"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action, args = args[0], args[1:]
	}
	switch action {
	case "all", "candidates", "run", "triage", "promote":
	default:
		return fmt.Errorf("onboard %s: want candidates, run, triage, or promote", action)
	}
	fs := cli.NewFlagSet("me7parity", "onboard", "[candidates|run|triage|promote] [flags] [NAME...]", onboardHelp)
	data := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`")
	incoming := fs.String("incoming", "", "incoming `<dir>` (default <data>/incoming)")
	corpus := fs.String("corpus", ecucorpus.Dir("."), "ecu-corpus checkout `<dir>`")
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
	switch action {
	case "candidates":
		_, err := o.printCandidates()
		return err
	case "run":
		return o.run(fs.Args())
	case "triage":
		return o.triage(fs.Args())
	case "promote":
		return o.promote(fs.Args())
	default:
		cands, err := o.printCandidates()
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
		}
		return o.triage(nil)
	}
}

type onboard struct {
	data string
	in   string
	c    *ecucorpus.Corpus
}

type candidate struct {
	name, status, id, note string
}

func (row candidate) String() string {
	id := row.id
	if id == "" {
		id = "-"
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s", row.name, row.status, id, row.note)
}

func (o *onboard) printCandidates() ([]candidate, error) {
	rows, err := o.candidates()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		fmt.Println(row)
	}
	return rows, nil
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
		row.status = statusStaged
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
	if len(names) == 0 {
		rows, err := o.candidates()
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.status == statusNeeds {
				names = append(names, row.name)
			}
		}
	}
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
	if statErr != nil || st.Size() == 0 || !strings.Contains(string(log), "success") {
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
	name, status, frac string
}

func (o *onboard) triage(names []string) error {
	if len(names) == 0 {
		rows, err := o.candidates()
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.status == statusStaged && !strings.Contains(row.note, "dup:") {
				names = append(names, row.name)
			}
		}
	}
	var out []triageRow
	oracles := map[string]string{}
	var score []string
	for _, name := range names {
		src := o.ecuPath(name)
		if src == "" {
			out = append(out, triageRow{name, statusUnscored, "0/0"})
			continue
		}
		dst, err := o.copyOracle(name, src)
		if err != nil {
			return err
		}
		oracles[name] = dst
		score = append(score, name)
	}
	if len(score) > 0 {
		got, err := o.scoreNames(score, oracles)
		if err != nil {
			return err
		}
		out = append(out, got...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].status != out[j].status {
			return out[i].status < out[j].status
		}
		return out[i].name < out[j].name
	})
	var b strings.Builder
	var ready []string
	for _, row := range out {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", row.name, row.status, row.frac)
		fmt.Printf("%s\t%s\t%s\n", row.name, row.status, row.frac)
		if row.status == statusReady {
			ready = append(ready, row.name)
		}
	}
	if err := os.MkdirAll(o.in, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(o.in, "triage.tsv"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	if len(ready) > 0 {
		fmt.Printf("ready %d: me7parity onboard promote %s\n", len(ready), strings.Join(ready, " "))
	}
	return nil
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
				row.status = statusReady
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
		return fmt.Errorf("promote wants NAME or NAME=BLOCK")
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
	for _, arg := range args {
		name, block, _ := strings.Cut(arg, "=")
		if _, ok := scoredSet[name]; ok {
			return fmt.Errorf("%s is already in images.yaml", name)
		}
		if ready[name] != statusReady {
			return fmt.Errorf("%s is not ready in triage.tsv", name)
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
		if block == "" {
			near := nearestName(name, scored)
			block = nameBlock[near]
		}
		if block == "" {
			return fmt.Errorf("%s: no layout block; pass NAME=BLOCK", name)
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
	if miss := me7Misses(rep, scored, specs); len(miss) > 0 {
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
