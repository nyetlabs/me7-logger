package generate

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"go.nyet.org/xdfkit/model"

	"go.nyet.org/me7-logger/opcode"
)

// Label is one row of the TSV ME7ImportLabelsScript reads. Size is 1 or 2
// for a byte or word and 0 otherwise.
type Label struct {
	Addr    uint32
	Name    string
	Size    int
	Comment string
	Bit     bool
}

// Labels are the function entries and RAM items this result located on img,
// and the bodies and axes of objs, the full XDF model of its maps. A needle
// with several entries is NAME_ADDR. The first row with a name
// (case-insensitive) wins. Rows sort by address with bit flags last.
func (r *Result) Labels(img []byte, objs []*model.Object) []Label {
	var out []Label
	seen := map[string]bool{}
	add := func(lb Label) {
		if k := strings.ToLower(lb.Name); lb.Name != "" && !seen[k] {
			seen[k] = true
			out = append(out, lb)
		}
	}
	var names []string
	entries := map[string][]uint32{}
	for _, n := range r.Needles {
		if _, ok := entries[n.Name]; !ok {
			names = append(names, n.Name)
			entries[n.Name] = nil
		}
		for _, e := range n.Labels(img) {
			if addr := uint32(opcode.FlashBase + e); !slices.Contains(entries[n.Name], addr) {
				entries[n.Name] = append(entries[n.Name], addr)
			}
		}
	}
	for _, name := range names {
		for _, addr := range entries[name] {
			lb := Label{addr, name, 0, "me7info needle", false}
			if len(entries[name]) > 1 {
				lb.Name = fmt.Sprintf("%s_%06X", name, addr)
			}
			add(lb)
		}
	}
	for _, it := range r.File.Items {
		lb := Label{it.Addr, it.Name, it.Size, it.Comment, it.Bitmask != 0}
		if lb.Bit {
			lb.Size = 0
			lb.Comment = strings.TrimSpace(fmt.Sprintf("%s mask 0x%X", it.Comment, it.Bitmask))
		}
		add(lb)
	}
	for _, o := range objs {
		desc := cmp.Or(o.Description, o.Comment)
		add(Label{opcode.FlashBase + uint32(o.Address), o.ID, cellSize(&o.Data, o.Rows*o.Cols), mapComment(desc, o.Value.Units), false})
		for i, ax := range []*model.Axis{o.X, o.Y} {
			if ax != nil && ax.Address != nil && *ax.Address != o.Address {
				add(Label{opcode.FlashBase + uint32(*ax.Address), o.ID + []string{"_x", "_y"}[i],
					cellSize(ax.Data, []int{o.Cols, o.Rows}[i]), mapComment(desc, ax.Value.Units), false})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Addr != out[j].Addr {
			return out[i].Addr < out[j].Addr
		}
		return !out[i].Bit && out[j].Bit
	})
	return out
}

// cellSize is the byte or word width of a lone cell, else 0.
func cellSize(d *model.Data, n int) int {
	if d == nil || n > 1 || (d.Bits != 8 && d.Bits != 16) {
		return 0
	}
	return d.Bits / 8
}

func mapComment(comment, unit string) string {
	if unit != "" {
		comment += " [" + unit + "]"
	}
	return strings.TrimSpace(comment)
}

// WriteLabels writes labels as the addr, name, size, comment TSV. Path "-"
// is stdout.
func WriteLabels(path string, labels []Label) error {
	var b strings.Builder
	b.WriteString("addr\tname\tsize\tcomment\n")
	for _, lb := range labels {
		fmt.Fprintf(&b, "0x%06X\t%s\t%d\t%s\n", lb.Addr, lb.Name, lb.Size, strings.Join(strings.Fields(lb.Comment), " "))
	}
	if path == "-" {
		_, err := os.Stdout.WriteString(b.String())
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
