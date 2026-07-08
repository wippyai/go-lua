package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// ObjectLiteralStaticStringKeysAtSource returns the closed string-key set of an
// object literal source or of a nested object literal at suffix. Array/int
// entries are ignored because they do not contribute to string-key dispatch
// domains; dynamic or invalid field keys clear the completeness proof.
func (r *Result) ObjectLiteralStaticStringKeysAtSource(source factflow.ValueSource, suffix []segment.Segment) (map[string]bool, SourceSpan, bool) {
	literal, ok := r.ObjectLiteralViewForSource(source)
	if !ok {
		return nil, SourceSpan{}, false
	}
	return r.objectLiteralStaticStringKeysAtView(literal, suffix)
}

func (r *Result) objectLiteralStaticStringKeysAtView(literal factflow.ObjectLiteralView, suffix []segment.Segment) (map[string]bool, SourceSpan, bool) {
	if len(suffix) != 0 {
		nested, ok := r.nestedObjectLiteralView(literal, suffix)
		if !ok {
			return nil, SourceSpan{}, false
		}
		literal = nested
	}
	if !literal.StaticStringKeysComplete() {
		return nil, SourceSpan{}, false
	}
	keys := make(map[string]bool, literal.EntryCount())
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if entry.SuffixSegmentCount() != 1 {
			return true
		}
		seg, ok := entry.SuffixSegmentAt(0)
		if !ok {
			return true
		}
		if key, ok := objectLiteralStaticStringKey(seg); ok {
			keys[key] = true
		}
		return true
	})
	span, ok := literal.Span()
	if !ok {
		return nil, SourceSpan{}, false
	}
	return keys, sourceSpanFromFactflow(span), true
}

func (r *Result) nestedObjectLiteralView(literal factflow.ObjectLiteralView, suffix []segment.Segment) (factflow.ObjectLiteralView, bool) {
	if r == nil || len(suffix) == 0 {
		return factflow.ObjectLiteralView{}, false
	}
	var out factflow.ObjectLiteralView
	var found bool
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if !sameSegments(entry.SuffixSegmentsView(), suffix) {
			return true
		}
		nested, ok := r.ObjectLiteralViewForSource(entry.Source())
		if !ok {
			return true
		}
		out = nested
		found = true
		return false
	})
	return out, found
}

func objectLiteralStaticStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func sameSegments(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
