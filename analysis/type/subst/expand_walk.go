package subst

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func expandInstantiatedGuardMode(t typ.Type, state *expandState, mode expandMode) typ.Type {
	if t == nil {
		return t
	}
	if mode == expandModeStructural && !typ.ContainsInstantiated(t) {
		return t
	}

	key := expandMemoKey{t: t, mode: mode}
	if cached, ok := state.memo[key]; ok {
		return cached
	}

	orig := t
	t = unwrap.Annotations(t)

	key = expandMemoKey{t: orig, mode: mode}
	state.memo[key] = orig
	result := expandInstantiatedCore(t, orig, state, mode)
	state.memo[key] = result
	return result
}

func expandInstantiatedCore(t typ.Type, orig typ.Type, state *expandState, mode expandMode) typ.Type {
	switch v := t.(type) {
	case *typ.Instantiated:
		return expandInstantiatedGeneric(v, orig, state)
	case *typ.Optional:
		return expandOptional(v, orig, state, mode)
	case *typ.Union:
		return expandUnion(v, orig, state, mode)
	case *typ.Intersection:
		return expandIntersection(v, orig, state, mode)
	case *typ.Array:
		return expandArray(v, orig, state, mode)
	case *typ.Map:
		return expandMap(v, orig, state, mode)
	case *typ.ReadonlyMap:
		return expandReadonlyMap(v, orig, state, mode)
	case *typ.Tuple:
		return expandTuple(v, orig, state, mode)
	case *typ.Function:
		return expandFunction(v, orig, state, mode)
	case *typ.Record:
		return expandRecord(v, orig, state, mode)
	case *typ.Alias:
		return expandAlias(v, orig, state, mode)
	case *typ.Interface:
		return expandInterface(v, orig, state, mode)
	case *typ.Ref, *typ.Generic:
		return orig
	default:
		return orig
	}
}
