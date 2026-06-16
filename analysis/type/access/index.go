package access

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type indexMode uint8

const (
	indexStatic indexMode = iota
	indexRuntime
)

// Index resolves a bracket-index projection against a type using static type
// facts only.
func Index(container typ.Type, key typ.Type) (typ.Type, bool) {
	return indexDepth(container, key, 0, indexStatic).materialize()
}

// RuntimeIndex resolves a bracket-index projection with Lua read semantics:
// missing table slots can produce nil, but non-table indexing still fails.
func RuntimeIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	return indexDepth(container, key, 0, indexRuntime).materialize()
}

func indexDepth(container typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	if stopDepth(container, depth) {
		return fieldResult{}
	}
	if top, ok := specialAccessType(container); ok {
		return fieldResult{t: top, ok: true}
	}

	res := indexDepthCore(container, key, depth, mode)
	if res.ok || mode != indexRuntime {
		return res
	}
	if missingFieldReadsNilDepth(container, depth+1) {
		return fieldResult{t: typ.Nil, ok: true}
	}
	return fieldResult{}
}

func indexDepthCore(container typ.Type, key typ.Type, depth int, mode indexMode) fieldResult {
	return descendAccessWrappers(container, depth, func(t typ.Type, depth int) fieldResult {
		if top, ok := specialAccessType(t); ok {
			return fieldResult{t: top, ok: true}
		}
		switch v := unwrap.Annotated(t).(type) {
		case *typ.Record:
			return indexInRecord(v, key, depth+1, mode)
		case *typ.Map:
			return indexInMap(v.Key, v.Value, key, depth+1, mode)
		case *typ.ReadonlyMap:
			return indexInMap(v.Key, v.Value, key, depth+1, mode)
		case *typ.Array:
			return indexInArray(v, key, depth+1, mode)
		case *typ.Tuple:
			return indexInTuple(v, key, depth+1, mode)
		case *typ.Union:
			return indexInUnion(v, key, depth+1, mode)
		case *typ.Intersection:
			return indexInIntersection(v, key, depth+1, mode)
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
