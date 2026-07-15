package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// FieldAtPath resolves the static member type reached by following suffix from t,
// descending through Alias/Recursive/Optional/Instantiated wrappers and lifting
// the access pointwise over union members. It reports false when any segment is
// not a static member of the type reached at that step.
func FieldAtPath(t typ.Type, suffix []segment.Segment) (typ.Type, bool) {
	field, ok, productive := fieldAtPathSeen(t, suffix, &typegraph.Path{})
	return field, ok && productive
}

func fieldAtPath(t typ.Type, suffix []segment.Segment) (typ.Type, bool) {
	field, ok, productive := fieldAtPathSeen(t, suffix, &typegraph.Path{})
	return field, ok && productive
}

func fieldAtPathSeen(t typ.Type, suffix []segment.Segment, active *typegraph.Path) (typ.Type, bool, bool) {
	if t == nil || len(suffix) == 0 {
		return nil, false, true
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, len(suffix)) {
		return nil, true, false
	}
	defer active.Leave(t, len(suffix))
	switch v := t.(type) {
	case *typ.Alias:
		return fieldAtPathSeen(v.UnaliasedTarget(), suffix, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, true, false
		}
		return fieldAtPathSeen(v.Body, suffix, active)
	case *typ.Optional:
		return fieldAtPathSeen(v.Inner, suffix, active)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil, true, false
		}
		return fieldAtPathSeen(expanded, suffix, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		productive := false
		for _, member := range v.Members {
			field, ok, memberProductive := fieldAtPathSeen(member, suffix, active)
			if !ok {
				return nil, false, true
			}
			if !memberProductive {
				continue
			}
			productive = true
			out = append(out, field)
		}
		if !productive {
			return nil, true, false
		}
		return normalize.UnionForEvidence(out...), true, true
	case *typ.Record:
		field, ok := directRecordMember(v, suffix[0])
		if !ok {
			return nil, false, true
		}
		if len(suffix) == 1 {
			return field, true, true
		}
		return fieldAtPathSeen(field, suffix[1:], active)
	default:
		return nil, false, true
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
