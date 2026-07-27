package access

import (
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
	if typ.IsBuiltinTableTopMarker(t) {
		return typ.Any, true
	}
	return nil, false
}

// stopDepth reports whether a field/index resolution lacks a type to inspect.
//
// The depth argument is retained while the query helpers share their existing
// call shape, but it is deliberately not a semantic budget. query.enter and
// descendAccessWrappers' graph.Path detect repeated interned type nodes, while
// distinct finite chains are allowed to reach their exact leaf.
func stopDepth(t typ.Type, depth int) bool {
	return t == nil
}
