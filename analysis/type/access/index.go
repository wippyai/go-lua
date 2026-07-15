package access

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type indexMode uint8

const (
	indexStatic indexMode = iota
	indexRuntime
	indexWrite
)

// Index resolves a bracket-index projection against a type using static type
// facts only.
func Index(container typ.Type, key typ.Type) (typ.Type, bool) {
	return newQuery().index(container, key, 0, indexStatic, fieldResult{}).materialize()
}

// RuntimeIndex resolves a bracket-index projection with Lua read semantics:
// missing table slots can produce nil, but non-table indexing still fails.
func RuntimeIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	return newQuery().index(container, key, 0, indexRuntime, fieldResult{}).materialize()
}

// WritableIndex resolves the value contract for a bracket-index assignment
// target. It uses index admissibility, but does not add nil just because a read
// from the same slot could miss. Declared optional element/member types still
// carry their own nilability.
func WritableIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	return newQuery().index(container, key, 0, indexWrite, fieldResult{}).materialize()
}

func (q *query) index(container typ.Type, key typ.Type, depth int, mode indexMode, cycle fieldResult) fieldResult {
	if stopDepth(container, depth) {
		return fieldResult{}
	}
	if top, ok := SpecialAccessType(container); ok {
		return fieldResult{t: top, ok: true}
	}
	visit := queryKey{op: 3, t: container, key: key, mode: mode}
	if !q.enter(visit) {
		return cycle
	}
	defer q.leave(visit)

	res := q.indexCore(container, key, depth, mode)
	if res.ok || mode != indexRuntime {
		return res
	}
	if q.missingFieldReadsNil(container, depth+1, false) {
		return fieldResult{t: typ.Nil, ok: true}
	}
	return fieldResult{}
}

func (q *query) indexCore(container typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	return descendAccessWrappers(container, depth, nil, func(t typ.Type, depth int) fieldResult {
		if top, ok := SpecialAccessType(t); ok {
			return fieldResult{t: top, ok: true}
		}
		switch v := unwrap.Annotated(t).(type) {
		case *typ.Record:
			return q.indexInRecord(v, key, depth+1, mode)
		case *typ.Map:
			return q.indexInMap(v.Key, v.Value, key, depth+1, mode)
		case *typ.ReadonlyMap:
			return q.indexInMap(v.Key, v.Value, key, depth+1, mode)
		case *typ.Array:
			return q.indexInArray(v, key, depth+1, mode)
		case *typ.Tuple:
			return q.indexInTuple(v, key, depth+1, mode)
		case *typ.Union:
			return q.indexInUnion(v, key, depth+1, mode)
		case *typ.Intersection:
			return q.indexInIntersection(v, key, depth+1, mode)
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
