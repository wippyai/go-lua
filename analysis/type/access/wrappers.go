package access

import (
	"github.com/wippyai/go-lua/analysis/type/internal/graph"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func descendAccessWrappers[T any](
	t typ.Type,
	depth int,
	active *graph.Path,
	descend func(typ.Type, int) T,
	optionalize func(T) T,
) T {
	if stopDepth(t, depth) {
		var zero T
		return zero
	}
	if active == nil {
		active = &graph.Path{}
	}
	if !active.Enter(t) {
		var zero T
		return zero
	}
	defer active.Leave(t)

	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return optionalize(descendAccessWrappers(v.Inner, depth+1, active, descend, optionalize))
	case *typ.Alias:
		return descendAccessWrappers(v.UnaliasedTarget(), depth+1, active, descend, optionalize)
	case *typ.TypeParam:
		if v.Constraint == nil {
			var zero T
			return zero
		}
		return descendAccessWrappers(v.Constraint, depth+1, active, descend, optionalize)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			var zero T
			return zero
		}
		return descendAccessWrappers(v.Body, depth+1, active, descend, optionalize)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			var zero T
			return zero
		}
		return descendAccessWrappers(expanded, depth+1, active, descend, optionalize)
	default:
		return descend(t, depth)
	}
}
