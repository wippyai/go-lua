package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func fieldAtPath(t typ.Type, suffix []segment.Segment, depth int) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return fieldAtPath(v.UnaliasedTarget(), suffix, depth+1)
	case *typ.Optional:
		return fieldAtPath(v.Inner, suffix, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil, false
		}
		return fieldAtPath(expanded, suffix, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			field, ok := fieldAtPath(member, suffix, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, field)
		}
		return normalize.UnionForEvidence(out...), true
	case *typ.Record:
		field, ok := directRecordMember(v, suffix[0])
		if !ok {
			return nil, false
		}
		if len(suffix) == 1 {
			return field, true
		}
		return fieldAtPath(field, suffix[1:], depth+1)
	default:
		return nil, false
	}
}

func directRecordMember(r *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		if field := r.GetField(seg.Name); field != nil {
			return field.Type, true
		}
		if member := r.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, true
		}
	case segment.SegmentIndexString:
		if member := r.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, true
		}
		if field := r.GetField(seg.Name); field != nil {
			return field.Type, true
		}
	case segment.SegmentIndexInt:
		if member := r.GetStaticIntIndex(int64(seg.Index)); member != nil {
			return member.Type, true
		}
	}
	return nil, false
}
