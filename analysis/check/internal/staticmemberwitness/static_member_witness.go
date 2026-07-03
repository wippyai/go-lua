package staticmemberwitness

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

// Builder builds a record witness from proven rootless member suffixes. It sits
// in the check layer because the inputs are path segments while the output is a
// type witness; keeping it here avoids making the type layer depend on path
// vocabulary.
type Builder struct {
	root *node
}

type node struct {
	value         typ.Type
	fields        map[string]*node
	stringIndexes map[string]*node
	intIndexes    map[int64]*node
}

func NewBuilder() *Builder {
	return &Builder{root: &node{}}
}

func (b *Builder) Add(segs []segment.Segment, t typ.Type) {
	if b == nil || b.root == nil || len(segs) == 0 || t == nil {
		return
	}
	b.root.insert(segs, t)
}

func (b *Builder) Build() (typ.Type, bool) {
	if b == nil || b.root == nil {
		return nil, false
	}
	return b.root.build()
}

func (n *node) insert(segs []segment.Segment, t typ.Type) bool {
	if n == nil || len(segs) == 0 || t == nil {
		return false
	}
	child, ok := n.child(segs[0])
	if !ok {
		return false
	}
	if len(segs) == 1 {
		child.value = t
		return true
	}
	return child.insert(segs[1:], t)
}

func (n *node) child(seg segment.Segment) (*node, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if seg.Name == "" {
			return nil, false
		}
		if n.fields == nil {
			n.fields = make(map[string]*node)
		}
		if n.fields[seg.Name] == nil {
			n.fields[seg.Name] = &node{}
		}
		return n.fields[seg.Name], true
	case segment.SegmentIndexString:
		if seg.Name == "" {
			return nil, false
		}
		if n.stringIndexes == nil {
			n.stringIndexes = make(map[string]*node)
		}
		if n.stringIndexes[seg.Name] == nil {
			n.stringIndexes[seg.Name] = &node{}
		}
		return n.stringIndexes[seg.Name], true
	case segment.SegmentIndexInt:
		if n.intIndexes == nil {
			n.intIndexes = make(map[int64]*node)
		}
		index := int64(seg.Index)
		if n.intIndexes[index] == nil {
			n.intIndexes[index] = &node{}
		}
		return n.intIndexes[index], true
	default:
		return nil, false
	}
}

func (n *node) build() (typ.Type, bool) {
	if n == nil {
		return nil, false
	}
	if len(n.fields) == 0 && len(n.stringIndexes) == 0 && len(n.intIndexes) == 0 {
		return n.value, n.value != nil
	}
	builder := typetable.NewRecord()
	for _, name := range sortedStringKeys(n.fields) {
		t, ok := n.fields[name].build()
		if !ok {
			return nil, false
		}
		builder.Field(name, t)
	}
	for _, name := range sortedStringKeys(n.stringIndexes) {
		t, ok := n.stringIndexes[name].build()
		if !ok {
			return nil, false
		}
		builder.StaticStringIndex(name, t)
	}
	for _, index := range sortedIntKeys(n.intIndexes) {
		t, ok := n.intIndexes[index].build()
		if !ok {
			return nil, false
		}
		builder.StaticIntIndex(index, t)
	}
	record := builder.Build()
	if n.value != nil {
		return typeexpr.Intersection(n.value, record), true
	}
	return record, true
}

func sortedStringKeys(in map[string]*node) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedIntKeys(in map[int64]*node) []int64 {
	out := make([]int64, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
