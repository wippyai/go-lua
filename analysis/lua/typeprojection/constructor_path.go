package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
)

// ConstructorPathFromSegments converts a static Lua path suffix into table
// constructor keys.
func ConstructorPathFromSegments(segs []segment.Segment) ([]typetable.ConstructorKey, bool) {
	if len(segs) == 0 {
		return nil, false
	}
	out := make([]typetable.ConstructorKey, 0, len(segs))
	for _, seg := range segs {
		key, ok := ConstructorKeyFromSegment(seg)
		if !ok {
			return nil, false
		}
		out = append(out, key)
	}
	return out, true
}

// ConstructorPathFromSegmentReader converts static Lua path suffix segments
// supplied by a read-only segment reader into table constructor keys.
func ConstructorPathFromSegmentReader(count int, segmentAt func(int) (segment.Segment, bool)) ([]typetable.ConstructorKey, bool) {
	if count <= 0 || segmentAt == nil {
		return nil, false
	}
	out := make([]typetable.ConstructorKey, 0, count)
	for i := 0; i < count; i++ {
		seg, ok := segmentAt(i)
		if !ok {
			return nil, false
		}
		key, ok := ConstructorKeyFromSegment(seg)
		if !ok {
			return nil, false
		}
		out = append(out, key)
	}
	return out, true
}

// ConstructorKeyFromSegment converts one static Lua path segment into a table
// constructor key.
func ConstructorKeyFromSegment(seg segment.Segment) (typetable.ConstructorKey, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if seg.Name == "" {
			return typetable.ConstructorKey{}, false
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: seg.Name}, true
	case segment.SegmentIndexString:
		if seg.Name == "" {
			return typetable.ConstructorKey{}, false
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorStringIndex, Name: seg.Name}, true
	case segment.SegmentIndexInt:
		return typetable.ConstructorKey{Kind: typetable.ConstructorIntIndex, Index: int64(seg.Index)}, true
	default:
		return typetable.ConstructorKey{}, false
	}
}
