package access

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (q *query) indexInArray(a *typ.Array, key typ.Type, depth int, mode indexMode) fieldResult {
	if a == nil {
		return fieldResult{}
	}
	return q.indexByKeyVariants(key, depth, mode, true, fieldResult{}, func(key typ.Type) fieldResult {
		if mode == indexRuntime {
			if !q.arrayRuntimeKeyMayBeInteger(key, depth+1) {
				return fieldResult{}
			}
		} else if !subtype.IsSubtype(key, typ.Integer) {
			return fieldResult{}
		}
		elem := a.Element
		if elem == nil {
			elem = typ.Unknown
		}
		return fieldResult{t: elem, ok: true, nilable: mode != indexWrite}
	})
}

func (q *query) arrayRuntimeKeyMayBeInteger(key typ.Type, depth int) bool {
	if stopDepth(key, depth) {
		// May-contain query (invariants.md Rule 1 dual): stopping without a
		// definitive answer must not narrow a runtime read to "never an
		// integer key". A false here would make the caller treat the array
		// element as unreachable and fall back to a bare-nil read type,
		// silently dropping a runtime-possible value from the result.
		return true
	}
	if typ.IsAny(key) || typ.IsUnknown(key) {
		return true
	}
	visit := queryKey{op: 4, t: key}
	if !q.enter(visit) {
		return false
	}
	defer q.leave(visit)
	return descendAccessWrappers(key, depth, nil, trueThunk, func(key typ.Type, depth int) bool {
		if typ.IsAny(key) || typ.IsUnknown(key) {
			return true
		}
		switch v := unwrap.Annotated(key).(type) {
		case *typ.Literal:
			switch v.Base {
			case kind.Integer:
				return true
			case kind.Number:
				number, ok := v.Value.(float64)
				return ok && number == float64(int64(number))
			default:
				return false
			}
		case *typ.Union:
			for _, member := range v.Members {
				if q.arrayRuntimeKeyMayBeInteger(member, depth+1) {
					return true
				}
			}
			return false
		default:
			return v.Kind() == kind.Integer || v.Kind() == kind.Number
		}
	}, func(v bool) bool { return v })
}
