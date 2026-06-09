package transfer

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// structuralPathLens is the canonical type-level lens for a static path inside a
// structural value. It owns the paired operations callers need to keep in sync:
// reading the slot at a path and rebuilding the root with that slot refined.
type structuralPathLens struct {
	segments []constraint.Segment
}

func structuralPath(segments []constraint.Segment) structuralPathLens {
	return structuralPathLens{segments: append([]constraint.Segment(nil), segments...)}
}

// Read resolves the type at the lens path, descending static fields/string keys
// through the value-domain resolver and tuple integer indices directly.
func (l structuralPathLens) Read(root typ.Type) (typ.Type, bool) {
	cur := root
	for _, seg := range l.segments {
		if cur == nil {
			return nil, false
		}
		switch seg.Kind {
		case constraint.SegmentField:
			ft, ok := fieldResolver.Field(cur, seg.Name)
			if !ok || ft == nil {
				return nil, false
			}
			cur = ft
		case constraint.SegmentIndexString:
			it, ok := fieldResolver.Index(cur, typ.LiteralString(seg.Name))
			if !ok || it == nil {
				return nil, false
			}
			cur = it
		case constraint.SegmentIndexInt:
			it, ok := fieldResolver.Index(cur, typ.LiteralInt(int64(seg.Index)))
			if !ok || it == nil {
				return nil, false
			}
			cur = it
		default:
			return nil, false
		}
	}
	return cur, true
}

// Refine rebuilds root with the value at the lens path replaced by refine(value).
// Missing static fields are treated as path absence; callers that want "absent
// survives" semantics use mapUnionField directly because that is a guard policy,
// not a path-lens policy.
func (l structuralPathLens) Refine(root typ.Type, refine func(typ.Type) typ.Type) typ.Type {
	return l.refine(root, l.segments, refine)
}

func (l structuralPathLens) refine(root typ.Type, segments []constraint.Segment, refine func(typ.Type) typ.Type) typ.Type {
	if len(segments) == 0 {
		return refine(root)
	}
	seg := segments[0]
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		if len(segments) == 1 {
			return mapUnionField(root, seg.Name, refine, false)
		}
		return mapUnionField(root, seg.Name, func(ft typ.Type) typ.Type {
			return l.refine(ft, segments[1:], refine)
		}, false)
	case constraint.SegmentIndexInt:
		tuple, ok := unwrap.Alias(root).(*typ.Tuple)
		if !ok {
			return root
		}
		idx := seg.Index - 1
		if idx < 0 || idx >= len(tuple.Elements) {
			return root
		}
		next := l.refine(tuple.Elements[idx], segments[1:], refine)
		if next == nil || typ.TypeEquals(next, tuple.Elements[idx]) {
			return root
		}
		elems := append([]typ.Type(nil), tuple.Elements...)
		elems[idx] = next
		return typ.NewTuple(elems...)
	default:
		return root
	}
}

func (l structuralPathLens) HasMultiUnion(root typ.Type) bool {
	target, ok := l.Read(root)
	if !ok || target == nil {
		return false
	}
	u := unwrap.Union(target)
	return u != nil && len(u.Members) > 1
}
