package access

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (q *query) field(t typ.Type, name string, depth int, cycle fieldResult) fieldResult {
	if stopDepth(t, depth) {
		return fieldResult{}
	}
	visit := queryKey{op: 1, t: t, name: name}
	if !q.enter(visit) {
		return cycle
	}
	defer q.leave(visit)
	if top, ok := SpecialAccessType(t); ok {
		return fieldResult{t: top, ok: true}
	}

	return descendAccessWrappers(t, depth, nil, zeroFieldResult, func(t typ.Type, depth int) fieldResult {
		if top, ok := SpecialAccessType(t); ok {
			return fieldResult{t: top, ok: true}
		}
		switch v := unwrap.Annotated(t).(type) {
		case *typ.Record:
			return fieldInRecord(v, name)
		case *typ.Interface:
			return fieldInInterface(v, t, name)
		case *typ.Map:
			return fieldInMap(v.Key, v.Value, name)
		case *typ.ReadonlyMap:
			return fieldInMap(v.Key, v.Value, name)
		case *typ.Union:
			return q.fieldInUnion(v, name, depth+1)
		case *typ.Intersection:
			return q.fieldInIntersection(v, name, depth+1)
		default:
			return fieldResult{}
		}
	}, func(res fieldResult) fieldResult {
		if res.ok {
			res.nilable = true
		}
		return res
	})
}

func SpecialAccessType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	if typ.IsAny(t) {
		return typ.Any, true
	}
	if typ.IsUnknown(t) {
		return typ.Unknown, true
	}
	if typ.IsNever(t) {
		return typ.Never, true
	}
	if typetable.IsBuiltinTopMarker(t) {
		return typ.Any, true
	}
	return nil, false
}

// stopDepth reports whether a field/index resolution must stop without
// descending further: the type is missing, or the recursion has exhausted
// its depth budget. Beyond the O(1)-stack wrapper-unwind loop in
// descendAccessWrappers, field()/index() re-enter themselves through
// fieldInUnion/fieldInIntersection (and their index_* counterparts) via
// ordinary Go recursion, one stack frame per union/intersection member
// nesting level; a raw, non-normalized deeply nested union or intersection
// costs one real frame per level and is not bounded by any cycle guard here.
func stopDepth(t typ.Type, depth int) bool {
	return t == nil || depth > typ.DefaultRecursionDepth
}
