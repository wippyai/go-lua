package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type objectLiteralTypeNode struct {
	value   typ.Type
	sealed  bool
	fields  map[string]*objectLiteralTypeNode
	indexes map[int]*objectLiteralTypeNode
}

func newObjectLiteralTypeNode() *objectLiteralTypeNode {
	return &objectLiteralTypeNode{
		fields:  make(map[string]*objectLiteralTypeNode),
		indexes: make(map[int]*objectLiteralTypeNode),
	}
}

func (n *objectLiteralTypeNode) add(segs []segment.Segment, t typ.Type) bool {
	return n.insert(segs, t, false)
}

// addSealed records the adopted declared type for a path prefix and seals the
// node so deeper entries enumerating the same constructor do not rebuild it into
// a narrower structural record. Sealing is order-independent: a sealed node drops
// any field children already collected and ignores later writes beneath it.
func (n *objectLiteralTypeNode) addSealed(segs []segment.Segment, t typ.Type) bool {
	return n.insert(segs, t, true)
}

func (n *objectLiteralTypeNode) insert(segs []segment.Segment, t typ.Type, sealed bool) bool {
	if n == nil || len(segs) == 0 || t == nil {
		return false
	}
	seg := segs[0]
	var child *objectLiteralTypeNode
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		name, ok := staticStringSegment(seg)
		if !ok {
			return false
		}
		child = n.fields[name]
		if child == nil {
			child = newObjectLiteralTypeNode()
			n.fields[name] = child
		}
	case segment.SegmentIndexInt:
		if seg.Index < 0 {
			return false
		}
		child = n.indexes[seg.Index]
		if child == nil {
			child = newObjectLiteralTypeNode()
			n.indexes[seg.Index] = child
		}
	default:
		return false
	}
	if len(segs) == 1 {
		if sealed {
			child.value = t
			child.sealed = true
			child.fields = nil
			child.indexes = nil
		} else if !child.sealed {
			child.value = t
		}
		return true
	}
	if child.sealed {
		return true
	}
	return child.insert(segs[1:], t, sealed)
}

func (n *objectLiteralTypeNode) build() (typ.Type, bool) {
	if n == nil {
		return nil, false
	}
	if len(n.fields) == 0 && len(n.indexes) == 0 {
		return n.value, n.value != nil
	}
	if len(n.fields) == 0 && len(n.indexes) > 0 {
		indexes := make([]int, 0, len(n.indexes))
		for idx := range n.indexes {
			indexes = append(indexes, idx)
		}
		sort.Ints(indexes)
		elems := make([]typ.Type, 0, len(indexes))
		for _, idx := range indexes {
			t, ok := n.indexes[idx].build()
			if !ok {
				return nil, false
			}
			elems = append(elems, t)
		}
		if len(elems) == 0 {
			return nil, false
		}
		return typ.NewTuple(elems...), true
	}
	builder := typetable.NewRecord()
	for name, child := range n.fields {
		t, ok := child.build()
		if !ok {
			return nil, false
		}
		builder.Field(name, t)
	}
	for _, idx := range sortedIndexKeys(n.indexes) {
		t, ok := n.indexes[idx].build()
		if !ok {
			return nil, false
		}
		builder.StaticIntIndex(int64(idx), t)
	}
	return builder.Build(), true
}

func sortedIndexKeys(indexes map[int]*objectLiteralTypeNode) []int {
	if len(indexes) == 0 {
		return nil
	}
	keys := make([]int, 0, len(indexes))
	for idx := range indexes {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	return keys
}

func staticStringSegment(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}
