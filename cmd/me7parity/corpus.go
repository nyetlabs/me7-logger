package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
	"gopkg.in/yaml.v3"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/internal/locate"
	"go.nyet.org/me7-logger/parity"
)

const corpusHelp = `
Lists every parity image and what --submit would store.
--submit writes those rows and does not commit or upload.
An identical row is stored when its stamp digests differ. stamp.tool
is not one of those digests, so a tool name or version does not count.
Origin stays.
With no NAME, every image is listed. A named row also prints
manual, open, and block.

Columns are status, tier, delta, and confidence.
Status and delta marks are in docs/parity.md.
--grades prints name, stage, confidence, and source, and does not submit.
--staged reads each zip's definition instead of locating maps again.
Stage is the tuner tier: D is 1, C is 2, B is 3-, A is 3, S is 3+.
An open D is -. Source is verified, DAMOS, matched, manual, or auto
(docs/parity.md).

The corpus is --corpus, else CORPUS, else ../ecu-corpus when that
checkout exists, else ecu-corpus/.
` + nameArg

func cmdCorpus(args []string) error {
	fs := cli.NewFlagSet("me7parity", "corpus", "[--submit] [NAME...]", corpusHelp)
	data := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`")
	corpusFlag := fs.String("corpus", "", "ecu-corpus checkout `<dir>` (default CORPUS, else ../ecu-corpus, else ecu-corpus/)")
	submit := fs.Bool("submit", false, "write the rows marked for storage into the corpus")
	grades := fs.Bool("grades", false, "print name, stage, and confidence")
	staged := fs.String("staged", "", "grade each zip's definition in this `<dir>`")
	d := locate.Flags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := ecucorpus.ModuleRoot()
	if err != nil {
		return err
	}
	dir := corpusDir(root, *corpusFlag)
	c, err := ecucorpus.Load(dir)
	if err != nil {
		return err
	}
	if *staged != "" {
		if !*grades {
			return fmt.Errorf("--staged is for --grades")
		}
		if *submit {
			return fmt.Errorf("--grades does not submit")
		}
		paths, err := c.List(filepath.Join(*data, "images.yaml"))
		if err != nil {
			return err
		}
		var stems []string
		for _, p := range paths {
			stems = append(stems, strings.TrimSuffix(filepath.Base(p), ".bin"))
		}
		selected, err := matchStems(stems, fs.Args())
		if err != nil {
			return err
		}
		inSet := map[string]bool{}
		for _, s := range stems {
			inSet[s] = true
		}
		want := map[string]bool{}
		if len(fs.Args()) > 0 {
			for _, s := range selected {
				want[s] = true
			}
		}
		return gradeStaged(*data, *staged, c, inSet, want)
	}
	rep, err := parity.Run(*data, c)
	if err != nil {
		return err
	}
	var userDir string
	var cats *model.CategoryTable
	prepare := func() (string, *model.CategoryTable, error) {
		if cats != nil {
			return userDir, cats, nil
		}
		var err error
		if userDir, err = locate.UserDir(fs, *d.User); err != nil {
			return "", nil, err
		}
		if cats, err = config.LoadCategories(""); err != nil {
			return "", nil, err
		}
		return userDir, cats, nil
	}
	selected, err := matchStems(reportStems(rep), fs.Args())
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, s := range selected {
		want[s] = true
	}
	if *grades {
		if *submit {
			return fmt.Errorf("--grades does not submit")
		}
		return printGrades(rep, want)
	}
	refs, err := loadRefTable(*data)
	if err != nil {
		return err
	}
	if _, err := mapSets(); err != nil {
		return err
	}
	var eligible []pubItem
	var lines []pubLine
	for _, im := range rep.Tuner {
		name := strings.TrimSuffix(im.Name, ".bin")
		if len(want) > 0 && !want[name] {
			continue
		}
		item := pubItem{name: name, tier: im.Tier, def: c.Def(name), conf: im.ConfByName}
		if item.tier == "" {
			item.tier = "-"
		}
		var prev *model.Model
		nKey := 0
		if item.def != "" {
			prev, err = readModel(item.def)
			if err != nil {
				item.reason = "def unreadable"
			} else if prev.Provenance != nil {
				item.origin = prev.Provenance.Origin
				if prev.Provenance.Verified {
					item.reason = "verified"
				}
			}
			nKey = placeholderCount(prev)
		}
		if item.reason == "" {
			item.reason = gateReason(im.Tier, im.Confidence, im.ConfByName, item.def != "", item.origin)
		}
		delta := "-"
		if item.reason == "" && (item.origin == "manual" || item.origin == "auto") {
			loc, err := located(d, prepare, c, name)
			if err != nil {
				return err
			}
			reason, added, notes := coverAll(prev, loc)
			item.reason = reason
			item.notes = notes
			item.ref = reference(prev, loc, refs)
			moves := moveCount(notes)
			switch {
			case reason == "" || reason == "identical":
				item.merged, err = mergeKept(prev, loc)
			case moves > 0 && !hasShort(notes):
				moved := applyMoves(prev, loc, notes)
				item.merged, err = mergeKept(moved, loc)
				if err != nil {
					return err
				}
				reason, added, _ = coverAll(moved, loc)
				item.reason = reason
			case hasShort(notes) && (moves > 0 || added > 0):
				item.merged = applyMoves(prev, loc, notes)
				if isAuto(prev) {
					relayout(item.merged, loc)
				}
				item.keepOrigin = true
				delta = partialDelta(moves, added)
				item.reason = reason + ", " + delta
			}
			if err != nil {
				return err
			}
			if item.merged != nil {
				clearPlaceholders(item.merged)
				annotateConfidence(item.merged, item.conf)
				if isAuto(item.merged) {
					config.ApplyScales(item.merged)
				}
			}
			switch reason {
			case "":
				delta = fmt.Sprintf("+%d", added)
			case "identical":
				n := corrections(prev, item.merged)
				if moves > 0 {
					n = moves
				}
				if n > 0 {
					item.reason = ""
					delta = fmt.Sprintf("~%d", n)
				}
			}
			if item.reason != "" && !item.keepOrigin && isAuto(prev) {
				r := relayout(clearedCopy(prev), loc)
				annotateConfidence(r, item.conf)
				config.ApplyScales(r)
				if n := corrections(prev, r); n > 0 {
					item.merged, item.keepOrigin = r, true
					delta = fmt.Sprintf("layout %d", n)
					item.reason += ", " + delta
				}
			}
		}
		if item.reason == "verified" {
			loc, err := located(d, prepare, c, name)
			if err != nil {
				return err
			}
			merged, added := verifiedAdd(prev, loc)
			if added > 0 {
				annotateConfidence(merged, item.conf)
				item.merged = merged
				item.keepOrigin = true
				delta = fmt.Sprintf("+%d", added)
				item.reason = "verified, " + delta
			}
		}
		if nKey > 0 && item.reason != "" && item.reason != "def unreadable" {
			if item.merged == nil {
				item.merged = clearedCopy(prev)
			}
			item.keepOrigin = true
			if item.reason == "identical" || strings.HasPrefix(item.reason, "origin ") {
				item.reason = "keys"
			} else {
				item.reason = fmt.Sprintf("%s, key %d", item.reason, nKey)
			}
			if delta == "-" {
				delta = fmt.Sprintf("key %d", nKey)
			}
		} else if nKey > 0 && item.reason == "" {
			if delta == "-" {
				delta = fmt.Sprintf("key %d", nKey)
			} else {
				delta += fmt.Sprintf(" key %d", nKey)
			}
		}
		var lintText string
		if item.reason == "" && item.merged == nil && item.def == "" {
			loc, err := located(d, prepare, c, name)
			if err != nil {
				return err
			}
			item.merged = loc
		}
		if item.reason == "" && item.merged != nil && item.def == "" {
			annotateConfidence(item.merged, item.conf)
		}
		if err := applyForce(&item, &delta); err != nil {
			return err
		}
		if item.merged != nil && (item.reason == "" || item.keepOrigin) {
			block, text, err := holdLint(c, name, item.merged)
			if err != nil {
				return err
			}
			lintText = text
			if block != "" {
				item.reason = block
				item.keepOrigin = false
			}
		}
		status := item.reason
		if status == "" {
			status = "eligible"
			eligible = append(eligible, item)
		} else if item.keepOrigin {
			eligible = append(eligible, item)
		}
		tier := item.tier
		if im.NoDef {
			tier += "*"
		}
		lines = append(lines, pubLine{
			name: name, status: status, tier: tier, delta: delta,
			hit: im.Confidence.Hit, total: im.Confidence.Total,
			notes: item.notes, open: im.Open, block: im.BlockOpen,
			lint: lintText,
		})
	}
	extra, err := placeholderDefs(dir, lines)
	if err != nil {
		return err
	}
	if len(want) > 0 {
		extra = placeholderExtra{}
	}
	for i := range extra.items {
		block, text, err := holdLint(c, extra.items[i].name, extra.items[i].merged)
		if err != nil {
			return err
		}
		extra.lines[i].lint = text
		if block != "" {
			extra.items[i].reason = block
			extra.items[i].keepOrigin = false
			extra.lines[i].status = block
			continue
		}
		eligible = append(eligible, extra.items[i])
	}
	lines = append(lines, extra.lines...)
	printPublish(lines)
	if len(want) > 0 {
		printDetail(lines)
	}
	if !*submit {
		return nil
	}
	for _, item := range eligible {
		dst := filepath.Join(dir, "defs", item.name+".json")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		switch {
		case item.merged != nil:
			annotateConfidence(item.merged, item.conf)
			if err := writePublished(dst, item.merged, item.keepOrigin, item.ref); err != nil {
				return err
			}
		case item.def != "":
			return fmt.Errorf("%s: eligible without a merged model", item.name)
		default:
			imgPath, err := c.Path(item.name)
			if err != nil {
				return err
			}
			img, err := os.ReadFile(imgPath)
			if err != nil {
				return err
			}
			userDir, cats, err := prepare()
			if err != nil {
				return err
			}
			res, err := d.Generate(imgPath, img, userDir, 0, "auto", "")
			if err != nil {
				return err
			}
			m, _, err := locate.Model(imgPath, img, res.Maps, cats, false)
			if err != nil {
				return err
			}
			if m == nil || len(m.Objects) == 0 {
				return fmt.Errorf("%s: no located maps", item.name)
			}
			annotateConfidence(m, item.conf)
			if err := writePublished(dst, m, false, nil); err != nil {
				return err
			}
		}
		if err := setCorpusDef(dir, item.name, "defs/"+item.name+".json"); err != nil {
			return err
		}
	}
	return nil
}

type pubItem struct {
	name, tier, origin, reason, def string
	merged                          *model.Model
	ref                             *model.Reference
	keepOrigin                      bool
	notes                           []coverNote
	conf                            map[string]parity.Conf
}

type pubLine struct {
	name, status, tier, delta string
	hit, total                int
	notes                     []coverNote
	open, block               []parity.OpenName
	lint                      string
}

var zipStem = regexp.MustCompile(`^(.+)-[0-9]{8}\.zip$`)

// gradeStaged scores the definition stored in each staged zip.
func gradeStaged(data, stage string, c *ecucorpus.Corpus, inSet, want map[string]bool) error {
	matches, err := filepath.Glob(filepath.Join(stage, "*.zip"))
	if err != nil {
		return err
	}
	seen := map[string]string{}
	var packed []parity.Packed
	for _, path := range matches {
		m := zipStem.FindStringSubmatch(filepath.Base(path))
		if m == nil {
			continue
		}
		stem := m[1]
		if !inSet[stem] || len(want) > 0 && !want[stem] {
			continue
		}
		if prev, ok := seen[stem]; ok {
			return fmt.Errorf("%s: also %s", filepath.Base(path), filepath.Base(prev))
		}
		seen[stem] = path
		def, err := zipDef(path, stem)
		if err != nil {
			return err
		}
		imgPath, err := c.Path(stem)
		if err != nil {
			return err
		}
		img, err := os.ReadFile(imgPath)
		if err != nil {
			return err
		}
		packed = append(packed, parity.Packed{Stem: stem, Img: img, Def: def})
	}
	rows, err := parity.GradePacked(data, packed)
	if err != nil {
		return err
	}
	return printGrades(&parity.Report{Tuner: rows}, nil)
}

func zipDef(path, stem string) ([]byte, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	name := stem + ".json"
	for _, f := range z.File {
		if f.Name != name {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, fmt.Errorf("%s: no %s", filepath.Base(path), name)
}

func printGrades(rep *parity.Report, want map[string]bool) error {
	for _, im := range rep.Tuner {
		name := strings.TrimSuffix(im.Name, ".bin")
		if len(want) > 0 && !want[name] {
			continue
		}
		fmt.Printf("%s\t%s\t%d/%d\t%s\n", name, stageLabel(im.Tier), im.Confidence.Hit, im.Confidence.Total, sourceClass(im.Provenance))
	}
	return nil
}

// stageLabel is the publish column for a tuner grade. D through A are the
// tuning stages. S is 3+, the list past those stages. An open D is -.
func stageLabel(tier string) string {
	return map[string]string{"": "-", "-": "-", "D": "1", "C": "2", "B": "3-", "A": "3", "S": "3+", "S+": "S+"}[tier]
}

func printPublish(lines []pubLine) {
	nameW, statusW, deltaW := len("name"), len("status"), len("delta")
	confW := len("confidence")
	confs := make([]string, len(lines))
	for i, ln := range lines {
		nameW = max(nameW, len(ln.name))
		statusW = max(statusW, len(ln.status))
		deltaW = max(deltaW, len(ln.delta))
		if ln.total < 0 {
			confs[i] = "-"
		} else {
			confs[i] = fmt.Sprintf("%d/%d", ln.hit, ln.total)
		}
		confW = max(confW, len(confs[i]))
	}
	fmt.Printf("%-*s  %-*s  %-4s  %-*s  %*s\n",
		nameW, "name", statusW, "status", "tier", deltaW, "delta", confW, "confidence")
	for i, ln := range lines {
		fmt.Printf("%-*s  %-*s  %-4s  %-*s  %*s\n",
			nameW, ln.name, statusW, ln.status, ln.tier, deltaW, ln.delta, confW, confs[i])
	}
}

// printDetail lists the manual gaps, the tuner names that did not hit, and
// the lint findings on the definition that would be stored.
func printDetail(lines []pubLine) {
	for _, ln := range lines {
		if len(ln.notes) == 0 && len(ln.open) == 0 && len(ln.block) == 0 && ln.lint == "" {
			continue
		}
		fmt.Printf("\n%s\n", ln.name)
		if len(ln.notes) > 0 {
			fmt.Println("manual")
			for _, n := range ln.notes {
				fmt.Printf("  %s\n", n.line())
			}
		}
		if len(ln.open) > 0 {
			fmt.Println("open")
			for _, n := range ln.open {
				fmt.Printf("  %s\n", openLine(n))
			}
		}
		if len(ln.block) > 0 {
			fmt.Println("block")
			for _, n := range ln.block {
				fmt.Printf("  %s\n", openLine(n))
			}
		}
		if ln.lint != "" {
			fmt.Println("lint")
			fmt.Print(ln.lint)
		}
	}
}

func openLine(n parity.OpenName) string {
	tier := n.Tier
	if tier == "" {
		tier = "-"
	}
	return fmt.Sprintf("%-4s %s%s", tier, n.Name, n.Note())
}

// gateReason is empty when the image may be submitted, or when origin
// manual or auto still has to pass coverAll. Hard fails are a turned
// non-zero axis (axis NAME) or an unreadable body (body NAME). Soft low
// confidence (shape, all-zero axis, zero body, peer miss) still uses the
// Hit/Total gate. Grade "-" passes only for manual. damos, a2l, and an
// unset origin do not.
func gateReason(tier string, conf parity.Fraction, byName map[string]parity.Conf, hasDef bool, origin string) string {
	if status := hardConf(byName); status != "" {
		return status
	}
	if conf.Total == 0 || conf.Hit*2 <= conf.Total {
		return fmt.Sprintf("confidence %d/%d", conf.Hit, conf.Total)
	}
	if hasDef && origin != "manual" && origin != "auto" {
		if origin == "" {
			origin = "unset"
		}
		return "origin " + origin
	}
	if origin == "manual" {
		return ""
	}
	switch tier {
	case "D", "C", "B", "A", "S", "S+":
		return ""
	}
	if tier == "" {
		tier = "-"
	}
	return "grade " + tier
}

// hardConf is "axis NAME" or "body NAME" for the first hard tuner fail.
// Axis is checked before body. Soft reasons (shape, axis-zeros, zeros,
// peer-*) are not hard.
func hardConf(byName map[string]parity.Conf) string {
	for _, reason := range []string{"axis", "body"} {
		if name := hardReason(byName, reason); name != "" {
			return reason + " " + name
		}
	}
	return ""
}

func hardReason(byName map[string]parity.Conf, reason string) string {
	var names []string
	for n, c := range byName {
		if c.Reason == reason {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	slices.Sort(names)
	return names[0]
}

// coverNote is one manual key the detail list prints.
type coverNote struct {
	op     string
	key    string
	manual model.Addr
	have   []model.Addr
}

func (n coverNote) line() string {
	switch n.op {
	case "short":
		return fmt.Sprintf("short %s %s", n.key, addrText(n.manual))
	case "disagrees":
		return fmt.Sprintf("disagrees %s manual %s located %s", n.key, addrText(n.manual), addrsText(n.have))
	default:
		return fmt.Sprintf("added %s %s", n.key, addrsText(n.have))
	}
}

func addrText(a model.Addr) string {
	return fmt.Sprintf("0x%X", uint32(a))
}

func addrsText(as []model.Addr) string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = addrText(a)
	}
	return strings.Join(out, ",")
}

// coverAll's reason is empty when located strictly covers prev, and the
// count is how many names are new. A name the locator does not have is short.
// The same name at another address disagrees. An equal set of names is
// identical. --submit still stores that set when a description or comment
// would change. Both KFPED variants cover a manual KFPED, at the _0_A
// address. The same for KFPEDR. One variant does not, and a located KFPED
// does not cover a manual KFPED_0_A.
// The notes are every short, disagreeing, and added key. The reason is
// the first short or disagree. Added keys are listed and counted even when
// a short or disagree blocks a full replace.
func coverAll(prev, located *model.Model) (string, int, []coverNote) {
	have := map[string][]model.Addr{}
	for _, o := range located.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		have[k] = append(have[k], o.Address)
	}
	prevKey := map[string]struct{}{}
	var reason string
	var notes []coverNote
	for _, o := range prev.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		prevKey[k] = struct{}{}
		addrs := pedalAddrs(k, have)
		if len(addrs) == 0 {
			notes = append(notes, coverNote{op: "short", key: k, manual: o.Address})
			if reason == "" {
				reason = "manual short " + k
			}
			continue
		}
		match := false
		for _, a := range addrs {
			if a == o.Address {
				match = true
				break
			}
		}
		if !match {
			notes = append(notes, coverNote{op: "disagrees", key: k, manual: o.Address, have: addrs})
			if reason == "" {
				reason = "manual disagrees " + k
			}
		}
	}
	added := 0
	seen := map[string]struct{}{}
	for _, o := range located.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		if _, ok := prevKey[k]; ok {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		notes = append(notes, coverNote{op: "added", key: k, have: have[k]})
		added++
	}
	if reason != "" {
		return reason, added, notes
	}
	if added == 0 {
		return "identical", 0, notes
	}
	return "", added, notes
}

// moveCount is the disagreeing names with one located address. Several
// addresses are left for the report.
func moveCount(notes []coverNote) int {
	n := 0
	for _, note := range notes {
		if note.op == "disagrees" && len(note.have) == 1 {
			n++
		}
	}
	return n
}

func hasShort(notes []coverNote) bool {
	for _, n := range notes {
		if n.op == "short" {
			return true
		}
	}
	return false
}

// partialDelta is the address corrections and the located names the manual
// file does not have. Either count may be zero.
func partialDelta(moves, added int) string {
	var b string
	if moves > 0 {
		b = fmt.Sprintf("~%d", moves)
	}
	if added > 0 {
		if b != "" {
			b += " "
		}
		b += fmt.Sprintf("+%d", added)
	}
	if b == "" {
		return "-"
	}
	return b
}

// applyMoves copies prev, writes each single located address onto that
// name, and appends each added map. Conversion, units, and layout stay
// on a name the manual file already has. The description and comment come
// from the located object of the same key. A short key is unchanged.
func applyMoves(prev, located *model.Model, notes []coverNote) *model.Model {
	out := clearedCopy(prev)
	byKey := map[string]*model.Object{}
	if located != nil {
		for _, o := range located.Objects {
			if k := objKey(o); k != "" && byKey[k] == nil {
				byKey[k] = o
			}
		}
	}
	for _, n := range notes {
		if n.op != "disagrees" || len(n.have) != 1 {
			continue
		}
		for _, o := range out.Objects {
			if objKey(o) != n.key {
				continue
			}
			o.Address = n.have[0]
			if src := byKey[n.key]; src != nil {
				o.Description = src.Description
				o.Comment = src.Comment
			}
		}
	}
	for _, n := range notes {
		if n.op != "added" {
			continue
		}
		src := byKey[n.key]
		if src == nil {
			continue
		}
		c := *src
		out.Objects = append(out.Objects, &c)
	}
	return out
}

// verifiedAdd appends located names the verified file does not have.
// Addresses, conversions, and comments already in the file stay.
func verifiedAdd(prev, located *model.Model) (*model.Model, int) {
	_, added, notes := coverAll(prev, located)
	if added == 0 {
		return nil, 0
	}
	var add []coverNote
	for _, n := range notes {
		if n.op == "added" {
			add = append(add, n)
		}
	}
	out := applyMoves(prev, located, add)
	for _, o := range out.Objects[len(prev.Objects):] {
		o.Categories = adoptCats(out, located, o.Categories)
	}
	return out, added
}

// adoptCats files ids from src under the same names in dst. A name dst
// does not have is added. The two files number their categories apart.
func adoptCats(dst, src *model.Model, ids []int) []int {
	var out []int
	if src == nil {
		return out
	}
	for _, id := range ids {
		if id < 0 || id >= len(src.Categories) || src.Categories[id] == nil {
			continue
		}
		out = append(out, ensureCat(dst, src.Categories[id].Name))
	}
	return out
}

func ensureCat(m *model.Model, name string) int {
	for _, c := range m.Categories {
		if c != nil && c.Name == name {
			return c.ID
		}
	}
	id := 0
	for _, c := range m.Categories {
		if c != nil && c.ID >= id {
			id = c.ID + 1
		}
	}
	m.Categories = append(m.Categories, &model.Category{ID: id, Name: name})
	return id
}

// corrections is how many previous objects change description, comment,
// or layout in merged. Address is part of the match, so a moved name is
// not counted. Copies of one key pair in order, so one copy is not
// compared with another.
func corrections(prev, merged *model.Model) int {
	if prev == nil || merged == nil {
		return 0
	}
	next := map[keyAddr][]*model.Object{}
	for _, o := range merged.Objects {
		if k := objKey(o); k != "" {
			ka := keyAddr{k, o.Address}
			next[ka] = append(next[ka], o)
		}
	}
	used := map[keyAddr]int{}
	n := 0
	for _, o := range prev.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		ka := keyAddr{k, o.Address}
		list := next[ka]
		i := used[ka]
		if i >= len(list) {
			continue
		}
		used[ka] = i + 1
		m := list[i]
		if m.Description != o.Description || m.Comment != o.Comment || !sameLayout(m, o) {
			n++
		}
	}
	return n
}

// mergeKept folds the located model into the previous pack. Conversion,
// units, and layout stay on the previous object. The description comes from
// the located model. On an auto pack the comment is replaced too; on a
// manual pack an existing comment is kept. When both pedal variants are
// located, the plain name is dropped and the variants take its category. A
// variant already in the pack keeps its conversion. In an auto pack, an
// object still in raw located form takes the located layout.
func mergeKept(prev, located *model.Model) (*model.Model, error) {
	drop, stamp := pedalPlan(prev, located)
	base := withoutKeys(prev, drop)
	comment := "keep"
	if isAuto(prev) {
		comment = "replace"
	}
	out, _, err := model.Merge(base, located, model.MergeOptions{
		Description: "replace",
		Comment:     comment,
	})
	if err != nil {
		return nil, err
	}
	if isAuto(prev) {
		relayout(out, located)
	}
	for _, o := range out.Objects {
		cats, ok := stamp[objKey(o)]
		if !ok {
			continue
		}
		o.Categories = append([]int(nil), cats...)
	}
	return out, nil
}

func isAuto(m *model.Model) bool {
	return m != nil && m.Provenance != nil && m.Provenance.Origin == "auto"
}

// annotateConfidence notes low body confidence, a missing axis, and a value
// that stayed factor 1 with no units. High confidence keeps the needle, then
// the gap notes. A name without a Conf still gets the gap notes. A bitmap
// stays raw and does not get a no-scale note.
func annotateConfidence(m *model.Model, byName map[string]parity.Conf) {
	if m == nil {
		return
	}
	for _, o := range m.Objects {
		if o == nil {
			continue
		}
		c := byName[objKey(o)]
		if !rawValue(o) {
			c = parity.Conf{}
		}
		o.Comment = parity.JoinComment(o.Comment, c, gapNotes(o))
	}
}

// gapNotes is a missing image axis, or a cell or image axis left at factor 1
// with no units. An ordinal axis is not a missing scale. A codeword, a
// configuration byte, or an error class is raw on purpose.
func gapNotes(o *model.Object) []string {
	var g []string
	for _, ax := range []struct {
		w string
		a *model.Axis
		n bool
	}{
		{"x", o.X, o.Shape == "1d" || o.Shape == "2d"},
		{"y", o.Y, o.Shape == "2d"},
	} {
		if ax.n && (ax.a == nil || ax.a.Source == "" || ax.a.Source == "unknown") {
			g = append(g, "missing "+ax.w+" axis")
		}
		if ax.a != nil && ax.a.Source == "image" && rawScale(ax.a.Value) {
			g = append(g, "no "+ax.w+" scale")
		}
	}
	if rawValue(o) && !config.Bitmap(objKey(o)) {
		g = append(g, "no scale")
	}
	return g
}

// relayout gives each object of out that has a raw value the shape,
// storage, and axes of the located object with its key and address.
func relayout(out, located *model.Model) *model.Model {
	at := byKeyAddr(located)
	for _, o := range out.Objects {
		if src := at[keyAddr{objKey(o), o.Address}]; src != nil && rawValue(o) {
			o.Shape, o.Rows, o.Cols, o.Data, o.X, o.Y = src.Shape, src.Rows, src.Cols, src.Data, src.X, src.Y
		}
	}
	return out
}

type keyAddr struct {
	key  string
	addr model.Addr
}

func byKeyAddr(m *model.Model) map[keyAddr]*model.Object {
	out := map[keyAddr]*model.Object{}
	if m == nil {
		return out
	}
	for _, o := range m.Objects {
		if k := objKey(o); k != "" {
			out[keyAddr{k, o.Address}] = o
		}
	}
	return out
}

// rawValue is an object with the located value: factor 1, no offset, no units.
func rawValue(o *model.Object) bool {
	return o != nil && rawScale(o.Value)
}

func rawScale(v model.Value) bool {
	c := v.Conversion
	return c.Factor == 1 && c.Offset == 0 && !c.Reciprocal && v.Units == ""
}

// sameLayout is the same shape, storage, and axes.
func sameLayout(a, b *model.Object) bool {
	return a.Shape == b.Shape && a.Rows == b.Rows && a.Cols == b.Cols && a.Data == b.Data &&
		reflect.DeepEqual(a.X, b.X) && reflect.DeepEqual(a.Y, b.Y)
}

// setIndex is config/sets.yaml. peers is an either name and its other names.
// both is the sets whose later names together cover the plain name.
type setIndex struct {
	peers map[string][]string
	both  []config.Set
}

var (
	setOnce sync.Once
	setIdx  setIndex
	setErr  error
)

func mapSets() (setIndex, error) {
	setOnce.Do(func() { setIdx, setErr = loadSetIndex() })
	return setIdx, setErr
}

func loadSetIndex() (setIndex, error) {
	all, err := config.Sets()
	if err != nil {
		return setIndex{}, err
	}
	idx := setIndex{peers: map[string][]string{}}
	for _, s := range all {
		switch s.Rule {
		case config.RuleEither:
			for _, n := range s.Names {
				var others []string
				for _, o := range s.Names {
					if o != n {
						others = append(others, o)
					}
				}
				idx.peers[n] = others
			}
		case config.RuleBoth:
			idx.both = append(idx.both, s)
		}
	}
	return idx, nil
}

// pedalAddrs is the addresses that cover a manual key. Every later name of a
// both set covers the plain name at the first of those addresses. One does not.
func pedalAddrs(k string, have map[string][]model.Addr) []model.Addr {
	idx, err := mapSets()
	if err != nil {
		return have[k]
	}
	for _, p := range idx.both {
		if k != p.Names[0] {
			continue
		}
		ok := true
		for _, n := range p.Names[1:] {
			if len(have[n]) == 0 {
				ok = false
				break
			}
		}
		if ok {
			return have[p.Names[1]]
		}
	}
	return have[k]
}

// pedalPlan drops the plain name when both variants are located, and records
// the category those new variants should keep. An existing variant is left
// alone, including its conversion.
func pedalPlan(prev, located *model.Model) (drop []string, stamp map[string][]int) {
	stamp = map[string][]int{}
	if prev == nil || located == nil {
		return nil, stamp
	}
	loc := map[string]*model.Object{}
	for _, o := range located.Objects {
		if k := objKey(o); k != "" {
			loc[k] = o
		}
	}
	have := map[string]*model.Object{}
	for _, o := range prev.Objects {
		if k := objKey(o); k != "" {
			have[k] = o
		}
	}
	idx, err := mapSets()
	if err != nil {
		return nil, stamp
	}
	for _, p := range idx.both {
		banks := p.Names[1:]
		var found []*model.Object
		for _, n := range banks {
			o := loc[n]
			if o == nil {
				found = nil
				break
			}
			found = append(found, o)
		}
		if found == nil {
			continue
		}
		plain := have[p.Names[0]]
		if plain != nil {
			drop = append(drop, p.Names[0])
			for i, n := range banks {
				if have[n] != nil {
					continue
				}
				stampCats(stamp, n, plain.Categories)
				if i == 0 && found[0].Address == plain.Address {
					found[0].Value = plain.Value
				}
			}
			continue
		}
		if have[banks[0]] == nil {
			continue
		}
		for _, n := range banks[1:] {
			if have[n] == nil {
				stampCats(stamp, n, have[banks[0]].Categories)
			}
		}
	}
	return drop, stamp
}

func stampCats(stamp map[string][]int, name string, cats []int) {
	if len(cats) == 0 {
		return
	}
	stamp[name] = cats
}

// withoutKeys is m without those object keys. m itself is unchanged.
func withoutKeys(m *model.Model, keys []string) *model.Model {
	if m == nil || len(keys) == 0 {
		return m
	}
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	out := *m
	out.Objects = nil
	for _, o := range m.Objects {
		if drop[objKey(o)] {
			continue
		}
		out.Objects = append(out.Objects, o)
	}
	return &out
}

type placeholderExtra struct {
	items []pubItem
	lines []pubLine
}

// placeholderDefs lists corpus definitions that are not already in lines and
// whose key is obj-N with a blank id. They are not parity images, so they
// have no tier or confidence. --submit clears the key and keeps the origin.
func placeholderDefs(dir string, lines []pubLine) (placeholderExtra, error) {
	seen := map[string]bool{}
	for _, ln := range lines {
		seen[ln.name] = true
	}
	ents, err := os.ReadDir(filepath.Join(dir, "defs"))
	if err != nil {
		return placeholderExtra{}, err
	}
	var out placeholderExtra
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".json")
		if seen[stem] {
			continue
		}
		m, err := readModel(filepath.Join(dir, "defs", e.Name()))
		if err != nil {
			return placeholderExtra{}, err
		}
		n := placeholderCount(m)
		if n == 0 {
			continue
		}
		out.lines = append(out.lines, pubLine{
			name: stem, status: "keys", tier: "-", delta: fmt.Sprintf("key %d", n), total: -1,
		})
		out.items = append(out.items, pubItem{
			name: stem, merged: clearedCopy(m), keepOrigin: true,
			def: filepath.Join(dir, "defs", e.Name()),
		})
	}
	return out, nil
}

// placeholderKey is the key AssignKeys used to invent for a blank id.
var placeholderKey = regexp.MustCompile(`^obj-[0-9]+$`)

func isPlaceholder(o *model.Object) bool {
	return o != nil && strings.TrimSpace(o.ID) == "" && placeholderKey.MatchString(o.Key)
}

func placeholderCount(m *model.Model) int {
	if m == nil {
		return 0
	}
	n := 0
	for _, o := range m.Objects {
		if isPlaceholder(o) {
			n++
		}
	}
	return n
}

func clearPlaceholders(m *model.Model) {
	for _, o := range m.Objects {
		if isPlaceholder(o) {
			o.Key = ""
		}
	}
}

// clearedCopy is m with placeholder keys blanked. m itself is unchanged,
// and the copy keeps m's origin.
func clearedCopy(m *model.Model) *model.Model {
	out := *m
	out.Objects = make([]*model.Object, len(m.Objects))
	for i, o := range m.Objects {
		if o == nil {
			continue
		}
		c := *o
		if isPlaceholder(&c) {
			c.Key = ""
		}
		out.Objects[i] = &c
	}
	return &out
}

func objKey(o *model.Object) string {
	if o == nil || isPlaceholder(o) {
		return ""
	}
	if o.Key != "" {
		return o.Key
	}
	return o.ID
}

// applyForce stores an identical row when the merged model's stamp digests
// differ from the file. Origin stays. stamp.tool is not a digest, so a
// different tool name or version leaves the row identical.
func applyForce(item *pubItem, delta *string) error {
	if item.reason != "identical" || item.merged == nil || item.def == "" {
		return nil
	}
	same, err := sameContent(item.def, item.merged)
	if err != nil || same {
		return err
	}
	item.keepOrigin = true
	item.reason = "forced"
	*delta = "force"
	return nil
}

// sameContent reports whether m and the definition at path have the same
// stamp digests. The digests cover the document without its stamp, so
// stamp.tool is not compared.
func sameContent(path string, m *model.Model) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var old model.Model
	if err := canon.Unmarshal(b, &old); err != nil {
		return false, err
	}
	a, err := canon.Digests(&old)
	if err != nil {
		return false, err
	}
	c, err := canon.Digests(m)
	if err != nil {
		return false, err
	}
	return slices.Equal(a, c), nil
}

func readModel(path string) (*model.Model, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m model.Model
	if err := canon.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func located(d locate.Defs, prepare func() (string, *model.CategoryTable, error), c *ecucorpus.Corpus, name string) (*model.Model, error) {
	userDir, cats, err := prepare()
	if err != nil {
		return nil, err
	}
	imgPath, err := c.Path(name)
	if err != nil {
		return nil, err
	}
	img, err := os.ReadFile(imgPath)
	if err != nil {
		return nil, err
	}
	res, err := d.Generate(imgPath, img, userDir, 0, "auto", "")
	if err != nil {
		return nil, err
	}
	m, _, err := locate.Model(imgPath, img, res.Maps, cats, false)
	if err != nil {
		return nil, err
	}
	if m == nil || len(m.Objects) == 0 {
		return nil, fmt.Errorf("%s: no located maps", name)
	}
	return m, nil
}

// refTable is references.yaml: the reference names our address supersedes.
// Aliases of one map are config/sets.yaml.
type refTable struct {
	Except []string `yaml:"except"`
}

func loadRefTable(data string) (refTable, error) {
	var t refTable
	b, err := os.ReadFile(filepath.Join(data, "references.yaml"))
	if err != nil {
		return t, err
	}
	if err := yaml.Unmarshal(b, &t); err != nil {
		return t, fmt.Errorf("references.yaml: %w", err)
	}
	return t, nil
}

// reference compares located with the manual, DAMOS, or A2L definition prev.
// It is nil when prev is auto or has no provenance.
func reference(prev, located *model.Model, t refTable) *model.Reference {
	p := prev.Provenance
	if p == nil || p.Origin == "auto" {
		return nil
	}
	idx, err := mapSets()
	if err != nil {
		return nil
	}
	have := map[string][]model.Addr{}
	for _, o := range located.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		have[k] = append(have[k], o.Address)
		for _, v := range idx.peers[k] {
			have[v] = append(have[v], o.Address)
		}
	}
	r := &model.Reference{Origin: p.Origin, File: p.File, SHA256: p.SHA256}
	listed := map[string]bool{}
	for _, o := range prev.Objects {
		k := objKey(o)
		if k == "" || listed[k] {
			continue
		}
		listed[k] = true
		r.Agree[1]++
		switch {
		case slices.Contains(pedalAddrs(k, have), o.Address):
			r.Agree[0]++
		case slices.Contains(t.Except, k):
			r.Except = append(r.Except, k)
		}
	}
	for _, o := range located.Objects {
		k := objKey(o)
		if k != "" && !listed[k] && !peerListed(idx, listed, k) && !listed[pedalPlain(k)] {
			r.Unreferenced = append(r.Unreferenced, k)
		}
	}
	slices.Sort(r.Unreferenced)
	r.Unreferenced = slices.Compact(r.Unreferenced)
	return r
}

// pedalPlain is the plain name of a both-set bank, else "".
func pedalPlain(k string) string {
	idx, err := mapSets()
	if err != nil {
		return ""
	}
	for _, p := range idx.both {
		for _, n := range p.Names[1:] {
			if k == n {
				return p.Names[0]
			}
		}
	}
	return ""
}

// peerListed is true when an either-set peer of k is already in the reference.
func peerListed(idx setIndex, listed map[string]bool, k string) bool {
	for _, v := range idx.peers[k] {
		if listed[v] {
			return true
		}
	}
	return false
}

// sourceClass is the publish Source column: verified, DAMOS, matched, manual,
// or auto.
func sourceClass(p *model.Provenance) string {
	switch {
	case p == nil:
		return "auto"
	case p.Verified:
		return "verified"
	case p.Origin == "damos" || p.Origin == "a2l":
		return "DAMOS"
	case p.Origin == "manual":
		return "manual"
	case p.Reference.Agreed():
		return "matched"
	}
	return "auto"
}

func writePublished(path string, m *model.Model, keepOrigin bool, ref *model.Reference) error {
	if m == nil || len(m.Objects) == 0 {
		return fmt.Errorf("%s: no located maps", path)
	}
	if !keepOrigin {
		p := model.Provenance{Format: "image"}
		if m.Provenance != nil {
			p = *m.Provenance
		}
		p.Origin = "auto"
		if ref != nil {
			p.Reference = ref
		}
		m.Provenance = &p
	}
	b, err := canon.MarshalStamped(m, "me7parity corpus")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
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

func setCorpusDef(dir, name, rel string) error {
	path := filepath.Join(dir, "corpus.tsv")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	nl := "\n"
	text := string(b)
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	nameCol, defCol := -1, -1
	for i, h := range strings.Split(lines[0], "\t") {
		switch h {
		case "name":
			nameCol = i
		case "def":
			defCol = i
		}
	}
	if nameCol < 0 || defCol < 0 {
		return fmt.Errorf("%s: no name or def column", path)
	}
	found := false
	for i := 1; i < len(lines); i++ {
		f := strings.Split(lines[i], "\t")
		if nameCol >= len(f) || f[nameCol] != name {
			continue
		}
		for len(f) <= defCol {
			f = append(f, "")
		}
		f[defCol] = rel
		lines[i] = strings.Join(f, "\t")
		found = true
		break
	}
	if !found {
		return fmt.Errorf("%s: %s is not in corpus.tsv", path, name)
	}
	out := strings.Join(lines, nl) + nl
	tmp, err := os.CreateTemp(dir, "corpus.tsv.")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.WriteString(out)
	cerr := tmp.Close()
	if werr != nil {
		_ = os.Remove(tmpName)
		return werr
	}
	if cerr != nil {
		_ = os.Remove(tmpName)
		return cerr
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
