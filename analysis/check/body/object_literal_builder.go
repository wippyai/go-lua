package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
)

func objectLiteralConstructorPath(segs []segment.Segment) ([]typetable.ConstructorKey, bool) {
	if len(segs) == 0 {
		return nil, false
	}
	out := make([]typetable.ConstructorKey, 0, len(segs))
	for _, seg := range segs {
		key, ok := objectLiteralConstructorKey(seg)
		if !ok {
			return nil, false
		}
		out = append(out, key)
	}
	return out, true
}

func objectLiteralConstructorKey(seg segment.Segment) (typetable.ConstructorKey, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		name, ok := staticStringSegment(seg)
		if !ok {
			return typetable.ConstructorKey{}, false
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: name}, true
	case segment.SegmentIndexInt:
		return typetable.ConstructorKey{Kind: typetable.ConstructorIntIndex, Index: int64(seg.Index)}, true
	default:
		return typetable.ConstructorKey{}, false
	}
}

func staticStringSegment(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}
