package subst

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expandInstantiatedGuardMode(t typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	if t == nil {
		return t
	}
	if mode == expandModeStructural && !inspect.ContainsInstantiated(t) {
		return t
	}

	key := expandMemoKey{t: t, mode: mode}
	if cached, ok := memo[key]; ok {
		return cached
	}

	next, ok := guard.Enter()
	if !ok {
		return t
	}

	orig := t
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == t {
			break
		}
		t = ann.Inner
	}

	key = expandMemoKey{t: orig, mode: mode}
	memo[key] = orig
	result := expandInstantiatedCore(t, orig, next, memo, mode)
	memo[key] = result
	return result
}

func expandInstantiatedCore(t typ.Type, orig typ.Type, guard recursion.Guard, memo map[expandMemoKey]typ.Type, mode expandMode) typ.Type {
	switch v := t.(type) {
	case *typ.Instantiated:
		return expandInstantiatedGeneric(v, orig, guard, memo)
	case *typ.Optional:
		return expandOptional(v, orig, guard, memo, mode)
	case *typ.Union:
		return expandUnion(v, orig, guard, memo, mode)
	case *typ.Intersection:
		return expandIntersection(v, orig, guard, memo, mode)
	case *typ.Array:
		return expandArray(v, orig, guard, memo, mode)
	case *typ.Map:
		return expandMap(v, orig, guard, memo, mode)
	case *typ.ReadonlyMap:
		return expandReadonlyMap(v, orig, guard, memo, mode)
	case *typ.Tuple:
		return expandTuple(v, orig, guard, memo, mode)
	case *typ.Function:
		return expandFunction(v, orig, guard, memo, mode)
	case *typ.Record:
		return expandRecord(v, orig, guard, memo, mode)
	case *typ.Alias:
		return expandAlias(v, orig, guard, memo, mode)
	case *typ.Interface:
		return expandInterface(v, orig, guard, memo, mode)
	case *typ.Ref, *typ.Generic:
		return orig
	default:
		return orig
	}
}
