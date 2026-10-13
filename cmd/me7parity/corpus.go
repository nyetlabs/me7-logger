package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/config"
	"go.nyet.org/me7-logger/internal/cli"
	"go.nyet.org/me7-logger/internal/ecucorpus"
	"go.nyet.org/me7-logger/internal/locate"
	"go.nyet.org/me7-logger/parity"
)

const corpusHelp = `
Lists every parity image. Confidence has to pass on every row: high
bodies are more than half the tuner names that scored. A zero
denominator fails the row.

  status       whether --submit stores the row, and why
  tier         the names grade. * is below S, except on a DAMOS or
               A2L definition
  delta        the edit --submit would write. - is nothing
  confidence   high bodies / tuner names that scored

status
  eligible               stored. Origin becomes located.
  identical              same names, and the description and comment
                         would stay.
  origin damos           DAMOS definition. Left in place.
  origin a2l             A2L definition. Left in place.
  origin unset           no provenance origin. Left in place.
  hand short NAME        that hand name was not located, so the file
                         stays. When other names can move or be added,
                         the status continues with the delta and
                         --submit writes those. Origin stays hand.
  hand disagrees NAME    that name has more than one located
                         address. A following delta is a partial
                         write and the origin stays hand. With no
                         delta the row is left as it is.
  grade -                the name grade is below D. Origin hand is
                         scored without waiting for a grade. With no
                         definition, D or better is eligible.
  confidence N/M         high bodies are half or fewer.
  keys                   blank-id keys named obj-N. --submit clears
                         them and leaves the origin. Another status
                         ends with ", key N" for the same clear.
  def unreadable         the definition file could not be read.

delta
  +N       names added. Status is eligible.
  ~N       descriptions, comments, or addresses brought in line.
           Status is eligible when nothing is short.
  ~N +N    both, while a short name remains. Origin stays hand.
  key N    placeholder keys cleared. On an eligible row the origin
           still becomes located.

A definition that is not a parity image is listed when it has
placeholder keys. Its tier is - and its confidence is -.

A pn is a stem, a stem.bin, or the part number in front of the
software index: 8D0907551H matches 8D0907551H-0003. An exact stem
matches only that image. Under each named row:
  hand    short, disagreeing, and added keys
  open    tuner names that did not hit
  block   layout-block names that did not hit
absent and "met by NAME" are the same marks as the report.
--submit stores only the images named.

--submit writes defs/NAME.json and sets that row's def column.
A stored object keeps the previous conversion, units, and layout.
The description becomes the name table and the comment the needle
that located the map. It leaves the checkout uncommitted, and it
does not push or upload.

The corpus is --corpus, else CORPUS, else ../ecu-corpus when that
checkout exists, else corpus/.
`

