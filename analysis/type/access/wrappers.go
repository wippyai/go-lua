package access

import (
	"github.com/wippyai/go-lua/analysis/type/internal/graph"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// falseThunk and trueThunk are the two boolean descendAccessWrappers
// stop-value thunks used across this package: falseThunk for a positive
// predicate (fails closed at exhaustion), trueThunk for a may-contain
// predicate (invariants.md Rule 1 dual).
func falseThunk() bool { return false }
func trueThunk() bool  { return true }

// descendAccessWrappers unwraps Optional/Alias/TypeParam/Recursive/
// Instantiated layers one at a time until it reaches a type descend can
// handle directly. The loop is iterative (not recursive) so unwrapping costs
// O(1) Go stack regardless of chain length; stopDepth's depth budget is
// still consulted every iteration as the termination backstop for a
// non-cyclic chain (invariants.md Rule 1), since bounding stack usage is not
// the same as bounding total work. stopValue is called lazily, only on an
// actual stop (missing type, depth exhaustion, a structural cycle, an
// unconstrained type param, a self-recursive body, an unexpandable
// instantiation); it is never invoked on the ordinary descend path, so a
// caller whose fallback is expensive (e.g. allocates) pays nothing when the
// walk completes normally. Callers resolving a concrete type (fieldResult)
// pass a thunk returning its zero
// value ({ok: false}, "unresolved"); a boolean may-contain caller
// (invariants.md Rule 1 dual) must pass a thunk returning true, since a
// hardcoded false zero value would silently assert "never" on a query whose
// whole point is "maybe".
func descendAccessWrappers[T any](
	t typ.Type,
	depth int,
	active *graph.Path,
	stopValue func() T,
	descend func(typ.Type, int) T,
	optionalize func(T) T,
) T {
	if active == nil {
		active = &graph.Path{}
	}
	entered := make([]typ.Type, 0, 8)
	optionalDepth := 0
	finish := func(value T) T {
		for optionalDepth > 0 {
			value = optionalize(value)
			optionalDepth--
		}
		for index := len(entered) - 1; index >= 0; index-- {
			active.Leave(entered[index])
		}
		return value
	}
	for {
		if stopDepth(t, depth) || !active.Enter(t) {
			return finish(stopValue())
		}
		entered = append(entered, t)
		switch v := unwrap.Annotated(t).(type) {
		case *typ.Optional:
			optionalDepth++
			t = v.Inner
		case *typ.Alias:
			// Peel exactly one wrapper per iteration so depth accounting
			// matches the chain's real length. UnaliasedTarget short-circuits
			// a fully-flattened alias (typ.NewAlias precomputes it) but falls
			// back to its own recursive walk for a raw, unflattened chain,
			// which would reintroduce unbounded stack growth here.
			t = v.Target
		case *typ.TypeParam:
			if v.Constraint == nil {
				return finish(stopValue())
			}
			t = v.Constraint
		case *typ.Recursive:
			if v.Body == nil || v.Body == t {
				return finish(stopValue())
			}
			t = v.Body
		case *typ.Instantiated:
			expanded := subst.ExpandInstantiated(v)
			if expanded == nil || expanded == t {
				return finish(stopValue())
			}
			t = expanded
		default:
			return finish(descend(t, depth))
		}
		depth++
	}
}
