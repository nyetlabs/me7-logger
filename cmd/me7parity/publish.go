package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/internal/locate"
	"go.nyet.org/me7-logger/parity"
)

const publishHelp = `
Prints every parity image. Confidence has to pass for every image:
high bodies are more than half the tuner names that scored, and a zero
denominator is not eligible.

A star after the tier is an empty corpus definition, or origin hand.
With no definition, the name grade must be D, C, B, A, S, or S+.
Origin hand does not wait for that grade. Origin hand or located is
eligible when the located model is a strict superset: every current
name is at the same address, and at least one more name is present.
The delta is that added count. An identical name set is not eligible
and is not written. Shared objects keep the previous conversion,
units, and description. Origin damos, a2l, or unset is never replaced.

--write stores an eligible model at defs/NAME.json (provenance.origin
located) and sets the row's def column. It does not commit, push, bump,
or upload. The corpus is CORPUS, else ../ecu-corpus when that checkout
exists, else corpus/.
`

func cmdPublish(args []string) error {
	fs := cli.NewFlagSet("me7parity", "publish", "[flags]", publishHelp)
	data := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`")
	corpusFlag := fs.String("corpus", "", "ecu-corpus checkout `<dir>` (default CORPUS, else ../ecu-corpus, else corpus/)")
	write := fs.Bool("write", false, "write eligible located models into the corpus")
	d := locate.Flags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("publish takes no image; images.yaml names them")
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
	var eligible []pubItem
	var lines []pubLine
	for _, im := range rep.Tuner {
		name := strings.TrimSuffix(im.Name, ".bin")
		item := pubItem{name: name, tier: im.Tier, def: c.Def(name)}
		if item.tier == "" {
			item.tier = "-"
		}
		var prev *model.Model
		if item.def != "" {
			prev, err = readModel(item.def)
			if err != nil {
				item.reason = "def unreadable"
			} else if prev.Provenance != nil {
				item.origin = prev.Provenance.Origin
			}
		}
		if item.reason == "" {
			item.reason = gateReason(im.Tier, im.Confidence, item.def != "", item.origin)
		}
		delta := "-"
		if item.reason == "" && (item.origin == "hand" || item.origin == "located") {
			loc, err := located(d, prepare, c, name)
			if err != nil {
				return err
			}
			reason, added := handCover(prev, loc)
			item.reason = reason
			if reason == "" || reason == "identical" {
				delta = fmt.Sprintf("+%d", added)
			}
			if item.reason == "" {
				item.merged = mergeKept(prev, loc)
			}
		}
		status := item.reason
		if status == "" {
			status = "eligible"
			eligible = append(eligible, item)
		}
		tier := item.tier
		if im.NoDef {
			tier += "*"
		}
		lines = append(lines, pubLine{
			name: name, status: status, tier: tier, delta: delta,
			hit: im.Confidence.Hit, total: im.Confidence.Total,
		})
	}
	printPublish(lines)
	if !*write {
		return nil
	}
	for _, item := range eligible {
		dst := filepath.Join(dir, "defs", item.name+".json")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		switch {
		case item.merged != nil:
			if err := writePublished(dst, item.merged); err != nil {
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
			wrote, err := locate.WriteModel(dst, imgPath, img, res.Maps, cats, "me7parity publish")
			if err != nil {
				return err
			}
			if !wrote {
				return fmt.Errorf("%s: no located maps", item.name)
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
}

type pubLine struct {
	name, status, tier, delta string
	hit, total                int
}

func printPublish(lines []pubLine) {
	nameW, statusW, deltaW := len("name"), len("status"), len("delta")
	confW := len("confidence")
	confs := make([]string, len(lines))
	for i, ln := range lines {
		nameW = max(nameW, len(ln.name))
		statusW = max(statusW, len(ln.status))
		deltaW = max(deltaW, len(ln.delta))
		confs[i] = fmt.Sprintf("%d/%d", ln.hit, ln.total)
		confW = max(confW, len(confs[i]))
	}
	fmt.Printf("%-*s  %-*s  %-4s  %-*s  %*s\n",
		nameW, "name", statusW, "status", "tier", deltaW, "delta", confW, "confidence")
	for i, ln := range lines {
		fmt.Printf("%-*s  %-*s  %-4s  %-*s  %*s\n",
			nameW, ln.name, statusW, ln.status, ln.tier, deltaW, ln.delta, confW, confs[i])
	}
}

// gateReason is empty when the image may be published, or when origin
// hand or located still has to pass handCover. Confidence is required first.
// Grade "-" passes only for hand. damos, a2l, and an unset origin do not.
func gateReason(tier string, conf parity.Fraction, hasDef bool, origin string) string {
	if conf.Total == 0 || conf.Hit*2 <= conf.Total {
		return fmt.Sprintf("confidence %d/%d", conf.Hit, conf.Total)
	}
	if hasDef && origin != "hand" && origin != "located" {
		if origin == "" {
			origin = "unset"
		}
		return "origin " + origin
	}
	if origin == "hand" {
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

// handCover is empty when located strictly covers prev, and the count
// is how many names are new. A name the locator does not have is short.
// The same name at another address disagrees. An equal set of names is
// identical and is not written.
func handCover(prev, located *model.Model) (string, int) {
	have := map[string][]model.Addr{}
	for _, o := range located.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		have[k] = append(have[k], o.Address)
	}
	prevKey := map[string]struct{}{}
	for _, o := range prev.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		prevKey[k] = struct{}{}
		addrs := have[k]
		if len(addrs) == 0 {
			return "hand short " + k, 0
		}
		match := false
		for _, a := range addrs {
			if a == o.Address {
				match = true
				break
			}
		}
		if !match {
			return "hand disagrees " + k, 0
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
		added++
	}
	if added == 0 {
		return "identical", 0
	}
	return "", added
}

// mergeKept is the located model, with any object the previous file
// already had at that address kept whole so its conversion survives.
func mergeKept(prev, located *model.Model) *model.Model {
	type at struct {
		key  string
		addr model.Addr
	}
	old := map[at]*model.Object{}
	for _, o := range prev.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		old[at{k, o.Address}] = o
	}
	out := *located
	out.Objects = make([]*model.Object, 0, len(located.Objects))
	for _, o := range located.Objects {
		if o == nil {
			continue
		}
		if kept := old[at{objKey(o), o.Address}]; kept != nil {
			out.Objects = append(out.Objects, kept)
			continue
		}
		out.Objects = append(out.Objects, o)
	}
	if located.Provenance != nil {
		p := *located.Provenance
		p.Origin = "located"
		out.Provenance = &p
	}
	return &out
}

func objKey(o *model.Object) string {
	if o == nil {
		return ""
	}
	if o.Key != "" {
		return o.Key
	}
	return o.ID
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

func writePublished(path string, m *model.Model) error {
	if m == nil || len(m.Objects) == 0 {
		return fmt.Errorf("%s: no located maps", path)
	}
	if m.Provenance == nil {
		m.Provenance = &model.Provenance{Format: "image", Origin: "located"}
	}
	m.Provenance.Origin = "located"
	b, err := canon.MarshalStamped(m, "me7parity publish")
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