func cmdCorpus(args []string) error {
	fs := cli.NewFlagSet("me7parity", "corpus", "[flags] [pn...]", corpusHelp)
	data := fs.StringP("data", "d", "testdata/parity", "parity root `<dir>`")
	corpusFlag := fs.String("corpus", "", "ecu-corpus checkout `<dir>` (default CORPUS, else ../ecu-corpus, else corpus/)")
	submit := fs.Bool("submit", false, "write the rows marked for storage into the corpus")
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
	var eligible []pubItem
	var lines []pubLine
	for _, im := range rep.Tuner {
		name := strings.TrimSuffix(im.Name, ".bin")
		if len(want) > 0 && !want[name] {
			continue
		}
		item := pubItem{name: name, tier: im.Tier, def: c.Def(name)}
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
			}
			nKey = placeholderCount(prev)
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
			reason, added, notes := coverAll(prev, loc)
			item.reason = reason
			item.notes = notes
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
				item.keepOrigin = true
				delta = partialDelta(moves, added)
				item.reason = reason + ", " + delta
			}
			if err != nil {
				return err
			}
			if item.merged != nil {
				clearPlaceholders(item.merged)
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
		})
	}
	extra, err := placeholderDefs(dir, lines)
	if err != nil {
		return err
	}
	if len(want) > 0 {
		extra = placeholderExtra{}
	}
	for _, item := range extra.items {
		eligible = append(eligible, item)
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
			if err := writePublished(dst, item.merged, item.keepOrigin); err != nil {
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
			wrote, err := locate.WriteModel(dst, imgPath, img, res.Maps, cats, "me7parity corpus")
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
	keepOrigin                      bool
	notes                           []coverNote
}

type pubLine struct {
	name, status, tier, delta string
	hit, total                int
	notes                     []coverNote
	open, block               []parity.OpenName
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

// printDetail lists the hand gaps and the tuner names that did not hit.
func printDetail(lines []pubLine) {
	for _, ln := range lines {
		if len(ln.notes) == 0 && len(ln.open) == 0 && len(ln.block) == 0 {
			continue
		}
		fmt.Printf("\n%s\n", ln.name)
		if len(ln.notes) > 0 {
			fmt.Println("hand")
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
// identical. --submit still stores that set when a description or comment
// would change. Both KFPED variants cover a hand KFPED, at the _0_A
// address. The same for KFPEDR. One variant does not, and a located KFPED
// does not cover a hand KFPED_0_A.
func handCover(prev, located *model.Model) (string, int) {
	reason, added, _ := coverAll(prev, located)
	return reason, added
}

// coverNote is one hand key the detail list prints.
type coverNote struct {
	op   string
	key  string
	hand model.Addr
	have []model.Addr
}

func (n coverNote) line() string {
	switch n.op {
	case "short":
		return fmt.Sprintf("short %s %s", n.key, addrText(n.hand))
	case "disagrees":
		return fmt.Sprintf("disagrees %s hand %s located %s", n.key, addrText(n.hand), addrsText(n.have))
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

// coverAll is handCover plus every short, disagreeing, and added key.
// The reason is still the first short or disagree. Added keys are listed
// and counted even when a short or disagree blocks a full replace.
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
			notes = append(notes, coverNote{op: "short", key: k, hand: o.Address})
			if reason == "" {
				reason = "hand short " + k
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
			notes = append(notes, coverNote{op: "disagrees", key: k, hand: o.Address, have: addrs})
			if reason == "" {
				reason = "hand disagrees " + k
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

// partialDelta is the address corrections and the located names the hand
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
// on a name the hand file already has. The description and comment come
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

// corrections is how many previous objects change description or comment
// in merged. Address is part of the match, so a moved name is not counted.
func corrections(prev, merged *model.Model) int {
	if prev == nil || merged == nil {
		return 0
	}
	type at struct {
		key  string
		addr model.Addr
	}
	next := map[at]*model.Object{}
	for _, o := range merged.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		next[at{k, o.Address}] = o
	}
	n := 0
	for _, o := range prev.Objects {
		k := objKey(o)
		if k == "" {
			continue
		}
		m := next[at{k, o.Address}]
		if m == nil {
			continue
		}
		if m.Description != o.Description || m.Comment != o.Comment {
			n++
		}
	}
	return n
}

// mergeKept folds the located model into the previous pack. Conversion,
// units, and layout stay on the previous object. The description and the
// comment come from the located model. When both pedal variants are located,
// the plain name is dropped and the variants take its category. A variant
// already in the pack keeps its conversion.
func mergeKept(prev, located *model.Model) (*model.Model, error) {
	drop, stamp := pedalPlan(prev, located)
	base := withoutKeys(prev, drop)
	out, _, err := model.Merge(base, located, model.MergeOptions{
		Description: "replace",
		Comment:     "replace",
	})
	if err != nil {
		return nil, err
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

// pedalPair is the plain name and the two variants that replace it.
var pedalPair = []struct{ plain, a, b string }{
	{"KFPED", "KFPED_0_A", "KFPED_1_A"},
	{"KFPEDR", "KFPEDR_0_A", "KFPEDR_1_A"},
}

// pedalAddrs is the addresses that cover a hand key. Both variants cover
// the plain name at the _0_A address. One variant does not.
func pedalAddrs(k string, have map[string][]model.Addr) []model.Addr {
	for _, p := range pedalPair {
		if k != p.plain {
			continue
		}
		if len(have[p.a]) > 0 && len(have[p.b]) > 0 {
			return have[p.a]
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
	for _, p := range pedalPair {
		a, b := loc[p.a], loc[p.b]
		if a == nil || b == nil {
			continue
		}
		plain := have[p.plain]
		if plain != nil {
			drop = append(drop, p.plain)
			if have[p.a] == nil {
				stampCats(stamp, p.a, plain.Categories)
				if a.Address == plain.Address {
					a.Value = plain.Value
				}
			}
			if have[p.b] == nil {
				stampCats(stamp, p.b, plain.Categories)
			}
			continue
		}
		if have[p.a] != nil && have[p.b] == nil {
			stampCats(stamp, p.b, have[p.a].Categories)
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

func writePublished(path string, m *model.Model, keepOrigin bool) error {
	if m == nil || len(m.Objects) == 0 {
		return fmt.Errorf("%s: no located maps", path)
	}
	if !keepOrigin {
		if m.Provenance == nil {
			m.Provenance = &model.Provenance{Format: "image", Origin: "located"}
		}
		m.Provenance.Origin = "located"
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
